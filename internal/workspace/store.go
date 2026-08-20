package workspace

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/alexandr/nterm/internal/domain"
	_ "github.com/mattn/go-sqlite3"
)

const (
	currentSchemaVersion    = 1
	maxPersistedOutputBytes = 4 << 20
	maxRestoredBlocksPerTab = 500
	outputFlushInterval     = 100 * time.Millisecond
)

type Snapshot struct {
	ActiveTabID string        `json:"activeTabId"`
	Tabs        []TabSnapshot `json:"tabs"`
}

type TabSnapshot struct {
	Tab    domain.Tab    `json:"tab"`
	Draft  string        `json:"draft"`
	Blocks []BlockRecord `json:"blocks"`
}

type BlockRecord struct {
	Block  domain.Block `json:"block"`
	Output string       `json:"output"`
}

// Store keeps non-secret, restorable workspace state. It intentionally lives
// beside config.yml rather than inside it: the YAML document remains suitable
// for hand editing while block output is handled transactionally by SQLite.
type Store struct {
	db        *sql.DB
	mu        sync.Mutex
	flushMu   sync.Mutex
	pending   map[string]string
	timer     *time.Timer
	closed    bool
	outputErr error
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create workspace directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("secure workspace directory: %w", err)
	}
	dsn := (&url.URL{Scheme: "file", Path: path}).String() + "?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open workspace database: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, pending: make(map[string]string)}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("secure workspace database: %w", err)
	}
	return store, nil
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read workspace schema version: %w", err)
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("workspace schema version %d is newer than supported version %d", version, currentSchemaVersion)
	}
	if version == 0 {
		statements := []string{
			`CREATE TABLE workspace_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
			`CREATE TABLE tabs (
				id TEXT PRIMARY KEY,
				title TEXT NOT NULL,
				cwd TEXT NOT NULL,
				position INTEGER NOT NULL,
				draft TEXT NOT NULL DEFAULT '',
				updated_at INTEGER NOT NULL
			)`,
			`CREATE TABLE blocks (
				id TEXT PRIMARY KEY,
				tab_id TEXT NOT NULL REFERENCES tabs(id) ON DELETE CASCADE,
				position INTEGER NOT NULL,
				command TEXT NOT NULL,
				cwd TEXT NOT NULL,
				final_cwd TEXT NOT NULL DEFAULT '',
				remote TEXT NOT NULL DEFAULT '',
				state TEXT NOT NULL,
				exit_code INTEGER,
				started_at INTEGER NOT NULL,
				ended_at INTEGER,
				duration_ms INTEGER NOT NULL DEFAULT 0,
				output BLOB NOT NULL DEFAULT '',
				updated_at INTEGER NOT NULL
			)`,
			`CREATE INDEX blocks_tab_position ON blocks(tab_id, position)`,
			`PRAGMA user_version = 1`,
		}
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin workspace migration: %w", err)
		}
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migrate workspace database: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit workspace migration: %w", err)
		}
	}
	// A process cannot be resurrected safely. Preserve its context and output,
	// but make the interrupted state explicit to the user on the next launch.
	now := time.Now().UnixMilli()
	if _, err := s.db.Exec(`UPDATE blocks
		SET state = ?, ended_at = COALESCE(ended_at, ?),
			duration_ms = CASE WHEN duration_ms > 0 THEN duration_ms ELSE MAX(0, ? - started_at) END,
			updated_at = ?
		WHERE state = ?`, domain.BlockCancelled, now, now, now, domain.BlockRunning); err != nil {
		return fmt.Errorf("recover interrupted blocks: %w", err)
	}
	return nil
}

func (s *Store) Load() (Snapshot, error) {
	if err := s.flushOutput(); err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	err := s.db.QueryRow(`SELECT value FROM workspace_meta WHERE key = 'active_tab_id'`).Scan(&snapshot.ActiveTabID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, fmt.Errorf("load active tab: %w", err)
	}
	rows, err := s.db.Query(`SELECT id, title, cwd, position, draft FROM tabs ORDER BY position, updated_at`)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load workspace tabs: %w", err)
	}
	for rows.Next() {
		var tab TabSnapshot
		var position int
		if err := rows.Scan(&tab.Tab.ID, &tab.Tab.Title, &tab.Tab.CWD, &position, &tab.Draft); err != nil {
			_ = rows.Close()
			return Snapshot{}, fmt.Errorf("scan workspace tab: %w", err)
		}
		snapshot.Tabs = append(snapshot.Tabs, tab)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Snapshot{}, fmt.Errorf("iterate workspace tabs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return Snapshot{}, fmt.Errorf("close workspace tabs: %w", err)
	}
	// Close the tab cursor before loading child rows. Store deliberately uses a
	// single SQLite connection so writes remain ordered and bounded.
	for index := range snapshot.Tabs {
		snapshot.Tabs[index].Blocks, err = s.loadBlocks(snapshot.Tabs[index].Tab.ID)
		if err != nil {
			return Snapshot{}, err
		}
	}
	return snapshot, nil
}

func (s *Store) loadBlocks(tabID string) ([]BlockRecord, error) {
	rows, err := s.db.Query(`SELECT id, command, cwd, final_cwd, remote, state, exit_code,
		started_at, ended_at, duration_ms, output
		FROM (
			SELECT id, command, cwd, final_cwd, remote, state, exit_code, started_at,
				ended_at, duration_ms, output, position
			FROM blocks WHERE tab_id = ? ORDER BY position DESC LIMIT ?
		) ORDER BY position`, tabID, maxRestoredBlocksPerTab)
	if err != nil {
		return nil, fmt.Errorf("load blocks for tab %q: %w", tabID, err)
	}
	defer rows.Close()
	var result []BlockRecord
	for rows.Next() {
		var record BlockRecord
		var state string
		var exitCode, endedAt sql.NullInt64
		var startedAt int64
		if err := rows.Scan(&record.Block.ID, &record.Block.Command, &record.Block.CWD,
			&record.Block.FinalCWD, &record.Block.Remote, &state, &exitCode, &startedAt,
			&endedAt, &record.Block.Duration, &record.Output); err != nil {
			return nil, fmt.Errorf("scan restored block: %w", err)
		}
		record.Block.TabID = tabID
		record.Block.State = domain.BlockState(state)
		record.Block.StartedAt = time.UnixMilli(startedAt)
		if exitCode.Valid {
			value := int(exitCode.Int64)
			record.Block.ExitCode = &value
		}
		if endedAt.Valid {
			value := time.UnixMilli(endedAt.Int64)
			record.Block.EndedAt = &value
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate restored blocks: %w", err)
	}
	return result, nil
}

func (s *Store) SaveTab(tab domain.Tab, draft string, position int) error {
	_, err := s.db.Exec(`INSERT INTO tabs(id, title, cwd, position, draft, updated_at)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET title=excluded.title, cwd=excluded.cwd,
			position=excluded.position, draft=excluded.draft, updated_at=excluded.updated_at`,
		tab.ID, tab.Title, tab.CWD, position, draft, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("save workspace tab: %w", err)
	}
	return nil
}

func (s *Store) SetDraft(tabID, draft string) error {
	_, err := s.db.Exec(`UPDATE tabs SET draft = ?, updated_at = ? WHERE id = ?`, draft, time.Now().UnixMilli(), tabID)
	if err != nil {
		return fmt.Errorf("save tab draft: %w", err)
	}
	return nil
}

func (s *Store) SetActiveTab(tabID string) error {
	_, err := s.db.Exec(`INSERT INTO workspace_meta(key, value) VALUES('active_tab_id', ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, tabID)
	if err != nil {
		return fmt.Errorf("save active tab: %w", err)
	}
	return nil
}

func (s *Store) DeleteTab(tabID string) error {
	if err := s.flushOutput(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM tabs WHERE id = ?`, tabID); err != nil {
		return fmt.Errorf("delete workspace tab: %w", err)
	}
	return nil
}

func (s *Store) SaveBlock(block domain.Block) error {
	_, err := s.db.Exec(`INSERT INTO blocks(
		id, tab_id, position, command, cwd, final_cwd, remote, state, exit_code,
		started_at, ended_at, duration_ms, updated_at
	) VALUES(?, ?, COALESCE((SELECT MAX(position) + 1 FROM blocks WHERE tab_id = ?), 0),
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET final_cwd=excluded.final_cwd, remote=excluded.remote,
		state=excluded.state, exit_code=excluded.exit_code, ended_at=excluded.ended_at,
		duration_ms=excluded.duration_ms, updated_at=excluded.updated_at`,
		block.ID, block.TabID, block.TabID, block.Command, block.CWD, block.FinalCWD,
		block.Remote, block.State, nullableInt(block.ExitCode), block.StartedAt.UnixMilli(),
		nullableTime(block.EndedAt), block.Duration, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("save command block: %w", err)
	}
	return nil
}

func (s *Store) AppendOutput(blockID, value string) error {
	if value == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("workspace database is closed")
	}
	if s.outputErr != nil {
		return s.outputErr
	}
	buffered := s.pending[blockID] + value
	if len(buffered) > maxPersistedOutputBytes {
		buffered = buffered[len(buffered)-maxPersistedOutputBytes:]
	}
	s.pending[blockID] = buffered
	if s.timer == nil {
		s.timer = time.AfterFunc(outputFlushInterval, s.flushOutputBackground)
	}
	return nil
}

func (s *Store) ClearFinishedBlocks(tabID string) error {
	if err := s.flushOutput(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM blocks WHERE tab_id = ? AND state != ?`, tabID, domain.BlockRunning)
	if err != nil {
		return fmt.Errorf("clear workspace blocks: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	pending := s.pending
	s.pending = make(map[string]string)
	s.flushMu.Lock()
	s.mu.Unlock()
	err := s.writeOutputBatch(pending)
	closeErr := s.db.Close()
	s.flushMu.Unlock()
	return errors.Join(err, closeErr)
}

func (s *Store) flushOutputBackground() {
	if err := s.flushOutput(); err != nil {
		s.mu.Lock()
		s.outputErr = err
		s.mu.Unlock()
	}
}

func (s *Store) flushOutput() error {
	s.mu.Lock()
	if s.closed {
		err := s.outputErr
		s.mu.Unlock()
		return err
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	pending := s.pending
	s.pending = make(map[string]string)
	previousErr := s.outputErr
	// Taking flushMu before releasing mu ensures Close cannot overtake a timer
	// callback that has already removed a batch from pending.
	s.flushMu.Lock()
	s.mu.Unlock()
	err := s.writeOutputBatch(pending)
	s.flushMu.Unlock()
	if err != nil {
		s.mu.Lock()
		s.outputErr = err
		s.mu.Unlock()
	}
	return errors.Join(previousErr, err)
}

func (s *Store) writeOutputBatch(pending map[string]string) error {
	if len(pending) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin block output batch: %w", err)
	}
	for blockID, value := range pending {
		if _, err := tx.Exec(`UPDATE blocks SET output = CASE
			WHEN length(output) + length(?) > ? THEN substr(output || ?, -?)
			ELSE output || ? END, updated_at = ? WHERE id = ?`,
			value, maxPersistedOutputBytes, value, maxPersistedOutputBytes, value, time.Now().UnixMilli(), blockID); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("append block output: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit block output batch: %w", err)
	}
	return nil
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UnixMilli()
}
