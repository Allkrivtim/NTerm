package terminal

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexandr/nterm/internal/domain"
)

func TestTerminalEnvironmentReplacesTerminalOverrides(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("COLORTERM", "legacy")
	t.Setenv("CLICOLOR_FORCE", "0")
	t.Setenv("NTERM_ENV_PRESERVE", "yes")

	values := make(map[string]string)
	counts := make(map[string]int)
	for _, entry := range terminalEnvironment() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		values[name] = value
		counts[name]++
	}
	for name, expected := range map[string]string{
		"TERM": "xterm-256color", "COLORTERM": "truecolor", "CLICOLOR_FORCE": "1",
	} {
		if counts[name] != 1 || values[name] != expected {
			t.Fatalf("%s entries = %d, value = %q; want one %q entry", name, counts[name], values[name], expected)
		}
	}
	if values["NTERM_ENV_PRESERVE"] != "yes" {
		t.Fatalf("unrelated environment variable was not preserved: %q", values["NTERM_ENV_PRESERVE"])
	}
	if _, exists := values["PATH"]; !exists && os.Getenv("PATH") != "" {
		t.Fatal("PATH was not preserved")
	}
}

func TestInputQueuedUntilPTYIsReady(t *testing.T) {
	session, err := NewSessionAt(t.TempDir(), "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	session.BeginInput()
	if err := session.Input("yes\r"); err != nil {
		t.Fatal(err)
	}
	var input bytes.Buffer
	if err := session.attachInput(&input, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := input.String(); got != "yes\r" {
		t.Fatalf("buffered input = %q", got)
	}
	session.EndInput()
}

func TestSessionStreamsOutputAndExitCode(t *testing.T) {
	session, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	block := &domain.Block{ID: "test", Command: `sh -c 'printf hello; printf problem >&2; exit 7'`, StartedAt: time.Now()}
	var mu sync.Mutex
	var output strings.Builder
	err = session.Run(context.Background(), block, func(chunk domain.OutputChunk) {
		mu.Lock()
		output.Write(chunk.Bytes())
		mu.Unlock()
	})
	if code := ExitCode(err); code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	if got := output.String(); !strings.Contains(got, "hello") || !strings.Contains(got, "problem") {
		t.Fatalf("output = %q, want stdout and stderr", got)
	}
}

func TestSessionProvidesTTYAndWindowSize(t *testing.T) {
	session, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Resize(91, 27); err != nil {
		t.Fatal(err)
	}
	block := &domain.Block{ID: "tty", Command: `test -t 0 && test -t 1 && printf 'tty-device\n' >/dev/tty && stty size`, StartedAt: time.Now()}
	var output strings.Builder
	if err := session.Run(context.Background(), block, func(chunk domain.OutputChunk) { output.Write(chunk.Bytes()) }); err != nil {
		if strings.Contains(output.String(), "operation not permitted") {
			t.Skip("test sandbox blocks /dev/tty")
		}
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "tty-device") || !strings.Contains(output.String(), "27 91") {
		t.Fatalf("PTY size output = %q", output.String())
	}
}

func TestLinePromptAcceptsRawInputWithoutFullscreenMode(t *testing.T) {
	session, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	block := &domain.Block{ID: "prompt", Command: `printf 'Continue? '; IFS= read answer; printf '\nanswer=%s\n' "$answer"`, StartedAt: time.Now()}
	var output strings.Builder
	var answered sync.Once
	err = session.Run(ctx, block, func(chunk domain.OutputChunk) {
		output.Write(chunk.Bytes())
		if strings.Contains(output.String(), "Continue?") {
			answered.Do(func() {
				if inputErr := session.Input("Y\r"); inputErr != nil {
					t.Errorf("send prompt answer: %v", inputErr)
				}
			})
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "answer=Y") {
		t.Fatalf("prompt output = %q", output.String())
	}
}

func TestMicroCanStartAndExitInPTY(t *testing.T) {
	if _, err := exec.LookPath("micro"); err != nil {
		t.Skip("micro is not installed")
	}
	session, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	requireControllingTTY(t, session)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	block := &domain.Block{ID: "micro", Command: "micro", StartedAt: time.Now()}
	var mu sync.Mutex
	var output strings.Builder
	ready := make(chan struct{})
	var readyOnce sync.Once
	done := make(chan error, 1)
	go func() {
		done <- session.Run(ctx, block, func(chunk domain.OutputChunk) {
			mu.Lock()
			output.Write(chunk.Bytes())
			if strings.Contains(output.String(), "\x1b[") {
				readyOnce.Do(func() { close(ready) })
			}
			mu.Unlock()
		})
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("micro did not render its first terminal frame")
	}
	time.Sleep(120 * time.Millisecond)
	if err := session.Input("\x11"); err != nil { // Ctrl+Q
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			mu.Lock()
			rendered := output.String()
			mu.Unlock()
			t.Fatalf("micro exited with %v; output prefix %q", err, rendered[:min(len(rendered), 3000)])
		}
	case <-ctx.Done():
		mu.Lock()
		rendered := output.String()
		mu.Unlock()
		t.Fatalf("micro did not exit after Ctrl+Q; output prefix %q", rendered[:min(len(rendered), 300)])
	}
	mu.Lock()
	rendered := output.String()
	mu.Unlock()
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("micro emitted no terminal control sequences: %q", rendered)
	}
}

func TestNanoCanStartAndExitInPTY(t *testing.T) {
	if _, err := exec.LookPath("nano"); err != nil {
		t.Skip("nano is not installed")
	}
	session, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	requireControllingTTY(t, session)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	block := &domain.Block{ID: "nano", Command: "nano", StartedAt: time.Now()}
	var mu sync.Mutex
	var output strings.Builder
	ready := make(chan struct{})
	var readyOnce sync.Once
	done := make(chan error, 1)
	go func() {
		done <- session.Run(ctx, block, func(chunk domain.OutputChunk) {
			mu.Lock()
			output.Write(chunk.Bytes())
			if strings.Contains(output.String(), "\x1b[") {
				readyOnce.Do(func() { close(ready) })
			}
			mu.Unlock()
		})
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("nano did not render its first terminal frame")
	}
	time.Sleep(120 * time.Millisecond)
	if err := session.Input("\x18"); err != nil { // Ctrl+X
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("nano exited with %v", err)
		}
	case <-ctx.Done():
		t.Fatal("nano did not exit after Ctrl+X")
	}
	mu.Lock()
	rendered := output.String()
	mu.Unlock()
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("nano emitted no terminal control sequences: %q", rendered)
	}
}

