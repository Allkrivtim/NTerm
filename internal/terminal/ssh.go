package terminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexandr/nterm/internal/domain"
	"github.com/alexandr/nterm/internal/gitstatus"
	"github.com/alexandr/nterm/internal/secrets"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const remoteMetadataPrefix = "\x1eNTERM_META\x1f"

const (
	sshKeepaliveInterval = 15 * time.Second
	sshKeepaliveTimeout  = 12 * time.Second
	sshReconnectAttempts = 6
)

type SSHStatus struct {
	State     string `json:"state"`
	Label     string `json:"label"`
	Message   string `json:"message,omitempty"`
	LatencyMS int64  `json:"latencyMs,omitempty"`
	Attempt   int    `json:"attempt,omitempty"`
}

type SSHHostKey struct {
	Hostname    string
	Address     string
	Algorithm   string
	Fingerprint string
}

// remoteHelper is deliberately small and POSIX-only. It is copied to a
// private temporary directory after connecting and removed on disconnect.
// It does not start a daemon and never changes shell startup files.
const remoteHelper = `#!/bin/sh
case "$1" in
  complete)
    cwd=$2
    token=$3
    case "$token" in
      ~/*) expanded=$HOME/${token#~/} ;;
      /*) expanded=$token ;;
      *) expanded=$cwd/$token ;;
    esac
    case "$expanded" in
      */*) directory=${expanded%/*}; leaf=${expanded##*/} ;;
      *) directory=$cwd; leaf=$expanded ;;
    esac
    [ -d "$directory" ] || exit 0
    for path in "$directory"/"$leaf"*; do
      [ -e "$path" ] || [ -L "$path" ] || continue
      name=${path##*/}
      case "$token" in
        */*) prefix=${token%/*}/ ;;
        *) prefix= ;;
      esac
      kind=f
      [ -d "$path" ] && kind=d
      printf '%s\037%s\036' "$prefix$name" "$kind"
    done
    ;;
  commands)
    shell=$2
    case "${shell##*/}" in
      zsh) "$shell" -lic 'printf "%s\n" ${(k)commands} ${(k)aliases} ${(k)builtins} ${(k)functions}' 2>/dev/null ;;
      bash) "$shell" -lic 'compgen -c' 2>/dev/null ;;
      *) printf '%s\n' cd echo export history jobs kill pwd source type unset which ;;
    esac
    ;;
  git)
    cd "$2" 2>/dev/null || exit 2
    root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 3
    printf '%s\000' "$root"
    GIT_OPTIONAL_LOCKS=0 LC_ALL=C git status --porcelain=v2 --branch --untracked-files=normal 2>/dev/null
    ;;
esac
`

type sshSession struct {
	client      *ssh.Client
	connection  net.Conn
	destination string
	profile     SSHProfile
	helperPath  string
	shell       string
	home        string
	hostname    string
	platform    string
	environment string

	mu            sync.RWMutex
	reconnectMu   sync.Mutex
	cwd           string
	commands      []string
	closed        bool
	connected     bool
	reconnecting  bool
	keepaliveStop chan struct{}
	keepaliveDone chan struct{}
	statusHandler func(SSHStatus)
	lastActivity  time.Time
	latency       time.Duration
	commandShell  *blockShell
}

var knownHostsMu sync.Mutex

type RemotePathEntry struct {
	Value string
	IsDir bool
}

