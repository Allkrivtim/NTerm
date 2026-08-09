package gitstatus

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Info struct {
	IsRepository bool   `json:"isRepository"`
	Root         string `json:"root,omitempty"`
	Name         string `json:"name,omitempty"`
	Branch       string `json:"branch,omitempty"`
	Detached     bool   `json:"detached,omitempty"`
	Staged       int    `json:"staged,omitempty"`
	Modified     int    `json:"modified,omitempty"`
	Untracked    int    `json:"untracked,omitempty"`
	Ahead        int    `json:"ahead,omitempty"`
	Behind       int    `json:"behind,omitempty"`
}

func (i Info) Dirty() bool { return i.Staged+i.Modified+i.Untracked > 0 }

func Inspect(ctx context.Context, cwd string) Info {
	root, gitDir, ok := findRepository(cwd)
	if !ok {
		return Info{}
	}
	info := Info{IsRepository: true, Root: root, Name: filepath.Base(root)}
	info.Branch, info.Detached = readHead(gitDir)
	if path, err := exec.LookPath("git"); err == nil {
		readStatus(ctx, path, &info)
	}
	return info
}

func findRepository(cwd string) (string, string, bool) {
	current, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", false
	}
	for {
		marker := filepath.Join(current, ".git")
		if stat, err := os.Stat(marker); err == nil {
			if stat.IsDir() {
				return current, marker, true
			}
			if gitDir, ok := readGitFile(marker, current); ok {
				return current, gitDir, true
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", false
		}
		current = parent
	}
}

func readGitFile(path, root string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(data))
	if !strings.HasPrefix(value, "gitdir:") {
		return "", false
	}
	directory := strings.TrimSpace(strings.TrimPrefix(value, "gitdir:"))
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(root, directory)
	}
	return filepath.Clean(directory), true
}

func readHead(gitDir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", false
	}
	head := strings.TrimSpace(string(data))
	if strings.HasPrefix(head, "ref:") {
		return filepath.Base(strings.TrimSpace(strings.TrimPrefix(head, "ref:"))), false
	}
	if len(head) > 8 {
		head = head[:8]
	}
	return head, head != ""
}

func readStatus(ctx context.Context, gitPath string, info *Info) {
	command := exec.CommandContext(ctx, gitPath, "-C", info.Root, "status", "--porcelain=v2", "--branch", "--untracked-files=normal")
	command.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	var output cappedBuffer
	output.remaining = 2 << 20
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return
	}
	parseStatus(output.String(), info)
}

func parseStatus(output string, info *Info) {
	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			branch := strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
			if branch == "(detached)" {
				info.Detached = true
			} else if branch != "" {
				info.Branch = branch
				info.Detached = false
			}
		case strings.HasPrefix(line, "# branch.oid ") && info.Detached:
			oid := strings.TrimSpace(strings.TrimPrefix(line, "# branch.oid "))
			if len(oid) > 8 {
				oid = oid[:8]
			}
			info.Branch = oid
		case strings.HasPrefix(line, "# branch.ab "):
			parts := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(parts) == 2 {
				info.Ahead, _ = strconv.Atoi(strings.TrimPrefix(parts[0], "+"))
				info.Behind, _ = strconv.Atoi(strings.TrimPrefix(parts[1], "-"))
			}
		case strings.HasPrefix(line, "? "):
			info.Untracked++
		case strings.HasPrefix(line, "1 ") || strings.HasPrefix(line, "2 ") || strings.HasPrefix(line, "u "):
			parts := strings.Fields(line)
			if len(parts) < 2 || len(parts[1]) < 2 {
				continue
			}
			if parts[1][0] != '.' {
				info.Staged++
			}
			if parts[1][1] != '.' {
				info.Modified++
			}
		}
	}
}

// ParsePorcelainV2 applies git status --porcelain=v2 output to Info. It is
// exported so remote terminal sessions can reuse the exact same parser.
func ParsePorcelainV2(output string, info *Info) {
	parseStatus(output, info)
}

type cappedBuffer struct {
	buffer    bytes.Buffer
	remaining int
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	written := len(value)
	if b.remaining > 0 {
		chunk := value
		if len(chunk) > b.remaining {
			chunk = chunk[:b.remaining]
		}
		_, _ = b.buffer.Write(chunk)
		b.remaining -= len(chunk)
	}
	return written, nil
}

func (b *cappedBuffer) String() string { return b.buffer.String() }