func requireControllingTTY(t *testing.T, session *Session) {
	t.Helper()
	block := &domain.Block{ID: "tty-probe", Command: `printf ok >/dev/tty`, StartedAt: time.Now()}
	var output strings.Builder
	if err := session.Run(context.Background(), block, func(chunk domain.OutputChunk) { output.Write(chunk.Bytes()) }); err != nil {
		if strings.Contains(output.String(), "operation not permitted") {
			t.Skip("test sandbox blocks /dev/tty")
		}
		t.Fatalf("controlling tty is unavailable: %v (%q)", err, output.String())
	}
}

func TestSessionPersistsWorkingDirectory(t *testing.T) {
	session, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	block := &domain.Block{ID: "cd", Command: "cd " + shellQuote(target), StartedAt: time.Now()}
	if err := session.Run(context.Background(), block, func(domain.OutputChunk) {}); err != nil {
		t.Fatal(err)
	}
	if got := session.CWD(); got != filepath.Clean(target) {
		t.Fatalf("cwd = %q, want %q", got, target)
	}

	var output strings.Builder
	block = &domain.Block{ID: "pwd", Command: "pwd", StartedAt: time.Now()}
	if err := session.Run(context.Background(), block, func(chunk domain.OutputChunk) { output.Write(chunk.Bytes()) }); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(output.String()); got != filepath.Clean(target) {
		t.Fatalf("pwd = %q, want %q", got, filepath.Clean(target))
	}
}

func TestSessionPreservesShellStateAcrossBlocks(t *testing.T) {
	workingDirectory := t.TempDir()
	session, err := NewSessionAt(workingDirectory, "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	first := &domain.Block{
		ID:        "state-one",
		Command:   `export NTERM_PERSIST_TEST=alive; nterm_test_function() { printf function-ok; }; umask 027`,
		StartedAt: time.Now(),
	}
	if err := session.Run(context.Background(), first, func(domain.OutputChunk) {}); err != nil {
		t.Fatal(err)
	}

	second := &domain.Block{
		ID:        "state-two",
		Command:   `printf '%s|' "$NTERM_PERSIST_TEST"; nterm_test_function; printf '|'; umask`,
		StartedAt: time.Now(),
	}
	var output strings.Builder
	if err := session.Run(context.Background(), second, func(chunk domain.OutputChunk) { output.Write(chunk.Bytes()) }); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(output.String()); got != "alive|function-ok|0027" {
		t.Fatalf("persistent shell state = %q", got)
	}
}

func TestSessionReportsActiveShellEnvironment(t *testing.T) {
	session, err := NewSessionAt(t.TempDir(), "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	activate := &domain.Block{
		ID: "activate-environment", Command: `export VIRTUAL_ENV=/tmp/example/.venv`, StartedAt: time.Now(),
	}
	if err := session.Run(context.Background(), activate, func(domain.OutputChunk) {}); err != nil {
		t.Fatal(err)
	}
	if activate.Environment != "Python · .venv" || session.Environment() != "Python · .venv" {
		t.Fatalf("active environment = block %q, session %q", activate.Environment, session.Environment())
	}

	deactivate := &domain.Block{
		ID: "deactivate-environment", Command: `unset VIRTUAL_ENV`, StartedAt: time.Now(),
	}
	if err := session.Run(context.Background(), deactivate, func(domain.OutputChunk) {}); err != nil {
		t.Fatal(err)
	}
	if deactivate.Environment != "" || session.Environment() != "" {
		t.Fatalf("environment was not cleared: block %q, session %q", deactivate.Environment, session.Environment())
	}
}

