// Package scheduler provides test run scheduling and execution coordination.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// ScheduleRequest contains parameters for scheduling a new test run.
type ScheduleRequest struct {
	ServiceID   uuid.UUID
	GitRef      string
	GitSHA      string
	TriggerType database.TriggerType
	TriggeredBy string
	Priority    int
	TestIDs     []uuid.UUID // Optional: specific tests to run
	Tags        []string    // Optional: filter tests by tags
}

// AgentManager defines the interface for agent management operations.
type AgentManager interface {
	GetAvailableAgents(ctx context.Context, zones []string) ([]*AgentInfo, error)
	AssignWork(ctx context.Context, agentID uuid.UUID, run *database.TestRun) error
	CancelWork(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, reason string) error
}

// AgentInfo contains information about an available agent.
type AgentInfo struct {
	ID              uuid.UUID
	Name            string
	NetworkZones    []string
	MaxParallel     int
	ActiveRuns      int
	DockerAvailable bool
	LastHeartbeat   time.Time
}

// AvailableSlots returns the number of additional runs this agent can accept.
func (a *AgentInfo) AvailableSlots() int {
	return a.MaxParallel - a.ActiveRuns
}

// SchedulerService defines the interface for the test scheduling service.
type SchedulerService interface {
	// ScheduleRun creates a new test run and queues it for execution.
	ScheduleRun(ctx context.Context, req ScheduleRequest) (*database.TestRun, error)

	// CancelRun cancels a pending or running test run.
	CancelRun(ctx context.Context, runID uuid.UUID, reason string) error

	// RetryRun creates a new run with the same parameters as a failed run.
	RetryRun(ctx context.Context, runID uuid.UUID) (*database.TestRun, error)

	// ProcessQueue runs the background worker that assigns work to agents.
	ProcessQueue(ctx context.Context) error

	// Start begins the background queue processing.
	Start(ctx context.Context) error

	// Stop gracefully stops the scheduler.
	Stop(ctx context.Context) error
}

// Scheduler implements the SchedulerService interface.
type Scheduler struct {
	runRepo     database.TestRunRepository
	serviceRepo database.ServiceRepository
	agentRepo   database.AgentRepository
	agentMgr    AgentManager
	queue       *Queue
	logger      *slog.Logger

	mu           sync.RWMutex
	running      bool
	stopCh       chan struct{}
	wg           sync.WaitGroup
	pollInterval time.Duration
	batchSize    int
	maxRetries   int
	retryDelay   time.Duration
}

// Config holds configuration for the scheduler.
type Config struct {
	PollInterval time.Duration
	BatchSize    int
	MaxRetries   int
	RetryDelay   time.Duration
}

// DefaultConfig returns the default scheduler configuration.
func DefaultConfig() Config {
	return Config{
		PollInterval: 5 * time.Second,
		BatchSize:    10,
		MaxRetries:   3,
		RetryDelay:   5 * time.Second,
	}
}

