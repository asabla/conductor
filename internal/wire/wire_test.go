package wire

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
	"github.com/conductor/conductor/internal/server"
)

// Mock implementations for testing

type mockAgentRepository struct {
	agents        map[uuid.UUID]*database.Agent
	createErr     error
	updateErr     error
	getErr        error
	deleteErr     error
	listErr       error
	heartbeatErr  error
	statusErr     error
	countByStatus map[database.AgentStatus]int64
}

func newMockAgentRepository() *mockAgentRepository {
	return &mockAgentRepository{
		agents:        make(map[uuid.UUID]*database.Agent),
		countByStatus: make(map[database.AgentStatus]int64),
	}
}

func (m *mockAgentRepository) Create(ctx context.Context, agent *database.Agent) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentRepository) Update(ctx context.Context, agent *database.Agent) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentRepository) Get(ctx context.Context, id uuid.UUID) (*database.Agent, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.agents[id], nil
}

func (m *mockAgentRepository) GetByName(ctx context.Context, name string) (*database.Agent, error) {
	for _, a := range m.agents {
		if a.Name == name {
			return a, nil
		}
	}
	return nil, nil
}

func (m *mockAgentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.agents, id)
	return nil
}

func (m *mockAgentRepository) List(ctx context.Context, pagination database.Pagination) ([]database.Agent, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var agents []database.Agent
	for _, a := range m.agents {
		agents = append(agents, *a)
	}
	return agents, nil
}

func (m *mockAgentRepository) ListByStatus(ctx context.Context, status database.AgentStatus, pagination database.Pagination) ([]database.Agent, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var agents []database.Agent
	for _, a := range m.agents {
		if a.Status == status {
			agents = append(agents, *a)
		}
	}
	return agents, nil
}

func (m *mockAgentRepository) UpdateHeartbeat(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
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
	if m.statusErr != nil {
		return m.statusErr
	}
	if a, ok := m.agents[id]; ok {
		a.Status = status
	}
	return nil
}

func (m *mockAgentRepository) GetAvailable(ctx context.Context, zones []string, limit int) ([]database.Agent, error) {
	return nil, nil
}

func (m *mockAgentRepository) CountByStatus(ctx context.Context) (map[database.AgentStatus]int64, error) {
	return m.countByStatus, nil
}

func (m *mockAgentRepository) MarkOfflineAgents(ctx context.Context) (int64, error) {
	return 0, nil
}

// TestRunRepository mock

type mockTestRunRepository struct {
	runs       map[uuid.UUID]*database.TestRun
	createErr  error
	updateErr  error
	getErr     error
	listErr    error
	countTotal int64
}

func newMockTestRunRepository() *mockTestRunRepository {
	return &mockTestRunRepository{
		runs: make(map[uuid.UUID]*database.TestRun),
	}
}

func (m *mockTestRunRepository) Create(ctx context.Context, run *database.TestRun) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.runs[run.ID] = run
	return nil
}

func (m *mockTestRunRepository) Update(ctx context.Context, run *database.TestRun) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.runs[run.ID] = run
	return nil
}

func (m *mockTestRunRepository) Get(ctx context.Context, id uuid.UUID) (*database.TestRun, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.runs[id], nil
}

func (m *mockTestRunRepository) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.runs, id)
	return nil
}

func (m *mockTestRunRepository) List(ctx context.Context, pagination database.Pagination) ([]database.TestRun, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var runs []database.TestRun
	for _, r := range m.runs {
		runs = append(runs, *r)
	}
	return runs, nil
}

func (m *mockTestRunRepository) ListByService(ctx context.Context, serviceID uuid.UUID, pagination database.Pagination) ([]database.TestRun, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var runs []database.TestRun
	for _, r := range m.runs {
		if r.ServiceID == serviceID {
			runs = append(runs, *r)
		}
	}
	return runs, nil
}

func (m *mockTestRunRepository) ListByStatus(ctx context.Context, status database.RunStatus, pagination database.Pagination) ([]database.TestRun, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var runs []database.TestRun
	for _, r := range m.runs {
		if r.Status == status {
			runs = append(runs, *r)
		}
	}
	return runs, nil
}

