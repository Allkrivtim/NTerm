package domain

import (
	"encoding/base64"
	"time"
)

type BlockState string

const (
	BlockRunning   BlockState = "running"
	BlockSucceeded BlockState = "succeeded"
	BlockFailed    BlockState = "failed"
	BlockCancelled BlockState = "cancelled"
)

type Block struct {
	ID          string     `json:"id"`
	TabID       string     `json:"tabId"`
	Command     string     `json:"command"`
	CWD         string     `json:"cwd"`
	FinalCWD    string     `json:"finalCwd,omitempty"`
	Environment string     `json:"environment,omitempty"`
	Remote      string     `json:"remote,omitempty"`
	State       BlockState `json:"state"`
	ExitCode    *int       `json:"exitCode,omitempty"`
	StartedAt   time.Time  `json:"startedAt"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	Duration    int64      `json:"durationMs,omitempty"`
}

type OutputChunk struct {
	TabID      string `json:"tabId"`
	BlockID    string `json:"blockId"`
	Stream     string `json:"stream"`
	Data       string `json:"data,omitempty"`
	DataBase64 string `json:"dataBase64,omitempty"`
}

// NewOutputChunkBytes preserves PTY output exactly across JSON boundaries.
// An individual PTY read may split a UTF-8 code point, so converting each read
// to a string before JSON encoding can irreversibly corrupt terminal output.
func NewOutputChunkBytes(blockID, stream string, value []byte) OutputChunk {
	return OutputChunk{
		BlockID:    blockID,
		Stream:     stream,
		DataBase64: base64.StdEncoding.EncodeToString(value),
	}
}

func (c OutputChunk) Bytes() []byte {
	if c.DataBase64 != "" {
		value, err := base64.StdEncoding.DecodeString(c.DataBase64)
		if err == nil {
			return value
		}
	}
	return []byte(c.Data)
}

type Tab struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	CWD         string `json:"cwd"`
	Remote      string `json:"remote,omitempty"`
	Environment string `json:"environment,omitempty"`
	Running     bool   `json:"running"`
}

type BlockFinished struct {
	Block Block `json:"block"`
}
