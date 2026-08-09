package gitstatus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectFindsRepositoryFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "one", "two")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	info := Inspect(context.Background(), nested)
	if !info.IsRepository || info.Root != root || info.Branch != "main" || info.Name != filepath.Base(root) {
		t.Fatalf("unexpected info: %#v", info)
	}
}

func TestParsePorcelainV2(t *testing.T) {
	info := Info{}
	parseStatus("# branch.oid abcdef123456\n# branch.head feature/ui\n# branch.ab +2 -1\n1 M. N... 0 0 0 a b file\n1 .M N... 0 0 0 a b file2\n? new.txt\n", &info)
	if info.Branch != "feature/ui" || info.Ahead != 2 || info.Behind != 1 || info.Staged != 1 || info.Modified != 1 || info.Untracked != 1 {
		t.Fatalf("unexpected info: %#v", info)
	}
}
