package agentmgr

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// Mock repositories for testing

type mockAgentRepository struct {
	mu            sync.RWMutex
	agents        map[uuid.UUID]*database.Agent
	createErr     error
	updateErr     error
	getErr        error
	deleteErr     error
	heartbeatErr  error
	statusErr     error
	availableErr  error
	countByStatus map[database.AgentStatus]int64
}

func newMockAgentRepository() *mockAgentRepository {
	return &mockAgentRepository{
		agents:        make(map[uuid.UUID]*database.Agent),
		countByStatus: make(map[database.AgentStatus]int64),
	}
}

func (m *mockAgentRepository) Create(ctx context.Context, agent *database.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentRepository) Update(ctx context.Context, agent *database.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateErr != nil {
		return m.updateErr
	}
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentRepository) Get(ctx context.Context, id uuid.UUID) (*database.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.agents[id], nil
}

func (m *mockAgentRepository) GetByName(ctx context.Context, name string) (*database.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.agents {
		if a.Name == name {
			return a, nil
		}
	}
	return nil, nil
}

func (m *mockAgentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.agents, id)
	return nil
}

func (m *mockAgentRepository) List(ctx context.Context, pagination database.Pagination) ([]database.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var agents []database.Agent
	for _, a := range m.agents {
		agents = append(agents, *a)
	}
	return agents, nil
}

func (m *mockAgentRepository) ListByStatus(ctx context.Context, status database.AgentStatus, pagination database.Pagination) ([]database.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var agents []database.Agent
	for _, a := range m.agents {
		if a.Status == status {
			agents = append(agents, *a)
		}
	}
	return agents, nil
}

func (m *mockAgentRepository) UpdateHeartbeat(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.heartbeatErr != nil {
		return m.heartbeatErr
	}
	if a, ok := m.agents[id]; ok {
		now := time.Now()
		a.LastHeartbeat = &now
		a.Status = status
	}
	return nil
}

func (m *mockAgentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statusErr != nil {
		return m.statusErr
	}
	if a, ok := m.agents[id]; ok {
		a.Status = status
	}
	return nil
}

func (m *mockAgentRepository) GetAvailable(ctx context.Context, zones []string, limit int) ([]database.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.availableErr != nil {
		return nil, m.availableErr
	}
	var agents []database.Agent
	for _, a := range m.agents {
		if a.Status == database.AgentStatusIdle || a.Status == database.AgentStatusBusy {
			agents = append(agents, *a)
		}
	}
	return agents, nil
}

func (m *mockAgentRepository) CountByStatus(ctx context.Context) (map[database.AgentStatus]int64, error) {
	return m.countByStatus, nil
}

func (m *mockAgentRepository) MarkOfflineAgents(ctx context.Context) (int64, error) {
	return 0, nil
}

type mockServiceRepository struct {
	services map[uuid.UUID]*database.Service
	getErr   error
}

func newMockServiceRepository() *mockServiceRepository {
	return &mockServiceRepository{
		services: make(map[uuid.UUID]*database.Service),
	}
}

func (m *mockServiceRepository) Create(ctx context.Context, svc *database.Service) error {
	m.services[svc.ID] = svc
	return nil
}
func (m *mockServiceRepository) Get(ctx context.Context, id uuid.UUID) (*database.Service, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.services[id], nil
}
func (m *mockServiceRepository) GetByName(ctx context.Context, name string) (*database.Service, error) {
	return nil, nil
}
func (m *mockServiceRepository) Update(ctx context.Context, svc *database.Service) error { return nil }
func (m *mockServiceRepository) Delete(ctx context.Context, id uuid.UUID) error          { return nil }
func (m *mockServiceRepository) List(ctx context.Context, p database.Pagination) ([]database.Service, error) {
	return nil, nil
}
func (m *mockServiceRepository) Count(ctx context.Context) (int64, error) { return 0, nil }
func (m *mockServiceRepository) ListByOwner(ctx context.Context, owner string, p database.Pagination) ([]database.Service, error) {
	return nil, nil
}
func (m *mockServiceRepository) Search(ctx context.Context, query string, p database.Pagination) ([]database.Service, error) {
	return nil, nil
}

type mockTestDefinitionRepository struct {
	tests map[uuid.UUID][]database.TestDefinition
}

func newMockTestDefinitionRepository() *mockTestDefinitionRepository {
	return &mockTestDefinitionRepository{
		tests: make(map[uuid.UUID][]database.TestDefinition),
	}
}

