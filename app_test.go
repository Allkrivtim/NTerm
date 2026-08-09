package main

import (
	"path/filepath"
	"testing"

	"github.com/alexandr/nterm/internal/config"
)

func TestAppCreatesAndClosesIndependentTabs(t *testing.T) {
	directory := t.TempDir()
	store := config.NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	settings := config.Defaults()
	settings.DefaultPath = directory
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
	if result.CreatedTab == nil || result.ActiveID == "" {
		t.Fatalf("closing last tab should create a replacement: %#v", result)
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
