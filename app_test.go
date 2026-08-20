package main

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alexandr/nterm/internal/config"
	"github.com/alexandr/nterm/internal/domain"
	"github.com/alexandr/nterm/internal/terminal"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestAppCreatesAndClosesIndependentTabs(t *testing.T) {
	directory := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.Defaults()
	settings.DefaultPath = directory
	settings.OpenHomeOnLaunch = false
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	initial := app.InitialState()
	if len(initial.Tabs) != 1 || initial.Tabs[0].CWD != directory {
		t.Fatalf("initial tabs = %#v", initial.Tabs)
	}
	if len(initial.Commands) == 0 {
		t.Fatal("initial state must include the cached command inventory")
	}
	second, err := app.NewTab("")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == initial.Tabs[0].ID {
		t.Fatal("tabs must have unique IDs")
	}
	if _, err := app.CloseTab(initial.Tabs[0].ID); err != nil {
		t.Fatal(err)
	}
	result, err := app.CloseTab(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedTab != nil || result.ActiveID != "" || len(app.InitialState().Tabs) != 0 {
		t.Fatalf("closing last terminal tab should return to Home: %#v", result)
	}
}

func TestSaveSettingsAffectsNewTabs(t *testing.T) {
	firstDirectory := t.TempDir()
	secondDirectory := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.Defaults()
	settings.DefaultPath = firstDirectory
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	settings.DefaultPath = secondDirectory
	settings.Theme = "dark"
	settings.FontSize = 16
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	tab, err := app.NewTab("")
	if err != nil {
		t.Fatal(err)
	}
	if tab.CWD != secondDirectory {
		t.Fatalf("new tab cwd = %q, want %q", tab.CWD, secondDirectory)
	}
}

func TestDefaultHomeStartsWithoutTerminalTabs(t *testing.T) {
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.yml"))
	document := config.DefaultsDocument()
	document.Settings.DefaultPath = t.TempDir()
	if _, err := store.SaveDocument(document); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	initial := app.InitialState()
	if !initial.OpenHome || len(initial.Tabs) != 0 || initial.ActiveTabID != "" {
		t.Fatalf("Home must be the initial zero-tab state: %#v", initial)
	}
}

func TestAppRestoresTabsDraftsBlocksAndHistory(t *testing.T) {
	directory := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.yml"))
	document := config.DefaultsDocument()
	document.Settings.DefaultPath = directory
	document.Settings.OpenHomeOnLaunch = false
	if _, err := store.SaveDocument(document); err != nil {
		t.Fatal(err)
	}
	first, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	tabID := first.InitialState().ActiveTabID
	if err := first.SaveDraft(tabID, "go test ./..."); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	exitCode := 0
	block := domain.Block{
		ID: "restored-block", TabID: tabID, Command: "go test ./...", CWD: directory,
		FinalCWD: directory, State: domain.BlockSucceeded, ExitCode: &exitCode,
		StartedAt: now.Add(-time.Second), EndedAt: &now, Duration: 1000,
	}
	if err := first.workspace.SaveBlock(block); err != nil {
		t.Fatal(err)
	}
	if err := first.workspace.AppendOutput(block.ID, "ok\n"); err != nil {
		t.Fatal(err)
	}
	first.shutdown(context.Background())

	second, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.shutdown(context.Background()) })
	initial := second.InitialState()
	if initial.ActiveTabID != tabID || len(initial.Tabs) != 1 {
		t.Fatalf("restored initial state = %#v", initial)
	}
	if len(initial.Workspace.Tabs) != 1 || initial.Workspace.Tabs[0].Draft != "go test ./..." {
		t.Fatalf("restored workspace = %#v", initial.Workspace)
	}
	blocks := initial.Workspace.Tabs[0].Blocks
	if len(blocks) != 1 || blocks[0].Output != "ok\n" || blocks[0].Block.State != domain.BlockSucceeded {
		t.Fatalf("restored blocks = %#v", blocks)
	}
	history := second.History(tabID)
	if len(history) != 1 || history[0] != "go test ./..." {
		t.Fatalf("restored history = %#v", history)
	}
}