func resolveSSHProfile(arguments []string, destination string, profile SSHProfile) (SSHProfile, error) {
	if profile.Port == 0 {
		profile.Port = 22
	}
	for index := 0; index < len(arguments); index++ {
		value := arguments[index]
		if value == destination || value == "--" {
			continue
		}
		if !strings.HasPrefix(value, "-") {
			continue
		}
		option := value
		attached := ""
		if len(value) > 2 {
			option, attached = value[:2], value[2:]
		}
		readValue := func() (string, error) {
			if attached != "" {
				return attached, nil
			}
			index++
			if index >= len(arguments) {
				return "", fmt.Errorf("ssh: option %s needs a value", option)
			}
			return arguments[index], nil
		}
		switch option {
		case "-p":
			port, err := readValue()
			if err != nil {
				return SSHProfile{}, err
			}
			profile.Port, err = strconv.Atoi(port)
			if err != nil || profile.Port < 1 || profile.Port > 65535 {
				return SSHProfile{}, errors.New("ssh: port must be between 1 and 65535")
			}
		case "-i":
			path, err := readValue()
			if err != nil {
				return SSHProfile{}, err
			}
			profile.KeyPath = path
		case "-l":
			user, err := readValue()
			if err != nil {
				return SSHProfile{}, err
			}
			profile.User = user
		default:
			return SSHProfile{}, fmt.Errorf("ssh: option %s is not supported by the built-in client", value)
		}
	}
	if profile.Host == "" {
		host := strings.TrimSpace(destination)
		if separator := strings.LastIndex(host, "@"); separator >= 0 {
			if profile.User == "" {
				profile.User = host[:separator]
			}
			host = host[separator+1:]
		}
		profile.Host = strings.Trim(host, "[]")
	}
	profile.Host = strings.Trim(profile.Host, "[]")
	if profile.User == "" {
		profile.User = strings.TrimSpace(os.Getenv("USER"))
	}
	if profile.Host == "" || profile.User == "" {
		return SSHProfile{}, errors.New("ssh: host and user are required")
	}
	if len(profile.Host) > 253 || len(profile.User) > 128 {
		return SSHProfile{}, errors.New("ssh: host or user is too long")
	}
	if strings.ContainsAny(profile.Host+profile.User, " \t\r\n\x00") {
		return SSHProfile{}, errors.New("ssh: host and user cannot contain whitespace")
	}
	if strings.ContainsAny(profile.Host, "/\\") {
		return SSHProfile{}, errors.New("ssh: host cannot contain a path")
	}
	if strings.Contains(profile.Host, ":") && net.ParseIP(profile.Host) == nil {
		return SSHProfile{}, errors.New("ssh: host must not include a port; use -p")
	}
	if profile.KeyPath != "" {
		path, err := expandSSHPath(profile.KeyPath)
		if err != nil {
			return SSHProfile{}, err
		}
		profile.KeyPath = path
	}
	return profile, nil
}

func sshAuthMethods(profile SSHProfile) ([]ssh.AuthMethod, func(), error) {
	methods := make([]ssh.AuthMethod, 0, 4)
	signers := make([]ssh.Signer, 0, 4)
	cleanup := func() {}
	if socket := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK")); socket != "" {
		connection, err := net.DialTimeout("unix", socket, 750*time.Millisecond)
		if err == nil {
			agentClient := agent.NewClient(connection)
			if agentSigners, signerErr := agentClient.Signers(); signerErr == nil {
				signers = append(signers, agentSigners...)
			}
			cleanup = func() { _ = connection.Close() }
		}
	}
	password := ""
	if profile.CredentialAccount != "" {
		var err error
		password, err = secrets.Get(profile.CredentialAccount)
		if err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("ssh: %w", err)
		}
	}
	keyPaths := []string{}
	if profile.KeyPath != "" {
		keyPaths = append(keyPaths, profile.KeyPath)
	} else if home, err := os.UserHomeDir(); err == nil {
		for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			keyPaths = append(keyPaths, filepath.Join(home, ".ssh", name))
		}
	}
	for _, path := range keyPaths {
		signer, err := readPrivateKey(path, password)
		if err != nil {
			if profile.KeyPath != "" {
				cleanup()
				return nil, func() {}, err
			}
			continue
		}
		signers = append(signers, signer)
	}
	if len(signers) > 0 {
		// x/crypto/ssh groups authentication attempts by method name. Keeping all
		// signers in one publickey method ensures an empty agent cannot shadow an
		// explicitly selected private key.
		methods = append(methods, ssh.PublicKeys(signers...))
	}
	if password != "" {
		methods = append(methods, ssh.Password(password))
		methods = append(methods, ssh.KeyboardInteractive(func(_ string, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for index := range answers {
				answers[index] = password
			}
			return answers, nil
		}))
	}
	return methods, cleanup, nil
}