func (m *mockTestRunRepository) ListByServiceAndStatus(ctx context.Context, serviceID uuid.UUID, status database.RunStatus, pagination database.Pagination) ([]database.TestRun, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var runs []database.TestRun
	for _, r := range m.runs {
		if r.ServiceID == serviceID && r.Status == status {
			runs = append(runs, *r)
		}
	}
	return runs, nil
}

func (m *mockTestRunRepository) ListByDateRange(ctx context.Context, start, end time.Time, pagination database.Pagination) ([]database.TestRun, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.List(ctx, pagination)
}

func (m *mockTestRunRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status database.RunStatus) error {
	if r, ok := m.runs[id]; ok {
		r.Status = status
	}
	return nil
}

func (m *mockTestRunRepository) Start(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error {
	if r, ok := m.runs[id]; ok {
		r.AgentID = &agentID
		now := time.Now()
		r.StartedAt = &now
		r.Status = database.RunStatusRunning
	}
	return nil
}

func (m *mockTestRunRepository) Finish(ctx context.Context, id uuid.UUID, status database.RunStatus, results database.RunResults) error {
	if r, ok := m.runs[id]; ok {
		r.Status = status
		now := time.Now()
		r.FinishedAt = &now
		r.TotalTests = results.TotalTests
		r.PassedTests = results.PassedTests
		r.FailedTests = results.FailedTests
		r.SkippedTests = results.SkippedTests
	}
	return nil
}

func (m *mockTestRunRepository) GetPending(ctx context.Context, limit int) ([]database.TestRun, error) {
	return m.ListByStatus(ctx, database.RunStatusPending, database.Pagination{Limit: limit})
}

func (m *mockTestRunRepository) GetRunning(ctx context.Context) ([]database.TestRun, error) {
	return m.ListByStatus(ctx, database.RunStatusRunning, database.Pagination{Limit: 1000})
}

func (m *mockTestRunRepository) Count(ctx context.Context) (int64, error) {
	return m.countTotal, nil
}

func (m *mockTestRunRepository) CountByService(ctx context.Context, serviceID uuid.UUID) (int64, error) {
	var count int64
	for _, r := range m.runs {
		if r.ServiceID == serviceID {
			count++
		}
	}
	return count, nil
}

func (m *mockTestRunRepository) CountByStatus(ctx context.Context) (map[database.RunStatus]int64, error) {
	return nil, nil
}

// ServiceRepository mock

type mockServiceRepository struct {
	services   map[uuid.UUID]*database.Service
	createErr  error
	updateErr  error
	getErr     error
	deleteErr  error
	listErr    error
	countTotal int64
}

func newMockServiceRepository() *mockServiceRepository {
	return &mockServiceRepository{
		services: make(map[uuid.UUID]*database.Service),
	}
}

func (m *mockServiceRepository) Create(ctx context.Context, svc *database.Service) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.services[svc.ID] = svc
	return nil
}

func (m *mockServiceRepository) Update(ctx context.Context, svc *database.Service) error {
	if m.updateErr != nil {
		return m.updateErr
	}
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
	for _, s := range m.services {
		if s.Name == name {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockServiceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.services, id)
	return nil
}

func (m *mockServiceRepository) List(ctx context.Context, pagination database.Pagination) ([]database.Service, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var services []database.Service
	for _, s := range m.services {
		services = append(services, *s)
	}
	return services, nil
}

func (m *mockServiceRepository) ListByOwner(ctx context.Context, owner string, pagination database.Pagination) ([]database.Service, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var services []database.Service
	for _, s := range m.services {
		if s.Owner != nil && *s.Owner == owner {
			services = append(services, *s)
		}
	}
	return services, nil
}

func (m *mockServiceRepository) Search(ctx context.Context, query string, pagination database.Pagination) ([]database.Service, error) {
	return m.List(ctx, pagination)
}

func (m *mockServiceRepository) Count(ctx context.Context) (int64, error) {
	return m.countTotal, nil
}

// TestDefinitionRepository mock

type mockTestDefinitionRepository struct {
	tests     map[uuid.UUID]*database.TestDefinition
	createErr error
	updateErr error
	getErr    error
	deleteErr error
	listErr   error
}

func newMockTestDefinitionRepository() *mockTestDefinitionRepository {
	return &mockTestDefinitionRepository{
		tests: make(map[uuid.UUID]*database.TestDefinition),
	}
}

func (m *mockTestDefinitionRepository) Create(ctx context.Context, test *database.TestDefinition) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.tests[test.ID] = test
	return nil
}

