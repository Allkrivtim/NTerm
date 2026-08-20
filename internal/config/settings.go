package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const CurrentVersion = 1

const maxConfigSize = 4 << 20

type Settings struct {
	Theme                   string   `json:"theme" yaml:"theme"`
	FontFamily              string   `json:"fontFamily" yaml:"fontFamily"`
	FontSize                int      `json:"fontSize" yaml:"fontSize"`
	TerminalLineHeight      float64  `json:"terminalLineHeight" yaml:"terminalLineHeight"`
	BlockDensity            string   `json:"blockDensity" yaml:"blockDensity"`
	CursorStyle             string   `json:"cursorStyle" yaml:"cursorStyle"`
	CursorBlink             bool     `json:"cursorBlink" yaml:"cursorBlink"`
	ShellSyntaxHighlighting bool     `json:"shellSyntaxHighlighting" yaml:"shellSyntaxHighlighting"`
	DefaultPath             string   `json:"defaultPath" yaml:"defaultPath"`
	Shell                   string   `json:"shell" yaml:"shell,omitempty"`
	ReduceTransparency      bool     `json:"reduceTransparency" yaml:"reduceTransparency"`
	ShowBlockMetadata       bool     `json:"showBlockMetadata" yaml:"showBlockMetadata"`
	ShowBlockTimestamps     bool     `json:"showBlockTimestamps" yaml:"showBlockTimestamps"`
	AIEnabled               bool     `json:"aiEnabled" yaml:"aiEnabled"`
	AIModel                 string   `json:"aiModel" yaml:"aiModel"`
	OpenHomeOnLaunch        bool     `json:"openHomeOnLaunch" yaml:"openHomeOnLaunch"`
	RunProjectCommands      bool     `json:"runProjectCommands" yaml:"runProjectCommands"`
	SSHHelperEnabled        bool     `json:"sshHelperEnabled" yaml:"sshHelperEnabled"`
	MorningGreetings        []string `json:"morningGreetings" yaml:"morningGreetings"`
	DayGreetings            []string `json:"dayGreetings" yaml:"dayGreetings"`
	EveningGreetings        []string `json:"eveningGreetings" yaml:"eveningGreetings"`
	NightGreetings          []string `json:"nightGreetings" yaml:"nightGreetings"`
}

type Project struct {
	ID             string `json:"id" yaml:"id"`
	Name           string `json:"name" yaml:"name"`
	Group          string `json:"group" yaml:"group,omitempty"`
	Path           string `json:"path" yaml:"path"`
	ServerID       string `json:"serverId" yaml:"serverId,omitempty"`
	StartupCommand string `json:"startupCommand" yaml:"startupCommand,omitempty"`
	Icon           string `json:"icon" yaml:"icon,omitempty"`
	IconPath       string `json:"iconPath" yaml:"iconPath,omitempty"`
	IconData       string `json:"iconData,omitempty" yaml:"-"`
}

type SSHServer struct {
	ID             string `json:"id" yaml:"id"`
	Name           string `json:"name" yaml:"name"`
	Group          string `json:"group" yaml:"group,omitempty"`
	Host           string `json:"host" yaml:"host"`
	Port           int    `json:"port" yaml:"port,omitempty"`
	User           string `json:"user" yaml:"user"`
	KeyPath        string `json:"keyPath" yaml:"keyPath,omitempty"`
	LoginCommand   string `json:"loginCommand" yaml:"loginCommand,omitempty"`
	PasswordStored bool   `json:"passwordStored" yaml:"passwordStored,omitempty"`
	Icon           string `json:"icon" yaml:"icon,omitempty"`
}

type Catalog struct {
	Projects []Project   `json:"projects" yaml:"projects"`
	Servers  []SSHServer `json:"servers" yaml:"servers"`
}

type Document struct {
	Version  int         `json:"version" yaml:"version"`
	Settings Settings    `json:"settings" yaml:"settings"`
	Projects []Project   `json:"projects" yaml:"projects,omitempty"`
	Servers  []SSHServer `json:"servers" yaml:"servers,omitempty"`
}