func readPrivateKey(path, passphrase string) (ssh.Signer, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("ssh: inspect private key: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("ssh: private key %s is not a regular file", filepath.Base(path))
	}
	if info.Size() > 16*1024*1024 {
		return nil, fmt.Errorf("ssh: private key %s is unexpectedly large", filepath.Base(path))
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("ssh: private key %s has unsafe permissions %04o; run chmod 600 %s", filepath.Base(path), info.Mode().Perm(), shellQuote(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ssh: read private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err == nil {
		return signer, nil
	}
	var passphraseMissing *ssh.PassphraseMissingError
	if errors.As(err, &passphraseMissing) && passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
	}
	if err != nil {
		return nil, fmt.Errorf("ssh: parse private key %s: %w", filepath.Base(path), err)
	}
	return signer, nil
}

func expandSSHPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return filepath.Abs(path)
}

func nativeHostKeyCallback(confirm func(SSHHostKey) bool) (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("ssh: locate known_hosts: %w", err)
	}
	directory := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("ssh: create .ssh directory: %w", err)
	}
	path := filepath.Join(directory, "known_hosts")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("ssh: open known_hosts: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("ssh: close known_hosts: %w", err)
	}
	verify, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("ssh: read known_hosts: %w", err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := verify(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyError *knownhosts.KeyError
		if !errors.As(err, &keyError) || len(keyError.Want) > 0 {
			return fmt.Errorf("ssh: host key changed for %s; refusing connection: %w", hostname, err)
		}
		info := SSHHostKey{
			Hostname: hostname, Address: remote.String(), Algorithm: key.Type(),
			Fingerprint: ssh.FingerprintSHA256(key),
		}
		if confirm == nil || !confirm(info) {
			return fmt.Errorf("ssh: untrusted host key for %s (%s %s)", hostname, info.Algorithm, info.Fingerprint)
		}
		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()
		// The key may have been stored while the confirmation dialog was open.
		// Re-read known_hosts under the lock to avoid duplicate entries and to
		// still refuse a key that changed during confirmation.
		currentVerify, verifyErr := knownhosts.New(path)
		if verifyErr != nil {
			return fmt.Errorf("ssh: re-read known_hosts: %w", verifyErr)
		}
		if currentErr := currentVerify(hostname, remote, key); currentErr == nil {
			return nil
		} else {
			var currentKeyError *knownhosts.KeyError
			if !errors.As(currentErr, &currentKeyError) || len(currentKeyError.Want) > 0 {
				return fmt.Errorf("ssh: host key changed for %s during confirmation; refusing connection: %w", hostname, currentErr)
			}
		}
		entry := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key) + "\n"
		knownHostsFile, openErr := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if openErr != nil {
			return fmt.Errorf("ssh: save host key: %w", openErr)
		}
		_, writeErr := io.WriteString(knownHostsFile, entry)
		closeErr := knownHostsFile.Close()
		if writeErr != nil {
			return fmt.Errorf("ssh: save host key: %w", writeErr)
		}
		return closeErr
	}, nil
}

func connectSSH(ctx context.Context, arguments []string, destination string, profile SSHProfile) (*sshSession, error) {
	profile, err := resolveSSHProfile(arguments, destination, profile)
	if err != nil {
		return nil, err
	}
	remote, err := dialSSH(ctx, profile)
	if err != nil {
		return nil, err
	}
	remote.profile = profile
	remote.connected = true
	remote.lastActivity = time.Now()
	remote.keepaliveStop = make(chan struct{})
	remote.keepaliveDone = make(chan struct{})
	go remote.keepaliveLoop()
	go remote.watchClient(remote.client)
	return remote, nil
}

func dialSSH(ctx context.Context, profile SSHProfile) (*sshSession, error) {
	started := time.Now()
	auth, cleanupAuth, err := sshAuthMethods(profile)
	if err != nil {
		return nil, err
	}
	defer cleanupAuth()
	hostKeyCallback, err := nativeHostKeyCallback(profile.ConfirmHostKey)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(profile.Host, strconv.Itoa(profile.Port))
	config := &ssh.ClientConfig{
		User: profile.User, Auth: auth, HostKeyCallback: hostKeyCallback,
		Timeout: 20 * time.Second,
	}
	dialer := &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("ssh: connect to %s: %w", address, err)
	}
	if tcp, ok := connection.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}
	_ = connection.SetDeadline(time.Now().Add(20 * time.Second))
	clientConnection, channels, requests, err := ssh.NewClientConn(connection, address, config)
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("ssh: handshake failed: %w", err)
	}
	_ = connection.SetDeadline(time.Time{})
	remote := &sshSession{
		client: ssh.NewClient(clientConnection, channels, requests), connection: connection,
		destination: profile.User + "@" + profile.Host, profile: profile,
		connected: true, lastActivity: time.Now(),
	}
	cleanup := true
	defer func() {
		if cleanup {
			remote.close(true)
		}
	}()

	probeCommand := `os_id=""; if [ -r /etc/os-release ]; then os_id=$(sed -n 's/^ID=//p' /etc/os-release | head -n 1); fi; [ -n "$os_id" ] || os_id=$(uname -s 2>/dev/null); printf '%s\000%s\000%s\000%s\000%s' "$PWD" "$HOME" "$SHELL" "$(hostname -s 2>/dev/null || hostname)" "$os_id"`
	probe, err := remote.exec(ctx, nil, probeCommand)
	if err != nil {
		return nil, fmt.Errorf("ssh: probe remote shell: %w", err)
	}
	fields := bytes.SplitN(probe, []byte{0}, 5)
	if len(fields) != 5 || len(fields[0]) == 0 {
		return nil, errors.New("ssh: remote shell returned invalid session details")
	}
	remote.cwd, remote.home, remote.shell, remote.hostname = string(fields[0]), string(fields[1]), string(fields[2]), string(fields[3])
	remote.platform = normalizeServerPlatform(string(fields[4]))
	if remote.platform == "server" {
		remote.platform = normalizeServerPlatform(remote.hostname)
	}
	if remote.shell == "" {
		remote.shell = "/bin/sh"
	}

	if profile.HelperEnabled {
		install := `umask 077; d=$(mktemp -d "${TMPDIR:-/tmp}/nterm.XXXXXXXX") || exit 1; cat > "$d/helper" || exit 1; chmod 700 "$d/helper"; printf '%s' "$d/helper"`
		path, err := remote.exec(ctx, strings.NewReader(remoteHelper), install)
		if err != nil {
			return nil, fmt.Errorf("ssh: install temporary helper: %w", err)
		}
		remote.helperPath = strings.TrimSpace(string(path))
		if remote.helperPath == "" {
			return nil, errors.New("ssh: temporary helper path is empty")
		}
	}
	remote.latency = time.Since(started)
	cleanup = false
	return remote, nil
}

