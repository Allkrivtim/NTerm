package completion

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/alexandr/nterm/internal/terminal"
)

type Suggestion struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`
}

type ContextSource interface {
	CWD() string
	History() []string
}

type Service struct {
	session     ContextSource
	executables []string
	predictor   *LocalModelProvider
	shell       string
	shellOnce   sync.Once
	shellNames  []string
}

var shellBuiltins = []string{
	"cd", "echo", "export", "history", "jobs", "kill", "pwd", "source", "type", "unset", "which",
}

func NewService(session ContextSource, predictor ...*LocalModelProvider) *Service {
	service := &Service{session: session, executables: discoverExecutables()}
	if source, ok := session.(interface{ Shell() string }); ok {
		service.shell = source.Shell()
	}
	if len(predictor) > 0 {
		service.predictor = predictor[0]
	}
	return service
}

// Commands returns the command names that are actually available to the
// terminal: shell builtins plus executable files found in PATH. The frontend
// uses this cached inventory for syntax highlighting without touching the
// filesystem on every keystroke.
func (s *Service) Commands() []string {
	commands := make([]string, 0, len(shellBuiltins)+len(s.executables))
	commands = append(commands, shellBuiltins...)
	commands = append(commands, s.executables...)
	sort.Strings(commands)
	return deduplicateStrings(commands)
}

// ShellCommands augments PATH with aliases, functions and builtins loaded by
// the user's login shell. It is intentionally lazy and cached: opening the
// window and typing never waits on shell startup more than once.
func (s *Service) ShellCommands(ctx context.Context) []string {
	s.shellOnce.Do(func() {
		s.shellNames = discoverShellCommands(ctx, s.shell)
	})
	commands := append(s.Commands(), s.shellNames...)
	sort.Strings(commands)
	return deduplicateStrings(commands)
}

// Suggest is intentionally deterministic and fast. Model inference lives in
// Predict so a slow or unavailable model can never delay normal completion.
func (s *Service) Suggest(ctx context.Context, input string, limit int) []Suggestion {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	items := append(s.historySuggestions(input), s.localSuggestions(ctx, input)...)
	items = deduplicate(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (s *Service) Predict(ctx context.Context, input, model string) (Suggestion, error) {
	if s.predictor == nil {
		return Suggestion{}, ErrAIUnavailable
	}
	return s.predictor.Predict(ctx, PredictionContext{
		Input: input, CWD: s.session.CWD(), History: s.session.History(), Model: model,
	})
}

func (s *Service) historySuggestions(input string) []Suggestion {
	history := s.session.History()
	items := make([]Suggestion, 0, 4)
	for i := len(history) - 1; i >= 0; i-- {
		if input == "" || (strings.HasPrefix(history[i], input) && history[i] != input) {
			items = append(items, Suggestion{Value: history[i], Label: history[i], Description: "recent command", Source: "history"})
		}
	}
	return items
}

func (s *Service) localSuggestions(ctx context.Context, input string) []Suggestion {
	trimmedLeft := strings.TrimLeft(input, " \t")
	parts := strings.Fields(trimmedLeft)
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 && !strings.ContainsAny(trimmedLeft, " \t") {
		prefix := parts[0]
		items := make([]Suggestion, 0, 8)
		candidates := append(shellBuiltins, s.executables...)
		if source, ok := s.session.(interface {
			CommandNames(context.Context) ([]string, bool)
		}); ok {
			if remoteCommands, remote := source.CommandNames(ctx); remote {
				candidates = remoteCommands
			}
		}
		for _, candidate := range candidates {
			if strings.HasPrefix(candidate, prefix) && candidate != prefix {
				items = append(items, Suggestion{Value: candidate, Label: candidate, Description: "command", Source: "path"})
			}
		}
		sort.SliceStable(items, func(i, j int) bool {
			return len(items[i].Value) < len(items[j].Value) || (len(items[i].Value) == len(items[j].Value) && items[i].Value < items[j].Value)
		})
		return items
	}
	items := specSuggestions(input)
	items = append(items, s.fileSuggestions(ctx, input)...)
	return items
}

func (s *Service) fileSuggestions(ctx context.Context, input string) []Suggestion {
	lastSpace := strings.LastIndexAny(input, " \t\n")
	prefixStart := lastSpace + 1
	token := input[prefixStart:]
	cleanToken := strings.TrimPrefix(token, "\"")
	if source, ok := s.session.(interface {
		CompletePath(context.Context, string) ([]terminal.RemotePathEntry, bool)
	}); ok {
		if entries, handled := source.CompletePath(ctx, cleanToken); handled {
			items := make([]Suggestion, 0, len(entries))
			for _, entry := range entries {
				description := "file"
				if entry.IsDir {
					description = "directory"
				}
				items = append(items, Suggestion{Value: input[:prefixStart] + entry.Value, Label: entry.Value, Description: description, Source: "files"})
			}
			return items
		}
	}
	base := filepath.Dir(cleanToken)
	namePrefix := filepath.Base(cleanToken)
	searchDir := base
	if base == "." {
		searchDir = s.session.CWD()
	} else if !filepath.IsAbs(base) {
		searchDir = filepath.Join(s.session.CWD(), base)
	}
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}
	items := make([]Suggestion, 0, 8)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") && !strings.HasPrefix(namePrefix, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(entry.Name()), strings.ToLower(namePrefix)) {
			continue
		}
		completed := entry.Name()
		if base != "." {
			completed = filepath.Join(base, completed)
		}
		description := "file"
		if entry.IsDir() {
			completed += string(filepath.Separator)
			description = "directory"
		}
		items = append(items, Suggestion{
			Value: input[:prefixStart] + completed, Label: completed, Description: description, Source: "files",
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items
}

func discoverExecutables() []string {
	seen := map[string]struct{}{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err == nil && info.Mode()&0o111 != 0 {
				seen[entry.Name()] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func discoverShellCommands(ctx context.Context, shell string) []string {
	if shell == "" {
		return nil
	}
	var script string
	switch filepath.Base(shell) {
	case "zsh":
		script = `printf '%s\n' ${(k)commands} ${(k)aliases} ${(k)builtins} ${(k)functions}`
	case "bash":
		script = "compgen -c"
	default:
		return nil
	}
	command := exec.CommandContext(ctx, shell, "-lic", script)
	command.Stderr = nil
	output, err := command.Output()
	if err != nil {
		return nil
	}
	result := make([]string, 0, 256)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name == "" || len(name) > 128 || strings.ContainsAny(name, " \t\r\n\x00") {
			continue
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return deduplicateStrings(result)
}

func deduplicate(items []Suggestion) []Suggestion {
	seen := make(map[string]struct{}, len(items))
	result := make([]Suggestion, 0, len(items))
	for _, item := range items {
		if item.Value == "" {
			continue
		}
		if _, ok := seen[item.Value]; ok {
			continue
		}
		seen[item.Value] = struct{}{}
		result = append(result, item)
	}
	return result
}

func deduplicateStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
