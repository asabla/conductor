package database

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// agentRepo implements AgentRepository.
type agentRepo struct {
	db *DB
}

// NewAgentRepo creates a new agent repository.
func NewAgentRepo(db *DB) AgentRepository {
	return &agentRepo{db: db}
}

// Create creates a new agent.
func (r *agentRepo) Create(ctx context.Context, agent *Agent) error {
	err := r.db.pool.QueryRow(ctx, AgentInsert,
		agent.Name,
		agent.Status,
		agent.Version,
		agent.NetworkZones,
		agent.MaxParallel,
		agent.DockerAvailable,
	).Scan(&agent.ID, &agent.RegisteredAt)

	if err != nil {
		return fmt.Errorf("failed to create agent: %w", WrapDBError(err))
	}
	return nil
}

// Get retrieves an agent by ID.
func (r *agentRepo) Get(ctx context.Context, id uuid.UUID) (*Agent, error) {
	agent := &Agent{}
	err := r.db.pool.QueryRow(ctx, AgentGetByID, id).Scan(
		&agent.ID,
		&agent.Name,
		&agent.Status,
		&agent.Version,
		&agent.NetworkZones,
		&agent.MaxParallel,
		&agent.DockerAvailable,
		&agent.LastHeartbeat,
		&agent.RegisteredAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	return agent, nil
}

// GetByName retrieves an agent by name.
func (r *agentRepo) GetByName(ctx context.Context, name string) (*Agent, error) {
	agent := &Agent{}
	err := r.db.pool.QueryRow(ctx, AgentGetByName, name).Scan(
		&agent.ID,
		&agent.Name,
		&agent.Status,
		&agent.Version,
		&agent.NetworkZones,
		&agent.MaxParallel,
		&agent.DockerAvailable,
		&agent.LastHeartbeat,
		&agent.RegisteredAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get agent by name: %w", err)
	}
	return agent, nil
}

// Update updates an agent.
func (r *agentRepo) Update(ctx context.Context, agent *Agent) error {
	result, err := r.db.pool.Exec(ctx, AgentUpdate,
		agent.ID,
		agent.Name,
		agent.Status,
		agent.Version,
		agent.NetworkZones,
		agent.MaxParallel,
		agent.DockerAvailable,
	)

	if err != nil {
		return fmt.Errorf("failed to update agent: %w", WrapDBError(err))
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete deletes an agent.
func (r *agentRepo) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.pool.Exec(ctx, AgentDelete, id)
	if err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns agents with pagination.
func (r *agentRepo) List(ctx context.Context, page Pagination) ([]Agent, error) {
	rows, err := r.db.pool.Query(ctx, AgentList, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	defer rows.Close()

	return scanAgents(rows)
}

// ListByStatus returns agents with a specific status.
func (r *agentRepo) ListByStatus(ctx context.Context, status AgentStatus, page Pagination) ([]Agent, error) {
	rows, err := r.db.pool.Query(ctx, AgentListByStatus, status, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents by status: %w", err)
	}
	defer rows.Close()

	return scanAgents(rows)
}

// UpdateStatus updates only the agent's status.
func (r *agentRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status AgentStatus) error {
	result, err := r.db.pool.Exec(ctx, AgentUpdateStatus, id, status)
	if err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateHeartbeat updates the agent's heartbeat time and status.
func (r *agentRepo) UpdateHeartbeat(ctx context.Context, id uuid.UUID, status AgentStatus) error {
	result, err := r.db.pool.Exec(ctx, AgentUpdateHeartbeat, id, status)
	if err != nil {
		return fmt.Errorf("failed to update agent heartbeat: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAvailable returns agents available to run tests for services in the given zones.
func (r *agentRepo) GetAvailable(ctx context.Context, zones []string, limit int) ([]Agent, error) {
	rows, err := r.db.pool.Query(ctx, AgentGetAvailable, zones, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get available agents: %w", err)
	}
	defer rows.Close()

	return scanAgents(rows)
}

// MarkOfflineAgents marks agents as offline if they haven't sent a heartbeat recently.
func (r *agentRepo) MarkOfflineAgents(ctx context.Context) (int64, error) {
	result, err := r.db.pool.Exec(ctx, AgentMarkOffline)
	if err != nil {
		return 0, fmt.Errorf("failed to mark agents offline: %w", err)
	}
	return result.RowsAffected(), nil
}

// CountByStatus returns the count of agents grouped by status.
func (r *agentRepo) CountByStatus(ctx context.Context) (map[AgentStatus]int64, error) {
	rows, err := r.db.pool.Query(ctx, AgentCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count agents by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[AgentStatus]int64)
	for rows.Next() {
		var status AgentStatus
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan agent count: %w", err)
		}
		counts[status] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating agent counts: %w", err)
	}

	return counts, nil
}

// scanAgents scans rows into a slice of agents.
func scanAgents(rows pgx.Rows) ([]Agent, error) {
	var agents []Agent
	for rows.Next() {
		var agent Agent
		err := rows.Scan(
			&agent.ID,
			&agent.Name,
			&agent.Status,
			&agent.Version,
			&agent.NetworkZones,
			&agent.MaxParallel,
			&agent.DockerAvailable,
			&agent.LastHeartbeat,
			&agent.RegisteredAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}
		agents = append(agents, agent)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating agents: %w", err)
	}

	return agents, nil
}
