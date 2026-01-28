// Package wire provides dependency wiring between database repositories and server interfaces.
package wire

import (
	"context"
	"time"

	"github.com/google/uuid"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/artifact"
	"github.com/conductor/conductor/internal/database"
	"github.com/conductor/conductor/internal/server"
)

// AgentRepositoryAdapter adapts database.AgentRepository to server.AgentRepository.
type AgentRepositoryAdapter struct {
	repo database.AgentRepository
}

// NewAgentRepositoryAdapter creates a new adapter.
func NewAgentRepositoryAdapter(repo database.AgentRepository) *AgentRepositoryAdapter {
	return &AgentRepositoryAdapter{repo: repo}
}

func (a *AgentRepositoryAdapter) Create(ctx context.Context, agent *database.Agent) error {
	return a.repo.Create(ctx, agent)
}

func (a *AgentRepositoryAdapter) Update(ctx context.Context, agent *database.Agent) error {
	return a.repo.Update(ctx, agent)
}

func (a *AgentRepositoryAdapter) GetByID(ctx context.Context, id uuid.UUID) (*database.Agent, error) {
	agent, err := a.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return agent, nil
}

func (a *AgentRepositoryAdapter) UpdateHeartbeat(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	return a.repo.UpdateHeartbeat(ctx, id, status)
}

func (a *AgentRepositoryAdapter) UpdateStatus(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	return a.repo.UpdateStatus(ctx, id, status)
}

func (a *AgentRepositoryAdapter) List(ctx context.Context, filter server.AgentFilter, pagination database.Pagination) ([]*database.Agent, int, error) {
	var agents []database.Agent
	var err error

	if len(filter.Statuses) > 0 {
		// Use first status for now - database repo only supports single status
		agents, err = a.repo.ListByStatus(ctx, filter.Statuses[0], pagination)
	} else {
		agents, err = a.repo.List(ctx, pagination)
	}

	if err != nil {
		return nil, 0, err
	}

	// Convert to pointer slice
	result := make([]*database.Agent, len(agents))
	for i := range agents {
		result[i] = &agents[i]
	}

	// Get count
	counts, err := a.repo.CountByStatus(ctx)
	if err != nil {
		return result, len(result), nil // Return without total on error
	}

	var total int64
	for _, c := range counts {
		total += c
	}

	return result, int(total), nil
}

func (a *AgentRepositoryAdapter) Delete(ctx context.Context, id uuid.UUID) error {
	return a.repo.Delete(ctx, id)
}

// RunRepositoryAdapter adapts database.TestRunRepository to server.RunRepository.
type RunRepositoryAdapter struct {
	repo database.TestRunRepository
}

// NewRunRepositoryAdapter creates a new adapter.
func NewRunRepositoryAdapter(repo database.TestRunRepository) *RunRepositoryAdapter {
	return &RunRepositoryAdapter{repo: repo}
}

func (a *RunRepositoryAdapter) Create(ctx context.Context, run *database.TestRun) error {
	return a.repo.Create(ctx, run)
}

func (a *RunRepositoryAdapter) Update(ctx context.Context, run *database.TestRun) error {
	return a.repo.Update(ctx, run)
}

func (a *RunRepositoryAdapter) GetByID(ctx context.Context, id uuid.UUID) (*database.TestRun, error) {
	run, err := a.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return run, nil
}

func (a *RunRepositoryAdapter) List(ctx context.Context, filter server.RunFilter, pagination database.Pagination) ([]*database.TestRun, int, error) {
	var runs []database.TestRun
	var err error

	// Apply filters - database repo has limited filtering support
	if filter.ServiceID != nil && len(filter.Statuses) > 0 {
		runs, err = a.repo.ListByServiceAndStatus(ctx, *filter.ServiceID, filter.Statuses[0], pagination)
	} else if filter.ServiceID != nil {
		runs, err = a.repo.ListByService(ctx, *filter.ServiceID, pagination)
	} else if len(filter.Statuses) > 0 {
		runs, err = a.repo.ListByStatus(ctx, filter.Statuses[0], pagination)
	} else if filter.StartTime != nil && filter.EndTime != nil {
		runs, err = a.repo.ListByDateRange(ctx, *filter.StartTime, *filter.EndTime, pagination)
	} else {
		runs, err = a.repo.List(ctx, pagination)
	}

	if err != nil {
		return nil, 0, err
	}

	// Convert to pointer slice
	result := make([]*database.TestRun, len(runs))
	for i := range runs {
		result[i] = &runs[i]
	}

	// Get total count
	total, err := a.repo.Count(ctx)
	if err != nil {
		return result, len(result), nil
	}

	return result, int(total), nil
}