func (r *sshSession) setStatusHandler(handler func(SSHStatus)) {
	r.mu.Lock()
	r.statusHandler = handler
	status := r.statusLocked("", 0)
	r.mu.Unlock()
	if handler != nil {
		handler(status)
	}
}

func (r *sshSession) statusLocked(message string, attempt int) SSHStatus {
	state := "disconnected"
	if r.reconnecting {
		state = "reconnecting"
	} else if r.connected {
		state = "connected"
	}
	return SSHStatus{
		State: state, Label: r.labelLocked(), Message: message,
		LatencyMS: r.latency.Milliseconds(), Attempt: attempt,
	}
}

func (r *sshSession) notifyStatus(message string, attempt int) {
	r.mu.RLock()
	handler := r.statusHandler
	status := r.statusLocked(message, attempt)
	r.mu.RUnlock()
	if handler != nil {
		handler(status)
	}
}

func (r *sshSession) labelLocked() string {
	if r.hostname != "" {
		return r.hostname
	}
	return r.destination
}

func (r *sshSession) touch() {
	r.mu.Lock()
	r.lastActivity = time.Now()
	r.mu.Unlock()
}

func (r *sshSession) keepaliveLoop() {
	defer close(r.keepaliveDone)
	ticker := time.NewTicker(sshKeepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.keepaliveStop:
			return
		case <-ticker.C:
			r.mu.RLock()
			idle := time.Since(r.lastActivity)
			connected := r.connected && !r.closed
			r.mu.RUnlock()
			if !connected || idle < sshKeepaliveInterval {
				continue
			}
			if err := r.keepalive(); err != nil {
				r.connectionLost(err)
			}
		}
	}
}

// watchClient reacts immediately when the SSH transport disappears. The
// keepalive loop remains the fallback for half-open TCP connections where the
// operating system has not reported a close yet.
func (r *sshSession) watchClient(client *ssh.Client) {
	if client == nil {
		return
	}
	err := client.Wait()
	r.mu.RLock()
	current := r.client == client && r.connected && !r.closed
	r.mu.RUnlock()
	if current {
		if err == nil {
			err = io.EOF
		}
		r.connectionLost(err)
	}
}

func (r *sshSession) keepalive() error {
	r.mu.RLock()
	client := r.client
	r.mu.RUnlock()
	if client == nil {
		return io.EOF
	}
	started := time.Now()
	done := make(chan error, 1)
	go func() {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			return err
		}
		r.mu.Lock()
		r.latency = time.Since(started)
		r.lastActivity = time.Now()
		r.mu.Unlock()
		r.notifyStatus("", 0)
		return nil
	case <-time.After(sshKeepaliveTimeout):
		_ = client.Close()
		return errors.New("keepalive timed out")
	case <-r.keepaliveStop:
		return context.Canceled
	}
}

func (r *sshSession) connectionLost(cause error) {
	r.mu.Lock()
	if r.closed || !r.connected {
		r.mu.Unlock()
		return
	}
	r.connected = false
	client := r.client
	commandShell := r.commandShell
	r.commandShell = nil
	r.mu.Unlock()
	if commandShell != nil {
		commandShell.Close()
	}
	if client != nil {
		_ = client.Close()
	}
	r.notifyStatus("Connection lost", 0)
	go func() { _ = r.reconnect(context.Background(), false) }()
}

