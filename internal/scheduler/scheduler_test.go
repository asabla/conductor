package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/conductor/conductor/internal/database"
)

// MockRunRepo is a mock implementation of database.TestRunRepository.
type MockRunRepo struct {
	mock.Mock
}

func (m *MockRunRepo) Create(ctx context.Context, run *database.TestRun) error {
	args := m.Called(ctx, run)
	return args.Error(0)
}

func (m *MockRunRepo) Get(ctx context.Context, id uuid.UUID) (*database.TestRun, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*database.TestRun), args.Error(1)
}

func (m *MockRunRepo) Update(ctx context.Context, run *database.TestRun) error {
	args := m.Called(ctx, run)
	return args.Error(0)
}

func (m *MockRunRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status database.RunStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockRunRepo) Start(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error {
	args := m.Called(ctx, id, agentID)
	return args.Error(0)
}

func (m *MockRunRepo) Finish(ctx context.Context, id uuid.UUID, status database.RunStatus, results database.RunResults) error {
	args := m.Called(ctx, id, status, results)
	return args.Error(0)
}

func (m *MockRunRepo) List(ctx context.Context, page database.Pagination) ([]database.TestRun, error) {
	args := m.Called(ctx, page)
	return args.Get(0).([]database.TestRun), args.Error(1)
}

func (m *MockRunRepo) ListByService(ctx context.Context, serviceID uuid.UUID, page database.Pagination) ([]database.TestRun, error) {
	args := m.Called(ctx, serviceID, page)
	return args.Get(0).([]database.TestRun), args.Error(1)
}

func (m *MockRunRepo) ListByStatus(ctx context.Context, status database.RunStatus, page database.Pagination) ([]database.TestRun, error) {
	args := m.Called(ctx, status, page)
	return args.Get(0).([]database.TestRun), args.Error(1)
}

func (m *MockRunRepo) ListByServiceAndStatus(ctx context.Context, serviceID uuid.UUID, status database.RunStatus, page database.Pagination) ([]database.TestRun, error) {
	args := m.Called(ctx, serviceID, status, page)
	return args.Get(0).([]database.TestRun), args.Error(1)
}

func (m *MockRunRepo) ListByDateRange(ctx context.Context, start, end time.Time, page database.Pagination) ([]database.TestRun, error) {
	args := m.Called(ctx, start, end, page)
	return args.Get(0).([]database.TestRun), args.Error(1)
}

func (m *MockRunRepo) GetPending(ctx context.Context, limit int) ([]database.TestRun, error) {
	args := m.Called(ctx, limit)
	return args.Get(0).([]database.TestRun), args.Error(1)
}

func (m *MockRunRepo) GetRunning(ctx context.Context) ([]database.TestRun, error) {
	args := m.Called(ctx)
	return args.Get(0).([]database.TestRun), args.Error(1)
}

func (m *MockRunRepo) Count(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRunRepo) CountByStatus(ctx context.Context) (map[database.RunStatus]int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(map[database.RunStatus]int64), args.Error(1)
}

// MockServiceRepo is a mock implementation of database.ServiceRepository.
type MockServiceRepo struct {
	mock.Mock
}

func (m *MockServiceRepo) Create(ctx context.Context, svc *database.Service) error {
	args := m.Called(ctx, svc)
	return args.Error(0)
}

func (m *MockServiceRepo) Get(ctx context.Context, id uuid.UUID) (*database.Service, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*database.Service), args.Error(1)
}

func (m *MockServiceRepo) GetByName(ctx context.Context, name string) (*database.Service, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*database.Service), args.Error(1)
}

func (m *MockServiceRepo) Update(ctx context.Context, svc *database.Service) error {
	args := m.Called(ctx, svc)
	return args.Error(0)
}

func (m *MockServiceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockServiceRepo) List(ctx context.Context, page database.Pagination) ([]database.Service, error) {
	args := m.Called(ctx, page)
	return args.Get(0).([]database.Service), args.Error(1)
}

func (m *MockServiceRepo) Count(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockServiceRepo) ListByOwner(ctx context.Context, owner string, page database.Pagination) ([]database.Service, error) {
	args := m.Called(ctx, owner, page)
	return args.Get(0).([]database.Service), args.Error(1)
}

