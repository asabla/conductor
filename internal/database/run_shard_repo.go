package database

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// runShardRepo implements RunShardRepository.
type runShardRepo struct {
	db *DB
}

// NewRunShardRepo creates a new run shard repository.
func NewRunShardRepo(db *DB) RunShardRepository {
	return &runShardRepo{db: db}
}

// Create creates a new shard record.
func (r *runShardRepo) Create(ctx context.Context, shard *RunShard) error {
	status := shard.Status
	if status == "" {
		status = ShardStatusPending
	}

	err := r.db.pool.QueryRow(ctx, RunShardInsert,
		shard.RunID,
		shard.ShardIndex,
		shard.ShardCount,
		status,
		shard.TotalTests,
	).Scan(&shard.ID, &shard.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create run shard: %w", WrapDBError(err))
	}

	shard.Status = status
	return nil
}

// Get retrieves a shard by ID.
func (r *runShardRepo) Get(ctx context.Context, id uuid.UUID) (*RunShard, error) {
	shard := &RunShard{}
	err := r.db.pool.QueryRow(ctx, RunShardGetByID, id).Scan(
		&shard.ID,
		&shard.RunID,
		&shard.ShardIndex,
		&shard.ShardCount,
		&shard.Status,
		&shard.AgentID,
		&shard.TotalTests,
		&shard.PassedTests,
		&shard.FailedTests,
		&shard.SkippedTests,
		&shard.ErrorMessage,
		&shard.StartedAt,
		&shard.FinishedAt,
		&shard.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get run shard: %w", err)
	}
	return shard, nil
}

// ListByRun lists shards for a run.
func (r *runShardRepo) ListByRun(ctx context.Context, runID uuid.UUID) ([]RunShard, error) {
	rows, err := r.db.pool.Query(ctx, RunShardListByRun, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list run shards: %w", err)
	}
	defer rows.Close()

	var shards []RunShard
	for rows.Next() {
		var shard RunShard
		err := rows.Scan(
			&shard.ID,
			&shard.RunID,
			&shard.ShardIndex,
			&shard.ShardCount,
			&shard.Status,
			&shard.AgentID,
			&shard.TotalTests,
			&shard.PassedTests,
			&shard.FailedTests,
			&shard.SkippedTests,
			&shard.ErrorMessage,
			&shard.StartedAt,
			&shard.FinishedAt,
			&shard.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run shard: %w", err)
		}
		shards = append(shards, shard)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating run shards: %w", err)
	}

	return shards, nil
}

// UpdateStatus updates a shard status.
func (r *runShardRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status ShardStatus) error {
	result, err := r.db.pool.Exec(ctx, RunShardUpdateStatus, id, status)
	if err != nil {
		return fmt.Errorf("failed to update shard status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Start marks a shard as started.
func (r *runShardRepo) Start(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error {
	result, err := r.db.pool.Exec(ctx, RunShardStart, id, agentID)
	if err != nil {
		return fmt.Errorf("failed to start shard: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Finish marks a shard as finished with results.
func (r *runShardRepo) Finish(ctx context.Context, id uuid.UUID, status ShardStatus, results RunResults) error {
	var errorMsg *string
	if results.ErrorMessage != "" {
		errorMsg = &results.ErrorMessage
	}

	result, err := r.db.pool.Exec(ctx, RunShardFinish,
		id,
		status,
		results.TotalTests,
		results.PassedTests,
		results.FailedTests,
		results.SkippedTests,
		errorMsg,
	)
	if err != nil {
		return fmt.Errorf("failed to finish shard: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Reset resets a shard for retry.
func (r *runShardRepo) Reset(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.pool.Exec(ctx, RunShardReset, id)
	if err != nil {
		return fmt.Errorf("failed to reset shard: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteByRun deletes shards for a run.
func (r *runShardRepo) DeleteByRun(ctx context.Context, runID uuid.UUID) error {
	_, err := r.db.pool.Exec(ctx, RunShardDeleteByRun, runID)
	if err != nil {
		return fmt.Errorf("failed to delete run shards: %w", err)
	}
	return nil
}