func (r *sshSession) reconnect(ctx context.Context, immediate bool) error {
	r.reconnectMu.Lock()
	defer r.reconnectMu.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errors.New("ssh: session is closed")
	}
	if r.connected {
		r.mu.Unlock()
		return nil
	}
	r.reconnecting = true
	profile := r.profile
	desiredCWD := r.cwd
	handler := r.statusHandler
	r.mu.Unlock()
	var lastErr error
	for attempt := 1; attempt <= sshReconnectAttempts; attempt++ {
		if !immediate || attempt > 1 {
			delay := time.Duration(1<<min(attempt-1, 4)) * 500 * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				r.mu.Lock()
				r.reconnecting = false
				r.mu.Unlock()
				return ctx.Err()
			case <-r.keepaliveStop:
				timer.Stop()
				r.mu.Lock()
				r.reconnecting = false
				r.mu.Unlock()
				return errors.New("ssh: session is closed")
			case <-timer.C:
			}
		}
		r.notifyStatus("Reconnecting…", attempt)
		attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		fresh, err := dialSSH(attemptCtx, profile)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if desiredCWD != "" {
			checkCtx, checkCancel := context.WithTimeout(ctx, 4*time.Second)
			output, checkErr := fresh.exec(checkCtx, nil, "cd "+shellQuote(desiredCWD)+" 2>/dev/null && pwd")
			checkCancel()
			if checkErr == nil && strings.TrimSpace(string(output)) != "" {
				fresh.cwd = strings.TrimSpace(string(output))
			}
		}

		r.mu.Lock()
		if r.closed {
			r.reconnecting = false
			r.mu.Unlock()
			fresh.close(false)
			return errors.New("ssh: session is closed")
		}
		oldClient := r.client
		oldCommandShell := r.commandShell
		oldHelper := r.helperPath
		r.client, r.connection = fresh.client, fresh.connection
		newClient := r.client
		r.helperPath, r.shell, r.home = fresh.helperPath, fresh.shell, fresh.home
		newHelper := r.helperPath
		r.hostname, r.platform, r.cwd = fresh.hostname, fresh.platform, fresh.cwd
		r.commands = nil
		r.commandShell = nil
		r.connected = true
		r.reconnecting = false
		r.lastActivity = time.Now()
		r.latency = fresh.latency
		r.statusHandler = handler
		fresh.client = nil
		fresh.connection = nil
		r.mu.Unlock()
		if oldCommandShell != nil {
			oldCommandShell.Close()
		}
		go r.watchClient(newClient)
		if oldClient != nil {
			_ = oldClient.Close()
		}
		if oldHelper != "" && oldHelper != newHelper {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, _ = execSSHClient(cleanupCtx, newClient, nil, "rm -rf -- "+shellQuote(oldHelper))
			cleanupCancel()
		}
		r.notifyStatus("Reconnected", 0)
		return nil
	}
	r.mu.Lock()
	r.reconnecting = false
	r.mu.Unlock()
	r.notifyStatus("Could not reconnect", sshReconnectAttempts)
	if lastErr == nil {
		lastErr = errors.New("reconnect failed")
	}
	return fmt.Errorf("ssh: reconnect: %w", lastErr)
}

func (r *sshSession) ensureConnected(ctx context.Context) error {
	r.mu.RLock()
	connected, closed := r.connected, r.closed
	r.mu.RUnlock()
	if closed {
		return errors.New("ssh: session is closed")
	}
	if connected {
		return nil
	}
	return r.reconnect(ctx, true)
}

func (r *sshSession) exec(ctx context.Context, stdin io.Reader, command string) ([]byte, error) {
	if err := r.ensureConnected(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	client := r.client
	r.mu.RUnlock()
	output, err := execSSHClient(ctx, client, stdin, command)
	if err != nil && isSSHTransportError(err) && r.isManaged() {
		r.connectionLost(err)
	}
	if err == nil {
		r.touch()
	}
	return output, err
}

func (r *sshSession) isManaged() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.keepaliveStop != nil && !r.closed
}

func execSSHClient(ctx context.Context, client *ssh.Client, stdin io.Reader, command string) ([]byte, error) {
	if client == nil {
		return nil, io.EOF
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh: open channel: %w", err)
	}
	defer session.Close()
	session.Stdin = stdin
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()
	select {
	case <-ctx.Done():
		_ = session.Close()
		return stdout.Bytes(), ctx.Err()
	case err = <-done:
	}
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return stdout.Bytes(), errors.New(message)
		}
	}
	return stdout.Bytes(), err
}

