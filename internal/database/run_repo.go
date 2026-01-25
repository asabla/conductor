package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// runRepo implements TestRunRepository.
type runRepo struct {
	db *DB
}

// NewRunRepo creates a new test run repository.
func NewRunRepo(db *DB) TestRunRepository {
	return &runRepo{db: db}
}

// Create creates a new test run.
func (r *runRepo) Create(ctx context.Context, run *TestRun) error {
	err := r.db.pool.QueryRow(ctx, RunInsert,
		run.ServiceID,
		run.Status,
		run.GitRef,
		run.GitSHA,
		run.TriggerType,
		run.TriggeredBy,
		run.Priority,
	).Scan(&run.ID, &run.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create test run: %w", WrapDBError(err))
	}
	return nil
}

// Get retrieves a test run by ID.
func (r *runRepo) Get(ctx context.Context, id uuid.UUID) (*TestRun, error) {
	run := &TestRun{}
	err := r.db.pool.QueryRow(ctx, RunGetByID, id).Scan(
		&run.ID,
		&run.ServiceID,
		&run.AgentID,
		&run.Status,
		&run.GitRef,
		&run.GitSHA,
		&run.TriggerType,
		&run.TriggeredBy,
		&run.Priority,
		&run.CreatedAt,
		&run.StartedAt,
		&run.FinishedAt,
		&run.TotalTests,
		&run.PassedTests,
		&run.FailedTests,
		&run.SkippedTests,
		&run.ShardCount,
		&run.ShardsDone,
		&run.ShardsFailed,
		&run.MaxParallel,
		&run.DurationMs,
		&run.ErrorMessage,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get test run: %w", err)
	}
	return run, nil
}

// Update updates a test run.
func (r *runRepo) Update(ctx context.Context, run *TestRun) error {
	result, err := r.db.pool.Exec(ctx, RunUpdate,
		run.ID,
		run.AgentID,
		run.Status,
		run.StartedAt,
		run.FinishedAt,
		run.TotalTests,
		run.PassedTests,
		run.FailedTests,
		run.SkippedTests,
		run.DurationMs,
		run.ErrorMessage,
	)

	if err != nil {
		return fmt.Errorf("failed to update test run: %w", WrapDBError(err))
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateStatus updates only the run's status.
func (r *runRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status RunStatus) error {
	result, err := r.db.pool.Exec(ctx, RunUpdateStatus, id, status)
	if err != nil {
		return fmt.Errorf("failed to update run status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Start marks a run as started with the given agent.
func (r *runRepo) Start(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error {
	result, err := r.db.pool.Exec(ctx, RunStart, id, agentID)
	if err != nil {
		return fmt.Errorf("failed to start test run: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Finish marks a run as finished with results.
func (r *runRepo) Finish(ctx context.Context, id uuid.UUID, status RunStatus, results RunResults) error {
	var errorMsg *string
	if results.ErrorMessage != "" {
		errorMsg = &results.ErrorMessage
	}

	result, err := r.db.pool.Exec(ctx, RunFinish,
		id,
		status,
		results.TotalTests,
		results.PassedTests,
		results.FailedTests,
		results.SkippedTests,
		results.DurationMs,
		errorMsg,
	)
	if err != nil {
		return fmt.Errorf("failed to finish test run: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns test runs with pagination.
func (r *runRepo) List(ctx context.Context, page Pagination) ([]TestRun, error) {
	rows, err := r.db.pool.Query(ctx, RunList, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list test runs: %w", err)
	}
	defer rows.Close()

	return scanTestRuns(rows)
}

// ListByService returns test runs for a service.
func (r *runRepo) ListByService(ctx context.Context, serviceID uuid.UUID, page Pagination) ([]TestRun, error) {
	rows, err := r.db.pool.Query(ctx, RunListByService, serviceID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list test runs by service: %w", err)
	}
	defer rows.Close()

	return scanTestRuns(rows)
}

// ListByStatus returns test runs with a specific status.
func (r *runRepo) ListByStatus(ctx context.Context, status RunStatus, page Pagination) ([]TestRun, error) {
	rows, err := r.db.pool.Query(ctx, RunListByStatus, status, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list test runs by status: %w", err)
	}
	defer rows.Close()

	return scanTestRuns(rows)
}

// ListByServiceAndStatus returns test runs for a service with a specific status.
func (r *runRepo) ListByServiceAndStatus(ctx context.Context, serviceID uuid.UUID, status RunStatus, page Pagination) ([]TestRun, error) {
	rows, err := r.db.pool.Query(ctx, RunListByServiceAndStatus, serviceID, status, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list test runs by service and status: %w", err)
	}
	defer rows.Close()

	return scanTestRuns(rows)
}

// ListByDateRange returns test runs within a date range.
func (r *runRepo) ListByDateRange(ctx context.Context, start, end time.Time, page Pagination) ([]TestRun, error) {
	rows, err := r.db.pool.Query(ctx, RunListByDateRange, start, end, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list test runs by date range: %w", err)
	}
	defer rows.Close()

	return scanTestRuns(rows)
}

// GetPending returns pending runs ordered by priority.
func (r *runRepo) GetPending(ctx context.Context, limit int) ([]TestRun, error) {
	rows, err := r.db.pool.Query(ctx, RunGetPending, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending runs: %w", err)
	}
	defer rows.Close()

	return scanTestRuns(rows)
}

// GetRunning returns currently running tests.
func (r *runRepo) GetRunning(ctx context.Context) ([]TestRun, error) {
	rows, err := r.db.pool.Query(ctx, RunGetRunning)
	if err != nil {
		return nil, fmt.Errorf("failed to get running tests: %w", err)
	}
	defer rows.Close()

	return scanTestRuns(rows)
}

// Count returns the total number of test runs.
func (r *runRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.pool.QueryRow(ctx, RunCount).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count test runs: %w", err)
	}
	return count, nil
}

// CountByStatus returns the count of runs grouped by status.
func (r *runRepo) CountByStatus(ctx context.Context) (map[RunStatus]int64, error) {
	rows, err := r.db.pool.Query(ctx, RunCountByStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to count runs by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[RunStatus]int64)
	for rows.Next() {
		var status RunStatus
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan run count: %w", err)
		}
		counts[status] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating run counts: %w", err)
	}

	return counts, nil
}

// scanTestRuns scans rows into a slice of test runs.
func scanTestRuns(rows pgx.Rows) ([]TestRun, error) {
	var runs []TestRun
	for rows.Next() {
		var run TestRun
		err := rows.Scan(
			&run.ID,
			&run.ServiceID,
			&run.AgentID,
			&run.Status,
			&run.GitRef,
			&run.GitSHA,
			&run.TriggerType,
			&run.TriggeredBy,
			&run.Priority,
			&run.CreatedAt,
			&run.StartedAt,
			&run.FinishedAt,
			&run.TotalTests,
			&run.PassedTests,
			&run.FailedTests,
			&run.SkippedTests,
			&run.ShardCount,
			&run.ShardsDone,
			&run.ShardsFailed,
			&run.MaxParallel,
			&run.DurationMs,
			&run.ErrorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test run: %w", err)
		}
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating test runs: %w", err)
	}

	return runs, nil
}