func (m *mockTestDefinitionRepository) Update(ctx context.Context, test *database.TestDefinition) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.tests[test.ID] = test
	return nil
}

func (m *mockTestDefinitionRepository) Get(ctx context.Context, id uuid.UUID) (*database.TestDefinition, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.tests[id], nil
}

func (m *mockTestDefinitionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.tests, id)
	return nil
}

func (m *mockTestDefinitionRepository) ListByService(ctx context.Context, serviceID uuid.UUID, pagination database.Pagination) ([]database.TestDefinition, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var tests []database.TestDefinition
	for _, t := range m.tests {
		if t.ServiceID == serviceID {
			tests = append(tests, *t)
		}
	}
	return tests, nil
}

func (m *mockTestDefinitionRepository) ListByTags(ctx context.Context, serviceID uuid.UUID, tags []string, pagination database.Pagination) ([]database.TestDefinition, error) {
	return m.ListByService(ctx, serviceID, pagination)
}

// Tests

func TestAgentRepositoryAdapter_Create(t *testing.T) {
	mock := newMockAgentRepository()
	adapter := NewAgentRepositoryAdapter(mock)

	agent := &database.Agent{
		ID:     uuid.New(),
		Name:   "test-agent",
		Status: database.AgentStatusIdle,
	}

	if err := adapter.Create(context.Background(), agent); err != nil {
		t.Errorf("Create() error = %v", err)
	}

	// Verify it was stored
	if _, ok := mock.agents[agent.ID]; !ok {
		t.Error("Agent was not created")
	}
}

func TestAgentRepositoryAdapter_Create_Error(t *testing.T) {
	mock := newMockAgentRepository()
	mock.createErr = errors.New("create failed")
	adapter := NewAgentRepositoryAdapter(mock)

	agent := &database.Agent{ID: uuid.New()}
	if err := adapter.Create(context.Background(), agent); err == nil {
		t.Error("Create() expected error, got nil")
	}
}

func TestAgentRepositoryAdapter_GetByID(t *testing.T) {
	mock := newMockAgentRepository()
	adapter := NewAgentRepositoryAdapter(mock)

	agentID := uuid.New()
	mock.agents[agentID] = &database.Agent{
		ID:     agentID,
		Name:   "test-agent",
		Status: database.AgentStatusIdle,
	}

	got, err := adapter.GetByID(context.Background(), agentID)
	if err != nil {
		t.Errorf("GetByID() error = %v", err)
	}
	if got == nil {
		t.Error("GetByID() returned nil")
	}
	if got.Name != "test-agent" {
		t.Errorf("GetByID() Name = %q, want %q", got.Name, "test-agent")
	}
}

func TestAgentRepositoryAdapter_UpdateHeartbeat(t *testing.T) {
	mock := newMockAgentRepository()
	adapter := NewAgentRepositoryAdapter(mock)

	agentID := uuid.New()
	mock.agents[agentID] = &database.Agent{
		ID:     agentID,
		Status: database.AgentStatusIdle,
	}

	err := adapter.UpdateHeartbeat(context.Background(), agentID, database.AgentStatusBusy)
	if err != nil {
		t.Errorf("UpdateHeartbeat() error = %v", err)
	}

	if mock.agents[agentID].Status != database.AgentStatusBusy {
		t.Errorf("Status = %v, want %v", mock.agents[agentID].Status, database.AgentStatusBusy)
	}
}

