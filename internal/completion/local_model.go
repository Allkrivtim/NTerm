package completion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrAIUnavailable = errors.New("built-in AI is unavailable")

const (
	bundledModelID   = "qwen2.5-coder:0.5b"
	bundledModelFile = "qwen2.5-coder-0.5b-q4km.gguf"
	modelIdleTimeout = 90 * time.Second
)

type PredictionContext struct {
	Input   string
	CWD     string
	History []string
	Model   string
}

type AIStatus struct {
	State   string `json:"state"`
	Model   string `json:"model"`
	Message string `json:"message"`
}

type predictionCacheEntry struct {
	suggestion Suggestion
	expiresAt  time.Time
}

// LocalModelProvider owns a bundled llama.cpp process. It is started lazily,
// shared by every tab, and stopped after a short idle window.
type LocalModelProvider struct {
	client   *http.Client
	endpoint string // Used by tests; production gets it from runtime.
	runtime  *localModelRuntime
	gate     chan struct{}
	mu       sync.Mutex
	cache    map[string]predictionCacheEntry
}

func NewLocalModelProvider() *LocalModelProvider {
	return &LocalModelProvider{
		client: &http.Client{Timeout: 2800 * time.Millisecond}, runtime: newLocalModelRuntime(),
		gate: make(chan struct{}, 1), cache: make(map[string]predictionCacheEntry),
	}
}

func (p *LocalModelProvider) Close() { p.runtime.Close() }

