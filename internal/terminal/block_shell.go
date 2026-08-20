package terminal

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

const shellInterruptGrace = time.Second

var (
	errShellClosed = errors.New("terminal shell is closed")
	errShellExited = errors.New("terminal shell exited before the command completed")
)

type blockShell struct {
	input          io.WriteCloser
	resize         func(uint16, uint16) error
	closeTransport func()

	mu        sync.Mutex
	writeMu   sync.Mutex
	active    *shellCommand
	closed    bool
	closeOnce sync.Once
	endOnce   sync.Once
}

type shellCommand struct {
	filter *shellCommandFilter
	done   chan shellCommandResult
}

type shellCommandResult struct {
	Status      int
	CWD         string
	Environment string
	Err         error
}

type shellCommandFilter struct {
	mu         sync.Mutex
	startToken []byte
	endPrefix  []byte
	endSuffix  []byte
	emit       func([]byte)
	started    func() error
	finish     func(shellCommandResult)
	buffer     []byte
	state      int
}

func startLocalBlockShell(shellPath, cwd string, cols, rows uint16) (*blockShell, error) {
	cmd := exec.Command(shellPath, "-l", "-i")
	cmd.Dir = cwd
	cmd.Env = terminalEnvironment()
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, fmt.Errorf("start persistent shell: %w", err)
	}
	result := &blockShell{
		input:  terminal,
		resize: func(cols, rows uint16) error { return pty.Setsize(terminal, &pty.Winsize{Cols: cols, Rows: rows}) },
	}
	result.closeTransport = func() {
		_ = terminal.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	go result.readLoop(terminal)
	go func() {
		err := cmd.Wait()
		if err == nil {
			// `exit` and `exec true` legitimately end the persistent shell.
			// Preserve the zero status even though the shell cannot write a final
			// metadata frame after its process has gone away.
			err = exitError{code: 0}
		}
		result.transportEnded(err)
	}()
	return result, nil
}

func terminalEnvironment() []string {
	overrides := map[string]struct{}{
		"TERM": {}, "COLORTERM": {}, "CLICOLOR": {}, "CLICOLOR_FORCE": {}, "LSCOLORS": {}, "LS_COLORS": {},
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[name]; !replaced {
			environment = append(environment, entry)
		}
	}
	return append(environment,
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"CLICOLOR=1",
		"CLICOLOR_FORCE=1",
		"LSCOLORS=GxFxCxDxBxegedabagaced",
		"LS_COLORS=di=34:ln=36:so=35:pi=33:ex=32:*.zip=33:*.tar=33:*.tgz=33:*.gz=33:*.bz2=33:*.xz=33:*.zst=33:*.7z=33:*.rar=33:*.dmg=33:*.pkg=33:*.go=36:*.rs=36:*.js=36:*.ts=36:*.tsx=36:*.jsx=36:*.py=36:*.swift=36:*.java=36:*.kt=36:*.c=36:*.h=36:*.cpp=36:*.hpp=36:*.json=35:*.yaml=35:*.yml=35:*.toml=35:*.md=35:*.pdf=35:*.png=35:*.jpg=35:*.jpeg=35:*.gif=35:*.webp=35",
	)
}

