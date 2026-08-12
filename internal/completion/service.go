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
	"time"

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
	shellMu     sync.Mutex
	shellLoaded bool
	shellNames  []string
	pathMu      sync.Mutex
	pathCache   map[string]pathCacheEntry
	pathPending map[string]chan struct{}
}

type pathCacheEntry struct {
	entries   []terminal.RemotePathEntry
	expiresAt time.Time
}

type completionToken struct {
	start  int
	prefix string
	raw    string
	clean  string
	quote  byte
}

var shellBuiltins = []string{
	"cd", "echo", "export", "history", "jobs", "kill", "pwd", "source", "type", "unset", "which",
}

func NewService(session ContextSource, predictor ...*LocalModelProvider) *Service {
	service := &Service{
		session: session, executables: discoverExecutables(),
		pathCache: make(map[string]pathCacheEntry), pathPending: make(map[string]chan struct{}),
	}
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
	s.shellMu.Lock()
	loaded := s.shellLoaded
	names := append([]string(nil), s.shellNames...)
	s.shellMu.Unlock()
	if !loaded {
		discovered := discoverShellCommands(ctx, s.shell)
		// A cancelled first request must not poison the cache for the lifetime of
		// the tab. Only a completed shell probe, including a legitimate empty
		// result for an unsupported shell, is final.
		if ctx.Err() == nil {
			s.shellMu.Lock()
			if !s.shellLoaded {
				s.shellNames = append([]string(nil), discovered...)
				s.shellLoaded = true
			}
			names = append([]string(nil), s.shellNames...)
			s.shellMu.Unlock()
		}
	}
	commands := append(s.Commands(), names...)
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

func (s *Service) Predict(ctx context.Context, input, model string, candidates ...string) (Suggestion, error) {
	if s.predictor == nil {
		return Suggestion{}, ErrAIUnavailable
	}
	return s.predictor.Predict(ctx, PredictionContext{
		Input: input, CWD: s.session.CWD(), History: s.session.History(), Model: model, Candidates: candidates,
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
	token := parseCompletionToken(input)
	base := filepath.Dir(token.clean)
	directoryPrefix := ""
	if base != "." {
		directoryPrefix = base
		if !strings.HasSuffix(directoryPrefix, string(filepath.Separator)) {
			directoryPrefix += string(filepath.Separator)
		}
	}
	entries := s.cachedDirectoryEntries(ctx, directoryPrefix)
	namePrefix := filepath.Base(token.clean)
	items := make([]Suggestion, 0, min(len(entries), 24))
	for _, entry := range entries {
		if entry.IsDir && !strings.HasSuffix(entry.Value, string(filepath.Separator)) {
			entry.Value += string(filepath.Separator)
		}
		name := strings.TrimSuffix(filepath.Base(entry.Value), string(filepath.Separator))
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(namePrefix, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(entry.Value), strings.ToLower(token.clean)) {
			continue
		}
		value := renderPathCompletion(token, entry)
		if value == input {
			continue
		}
		description := "file"
		if entry.IsDir {
			description = "directory"
		}
		items = append(items, Suggestion{Value: value, Label: entry.Value, Description: description, Source: "files"})
		if len(items) == 24 {
			break
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Description != items[j].Description {
			return items[i].Description == "directory"
		}
		return strings.ToLower(items[i].Label) < strings.ToLower(items[j].Label)
	})
	return items
}

func (s *Service) cachedDirectoryEntries(ctx context.Context, directoryPrefix string) []terminal.RemotePathEntry {
	key := s.session.CWD() + "\x00" + directoryPrefix
	now := time.Now()
	s.pathMu.Lock()
	if entry, ok := s.pathCache[key]; ok && now.Before(entry.expiresAt) {
		result := append([]terminal.RemotePathEntry(nil), entry.entries...)
		s.pathMu.Unlock()
		return result
	}
	if pending := s.pathPending[key]; pending != nil {
		s.pathMu.Unlock()
		select {
		case <-pending:
			s.pathMu.Lock()
			entry := s.pathCache[key]
			result := append([]terminal.RemotePathEntry(nil), entry.entries...)
			s.pathMu.Unlock()
			return result
		case <-ctx.Done():
			return nil
		}
	}
	if s.pathPending == nil {
		s.pathPending = make(map[string]chan struct{})
	}
	pending := make(chan struct{})
	s.pathPending[key] = pending
	s.pathMu.Unlock()
	defer s.finishDirectoryLoad(key, pending)

	if source, ok := s.session.(interface {
		CompletePath(context.Context, string) ([]terminal.RemotePathEntry, bool)
	}); ok {
		if entries, handled := source.CompletePath(ctx, directoryPrefix); handled {
			s.storeDirectoryEntries(key, entries, now.Add(3*time.Second))
			return entries
		}
	}
	searchDir := directoryPrefix
	if searchDir != string(filepath.Separator) {
		searchDir = strings.TrimSuffix(searchDir, string(filepath.Separator))
	}
	if searchDir == "" {
		searchDir = s.session.CWD()
	} else if searchDir == "~" || strings.HasPrefix(searchDir, "~"+string(filepath.Separator)) {
		if home, err := os.UserHomeDir(); err == nil {
			if searchDir == "~" {
				searchDir = home
			} else {
				searchDir = filepath.Join(home, strings.TrimPrefix(searchDir, "~"+string(filepath.Separator)))
			}
		}
	} else if !filepath.IsAbs(searchDir) {
		searchDir = filepath.Join(s.session.CWD(), searchDir)
	}
	directoryEntries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}
	entries := make([]terminal.RemotePathEntry, 0, len(directoryEntries))
	for _, entry := range directoryEntries {
		completed := directoryPrefix + entry.Name()
		if entry.IsDir() {
			completed += string(filepath.Separator)
		}
		entries = append(entries, terminal.RemotePathEntry{Value: completed, IsDir: entry.IsDir()})
	}
	sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Value) < strings.ToLower(entries[j].Value) })
	s.storeDirectoryEntries(key, entries, now.Add(1500*time.Millisecond))
	return entries
}

