package terminal

import (
	"strings"
	"testing"
)

func TestShellCommandFilterHandlesFragmentedProtocolFrames(t *testing.T) {
	start := []byte("\x1eNTERM_TEST_START\x1f")
	end := []byte("\x1eNTERM_TEST_END\x1f")
	stop := []byte("\x1eNTERM_TEST_STOP\x1f")
	var output strings.Builder
	started := 0
	results := make(chan shellCommandResult, 1)
	filter := &shellCommandFilter{
		startToken: start,
		endPrefix:  end,
		endSuffix:  stop,
		emit:       func(value []byte) { output.Write(value) },
		started: func() error {
			started++
			return nil
		},
		finish: func(result shellCommandResult) { results <- result },
	}

	for _, fragment := range [][]byte{
		[]byte("ignored shell prompt"),
		start[:5],
		append(append([]byte(nil), start[5:]...), []byte("hel")...),
		append([]byte("lo"), end[:7]...),
		append(append([]byte(nil), end[7:]...), []byte("7\x1f/tmp/work\x1fPython · .venv")...),
		stop[:4],
		stop[4:],
	} {
		filter.Write(fragment)
	}

	result := <-results
	if started != 1 {
		t.Fatalf("started callback count = %d, want 1", started)
	}
	if output.String() != "hello" {
		t.Fatalf("output = %q, want %q", output.String(), "hello")
	}
	if result.Status != 7 || result.CWD != "/tmp/work" || result.Environment != "Python · .venv" || result.Err != nil {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellCommandFilterRejectsMalformedMetadata(t *testing.T) {
	start := []byte("<start>")
	end := []byte("<end>")
	stop := []byte("<stop>")
	results := make(chan shellCommandResult, 1)
	filter := &shellCommandFilter{
		startToken: start,
		endPrefix:  end,
		endSuffix:  stop,
		emit:       func([]byte) {},
		finish:     func(result shellCommandResult) { results <- result },
	}

	filter.Write([]byte("<start><end>missing-separator<stop>"))
	result := <-results
	if result.Err == nil || !strings.Contains(result.Err.Error(), "malformed") {
		t.Fatalf("error = %v, want malformed metadata error", result.Err)
	}
}
