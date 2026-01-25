package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// State manages local state persistence using SQLite.
type State struct {
	db     *sql.DB
	dbPath string
	mu     sync.RWMutex
}

// RunState represents a persisted run state.
type RunState struct {
	RunID     string
	Status    string
	Work      *conductorv1.AssignWork
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewState creates a new state manager.
func NewState(stateDir string) (*State, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	dbPath := filepath.Join(stateDir, "agent.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set journal mode: %w", err)
	}

	// Create tables
	if err := createTables(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return &State{
		db:     db,
		dbPath: dbPath,
	}, nil
}

// createTables creates the required database tables.
func createTables(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS runs (
			run_id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			work_json TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
		CREATE INDEX IF NOT EXISTS idx_runs_created_at ON runs(created_at);
	`

	_, err := db.Exec(schema)
	return err
}

// SaveRunState saves or updates a run state.
func (s *State) SaveRunState(runID, status string, work *conductorv1.AssignWork) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Serialize work to JSON
	workJSON, err := json.Marshal(work)
	if err != nil {
		return fmt.Errorf("failed to serialize work: %w", err)
	}

	query := `
		INSERT INTO runs (run_id, status, work_json, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(run_id) DO UPDATE SET
			status = excluded.status,
			work_json = excluded.work_json,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = s.db.Exec(query, runID, status, string(workJSON))
	if err != nil {
		return fmt.Errorf("failed to save run state: %w", err)
	}

	return nil
}

// GetRunState retrieves a run state by ID.
func (s *State) GetRunState(runID string) (*RunState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT run_id, status, work_json, created_at, updated_at
		FROM runs
		WHERE run_id = ?
	`

	row := s.db.QueryRow(query, runID)

	var state RunState
	var workJSON string

	err := row.Scan(&state.RunID, &state.Status, &workJSON, &state.CreatedAt, &state.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get run state: %w", err)
	}

	// Deserialize work
	if err := json.Unmarshal([]byte(workJSON), &state.Work); err != nil {
		return nil, fmt.Errorf("failed to deserialize work: %w", err)
	}

	return &state, nil
}

// GetPendingRuns returns all runs in a running or pending state.
func (s *State) GetPendingRuns() ([]*RunState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT run_id, status, work_json, created_at, updated_at
		FROM runs
		WHERE status IN ('running', 'pending')
		ORDER BY created_at ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending runs: %w", err)
	}
	defer rows.Close()

	var states []*RunState

	for rows.Next() {
		var state RunState
		var workJSON string

		if err := rows.Scan(&state.RunID, &state.Status, &workJSON, &state.CreatedAt, &state.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan run state: %w", err)
		}

		if err := json.Unmarshal([]byte(workJSON), &state.Work); err != nil {
			return nil, fmt.Errorf("failed to deserialize work: %w", err)
		}

		states = append(states, &state)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return states, nil
}

// DeleteRunState removes a run state.
func (s *State) DeleteRunState(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM runs WHERE run_id = ?", runID)
	if err != nil {
		return fmt.Errorf("failed to delete run state: %w", err)
	}

	return nil
}

// Cleanup removes old completed run states.
func (s *State) Cleanup(maxAge time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	_, err := s.db.Exec(`
		DELETE FROM runs
		WHERE status NOT IN ('running', 'pending')
		AND updated_at < ?
	`, cutoff)

	if err != nil {
		return fmt.Errorf("failed to cleanup old states: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (s *State) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
