package completion

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexandr/nterm/internal/terminal"
)

type fakeContext struct {
	cwd     string
	history []string
}

type fakeRemoteContext struct {
	fakeContext
	pathCalls int
	entries   []terminal.RemotePathEntry
}

func (f *fakeRemoteContext) CompletePath(_ context.Context, _ string) ([]terminal.RemotePathEntry, bool) {
	f.pathCalls++
	return append([]terminal.RemotePathEntry(nil), f.entries...), true
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func mockProvider(handler func(*http.Request) string) *LocalModelProvider {
	provider := NewLocalModelProvider()
	provider.endpoint = "http://127.0.0.1:11434"
	provider.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := handler(request)
		return &http.Response{
			StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body)),
			Header: make(http.Header), Request: request,
		}, nil
	})}
	return provider
}

func (f fakeContext) CWD() string       { return f.cwd }
func (f fakeContext) History() []string { return f.history }

func TestSuggestUsesHistoryFirst(t *testing.T) {
	service := &Service{session: fakeContext{history: []string{"git status", "git log"}}}
	items := service.Suggest(context.Background(), "git", 8)
	if len(items) < 2 || items[0].Value != "git log" || items[1].Value != "git status" {
		t.Fatalf("unexpected history suggestions: %#v", items)
	}
}

func TestSuggestCompletesFilesRelativeToCWD(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "notes.txt"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
	service := &Service{session: fakeContext{cwd: directory}}
	items := service.Suggest(context.Background(), "cat no", 8)
	if len(items) != 2 {
		t.Fatalf("suggestions = %#v, want file and directory", items)
	}
	if items[0].Value != "cat node_modules/" || items[1].Value != "cat notes.txt" {
		t.Fatalf("unexpected suggestions: %#v", items)
	}
}

func TestSuggestEscapesAndQuotesPathsWithSpaces(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "My File.txt"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := &Service{session: fakeContext{cwd: directory}}
	unquoted := service.Suggest(context.Background(), "cat My", 8)
	if len(unquoted) != 1 || unquoted[0].Value != `cat My\ File.txt` {
		t.Fatalf("unquoted suggestions = %#v", unquoted)
	}
	quoted := service.Suggest(context.Background(), `cat "My`, 8)
	if len(quoted) != 1 || quoted[0].Value != `cat "My File.txt"` {
		t.Fatalf("quoted suggestions = %#v", quoted)
	}
}

func TestSuggestCachesRemoteDirectoryAcrossKeystrokes(t *testing.T) {
	remote := &fakeRemoteContext{
		fakeContext: fakeContext{cwd: "/srv/project"},
		entries: []terminal.RemotePathEntry{
			{Value: "src/api", IsDir: true},
			{Value: "src/app.go"},
		},
	}
	service := &Service{session: remote}
	first := service.Suggest(context.Background(), "cat src/a", 8)
	second := service.Suggest(context.Background(), "cat src/ap", 8)
	if remote.pathCalls != 1 {
		t.Fatalf("remote directory calls = %d, want 1", remote.pathCalls)
	}
	if len(first) != 2 || len(second) != 2 || second[0].Value != "cat src/api/" {
		t.Fatalf("cached suggestions = %#v then %#v", first, second)
	}
}

func TestSuggestCompletesKnownCommandSpecs(t *testing.T) {
	service := &Service{session: fakeContext{}}
	items := service.Suggest(context.Background(), "git che", 8)
	if len(items) == 0 || items[0].Value != "git checkout" || items[0].Source != "spec" {
		t.Fatalf("unexpected spec suggestions: %#v", items)
	}
}

func TestCommandsContainBuiltinsAndRealExecutables(t *testing.T) {
	service := NewService(fakeContext{})
	commands := service.Commands()
	contains := func(want string) bool {
		for _, command := range commands {
			if command == want {
				return true
			}
		}
		return false
	}
	if !contains("cd") {
		t.Fatal("command inventory must contain shell builtins")
	}
	if !contains("go") {
		t.Fatal("command inventory must contain executables discovered in PATH")
	}
}

func TestLocalModelPredictionUsesSmallResourceBudgetAndCaches(t *testing.T) {
	requests := 0
	provider := mockProvider(func(request *http.Request) string {
		requests++
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["max_tokens"] != float64(64) || body["stream"] != false {
			t.Fatalf("unexpected resource options: %#v", body)
		}
		return `{"choices":[{"message":{"content":"{\"command\":\"git status --short\"}"}}]}`
	})
	service := NewService(fakeContext{cwd: "/tmp", history: []string{"git diff"}}, provider)
	for range 2 {
		got, err := service.Predict(context.Background(), "git st", "qwen2.5-coder:0.5b")
		if err != nil || got.Value != "git status --short" {
			t.Fatalf("prediction = %#v, %v", got, err)
		}
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want cached single request", requests)
	}
}

