package database

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// resultRepo implements ResultRepository.
type resultRepo struct {
	db *DB
}

// NewResultRepo creates a new result repository.
func NewResultRepo(db *DB) ResultRepository {
	return &resultRepo{db: db}
}

// Create creates a new test result.
func (r *resultRepo) Create(ctx context.Context, result *TestResult) error {
	err := r.db.pool.QueryRow(ctx, ResultInsert,
		result.RunID,
		result.TestDefinitionID,
		result.TestName,
		result.SuiteName,
		result.Status,
		result.DurationMs,
		result.ErrorMessage,
		result.StackTrace,
		result.Stdout,
		result.Stderr,
		result.RetryCount,
	).Scan(&result.ID, &result.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create test result: %w", WrapDBError(err))
	}
	return nil
}

// BatchCreate creates multiple test results in a single operation.
func (r *resultRepo) BatchCreate(ctx context.Context, results []TestResult) error {
	if len(results) == 0 {
		return nil
	}

	// Use a transaction for batch insert
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}

		for i := range results {
			result := &results[i]
			batch.Queue(ResultInsert,
				result.RunID,
				result.TestDefinitionID,
				result.TestName,
				result.SuiteName,
				result.Status,
				result.DurationMs,
				result.ErrorMessage,
				result.StackTrace,
				result.Stdout,
				result.Stderr,
				result.RetryCount,
			)
		}

		br := tx.SendBatch(ctx, batch)
		defer br.Close()

		// Process all results to check for errors and scan IDs
		for i := range results {
			err := br.QueryRow().Scan(&results[i].ID, &results[i].CreatedAt)
			if err != nil {
				return fmt.Errorf("failed to create test result %d: %w", i, WrapDBError(err))
			}
		}

		return nil
	})
}

// Get retrieves a test result by ID.
func (r *resultRepo) Get(ctx context.Context, id uuid.UUID) (*TestResult, error) {
	result := &TestResult{}
	err := r.db.pool.QueryRow(ctx, ResultGetByID, id).Scan(
		&result.ID,
		&result.RunID,
		&result.TestDefinitionID,
		&result.TestName,
		&result.SuiteName,
		&result.Status,
		&result.DurationMs,
		&result.ErrorMessage,
		&result.StackTrace,
		&result.Stdout,
		&result.Stderr,
		&result.RetryCount,
		&result.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get test result: %w", err)
	}
	return result, nil
}

// ListByRun returns all results for a test run.
func (r *resultRepo) ListByRun(ctx context.Context, runID uuid.UUID) ([]TestResult, error) {
	rows, err := r.db.pool.Query(ctx, ResultListByRun, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list test results: %w", err)
	}
	defer rows.Close()

	return scanTestResults(rows)
}

// ListByRunAndStatus returns results for a run with a specific status.
func (r *resultRepo) ListByRunAndStatus(ctx context.Context, runID uuid.UUID, status ResultStatus) ([]TestResult, error) {
	rows, err := r.db.pool.Query(ctx, ResultListByRunAndStatus, runID, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list test results by status: %w", err)
	}
	defer rows.Close()

	return scanTestResults(rows)
}

// CountByRun returns the count of results grouped by status for a run.
func (r *resultRepo) CountByRun(ctx context.Context, runID uuid.UUID) (map[ResultStatus]int64, error) {
	rows, err := r.db.pool.Query(ctx, ResultCountByRun, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to count results by run: %w", err)
	}
	defer rows.Close()

	counts := make(map[ResultStatus]int64)
	for rows.Next() {
		var status ResultStatus
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan result count: %w", err)
		}
		counts[status] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating result counts: %w", err)
	}

	return counts, nil
}

// DeleteByRun deletes all results for a run.
func (r *resultRepo) DeleteByRun(ctx context.Context, runID uuid.UUID) error {
	_, err := r.db.pool.Exec(ctx, ResultDelete, runID)
	if err != nil {
		return fmt.Errorf("failed to delete results: %w", err)
	}
	return nil
}

// scanTestResults scans rows into a slice of test results.
func scanTestResults(rows pgx.Rows) ([]TestResult, error) {
	var results []TestResult
	for rows.Next() {
		var result TestResult
		err := rows.Scan(
			&result.ID,
			&result.RunID,
			&result.TestDefinitionID,
			&result.TestName,
			&result.SuiteName,
			&result.Status,
			&result.DurationMs,
			&result.ErrorMessage,
			&result.StackTrace,
			&result.Stdout,
			&result.Stderr,
			&result.RetryCount,
			&result.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test result: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating test results: %w", err)
	}

	return results, nil
}

// artifactRepo implements ArtifactRepository.
type artifactRepo struct {
	db *DB
}

// NewArtifactRepo creates a new artifact repository.
func NewArtifactRepo(db *DB) ArtifactRepository {
	return &artifactRepo{db: db}
}

// Create creates a new artifact record.
func (r *artifactRepo) Create(ctx context.Context, artifact *Artifact) error {
	err := r.db.pool.QueryRow(ctx, ArtifactInsert,
		artifact.RunID,
		artifact.Name,
		artifact.Path,
		artifact.ContentType,
		artifact.SizeBytes,
	).Scan(&artifact.ID, &artifact.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create artifact: %w", WrapDBError(err))
	}
	return nil
}

// Get retrieves an artifact by ID.
func (r *artifactRepo) Get(ctx context.Context, id uuid.UUID) (*Artifact, error) {
	artifact := &Artifact{}
	err := r.db.pool.QueryRow(ctx, ArtifactGetByID, id).Scan(
		&artifact.ID,
		&artifact.RunID,
		&artifact.Name,
		&artifact.Path,
		&artifact.ContentType,
		&artifact.SizeBytes,
		&artifact.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get artifact: %w", err)
	}
	return artifact, nil
}

// ListByRun returns all artifacts for a test run.
func (r *artifactRepo) ListByRun(ctx context.Context, runID uuid.UUID) ([]Artifact, error) {
	rows, err := r.db.pool.Query(ctx, ArtifactListByRun, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []Artifact
	for rows.Next() {
		var artifact Artifact
		err := rows.Scan(
			&artifact.ID,
			&artifact.RunID,
			&artifact.Name,
			&artifact.Path,
			&artifact.ContentType,
			&artifact.SizeBytes,
			&artifact.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan artifact: %w", err)
		}
		artifacts = append(artifacts, artifact)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating artifacts: %w", err)
	}

	return artifacts, nil
}

// Delete deletes an artifact record.
func (r *artifactRepo) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.pool.Exec(ctx, ArtifactDelete, id)
	if err != nil {
		return fmt.Errorf("failed to delete artifact: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteByRun deletes all artifact records for a run.
func (r *artifactRepo) DeleteByRun(ctx context.Context, runID uuid.UUID) error {
	_, err := r.db.pool.Exec(ctx, ArtifactDeleteByRun, runID)
	if err != nil {
		return fmt.Errorf("failed to delete artifacts for run: %w", err)
	}
	return nil
}
