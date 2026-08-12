package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexandr/nterm/internal/completion"
	"github.com/alexandr/nterm/internal/config"
	"github.com/alexandr/nterm/internal/domain"
	"github.com/alexandr/nterm/internal/gitstatus"
	"github.com/alexandr/nterm/internal/secrets"
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
	pending       *pendingExecution
}

type pendingExecution struct {
	ctx    context.Context
	cancel context.CancelFunc
	block  domain.Block
	timer  *time.Timer
}

const rendererReadyTimeout = 15 * time.Second

type App struct {
	ctx          context.Context
	ctxMu        sync.RWMutex
	store        *config.Store
	settings     config.Settings
	document     config.Document
	tabs         map[string]*tabState
	tabOrder     []string
	sequence     atomic.Uint64
	predictor    *completion.LocalModelProvider
	mu           sync.Mutex
	startupError string
}

type InitialState struct {
	Tabs         []domain.Tab    `json:"tabs"`
	ActiveTabID  string          `json:"activeTabId"`
	Settings     config.Settings `json:"settings"`
	AIEnabled    bool            `json:"aiEnabled"`
	Commands     []string        `json:"commands"`
	Catalog      config.Catalog  `json:"catalog"`
	ConfigPath   string          `json:"configPath"`
	OpenHome     bool            `json:"openHome"`
	StartupError string          `json:"startupError,omitempty"`
}

type LaunchSpec struct {
	Tab      domain.Tab     `json:"tab"`
	Commands []string       `json:"commands"`
	Catalog  config.Catalog `json:"catalog"`
	Warning  string         `json:"warning,omitempty"`
}

