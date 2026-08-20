package workspace

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexandr/nterm/internal/domain"
)

func TestStoreRoundTripAndRecoversRunningBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	tab := domain.Tab{ID: "tab-1", Title: "project", CWD: t.TempDir()}
	if err := store.SaveTab(tab, "go test ./...", 0); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveTab(tab.ID); err != nil {
		t.Fatal(err)
	}
	block := domain.Block{
		ID: "block-1", TabID: tab.ID, Command: "go test ./...", CWD: tab.CWD,
		State: domain.BlockRunning, StartedAt: time.Now().Add(-time.Second),
	}
	if err := store.SaveBlock(block); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendOutput(block.ID, "partial output\n"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	snapshot, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActiveTabID != tab.ID || len(snapshot.Tabs) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	got := snapshot.Tabs[0]
	if got.Draft != "go test ./..." || len(got.Blocks) != 1 {
		t.Fatalf("restored tab = %#v", got)
	}
	if got.Blocks[0].Block.State != domain.BlockCancelled || got.Blocks[0].Block.EndedAt == nil {
		t.Fatalf("running block was not recovered as cancelled: %#v", got.Blocks[0])
	}
	if got.Blocks[0].Output != "partial output\n" {
		t.Fatalf("output = %q", got.Blocks[0].Output)
	}
}

func TestStoreBoundsPersistedOutputAndClearsFinishedBlocks(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	tab := domain.Tab{ID: "tab", Title: "tab", CWD: t.TempDir()}
	if err := store.SaveTab(tab, "", 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	exitCode := 0
	block := domain.Block{ID: "block", TabID: tab.ID, Command: "yes", CWD: tab.CWD,
		State: domain.BlockSucceeded, ExitCode: &exitCode, StartedAt: now, EndedAt: &now}
	if err := store.SaveBlock(block); err != nil {
		t.Fatal(err)
	}
	value := strings.Repeat("x", maxPersistedOutputBytes+1024)
	if err := store.AppendOutput(block.ID, value); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(snapshot.Tabs[0].Blocks[0].Output); got != maxPersistedOutputBytes {
		t.Fatalf("persisted output size = %d", got)
	}
	if err := store.ClearFinishedBlocks(tab.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tabs[0].Blocks) != 0 {
		t.Fatalf("finished blocks remain: %#v", snapshot.Tabs[0].Blocks)
	}
}