func TestLocalModelReusesCompletionWhilePrefixGrows(t *testing.T) {
	requests := 0
	provider := mockProvider(func(*http.Request) string {
		requests++
		return `{"choices":[{"message":{"content":"{\"command\":\"git status --short\"}"}}]}`
	})
	contextValue := PredictionContext{Input: "git st", CWD: "/tmp", History: []string{"git diff"}}
	first, err := provider.Predict(context.Background(), contextValue)
	if err != nil {
		t.Fatal(err)
	}
	contextValue.Input = "git sta"
	second, err := provider.Predict(context.Background(), contextValue)
	if err != nil || second.Value != first.Value || requests != 1 {
		t.Fatalf("continued prediction = %#v, err=%v, requests=%d", second, err, requests)
	}
}

func TestLocalModelRefinesFilesystemCandidates(t *testing.T) {
	var requestBody []byte
	provider := mockProvider(func(request *http.Request) string {
		requestBody, _ = io.ReadAll(request.Body)
		return `{"choices":[{"message":{"content":"{\"command\":\"cat /tmp/alpha.txt\"}"}}]}`
	})
	got, err := provider.Predict(context.Background(), PredictionContext{
		Input: "cat /tmp/a", CWD: "/tmp",
		Candidates: []string{"cat /tmp/alpha.txt", "cat /tmp/archive/"},
	})
	if err != nil || got.Value != "cat /tmp/alpha.txt" {
		t.Fatalf("filesystem prediction = %#v, %v", got, err)
	}
	if !strings.Contains(string(requestBody), "Fast deterministic candidates") || !strings.Contains(string(requestBody), "cat /tmp/archive/") {
		t.Fatalf("filesystem candidates missing from prompt: %s", requestBody)
	}

	inventingProvider := mockProvider(func(*http.Request) string {
		return `{"choices":[{"message":{"content":"{\"command\":\"cat /tmp/absent.txt\"}"}}]}`
	})
	_, err = inventingProvider.Predict(context.Background(), PredictionContext{
		Input: "cat /tmp/a", CWD: "/tmp", Candidates: []string{"cat /tmp/alpha.txt"},
	})
	if err == nil {
		t.Fatal("model must not invent a path outside deterministic candidates")
	}
}

func TestLocalModelPredictionRejectsChangedPrefix(t *testing.T) {
	provider := mockProvider(func(*http.Request) string {
		return `{"choices":[{"message":{"content":"{\"command\":\"rm -rf /\"}"}}]}`
	})
	_, err := provider.Predict(context.Background(), PredictionContext{Input: "git st", CWD: "/tmp", Model: "qwen2.5-coder:0.5b"})
	if err == nil {
		t.Fatal("expected prefix validation error")
	}
}

func TestLocalModelPromptContainsRecentContextOnly(t *testing.T) {
	var requestBody []byte
	provider := mockProvider(func(request *http.Request) string {
		requestBody, _ = io.ReadAll(request.Body)
		return `{"choices":[{"message":{"content":"{\"command\":\"go test ./...\"}"}}]}`
	})
	history := []string{"old-secret", "one", "two", "three", "four", "five", "six"}
	_, err := provider.Predict(context.Background(), PredictionContext{Input: "go te", CWD: "/repo", History: history, Model: "qwen2.5-coder:0.5b"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(requestBody), "old-secret") || !strings.Contains(string(requestBody), "six") {
		t.Fatalf("unexpected history window: %s", requestBody)
	}
}

func TestBundledModelIntegration(t *testing.T) {
	if os.Getenv("NTERM_LOCAL_AI_INTEGRATION") != "1" {
		t.Skip("set NTERM_LOCAL_AI_INTEGRATION=1 to test the bundled model")
	}
	provider := NewLocalModelProvider()
	defer provider.Close()
	status := provider.Status()
	if status.State != "ready" {
		t.Fatalf("built-in model status = %#v", status)
	}
	started := time.Now()
	got, err := provider.Predict(context.Background(), PredictionContext{
		Input: "git st", CWD: "/tmp", History: []string{"git diff", "git add ."},
		Model: "qwen2.5-coder:0.5b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Value, "git st") {
		t.Fatalf("prediction = %#v", got)
	}
	t.Logf("prediction %q in %s", got.Value, time.Since(started))
}
