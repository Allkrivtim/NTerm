package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexandr/nterm/internal/completion"
	"github.com/alexandr/nterm/internal/config"
	"github.com/alexandr/nterm/internal/domain"
	"github.com/alexandr/nterm/internal/gitstatus"
	"github.com/alexandr/nterm/internal/terminal"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type tabState struct {
	info          domain.Tab
	session       *terminal.Session
	completion    *completion.Service
	cancel        context.CancelFunc
	runningID     string
	predictCancel context.CancelFunc
	predictID     uint64
}

type App struct {
	ctx       context.Context
	store     *config.Store
	settings  config.Settings
	tabs      map[string]*tabState
	tabOrder  []string
	sequence  atomic.Uint64
	predictor *completion.LocalModelProvider
	mu        sync.Mutex
}

type InitialState struct {
	Tabs        []domain.Tab    `json:"tabs"`
	ActiveTabID string          `json:"activeTabId"`
	Settings    config.Settings `json:"settings"`
	AIEnabled   bool            `json:"aiEnabled"`
	Commands    []string        `json:"commands"`
}

type CloseTabResult struct {
	ClosedID   string      `json:"closedId"`
	ActiveID   string      `json:"activeId"`
	CreatedTab *domain.Tab `json:"createdTab,omitempty"`
}

func NewApp() (*App, error) {
	store, err := config.NewStore()
	if err != nil {
		return nil, err
	}
	return newAppWithStore(store)
}

func newAppWithStore(store *config.Store) (*App, error) {
	settings, err := store.Load()
	if err != nil {
		return nil, err
	}
	app := &App{
		store: store, settings: settings, tabs: make(map[string]*tabState),
		predictor: completion.NewLocalModelProvider(),
	}
	app.mu.Lock()
	_, err = app.newTabLocked("")
	app.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return app, nil
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) domReady(context.Context) { installTrafficLightAlignment() }

func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, tab := range a.tabs {
		if tab.cancel != nil {
			tab.cancel()
		}
		if tab.predictCancel != nil {
			tab.predictCancel()
		}
		tab.session.Close()
	}
	a.predictor.Close()
}

func (a *App) InitialState() InitialState {
	a.mu.Lock()
	defer a.mu.Unlock()
	tabs := make([]domain.Tab, 0, len(a.tabOrder))
	for _, id := range a.tabOrder {
		tabs = append(tabs, a.tabs[id].info)
	}
	activeID := ""
	var commands []string
	if len(a.tabOrder) > 0 {
		activeID = a.tabOrder[0]
		commands = a.tabs[activeID].completion.Commands()
	}
	return InitialState{
		Tabs: tabs, ActiveTabID: activeID, Settings: a.settings,
		AIEnabled: a.settings.AIEnabled, Commands: commands,
	}
}

func (a *App) NewTab(path string) (domain.Tab, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.newTabLocked(path)
}

func (a *App) newTabLocked(requestedPath string) (domain.Tab, error) {
	if strings.TrimSpace(requestedPath) == "" {
		requestedPath = a.settings.DefaultPath
	}
	cwd, err := config.ResolveDirectory(requestedPath)
	if err != nil {
		return domain.Tab{}, err
	}
	session, err := terminal.NewSessionAt(cwd, a.settings.Shell)
	if err != nil {
		return domain.Tab{}, err
	}
	id := fmt.Sprintf("tab-%d-%d", time.Now().UnixMilli(), a.sequence.Add(1))
	info := domain.Tab{ID: id, Title: tabTitle(cwd), CWD: cwd}
	a.tabs[id] = &tabState{info: info, session: session, completion: completion.NewService(session, a.predictor)}
	a.tabOrder = append(a.tabOrder, id)
	return info, nil
}

func (a *App) CloseTab(tabID string) (CloseTabResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	tab, ok := a.tabs[tabID]
	if !ok {
		return CloseTabResult{}, errors.New("tab not found")
	}
	if tab.cancel != nil {
		tab.cancel()
	}
	if tab.predictCancel != nil {
		tab.predictCancel()
	}
	tab.session.Close()
	delete(a.tabs, tabID)
	for i, id := range a.tabOrder {
		if id == tabID {
			a.tabOrder = append(a.tabOrder[:i], a.tabOrder[i+1:]...)
			break
		}
	}
	result := CloseTabResult{ClosedID: tabID}
	if len(a.tabOrder) == 0 {
		created, err := a.newTabLocked("")
		if err != nil {
			return CloseTabResult{}, err
		}
		result.CreatedTab = &created
	}
	result.ActiveID = a.tabOrder[0]
	return result, nil
}

func (a *App) Execute(tabID, command string) (domain.Block, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return domain.Block{}, errors.New("command is empty")
	}
	a.mu.Lock()
	tab, ok := a.tabs[tabID]
	if !ok {
		a.mu.Unlock()
		return domain.Block{}, errors.New("tab not found")
	}
	if tab.cancel != nil {
		a.mu.Unlock()
		return domain.Block{}, terminal.ErrBusy
	}
	if tab.predictCancel != nil {
		tab.predictCancel()
		tab.predictCancel = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	id := fmt.Sprintf("block-%d-%d", time.Now().UnixMilli(), a.sequence.Add(1))
	block := domain.Block{
		ID: id, TabID: tabID, Command: command, CWD: tab.session.CWD(),
		State: domain.BlockRunning, StartedAt: time.Now(),
	}
	tab.cancel = cancel
	tab.runningID = id
	tab.info.Running = true
	a.mu.Unlock()

	go a.run(ctx, cancel, tabID, tab, block)
	return block, nil
}

