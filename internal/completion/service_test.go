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
)

type fakeContext struct {
	cwd     string
	history []string
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
