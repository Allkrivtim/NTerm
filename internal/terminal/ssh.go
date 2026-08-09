package terminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alexandr/nterm/internal/domain"
	"github.com/alexandr/nterm/internal/gitstatus"
)

const remoteMetadataPrefix = "\x1eNTERM_META\x1f"

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
	sshPath     string
	destination string
	socketDir   string
	socketPath  string
	helperPath  string
	shell       string
	home        string
	hostname    string

	mu         sync.RWMutex
	cwd        string
	master     *exec.Cmd
	masterDone chan error
	commands   []string
	closed     bool
}

type RemotePathEntry struct {
	Value string
	IsDir bool
}

func connectSSH(ctx context.Context, arguments []string, destination string) (*sshSession, error) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return nil, errors.New("ssh: OpenSSH is not available")
	}
	socketDir, err := os.MkdirTemp("", "nterm-ssh-*")
	if err != nil {
		return nil, fmt.Errorf("ssh: create control socket directory: %w", err)
	}
	remote := &sshSession{
		sshPath: sshPath, destination: destination, socketDir: socketDir,
		socketPath: filepath.Join(socketDir, "control"), masterDone: make(chan error, 1),
	}
	cleanup := true
	defer func() {
		if cleanup {
			remote.close(true)
		}
	}()

	masterArgs := []string{"-M", "-N", "-T", "-o", "ControlMaster=yes", "-o", "ControlPersist=no", "-o", "ExitOnForwardFailure=yes", "-S", remote.socketPath}
	masterArgs = append(masterArgs, arguments...)
	var stderr lockedBuffer
	remote.master = exec.Command(sshPath, masterArgs...)
	remote.master.Stdin = nil
	remote.master.Stdout = io.Discard
	remote.master.Stderr = &stderr
	remote.master.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := remote.master.Start(); err != nil {
		return nil, fmt.Errorf("ssh: start connection: %w", err)
	}
	go func() { remote.masterDone <- remote.master.Wait() }()

	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(120 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		check := exec.Command(sshPath, "-S", remote.socketPath, "-O", "check", destination)
		if check.Run() == nil {
			break
		}
		select {
		case err := <-remote.masterDone:
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = err.Error()
			}
			return nil, fmt.Errorf("ssh: %s", message)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("ssh: connection timed out")
		case <-ticker.C:
		}
	}

	probe, err := remote.exec(ctx, nil, `printf '%s\000%s\000%s\000%s' "$PWD" "$HOME" "$SHELL" "$(hostname -s 2>/dev/null || hostname)"`)
	if err != nil {
		return nil, fmt.Errorf("ssh: probe remote shell: %w", err)
	}
	fields := bytes.SplitN(probe, []byte{0}, 4)
	if len(fields) != 4 || len(fields[0]) == 0 {
		return nil, errors.New("ssh: remote shell returned invalid session details")
	}
	remote.cwd, remote.home, remote.shell, remote.hostname = string(fields[0]), string(fields[1]), string(fields[2]), string(fields[3])
	if remote.shell == "" {
		remote.shell = "/bin/sh"
	}

	install := `umask 077; d=$(mktemp -d "${TMPDIR:-/tmp}/nterm.XXXXXXXX") || exit 1; cat > "$d/helper" || exit 1; chmod 700 "$d/helper"; printf '%s' "$d/helper"`
	path, err := remote.exec(ctx, strings.NewReader(remoteHelper), install)
	if err != nil {
		return nil, fmt.Errorf("ssh: install temporary helper: %w", err)
	}
	remote.helperPath = strings.TrimSpace(string(path))
	if remote.helperPath == "" {
		return nil, errors.New("ssh: temporary helper path is empty")
	}
	cleanup = false
	return remote, nil
}

func (r *sshSession) exec(ctx context.Context, stdin io.Reader, command string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.sshPath, "-T", "-S", r.socketPath, r.destination, command)
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return output, errors.New(message)
		}
	}
	return output, err
}

func (r *sshSession) run(ctx context.Context, session *Session, block *domain.Block, emit func(domain.OutputChunk)) error {
	r.mu.RLock()
	cwd, shell := r.cwd, r.shell
	r.mu.RUnlock()
	script := "cd " + shellQuote(cwd) + " || exit 1\n" + block.Command + "\nnterm_status=$?\nprintf '\\036NTERM_META\\037%s\\037%s\\036\\n' \"$nterm_status\" \"$PWD\"\nexit \"$nterm_status\""
	remoteCommand := shellQuote(shell) + " -lc " + shellQuote(script)
	// A forced remote TTY combined with the local PTY is essential: without
	// both sides, OpenSSH may be multiplexed but remote TUI programs still see
	// pipes and either crash or render cursor controls as ordinary text.
	cmd := exec.Command(r.sshPath, "-tt", "-S", r.socketPath, r.destination, remoteCommand)
	filter := &remoteOutputFilter{blockID: block.ID, emit: emit, status: -1}
	waitErr := session.runPTY(ctx, cmd, func(value []byte) { _, _ = filter.Write(value) })
	filter.Flush()
	if filter.cwd != "" {
		r.mu.Lock()
		r.cwd = filter.cwd
		r.mu.Unlock()
		block.FinalCWD = filter.cwd
	}
	if filter.status >= 0 && filter.status != 0 && waitErr == nil {
		return exitError{code: filter.status}
	}
	return waitErr
}

func (r *sshSession) completePath(ctx context.Context, token string) []RemotePathEntry {
	r.mu.RLock()
	cwd, helper := r.cwd, r.helperPath
	r.mu.RUnlock()
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
	if r.hostname != "" {
		return r.hostname
	}
	return r.destination
}

func (r *sshSession) close(cleanRemote bool) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	helper := r.helperPath
	r.mu.Unlock()
	if cleanRemote && helper != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
		_, _ = r.exec(ctx, nil, "rm -rf -- "+shellQuote(filepath.Dir(helper)))
		cancel()
	}
	if r.socketPath != "" {
		command := exec.Command(r.sshPath, "-S", r.socketPath, "-O", "exit", r.destination)
		_ = command.Run()
	}
	if r.master != nil && r.master.Process != nil {
		_ = syscall.Kill(-r.master.Process.Pid, syscall.SIGTERM)
	}
	if r.socketDir != "" {
		_ = os.RemoveAll(r.socketDir)
	}
}

type remoteOutputFilter struct {
	blockID string
	emit    func(domain.OutputChunk)
	pending []byte
	cwd     string
	status  int
}

func (f *remoteOutputFilter) Write(value []byte) (int, error) {
	written := len(value)
	f.pending = append(f.pending, value...)
	prefix := []byte(remoteMetadataPrefix)
	if index := bytes.Index(f.pending, prefix); index >= 0 {
		if index > 0 {
			f.emit(domain.OutputChunk{BlockID: f.blockID, Stream: "stdout", Data: string(f.pending[:index])})
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
		f.emit(domain.OutputChunk{BlockID: f.blockID, Stream: "stdout", Data: string(f.pending[:cut])})
		f.pending = append(f.pending[:0], f.pending[cut:]...)
	}
	return written, nil
}

func (f *remoteOutputFilter) Flush() {
	if len(f.pending) > 0 {
		f.emit(domain.OutputChunk{BlockID: f.blockID, Stream: "stdout", Data: string(f.pending)})
		f.pending = nil
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(value)
}
func (b *lockedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }

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