func (m *MockServiceRepo) Search(ctx context.Context, query string, page database.Pagination) ([]database.Service, error) {
	args := m.Called(ctx, query, page)
	return args.Get(0).([]database.Service), args.Error(1)
}

// MockAgentRepo is a mock implementation of database.AgentRepository.
type MockAgentRepo struct {
	mock.Mock
}

func (m *MockAgentRepo) Create(ctx context.Context, agent *database.Agent) error {
	args := m.Called(ctx, agent)
	return args.Error(0)
}

func (m *MockAgentRepo) Get(ctx context.Context, id uuid.UUID) (*database.Agent, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*database.Agent), args.Error(1)
}

func (m *MockAgentRepo) GetByName(ctx context.Context, name string) (*database.Agent, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*database.Agent), args.Error(1)
}

func (m *MockAgentRepo) Update(ctx context.Context, agent *database.Agent) error {
	args := m.Called(ctx, agent)
	return args.Error(0)
}

func (m *MockAgentRepo) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockAgentRepo) List(ctx context.Context, page database.Pagination) ([]database.Agent, error) {
	args := m.Called(ctx, page)
	return args.Get(0).([]database.Agent), args.Error(1)
}

func (m *MockAgentRepo) ListByStatus(ctx context.Context, status database.AgentStatus, page database.Pagination) ([]database.Agent, error) {
	args := m.Called(ctx, status, page)
	return args.Get(0).([]database.Agent), args.Error(1)
}

func (m *MockAgentRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockAgentRepo) UpdateHeartbeat(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockAgentRepo) GetAvailable(ctx context.Context, zones []string, limit int) ([]database.Agent, error) {
	args := m.Called(ctx, zones, limit)
	return args.Get(0).([]database.Agent), args.Error(1)
}

func (m *MockAgentRepo) MarkOfflineAgents(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockAgentRepo) CountByStatus(ctx context.Context) (map[database.AgentStatus]int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(map[database.AgentStatus]int64), args.Error(1)
}

// MockAgentManager is a mock implementation of AgentManager.
type MockAgentManager struct {
	mock.Mock
}

func (m *MockAgentManager) GetAvailableAgents(ctx context.Context, zones []string) ([]*AgentInfo, error) {
	args := m.Called(ctx, zones)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*AgentInfo), args.Error(1)
}

func (m *MockAgentManager) AssignWork(ctx context.Context, agentID uuid.UUID, run *database.TestRun, shard *database.RunShard, tests []database.TestDefinition) error {
	args := m.Called(ctx, agentID, run, shard, tests)
	return args.Error(0)
}

func (m *MockAgentManager) CancelWork(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, reason string) error {
	args := m.Called(ctx, agentID, runID, reason)
	return args.Error(0)
}

// MockTestRepo is a mock implementation of TestDefinitionRepository.
type MockTestRepo struct {
	mock.Mock
}

func (m *MockTestRepo) Create(ctx context.Context, def *database.TestDefinition) error {
	args := m.Called(ctx, def)
	return args.Error(0)
}

func (m *MockTestRepo) Get(ctx context.Context, id uuid.UUID) (*database.TestDefinition, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*database.TestDefinition), args.Error(1)
}

func (m *MockTestRepo) Update(ctx context.Context, def *database.TestDefinition) error {
	args := m.Called(ctx, def)
	return args.Error(0)
}

func (m *MockTestRepo) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockTestRepo) ListByService(ctx context.Context, serviceID uuid.UUID, page database.Pagination) ([]database.TestDefinition, error) {
	args := m.Called(ctx, serviceID, page)
	return args.Get(0).([]database.TestDefinition), args.Error(1)
}

func (m *MockTestRepo) ListByTags(ctx context.Context, serviceID uuid.UUID, tags []string, page database.Pagination) ([]database.TestDefinition, error) {
	args := m.Called(ctx, serviceID, tags, page)
	return args.Get(0).([]database.TestDefinition), args.Error(1)
}

// MockRunShardRepo is a mock implementation of RunShardRepository.
type MockRunShardRepo struct {
	mock.Mock
}

