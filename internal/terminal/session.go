package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alexandr/nterm/internal/domain"
	"github.com/alexandr/nterm/internal/gitstatus"
	"github.com/creack/pty"
)

var ErrBusy = errors.New("a command is already running in this session")

type Session struct {
	mu        sync.RWMutex
	cwd       string
	localCWD  string
	previous  string
	shell     string
	history   []string
	running   bool
	remote    *sshSession
	activePTY *os.File
	cols      uint16
	rows      uint16
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
	return &Session{cwd: filepath.Clean(cwd), localCWD: filepath.Clean(cwd), shell: shell, cols: 120, rows: 34}, nil
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

func (s *Session) Shell() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.shell
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
	s.history = append(s.history, block.Command)
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
		connected, err := connectSSH(ctx, arguments, destination)
		if err != nil {
			emit(domain.OutputChunk{BlockID: block.ID, Stream: "stderr", Data: err.Error() + "\n"})
			return exitError{code: 255}
		}
		s.mu.Lock()
		s.remote = connected
		s.localCWD = cwd
		s.cwd = connected.cwd
		s.previous = ""
		s.mu.Unlock()
		block.FinalCWD = connected.cwd
		emit(domain.OutputChunk{BlockID: block.ID, Stream: "stdout", Data: "Connected to " + connected.label() + "\n"})
		return nil
	}

	if target, ok, err := s.cdTarget(block.Command, cwd); ok {
		if err != nil {
			emit(domain.OutputChunk{BlockID: block.ID, Stream: "stderr", Data: err.Error() + "\n"})
			return exitError{code: 1}
		}
		s.setCWD(target)
		block.FinalCWD = target
		return nil
	}

	return s.runShell(ctx, cwd, block, emit)
}

// Close releases a managed SSH connection and its temporary helper.
func (s *Session) Close() {
	s.mu.Lock()
	remote := s.remote
	activePTY := s.activePTY
	s.remote = nil
	s.mu.Unlock()
	if activePTY != nil {
		_ = activePTY.Close()
	}
	if remote != nil {
		remote.close(true)
	}
}

// Input sends raw bytes to the foreground program in this session. The PTY,
// rather than a command pipe, interprets control keys, escape sequences and
// bracketed paste exactly like a native terminal.
func (s *Session) Input(value string) error {
	s.mu.RLock()
	activePTY := s.activePTY
	s.mu.RUnlock()
	if activePTY == nil {
		return nil
	}
	_, err := io.WriteString(activePTY, value)
	return err
}

func (s *Session) Resize(cols, rows uint16) error {
	if cols < 2 || rows < 2 {
		return nil
	}
	s.mu.Lock()
	s.cols, s.rows = cols, rows
	activePTY := s.activePTY
	s.mu.Unlock()
	if activePTY == nil {
		return nil
	}
	return pty.Setsize(activePTY, &pty.Winsize{Cols: cols, Rows: rows})
}

func (s *Session) RemoteLabel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.remote == nil {
		return ""
	}
	return s.remote.label()
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

func (s *Session) runShell(ctx context.Context, cwd string, block *domain.Block, emit func(domain.OutputChunk)) error {
	cwdFile, err := os.CreateTemp("", "nterm-cwd-*")
	if err != nil {
		return fmt.Errorf("create cwd marker: %w", err)
	}
	cwdPath := cwdFile.Name()
	_ = cwdFile.Close()
	defer os.Remove(cwdPath)

	command := block.Command + "\nnterm_status=$?\nprintf '%s' \"$PWD\" > " + shellQuote(cwdPath) + "\nexit \"$nterm_status\""
	cmd := exec.Command(s.shell, "-l", "-c", command)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"CLICOLOR=1",
		"CLICOLOR_FORCE=1",
		"LSCOLORS=GxFxCxDxBxegedabagaced",
		"LS_COLORS=di=34:ln=36:so=35:pi=33:ex=32:*.zip=33:*.tar=33:*.tgz=33:*.gz=33:*.bz2=33:*.xz=33:*.zst=33:*.7z=33:*.rar=33:*.dmg=33:*.pkg=33:*.go=36:*.rs=36:*.js=36:*.ts=36:*.tsx=36:*.jsx=36:*.py=36:*.swift=36:*.java=36:*.kt=36:*.c=36:*.h=36:*.cpp=36:*.hpp=36:*.json=35:*.yaml=35:*.yml=35:*.toml=35:*.md=35:*.pdf=35:*.png=35:*.jpg=35:*.jpeg=35:*.gif=35:*.webp=35",
	)
	waitErr := s.runPTY(ctx, cmd, func(value []byte) {
		emit(domain.OutputChunk{BlockID: block.ID, Stream: "stdout", Data: string(value)})
	})

	if finalCWDBytes, cwdErr := os.ReadFile(cwdPath); cwdErr == nil {
		finalCWD := strings.TrimSpace(string(finalCWDBytes))
		if finalCWD != "" {
			s.setCWD(finalCWD)
			block.FinalCWD = finalCWD
		}
	}
	return waitErr
}

