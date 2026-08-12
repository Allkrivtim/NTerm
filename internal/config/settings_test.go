package config

import (
	"os"
	"path/filepath"
	"reflect"
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
	want.NightGreetings = []string{"Quiet night.", "Keep going."}
	if _, err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
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
	settings = Defaults()
	settings.MorningGreetings = []string{"first line\nsecond line"}
	if _, err := Normalize(settings); err == nil {
		t.Fatal("expected multiline greeting error")
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

func TestDocumentRoundTrip(t *testing.T) {
	localProjectPath := t.TempDir()
	store := NewStoreAt(filepath.Join(t.TempDir(), "config.yml"))
	want := DefaultsDocument()
	want.Settings.Theme = "dark"
	want.Servers = []SSHServer{{
		ID: "staging", Name: "Staging", Group: "Work", Host: "192.0.2.10", Port: 2222, User: "deploy", PasswordStored: true,
	}}
	want.Projects = []Project{
		{ID: "local-api", Name: "Local API", Group: "Work", Path: localProjectPath, StartupCommand: "make dev"},
		{ID: "remote-api", Name: "Remote API", Group: "Work", Path: "/srv/api", ServerID: "staging", StartupCommand: "docker compose up"},
	}
	if _, err := store.SaveDocument(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadDocument()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Projects) != 2 || len(got.Servers) != 1 {
		t.Fatalf("catalog = %#v", got)
	}
	if got.Projects[1].ServerID != "staging" || got.Servers[0].Port != 2222 || !got.Servers[0].PasswordStored {
		t.Fatalf("document did not survive YAML round trip: %#v", got)
	}
	if got.Projects[0].Icon != "code" || got.Servers[0].Icon != "server" {
		t.Fatalf("default resource icons were not applied: %#v", got)
	}
}

func TestNormalizeProjectAcceptsCustomIconPath(t *testing.T) {
	iconPath := filepath.Join(t.TempDir(), "project.svg")
	if err := os.WriteFile(iconPath, []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"), 0o600); err != nil {
		t.Fatal(err)
	}
	project, err := NormalizeProject(Project{ID: "web", Name: "Web", Path: t.TempDir(), Icon: "web", IconPath: iconPath}, map[string]struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if project.Icon != "web" || project.IconPath != iconPath {
		t.Fatalf("normalized project = %#v", project)
	}
}

func TestLoadDocumentRejectsUnknownYAMLFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("version: 1\nsettings:\n  them: dark\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStoreAt(path).LoadDocument(); err == nil {
		t.Fatal("expected an unknown YAML field to be rejected")
	}
}

func TestLoadDocumentKeepsDefaultsForMissingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := NewStoreAt(path).LoadDocument()
	if err != nil {
		t.Fatal(err)
	}
	if !document.Settings.OpenHomeOnLaunch || !document.Settings.RunProjectCommands || !document.Settings.SSHHelperEnabled {
		t.Fatalf("missing settings lost their defaults: %#v", document.Settings)
	}
	if len(document.Settings.MorningGreetings) != 3 || len(document.Settings.NightGreetings) != 3 {
		t.Fatalf("missing greeting lists lost their defaults: %#v", document.Settings)
	}
}

func TestLoadDocumentRejectsFutureAndMultipleDocuments(t *testing.T) {
	for name, contents := range map[string]string{
		"future":   "version: 999\n",
		"multiple": "version: 1\n---\nversion: 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewStoreAt(path).LoadDocument(); err == nil {
				t.Fatalf("expected %s config to be rejected", name)
			}
		})
	}
}

func TestLoadDocumentRejectsOversizedFileWithoutParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxConfigSize + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStoreAt(path).LoadDocument(); err == nil {
		t.Fatal("expected oversized config to be rejected")
	}
}

func TestNormalizeRejectsHostWithEmbeddedPortAndSpecialFiles(t *testing.T) {
	_, err := NormalizeServer(SSHServer{ID: "bad", Name: "Bad", Host: "example.com:2222", User: "me"})
	if err == nil {
		t.Fatal("host with an embedded port was accepted")
	}
	project := Project{ID: "icon", Name: "Icon", Path: t.TempDir(), IconPath: t.TempDir()}
	if _, err := NormalizeProject(project, map[string]struct{}{}); err == nil {
		t.Fatal("directory was accepted as a custom icon")
	}
}

func TestNormalizeServerAcceptsBracketedIPv6WithoutDoubleBrackets(t *testing.T) {
	server, err := NormalizeServer(SSHServer{ID: "ipv6", Name: "IPv6", Host: "[2001:db8::1]", User: "me"})
	if err != nil {
		t.Fatal(err)
	}
	if server.Host != "2001:db8::1" {
		t.Fatalf("normalized host = %q", server.Host)
	}
}
