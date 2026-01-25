// Package agentmgr provides agent lifecycle management and work distribution.
package agentmgr

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// AgentManager defines the interface for agent management operations.
type AgentManager interface {
	// RegisterAgent registers a new agent connection.
	RegisterAgent(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error)

	// HandleHeartbeat processes a heartbeat from an agent.
	HandleHeartbeat(ctx context.Context, agentID uuid.UUID, status *HeartbeatStatus) error

	// AssignWork dispatches a test run to an agent.
	AssignWork(ctx context.Context, agentID uuid.UUID, run *database.TestRun, shard *database.RunShard, tests []database.TestDefinition) error

	// CancelWork sends a cancellation request to an agent.
	CancelWork(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, reason string) error

	// DrainAgent marks an agent for draining (no new work).
	DrainAgent(ctx context.Context, agentID uuid.UUID, reason string) error

	// UndrainAgent removes the draining state from an agent.
	UndrainAgent(ctx context.Context, agentID uuid.UUID) error

	// GetAvailableAgents returns agents available to run work in the given zones.
	GetAvailableAgents(ctx context.Context, zones []string) ([]*AgentInfo, error)

	// GetAgent returns information about a specific agent.
	GetAgent(ctx context.Context, agentID uuid.UUID) (*AgentInfo, error)

	// RemoveAgent removes an agent from the registry.
	RemoveAgent(ctx context.Context, agentID uuid.UUID) error

	// Start begins background processes (heartbeat monitoring).
	Start(ctx context.Context) error

	// Stop gracefully stops the manager.
	Stop(ctx context.Context) error
}

// RegisterRequest contains agent registration information.
type RegisterRequest struct {
	AgentID         uuid.UUID
	Name            string
	Version         string
	NetworkZones    []string
	MaxParallel     int
	DockerAvailable bool
	OS              string
	Arch            string
	Hostname        string
	Labels          map[string]string
}

// RegisterResponse contains the result of agent registration.
type RegisterResponse struct {
	Success                  bool
	HeartbeatIntervalSeconds int
	ServerVersion            string
	ErrorMessage             string
}

// HeartbeatStatus contains agent status from a heartbeat.
type HeartbeatStatus struct {
	Status       database.AgentStatus
	ActiveRunIDs []uuid.UUID
	CPUPercent   float64
	MemoryBytes  int64
	DiskBytes    int64
}

// AgentInfo contains information about an available agent.
type AgentInfo struct {
	ID              uuid.UUID
	Name            string
	Status          database.AgentStatus
	Version         string
	NetworkZones    []string
	MaxParallel     int
	ActiveRuns      int
	DockerAvailable bool
	LastHeartbeat   time.Time
	OS              string
	Arch            string
	Hostname        string
}

// AvailableSlots returns the number of additional runs this agent can accept.
func (a *AgentInfo) AvailableSlots() int {
	return a.MaxParallel - a.ActiveRuns
}

// WorkAssignment contains information about work to assign to an agent.
type WorkAssignment struct {
	Run         *database.TestRun
	Shard       *database.RunShard
	Service     *database.Service
	Tests       []database.TestDefinition
	Environment map[string]string
}

// Manager implements the AgentManager interface.
type Manager struct {
	agentRepo   database.AgentRepository
	serviceRepo database.ServiceRepository
	testRepo    database.TestDefinitionRepository
	runRepo     database.TestRunRepository
	logger      *slog.Logger

	mu          sync.RWMutex
	connections map[uuid.UUID]*AgentConnection
	running     bool
	stopCh      chan struct{}
	wg          sync.WaitGroup

	heartbeatTimeout time.Duration
	checkInterval    time.Duration
	serverVersion    string
}

// ManagerConfig holds configuration for the agent manager.
type ManagerConfig struct {
	HeartbeatTimeout time.Duration
	CheckInterval    time.Duration
	ServerVersion    string
}

// DefaultManagerConfig returns the default manager configuration.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		HeartbeatTimeout: 90 * time.Second,
		CheckInterval:    30 * time.Second,
		ServerVersion:    "1.0.0",
	}
}

