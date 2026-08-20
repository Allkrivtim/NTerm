package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexandr/nterm/internal/domain"
	"github.com/alexandr/nterm/internal/gitstatus"
)

var ErrBusy = errors.New("a command is already running in this session")

const maxSessionHistory = 2000

type Session struct {
	mu           sync.RWMutex
	inputMu      sync.Mutex
	cwd          string
	localCWD     string
	shell        string
	environment  string
	history      []string
	running      bool
	localShell   *blockShell
	remote       *sshSession
	activeInput  io.Writer
	pendingInput []byte
	acceptInput  bool
	activeResize func(uint16, uint16) error
	activeClose  func() error
	cols         uint16
	rows         uint16
	sshProfile   SSHProfile
	sshStatus    func(SSHStatus)
}

type SSHProfile struct {
	CredentialAccount string
	HelperEnabled     bool
	Host              string
	Port              int
	User              string
	KeyPath           string
	ConfirmHostKey    func(SSHHostKey) bool
}

func NewSession() (*Session, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	return NewSessionAt(cwd, "")
}

func NewSessionAt(cwd, configuredShell string) (*Session, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return nil, fmt.Errorf("open working directory: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("working directory is not a directory")
	}
	shell := configuredShell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	return &Session{
		cwd:        filepath.Clean(cwd),
		localCWD:   filepath.Clean(cwd),
		shell:      shell,
		cols:       120,
		rows:       34,
		sshProfile: SSHProfile{HelperEnabled: true},
	}, nil
}

func (s *Session) CWD() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cwd
}

func (s *Session) History() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.history))
	copy(result, s.history)
	return result
}

// RestoreHistory seeds completion and arrow-key history from persisted command
// blocks without replaying any command or shell state.
func (s *Session) RestoreHistory(history []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(history) > maxSessionHistory {
		history = history[len(history)-maxSessionHistory:]
	}
	s.history = append([]string(nil), history...)
}

func (s *Session) Shell() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.shell
}

func (s *Session) Environment() string {
	s.mu.RLock()
	remote := s.remote
	environment := s.environment
	s.mu.RUnlock()
	if remote != nil {
		return remote.environmentLabel()
	}
	return environment
}

func (s *Session) ConfigureSSHProfile(profile SSHProfile) {
	s.mu.Lock()
	s.sshProfile = profile
	s.mu.Unlock()
}

func (s *Session) SetSSHStatusHandler(handler func(SSHStatus)) {
	s.mu.Lock()
	s.sshStatus = handler
	remote := s.remote
	s.mu.Unlock()
	if remote != nil {
		remote.setStatusHandler(handler)
	}
}

func (s *Session) emitSSHStatus(status SSHStatus) {
	s.mu.RLock()
	handler := s.sshStatus
	s.mu.RUnlock()
	if handler != nil {
		handler(status)
	}
}

func (s *Session) ConnectSSH(ctx context.Context, profile SSHProfile) error {
	s.mu.Lock()
	if s.running || s.remote != nil {
		s.mu.Unlock()
		return ErrBusy
	}
	s.running = true
	handler := s.sshStatus
	s.mu.Unlock()
	if handler != nil {
		handler(SSHStatus{State: "connecting", Label: profile.User + "@" + profile.Host, Message: "Connecting…"})
	}
	defer func() {
		s.mu.Lock()
		s.running = false
		s.pendingInput = nil
		s.mu.Unlock()
	}()
	connected, err := connectSSH(ctx, nil, "", profile)
	if err != nil {
		if handler != nil {
			handler(SSHStatus{State: "disconnected", Label: profile.User + "@" + profile.Host, Message: err.Error()})
		}
		return err
	}
	connected.setStatusHandler(handler)
	s.mu.Lock()
	s.remote = connected
	s.localCWD = s.cwd
	s.cwd = connected.cwd
	s.sshProfile = profile
	s.mu.Unlock()
	return nil
}