func (r *sshSession) run(ctx context.Context, session *Session, block *domain.Block, emit func(domain.OutputChunk)) error {
	if err := r.ensureConnected(ctx); err != nil {
		emit(domain.OutputChunk{BlockID: block.ID, Stream: "stderr", Data: "SSH is offline. Reconnect failed: " + err.Error() + "\n"})
		return err
	}
	session.mu.RLock()
	cols, rows := session.cols, session.rows
	session.mu.RUnlock()
	commandShell, err := r.ensureCommandShell(cols, rows)
	if err != nil {
		if isSSHTransportError(err) {
			r.connectionLost(err)
		}
		return err
	}
	_ = commandShell.Resize(cols, rows)
	result, waitErr := commandShell.Execute(ctx, block.Command, func(value []byte) {
		r.touch()
		emit(domain.NewOutputChunkBytes(block.ID, "stdout", value))
	}, func() error {
		return session.attachInput(commandShell, commandShell.Resize, nil)
	})
	session.mu.Lock()
	if session.activeInput == commandShell {
		session.activeInput = nil
		session.activeResize = nil
		session.activeClose = nil
	}
	session.mu.Unlock()
	r.mu.Lock()
	if commandShell.Closed() && r.commandShell == commandShell {
		r.commandShell = nil
	}
	if result.CWD != "" {
		r.cwd = result.CWD
	}
	r.environment = result.Environment
	r.mu.Unlock()
	if result.CWD != "" {
		block.FinalCWD = result.CWD
	}
	block.Environment = result.Environment
	if result.Status != 0 && waitErr == nil {
		return exitError{code: result.Status}
	}
	if isSSHTransportError(waitErr) {
		r.connectionLost(waitErr)
		emit(domain.OutputChunk{
			BlockID: block.ID, Stream: "stderr",
			Data: "\nSSH connection lost. Reconnecting in the background; this command was not replayed.\n",
		})
		return fmt.Errorf("ssh: connection lost; the command was not replayed: %w", waitErr)
	}
	if waitErr == nil {
		r.touch()
	}
	return waitErr
}

func (r *sshSession) environmentLabel() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.environment
}

func (r *sshSession) ensureCommandShell(cols, rows uint16) (*blockShell, error) {
	r.mu.RLock()
	existing := r.commandShell
	client, shellPath, cwd := r.client, r.shell, r.cwd
	connected, closed := r.connected, r.closed
	r.mu.RUnlock()
	if closed || !connected || client == nil {
		return nil, errors.New("ssh: session is offline")
	}
	if existing != nil && !existing.Closed() {
		return existing, nil
	}
	started, err := startRemoteBlockShell(client, shellPath, cwd, cols, rows, r.touch)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.client != client || !r.connected || r.closed {
		r.mu.Unlock()
		started.Close()
		return nil, errors.New("ssh: connection changed while opening terminal")
	}
	if r.commandShell != nil && !r.commandShell.Closed() {
		existing = r.commandShell
		r.mu.Unlock()
		started.Close()
		return existing, nil
	}
	r.commandShell = started
	r.mu.Unlock()
	return started, nil
}

func startRemoteBlockShell(client *ssh.Client, shellPath, cwd string, cols, rows uint16, touch func()) (*blockShell, error) {
	channel, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh: open persistent terminal channel: %w", err)
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 38400, ssh.TTY_OP_OSPEED: 38400}
	if err := channel.RequestPty("xterm-256color", int(rows), int(cols), modes); err != nil {
		_ = channel.Close()
		return nil, fmt.Errorf("ssh: request persistent PTY: %w", err)
	}
	input, err := channel.StdinPipe()
	if err != nil {
		_ = channel.Close()
		return nil, fmt.Errorf("ssh: open persistent terminal input: %w", err)
	}
	reader, writer := io.Pipe()
	output := &lockedWriter{writer: writer, afterWrite: touch}
	channel.Stdout = output
	channel.Stderr = output
	remoteCommand := "cd " + shellQuote(cwd) + " || exit 1; " +
		"export TERM=xterm-256color COLORTERM=truecolor CLICOLOR=1 CLICOLOR_FORCE=1; exec " +
		shellQuote(shellPath) + " -l -i"
	if err := channel.Start(remoteCommand); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		_ = channel.Close()
		return nil, fmt.Errorf("ssh: start persistent shell: %w", err)
	}
	result := &blockShell{
		input: input,
		resize: func(cols, rows uint16) error {
			return channel.WindowChange(int(rows), int(cols))
		},
	}
	result.closeTransport = func() {
		_ = input.Close()
		_ = channel.Close()
		_ = writer.Close()
	}
	go result.readLoop(reader)
	go func() {
		waitErr := channel.Wait()
		_ = writer.CloseWithError(waitErr)
		result.transportEnded(waitErr)
	}()
	return result, nil
}