// NewManager creates a new agent Manager.
func NewManager(
	agentRepo database.AgentRepository,
	serviceRepo database.ServiceRepository,
	testRepo database.TestDefinitionRepository,
	runRepo database.TestRunRepository,
	logger *slog.Logger,
	cfg ManagerConfig,
) *Manager {
	if cfg.HeartbeatTimeout == 0 {
		cfg = DefaultManagerConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Manager{
		agentRepo:        agentRepo,
		serviceRepo:      serviceRepo,
		testRepo:         testRepo,
		runRepo:          runRepo,
		logger:           logger.With("component", "agent_manager"),
		connections:      make(map[uuid.UUID]*AgentConnection),
		stopCh:           make(chan struct{}),
		heartbeatTimeout: cfg.HeartbeatTimeout,
		checkInterval:    cfg.CheckInterval,
		serverVersion:    cfg.ServerVersion,
	}
}

// RegisterAgent registers a new agent connection.
func (m *Manager) RegisterAgent(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("registering agent",
		"agent_id", req.AgentID,
		"name", req.Name,
		"version", req.Version,
		"zones", req.NetworkZones,
	)

	// Check if agent already exists
	existing, err := m.agentRepo.Get(ctx, req.AgentID)
	if err != nil {
		return &RegisterResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to check existing agent: %v", err),
		}, nil
	}

	now := time.Now().UTC()

	if existing != nil {
		// Update existing agent
		existing.Name = req.Name
		existing.Version = &req.Version
		existing.NetworkZones = req.NetworkZones
		existing.MaxParallel = req.MaxParallel
		existing.DockerAvailable = req.DockerAvailable
		existing.Status = database.AgentStatusIdle
		existing.LastHeartbeat = &now

		if err := m.agentRepo.Update(ctx, existing); err != nil {
			return &RegisterResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("failed to update agent: %v", err),
			}, nil
		}
	} else {
		// Create new agent
		agent := &database.Agent{
			ID:              req.AgentID,
			Name:            req.Name,
			Status:          database.AgentStatusIdle,
			Version:         &req.Version,
			NetworkZones:    req.NetworkZones,
			MaxParallel:     req.MaxParallel,
			DockerAvailable: req.DockerAvailable,
			LastHeartbeat:   &now,
			RegisteredAt:    now,
		}

		if err := m.agentRepo.Create(ctx, agent); err != nil {
			return &RegisterResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("failed to create agent: %v", err),
			}, nil
		}
	}

	return &RegisterResponse{
		Success:                  true,
		HeartbeatIntervalSeconds: int(m.heartbeatTimeout.Seconds() / 3),
		ServerVersion:            m.serverVersion,
	}, nil
}

// HandleHeartbeat processes a heartbeat from an agent.
func (m *Manager) HandleHeartbeat(ctx context.Context, agentID uuid.UUID, status *HeartbeatStatus) error {
	m.logger.Debug("received heartbeat",
		"agent_id", agentID,
		"status", status.Status,
		"active_runs", len(status.ActiveRunIDs),
	)

	// Update heartbeat timestamp and status in database
	if err := m.agentRepo.UpdateHeartbeat(ctx, agentID, status.Status); err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	// Update connection state
	m.mu.Lock()
	if conn, ok := m.connections[agentID]; ok {
		conn.UpdateHeartbeat(status)
	}
	m.mu.Unlock()

	return nil
}

