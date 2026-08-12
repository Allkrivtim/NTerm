package terminal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alexandr/nterm/internal/domain"
	"golang.org/x/crypto/ssh"
)

func TestUnknownHostKeyRequiresConfirmationAndPersistsTrust(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	address := &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 22}
	confirmed := false
	callback, err := nativeHostKeyCallback(func(info SSHHostKey) bool {
		confirmed = true
		return info.Hostname == "example.test:22" && info.Fingerprint == ssh.FingerprintSHA256(key)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("example.test:22", address, key); err != nil {
		t.Fatal(err)
	}
	if !confirmed {
		t.Fatal("unknown host key was trusted without confirmation")
	}
	trustedCallback, err := nativeHostKeyCallback(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := trustedCallback("example.test:22", address, key); err != nil {
		t.Fatalf("persisted host key was not trusted: %v", err)
	}
}

func TestPrivateKeyRejectsUnsafePermissionsBeforeParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(path, []byte("not a key"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readPrivateKey(path, "")
	if runtime.GOOS != "windows" && (err == nil || !strings.Contains(err.Error(), "unsafe permissions")) {
		t.Fatalf("readPrivateKey error = %v", err)
	}
}

func TestParseSSHInvocation(t *testing.T) {
	arguments, destination, ok, err := parseSSHInvocation(`ssh -p 2222 -i "my key" -L 8080:localhost:80 user@example.com`)
	if err != nil || !ok {
		t.Fatalf("parseSSHInvocation() = %v, %v", ok, err)
	}
	if destination != "user@example.com" {
		t.Fatalf("destination = %q", destination)
	}
	joined := strings.Join(arguments, "|")
	if joined != "-p|2222|-i|my key|-L|8080:localhost:80|user@example.com" {
		t.Fatalf("arguments = %q", joined)
	}
}

func TestResolveSSHProfileForBuiltInClient(t *testing.T) {
	profile, err := resolveSSHProfile([]string{"-p", "2222", "-l", "deploy", "example.com"}, "example.com", SSHProfile{HelperEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Host != "example.com" || profile.User != "deploy" || profile.Port != 2222 || !profile.HelperEnabled {
		t.Fatalf("resolved profile = %#v", profile)
	}
}

func TestResolveSSHProfileRejectsUnsupportedSystemOptions(t *testing.T) {
	_, err := resolveSSHProfile([]string{"-L", "8080:localhost:80", "example.com"}, "example.com", SSHProfile{})
	if err == nil || !strings.Contains(err.Error(), "built-in client") {
		t.Fatalf("expected a built-in SSH option error, got %v", err)
	}
}

func TestResolveSSHProfileRejectsEmbeddedPort(t *testing.T) {
	_, err := resolveSSHProfile([]string{"example.com:2222"}, "example.com:2222", SSHProfile{})
	if err == nil || !strings.Contains(err.Error(), "use -p") {
		t.Fatalf("expected embedded port error, got %v", err)
	}
}

func TestNormalizeServerPlatform(t *testing.T) {
	cases := map[string]string{
		"ubuntu": "ubuntu", "\"debian\"": "debian", "ArchLinux": "arch",
		"almalinux": "rocky", "Darwin": "macos", "unknown-os": "server",
	}
	for input, want := range cases {
		if got := normalizeServerPlatform(input); got != want {
			t.Fatalf("normalizeServerPlatform(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseSSHInvocationRejectsRemoteCommand(t *testing.T) {
	_, _, ok, err := parseSSHInvocation("ssh example.com uptime")
	if !ok || err == nil {
		t.Fatalf("expected managed SSH parse error, got ok=%v err=%v", ok, err)
	}
}

func TestRemoteOutputFilterStreamsAndRemovesMetadata(t *testing.T) {
	var output strings.Builder
	filter := &remoteOutputFilter{
		blockID: "block-1",
		status:  -1,
		emit: func(chunk domain.OutputChunk) {
			output.WriteString(chunk.Data)
		},
	}
	for _, part := range []string{"hello\n\x1eNTER", "M_META\x1f7\x1f/tmp/project\x1e\n"} {
		_, _ = filter.Write([]byte(part))
	}
	filter.Flush()
	if output.String() != "hello\n" {
		t.Fatalf("output = %q", output.String())
	}
	if filter.status != 7 || filter.cwd != "/tmp/project" {
		t.Fatalf("metadata = status %d, cwd %q", filter.status, filter.cwd)
	}
}

func TestSSHStatusReflectsConnectionAndLatency(t *testing.T) {
	remote := &sshSession{
		connected: true, hostname: "build-box", destination: "deploy@example.com",
		latency: 42 * time.Millisecond,
	}
	var got SSHStatus
	remote.setStatusHandler(func(status SSHStatus) { got = status })
	if got.State != "connected" || got.Label != "build-box" || got.LatencyMS != 42 {
		t.Fatalf("status = %#v", got)
	}
	remote.mu.Lock()
	remote.connected = false
	remote.reconnecting = true
	remote.mu.Unlock()
	remote.notifyStatus("Reconnecting…", 2)
	if got.State != "reconnecting" || got.Attempt != 2 {
		t.Fatalf("reconnecting status = %#v", got)
	}
}

func TestSSHTransportErrorsDoNotConfuseRemoteExit(t *testing.T) {
	if !isSSHTransportError(io.EOF) || !isSSHTransportError(errors.New("broken pipe")) {
		t.Fatal("transport failures must trigger reconnect")
	}
	if isSSHTransportError(context.Canceled) || isSSHTransportError(&ssh.ExitError{}) {
		t.Fatal("cancellation and remote exit must not trigger reconnect")
	}
}