func (s *Service) finishDirectoryLoad(key string, pending chan struct{}) {
	s.pathMu.Lock()
	defer s.pathMu.Unlock()
	if s.pathPending[key] == pending {
		delete(s.pathPending, key)
		close(pending)
	}
}

func (s *Service) storeDirectoryEntries(key string, entries []terminal.RemotePathEntry, expiresAt time.Time) {
	s.pathMu.Lock()
	defer s.pathMu.Unlock()
	if s.pathCache == nil {
		s.pathCache = make(map[string]pathCacheEntry)
	}
	if len(s.pathCache) >= 48 {
		for cacheKey, entry := range s.pathCache {
			if time.Now().After(entry.expiresAt) || len(s.pathCache) >= 48 {
				delete(s.pathCache, cacheKey)
			}
		}
	}
	s.pathCache[key] = pathCacheEntry{entries: append([]terminal.RemotePathEntry(nil), entries...), expiresAt: expiresAt}
}

func parseCompletionToken(input string) completionToken {
	start := 0
	quote := byte(0)
	escaped := false
	for index := 0; index < len(input); index++ {
		char := input[index]
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char == ' ' || char == '\t' || char == '\n' || strings.ContainsRune("|;&<>", rune(char)) {
			start = index + 1
		}
	}
	raw := input[start:]
	openingQuote := byte(0)
	clean := raw
	if len(clean) > 0 && (clean[0] == '\'' || clean[0] == '"') {
		openingQuote = clean[0]
		clean = clean[1:]
	}
	clean = unescapePathToken(clean, openingQuote)
	return completionToken{start: start, prefix: input[:start], raw: raw, clean: clean, quote: openingQuote}
}

func unescapePathToken(value string, quote byte) string {
	if quote == '\'' {
		return strings.TrimSuffix(value, "'")
	}
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] == '\\' && index+1 < len(value) {
			index++
		}
		result.WriteByte(value[index])
	}
	if quote == '"' {
		return strings.TrimSuffix(result.String(), "\"")
	}
	return result.String()
}

func renderPathCompletion(token completionToken, entry terminal.RemotePathEntry) string {
	completed := entry.Value
	if token.quote != 0 {
		completed = string(token.quote) + completed
		if !entry.IsDir {
			completed += string(token.quote)
		}
	} else {
		completed = escapePathToken(completed)
	}
	return completedWithPrefix(token, completed)
}

func completedWithPrefix(token completionToken, completed string) string {
	return token.prefix + completed
}

func escapePathToken(value string) string {
	return strings.NewReplacer("\\", "\\\\", " ", "\\ ", "\t", "\\\t", "\"", "\\\"", "'", "\\'").Replace(value)
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