// AssignWork dispatches a test run to an agent.
func (m *Manager) AssignWork(ctx context.Context, agentID uuid.UUID, run *database.TestRun, shard *database.RunShard, tests []database.TestDefinition) error {
	m.mu.RLock()
	conn, ok := m.connections[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no connection to agent %s", agentID)
	}

	// Get service details
	service, err := m.serviceRepo.Get(ctx, run.ServiceID)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}
	if service == nil {
		return fmt.Errorf("service not found: %s", run.ServiceID)
	}

	if len(tests) == 0 {
		var err error
		tests, err = m.testRepo.ListByService(ctx, run.ServiceID, database.Pagination{Limit: 1000})
		if err != nil {
			return fmt.Errorf("failed to get test definitions: %w", err)
		}
	}

	// Build assignment
	assignment := &WorkAssignment{
		Run:     run,
		Shard:   shard,
		Service: service,
		Tests:   tests,
	}

	m.logger.Info("assigning work to agent",
		"agent_id", agentID,
		"run_id", run.ID,
		"shard_id", shardID(shard),
		"service_id", run.ServiceID,
		"test_count", len(tests),
	)

	return conn.SendAssignment(ctx, assignment)
}

// CancelWork sends a cancellation request to an agent.
func (m *Manager) CancelWork(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, reason string) error {
	m.mu.RLock()
	conn, ok := m.connections[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no connection to agent %s", agentID)
	}

	m.logger.Info("cancelling work on agent",
		"agent_id", agentID,
		"run_id", runID,
		"reason", reason,
	)

	return conn.SendCancel(ctx, runID, reason)
}

// DrainAgent marks an agent for draining.
func (m *Manager) DrainAgent(ctx context.Context, agentID uuid.UUID, reason string) error {
	m.logger.Info("draining agent",
		"agent_id", agentID,
		"reason", reason,
	)

	if err := m.agentRepo.UpdateStatus(ctx, agentID, database.AgentStatusDraining); err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}

	// Notify agent of drain
	m.mu.RLock()
	conn, ok := m.connections[agentID]
	m.mu.RUnlock()

	if ok {
		if err := conn.SendDrain(ctx, reason); err != nil {
			m.logger.Warn("failed to notify agent of drain",
				"agent_id", agentID,
				"error", err,
			)
		}
	}

	return nil
}

