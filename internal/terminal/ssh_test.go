package terminal

import (
	"strings"
	"testing"

	"github.com/alexandr/nterm/internal/domain"
)

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