func (m *mockTestDefinitionRepository) Create(ctx context.Context, def *database.TestDefinition) error {
	m.tests[def.ServiceID] = append(m.tests[def.ServiceID], *def)
	return nil
}
func (m *mockTestDefinitionRepository) Get(ctx context.Context, id uuid.UUID) (*database.TestDefinition, error) {
	return nil, nil
}
func (m *mockTestDefinitionRepository) Update(ctx context.Context, def *database.TestDefinition) error {
	return nil
}
func (m *mockTestDefinitionRepository) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockTestDefinitionRepository) ListByService(ctx context.Context, serviceID uuid.UUID, p database.Pagination) ([]database.TestDefinition, error) {
	return m.tests[serviceID], nil
}
func (m *mockTestDefinitionRepository) ListByTags(ctx context.Context, serviceID uuid.UUID, tags []string, p database.Pagination) ([]database.TestDefinition, error) {
	return m.tests[serviceID], nil
}

type mockTestRunRepository struct {
	runs map[uuid.UUID]*database.TestRun
}

func newMockTestRunRepository() *mockTestRunRepository {
	return &mockTestRunRepository{
		runs: make(map[uuid.UUID]*database.TestRun),
	}
}

func (m *mockTestRunRepository) Create(ctx context.Context, run *database.TestRun) error {
	m.runs[run.ID] = run
	return nil
}
func (m *mockTestRunRepository) Get(ctx context.Context, id uuid.UUID) (*database.TestRun, error) {
	return m.runs[id], nil
}
func (m *mockTestRunRepository) Update(ctx context.Context, run *database.TestRun) error { return nil }
func (m *mockTestRunRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status database.RunStatus) error {
	return nil
}
func (m *mockTestRunRepository) Start(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error {
	return nil
}
func (m *mockTestRunRepository) Finish(ctx context.Context, id uuid.UUID, status database.RunStatus, results database.RunResults) error {
	return nil
}
func (m *mockTestRunRepository) UpdateShardStats(ctx context.Context, id uuid.UUID, shardCount int, shardsFailed int, results database.RunResults) error {
	return nil
}
func (m *mockTestRunRepository) List(ctx context.Context, p database.Pagination) ([]database.TestRun, error) {
	return nil, nil
}
func (m *mockTestRunRepository) ListByService(ctx context.Context, serviceID uuid.UUID, p database.Pagination) ([]database.TestRun, error) {
	return nil, nil
}
func (m *mockTestRunRepository) ListByStatus(ctx context.Context, status database.RunStatus, p database.Pagination) ([]database.TestRun, error) {
	return nil, nil
}
func (m *mockTestRunRepository) ListByServiceAndStatus(ctx context.Context, serviceID uuid.UUID, status database.RunStatus, p database.Pagination) ([]database.TestRun, error) {
	return nil, nil
}
func (m *mockTestRunRepository) ListByDateRange(ctx context.Context, start, end time.Time, p database.Pagination) ([]database.TestRun, error) {
	return nil, nil
}
func (m *mockTestRunRepository) GetPending(ctx context.Context, limit int) ([]database.TestRun, error) {
	return nil, nil
}
func (m *mockTestRunRepository) GetRunning(ctx context.Context) ([]database.TestRun, error) {
	return nil, nil
}
func (m *mockTestRunRepository) Count(ctx context.Context) (int64, error) { return 0, nil }
func (m *mockTestRunRepository) CountByStatus(ctx context.Context) (map[database.RunStatus]int64, error) {
	return nil, nil
}

// Tests

func TestNewManager(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()
	logger := slog.Default()
	cfg := DefaultManagerConfig()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, logger, cfg)

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
	if mgr.heartbeatTimeout != cfg.HeartbeatTimeout {
		t.Errorf("heartbeatTimeout = %v, want %v", mgr.heartbeatTimeout, cfg.HeartbeatTimeout)
	}
	if mgr.serverVersion != cfg.ServerVersion {
		t.Errorf("serverVersion = %q, want %q", mgr.serverVersion, cfg.ServerVersion)
	}
}

func TestDefaultManagerConfig(t *testing.T) {
	cfg := DefaultManagerConfig()

	if cfg.HeartbeatTimeout != 90*time.Second {
		t.Errorf("HeartbeatTimeout = %v, want %v", cfg.HeartbeatTimeout, 90*time.Second)
	}
	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval = %v, want %v", cfg.CheckInterval, 30*time.Second)
	}
	if cfg.ServerVersion != "1.0.0" {
		t.Errorf("ServerVersion = %q, want %q", cfg.ServerVersion, "1.0.0")
	}
}

