package domain

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestOutputChunkBytesSurviveJSONWithoutUTF8Coercion(t *testing.T) {
	want := []byte{0x1b, '[', '3', '1', 'm', 0xf0, 0x9f, 0x98, 0x80, 0xff}
	chunk := NewOutputChunkBytes("block-1", "stdout", want)
	chunk.TabID = "tab-1"

	encoded, err := json.Marshal(chunk)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"data":`)) {
		t.Fatalf("raw PTY bytes were serialized through the string field: %s", encoded)
	}

	var decoded OutputChunk
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("round trip bytes = %x, want %x", got, want)
	}
}

func TestOutputChunkBytesKeepsTextEventsCompatible(t *testing.T) {
	chunk := OutputChunk{Data: "renderer failed\n"}
	if got := string(chunk.Bytes()); got != chunk.Data {
		t.Fatalf("Bytes() = %q, want %q", got, chunk.Data)
	}
}
