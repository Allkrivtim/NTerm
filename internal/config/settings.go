package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Settings struct {
	Theme              string `json:"theme"`
	FontFamily         string `json:"fontFamily"`
	FontSize           int    `json:"fontSize"`
	DefaultPath        string `json:"defaultPath"`
	Shell              string `json:"shell"`
	ReduceTransparency bool   `json:"reduceTransparency"`
	ShowBlockMetadata  bool   `json:"showBlockMetadata"`
	AIEnabled          bool   `json:"aiEnabled"`
	AIModel            string `json:"aiModel"`
}

var allowedFonts = map[string]struct{}{
	"SF Mono": {}, "Menlo": {}, "Monaco": {}, "Cascadia Code": {}, "JetBrains Mono": {},
}

var allowedAIModels = map[string]struct{}{
	"qwen2.5-coder:0.5b": {},
}

func Defaults() Settings {
	return Settings{
		Theme:             "system",
		FontFamily:        "SF Mono",
		FontSize:          13,
		DefaultPath:       "~",
		ShowBlockMetadata: true,
		AIEnabled:         true,
		AIModel:           "qwen2.5-coder:0.5b",
	}
}

type Store struct{ path string }

func NewStore() (*Store, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate config directory: %w", err)
	}
	return &Store{path: filepath.Join(directory, "NTerm", "settings.json")}, nil
}

func NewStoreAt(path string) *Store { return &Store{path: path} }

func (s *Store) Load() (Settings, error) {
	settings := Defaults()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read settings: %w", err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return Defaults(), fmt.Errorf("decode settings: %w", err)
	}
	return Normalize(settings)
}

func (s *Store) Save(value Settings) (Settings, error) {
	settings, err := Normalize(value)
	if err != nil {
		return Settings{}, err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return Settings{}, fmt.Errorf("create settings directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*")
	if err != nil {
		return Settings{}, fmt.Errorf("create settings file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	_ = temporary.Chmod(0o600)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(settings); err != nil {
		_ = temporary.Close()
		return Settings{}, fmt.Errorf("encode settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return Settings{}, fmt.Errorf("sync settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Settings{}, fmt.Errorf("close settings: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return Settings{}, fmt.Errorf("replace settings: %w", err)
	}
	return settings, nil
}

func Normalize(value Settings) (Settings, error) {
	defaults := Defaults()
	value.Theme = strings.ToLower(strings.TrimSpace(value.Theme))
	switch value.Theme {
	case "system", "light", "dark":
	case "":
		value.Theme = defaults.Theme
	default:
		return Settings{}, errors.New("theme must be system, light, or dark")
	}
	value.FontFamily = strings.TrimSpace(value.FontFamily)
	if value.FontFamily == "" {
		value.FontFamily = defaults.FontFamily
	}
	if _, ok := allowedFonts[value.FontFamily]; !ok {
		return Settings{}, errors.New("unsupported terminal font")
	}
	if value.FontSize == 0 {
		value.FontSize = defaults.FontSize
	}
	if value.FontSize < 10 || value.FontSize > 24 {
		return Settings{}, errors.New("font size must be between 10 and 24")
	}
	value.DefaultPath = strings.TrimSpace(value.DefaultPath)
	if value.DefaultPath == "" {
		value.DefaultPath = defaults.DefaultPath
	}
	if _, err := ResolveDirectory(value.DefaultPath); err != nil {
		return Settings{}, fmt.Errorf("default path: %w", err)
	}
	value.Shell = strings.TrimSpace(value.Shell)
	if value.Shell != "" {
		info, err := os.Stat(value.Shell)
		if err != nil {
			return Settings{}, fmt.Errorf("shell: %w", err)
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			return Settings{}, errors.New("shell must be an executable file")
		}
	}
	value.AIModel = strings.TrimSpace(value.AIModel)
	if value.AIModel == "" {
		value.AIModel = defaults.AIModel
	}
	// Migrate the short-lived external Balanced profile to the single model
	// that is guaranteed to be present in every self-contained build.
	if value.AIModel == "qwen2.5-coder:1.5b" {
		value.AIModel = defaults.AIModel
	}
	if _, ok := allowedAIModels[value.AIModel]; !ok {
		return Settings{}, errors.New("unsupported local AI model")
	}
	return value, nil
}

func ResolveDirectory(value string) (string, error) {
	path := strings.TrimSpace(value)
	home, _ := os.UserHomeDir()
	if path == "" || path == "~" {
		path = home
	} else if strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("not a directory")
	}
	return filepath.Clean(absolute), nil
}