func (r *sshSession) completePath(ctx context.Context, token string) []RemotePathEntry {
	r.mu.RLock()
	cwd, helper := r.cwd, r.helperPath
	r.mu.RUnlock()
	if helper == "" {
		return nil
	}
	output, err := r.exec(ctx, nil, "sh "+shellQuote(helper)+" complete "+shellQuote(cwd)+" "+shellQuote(strings.TrimPrefix(token, "\"")))
	if err != nil {
		return nil
	}
	items := make([]RemotePathEntry, 0, 16)
	for _, record := range bytes.Split(output, []byte{0x1e}) {
		fields := bytes.SplitN(record, []byte{0x1f}, 2)
		if len(fields) != 2 || len(fields[0]) == 0 {
			continue
		}
		value := string(fields[0])
		isDir := string(fields[1]) == "d"
		if isDir {
			value += "/"
		}
		items = append(items, RemotePathEntry{Value: value, IsDir: isDir})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value < items[j].Value })
	return items
}

func (r *sshSession) commandNames(ctx context.Context) []string {
	r.mu.RLock()
	helper, shell := r.helperPath, r.shell
	if len(r.commands) > 0 {
		result := append([]string(nil), r.commands...)
		r.mu.RUnlock()
		return result
	}
	r.mu.RUnlock()
	if helper == "" {
		return nil
	}
	output, err := r.exec(ctx, nil, "sh "+shellQuote(helper)+" commands "+shellQuote(shell))
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, 256)
	for _, line := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || len(name) > 128 || strings.ContainsAny(name, " \t\r\n\x00") {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	sort.Strings(result)
	r.mu.Lock()
	if len(r.commands) == 0 {
		r.commands = append([]string(nil), result...)
	}
	r.mu.Unlock()
	return result
}

func (r *sshSession) gitContext(ctx context.Context) (gitstatus.Info, bool) {
	r.mu.RLock()
	cwd, helper := r.cwd, r.helperPath
	r.mu.RUnlock()
	if helper == "" {
		return gitstatus.Info{}, true
	}
	output, err := r.exec(ctx, nil, "sh "+shellQuote(helper)+" git "+shellQuote(cwd))
	if err != nil {
		return gitstatus.Info{}, true
	}
	separator := bytes.IndexByte(output, 0)
	if separator < 1 {
		return gitstatus.Info{}, true
	}
	root := string(output[:separator])
	info := gitstatus.Info{IsRepository: true, Root: root, Name: filepath.Base(root)}
	gitstatus.ParsePorcelainV2(string(output[separator+1:]), &info)
	return info, true
}

func (r *sshSession) label() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.labelLocked()
}

func normalizeServerPlatform(value string) string {
	normalized := strings.Trim(strings.ToLower(strings.TrimSpace(value)), "\"'")
	switch normalized {
	case "ubuntu":
		return "ubuntu"
	case "debian":
		return "debian"
	case "fedora":
		return "fedora"
	case "arch", "archlinux":
		return "arch"
	case "alpine":
		return "alpine"
	case "centos", "rhel", "redhat":
		return "centos"
	case "rocky", "almalinux":
		return "rocky"
	case "opensuse", "opensuse-leap", "sles", "suse":
		return "opensuse"
	case "freebsd":
		return "freebsd"
	case "darwin", "macos":
		return "macos"
	case "windows", "windows_nt":
		return "windows"
	}
	markers := [][2]string{
		{"ubuntu", "ubuntu"}, {"debian", "debian"}, {"fedora", "fedora"}, {"arch", "arch"},
		{"alpine", "alpine"}, {"centos", "centos"}, {"rocky", "rocky"}, {"suse", "opensuse"},
		{"freebsd", "freebsd"}, {"darwin", "macos"}, {"windows", "windows"},
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker[0]) {
			return marker[1]
		}
	}
	return "server"
}

func (r *sshSession) close(cleanRemote bool) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.connected = false
	helper := r.helperPath
	client := r.client
	commandShell := r.commandShell
	r.commandShell = nil
	stop := r.keepaliveStop
	done := r.keepaliveDone
	r.mu.Unlock()
	if commandShell != nil {
		commandShell.Close()
	}
	if cleanRemote && helper != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
		_, _ = execSSHClient(ctx, client, nil, "rm -rf -- "+shellQuote(filepath.Dir(helper)))
		cancel()
	}
	if stop != nil {
		close(stop)
	}
	if client != nil {
		_ = client.Close()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
	}
	r.notifyStatus("Disconnected", 0)
}

func isSSHTransportError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		return false
	}
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || strings.Contains(strings.ToLower(err.Error()), "connection") || strings.Contains(strings.ToLower(err.Error()), "broken pipe")
}