func TestExecuteWaitsForRendererReadiness(t *testing.T) {
	directory := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.Defaults()
	settings.DefaultPath = directory
	settings.OpenHomeOnLaunch = false
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	tabID := app.InitialState().ActiveTabID
	block, err := app.Execute(tabID, "printf first-frame")
	if err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	tab := app.tabs[tabID]
	pending := tab.pending
	history := tab.session.History()
	app.mu.Unlock()
	if pending == nil || pending.block.ID != block.ID {
		t.Fatal("block must remain pending until the renderer acknowledges it")
	}
	if len(history) != 0 {
		t.Fatalf("process started before renderer readiness: history = %#v", history)
	}
	if _, err := app.CloseTab(tabID); err != nil {
		t.Fatal(err)
	}
}

func TestCancelPendingBlockReleasesTab(t *testing.T) {
	directory := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.Defaults()
	settings.DefaultPath = directory
	settings.OpenHomeOnLaunch = false
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	tabID := app.InitialState().ActiveTabID
	block, err := app.Execute(tabID, "printf never-started")
	if err != nil {
		t.Fatal(err)
	}
	if !app.Cancel(tabID, block.ID) {
		t.Fatal("pending block was not cancelled")
	}
	if _, err := app.Execute(tabID, "printf next"); err != nil {
		t.Fatalf("tab remained busy after pending cancellation: %v", err)
	}
	if !app.Cancel(tabID, "") {
		t.Fatal("second pending block was not cancelled")
	}
}

func TestCancelRunningBlockInterruptsPTYAndReleasesTab(t *testing.T) {
	directory := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.Defaults()
	settings.DefaultPath = directory
	settings.OpenHomeOnLaunch = false
	if _, err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.shutdown(context.Background()) })

	tabID := app.InitialState().ActiveTabID
	block, err := app.Execute(tabID, "sleep 30")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.StartBlock(tabID, block.ID); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		app.mu.Lock()
		tab := app.tabs[tabID]
		app.mu.Unlock()
		started := tab != nil && len(tab.session.History()) > 0
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command did not reach the persistent shell")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// History is updated just before the persistent shell attaches its PTY.
	// Give the foreground sleep time to take ownership of the terminal so this
	// exercises the same interrupt path as a real keyboard event.
	time.Sleep(150 * time.Millisecond)

	if !app.Cancel(tabID, block.ID) {
		t.Fatal("running block did not accept Ctrl+C cancellation")
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		app.mu.Lock()
		tab := app.tabs[tabID]
		released := tab != nil && tab.cancel == nil && tab.runningID == "" && !tab.info.Running
		app.mu.Unlock()
		if released {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("running block did not release the tab after Ctrl+C")
		}
		time.Sleep(10 * time.Millisecond)
	}

	next, err := app.Execute(tabID, "printf next")
	if err != nil {
		t.Fatalf("tab remained busy after Ctrl+C: %v", err)
	}
	if !app.Cancel(tabID, next.ID) {
		t.Fatal("cleanup of the next pending block failed")
	}
}

func TestNewWindowProcessFlag(t *testing.T) {
	if !isNewWindowProcess([]string{"--unrelated", newWindowFlag}) {
		t.Fatal("new-window flag was not detected")
	}
	if isNewWindowProcess([]string{"--unrelated"}) {
		t.Fatal("unrelated arguments selected new-window mode")
	}
}

func TestNewWindowLaunchesSiblingExecutable(t *testing.T) {
	originalExecutable := currentExecutable
	originalPlatform := currentPlatform
	originalLauncher := launchWindowProcess
	defer func() {
		currentExecutable = originalExecutable
		currentPlatform = originalPlatform
		launchWindowProcess = originalLauncher
	}()

	currentExecutable = func() (string, error) { return "/Applications/NTerm.app/Contents/MacOS/NTerm", nil }
	currentPlatform = "darwin"
	var launchedExecutable string
	var launchedArguments []string
	launchWindowProcess = func(executable string, arguments ...string) error {
		launchedExecutable = executable
		launchedArguments = append([]string(nil), arguments...)
		return nil
	}

	if err := (&App{}).NewWindow(); err != nil {
		t.Fatal(err)
	}
	if launchedExecutable != "/usr/bin/open" {
		t.Fatalf("launched executable = %q", launchedExecutable)
	}
	expectedArguments := []string{"-n", "/Applications/NTerm.app", "--args", newWindowFlag}
	if !reflect.DeepEqual(launchedArguments, expectedArguments) {
		t.Fatalf("launched arguments = %#v", launchedArguments)
	}
}