func (m *MockRunShardRepo) Create(ctx context.Context, shard *database.RunShard) error {
	args := m.Called(ctx, shard)
	return args.Error(0)
}

func (m *MockRunShardRepo) Get(ctx context.Context, id uuid.UUID) (*database.RunShard, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*database.RunShard), args.Error(1)
}

func (m *MockRunShardRepo) ListByRun(ctx context.Context, runID uuid.UUID) ([]database.RunShard, error) {
	args := m.Called(ctx, runID)
	return args.Get(0).([]database.RunShard), args.Error(1)
}

func (m *MockRunShardRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status database.ShardStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockRunShardRepo) Start(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error {
	args := m.Called(ctx, id, agentID)
	return args.Error(0)
}

func (m *MockRunShardRepo) Finish(ctx context.Context, id uuid.UUID, status database.ShardStatus, results database.RunResults) error {
	args := m.Called(ctx, id, status, results)
	return args.Error(0)
}

func (m *MockRunShardRepo) DeleteByRun(ctx context.Context, runID uuid.UUID) error {
	args := m.Called(ctx, runID)
	return args.Error(0)
}

func TestScheduler_ScheduleRun(t *testing.T) {
	ctx := context.Background()
	serviceID := uuid.New()
	triggerType := database.TriggerTypeManual

	service := &database.Service{
		ID:           serviceID,
		Name:         "test-service",
		NetworkZones: []string{"zone-a"},
	}

	t.Run("successfully schedules a run", func(t *testing.T) {
		mockRunRepo := new(MockRunRepo)
		mockServiceRepo := new(MockServiceRepo)
		mockAgentRepo := new(MockAgentRepo)
		mockTestRepo := new(MockTestRepo)
		mockShardRepo := new(MockRunShardRepo)
		mockAgentMgr := new(MockAgentManager)
		queue := NewQueue(mockRunRepo)

		scheduler := NewScheduler(
			mockRunRepo,
			mockServiceRepo,
			mockAgentRepo,
			mockTestRepo,
			mockShardRepo,
			mockAgentMgr,
			queue,
			nil,
			DefaultConfig(),
		)

		// Setup expectations
		mockServiceRepo.On("Get", ctx, serviceID).Return(service, nil)
		mockRunRepo.On("Create", ctx, mock.AnythingOfType("*database.TestRun")).Return(nil)

		// Execute
		req := ScheduleRequest{
			ServiceID:   serviceID,
			GitRef:      "main",
			TriggerType: triggerType,
			TriggeredBy: "user@example.com",
			Priority:    10,
		}

		run, err := scheduler.ScheduleRun(ctx, req)

		// Assert
		require.NoError(t, err)
		assert.NotNil(t, run)
		assert.Equal(t, serviceID, run.ServiceID)
		assert.Equal(t, database.RunStatusPending, run.Status)
		assert.Equal(t, 10, run.Priority)

		mockServiceRepo.AssertExpectations(t)
		mockRunRepo.AssertExpectations(t)
	})

	t.Run("returns error when service not found", func(t *testing.T) {
		mockRunRepo := new(MockRunRepo)
		mockServiceRepo := new(MockServiceRepo)
		mockAgentRepo := new(MockAgentRepo)
		mockTestRepo := new(MockTestRepo)
		mockShardRepo := new(MockRunShardRepo)
		mockAgentMgr := new(MockAgentManager)
		queue := NewQueue(mockRunRepo)

		scheduler := NewScheduler(
			mockRunRepo,
			mockServiceRepo,
			mockAgentRepo,
			mockTestRepo,
			mockShardRepo,
			mockAgentMgr,
			queue,
			nil,
			DefaultConfig(),
		)

		// Setup expectations
		mockServiceRepo.On("Get", ctx, serviceID).Return(nil, nil)

		// Execute
		req := ScheduleRequest{
			ServiceID:   serviceID,
			GitRef:      "main",
			TriggerType: triggerType,
		}

		run, err := scheduler.ScheduleRun(ctx, req)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, run)
		assert.Contains(t, err.Error(), "service not found")

		mockServiceRepo.AssertExpectations(t)
	})
}