type remoteOutputFilter struct {
	blockID string
	emit    func(domain.OutputChunk)
	pending []byte
	prefix  []byte
	cwd     string
	status  int
}

func (f *remoteOutputFilter) Write(value []byte) (int, error) {
	written := len(value)
	f.pending = append(f.pending, value...)
	prefix := f.prefix
	if len(prefix) == 0 {
		prefix = []byte(remoteMetadataPrefix)
	}
	if index := bytes.Index(f.pending, prefix); index >= 0 {
		if index > 0 {
			f.emit(domain.NewOutputChunkBytes(f.blockID, "stdout", f.pending[:index]))
		}
		metadata := f.pending[index+len(prefix):]
		if end := bytes.IndexByte(metadata, 0x1e); end >= 0 {
			parts := bytes.SplitN(metadata[:end], []byte{0x1f}, 2)
			if len(parts) == 2 {
				f.status, _ = strconv.Atoi(string(parts[0]))
				f.cwd = string(parts[1])
			}
			remaining := bytes.TrimPrefix(metadata[end+1:], []byte("\n"))
			f.pending = append(f.pending[:0], remaining...)
		}
		return written, nil
	}
	keep := len(prefix) - 1
	if len(f.pending) > keep {
		cut := len(f.pending) - keep
		f.emit(domain.NewOutputChunkBytes(f.blockID, "stdout", f.pending[:cut]))
		f.pending = append(f.pending[:0], f.pending[cut:]...)
	}
	return written, nil
}

func (f *remoteOutputFilter) Flush() {
	if len(f.pending) > 0 {
		f.emit(domain.NewOutputChunkBytes(f.blockID, "stdout", f.pending))
		f.pending = nil
	}
}

type lockedWriter struct {
	mu         sync.Mutex
	writer     io.Writer
	afterWrite func()
}

func (w *lockedWriter) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	written, err := w.writer.Write(value)
	if written > 0 && w.afterWrite != nil {
		w.afterWrite()
	}
	return written, err
}

func parseSSHInvocation(command string) ([]string, string, bool, error) {
	words, err := splitShellWords(command)
	if err != nil || len(words) < 2 || filepath.Base(words[0]) != "ssh" {
		return nil, "", false, err
	}
	valueOptions := map[string]bool{"-B": true, "-b": true, "-c": true, "-D": true, "-E": true, "-e": true, "-F": true, "-I": true, "-i": true, "-J": true, "-L": true, "-l": true, "-m": true, "-O": true, "-o": true, "-P": true, "-p": true, "-Q": true, "-R": true, "-S": true, "-W": true, "-w": true}
	managed := map[string]bool{"-M": true, "-N": true, "-f": true, "-T": true, "-t": true, "-tt": true, "-O": true, "-S": true}
	destination := ""
	for index := 1; index < len(words); index++ {
		word := words[index]
		if destination != "" {
			return nil, "", true, errors.New("ssh: remote commands are not supported when opening a managed session")
		}
		if word == "--" {
			if index+1 >= len(words) {
				return nil, "", true, errors.New("ssh: destination is missing")
			}
			destination = words[index+1]
			index++
			continue
		}
		if !strings.HasPrefix(word, "-") || word == "-" {
			destination = word
			continue
		}
		option := word
		if len(word) > 2 {
			option = word[:2]
		}
		if managed[word] || managed[option] {
			return nil, "", true, fmt.Errorf("ssh: option %s is managed by NTerm", word)
		}
		if valueOptions[option] && len(word) == 2 {
			index++
			if index >= len(words) {
				return nil, "", true, fmt.Errorf("ssh: option %s needs a value", word)
			}
		}
	}
	if destination == "" {
		return nil, "", true, errors.New("ssh: destination is missing")
	}
	return words[1:], destination, true, nil
}

func splitShellWords(value string) ([]string, error) {
	var words []string
	var current strings.Builder
	quote := rune(0)
	escaped := false
	started := false
	flush := func() {
		if started {
			words = append(words, current.String())
			current.Reset()
			started = false
		}
	}
	for _, char := range value {
		if escaped {
			current.WriteRune(char)
			escaped = false
			started = true
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			started = true
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			started = true
			continue
		}
		if char == ' ' || char == '\t' || char == '\n' {
			flush()
			continue
		}
		if strings.ContainsRune(";&|<>()`", char) {
			return nil, errors.New("ssh: shell operators are not supported in a managed connection")
		}
		current.WriteRune(char)
		started = true
	}
	if escaped || quote != 0 {
		return nil, errors.New("ssh: unfinished quote or escape")
	}
	flush()
	return words, nil
}