func TestNewWindowUsesDirectExecutableOutsideAppBundle(t *testing.T) {
	executable, arguments := newWindowProcessCommand("/tmp/nterm-dev", "darwin")
	if executable != "/tmp/nterm-dev" || !reflect.DeepEqual(arguments, []string{newWindowFlag}) {
		t.Fatalf("development launch = %q %#v", executable, arguments)
	}
}

func TestSecondaryWindowStartsFreshWithoutSharedWorkspace(t *testing.T) {
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.yml"))
	document := config.DefaultsDocument()
	document.Settings.DefaultPath = t.TempDir()
	document.Settings.OpenHomeOnLaunch = true
	if _, err := store.SaveDocument(document); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStoreMode(store, false)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(context.Background())
	if app.workspace != nil {
		t.Fatal("secondary window unexpectedly shares the primary workspace database")
	}
	state := app.InitialState()
	if len(state.Tabs) != 1 || state.ActiveTabID == "" {
		t.Fatalf("secondary window state = %#v", state)
	}
	if state.OpenHome {
		t.Fatal("secondary window opened Home instead of its fresh terminal tab")
	}
}

func TestRendererWatchdogReleasesPendingBlock(t *testing.T) {
	directory := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.yml"))
	document := config.DefaultsDocument()
	document.Settings.DefaultPath = directory
	document.Settings.OpenHomeOnLaunch = false
	if _, err := store.SaveDocument(document); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	tabID := app.InitialState().ActiveTabID
	block, err := app.Execute(tabID, "printf never-rendered")
	if err != nil {
		t.Fatal(err)
	}
	app.expirePendingBlock(tabID, block.ID)
	if _, err := app.Execute(tabID, "printf recovered"); err != nil {
		t.Fatalf("watchdog left tab busy: %v", err)
	}
	if !app.Cancel(tabID, "") {
		t.Fatal("cleanup of recovered pending block failed")
	}
}

func TestSaveSettingsPreservesCatalogInMemoryAndOnDisk(t *testing.T) {
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.yml"))
	document := config.DefaultsDocument()
	document.Projects = []config.Project{{ID: "project", Name: "Project", Path: t.TempDir()}}
	if _, err := store.SaveDocument(document); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	settings := document.Settings
	settings.Theme = "dark"
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if got := app.InitialState().Catalog.Projects; len(got) != 1 || got[0].ID != "project" {
		t.Fatalf("in-memory catalog was lost: %#v", got)
	}
	reloaded, err := store.LoadDocument()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Projects) != 1 || reloaded.Settings.Theme != "dark" {
		t.Fatalf("saved document is inconsistent: %#v", reloaded)
	}
}

func TestOpenLocalProjectUsesPathAndStartupCommand(t *testing.T) {
	defaultPath := t.TempDir()
	projectPath := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.yml"))
	document := config.DefaultsDocument()
	document.Settings.DefaultPath = defaultPath
	document.Projects = []config.Project{{
		ID: "telegram-bot", Name: "TelegramBot", Group: "Bots", Path: projectPath, StartupCommand: "go run ./cmd/bot",
	}}
	if _, err := store.SaveDocument(document); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	launch, err := app.OpenProject("telegram-bot")
	if err != nil {
		t.Fatal(err)
	}
	if launch.Tab.CWD != projectPath || launch.Tab.Title != "TelegramBot" {
		t.Fatalf("project tab = %#v", launch.Tab)
	}
	if len(launch.Commands) != 1 || launch.Commands[0] != "go run ./cmd/bot" {
		t.Fatalf("project commands = %#v", launch.Commands)
	}
}