func (a *App) run(ctx context.Context, cancel context.CancelFunc, tabID string, tab *tabState, block domain.Block) {
	err := tab.session.Run(ctx, &block, func(chunk domain.OutputChunk) {
		chunk.TabID = tabID
		runtime.EventsEmit(a.ctx, "block:output", chunk)
	})
	block.Remote = tab.session.RemoteLabel()
	cancelled := errors.Is(ctx.Err(), context.Canceled)
	terminal.Finish(&block, err, cancelled)

	a.mu.Lock()
	if current, ok := a.tabs[tabID]; ok && current == tab {
		if tab.runningID == block.ID {
			tab.cancel = nil
			tab.runningID = ""
		}
		tab.info.Running = false
		tab.info.CWD = tab.session.CWD()
		tab.info.Remote = tab.session.RemoteLabel()
		if tab.info.Remote != "" {
			tab.info.Title = tab.info.Remote
		} else {
			tab.info.Title = tabTitle(tab.info.CWD)
		}
	}
	a.mu.Unlock()
	cancel()
	runtime.EventsEmit(a.ctx, "block:done", domain.BlockFinished{Block: block})
}

func (a *App) Cancel(tabID, blockID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	tab, ok := a.tabs[tabID]
	if !ok || tab.cancel == nil || (blockID != "" && blockID != tab.runningID) {
		return false
	}
	tab.cancel()
	return true
}

// TerminalInput is intentionally a narrow raw-byte bridge. It can only write
// to the PTY already owned by a known tab and cannot start another process.
func (a *App) TerminalInput(tabID, value string) error {
	a.mu.Lock()
	tab := a.tabs[tabID]
	a.mu.Unlock()
	if tab == nil {
		return errors.New("tab not found")
	}
	return tab.session.Input(value)
}

func (a *App) ResizeTerminal(tabID string, cols, rows int) error {
	a.mu.Lock()
	tab := a.tabs[tabID]
	a.mu.Unlock()
	if tab == nil {
		return errors.New("tab not found")
	}
	if cols < 2 || rows < 2 || cols > 1000 || rows > 1000 {
		return errors.New("invalid terminal size")
	}
	return tab.session.Resize(uint16(cols), uint16(rows))
}

func (a *App) Suggest(tabID, input string) []completion.Suggestion {
	a.mu.Lock()
	tab := a.tabs[tabID]
	a.mu.Unlock()
	if tab == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 650*time.Millisecond)
	defer cancel()
	return tab.completion.Suggest(ctx, input, 8)
}

func (a *App) CommandNames(tabID string) []string {
	a.mu.Lock()
	tab := a.tabs[tabID]
	a.mu.Unlock()
	if tab == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if commands, remote := tab.session.CommandNames(ctx); remote && len(commands) > 0 {
		return commands
	}
	return tab.completion.ShellCommands(ctx)
}

func (a *App) GitContext(tabID string) gitstatus.Info {
	a.mu.Lock()
	tab := a.tabs[tabID]
	a.mu.Unlock()
	if tab == nil {
		return gitstatus.Info{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancel()
	if info, remote := tab.session.GitContext(ctx); remote {
		return info
	}
	return gitstatus.Inspect(ctx, tab.session.CWD())
}

// Predict runs the optional model separately from Suggest. A new request for
// the same tab cancels the previous inference, so stale keystrokes do not keep
// consuming CPU.
func (a *App) Predict(tabID, input string) []completion.Suggestion {
	a.mu.Lock()
	tab := a.tabs[tabID]
	if tab == nil || !a.settings.AIEnabled {
		a.mu.Unlock()
		return nil
	}
	if tab.predictCancel != nil {
		tab.predictCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2200*time.Millisecond)
	predictID := a.sequence.Add(1)
	tab.predictCancel = cancel
	tab.predictID = predictID
	model := a.settings.AIModel
	a.mu.Unlock()

	suggestion, err := tab.completion.Predict(ctx, input, model)
	cancel()

	a.mu.Lock()
	if current := a.tabs[tabID]; current == tab && tab.predictID == predictID {
		tab.predictCancel = nil
	}
	a.mu.Unlock()
	if err != nil {
		return nil
	}
	return []completion.Suggestion{suggestion}
}

func (a *App) AutocompleteStatus() completion.AIStatus {
	return a.predictor.Status()
}

func (a *App) History(tabID string) []string {
	a.mu.Lock()
	tab := a.tabs[tabID]
	a.mu.Unlock()
	if tab == nil {
		return nil
	}
	return tab.session.History()
}

func (a *App) SaveSettings(value config.Settings) (config.Settings, error) {
	settings, err := a.store.Save(value)
	if err != nil {
		return config.Settings{}, err
	}
	a.mu.Lock()
	modelChanged := a.settings.AIModel != settings.AIModel || a.settings.AIEnabled != settings.AIEnabled
	a.settings = settings
	if modelChanged {
		for _, tab := range a.tabs {
			if tab.predictCancel != nil {
				tab.predictCancel()
				tab.predictCancel = nil
			}
		}
	}
	a.mu.Unlock()
	return settings, nil
}

func tabTitle(cwd string) string {
	home, _ := os.UserHomeDir()
	if filepath.Clean(cwd) == filepath.Clean(home) {
		return "~"
	}
	title := filepath.Base(filepath.Clean(cwd))
	if title == "." || title == string(filepath.Separator) {
		return cwd
	}
	return title
}