func (p *LocalModelProvider) Predict(ctx context.Context, value PredictionContext) (Suggestion, error) {
	input := strings.TrimSpace(value.Input)
	if len(input) < 3 || strings.ContainsAny(value.Input, "\r\n") {
		return Suggestion{}, ErrAIUnavailable
	}
	model := bundledModelID
	key := predictionCacheKey(value, model)
	if suggestion, ok := p.cached(key); ok {
		return suggestion, nil
	}
	select {
	case p.gate <- struct{}{}:
		defer func() { <-p.gate }()
	case <-ctx.Done():
		return Suggestion{}, ctx.Err()
	}

	endpoint := p.endpoint
	if endpoint == "" {
		var err error
		endpoint, err = p.runtime.EnsureRunning(ctx)
		if err != nil {
			return Suggestion{}, err
		}
	}
	history := recentHistory(value.History, 6)
	prompt := "Examples:\nExact command prefix: git st\nResult: {\"command\":\"git status\"}\nExact command prefix: go te\nResult: {\"command\":\"go test ./...\"}\n\nWorking directory: " + value.CWD + "\n"
	if history != "" {
		prompt += "Recent commands, oldest first:\n" + history + "\n"
	}
	prompt += "Exact command prefix: " + value.Input
	body, err := json.Marshal(map[string]any{
		"model": bundledModelID,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a shell autocomplete engine. Produce one safe, likely, complete shell command that starts exactly with the supplied prefix and is strictly longer than that prefix. Never return the unchanged prefix. Never explain, never use markdown, never invent file paths. Return JSON only."},
			{"role": "user", "content": prompt},
		},
		"stream": false, "temperature": 0, "seed": 7, "max_tokens": 64,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name": "shell_completion", "strict": true,
				"schema": map[string]any{
					"type": "object", "additionalProperties": false,
					"properties": map[string]any{"command": map[string]string{"type": "string"}},
					"required":   []string{"command"},
				},
			},
		},
	})
	if err != nil {
		return Suggestion{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Suggestion{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(req)
	if err != nil {
		return Suggestion{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Suggestion{}, fmt.Errorf("local model returned %s", response.Status)
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Suggestion{}, err
	}
	if len(payload.Choices) == 0 {
		return Suggestion{}, errors.New("local model returned no completion")
	}
	command := parsePredictedCommand(payload.Choices[0].Message.Content)
	if !validPrediction(value.Input, command) {
		return Suggestion{}, fmt.Errorf("model returned an invalid completion %q", command)
	}
	suggestion := Suggestion{Value: command, Label: command, Description: "built-in · 0.5B", Source: "ai"}
	p.store(key, suggestion)
	return suggestion, nil
}

func (p *LocalModelProvider) Status() AIStatus {
	if _, _, err := p.runtime.AssetPaths(); err != nil {
		return AIStatus{State: "unavailable", Model: bundledModelID, Message: "Built-in model assets are missing"}
	}
	return AIStatus{State: "ready", Model: bundledModelID, Message: "Built-in model · ready"}
}

type localModelRuntime struct {
	mu         sync.Mutex
	command    *exec.Cmd
	wait       chan error
	endpoint   string
	ready      bool
	idleTimer  *time.Timer
	generation uint64
}

func newLocalModelRuntime() *localModelRuntime { return &localModelRuntime{} }

func (r *localModelRuntime) AssetPaths() (string, string, error) {
	if runtimePath, modelPath := os.Getenv("NTERM_AI_RUNTIME"), os.Getenv("NTERM_AI_MODEL"); runtimePath != "" || modelPath != "" {
		if runtimePath == "" || modelPath == "" {
			return "", "", errors.New("both NTERM_AI_RUNTIME and NTERM_AI_MODEL are required")
		}
		return validateAIAssets(runtimePath, modelPath)
	}
	executable, err := os.Executable()
	if err == nil {
		resourceDirectory := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Resources", "ai"))
		if runtimePath, modelPath, assetErr := validateAIAssets(
			filepath.Join(resourceDirectory, "llama-server"), filepath.Join(resourceDirectory, bundledModelFile),
		); assetErr == nil {
			return runtimePath, modelPath, nil
		}
	}
	developmentDirectories := []string{filepath.Join("build", "ai", runtime.GOOS+"-"+runtime.GOARCH)}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		projectRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
		developmentDirectories = append(developmentDirectories, filepath.Join(projectRoot, "build", "ai", runtime.GOOS+"-"+runtime.GOARCH))
	}
	for _, developmentDirectory := range developmentDirectories {
		if runtimePath, modelPath, assetErr := validateAIAssets(
			filepath.Join(developmentDirectory, "llama-server"), filepath.Join(developmentDirectory, bundledModelFile),
		); assetErr == nil {
			return runtimePath, modelPath, nil
		}
	}
	return "", "", errors.New("built-in AI assets were not found")
}

func validateAIAssets(runtimePath, modelPath string) (string, string, error) {
	runtimeInfo, err := os.Stat(runtimePath)
	if err != nil || runtimeInfo.IsDir() || runtimeInfo.Mode()&0o111 == 0 {
		return "", "", errors.New("built-in inference runtime is unavailable")
	}
	modelInfo, err := os.Stat(modelPath)
	if err != nil || modelInfo.IsDir() || modelInfo.Size() < 100*1024*1024 {
		return "", "", errors.New("built-in model is unavailable")
	}
	return filepath.Clean(runtimePath), filepath.Clean(modelPath), nil
}

func (r *localModelRuntime) EnsureRunning(ctx context.Context) (string, error) {
	r.mu.Lock()
	if r.command != nil {
		endpoint := r.endpoint
		ready := r.ready
		command := r.command
		wait := r.wait
		r.resetIdleTimerLocked()
		r.mu.Unlock()
		if ready {
			return endpoint, nil
		}
		return r.waitUntilReady(ctx, command, wait, endpoint)
	}
	runtimePath, modelPath, err := r.AssetPaths()
	if err != nil {
		r.mu.Unlock()
		return "", err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		r.mu.Unlock()
		return "", err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	command := exec.Command(runtimePath,
		"--model", modelPath,
		"--ctx-size", "1024",
		"--predict", "64",
		"--parallel", "1",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--no-webui",
		"--no-slots",
	)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if os.Getenv("NTERM_AI_DEBUG") == "1" {
		command.Stdout = os.Stderr
		command.Stderr = os.Stderr
	}
	if err := command.Start(); err != nil {
		r.mu.Unlock()
		return "", fmt.Errorf("start built-in model: %w", err)
	}
	r.command = command
	r.endpoint = "http://127.0.0.1:" + strconv.Itoa(port)
	r.ready = false
	r.wait = make(chan error, 1)
	wait := r.wait
	go func() {
		wait <- command.Wait()
		r.clearStopped(command)
	}()
	endpoint := r.endpoint
	r.resetIdleTimerLocked()
	r.mu.Unlock()
	if os.Getenv("NTERM_AI_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "NTerm AI: starting %s at %s with %s\n", runtimePath, endpoint, modelPath)
	}
	return r.waitUntilReady(ctx, command, wait, endpoint)
}

func (r *localModelRuntime) waitUntilReady(ctx context.Context, command *exec.Cmd, wait chan error, endpoint string) (string, error) {
	deadline := time.NewTimer(6 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Keep the child warming in the background. A newer prediction will
			// reuse it instead of repeatedly paying the cold-start cost.
			return "", ctx.Err()
		case <-deadline.C:
			r.Close()
			return "", errors.New("built-in model startup timed out")
		case <-wait:
			r.clearStopped(command)
			return "", errors.New("built-in model stopped during startup")
		case <-ticker.C:
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/health", nil)
			response, requestErr := http.DefaultClient.Do(request)
			if requestErr == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					r.mu.Lock()
					if r.command == command {
						r.ready = true
					}
					r.mu.Unlock()
					return endpoint, nil
				}
			} else if os.Getenv("NTERM_AI_DEBUG") == "1" {
				fmt.Fprintf(os.Stderr, "NTerm AI: waiting for %s: %v\n", endpoint, requestErr)
			}
		}
	}
}

