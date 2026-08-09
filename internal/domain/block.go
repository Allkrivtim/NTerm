package domain

import "time"

type BlockState string

const (
	BlockRunning   BlockState = "running"
	BlockSucceeded BlockState = "succeeded"
	BlockFailed    BlockState = "failed"
	BlockCancelled BlockState = "cancelled"
)

type Block struct {
	ID        string     `json:"id"`
	TabID     string     `json:"tabId"`
	Command   string     `json:"command"`
	CWD       string     `json:"cwd"`
	FinalCWD  string     `json:"finalCwd,omitempty"`
	Remote    string     `json:"remote,omitempty"`
	State     BlockState `json:"state"`
	ExitCode  *int       `json:"exitCode,omitempty"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	Duration  int64      `json:"durationMs,omitempty"`
}

type OutputChunk struct {
	TabID   string `json:"tabId"`
	BlockID string `json:"blockId"`
	Stream  string `json:"stream"`
	Data    string `json:"data"`
}

type Tab struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	CWD     string `json:"cwd"`
	Remote  string `json:"remote,omitempty"`
	Running bool   `json:"running"`
}

type BlockFinished struct {
	Block Block `json:"block"`
}