func TestAgentRepositoryAdapter_List(t *testing.T) {
	mock := newMockAgentRepository()
	adapter := NewAgentRepositoryAdapter(mock)

	// Add some agents
	for i := 0; i < 3; i++ {
		id := uuid.New()
		mock.agents[id] = &database.Agent{
			ID:     id,
			Status: database.AgentStatusIdle,
		}
	}
	mock.countByStatus[database.AgentStatusIdle] = 3

	agents, total, err := adapter.List(context.Background(), server.AgentFilter{}, database.Pagination{Limit: 10})
	if err != nil {
		t.Errorf("List() error = %v", err)
	}
	if len(agents) != 3 {
		t.Errorf("List() returned %d agents, want 3", len(agents))
	}
	if total != 3 {
		t.Errorf("List() total = %d, want 3", total)
	}
}

func TestAgentRepositoryAdapter_ListByStatus(t *testing.T) {
	mock := newMockAgentRepository()
	adapter := NewAgentRepositoryAdapter(mock)

	// Add agents with different statuses
	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()
	mock.agents[id1] = &database.Agent{ID: id1, Status: database.AgentStatusIdle}
	mock.agents[id2] = &database.Agent{ID: id2, Status: database.AgentStatusBusy}
	mock.agents[id3] = &database.Agent{ID: id3, Status: database.AgentStatusIdle}
	mock.countByStatus[database.AgentStatusIdle] = 2
	mock.countByStatus[database.AgentStatusBusy] = 1

	filter := server.AgentFilter{Statuses: []database.AgentStatus{database.AgentStatusIdle}}
	agents, _, err := adapter.List(context.Background(), filter, database.Pagination{Limit: 10})
	if err != nil {
		t.Errorf("List() error = %v", err)
	}
	// Verify all returned agents have the filtered status
	for _, a := range agents {
		if a.Status != database.AgentStatusIdle {
			t.Errorf("List() returned agent with status %v, want only Idle", a.Status)
		}
	}
}

func TestRunRepositoryAdapter_Create(t *testing.T) {
	mock := newMockTestRunRepository()
	adapter := NewRunRepositoryAdapter(mock)

	run := &database.TestRun{
		ID:        uuid.New(),
		ServiceID: uuid.New(),
		Status:    database.RunStatusPending,
	}

	if err := adapter.Create(context.Background(), run); err != nil {
		t.Errorf("Create() error = %v", err)
	}

	if _, ok := mock.runs[run.ID]; !ok {
		t.Error("Run was not created")
	}
}

func TestRunRepositoryAdapter_UpdateStatus(t *testing.T) {
	mock := newMockTestRunRepository()
	adapter := NewRunRepositoryAdapter(mock)

	runID := uuid.New()
	mock.runs[runID] = &database.TestRun{
		ID:     runID,
		Status: database.RunStatusPending,
	}

	errMsg := "test failed"
	err := adapter.UpdateStatus(context.Background(), runID, database.RunStatusFailed, &errMsg)
	if err != nil {
		t.Errorf("UpdateStatus() error = %v", err)
	}

	if mock.runs[runID].Status != database.RunStatusFailed {
		t.Errorf("Status = %v, want %v", mock.runs[runID].Status, database.RunStatusFailed)
	}
	if mock.runs[runID].ErrorMessage == nil || *mock.runs[runID].ErrorMessage != errMsg {
		t.Errorf("ErrorMessage = %v, want %q", mock.runs[runID].ErrorMessage, errMsg)
	}
}