func (s *Session) runPTY(ctx context.Context, cmd *exec.Cmd, consume func([]byte)) error {
	s.mu.RLock()
	size := &pty.Winsize{Cols: s.cols, Rows: s.rows}
	s.mu.RUnlock()
	terminal, err := pty.StartWithSize(cmd, size)
	if err != nil {
		return fmt.Errorf("start PTY: %w", err)
	}
	s.mu.Lock()
	s.activePTY = terminal
	s.mu.Unlock()

	processDone := make(chan struct{})
	go stopProcessOnCancel(ctx, cmd, processDone)
	buffer := make([]byte, 16*1024)
	for {
		read, readErr := terminal.Read(buffer)
		if read > 0 {
			consume(buffer[:read])
		}
		if readErr != nil {
			break
		}
	}
	waitErr := cmd.Wait()
	close(processDone)
	s.mu.Lock()
	if s.activePTY == terminal {
		s.activePTY = nil
	}
	s.mu.Unlock()
	_ = terminal.Close()
	return waitErr
}

func copyOutput(group *sync.WaitGroup, reader io.Reader, blockID, stream string, emit func(domain.OutputChunk)) {
	defer group.Done()
	buffer := make([]byte, 4096)
	for {
		read, err := reader.Read(buffer)
		if read > 0 {
			emit(domain.OutputChunk{BlockID: blockID, Stream: stream, Data: string(buffer[:read])})
		}
		if err != nil {
			return
		}
	}
}

func stopProcessOnCancel(ctx context.Context, cmd *exec.Cmd, done <-chan struct{}) {
	select {
	case <-ctx.Done():
		if runtime.GOOS != "windows" {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		} else {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		timer := time.NewTimer(600 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-done:
			return
		case <-timer.C:
			if runtime.GOOS != "windows" {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			} else {
				_ = cmd.Process.Kill()
			}
		}
	case <-done:
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (s *Session) cdTarget(command, cwd string) (string, bool, error) {
	trimmed := strings.TrimSpace(command)
	if trimmed != "cd" && !strings.HasPrefix(trimmed, "cd ") {
		return "", false, nil
	}
	raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "cd"))
	if strings.ContainsAny(raw, ";&|\n") {
		return "", false, nil
	}
	raw = strings.Trim(raw, "\"'")
	if raw == "" || raw == "~" {
		raw, _ = os.UserHomeDir()
	} else if raw == "-" {
		s.mu.RLock()
		raw = s.previous
		s.mu.RUnlock()
		if raw == "" {
			return "", true, errors.New("cd: OLDPWD not set")
		}
	} else if strings.HasPrefix(raw, "~/") {
		home, _ := os.UserHomeDir()
		raw = filepath.Join(home, strings.TrimPrefix(raw, "~/"))
	} else if !filepath.IsAbs(raw) {
		raw = filepath.Join(cwd, raw)
	}
	target, err := filepath.Abs(raw)
	if err != nil {
		return "", true, fmt.Errorf("cd: %w", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", true, fmt.Errorf("cd: %s: %w", raw, err)
	}
	if !info.IsDir() {
		return "", true, fmt.Errorf("cd: %s: not a directory", raw)
	}
	return filepath.Clean(target), true, nil
}

func (s *Session) setCWD(cwd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cwd != s.cwd {
		s.previous = s.cwd
		s.cwd = cwd
	}
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