var allowedFonts = map[string]struct{}{
	"SF Mono": {}, "Menlo": {}, "Monaco": {}, "Cascadia Code": {}, "JetBrains Mono": {},
}

var allowedAIModels = map[string]struct{}{
	"qwen2.5-coder:0.5b": {},
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

var allowedProjectIcons = map[string]struct{}{
	"code": {}, "go": {}, "python": {}, "docker": {}, "node": {}, "rust": {},
	"web": {}, "database": {}, "mobile": {}, "bot": {},
}

var allowedServerIcons = map[string]struct{}{
	"server": {}, "ubuntu": {}, "debian": {}, "fedora": {}, "arch": {},
	"alpine": {}, "centos": {}, "rocky": {}, "opensuse": {}, "freebsd": {},
	"macos": {}, "windows": {},
}

func Defaults() Settings {
	return Settings{
		Theme:                   "system",
		FontFamily:              "SF Mono",
		FontSize:                13,
		TerminalLineHeight:      1.3,
		BlockDensity:            "comfortable",
		CursorStyle:             "block",
		CursorBlink:             true,
		ShellSyntaxHighlighting: true,
		DefaultPath:             "~",
		ShowBlockMetadata:       true,
		AIEnabled:               true,
		AIModel:                 "qwen2.5-coder:0.5b",
		OpenHomeOnLaunch:        true,
		RunProjectCommands:      true,
		SSHHelperEnabled:        true,
		MorningGreetings:        []string{"Morning. Ready when you are.", "A fresh start for good work.", "Good morning — let's build something."},
		DayGreetings:            []string{"Good afternoon. Keep the momentum.", "Back to the craft.", "Your workspace is ready."},
		EveningGreetings:        []string{"Good evening. One more thoughtful step.", "A quiet evening for focused work.", "Welcome back. Let's finish strong."},
		NightGreetings:          []string{"Still creating? Your workspace is ready.", "A calm night for deep focus.", "Late hours, clear thoughts."},
	}
}

func DefaultsDocument() Document {
	return Document{Version: CurrentVersion, Settings: Defaults(), Projects: []Project{}, Servers: []SSHServer{}}
}

type Store struct {
	path       string
	legacyPath string
	mu         sync.Mutex
}

func NewStore() (*Store, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate config directory: %w", err)
	}
	directory = filepath.Join(directory, "NTerm")
	return &Store{
		path:       filepath.Join(directory, "config.yml"),
		legacyPath: filepath.Join(directory, "settings.json"),
	}, nil
}

func NewStoreAt(path string) *Store { return &Store{path: path} }

func (s *Store) Path() string { return s.path }

func (s *Store) Load() (Settings, error) {
	document, err := s.LoadDocument()
	return document.Settings, err
}

func (s *Store) LoadDocument() (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadDocumentLocked()
}

func (s *Store) loadDocumentLocked() (Document, error) {
	document := DefaultsDocument()
	data, err := readLimitedFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		if s.legacyPath != "" {
			legacy, legacyErr := readLimitedFile(s.legacyPath)
			if legacyErr == nil {
				if decodeErr := json.Unmarshal(legacy, &document.Settings); decodeErr != nil {
					return Document{}, fmt.Errorf("decode legacy settings: %w", decodeErr)
				}
				document, err = NormalizeDocument(document)
				if err != nil {
					return Document{}, err
				}
				if err := s.writeDocumentLocked(document); err != nil {
					return Document{}, err
				}
				return document, nil
			} else if !errors.Is(legacyErr, os.ErrNotExist) {
				return Document{}, fmt.Errorf("read legacy settings: %w", legacyErr)
			}
		}
		if err := s.writeDocumentLocked(document); err != nil {
			return Document{}, err
		}
		return document, nil
	}
	if err != nil {
		return Document{}, fmt.Errorf("read config: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("decode config.yml: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Document{}, errors.New("decode config.yml: multiple YAML documents are not supported")
		}
		return Document{}, fmt.Errorf("decode config.yml: %w", err)
	}
	return NormalizeDocument(document)
}

func readLimitedFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if info, statErr := file.Stat(); statErr != nil {
		return nil, statErr
	} else if info.Size() > maxConfigSize {
		return nil, fmt.Errorf("configuration file is larger than %d MB", maxConfigSize>>20)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxConfigSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxConfigSize {
		return nil, fmt.Errorf("configuration file is larger than %d MB", maxConfigSize>>20)
	}
	return data, nil
}

func (s *Store) Save(value Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, err := s.loadDocumentLocked()
	if err != nil {
		return Settings{}, err
	}
	document.Settings = value
	document, err = NormalizeDocument(document)
	if err != nil {
		return Settings{}, err
	}
	if err := s.writeDocumentLocked(document); err != nil {
		return Settings{}, err
	}
	return document.Settings, nil
}

func (s *Store) SaveDocument(value Document) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, err := NormalizeDocument(value)
	if err != nil {
		return Document{}, err
	}
	if err := s.writeDocumentLocked(document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func (s *Store) writeDocumentLocked(document Document) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("secure config directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".config-*.yml")
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure config file: %w", err)
	}
	encoder := yaml.NewEncoder(temporary)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("close config encoder: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	if runtime.GOOS != "windows" {
		directory, err := os.Open(filepath.Dir(s.path))
		if err != nil {
			return fmt.Errorf("open config directory: %w", err)
		}
		if err := directory.Sync(); err != nil {
			_ = directory.Close()
			return fmt.Errorf("sync config directory: %w", err)
		}
		if err := directory.Close(); err != nil {
			return fmt.Errorf("close config directory: %w", err)
		}
	}
	return nil
}

func NormalizeDocument(value Document) (Document, error) {
	if value.Version > CurrentVersion {
		return Document{}, fmt.Errorf("config version %d is newer than this NTerm release supports", value.Version)
	}
	settings, err := Normalize(value.Settings)
	if err != nil {
		return Document{}, err
	}
	value.Version = CurrentVersion
	value.Settings = settings
	if value.Projects == nil {
		value.Projects = []Project{}
	}
	if value.Servers == nil {
		value.Servers = []SSHServer{}
	}

	servers := make(map[string]struct{}, len(value.Servers))
	for index := range value.Servers {
		server, err := NormalizeServer(value.Servers[index])
		if err != nil {
			return Document{}, fmt.Errorf("server %d: %w", index+1, err)
		}
		if _, exists := servers[server.ID]; exists {
			return Document{}, fmt.Errorf("duplicate server id %q", server.ID)
		}
		servers[server.ID] = struct{}{}
		value.Servers[index] = server
	}

	projects := make(map[string]struct{}, len(value.Projects))
	for index := range value.Projects {
		project, err := NormalizeProject(value.Projects[index], servers)
		if err != nil {
			return Document{}, fmt.Errorf("project %d: %w", index+1, err)
		}
		if _, exists := projects[project.ID]; exists {
			return Document{}, fmt.Errorf("duplicate project id %q", project.ID)
		}
		projects[project.ID] = struct{}{}
		value.Projects[index] = project
	}
	return value, nil
}

