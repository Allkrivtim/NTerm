package sshconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alexandr/nterm/internal/config"
)

func TestRenderOpenSSHConfigWithoutSecretsOrLoginCommands(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "key with spaces")
	if err := os.WriteFile(keyPath, []byte("test key"), 0o600); err != nil {
		t.Fatal(err)
	}
	servers := []config.SSHServer{{
		ID: "prod-api", Name: "Production API", Group: "Production",
		Host: "2001:db8::10", Port: 2222, User: "deploy", KeyPath: keyPath,
		PasswordStored: true, LoginCommand: "tmux attach || tmux",
	}}
	data, err := Render(servers)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, expected := range []string{
		"Host prod-api\n", "    HostName 2001:db8::10\n", "    Port 2222\n",
		"    User deploy\n", `    IdentityFile "` + keyPath + `"`,
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("export does not contain %q:\n%s", expected, got)
		}
	}
	for _, secret := range []string{"Password", "Keychain", "tmux attach", "RemoteCommand"} {
		if strings.Contains(got, secret) {
			t.Fatalf("export unexpectedly contains %q:\n%s", secret, got)
		}
	}
}

func TestWriteFileUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := WriteFile(path, []byte("Host dev\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Host dev\n" {
		t.Fatalf("contents = %q", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("permissions = %o, want 600", got)
		}
	}
}

func TestRenderRejectsEmptyCatalog(t *testing.T) {
	if _, err := Render(nil); err == nil {
		t.Fatal("empty server catalog must not produce a misleading export")
	}
}