func (r *localModelRuntime) resetIdleTimerLocked() {
	r.generation++
	generation := r.generation
	if r.idleTimer != nil {
		r.idleTimer.Stop()
	}
	r.idleTimer = time.AfterFunc(modelIdleTimeout, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.generation == generation {
			r.stopLocked()
		}
	})
}

func (r *localModelRuntime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopLocked()
}

func (r *localModelRuntime) stopLocked() {
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
	command := r.command
	wait := r.wait
	r.command = nil
	r.wait = nil
	r.endpoint = ""
	r.ready = false
	r.generation++
	if command == nil || command.Process == nil {
		return
	}
	_ = command.Process.Signal(os.Interrupt)
	go func() {
		select {
		case <-wait:
		case <-time.After(750 * time.Millisecond):
			_ = command.Process.Kill()
		}
	}()
}

func (r *localModelRuntime) clearStopped(command *exec.Cmd) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.command == command {
		r.command = nil
		r.wait = nil
		r.endpoint = ""
		r.ready = false
	}
}

func recentHistory(history []string, limit int) string {
	start := len(history) - limit
	if start < 0 {
		start = 0
	}
	lines := make([]string, 0, len(history)-start)
	for _, command := range history[start:] {
		command = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(command, "\n", " "), "\r", " "))
		if command != "" && len(command) <= 300 {
			lines = append(lines, "- "+command)
		}
	}
	return strings.Join(lines, "\n")
}

func parsePredictedCommand(raw string) string {
	var result struct {
		Command string `json:"command"`
	}
	if json.Unmarshal([]byte(raw), &result) == nil {
		return strings.TrimSpace(result.Command)
	}
	return strings.Trim(strings.Split(raw, "\n")[0], " `\t\r\n")
}

func validPrediction(prefix, command string) bool {
	if command == prefix || !strings.HasPrefix(command, prefix) || len(command) > 500 {
		return false
	}
	return !strings.ContainsAny(command, "\r\n\x00")
}

func predictionCacheKey(value PredictionContext, model string) string {
	return model + "\x00" + value.CWD + "\x00" + value.Input + "\x00" + recentHistory(value.History, 3)
}

func (p *LocalModelProvider) cached(key string) (Suggestion, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.cache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(p.cache, key)
		return Suggestion{}, false
	}
	return entry.suggestion, true
}

func (p *LocalModelProvider) store(key string, suggestion Suggestion) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.cache) >= 96 {
		for cacheKey, entry := range p.cache {
			if time.Now().After(entry.expiresAt) || len(p.cache) >= 96 {
				delete(p.cache, cacheKey)
			}
		}
	}
	p.cache[key] = predictionCacheEntry{suggestion: suggestion, expiresAt: time.Now().Add(10 * time.Minute)}
}