// NewScheduler creates a new Scheduler instance.
func NewScheduler(
	runRepo database.TestRunRepository,
	serviceRepo database.ServiceRepository,
	agentRepo database.AgentRepository,
	agentMgr AgentManager,
	queue *Queue,
	logger *slog.Logger,
	cfg Config,
) *Scheduler {
	if cfg.PollInterval == 0 {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Scheduler{
		runRepo:      runRepo,
		serviceRepo:  serviceRepo,
		agentRepo:    agentRepo,
		agentMgr:     agentMgr,
		queue:        queue,
		logger:       logger.With("component", "scheduler"),
		stopCh:       make(chan struct{}),
		pollInterval: cfg.PollInterval,
		batchSize:    cfg.BatchSize,
		maxRetries:   cfg.MaxRetries,
		retryDelay:   cfg.RetryDelay,
	}
}

// ScheduleRun creates a new test run and queues it for execution.
func (s *Scheduler) ScheduleRun(ctx context.Context, req ScheduleRequest) (*database.TestRun, error) {
	// Validate service exists
	service, err := s.serviceRepo.Get(ctx, req.ServiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	if service == nil {
		return nil, fmt.Errorf("service not found: %s", req.ServiceID)
	}

	// Create the test run record
	run := &database.TestRun{
		ID:          uuid.New(),
		ServiceID:   req.ServiceID,
		Status:      database.RunStatusPending,
		GitRef:      database.NullString(req.GitRef),
		GitSHA:      database.NullString(req.GitSHA),
		TriggerType: &req.TriggerType,
		TriggeredBy: database.NullString(req.TriggeredBy),
		Priority:    req.Priority,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create test run: %w", err)
	}

	s.logger.Info("created test run",
		"run_id", run.ID,
		"service_id", run.ServiceID,
		"priority", run.Priority,
	)

	// Queue the run for processing
	workItem := &WorkItem{
		RunID:        run.ID,
		ServiceID:    run.ServiceID,
		Priority:     run.Priority,
		NetworkZones: service.NetworkZones,
		CreatedAt:    run.CreatedAt,
	}

	if err := s.queue.Push(ctx, workItem); err != nil {
		// Run is created but not queued - update status to error
		_ = s.runRepo.UpdateStatus(ctx, run.ID, database.RunStatusError)
		return nil, fmt.Errorf("failed to queue test run: %w", err)
	}

	return run, nil
}

// CancelRun cancels a pending or running test run.
func (s *Scheduler) CancelRun(ctx context.Context, runID uuid.UUID, reason string) error {
	run, err := s.runRepo.Get(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found: %s", runID)
	}

	// Check if run can be cancelled
	if run.IsTerminal() {
		return fmt.Errorf("run %s is already in terminal state: %s", runID, run.Status)
	}

	s.logger.Info("cancelling run",
		"run_id", runID,
		"current_status", run.Status,
		"reason", reason,
	)

	// If pending, just remove from queue and update status
	if run.Status == database.RunStatusPending {
		if err := s.queue.Remove(ctx, runID); err != nil {
			s.logger.Warn("failed to remove run from queue",
				"run_id", runID,
				"error", err,
			)
		}
		return s.runRepo.UpdateStatus(ctx, runID, database.RunStatusCancelled)
	}

	// If running, notify the agent to cancel
	if run.Status == database.RunStatusRunning && run.AgentID != nil {
		if err := s.agentMgr.CancelWork(ctx, *run.AgentID, runID, reason); err != nil {
			s.logger.Error("failed to send cancel to agent",
				"run_id", runID,
				"agent_id", run.AgentID,
				"error", err,
			)
			// Continue to update status even if agent notification fails
		}
	}

	return s.runRepo.UpdateStatus(ctx, runID, database.RunStatusCancelled)
}

// RetryRun creates a new run with the same parameters as a failed run.
func (s *Scheduler) RetryRun(ctx context.Context, runID uuid.UUID) (*database.TestRun, error) {
	original, err := s.runRepo.Get(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get original run: %w", err)
	}
	if original == nil {
		return nil, fmt.Errorf("run not found: %s", runID)
	}

	// Only allow retrying terminal runs
	if !original.IsTerminal() {
		return nil, fmt.Errorf("can only retry terminal runs, current status: %s", original.Status)
	}

	// Build retry request from original run
	triggerType := database.TriggerTypeManual
	if original.TriggerType != nil {
		triggerType = *original.TriggerType
	}

	req := ScheduleRequest{
		ServiceID:   original.ServiceID,
		TriggerType: triggerType,
		Priority:    original.Priority,
	}

	if original.GitRef != nil {
		req.GitRef = *original.GitRef
	}
	if original.GitSHA != nil {
		req.GitSHA = *original.GitSHA
	}
	if original.TriggeredBy != nil {
		req.TriggeredBy = *original.TriggeredBy
	}

	s.logger.Info("retrying run",
		"original_run_id", runID,
		"service_id", original.ServiceID,
	)

	return s.ScheduleRun(ctx, req)
}

// Start begins the background queue processing.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.processLoop(ctx)
	}()

	s.logger.Info("scheduler started",
		"poll_interval", s.pollInterval,
		"batch_size", s.batchSize,
	)

	return nil
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()

	// Wait for goroutines with context timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("scheduler stopped gracefully")
		return nil
	case <-ctx.Done():
		s.logger.Warn("scheduler stop timed out")
		return ctx.Err()
	}
}

// processLoop is the main background processing loop.
func (s *Scheduler) processLoop(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ProcessQueue(ctx); err != nil {
				s.logger.Error("queue processing error", "error", err)
			}
		}
	}
}

