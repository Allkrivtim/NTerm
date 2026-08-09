package terminal

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexandr/nterm/internal/domain"
)

func TestSessionStreamsOutputAndExitCode(t *testing.T) {
	session, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	block := &domain.Block{ID: "test", Command: "printf hello; printf problem >&2; exit 7", StartedAt: time.Now()}
	var mu sync.Mutex
	var output strings.Builder
	err = session.Run(context.Background(), block, func(chunk domain.OutputChunk) {
		mu.Lock()
		output.WriteString(chunk.Data)
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
	if err := session.Run(context.Background(), block, func(chunk domain.OutputChunk) { output.WriteString(chunk.Data) }); err != nil {
		if strings.Contains(output.String(), "operation not permitted") {
			t.Skip("test sandbox blocks /dev/tty")
		}
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "tty-device") || !strings.Contains(output.String(), "27 91") {
		t.Fatalf("PTY size output = %q", output.String())
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
	done := make(chan error, 1)
	go func() {
		done <- session.Run(ctx, block, func(chunk domain.OutputChunk) {
			mu.Lock()
			output.WriteString(chunk.Data)
			mu.Unlock()
		})
	}()
	// Give the editor time to complete terminal initialization. This is not a
	// readiness protocol; the assertion is that it owns a TTY and accepts raw
	// control input without the nil /dev/tty panic reported for pipe execution.
	time.Sleep(350 * time.Millisecond)
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
	done := make(chan error, 1)
	go func() {
		done <- session.Run(ctx, block, func(chunk domain.OutputChunk) {
			mu.Lock()
			output.WriteString(chunk.Data)
			mu.Unlock()
		})
	}()
	time.Sleep(300 * time.Millisecond)
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
	if err := session.Run(context.Background(), block, func(chunk domain.OutputChunk) { output.WriteString(chunk.Data) }); err != nil {
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
	if err := session.Run(context.Background(), block, func(chunk domain.OutputChunk) { output.WriteString(chunk.Data) }); err != nil {
		t.Fatal(err)
	}
	physicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(output.String()); got != physicalTarget {
		t.Fatalf("pwd = %q, want %q", got, physicalTarget)
	}
}

func TestSessionConfiguresColoredListings(t *testing.T) {
	session, err := NewSession()
	if err != nil {
		t.Fatal(err)
	}
	block := &domain.Block{
		ID: "colors", Command: `printf '%s|%s|%s' "$CLICOLOR_FORCE" "$LSCOLORS" "$LS_COLORS"`,
		StartedAt: time.Now(),
	}
	var output strings.Builder
	if err := session.Run(context.Background(), block, func(chunk domain.OutputChunk) { output.WriteString(chunk.Data) }); err != nil {
		t.Fatal(err)
	}
	value := output.String()
	if !strings.HasPrefix(value, "1|GxFxCxDxBxegedabagaced|") || !strings.Contains(value, "*.zip=33") {
		t.Fatalf("listing colors are not configured: %q", value)
	}
}

func TestSessionCancellationStopsProcess(t *testing.T) {
	session, err := NewSession()
	if err != nil {
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
}