func (a *RunRepositoryAdapter) UpdateStatus(ctx context.Context, id uuid.UUID, status database.RunStatus, errorMsg *string) error {
	// Get run first to update it
	run, err := a.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	run.Status = status
	run.ErrorMessage = errorMsg
	return a.repo.Update(ctx, run)
}

func (a *RunRepositoryAdapter) Finish(ctx context.Context, id uuid.UUID, status database.RunStatus, results database.RunResults) error {
	return a.repo.Finish(ctx, id, status, results)
}

func (a *RunRepositoryAdapter) UpdateShardStats(ctx context.Context, id uuid.UUID, completed int, failed int, results database.RunResults) error {
	return a.repo.UpdateShardStats(ctx, id, completed, failed, results)
}

// ServiceRepositoryAdapter adapts database.ServiceRepository to server.FullServiceRepository.
type ServiceRepositoryAdapter struct {
	repo database.ServiceRepository
}

// NewServiceRepositoryAdapter creates a new adapter.
func NewServiceRepositoryAdapter(repo database.ServiceRepository) *ServiceRepositoryAdapter {
	return &ServiceRepositoryAdapter{repo: repo}
}

func (a *ServiceRepositoryAdapter) Create(ctx context.Context, service *database.Service) error {
	return a.repo.Create(ctx, service)
}

func (a *ServiceRepositoryAdapter) Update(ctx context.Context, service *database.Service) error {
	return a.repo.Update(ctx, service)
}