type ServerInput struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Group         string `json:"group"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	User          string `json:"user"`
	KeyPath       string `json:"keyPath"`
	LoginCommand  string `json:"loginCommand"`
	Password      string `json:"password"`
	ClearPassword bool   `json:"clearPassword"`
}

type CloseTabResult struct {
	ClosedID   string      `json:"closedId"`
	ActiveID   string      `json:"activeId"`
	CreatedTab *domain.Tab `json:"createdTab,omitempty"`
}

type SSHStatusEvent struct {
	TabID string `json:"tabId"`
	terminal.SSHStatus
}

var connectSessionSSH = func(ctx context.Context, session *terminal.Session, profile terminal.SSHProfile) error {
	return session.ConnectSSH(ctx, profile)
}

func NewApp() (*App, error) {
	store, err := config.NewStore()
	if err != nil {
		return nil, err
	}
	app, err := newAppWithStore(store)
	if err == nil {
		return app, nil
	}
	// A hand-edited or partially-written config must not make the desktop app
	// impossible to open. Keep the invalid file untouched, start at Home with
	// safe in-memory defaults and surface the exact error so it can be fixed and
	// reloaded from Settings.
	document := config.DefaultsDocument()
	return &App{
		store: store, settings: document.Settings, document: document,
		tabs: make(map[string]*tabState), predictor: completion.NewLocalModelProvider(),
		startupError: err.Error(),
	}, nil
}

func newAppWithStore(store *config.Store) (*App, error) {
	document, err := store.LoadDocument()
	if err != nil {
		return nil, err
	}
	for index := range document.Servers {
		document.Servers[index].PasswordStored = secrets.Has(document.Servers[index].ID)
	}
	app := &App{
		store: store, settings: document.Settings, document: document, tabs: make(map[string]*tabState),
		predictor: completion.NewLocalModelProvider(),
	}
	if !document.Settings.OpenHomeOnLaunch {
		app.mu.Lock()
		_, err = app.newTabLocked("")
		app.mu.Unlock()
		if err != nil {
			return nil, err
		}
	}
	return app, nil
}

func (a *App) startup(ctx context.Context) {
	a.ctxMu.Lock()
	a.ctx = ctx
	a.ctxMu.Unlock()
}

func (a *App) domReady(context.Context) { installTrafficLightAlignment() }

func (a *App) shutdown(context.Context) {
	a.ctxMu.Lock()
	a.ctx = nil
	a.ctxMu.Unlock()
	a.mu.Lock()
	tabs := make([]*tabState, 0, len(a.tabs))
	for _, tab := range a.tabs {
		if tab.cancel != nil {
			tab.cancel()
		}
		if tab.predictCancel != nil {
			tab.predictCancel()
		}
		if tab.pending != nil && tab.pending.timer != nil {
			tab.pending.timer.Stop()
		}
		tabs = append(tabs, tab)
	}
	a.mu.Unlock()
	for _, tab := range tabs {
		tab.session.Close()
	}
	a.predictor.Close()
}

func (a *App) emit(name string, data any) {
	a.ctxMu.RLock()
	ctx := a.ctx
	a.ctxMu.RUnlock()
	if ctx != nil && ctx.Err() == nil {
		runtime.EventsEmit(ctx, name, data)
	}
}

func (a *App) confirmSSHHostKey(info terminal.SSHHostKey) bool {
	a.ctxMu.RLock()
	ctx := a.ctx
	a.ctxMu.RUnlock()
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	message := fmt.Sprintf("The identity of %s is not known yet.\n\n%s\n%s\n\nOnly continue if you recognise this server.", info.Hostname, info.Algorithm, info.Fingerprint)
	answer, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type: runtime.QuestionDialog, Title: "Trust SSH host?", Message: message,
		Buttons: []string{"Cancel", "Trust and connect"}, DefaultButton: "Cancel", CancelButton: "Cancel",
	})
	return err == nil && answer == "Trust and connect"
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
		Catalog:    a.catalogLocked(),
		ConfigPath: a.store.Path(), OpenHome: a.settings.OpenHomeOnLaunch,
		StartupError: a.startupError,
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
	session.ConfigureSSHProfile(terminal.SSHProfile{HelperEnabled: a.settings.SSHHelperEnabled, ConfirmHostKey: a.confirmSSHHostKey})
	session.SetSSHStatusHandler(func(status terminal.SSHStatus) {
		a.emit("ssh:status", SSHStatusEvent{TabID: id, SSHStatus: status})
	})
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
	if tab.pending != nil && tab.pending.timer != nil {
		tab.pending.timer.Stop()
	}
	tab.session.Close()
	delete(a.tabs, tabID)
	closedIndex := 0
	for i, id := range a.tabOrder {
		if id == tabID {
			closedIndex = i
			a.tabOrder = append(a.tabOrder[:i], a.tabOrder[i+1:]...)
			break
		}
	}
	result := CloseTabResult{ClosedID: tabID}
	if len(a.tabOrder) > 0 {
		if closedIndex >= len(a.tabOrder) {
			closedIndex = len(a.tabOrder) - 1
		}
		result.ActiveID = a.tabOrder[closedIndex]
	}
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
	tab.session.BeginInput()
	pending := &pendingExecution{ctx: ctx, cancel: cancel, block: block}
	pending.timer = time.AfterFunc(rendererReadyTimeout, func() { a.expirePendingBlock(tabID, block.ID) })
	tab.pending = pending
	a.mu.Unlock()
	return block, nil
}

// StartBlock is the renderer readiness barrier. Execute only reserves the
// block; the process starts after the frontend has created its block record,
// initialized the VT parser and applied the first terminal size. This prevents
// fast commands and initial alternate-screen frames from being lost.
func (a *App) StartBlock(tabID, blockID string) error {
	a.mu.Lock()
	tab := a.tabs[tabID]
	if tab == nil {
		a.mu.Unlock()
		return errors.New("tab not found")
	}
	pending := tab.pending
	if pending == nil || pending.block.ID != blockID || tab.runningID != blockID {
		a.mu.Unlock()
		return errors.New("block is not waiting to start")
	}
	if pending.timer != nil {
		pending.timer.Stop()
	}
	tab.pending = nil
	a.mu.Unlock()
	go a.run(pending.ctx, pending.cancel, tabID, tab, pending.block)
	return nil
}

func (a *App) expirePendingBlock(tabID, blockID string) {
	a.mu.Lock()
	tab := a.tabs[tabID]
	if tab == nil || tab.pending == nil || tab.pending.block.ID != blockID {
		a.mu.Unlock()
		return
	}
	pending := tab.pending
	if pending.timer != nil {
		pending.timer.Stop()
	}
	pending.cancel()
	tab.pending = nil
	tab.cancel = nil
	tab.runningID = ""
	tab.info.Running = false
	a.mu.Unlock()
	tab.session.EndInput()
	a.emit("block:output", domain.OutputChunk{
		TabID: tabID, BlockID: blockID, Stream: "stderr",
		Data: "NTerm could not start the command because the terminal renderer did not become ready.\n",
	})
	terminal.Finish(&pending.block, errors.New("terminal renderer readiness timed out"), false)
	a.emit("block:done", domain.BlockFinished{Block: pending.block})
}

func (a *App) run(ctx context.Context, cancel context.CancelFunc, tabID string, tab *tabState, block domain.Block) {
	err := tab.session.Run(ctx, &block, func(chunk domain.OutputChunk) {
		chunk.TabID = tabID
		a.emit("block:output", chunk)
	})
	tab.session.EndInput()
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
		tab.pending = nil
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
	a.emit("block:done", domain.BlockFinished{Block: block})
}

func (a *App) Cancel(tabID, blockID string) bool {
	a.mu.Lock()
	tab, ok := a.tabs[tabID]
	if !ok || tab.cancel == nil || (blockID != "" && blockID != tab.runningID) {
		a.mu.Unlock()
		return false
	}
	tab.cancel()
	var pending *pendingExecution
	if tab.pending != nil {
		pending = tab.pending
		if pending.timer != nil {
			pending.timer.Stop()
		}
		tab.pending = nil
		tab.cancel = nil
		tab.runningID = ""
		tab.info.Running = false
	}
	a.mu.Unlock()
	if pending != nil {
		tab.session.EndInput()
		terminal.Finish(&pending.block, context.Canceled, true)
		a.emit("block:done", domain.BlockFinished{Block: pending.block})
	}
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

func (a *App) ReconnectSSH(tabID string) error {
	a.mu.Lock()
	tab := a.tabs[tabID]
	a.mu.Unlock()
	if tab == nil {
		return errors.New("tab not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return tab.session.ReconnectSSH(ctx)
}

func (a *App) Suggest(tabID, input string) []completion.Suggestion {
	a.mu.Lock()
	tab := a.tabs[tabID]
	// Any new keystroke makes an in-flight model result stale. Stop it before
	// doing the cheap deterministic completion so AI can never contend with
	// filesystem completion for CPU or reorder the visible suggestions.
	if tab != nil && tab.predictCancel != nil {
		tab.predictCancel()
		tab.predictCancel = nil
	}
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
func (a *App) Predict(tabID, input string, candidates []string) []completion.Suggestion {
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

	suggestion, err := tab.completion.Predict(ctx, input, model, candidates...)
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

func (a *App) OpenProject(projectID string) (LaunchSpec, error) {
	a.mu.Lock()
	project, ok := findProject(a.document.Projects, projectID)
	if !ok {
		a.mu.Unlock()
		return LaunchSpec{}, errors.New("project not found")
	}
	if project.ServerID == "" {
		tab, err := a.newTabLocked(project.Path)
		if err != nil {
			a.mu.Unlock()
			return LaunchSpec{}, err
		}
		a.tabs[tab.ID].info.Title = project.Name
		tab = a.tabs[tab.ID].info
		a.mu.Unlock()
		commands := []string{}
		if a.settings.RunProjectCommands && project.StartupCommand != "" {
			commands = append(commands, project.StartupCommand)
		}
		return LaunchSpec{Tab: tab, Commands: commands, Catalog: a.catalog()}, nil
	}
	server, ok := findServer(a.document.Servers, project.ServerID)
	if !ok {
		a.mu.Unlock()
		return LaunchSpec{}, errors.New("project SSH server not found")
	}
	tab, profile, err := a.newSSHLaunchTabLocked(server, project.Name)
	a.mu.Unlock()
	if err != nil {
		return LaunchSpec{}, err
	}
	tab, err = a.connectSSHLaunch(tab.ID, profile)
	if err != nil {
		return LaunchSpec{}, err
	}
	catalog, warning := a.rememberServerIcon(server.ID, a.remotePlatform(tab.ID))
	commands := []string{"cd " + quoteShell(project.Path)}
	if a.settings.RunProjectCommands && project.StartupCommand != "" {
		commands = append(commands, project.StartupCommand)
	}
	return LaunchSpec{Tab: tab, Commands: commands, Catalog: catalog, Warning: warning}, nil
}

func (a *App) OpenServer(serverID string) (LaunchSpec, error) {
	a.mu.Lock()
	server, ok := findServer(a.document.Servers, serverID)
	if !ok {
		a.mu.Unlock()
		return LaunchSpec{}, errors.New("SSH server not found")
	}
	tab, profile, err := a.newSSHLaunchTabLocked(server, server.Name)
	a.mu.Unlock()
	if err != nil {
		return LaunchSpec{}, err
	}
	tab, err = a.connectSSHLaunch(tab.ID, profile)
	if err != nil {
		return LaunchSpec{}, err
	}
	commands := []string{}
	if server.LoginCommand != "" {
		commands = append(commands, server.LoginCommand)
	}
	catalog, warning := a.rememberServerIcon(server.ID, a.remotePlatform(tab.ID))
	return LaunchSpec{Tab: tab, Commands: commands, Catalog: catalog, Warning: warning}, nil
}

func (a *App) newSSHLaunchTabLocked(server config.SSHServer, title string) (domain.Tab, terminal.SSHProfile, error) {
	tab, err := a.newTabLocked("")
	if err != nil {
		return domain.Tab{}, terminal.SSHProfile{}, err
	}
	state := a.tabs[tab.ID]
	state.info.Title = title
	credential := ""
	if server.PasswordStored {
		credential = server.ID
	}
	profile := terminal.SSHProfile{
		CredentialAccount: credential,
		HelperEnabled:     a.settings.SSHHelperEnabled,
		Host:              server.Host,
		Port:              server.Port,
		User:              server.User,
		KeyPath:           server.KeyPath,
		ConfirmHostKey:    a.confirmSSHHostKey,
	}
	state.session.ConfigureSSHProfile(profile)
	return state.info, profile, nil
}

func (a *App) connectSSHLaunch(tabID string, profile terminal.SSHProfile) (domain.Tab, error) {
	a.mu.Lock()
	tab := a.tabs[tabID]
	a.mu.Unlock()
	if tab == nil {
		return domain.Tab{}, errors.New("tab not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	err := connectSessionSSH(ctx, tab.session, profile)
	cancel()
	if err != nil {
		a.mu.Lock()
		delete(a.tabs, tabID)
		for index, id := range a.tabOrder {
			if id == tabID {
				a.tabOrder = append(a.tabOrder[:index], a.tabOrder[index+1:]...)
				break
			}
		}
		a.mu.Unlock()
		tab.session.Close()
		return domain.Tab{}, err
	}
	a.mu.Lock()
	tab.info.CWD = tab.session.CWD()
	tab.info.Remote = tab.session.RemoteLabel()
	info := tab.info
	a.mu.Unlock()
	return info, nil
}

func (a *App) SaveProject(project config.Project) (config.Catalog, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if strings.TrimSpace(project.ID) == "" {
		project.ID = a.newEntityID(project.Name)
	}
	document := cloneDocument(a.document)
	found := false
	for index := range document.Projects {
		if document.Projects[index].ID == project.ID {
			document.Projects[index] = project
			found = true
			break
		}
	}
	if !found {
		document.Projects = append(document.Projects, project)
	}
	saved, err := a.store.SaveDocument(document)
	if err != nil {
		return config.Catalog{}, err
	}
	a.document = saved
	return a.catalogLocked(), nil
}

func (a *App) DeleteProject(projectID string) (config.Catalog, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	document := cloneDocument(a.document)
	projects := document.Projects[:0]
	found := false
	for _, project := range document.Projects {
		if project.ID != projectID {
			projects = append(projects, project)
		} else {
			found = true
		}
	}
	if !found {
		return config.Catalog{}, errors.New("project not found")
	}
	document.Projects = projects
	saved, err := a.store.SaveDocument(document)
	if err != nil {
		return config.Catalog{}, err
	}
	a.document = saved
	return a.catalogLocked(), nil
}

func (a *App) SaveServer(input ServerInput) (config.Catalog, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(input.Password) > 64*1024 {
		return config.Catalog{}, errors.New("password or key passphrase is too large")
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = a.newEntityID(input.Name)
	}
	existing, exists := findServer(a.document.Servers, id)
	passwordStored := exists && existing.PasswordStored
	if input.ClearPassword {
		passwordStored = false
	} else if input.Password != "" {
		passwordStored = true
	}
	icon := "server"
	if exists && existing.Host == strings.TrimSpace(input.Host) {
		icon = existing.Icon
	}
	server := config.SSHServer{
		ID: id, Name: input.Name, Group: input.Group, Host: input.Host, Port: input.Port,
		User: input.User, KeyPath: input.KeyPath, LoginCommand: input.LoginCommand, PasswordStored: passwordStored, Icon: icon,
	}
	document := cloneDocument(a.document)
	found := false
	for index := range document.Servers {
		if document.Servers[index].ID == id {
			document.Servers[index] = server
			found = true
			break
		}
	}
	if !found {
		document.Servers = append(document.Servers, server)
	}
	normalized, err := config.NormalizeDocument(document)
	if err != nil {
		return config.Catalog{}, err
	}
	saved, err := a.store.SaveDocument(normalized)
	if err != nil {
		return config.Catalog{}, err
	}
	var secretErr error
	if input.ClearPassword {
		secretErr = secrets.Delete(id)
	} else if input.Password != "" {
		secretErr = secrets.Set(id, input.Password)
	}
	if secretErr != nil {
		_, rollbackErr := a.store.SaveDocument(a.document)
		if rollbackErr != nil {
			return config.Catalog{}, errors.Join(secretErr, fmt.Errorf("rollback config after Keychain error: %w", rollbackErr))
		}
		return config.Catalog{}, secretErr
	}
	a.document = saved
	return a.catalogLocked(), nil
}

func (a *App) DeleteServer(serverID string) (config.Catalog, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	existing, found := findServer(a.document.Servers, serverID)
	if !found {
		return config.Catalog{}, errors.New("SSH server not found")
	}
	for _, project := range a.document.Projects {
		if project.ServerID == serverID {
			return config.Catalog{}, fmt.Errorf("server is used by project %q", project.Name)
		}
	}
	document := cloneDocument(a.document)
	servers := document.Servers[:0]
	for _, server := range document.Servers {
		if server.ID != serverID {
			servers = append(servers, server)
		}
	}
	document.Servers = servers
	saved, err := a.store.SaveDocument(document)
	if err != nil {
		return config.Catalog{}, err
	}
	if existing.PasswordStored {
		if err := secrets.Delete(serverID); err != nil {
			_, rollbackErr := a.store.SaveDocument(a.document)
			if rollbackErr != nil {
				return config.Catalog{}, errors.Join(err, fmt.Errorf("rollback config after Keychain error: %w", rollbackErr))
			}
			return config.Catalog{}, err
		}
	}
	a.document = saved
	return a.catalogLocked(), nil
}

func (a *App) ReloadConfiguration() (InitialState, error) {
	document, err := a.store.LoadDocument()
	if err != nil {
		return InitialState{}, err
	}
	for index := range document.Servers {
		document.Servers[index].PasswordStored = secrets.Has(document.Servers[index].ID)
	}
	a.mu.Lock()
	a.document = document
	a.settings = document.Settings
	a.startupError = ""
	a.mu.Unlock()
	return a.InitialState(), nil
}

func (a *App) ConfigPath() string { return a.store.Path() }

func (a *App) catalogLocked() config.Catalog {
	projects := append([]config.Project(nil), a.document.Projects...)
	for index := range projects {
		projects[index].IconData = loadProjectIcon(projects[index].IconPath)
	}
	return config.Catalog{
		Projects: projects,
		Servers:  append([]config.SSHServer(nil), a.document.Servers...),
	}
}

func (a *App) catalog() config.Catalog {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.catalogLocked()
}

func (a *App) remotePlatform(tabID string) string {
	a.mu.Lock()
	tab := a.tabs[tabID]
	a.mu.Unlock()
	if tab == nil {
		return "server"
	}
	return tab.session.RemotePlatform()
}

func (a *App) rememberServerIcon(serverID, icon string) (config.Catalog, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if icon == "" || icon == "server" {
		return a.catalogLocked(), ""
	}
	document := cloneDocument(a.document)
	for index := range document.Servers {
		if document.Servers[index].ID == serverID && (document.Servers[index].Icon == "" || document.Servers[index].Icon == "server") {
			document.Servers[index].Icon = icon
			if saved, err := a.store.SaveDocument(document); err == nil {
				a.document = saved
			} else {
				return a.catalogLocked(), "Connected, but the detected server icon could not be saved: " + err.Error()
			}
			break
		}
	}
	return a.catalogLocked(), ""
}

func loadProjectIcon(path string) string {
	if path == "" {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 2*1024*1024+1))
	if err != nil || len(data) == 0 || len(data) > 2*1024*1024 {
		return ""
	}
	mimeType := http.DetectContentType(data)
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		mimeType = "image/svg+xml"
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return ""
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func (a *App) SaveSettings(value config.Settings) (config.Settings, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	document := cloneDocument(a.document)
	document.Settings = value
	saved, err := a.store.SaveDocument(document)
	if err != nil {
		return config.Settings{}, err
	}
	settings := saved.Settings
	modelChanged := a.settings.AIModel != settings.AIModel || a.settings.AIEnabled != settings.AIEnabled
	a.settings = settings
	a.document = saved
	if modelChanged {
		for _, tab := range a.tabs {
			if tab.predictCancel != nil {
				tab.predictCancel()
				tab.predictCancel = nil
			}
		}
	}
	return settings, nil
}

func (a *App) newEntityID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var builder strings.Builder
	for _, value := range name {
		switch {
		case value >= 'a' && value <= 'z', value >= '0' && value <= '9':
			builder.WriteRune(value)
		case value == ' ', value == '-', value == '_', value == '.':
			if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "-") {
				builder.WriteByte('-')
			}
		}
	}
	base := strings.Trim(builder.String(), "-")
	if base == "" {
		base = "item"
	}
	return base + "-" + strconv.FormatUint(a.sequence.Add(1), 10)
}

func findProject(projects []config.Project, id string) (config.Project, bool) {
	for _, project := range projects {
		if project.ID == id {
			return project, true
		}
	}
	return config.Project{}, false
}

func cloneDocument(document config.Document) config.Document {
	document.Projects = append([]config.Project(nil), document.Projects...)
	document.Servers = append([]config.SSHServer(nil), document.Servers...)
	return document
}

func findServer(servers []config.SSHServer, id string) (config.SSHServer, bool) {
	for _, server := range servers {
		if server.ID == id {
			return server, true
		}
	}
	return config.SSHServer{}, false
}

func quoteShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
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