func (s *blockShell) Execute(
	ctx context.Context,
	command string,
	emit func([]byte),
	attach func() error,
) (shellCommandResult, error) {
	if err := ctx.Err(); err != nil {
		return shellCommandResult{}, err
	}
	nonce, err := shellProtocolNonce()
	if err != nil {
		return shellCommandResult{}, err
	}
	startLabel := "NTERM_" + nonce + "_START"
	endLabel := "NTERM_" + nonce + "_END"
	stopLabel := "NTERM_" + nonce + "_STOP"
	startToken := []byte("\x1e" + startLabel + "\x1f")
	endPrefix := []byte("\x1e" + endLabel + "\x1f")
	endSuffix := []byte("\x1e" + stopLabel + "\x1f")
	done := make(chan shellCommandResult, 1)
	current := &shellCommand{done: done}
	current.filter = &shellCommandFilter{
		startToken: startToken, endPrefix: endPrefix, endSuffix: endSuffix, emit: emit, started: attach,
		finish: func(result shellCommandResult) {
			select {
			case done <- result:
			default:
			}
		},
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return shellCommandResult{}, errShellClosed
	}
	if s.active != nil {
		s.mu.Unlock()
		return shellCommandResult{}, ErrBusy
	}
	s.active = current
	s.mu.Unlock()

	// Keep protocol control bytes out of the interactive line editor. ZLE and
	// readline otherwise interpret them as editing commands before the shell
	// can execute the wrapper.
	environmentProbe := "nterm_environment=; " +
		"if [ -n \"${VIRTUAL_ENV-}\" ]; then nterm_environment=\"Python · ${VIRTUAL_ENV##*/}\"; " +
		"elif [ -n \"${CONDA_DEFAULT_ENV-}\" ]; then nterm_environment=\"Conda · $CONDA_DEFAULT_ENV\"; " +
		"elif [ -n \"${MAMBA_DEFAULT_ENV-}\" ]; then nterm_environment=\"Mamba · $MAMBA_DEFAULT_ENV\"; " +
		"elif [ -n \"${POETRY_ACTIVE-}\" ]; then nterm_environment=Poetry; " +
		"elif [ -n \"${PIPENV_ACTIVE-}\" ]; then nterm_environment=Pipenv; " +
		"elif [ -n \"${DEVBOX_SHELL_ENABLED-}\" ]; then nterm_environment=Devbox; " +
		"elif [ -n \"${IN_NIX_SHELL-}\" ] || [ -n \"${NIX_SHELL-}\" ]; then nterm_environment=Nix; " +
		"elif [ -n \"${DIRENV_DIR-}\" ]; then nterm_environment=direnv; fi; "
	completion := environmentProbe + "/usr/bin/printf '\\036%s\\037%s\\037%s\\037%s\\036%s\\037' " +
		shellQuote(endLabel) + " \"$nterm_status\" \"$PWD\" \"$nterm_environment\" " + shellQuote(stopLabel)
	finishFunction := "_nterm_finish_" + nonce
	setup := finishFunction + "() { " + completion + "; }\r"
	interruptHandler := "nterm_status=130; trap - INT; " + finishFunction + "; unset -f " + finishFunction
	wrapper := "trap " + shellQuote(interruptHandler) + " INT; " +
		"/usr/bin/printf '\\036%s\\037' " + shellQuote(startLabel) +
		"; eval " + shellQuote(command) +
		"; nterm_status=$?; trap - INT; " + finishFunction + "; unset -f " + finishFunction + "\r"
	if _, err := s.Write([]byte(setup + wrapper)); err != nil {
		s.clearActive(current)
		return shellCommandResult{}, fmt.Errorf("write command to persistent shell: %w", err)
	}
	select {
	case result := <-done:
		s.clearActive(current)
		if err := ctx.Err(); err != nil {
			return result, err
		}
		return result, result.Err
	case <-ctx.Done():
		_, _ = s.Write([]byte{3})
		timer := time.NewTimer(shellInterruptGrace)
		defer timer.Stop()
		select {
		case result := <-done:
			s.clearActive(current)
			return result, ctx.Err()
		case <-timer.C:
			s.clearActive(current)
			s.Close()
			return shellCommandResult{}, ctx.Err()
		}
	}
}

func (s *blockShell) Write(value []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return 0, errShellClosed
	}
	written := 0
	for written < len(value) {
		count, err := s.input.Write(value[written:])
		written += count
		if err != nil {
			return written, err
		}
		if count == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func (s *blockShell) Resize(cols, rows uint16) error {
	if cols < 2 || rows < 2 {
		return nil
	}
	s.mu.Lock()
	closed := s.closed
	resize := s.resize
	s.mu.Unlock()
	if closed {
		return errShellClosed
	}
	return resize(cols, rows)
}

func (s *blockShell) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *blockShell) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		active := s.active
		s.mu.Unlock()
		if s.closeTransport != nil {
			s.closeTransport()
		}
		if active != nil {
			active.filter.fail(errShellClosed)
		}
	})
}