func (a *ServiceRepositoryAdapter) GetByID(ctx context.Context, id uuid.UUID) (*database.Service, error) {
	svc, err := a.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func (a *ServiceRepositoryAdapter) List(ctx context.Context, filter server.ServiceFilter, pagination database.Pagination) ([]*database.Service, int, error) {
	var services []database.Service
	var err error

	if filter.Query != "" {
		services, err = a.repo.Search(ctx, filter.Query, pagination)
	} else if filter.Owner != "" {
		services, err = a.repo.ListByOwner(ctx, filter.Owner, pagination)
	} else {
		services, err = a.repo.List(ctx, pagination)
	}

	if err != nil {
		return nil, 0, err
	}

	// Convert to pointer slice
	result := make([]*database.Service, len(services))
	for i := range services {
		result[i] = &services[i]
	}

	// Get total count
	total, err := a.repo.Count(ctx)
	if err != nil {
		return result, len(result), nil
	}

	return result, int(total), nil
}

func (a *ServiceRepositoryAdapter) Delete(ctx context.Context, id uuid.UUID) error {
	return a.repo.Delete(ctx, id)
}

// TestDefinitionRepositoryAdapter adapts database.TestDefinitionRepository to server.TestDefinitionRepository.
type TestDefinitionRepositoryAdapter struct {
	repo database.TestDefinitionRepository
}

// NewTestDefinitionRepositoryAdapter creates a new adapter.
func NewTestDefinitionRepositoryAdapter(repo database.TestDefinitionRepository) *TestDefinitionRepositoryAdapter {
	return &TestDefinitionRepositoryAdapter{repo: repo}
}

func (a *TestDefinitionRepositoryAdapter) Create(ctx context.Context, test *database.TestDefinition) error {
	return a.repo.Create(ctx, test)
}

func (a *TestDefinitionRepositoryAdapter) Update(ctx context.Context, test *database.TestDefinition) error {
	return a.repo.Update(ctx, test)
}

func (a *TestDefinitionRepositoryAdapter) GetByID(ctx context.Context, serviceID, testID uuid.UUID) (*database.TestDefinition, error) {
	// Note: The database repo Get method takes only testID
	test, err := a.repo.Get(ctx, testID)
	if err != nil {
		return nil, err
	}
	// Verify service ID matches
	if test.ServiceID != serviceID {
		return nil, database.ErrNotFound
	}
	return test, nil
}

func (a *TestDefinitionRepositoryAdapter) ListByService(ctx context.Context, serviceID uuid.UUID, filter server.TestDefinitionFilter, pagination database.Pagination) ([]*database.TestDefinition, int, error) {
	var defs []database.TestDefinition
	var err error

	if len(filter.Tags) > 0 {
		defs, err = a.repo.ListByTags(ctx, serviceID, filter.Tags, pagination)
	} else {
		defs, err = a.repo.ListByService(ctx, serviceID, pagination)
	}

	if err != nil {
		return nil, 0, err
	}

	// Convert to pointer slice
	result := make([]*database.TestDefinition, len(defs))
	for i := range defs {
		result[i] = &defs[i]
	}

	return result, len(result), nil
}

func (a *TestDefinitionRepositoryAdapter) Delete(ctx context.Context, id uuid.UUID) error {
	return a.repo.Delete(ctx, id)
}

func (a *TestDefinitionRepositoryAdapter) DeleteByService(ctx context.Context, serviceID uuid.UUID) error {
	// List all tests for service and delete them
	tests, err := a.repo.ListByService(ctx, serviceID, database.Pagination{Limit: 1000})
	if err != nil {
		return err
	}
	for _, test := range tests {
		if err := a.repo.Delete(ctx, test.ID); err != nil {
			return err
		}
	}
	return nil
}

// ResultRepositoryAdapter adapts database.ResultRepository to server.ResultRepository.
type ResultRepositoryAdapter struct {
	repo database.ResultRepository
}

// NewResultRepositoryAdapter creates a new adapter.
func NewResultRepositoryAdapter(repo database.ResultRepository) *ResultRepositoryAdapter {
	return &ResultRepositoryAdapter{repo: repo}
}

func (a *ResultRepositoryAdapter) GetByRunID(ctx context.Context, runID uuid.UUID) ([]*database.TestResult, error) {
	results, err := a.repo.ListByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	// Convert to pointer slice
	ptrs := make([]*database.TestResult, len(results))
	for i := range results {
		ptrs[i] = &results[i]
	}
	return ptrs, nil
}

func (a *ResultRepositoryAdapter) List(ctx context.Context, runID uuid.UUID, filter server.ResultFilter, pagination database.Pagination) ([]*database.TestResult, int, error) {
	var results []database.TestResult
	var err error

	if len(filter.Statuses) > 0 {
		// Use first status for now - database repo only supports single status
		results, err = a.repo.ListByRunAndStatus(ctx, runID, filter.Statuses[0])
	} else {
		results, err = a.repo.ListByRun(ctx, runID)
	}

	if err != nil {
		return nil, 0, err
	}

	// Convert to pointer slice
	ptrs := make([]*database.TestResult, len(results))
	for i := range results {
		ptrs[i] = &results[i]
	}

	return ptrs, len(ptrs), nil
}

func (a *ResultRepositoryAdapter) Create(ctx context.Context, result *database.TestResult) error {
	return a.repo.Create(ctx, result)
}

// ArtifactRepositoryAdapter adapts database.ArtifactRepository to server.ArtifactRepository.
type ArtifactRepositoryAdapter struct {
	repo database.ArtifactRepository
}

// NewArtifactRepositoryAdapter creates a new adapter.
func NewArtifactRepositoryAdapter(repo database.ArtifactRepository) *ArtifactRepositoryAdapter {
	return &ArtifactRepositoryAdapter{repo: repo}
}

func (a *ArtifactRepositoryAdapter) GetByID(ctx context.Context, id uuid.UUID) (*database.Artifact, error) {
	artifact, err := a.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return artifact, nil
}

func (a *ArtifactRepositoryAdapter) ListByRunID(ctx context.Context, runID uuid.UUID, pagination database.Pagination) ([]*database.Artifact, int, error) {
	artifacts, err := a.repo.ListByRun(ctx, runID)
	if err != nil {
		return nil, 0, err
	}
	// Convert to pointer slice
	ptrs := make([]*database.Artifact, len(artifacts))
	for i := range artifacts {
		ptrs[i] = &artifacts[i]
	}
	return ptrs, len(ptrs), nil
}

func (a *ArtifactRepositoryAdapter) Create(ctx context.Context, artifact *database.Artifact) error {
	return a.repo.Create(ctx, artifact)
}

// NoopScheduler implements server.WorkScheduler as a no-op for initial setup.
// TODO: Replace with real scheduler integration.
type NoopScheduler struct{}

func (s *NoopScheduler) AssignWork(ctx context.Context, agentID uuid.UUID, capabilities *conductorv1.Capabilities) (*conductorv1.AssignWork, error) {
	return nil, nil
}

func (s *NoopScheduler) CancelWork(ctx context.Context, runID uuid.UUID, reason string) error {
	return nil
}

func (s *NoopScheduler) HandleWorkAccepted(ctx context.Context, agentID, runID uuid.UUID, shardID *uuid.UUID) error {
	return nil
}

func (s *NoopScheduler) HandleWorkRejected(ctx context.Context, agentID, runID uuid.UUID, shardID *uuid.UUID, reason string) error {
	return nil
}

func (s *NoopScheduler) HandleRunComplete(ctx context.Context, agentID, runID uuid.UUID, shardID *uuid.UUID, result *conductorv1.RunComplete) error {
	return nil
}

// ScheduleRun implements server.RunScheduler as a no-op.
// Returns nil, nil to indicate no run was scheduled (webhook will log but continue).
func (s *NoopScheduler) ScheduleRun(ctx context.Context, req server.ScheduleRunRequest) (*database.TestRun, error) {
	// No-op: Return nil to indicate scheduling is not enabled
	return nil, nil
}

// NoopGitSyncer implements server.GitSyncer as a no-op for initial setup.
// TODO: Replace with real git syncer when GitHub token is configured.
type NoopGitSyncer struct{}

func (s *NoopGitSyncer) SyncService(ctx context.Context, service *database.Service, branch string) (*server.SyncResult, error) {
	return &server.SyncResult{SyncedAt: time.Now()}, nil
}

// GitSyncerAdapter adapts git.Syncer to server.GitSyncer interface.
type GitSyncerAdapter struct {
	syncer interface {
		SyncService(ctx context.Context, service *database.Service, branch string) (*server.SyncResult, error)
	}
}

// NewGitSyncerAdapter creates a new adapter for the git syncer.
func NewGitSyncerAdapter(syncer interface {
	SyncService(ctx context.Context, service *database.Service, branch string) (*server.SyncResult, error)
}) *GitSyncerAdapter {
	return &GitSyncerAdapter{syncer: syncer}
}

// SyncService delegates to the underlying syncer.
func (a *GitSyncerAdapter) SyncService(ctx context.Context, service *database.Service, branch string) (*server.SyncResult, error) {
	return a.syncer.SyncService(ctx, service, branch)
}

// NoopArtifactStorage implements server.ArtifactStorage as a no-op.
// TODO: Replace with S3/MinIO implementation.
type NoopArtifactStorage struct{}

func (s *NoopArtifactStorage) GenerateDownloadURL(ctx context.Context, path string, expirationSeconds int) (string, time.Time, error) {
	return "", time.Now().Add(time.Hour), nil
}

// ArtifactStorageAdapter adapts artifact.Storage to server.ArtifactStorage interface.
type ArtifactStorageAdapter struct {
	storage *artifact.Storage
}

// NewArtifactStorageAdapter creates a new adapter for artifact storage.
func NewArtifactStorageAdapter(storage *artifact.Storage) *ArtifactStorageAdapter {
	return &ArtifactStorageAdapter{storage: storage}
}

// GenerateDownloadURL generates a presigned download URL for an artifact.
func (a *ArtifactStorageAdapter) GenerateDownloadURL(ctx context.Context, path string, expirationSeconds int) (string, time.Time, error) {
	expiration := time.Duration(expirationSeconds) * time.Second
	if expirationSeconds <= 0 {
		expiration = time.Hour // Default 1 hour
	}

	url, err := a.storage.GetPresignedURL(ctx, path, expiration)
	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().Add(expiration)
	return url, expiresAt, nil
}