func (s *Session) Run(ctx context.Context, block *domain.Block, emit func(domain.OutputChunk)) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrBusy
	}
	s.running = true
	cwd := s.cwd
	remote := s.remote
	sshProfile := s.sshProfile
	s.history = append(s.history, block.Command)
	if len(s.history) > maxSessionHistory {
		s.history = append([]string(nil), s.history[len(s.history)-maxSessionHistory:]...)
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	if remote != nil {
		trimmed := strings.TrimSpace(block.Command)
		if trimmed == "exit" || trimmed == "logout" {
			remote.close(true)
			s.mu.Lock()
			if s.remote == remote {
				s.remote = nil
				s.cwd = s.localCWD
			}
			s.mu.Unlock()
			s.emitSSHStatus(SSHStatus{State: "closed"})
			block.FinalCWD = s.CWD()
			emit(domain.OutputChunk{BlockID: block.ID, Stream: "stdout", Data: "SSH session disconnected\n"})
			return nil
		}
		return remote.run(ctx, s, block, emit)
	}

	if arguments, destination, ok, err := parseSSHInvocation(block.Command); ok {
		if err != nil {
			emit(domain.OutputChunk{BlockID: block.ID, Stream: "stderr", Data: err.Error() + "\n"})
			return exitError{code: 2}
		}
		s.emitSSHStatus(SSHStatus{State: "connecting", Label: destination, Message: "Connecting…"})
		connected, err := connectSSH(ctx, arguments, destination, sshProfile)
		if err != nil {
			s.emitSSHStatus(SSHStatus{State: "disconnected", Label: destination, Message: err.Error()})
			emit(domain.OutputChunk{BlockID: block.ID, Stream: "stderr", Data: err.Error() + "\n"})
			return exitError{code: 255}
		}
		connected.setStatusHandler(s.sshStatus)
		s.mu.Lock()
		s.remote = connected
		s.localCWD = cwd
		s.cwd = connected.cwd
		s.mu.Unlock()
		block.FinalCWD = connected.cwd
		emit(domain.OutputChunk{BlockID: block.ID, Stream: "stdout", Data: "Connected to " + connected.label() + "\n"})
		return nil
	}

	return s.runShell(ctx, block, emit)
}

func (s *Session) ReconnectSSH(ctx context.Context) error {
	s.mu.RLock()
	remote := s.remote
	s.mu.RUnlock()
	if remote == nil {
		return errors.New("ssh: no remote session")
	}
	return remote.reconnect(ctx, true)
}

// Close releases a managed SSH connection and its temporary helper.
func (s *Session) Close() {
	s.mu.Lock()
	remote := s.remote
	localShell := s.localShell
	activeClose := s.activeClose
	s.remote = nil
	s.localShell = nil
	s.activeInput = nil
	s.activeResize = nil
	s.activeClose = nil
	s.pendingInput = nil
	s.acceptInput = false
	s.mu.Unlock()
	if activeClose != nil {
		_ = activeClose()
	}
	if remote != nil {
		remote.close(true)
	}
	if localShell != nil {
		localShell.Close()
	}
}

// Input sends raw bytes to the foreground program in this session. The PTY,
// rather than a command pipe, interprets control keys, escape sequences and
// bracketed paste exactly like a native terminal.
func (s *Session) Input(value string) error {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.mu.Lock()
	activeInput := s.activeInput
	remote := s.remote
	if activeInput == nil {
		if s.acceptInput {
			s.pendingInput = append(s.pendingInput, value...)
		}
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	_, err := io.WriteString(activeInput, value)
	if err == nil && remote != nil {
		remote.touch()
	}
	return err
}

// BeginInput opens the short interval between creating a block in the UI and
// attaching its PTY. Without it, a fast answer to a prompt could arrive before
// the shell has published its input writer and would be lost.
func (s *Session) BeginInput() {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.mu.Lock()
	s.acceptInput = true
	s.pendingInput = nil
	s.mu.Unlock()
}

func (s *Session) EndInput() {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.mu.Lock()
	s.acceptInput = false
	s.pendingInput = nil
	s.mu.Unlock()
}

// attachInput atomically publishes a PTY writer and flushes keys that arrived
// after the block was created but before the process finished opening its PTY.
// inputMu also preserves key order when Wails delivers several bridge calls at
// nearly the same time.
func (s *Session) attachInput(input io.Writer, resize func(uint16, uint16) error, closeInput func() error) error {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.mu.Lock()
	s.activeInput = input
	s.activeResize = resize
	s.activeClose = closeInput
	pending := append([]byte(nil), s.pendingInput...)
	s.pendingInput = nil
	remote := s.remote
	s.mu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	_, err := input.Write(pending)
	if err == nil && remote != nil {
		remote.touch()
	}
	return err
}

func (s *Session) Resize(cols, rows uint16) error {
	if cols < 2 || rows < 2 {
		return nil
	}
	s.mu.Lock()
	s.cols, s.rows = cols, rows
	activeResize := s.activeResize
	s.mu.Unlock()
	if activeResize == nil {
		return nil
	}
	return activeResize(cols, rows)
}

func (s *Session) RemoteLabel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.remote == nil {
		return ""
	}
	return s.remote.label()
}