func TestOpenRemoteProjectBuildsManagedSSHSequence(t *testing.T) {
	defaultPath := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.yml"))
	document := config.DefaultsDocument()
	document.Settings.DefaultPath = defaultPath
	document.Servers = []config.SSHServer{{
		ID: "prod", Name: "Production", Host: "example.internal", Port: 2222, User: "deploy",
	}}
	document.Projects = []config.Project{{
		ID: "api", Name: "API", Path: "/srv/api current", ServerID: "prod", StartupCommand: "make attach",
	}}
	if _, err := store.SaveDocument(document); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	previousConnector := connectSessionSSH
	connectSessionSSH = func(_ context.Context, _ *terminal.Session, _ terminal.SSHProfile) error { return nil }
	t.Cleanup(func() { connectSessionSSH = previousConnector })
	launch, err := app.OpenProject("api")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cd '/srv/api current'", "make attach"}
	if len(launch.Commands) != len(want) {
		t.Fatalf("remote project commands = %#v", launch.Commands)
	}
	for index := range want {
		if launch.Commands[index] != want[index] {
			t.Fatalf("command %d = %q, want %q", index, launch.Commands[index], want[index])
		}
	}
}

func TestDeleteHomeResourcesPersistsEmptyCatalog(t *testing.T) {
	projectPath := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.yml"))
	document := config.DefaultsDocument()
	document.Settings.DefaultPath = t.TempDir()
	document.Projects = []config.Project{{ID: "local", Name: "Local", Path: projectPath}}
	document.Servers = []config.SSHServer{{ID: "dev", Name: "Dev", Host: "127.0.0.1", Port: 22, User: "developer"}}
	if _, err := store.SaveDocument(document); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	if catalog, err := app.DeleteProject("local"); err != nil || len(catalog.Projects) != 0 {
		t.Fatalf("delete project: catalog=%#v err=%v", catalog, err)
	}
	if catalog, err := app.DeleteServer("dev"); err != nil || len(catalog.Servers) != 0 {
		t.Fatalf("delete server: catalog=%#v err=%v", catalog, err)
	}
	saved, err := store.LoadDocument()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Projects) != 0 || len(saved.Servers) != 0 {
		t.Fatalf("deleted resources remain in config: %#v", saved)
	}
}

func TestExportSSHConfigUsesNativeDestinationWithoutSecrets(t *testing.T) {
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "config.yml"))
	document := config.DefaultsDocument()
	document.Servers = []config.SSHServer{{
		ID: "dev", Name: "Development", Host: "dev.example.com", Port: 22,
		User: "alex", PasswordStored: true, LoginCommand: "tmux attach",
	}}
	if _, err := store.SaveDocument(document); err != nil {
		t.Fatal(err)
	}
	app, err := newAppWithStore(store)
	if err != nil {
		t.Fatal(err)
	}
	app.startup(context.Background())
	t.Cleanup(func() { app.shutdown(context.Background()) })
	destination := filepath.Join(t.TempDir(), "ssh-config")
	previousDialog := saveSSHConfigFile
	previousWriter := writeSSHConfigFile
	saveSSHConfigFile = func(_ context.Context, options runtime.SaveDialogOptions) (string, error) {
		if options.DefaultFilename != "nterm-ssh-config" {
			t.Fatalf("default filename = %q", options.DefaultFilename)
		}
		return destination, nil
	}
	var exported string
	writeSSHConfigFile = func(path string, data []byte) error {
		if path != destination {
			t.Fatalf("destination = %q", path)
		}
		exported = string(data)
		return nil
	}
	t.Cleanup(func() {
		saveSSHConfigFile = previousDialog
		writeSSHConfigFile = previousWriter
	})
	result, err := app.ExportSSHConfig()
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != destination || result.Hosts != 1 {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(exported, "Host dev\n") || strings.Contains(exported, "tmux attach") {
		t.Fatalf("unexpected export:\n%s", exported)
	}
}