// ProcessQueue runs a single iteration of queue processing.
func (s *Scheduler) ProcessQueue(ctx context.Context) error {
	// Get pending work items
	items, err := s.queue.PeekBatch(ctx, s.batchSize)
	if err != nil {
		return fmt.Errorf("failed to peek queue: %w", err)
	}

	if len(items) == 0 {
		return nil
	}

	s.logger.Debug("processing queue", "items", len(items))

	// Process each item
	for _, item := range items {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := s.processWorkItem(ctx, item); err != nil {
			s.logger.Warn("failed to process work item",
				"run_id", item.RunID,
				"error", err,
			)
			// Continue processing other items
		}
	}

	return nil
}

// processWorkItem attempts to assign a single work item to an agent.
func (s *Scheduler) processWorkItem(ctx context.Context, item *WorkItem) error {
	// Get available agents for this work item's network zones
	agents, err := s.agentMgr.GetAvailableAgents(ctx, item.NetworkZones)
	if err != nil {
		return fmt.Errorf("failed to get available agents: %w", err)
	}

	if len(agents) == 0 {
		s.logger.Debug("no available agents",
			"run_id", item.RunID,
			"zones", item.NetworkZones,
		)
		return nil // No agents available, will retry next cycle
	}

	// Get the full run record
	run, err := s.runRepo.Get(ctx, item.RunID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}
	if run == nil {
		// Run was deleted, remove from queue
		return s.queue.Remove(ctx, item.RunID)
	}

	// Verify run is still pending
	if run.Status != database.RunStatusPending {
		s.logger.Debug("run no longer pending, removing from queue",
			"run_id", item.RunID,
			"status", run.Status,
		)
		return s.queue.Remove(ctx, item.RunID)
	}

	// Match run to best agent
	agent := s.matchAgent(run, agents)
	if agent == nil {
		s.logger.Debug("no suitable agent found", "run_id", item.RunID)
		return nil
	}

	// Assign work to agent
	if err := s.agentMgr.AssignWork(ctx, agent.ID, run); err != nil {
		return fmt.Errorf("failed to assign work to agent %s: %w", agent.ID, err)
	}

	// Update run status and agent assignment
	if err := s.runRepo.Start(ctx, run.ID, agent.ID); err != nil {
		// Work was assigned but we couldn't update the database
		// Agent will handle this case
		return fmt.Errorf("failed to start run: %w", err)
	}

	// Remove from queue
	if err := s.queue.Remove(ctx, item.RunID); err != nil {
		s.logger.Warn("failed to remove run from queue after assignment",
			"run_id", item.RunID,
			"error", err,
		)
	}

	s.logger.Info("assigned run to agent",
		"run_id", run.ID,
		"agent_id", agent.ID,
		"agent_name", agent.Name,
	)

	return nil
}

// matchAgent selects the best agent for a given run.
// Selection criteria:
// 1. Agent must have network zone overlap with service
// 2. Agent must have available capacity
// 3. Prefer agents with more available slots (load balancing)
// 4. Prefer agents with most recent heartbeat (healthiest)
func (s *Scheduler) matchAgent(run *database.TestRun, agents []*AgentInfo) *AgentInfo {
	if len(agents) == 0 {
		return nil
	}

	var best *AgentInfo
	var bestScore int

	for _, agent := range agents {
		// Skip agents with no capacity
		if agent.AvailableSlots() <= 0 {
			continue
		}

		// Calculate score - higher is better
		score := s.calculateAgentScore(agent)

		if best == nil || score > bestScore {
			best = agent
			bestScore = score
		}
	}

	return best
}

// calculateAgentScore computes a score for agent selection.
// Higher score = better candidate.
func (s *Scheduler) calculateAgentScore(agent *AgentInfo) int {
	score := 0

	// Favor agents with more available capacity
	score += agent.AvailableSlots() * 100

	// Favor agents with more recent heartbeats (healthier)
	heartbeatAge := time.Since(agent.LastHeartbeat)
	if heartbeatAge < 10*time.Second {
		score += 50
	} else if heartbeatAge < 30*time.Second {
		score += 25
	} else if heartbeatAge < 60*time.Second {
		score += 10
	}

	// Slightly favor agents with docker available (more flexible)
	if agent.DockerAvailable {
		score += 5
	}

	return score
}