func TestScheduler_CancelRun(t *testing.T) {
	ctx := context.Background()
	runID := uuid.New()
	serviceID := uuid.New()

	t.Run("cancels pending run", func(t *testing.T) {
		mockRunRepo := new(MockRunRepo)
		mockServiceRepo := new(MockServiceRepo)
		mockAgentRepo := new(MockAgentRepo)
		mockTestRepo := new(MockTestRepo)
		mockShardRepo := new(MockRunShardRepo)
		mockAgentMgr := new(MockAgentManager)
		queue := NewQueue(mockRunRepo)

		scheduler := NewScheduler(
			mockRunRepo,
			mockServiceRepo,
			mockAgentRepo,
			mockTestRepo,
			mockShardRepo,
			mockAgentMgr,
			queue,
			nil,
			DefaultConfig(),
		)

		run := &database.TestRun{
			ID:        runID,
			ServiceID: serviceID,
			Status:    database.RunStatusPending,
		}

		// Setup expectations
		mockRunRepo.On("Get", ctx, runID).Return(run, nil)
		mockRunRepo.On("UpdateStatus", ctx, runID, database.RunStatusCancelled).Return(nil)

		// Execute
		err := scheduler.CancelRun(ctx, runID, "user cancelled")

		// Assert
		require.NoError(t, err)
		mockRunRepo.AssertExpectations(t)
	})

	t.Run("cancels running run and notifies agent", func(t *testing.T) {
		mockRunRepo := new(MockRunRepo)
		mockServiceRepo := new(MockServiceRepo)
		mockAgentRepo := new(MockAgentRepo)
		mockTestRepo := new(MockTestRepo)
		mockShardRepo := new(MockRunShardRepo)
		mockAgentMgr := new(MockAgentManager)
		queue := NewQueue(mockRunRepo)

		scheduler := NewScheduler(
			mockRunRepo,
			mockServiceRepo,
			mockAgentRepo,
			mockTestRepo,
			mockShardRepo,
			mockAgentMgr,
			queue,
			nil,
			DefaultConfig(),
		)

		agentID := uuid.New()
		run := &database.TestRun{
			ID:        runID,
			ServiceID: serviceID,
			Status:    database.RunStatusRunning,
			AgentID:   &agentID,
		}

		// Setup expectations
		mockRunRepo.On("Get", ctx, runID).Return(run, nil)
		mockAgentMgr.On("CancelWork", ctx, agentID, runID, "user cancelled").Return(nil)
		mockRunRepo.On("UpdateStatus", ctx, runID, database.RunStatusCancelled).Return(nil)

		// Execute
		err := scheduler.CancelRun(ctx, runID, "user cancelled")

		// Assert
		require.NoError(t, err)
		mockRunRepo.AssertExpectations(t)
		mockAgentMgr.AssertExpectations(t)
	})

	t.Run("returns error for terminal run", func(t *testing.T) {
		mockRunRepo := new(MockRunRepo)
		mockServiceRepo := new(MockServiceRepo)
		mockAgentRepo := new(MockAgentRepo)
		mockTestRepo := new(MockTestRepo)
		mockShardRepo := new(MockRunShardRepo)
		mockAgentMgr := new(MockAgentManager)
		queue := NewQueue(mockRunRepo)

		scheduler := NewScheduler(
			mockRunRepo,
			mockServiceRepo,
			mockAgentRepo,
			mockTestRepo,
			mockShardRepo,
			mockAgentMgr,
			queue,
			nil,
			DefaultConfig(),
		)

		run := &database.TestRun{
			ID:        runID,
			ServiceID: serviceID,
			Status:    database.RunStatusPassed,
		}

		// Setup expectations
		mockRunRepo.On("Get", ctx, runID).Return(run, nil)

		// Execute
		err := scheduler.CancelRun(ctx, runID, "user cancelled")

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "terminal state")
		mockRunRepo.AssertExpectations(t)
	})
}