func TestSessionReusesTTYAcrossBlocks(t *testing.T) {
	session, err := NewSessionAt(t.TempDir(), "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	runTTY := func(id string) string {
		t.Helper()
		block := &domain.Block{ID: id, Command: "tty", StartedAt: time.Now()}
		var output strings.Builder
		if err := session.Run(context.Background(), block, func(chunk domain.OutputChunk) { output.Write(chunk.Bytes()) }); err != nil {
			if strings.Contains(output.String(), "operation not permitted") {
				t.Skip("test sandbox blocks /dev/tty")
			}
			t.Fatal(err)
		}
		return strings.TrimSpace(output.String())
	}

	first := runTTY("tty-one")
	second := runTTY("tty-two")
	if first == "" || strings.Contains(first, "not a tty") {
		t.Fatalf("first terminal device = %q", first)
	}
	if second != first {
		t.Fatalf("terminal device changed between blocks: %q -> %q", first, second)
	}
}

func TestSessionTreatsCleanShellExitAsSuccessAndRestarts(t *testing.T) {
	session, err := NewSessionAt(t.TempDir(), "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	exited := &domain.Block{ID: "exit", Command: "exit", StartedAt: time.Now()}
	if err := session.Run(context.Background(), exited, func(domain.OutputChunk) {}); err != nil {
		t.Fatalf("clean shell exit failed: %v", err)
	}

	restarted := &domain.Block{ID: "restart", Command: "printf restarted", StartedAt: time.Now()}
	var output strings.Builder
	if err := session.Run(context.Background(), restarted, func(chunk domain.OutputChunk) { output.Write(chunk.Bytes()) }); err != nil {
		t.Fatalf("command after clean shell exit failed: %v", err)
	}
	if output.String() != "restarted" {
		t.Fatalf("output after shell restart = %q", output.String())
	}
}

func TestSessionConfiguresColoredListings(t *testing.T) {
	session, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	block := &domain.Block{
		ID: "colors", Command: `printf '%s|%s|%s|%s|%s' "$TERM" "$COLORTERM" "$CLICOLOR_FORCE" "$LSCOLORS" "$LS_COLORS"`,
		StartedAt: time.Now(),
	}
	var output strings.Builder
	if err := session.Run(context.Background(), block, func(chunk domain.OutputChunk) { output.Write(chunk.Bytes()) }); err != nil {
		t.Fatal(err)
	}
	value := output.String()
	if !strings.HasPrefix(value, "xterm-256color|truecolor|1|GxFxCxDxBxegedabagaced|") || !strings.Contains(value, "*.zip=33") {
		t.Fatalf("listing colors are not configured: %q", value)
	}
}

func TestSessionCancellationStopsProcess(t *testing.T) {
	session, err := NewSessionAt(t.TempDir(), "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	state := &domain.Block{ID: "state", Command: "export NTERM_AFTER_CANCEL=preserved", StartedAt: time.Now()}
	if err := session.Run(context.Background(), state, func(domain.OutputChunk) {}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	block := &domain.Block{ID: "cancel", Command: "sleep 10", StartedAt: time.Now()}
	done := make(chan error, 1)
	go func() { done <- session.Run(ctx, block, func(domain.OutputChunk) {}) }()
	time.Sleep(80 * time.Millisecond)
	started := time.Now()
	cancel()
	select {
	case <-done:
		if elapsed := time.Since(started); elapsed > 2*time.Second {
			t.Fatalf("cancel took %s", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("process did not stop after cancellation")
	}
	block = &domain.Block{ID: "after-cancel", Command: `printf %s "$NTERM_AFTER_CANCEL"`, StartedAt: time.Now()}
	var output strings.Builder
	if err := session.Run(context.Background(), block, func(chunk domain.OutputChunk) { output.Write(chunk.Bytes()) }); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "preserved" {
		t.Fatalf("shell state after cancellation = %q", got)
	}
}

func TestRawControlCStopsForegroundCommand(t *testing.T) {
	session, err := NewSessionAt(t.TempDir(), "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	block := &domain.Block{ID: "raw-control-c", Command: "sleep 30", StartedAt: time.Now()}
	done := make(chan error, 1)
	go func() { done <- session.Run(ctx, block, func(domain.OutputChunk) {}) }()
	time.Sleep(150 * time.Millisecond)
	if err := session.Input("\x03"); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if code := ExitCode(err); code != 130 {
			t.Fatalf("Ctrl+C exit code = %d, want 130 (error %v)", code, err)
		}
	case <-ctx.Done():
		t.Fatal("raw Ctrl+C did not stop the foreground command")
	}
}