func (s *Session) RemotePlatform() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.remote == nil {
		return "server"
	}
	return s.remote.platform
}

func (s *Session) CompletePath(ctx context.Context, token string) ([]RemotePathEntry, bool) {
	s.mu.RLock()
	remote := s.remote
	s.mu.RUnlock()
	if remote == nil {
		return nil, false
	}
	return remote.completePath(ctx, token), true
}

func (s *Session) CommandNames(ctx context.Context) ([]string, bool) {
	s.mu.RLock()
	remote := s.remote
	s.mu.RUnlock()
	if remote == nil {
		return nil, false
	}
	return remote.commandNames(ctx), true
}

func (s *Session) GitContext(ctx context.Context) (gitstatus.Info, bool) {
	s.mu.RLock()
	remote := s.remote
	s.mu.RUnlock()
	if remote == nil {
		return gitstatus.Info{}, false
	}
	return remote.gitContext(ctx)
}

func (s *Session) runShell(ctx context.Context, block *domain.Block, emit func(domain.OutputChunk)) error {
	shell, err := s.ensureLocalShell()
	if err != nil {
		return err
	}
	s.mu.RLock()
	cols, rows := s.cols, s.rows
	s.mu.RUnlock()
	_ = shell.Resize(cols, rows)
	result, err := shell.Execute(ctx, block.Command, func(value []byte) {
		emit(domain.NewOutputChunkBytes(block.ID, "stdout", value))
	}, func() error {
		return s.attachInput(shell, shell.Resize, nil)
	})
	s.mu.Lock()
	if s.activeInput == shell {
		s.activeInput = nil
		s.activeResize = nil
		s.activeClose = nil
	}
	if shell.Closed() && s.localShell == shell {
		s.localShell = nil
	}
	s.mu.Unlock()
	if result.CWD != "" {
		s.setCWD(result.CWD)
		block.FinalCWD = result.CWD
	}
	s.mu.Lock()
	s.environment = result.Environment
	s.mu.Unlock()
	block.Environment = result.Environment
	if err != nil {
		if ExitCode(err) == 0 {
			block.FinalCWD = s.CWD()
			return nil
		}
		return err
	}
	if result.Status != 0 {
		return exitError{code: result.Status}
	}
	return nil
}

func (s *Session) ensureLocalShell() (*blockShell, error) {
	s.mu.RLock()
	existing := s.localShell
	cwd, shellPath, cols, rows := s.cwd, s.shell, s.cols, s.rows
	s.mu.RUnlock()
	if existing != nil && !existing.Closed() {
		return existing, nil
	}
	started, err := startLocalBlockShell(shellPath, cwd, cols, rows)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.localShell != nil && !s.localShell.Closed() {
		existing = s.localShell
		s.mu.Unlock()
		started.Close()
		return existing, nil
	}
	s.localShell = started
	s.mu.Unlock()
	return started, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (s *Session) setCWD(cwd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cwd = cwd
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var local exitError
	if errors.As(err, &local) {
		return local.code
	}
	var process *exec.ExitError
	if errors.As(err, &process) {
		return process.ExitCode()
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return 1
}

type exitError struct{ code int }

func (e exitError) Error() string { return "exit status " + strconv.Itoa(e.code) }

func Finish(block *domain.Block, err error, cancelled bool) {
	now := time.Now()
	code := ExitCode(err)
	block.EndedAt = &now
	block.Duration = now.Sub(block.StartedAt).Milliseconds()
	block.ExitCode = &code
	switch {
	case cancelled:
		block.State = domain.BlockCancelled
	case code == 0:
		block.State = domain.BlockSucceeded
	default:
		block.State = domain.BlockFailed
	}
}
