package config

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	want := Defaults()
	want.Theme = "dark"
	want.FontFamily = "Menlo"
	want.FontSize = 15
	want.DefaultPath = t.TempDir()
	want.ReduceTransparency = true
	if _, err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("settings = %#v, want %#v", got, want)
	}
}

func TestNormalizeRejectsInvalidValues(t *testing.T) {
	settings := Defaults()
	settings.FontSize = 42
	if _, err := Normalize(settings); err == nil {
		t.Fatal("expected invalid font size error")
	}
	settings = Defaults()
	settings.DefaultPath = filepath.Join(t.TempDir(), "missing")
	if _, err := Normalize(settings); err == nil {
		t.Fatal("expected invalid default path error")
	}
	settings = Defaults()
	settings.AIModel = "huge-remote-model"
	if _, err := Normalize(settings); err == nil {
		t.Fatal("expected invalid AI model error")
	}
}

func TestNormalizeMigratesLegacyExternalModel(t *testing.T) {
	settings := Defaults()
	settings.AIModel = "qwen2.5-coder:1.5b"
	got, err := Normalize(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got.AIModel != Defaults().AIModel {
		t.Fatalf("AI model = %q, want bundled model", got.AIModel)
	}
}
