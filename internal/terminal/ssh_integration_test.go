package terminal

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestNativeSSHWithConfiguredServer is opt-in because it needs a reachable
// server. It exercises the same in-process x/crypto/ssh path used by Home.
func TestNativeSSHWithConfiguredServer(t *testing.T) {
	host := os.Getenv("NTERM_TEST_SSH_HOST")
	user := os.Getenv("NTERM_TEST_SSH_USER")
	keyPath := os.Getenv("NTERM_TEST_SSH_KEY")
	if host == "" || user == "" || keyPath == "" {
		t.Skip("native SSH integration environment is not configured")
	}
	port := 22
	if value := os.Getenv("NTERM_TEST_SSH_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			t.Fatal(err)
		}
		port = parsed
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	remote, err := connectSSH(ctx, nil, "", SSHProfile{Host: host, Port: port, User: user, KeyPath: keyPath})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.close(false)
	if remote.cwd == "" || remote.hostname == "" || remote.platform == "" {
		t.Fatalf("incomplete remote probe: cwd=%q hostname=%q platform=%q", remote.cwd, remote.hostname, remote.platform)
	}
}