func TestManager_RegisterAgent_New(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), DefaultManagerConfig())

	req := &RegisterRequest{
		AgentID:         uuid.New(),
		Name:            "test-agent",
		Version:         "1.0.0",
		NetworkZones:    []string{"zone-1"},
		MaxParallel:     4,
		DockerAvailable: true,
		OS:              "linux",
		Arch:            "amd64",
		Hostname:        "test-host",
	}

	resp, err := mgr.RegisterAgent(context.Background(), req)
	if err != nil {
		t.Fatalf("RegisterAgent() error = %v", err)
	}
	if !resp.Success {
		t.Errorf("RegisterAgent() Success = false, ErrorMessage = %q", resp.ErrorMessage)
	}
	if resp.HeartbeatIntervalSeconds <= 0 {
		t.Errorf("HeartbeatIntervalSeconds = %d, want > 0", resp.HeartbeatIntervalSeconds)
	}
	if resp.ServerVersion == "" {
		t.Error("ServerVersion is empty")
	}

	// Verify agent was created in repo
	agent, _ := agentRepo.Get(context.Background(), req.AgentID)
	if agent == nil {
		t.Error("Agent was not created in repository")
	}
	if agent.Status != database.AgentStatusIdle {
		t.Errorf("Agent status = %v, want %v", agent.Status, database.AgentStatusIdle)
	}
}

func TestManager_RegisterAgent_Existing(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), DefaultManagerConfig())

	// Pre-create agent
	agentID := uuid.New()
	existingAgent := &database.Agent{
		ID:     agentID,
		Name:   "old-name",
		Status: database.AgentStatusOffline,
	}
	agentRepo.agents[agentID] = existingAgent

	req := &RegisterRequest{
		AgentID:         agentID,
		Name:            "new-name",
		Version:         "2.0.0",
		NetworkZones:    []string{"zone-2"},
		MaxParallel:     8,
		DockerAvailable: false,
	}

	resp, err := mgr.RegisterAgent(context.Background(), req)
	if err != nil {
		t.Fatalf("RegisterAgent() error = %v", err)
	}
	if !resp.Success {
		t.Errorf("RegisterAgent() Success = false, ErrorMessage = %q", resp.ErrorMessage)
	}

	// Verify agent was updated
	agent, _ := agentRepo.Get(context.Background(), agentID)
	if agent.Name != "new-name" {
		t.Errorf("Agent name = %q, want %q", agent.Name, "new-name")
	}
	if agent.Status != database.AgentStatusIdle {
		t.Errorf("Agent status = %v, want %v", agent.Status, database.AgentStatusIdle)
	}
	if agent.MaxParallel != 8 {
		t.Errorf("MaxParallel = %d, want %d", agent.MaxParallel, 8)
	}
}

func TestManager_HandleHeartbeat(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), DefaultManagerConfig())

	agentID := uuid.New()
	agentRepo.agents[agentID] = &database.Agent{
		ID:     agentID,
		Status: database.AgentStatusIdle,
	}

	status := &HeartbeatStatus{
		Status:       database.AgentStatusBusy,
		ActiveRunIDs: []uuid.UUID{uuid.New()},
		CPUPercent:   50.0,
		MemoryBytes:  1024 * 1024 * 1024,
	}

	err := mgr.HandleHeartbeat(context.Background(), agentID, status)
	if err != nil {
		t.Errorf("HandleHeartbeat() error = %v", err)
	}

	// Verify status was updated
	agent, _ := agentRepo.Get(context.Background(), agentID)
	if agent.Status != database.AgentStatusBusy {
		t.Errorf("Agent status = %v, want %v", agent.Status, database.AgentStatusBusy)
	}
	if agent.LastHeartbeat == nil {
		t.Error("LastHeartbeat was not updated")
	}
}

func TestManager_DrainAgent(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), DefaultManagerConfig())

	agentID := uuid.New()
	agentRepo.agents[agentID] = &database.Agent{
		ID:     agentID,
		Status: database.AgentStatusIdle,
	}

	err := mgr.DrainAgent(context.Background(), agentID, "maintenance")
	if err != nil {
		t.Errorf("DrainAgent() error = %v", err)
	}

	// Verify status was updated
	agent, _ := agentRepo.Get(context.Background(), agentID)
	if agent.Status != database.AgentStatusDraining {
		t.Errorf("Agent status = %v, want %v", agent.Status, database.AgentStatusDraining)
	}
}

