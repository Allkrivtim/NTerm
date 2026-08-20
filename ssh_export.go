package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexandr/nterm/internal/config"
	"github.com/alexandr/nterm/internal/sshconfig"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type SSHExportResult struct {
	Path  string `json:"path"`
	Hosts int    `json:"hosts"`
}

var saveSSHConfigFile = runtime.SaveFileDialog
var writeSSHConfigFile = sshconfig.WriteFile

// ExportSSHConfig writes only public connection metadata selected by the user.
// The native save dialog owns overwrite confirmation; NTerm never silently
// appends to or replaces ~/.ssh/config.
func (a *App) ExportSSHConfig() (SSHExportResult, error) {
	a.mu.Lock()
	servers := append([]config.SSHServer(nil), a.document.Servers...)
	a.mu.Unlock()
	data, err := sshconfig.Render(servers)
	if err != nil {
		return SSHExportResult{}, err
	}
	a.ctxMu.RLock()
	ctx := a.ctx
	a.ctxMu.RUnlock()
	if ctx == nil || ctx.Err() != nil {
		return SSHExportResult{}, errors.New("application window is not ready")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return SSHExportResult{}, fmt.Errorf("locate home directory: %w", err)
	}
	directory := filepath.Join(home, ".ssh")
	if info, statErr := os.Stat(directory); statErr != nil || !info.IsDir() {
		directory = home
	}
	path, err := saveSSHConfigFile(ctx, runtime.SaveDialogOptions{
		Title: "Export OpenSSH configuration", DefaultDirectory: directory,
		DefaultFilename: "nterm-ssh-config", ShowHiddenFiles: true, CanCreateDirectories: true,
	})
	if err != nil {
		return SSHExportResult{}, fmt.Errorf("choose SSH config export path: %w", err)
	}
	if path == "" {
		return SSHExportResult{}, nil
	}
	if err := writeSSHConfigFile(path, data); err != nil {
		return SSHExportResult{}, err
	}
	return SSHExportResult{Path: path, Hosts: len(servers)}, nil
}