func TestRunRepositoryAdapter_List_Filters(t *testing.T) {
	mock := newMockTestRunRepository()
	adapter := NewRunRepositoryAdapter(mock)

	serviceID := uuid.New()
	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()
	mock.runs[id1] = &database.TestRun{ID: id1, ServiceID: serviceID, Status: database.RunStatusPassed}
	mock.runs[id2] = &database.TestRun{ID: id2, ServiceID: serviceID, Status: database.RunStatusFailed}
	mock.runs[id3] = &database.TestRun{ID: id3, ServiceID: uuid.New(), Status: database.RunStatusPassed}
	mock.countTotal = 3

	t.Run("no filter", func(t *testing.T) {
		runs, total, err := adapter.List(context.Background(), server.RunFilter{}, database.Pagination{Limit: 10})
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(runs) != 3 {
			t.Errorf("List() returned %d runs, want 3", len(runs))
		}
		if total != 3 {
			t.Errorf("List() total = %d, want 3", total)
		}
	})

	t.Run("filter by service", func(t *testing.T) {
		filter := server.RunFilter{ServiceID: &serviceID}
		runs, _, err := adapter.List(context.Background(), filter, database.Pagination{Limit: 10})
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(runs) != 2 {
			t.Errorf("List() returned %d runs, want 2", len(runs))
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		filter := server.RunFilter{Statuses: []database.RunStatus{database.RunStatusPassed}}
		runs, _, err := adapter.List(context.Background(), filter, database.Pagination{Limit: 10})
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		for _, r := range runs {
			if r.Status != database.RunStatusPassed {
				t.Errorf("List() returned run with status %v, want %v", r.Status, database.RunStatusPassed)
			}
		}
	})

	t.Run("filter by service and status", func(t *testing.T) {
		filter := server.RunFilter{
			ServiceID: &serviceID,
			Statuses:  []database.RunStatus{database.RunStatusPassed},
		}
		runs, _, err := adapter.List(context.Background(), filter, database.Pagination{Limit: 10})
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(runs) != 1 {
			t.Errorf("List() returned %d runs, want 1", len(runs))
		}
	})
}

func TestServiceRepositoryAdapter_CRUD(t *testing.T) {
	mock := newMockServiceRepository()
	adapter := NewServiceRepositoryAdapter(mock)
	ctx := context.Background()

	// Create
	owner := "test-team"
	svc := &database.Service{
		ID:    uuid.New(),
		Name:  "test-service",
		Owner: &owner,
	}
	if err := adapter.Create(ctx, svc); err != nil {
		t.Errorf("Create() error = %v", err)
	}

	// Read
	got, err := adapter.GetByID(ctx, svc.ID)
	if err != nil {
		t.Errorf("GetByID() error = %v", err)
	}
	if got.Name != "test-service" {
		t.Errorf("Name = %q, want %q", got.Name, "test-service")
	}

	// Update
	svc.Name = "updated-service"
	if err := adapter.Update(ctx, svc); err != nil {
		t.Errorf("Update() error = %v", err)
	}
	got, _ = adapter.GetByID(ctx, svc.ID)
	if got.Name != "updated-service" {
		t.Errorf("Name after update = %q, want %q", got.Name, "updated-service")
	}

	// Delete
	if err := adapter.Delete(ctx, svc.ID); err != nil {
		t.Errorf("Delete() error = %v", err)
	}
	got, _ = adapter.GetByID(ctx, svc.ID)
	if got != nil {
		t.Error("Service still exists after delete")
	}
}

func TestServiceRepositoryAdapter_List_Filters(t *testing.T) {
	mock := newMockServiceRepository()
	adapter := NewServiceRepositoryAdapter(mock)
	ctx := context.Background()

	// Add services
	teamA := "team-a"
	teamB := "team-b"
	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()
	mock.services[id1] = &database.Service{ID: id1, Name: "svc1", Owner: &teamA}
	mock.services[id2] = &database.Service{ID: id2, Name: "svc2", Owner: &teamA}
	mock.services[id3] = &database.Service{ID: id3, Name: "svc3", Owner: &teamB}
	mock.countTotal = 3

	t.Run("no filter", func(t *testing.T) {
		services, total, err := adapter.List(ctx, server.ServiceFilter{}, database.Pagination{Limit: 10})
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(services) != 3 {
			t.Errorf("List() returned %d services, want 3", len(services))
		}
		if total != 3 {
			t.Errorf("List() total = %d, want 3", total)
		}
	})

	t.Run("filter by owner", func(t *testing.T) {
		filter := server.ServiceFilter{Owner: "team-a"}
		services, _, err := adapter.List(ctx, filter, database.Pagination{Limit: 10})
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(services) != 2 {
			t.Errorf("List() returned %d services, want 2", len(services))
		}
	})

	t.Run("filter by query (search)", func(t *testing.T) {
		filter := server.ServiceFilter{Query: "svc"}
		services, _, err := adapter.List(ctx, filter, database.Pagination{Limit: 10})
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		// Mock search returns all
		if len(services) != 3 {
			t.Errorf("List() returned %d services, want 3", len(services))
		}
	})
}

func TestTestDefinitionRepositoryAdapter_CRUD(t *testing.T) {
	mock := newMockTestDefinitionRepository()
	adapter := NewTestDefinitionRepositoryAdapter(mock)
	ctx := context.Background()

	serviceID := uuid.New()

	// Create
	test := &database.TestDefinition{
		ID:        uuid.New(),
		ServiceID: serviceID,
		Name:      "test-def",
	}
	if err := adapter.Create(ctx, test); err != nil {
		t.Errorf("Create() error = %v", err)
	}

	// Read
	got, err := adapter.GetByID(ctx, serviceID, test.ID)
	if err != nil {
		t.Errorf("GetByID() error = %v", err)
	}
	if got.Name != "test-def" {
		t.Errorf("Name = %q, want %q", got.Name, "test-def")
	}

	// GetByID with wrong serviceID
	otherServiceID := uuid.New()
	got, err = adapter.GetByID(ctx, otherServiceID, test.ID)
	if err != database.ErrNotFound {
		t.Errorf("GetByID() with wrong serviceID error = %v, want ErrNotFound", err)
	}

	// Update
	test.Name = "updated-test"
	if err := adapter.Update(ctx, test); err != nil {
		t.Errorf("Update() error = %v", err)
	}
	got, _ = adapter.GetByID(ctx, serviceID, test.ID)
	if got.Name != "updated-test" {
		t.Errorf("Name after update = %q, want %q", got.Name, "updated-test")
	}

	// Delete
	if err := adapter.Delete(ctx, test.ID); err != nil {
		t.Errorf("Delete() error = %v", err)
	}
}

func TestTestDefinitionRepositoryAdapter_ListByService(t *testing.T) {
	mock := newMockTestDefinitionRepository()
	adapter := NewTestDefinitionRepositoryAdapter(mock)
	ctx := context.Background()

	serviceID := uuid.New()
	otherServiceID := uuid.New()

	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()
	mock.tests[id1] = &database.TestDefinition{ID: id1, ServiceID: serviceID, Name: "test1"}
	mock.tests[id2] = &database.TestDefinition{ID: id2, ServiceID: serviceID, Name: "test2"}
	mock.tests[id3] = &database.TestDefinition{ID: id3, ServiceID: otherServiceID, Name: "test3"}

	tests, count, err := adapter.ListByService(ctx, serviceID, server.TestDefinitionFilter{}, database.Pagination{Limit: 10})
	if err != nil {
		t.Errorf("ListByService() error = %v", err)
	}
	if len(tests) != 2 {
		t.Errorf("ListByService() returned %d tests, want 2", len(tests))
	}
	if count != 2 {
		t.Errorf("ListByService() count = %d, want 2", count)
	}
}

func TestTestDefinitionRepositoryAdapter_DeleteByService(t *testing.T) {
	mock := newMockTestDefinitionRepository()
	adapter := NewTestDefinitionRepositoryAdapter(mock)
	ctx := context.Background()

	serviceID := uuid.New()
	otherServiceID := uuid.New()

	test1ID := uuid.New()
	test2ID := uuid.New()
	test3ID := uuid.New()
	mock.tests[test1ID] = &database.TestDefinition{ID: test1ID, ServiceID: serviceID, Name: "test1"}
	mock.tests[test2ID] = &database.TestDefinition{ID: test2ID, ServiceID: serviceID, Name: "test2"}
	mock.tests[test3ID] = &database.TestDefinition{ID: test3ID, ServiceID: otherServiceID, Name: "test3"}

	if err := adapter.DeleteByService(ctx, serviceID); err != nil {
		t.Errorf("DeleteByService() error = %v", err)
	}

	// Check that only tests from the target service are deleted
	if len(mock.tests) != 1 {
		t.Errorf("After DeleteByService, %d tests remain, want 1", len(mock.tests))
	}
	if _, ok := mock.tests[test3ID]; !ok {
		t.Error("Test from other service was deleted")
	}
}

func TestNoopScheduler(t *testing.T) {
	scheduler := &NoopScheduler{}
	ctx := context.Background()

	t.Run("AssignWork returns nil", func(t *testing.T) {
		work, err := scheduler.AssignWork(ctx, uuid.New(), nil)
		if err != nil {
			t.Errorf("AssignWork() error = %v", err)
		}
		if work != nil {
			t.Errorf("AssignWork() = %v, want nil", work)
		}
	})

	t.Run("CancelWork returns nil", func(t *testing.T) {
		if err := scheduler.CancelWork(ctx, uuid.New(), "reason"); err != nil {
			t.Errorf("CancelWork() error = %v", err)
		}
	})

	t.Run("HandleWorkAccepted returns nil", func(t *testing.T) {
		if err := scheduler.HandleWorkAccepted(ctx, uuid.New(), uuid.New()); err != nil {
			t.Errorf("HandleWorkAccepted() error = %v", err)
		}
	})

	t.Run("HandleWorkRejected returns nil", func(t *testing.T) {
		if err := scheduler.HandleWorkRejected(ctx, uuid.New(), uuid.New(), "reason"); err != nil {
			t.Errorf("HandleWorkRejected() error = %v", err)
		}
	})

	t.Run("HandleRunComplete returns nil", func(t *testing.T) {
		if err := scheduler.HandleRunComplete(ctx, uuid.New(), uuid.New(), nil); err != nil {
			t.Errorf("HandleRunComplete() error = %v", err)
		}
	})
}

func TestNoopGitSyncer(t *testing.T) {
	syncer := &NoopGitSyncer{}
	ctx := context.Background()

	result, err := syncer.SyncService(ctx, &database.Service{}, "main")
	if err != nil {
		t.Errorf("SyncService() error = %v", err)
	}
	if result == nil {
		t.Error("SyncService() returned nil result")
	}
	if result.SyncedAt.IsZero() {
		t.Error("SyncService() returned zero SyncedAt")
	}
}

func TestNoopArtifactStorage(t *testing.T) {
	storage := &NoopArtifactStorage{}
	ctx := context.Background()

	url, expiresAt, err := storage.GenerateDownloadURL(ctx, "/path/to/artifact", 3600)
	if err != nil {
		t.Errorf("GenerateDownloadURL() error = %v", err)
	}
	if url != "" {
		t.Errorf("GenerateDownloadURL() url = %q, want empty string", url)
	}
	if expiresAt.IsZero() {
		t.Error("GenerateDownloadURL() expiresAt is zero")
	}
}

func TestGitSyncerAdapter(t *testing.T) {
	mockSyncer := &mockGitSyncer{
		result: &server.SyncResult{
			SyncedAt:     time.Now(),
			TestsAdded:   2,
			TestsUpdated: 1,
		},
	}
	adapter := NewGitSyncerAdapter(mockSyncer)
	ctx := context.Background()

	result, err := adapter.SyncService(ctx, &database.Service{}, "main")
	if err != nil {
		t.Errorf("SyncService() error = %v", err)
	}
	if result.TestsAdded != 2 {
		t.Errorf("SyncService() TestsAdded = %d, want 2", result.TestsAdded)
	}
	if result.TestsUpdated != 1 {
		t.Errorf("SyncService() TestsUpdated = %d, want 1", result.TestsUpdated)
	}
}

func TestGitSyncerAdapter_Error(t *testing.T) {
	mockSyncer := &mockGitSyncer{
		err: errors.New("sync failed"),
	}
	adapter := NewGitSyncerAdapter(mockSyncer)
	ctx := context.Background()

	_, err := adapter.SyncService(ctx, &database.Service{}, "main")
	if err == nil {
		t.Error("SyncService() expected error, got nil")
	}
}

// Mock git syncer for testing adapter
type mockGitSyncer struct {
	result *server.SyncResult
	err    error
}

func (m *mockGitSyncer) SyncService(ctx context.Context, service *database.Service, branch string) (*server.SyncResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}