func TestAgentInfo_AvailableSlots(t *testing.T) {
	tests := []struct {
		name        string
		maxParallel int
		activeRuns  int
		expected    int
	}{
		{"no active runs", 4, 0, 4},
		{"some active runs", 4, 2, 2},
		{"at capacity", 4, 4, 0},
		{"over capacity", 4, 5, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &AgentInfo{
				MaxParallel: tt.maxParallel,
				ActiveRuns:  tt.activeRuns,
			}
			assert.Equal(t, tt.expected, agent.AvailableSlots())
		})
	}
}

func TestScheduler_matchAgent(t *testing.T) {
	t.Run("selects agent with most capacity", func(t *testing.T) {
		mockRunRepo := new(MockRunRepo)
		mockServiceRepo := new(MockServiceRepo)
		mockAgentRepo := new(MockAgentRepo)
		mockTestRepo := new(MockTestRepo)
		mockShardRepo := new(MockRunShardRepo)
		mockAgentMgr := new(MockAgentManager)
		queue := NewQueue(mockRunRepo)

		scheduler := NewScheduler(
			mockRunRepo,
			mockServiceRepo,
			mockAgentRepo,
			mockTestRepo,
			mockShardRepo,
			mockAgentMgr,
			queue,
			nil,
			DefaultConfig(),
		)

		run := &database.TestRun{
			ID: uuid.New(),
		}

		agents := []*AgentInfo{
			{ID: uuid.New(), MaxParallel: 4, ActiveRuns: 3, LastHeartbeat: time.Now()},
			{ID: uuid.New(), MaxParallel: 4, ActiveRuns: 1, LastHeartbeat: time.Now()},
			{ID: uuid.New(), MaxParallel: 4, ActiveRuns: 2, LastHeartbeat: time.Now()},
		}

		selected := scheduler.matchAgent(run, agents)

		require.NotNil(t, selected)
		// Should select agent with most available slots (index 1, 3 available)
		assert.Equal(t, agents[1].ID, selected.ID)
	})

	t.Run("skips agents at capacity", func(t *testing.T) {
		mockRunRepo := new(MockRunRepo)
		mockServiceRepo := new(MockServiceRepo)
		mockAgentRepo := new(MockAgentRepo)
		mockTestRepo := new(MockTestRepo)
		mockShardRepo := new(MockRunShardRepo)
		mockAgentMgr := new(MockAgentManager)
		queue := NewQueue(mockRunRepo)

		scheduler := NewScheduler(
			mockRunRepo,
			mockServiceRepo,
			mockAgentRepo,
			mockTestRepo,
			mockShardRepo,
			mockAgentMgr,
			queue,
			nil,
			DefaultConfig(),
		)

		run := &database.TestRun{
			ID: uuid.New(),
		}

		agents := []*AgentInfo{
			{ID: uuid.New(), MaxParallel: 4, ActiveRuns: 4, LastHeartbeat: time.Now()}, // At capacity
			{ID: uuid.New(), MaxParallel: 4, ActiveRuns: 2, LastHeartbeat: time.Now()}, // Available
		}

		selected := scheduler.matchAgent(run, agents)

		require.NotNil(t, selected)
		assert.Equal(t, agents[1].ID, selected.ID)
	})

	t.Run("returns nil when all agents at capacity", func(t *testing.T) {
		mockRunRepo := new(MockRunRepo)
		mockServiceRepo := new(MockServiceRepo)
		mockAgentRepo := new(MockAgentRepo)
		mockTestRepo := new(MockTestRepo)
		mockShardRepo := new(MockRunShardRepo)
		mockAgentMgr := new(MockAgentManager)
		queue := NewQueue(mockRunRepo)

		scheduler := NewScheduler(
			mockRunRepo,
			mockServiceRepo,
			mockAgentRepo,
			mockTestRepo,
			mockShardRepo,
			mockAgentMgr,
			queue,
			nil,
			DefaultConfig(),
		)

		run := &database.TestRun{
			ID: uuid.New(),
		}

		agents := []*AgentInfo{
			{ID: uuid.New(), MaxParallel: 4, ActiveRuns: 4, LastHeartbeat: time.Now()},
			{ID: uuid.New(), MaxParallel: 2, ActiveRuns: 2, LastHeartbeat: time.Now()},
		}

		selected := scheduler.matchAgent(run, agents)

		assert.Nil(t, selected)
	})
}