func TestManager_UndrainAgent(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), DefaultManagerConfig())

	agentID := uuid.New()
	agentRepo.agents[agentID] = &database.Agent{
		ID:     agentID,
		Status: database.AgentStatusDraining,
	}

	err := mgr.UndrainAgent(context.Background(), agentID)
	if err != nil {
		t.Errorf("UndrainAgent() error = %v", err)
	}

	// Verify status was updated to idle (no active runs)
	agent, _ := agentRepo.Get(context.Background(), agentID)
	if agent.Status != database.AgentStatusIdle {
		t.Errorf("Agent status = %v, want %v", agent.Status, database.AgentStatusIdle)
	}
}

func TestManager_UndrainAgent_NotDraining(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), DefaultManagerConfig())

	agentID := uuid.New()
	agentRepo.agents[agentID] = &database.Agent{
		ID:     agentID,
		Status: database.AgentStatusBusy,
	}

	err := mgr.UndrainAgent(context.Background(), agentID)
	if err == nil {
		t.Error("UndrainAgent() expected error for non-draining agent")
	}
}

func TestManager_GetAgent(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), DefaultManagerConfig())

	agentID := uuid.New()
	version := "1.0.0"
	now := time.Now()
	agentRepo.agents[agentID] = &database.Agent{
		ID:              agentID,
		Name:            "test-agent",
		Status:          database.AgentStatusIdle,
		Version:         &version,
		NetworkZones:    []string{"zone-1"},
		MaxParallel:     4,
		DockerAvailable: true,
		LastHeartbeat:   &now,
	}

	info, err := mgr.GetAgent(context.Background(), agentID)
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if info == nil {
		t.Fatal("GetAgent() returned nil")
	}
	if info.Name != "test-agent" {
		t.Errorf("Name = %q, want %q", info.Name, "test-agent")
	}
	if info.Status != database.AgentStatusIdle {
		t.Errorf("Status = %v, want %v", info.Status, database.AgentStatusIdle)
	}
	if info.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", info.Version, "1.0.0")
	}
	if info.MaxParallel != 4 {
		t.Errorf("MaxParallel = %d, want %d", info.MaxParallel, 4)
	}
}

func TestManager_GetAgent_NotFound(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), DefaultManagerConfig())

	info, err := mgr.GetAgent(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if info != nil {
		t.Error("GetAgent() should return nil for non-existent agent")
	}
}

func TestManager_RemoveAgent(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), DefaultManagerConfig())

	agentID := uuid.New()
	agentRepo.agents[agentID] = &database.Agent{
		ID:     agentID,
		Status: database.AgentStatusIdle,
	}

	err := mgr.RemoveAgent(context.Background(), agentID)
	if err != nil {
		t.Errorf("RemoveAgent() error = %v", err)
	}

	// Verify agent was deleted
	agent, _ := agentRepo.Get(context.Background(), agentID)
	if agent != nil {
		t.Error("Agent should have been deleted")
	}
}

func TestManager_StartStop(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	cfg := ManagerConfig{
		HeartbeatTimeout: 1 * time.Second,
		CheckInterval:    100 * time.Millisecond,
		ServerVersion:    "1.0.0",
	}

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), cfg)

	ctx := context.Background()

	// Start
	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Double start should error
	err = mgr.Start(ctx)
	if err == nil {
		t.Error("Start() should error when already running")
	}

	// Let the heartbeat checker run
	time.Sleep(200 * time.Millisecond)

	// Stop
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = mgr.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Double stop should be fine (idempotent)
	err = mgr.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop() second call error = %v", err)
	}
}

func TestManager_ConnectionCount(t *testing.T) {
	agentRepo := newMockAgentRepository()
	serviceRepo := newMockServiceRepository()
	testRepo := newMockTestDefinitionRepository()
	runRepo := newMockTestRunRepository()

	mgr := NewManager(agentRepo, serviceRepo, testRepo, runRepo, slog.Default(), DefaultManagerConfig())

	if mgr.ConnectionCount() != 0 {
		t.Errorf("ConnectionCount() = %d, want 0", mgr.ConnectionCount())
	}
}

func TestAgentInfo_AvailableSlots(t *testing.T) {
	tests := []struct {
		name        string
		maxParallel int
		activeRuns  int
		want        int
	}{
		{"no active runs", 4, 0, 4},
		{"some active runs", 4, 2, 2},
		{"at capacity", 4, 4, 0},
		{"over capacity", 4, 5, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &AgentInfo{
				MaxParallel: tt.maxParallel,
				ActiveRuns:  tt.activeRuns,
			}
			if got := info.AvailableSlots(); got != tt.want {
				t.Errorf("AvailableSlots() = %d, want %d", got, tt.want)
			}
		})
	}
}