func NormalizeProject(value Project, servers map[string]struct{}) (Project, error) {
	value.ID = strings.TrimSpace(value.ID)
	value.Name = strings.TrimSpace(value.Name)
	value.Group = strings.TrimSpace(value.Group)
	value.Path = strings.TrimSpace(value.Path)
	value.ServerID = strings.TrimSpace(value.ServerID)
	value.StartupCommand = strings.TrimSpace(value.StartupCommand)
	value.Icon = strings.ToLower(strings.TrimSpace(value.Icon))
	value.IconPath = strings.TrimSpace(value.IconPath)
	value.IconData = ""
	if !idPattern.MatchString(value.ID) {
		return Project{}, errors.New("id must contain only letters, digits, dots, dashes, or underscores")
	}
	if value.Name == "" {
		return Project{}, errors.New("name is required")
	}
	if err := validateTextLength("name", value.Name, 128); err != nil {
		return Project{}, err
	}
	if err := validateTextLength("group", value.Group, 128); err != nil {
		return Project{}, err
	}
	if err := validateTextLength("path", value.Path, 4096); err != nil {
		return Project{}, err
	}
	if err := validateTextLength("startup command", value.StartupCommand, 65536); err != nil {
		return Project{}, err
	}
	if strings.ContainsAny(value.Name+value.Group, "\r\n\t\x00") {
		return Project{}, errors.New("name and group must be single-line text")
	}
	if value.Path == "" {
		return Project{}, errors.New("path is required")
	}
	if value.Icon == "" {
		value.Icon = "code"
	}
	if _, ok := allowedProjectIcons[value.Icon]; !ok {
		return Project{}, fmt.Errorf("unsupported project icon %q", value.Icon)
	}
	if value.IconPath != "" {
		path, err := ResolvePath(value.IconPath)
		if err != nil {
			return Project{}, fmt.Errorf("icon: %w", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return Project{}, fmt.Errorf("icon: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
			return Project{}, errors.New("icon must be an image file smaller than 2 MB")
		}
		value.IconPath = path
	}
	if value.ServerID == "" {
		path, err := ResolveDirectory(value.Path)
		if err != nil {
			return Project{}, fmt.Errorf("path: %w", err)
		}
		value.Path = path
	} else if _, ok := servers[value.ServerID]; !ok {
		return Project{}, fmt.Errorf("server %q does not exist", value.ServerID)
	}
	return value, nil
}

func NormalizeServer(value SSHServer) (SSHServer, error) {
	value.ID = strings.TrimSpace(value.ID)
	value.Name = strings.TrimSpace(value.Name)
	value.Group = strings.TrimSpace(value.Group)
	value.Host = strings.Trim(strings.TrimSpace(value.Host), "[]")
	value.User = strings.TrimSpace(value.User)
	value.KeyPath = strings.TrimSpace(value.KeyPath)
	value.LoginCommand = strings.TrimSpace(value.LoginCommand)
	value.Icon = strings.ToLower(strings.TrimSpace(value.Icon))
	if !idPattern.MatchString(value.ID) {
		return SSHServer{}, errors.New("id must contain only letters, digits, dots, dashes, or underscores")
	}
	if value.Name == "" || value.Host == "" || value.User == "" {
		return SSHServer{}, errors.New("name, host, and user are required")
	}
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{{"name", value.Name, 128}, {"group", value.Group, 128}, {"host", value.Host, 253}, {"user", value.User, 128}, {"login command", value.LoginCommand, 65536}} {
		if err := validateTextLength(field.name, field.value, field.limit); err != nil {
			return SSHServer{}, err
		}
	}
	if strings.ContainsAny(value.Name+value.Group, "\r\n\t\x00") {
		return SSHServer{}, errors.New("name and group must be single-line text")
	}
	if strings.ContainsAny(value.Host+value.User, " \t\r\n\x00") {
		return SSHServer{}, errors.New("host and user cannot contain whitespace")
	}
	if strings.ContainsAny(value.Host, "/\\") {
		return SSHServer{}, errors.New("host cannot contain a path")
	}
	if strings.Contains(value.Host, ":") && net.ParseIP(strings.Trim(value.Host, "[]")) == nil {
		return SSHServer{}, errors.New("host must not include a port; use the port field")
	}
	if value.Port == 0 {
		value.Port = 22
	}
	if value.Port < 1 || value.Port > 65535 {
		return SSHServer{}, errors.New("port must be between 1 and 65535")
	}
	if value.Icon == "" {
		value.Icon = "server"
	}
	if _, ok := allowedServerIcons[value.Icon]; !ok {
		value.Icon = "server"
	}
	if value.KeyPath != "" {
		path, err := ResolvePath(value.KeyPath)
		if err != nil {
			return SSHServer{}, fmt.Errorf("key: %w", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return SSHServer{}, fmt.Errorf("key: %w", err)
		}
		if !info.Mode().IsRegular() {
			return SSHServer{}, errors.New("key must be a file")
		}
		value.KeyPath = path
	}
	return value, nil
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
	if value.TerminalLineHeight == 0 {
		value.TerminalLineHeight = defaults.TerminalLineHeight
	}
	if math.IsNaN(value.TerminalLineHeight) || math.IsInf(value.TerminalLineHeight, 0) ||
		value.TerminalLineHeight < 1.2 || value.TerminalLineHeight > 2 {
		return Settings{}, errors.New("terminal line height must be between 1.2 and 2.0")
	}
	value.BlockDensity = strings.ToLower(strings.TrimSpace(value.BlockDensity))
	if value.BlockDensity == "" {
		value.BlockDensity = defaults.BlockDensity
	}
	switch value.BlockDensity {
	case "compact", "comfortable", "spacious":
	default:
		return Settings{}, errors.New("block density must be compact, comfortable, or spacious")
	}
	value.CursorStyle = strings.ToLower(strings.TrimSpace(value.CursorStyle))
	if value.CursorStyle == "" {
		value.CursorStyle = defaults.CursorStyle
	}
	switch value.CursorStyle {
	case "block", "bar", "underline":
	default:
		return Settings{}, errors.New("cursor style must be block, bar, or underline")
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
		if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			return Settings{}, errors.New("shell must be an executable file")
		}
	}
	value.AIModel = strings.TrimSpace(value.AIModel)
	if value.AIModel == "" {
		value.AIModel = defaults.AIModel
	}
	if value.AIModel == "qwen2.5-coder:1.5b" {
		value.AIModel = defaults.AIModel
	}
	if _, ok := allowedAIModels[value.AIModel]; !ok {
		return Settings{}, errors.New("unsupported local AI model")
	}
	for _, group := range []struct {
		name   string
		values []string
	}{
		{"morning greetings", value.MorningGreetings},
		{"day greetings", value.DayGreetings},
		{"evening greetings", value.EveningGreetings},
		{"night greetings", value.NightGreetings},
	} {
		if err := validateGreetingList(group.name, group.values); err != nil {
			return Settings{}, err
		}
	}
	value.MorningGreetings = normalizeGreetings(value.MorningGreetings, defaults.MorningGreetings)
	value.DayGreetings = normalizeGreetings(value.DayGreetings, defaults.DayGreetings)
	value.EveningGreetings = normalizeGreetings(value.EveningGreetings, defaults.EveningGreetings)
	value.NightGreetings = normalizeGreetings(value.NightGreetings, defaults.NightGreetings)
	return value, nil
}

func validateGreetingList(name string, values []string) error {
	count := 0
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		count++
		if count > 24 {
			return fmt.Errorf("%s may contain at most 24 entries", name)
		}
		if utf8.RuneCountInString(value) > 240 {
			return fmt.Errorf("%s entries must be at most 240 characters", name)
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("%s entries must be single-line text", name)
		}
	}
	return nil
}

func normalizeGreetings(values, fallback []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if utf8.RuneCountInString(value) > 240 || len(result) >= 24 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return append([]string(nil), fallback...)
	}
	return result
}

func validateTextLength(name, value string, limit int) error {
	if utf8.RuneCountInString(value) > limit {
		return fmt.Errorf("%s must be at most %d characters", name, limit)
	}
	return nil
}

func ResolveDirectory(value string) (string, error) {
	path, err := ResolvePath(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("not a directory")
	}
	return path, nil
}

func ResolvePath(value string) (string, error) {
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
	return filepath.Clean(absolute), nil
}