func (s *blockShell) clearActive(command *shellCommand) {
	s.mu.Lock()
	if s.active == command {
		s.active = nil
	}
	s.mu.Unlock()
}

func (s *blockShell) readLoop(reader io.Reader) {
	buffer := make([]byte, 16*1024)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			if os.Getenv("NTERM_TERMINAL_DEBUG") == "1" {
				_, _ = os.Stderr.Write(buffer[:count])
			}
			s.mu.Lock()
			active := s.active
			s.mu.Unlock()
			if active != nil {
				active.filter.Write(buffer[:count])
			}
		}
		if err != nil {
			// The process/channel waiter owns the authoritative exit status. A
			// PTY commonly reports EOF slightly before Wait returns; completing
			// here would race a clean exit into a generic failure.
			return
		}
	}
}

func (s *blockShell) transportEnded(err error) {
	s.endOnce.Do(func() {
		if err == nil {
			err = errShellExited
		}
		s.mu.Lock()
		s.closed = true
		active := s.active
		s.mu.Unlock()
		if active != nil {
			active.filter.fail(fmt.Errorf("%w: %w", errShellExited, err))
		}
	})
}

func shellProtocolNonce() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create shell protocol marker: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func (f *shellCommandFilter) Write(value []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == 3 || len(value) == 0 {
		return
	}
	f.buffer = append(f.buffer, value...)
	for {
		switch f.state {
		case 0:
			index := bytes.Index(f.buffer, f.startToken)
			if index < 0 {
				f.retainTokenPrefix(f.startToken)
				return
			}
			f.buffer = f.buffer[index+len(f.startToken):]
			f.state = 1
			if f.started != nil {
				if err := f.started(); err != nil {
					f.failLocked(fmt.Errorf("attach persistent terminal input: %w", err))
					return
				}
				f.started = nil
			}
		case 1:
			index := bytes.Index(f.buffer, f.endPrefix)
			if index < 0 {
				f.emitUntilTokenPrefix(f.endPrefix)
				return
			}
			if index > 0 {
				f.emit(f.buffer[:index])
			}
			f.buffer = f.buffer[index+len(f.endPrefix):]
			f.state = 2
		case 2:
			index := bytes.Index(f.buffer, f.endSuffix)
			if index < 0 {
				return
			}
			metadata := bytes.SplitN(f.buffer[:index], []byte{0x1f}, 3)
			if len(metadata) != 3 {
				f.failLocked(errors.New("persistent shell returned malformed command metadata"))
				return
			}
			status, err := strconv.Atoi(string(metadata[0]))
			if err != nil {
				f.failLocked(fmt.Errorf("persistent shell returned invalid exit status: %w", err))
				return
			}
			cwd := string(metadata[1])
			environment := string(metadata[2])
			f.buffer = nil
			f.state = 3
			f.finish(shellCommandResult{Status: status, CWD: cwd, Environment: environment})
			return
		}
	}
}

func (f *shellCommandFilter) emitUntilTokenPrefix(token []byte) {
	retain := tokenPrefixAtEnd(f.buffer, token)
	count := len(f.buffer) - retain
	if count == 0 {
		return
	}
	f.emit(f.buffer[:count])
	f.buffer = append(f.buffer[:0], f.buffer[count:]...)
}

func (f *shellCommandFilter) retainTokenPrefix(token []byte) {
	retain := tokenPrefixAtEnd(f.buffer, token)
	if retain == 0 {
		f.buffer = nil
		return
	}
	f.buffer = append(f.buffer[:0], f.buffer[len(f.buffer)-retain:]...)
}

func tokenPrefixAtEnd(value, token []byte) int {
	maximum := min(len(value), len(token)-1)
	for length := maximum; length > 0; length-- {
		if bytes.Equal(value[len(value)-length:], token[:length]) {
			return length
		}
	}
	return 0
}

func (f *shellCommandFilter) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failLocked(err)
}

func (f *shellCommandFilter) failLocked(err error) {
	if f.state == 3 {
		return
	}
	if f.state == 1 && len(f.buffer) > 0 {
		f.emit(f.buffer)
	}
	f.buffer = nil
	f.state = 3
	f.finish(shellCommandResult{Status: 1, Err: err})
}