// UndrainAgent removes the draining state from an agent.
func (m *Manager) UndrainAgent(ctx context.Context, agentID uuid.UUID) error {
	m.logger.Info("undraining agent", "agent_id", agentID)

	// Get current agent state
	agent, err := m.agentRepo.Get(ctx, agentID)
	if err != nil {
		return fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	// Only undrain if currently draining
	if agent.Status != database.AgentStatusDraining {
		return fmt.Errorf("agent is not draining, current status: %s", agent.Status)
	}

	// Determine new status based on active runs
	newStatus := database.AgentStatusIdle
	m.mu.RLock()
	if conn, ok := m.connections[agentID]; ok && conn.ActiveRunCount() > 0 {
		newStatus = database.AgentStatusBusy
	}
	m.mu.RUnlock()

	return m.agentRepo.UpdateStatus(ctx, agentID, newStatus)
}

func shardID(shard *database.RunShard) string {
	if shard == nil {
		return ""
	}
	return shard.ID.String()
}

// GetAvailableAgents returns agents available to run work in the given zones.
func (m *Manager) GetAvailableAgents(ctx context.Context, zones []string) ([]*AgentInfo, error) {
	// Get available agents from database
	dbAgents, err := m.agentRepo.GetAvailable(ctx, zones, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get available agents: %w", err)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*AgentInfo, 0, len(dbAgents))
	for _, dbAgent := range dbAgents {
		// Only include agents with active connections
		conn, hasConn := m.connections[dbAgent.ID]
		if !hasConn {
			continue
		}

		info := &AgentInfo{
			ID:              dbAgent.ID,
			Name:            dbAgent.Name,
			Status:          dbAgent.Status,
			NetworkZones:    dbAgent.NetworkZones,
			MaxParallel:     dbAgent.MaxParallel,
			ActiveRuns:      conn.ActiveRunCount(),
			DockerAvailable: dbAgent.DockerAvailable,
		}

		if dbAgent.Version != nil {
			info.Version = *dbAgent.Version
		}
		if dbAgent.LastHeartbeat != nil {
			info.LastHeartbeat = *dbAgent.LastHeartbeat
		}

		// Only include if has capacity
		if info.AvailableSlots() > 0 {
			result = append(result, info)
		}
	}

	return result, nil
}

// GetAgent returns information about a specific agent.
func (m *Manager) GetAgent(ctx context.Context, agentID uuid.UUID) (*AgentInfo, error) {
	dbAgent, err := m.agentRepo.Get(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if dbAgent == nil {
		return nil, nil
	}

	info := &AgentInfo{
		ID:              dbAgent.ID,
		Name:            dbAgent.Name,
		Status:          dbAgent.Status,
		NetworkZones:    dbAgent.NetworkZones,
		MaxParallel:     dbAgent.MaxParallel,
		DockerAvailable: dbAgent.DockerAvailable,
	}

	if dbAgent.Version != nil {
		info.Version = *dbAgent.Version
	}
	if dbAgent.LastHeartbeat != nil {
		info.LastHeartbeat = *dbAgent.LastHeartbeat
	}

	m.mu.RLock()
	if conn, ok := m.connections[dbAgent.ID]; ok {
		info.ActiveRuns = conn.ActiveRunCount()
	}
	m.mu.RUnlock()

	return info, nil
}

// RemoveAgent removes an agent from the registry.
func (m *Manager) RemoveAgent(ctx context.Context, agentID uuid.UUID) error {
	m.logger.Info("removing agent", "agent_id", agentID)

	// Close connection if exists
	m.mu.Lock()
	if conn, ok := m.connections[agentID]; ok {
		conn.Close()
		delete(m.connections, agentID)
	}
	m.mu.Unlock()

	return m.agentRepo.Delete(ctx, agentID)
}

// AddConnection adds an agent connection to the manager.
func (m *Manager) AddConnection(agentID uuid.UUID, conn *AgentConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close existing connection if any
	if existing, ok := m.connections[agentID]; ok {
		existing.Close()
	}

	m.connections[agentID] = conn
	m.logger.Info("agent connection added", "agent_id", agentID)
}

// RemoveConnection removes an agent connection from the manager.
func (m *Manager) RemoveConnection(agentID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, ok := m.connections[agentID]; ok {
		conn.Close()
		delete(m.connections, agentID)
		m.logger.Info("agent connection removed", "agent_id", agentID)
	}
}

// Start begins background processes.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("manager already running")
	}
	m.running = true
	m.stopCh = make(chan struct{})
	m.mu.Unlock()

	// Start heartbeat timeout checker
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.heartbeatChecker(ctx)
	}()

	m.logger.Info("agent manager started",
		"heartbeat_timeout", m.heartbeatTimeout,
		"check_interval", m.checkInterval,
	)

	return nil
}

// Stop gracefully stops the manager.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()

	// Close all connections
	m.mu.Lock()
	for id, conn := range m.connections {
		conn.Close()
		delete(m.connections, id)
	}
	m.mu.Unlock()

	// Wait for goroutines
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("agent manager stopped gracefully")
		return nil
	case <-ctx.Done():
		m.logger.Warn("agent manager stop timed out")
		return ctx.Err()
	}
}

// heartbeatChecker runs in the background to detect offline agents.
func (m *Manager) heartbeatChecker(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkHeartbeatTimeouts(ctx)
		}
	}
}

// checkHeartbeatTimeouts marks agents as offline if they've missed heartbeats.
func (m *Manager) checkHeartbeatTimeouts(ctx context.Context) {
	count, err := m.agentRepo.MarkOfflineAgents(ctx)
	if err != nil {
		m.logger.Error("failed to mark offline agents", "error", err)
		return
	}

	if count > 0 {
		m.logger.Info("marked agents as offline", "count", count)
	}

	// Also check connections
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, conn := range m.connections {
		if time.Since(conn.LastHeartbeat()) > m.heartbeatTimeout {
			m.logger.Warn("agent connection timed out", "agent_id", id)
			conn.Close()
			delete(m.connections, id)
		}
	}
}

// ConnectionCount returns the number of active agent connections.
func (m *Manager) ConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}
