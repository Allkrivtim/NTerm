package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alexandr/nterm/internal/config"
	"github.com/alexandr/nterm/internal/terminal"
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
