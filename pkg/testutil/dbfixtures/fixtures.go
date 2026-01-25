//go:build integration

// Package dbfixtures provides database fixtures for integration tests.
package dbfixtures

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// FixtureOptions allows customizing fixture creation.
type FixtureOptions struct {
	// Name override for the fixture
	Name string
	// Owner override for the fixture
	Owner string
	// Tags for test definitions
	Tags []string
	// NetworkZones for services/agents
	NetworkZones []string
	// Status for agents/runs
	Status string
	// Count for creating multiple items
	Count int
	// Priority for test runs
	Priority int
	// TriggerType for test runs
	TriggerType database.TriggerType
	// TriggeredBy for test runs
	TriggeredBy string
}

// DefaultFixtureOptions returns sensible defaults for fixture options.
func DefaultFixtureOptions() FixtureOptions {
	return FixtureOptions{
		Owner:        "test-owner",
		NetworkZones: []string{"default"},
		Status:       "idle",
		Count:        1,
		Priority:     5,
		TriggerType:  database.TriggerTypeManual,
		TriggeredBy:  "test-user",
	}
}

// Fixtures provides factory methods for creating test data.
type Fixtures struct {
	db *database.DB
}

// NewFixtures creates a new Fixtures instance.
func NewFixtures(db *database.DB) *Fixtures {
	return &Fixtures{db: db}
}

// CreateTestService creates a test service in the database.
func (f *Fixtures) CreateTestService(ctx context.Context, opts ...FixtureOptions) (*database.Service, error) {
	opt := mergeOptions(opts...)

	name := opt.Name
	if name == "" {
		name = fmt.Sprintf("test-service-%s", uuid.New().String()[:8])
	}

	owner := opt.Owner
	if owner == "" {
		owner = "test-owner"
	}

	service := &database.Service{
		Name:          name,
		GitURL:        fmt.Sprintf("https://github.com/test/%s", name),
		DefaultBranch: "main",
		NetworkZones:  opt.NetworkZones,
		Owner:         &owner,
	}

	repo := database.NewServiceRepo(f.db)
	if err := repo.Create(ctx, service); err != nil {
		return nil, fmt.Errorf("failed to create test service: %w", err)
	}

	return service, nil
}

// CreateTestServices creates multiple test services in the database.
func (f *Fixtures) CreateTestServices(ctx context.Context, count int, opts ...FixtureOptions) ([]*database.Service, error) {
	opt := mergeOptions(opts...)

	services := make([]*database.Service, 0, count)
	for i := 0; i < count; i++ {
		// Set unique name for each service
		opt.Name = fmt.Sprintf("test-service-%d-%s", i, uuid.New().String()[:8])
		svc, err := f.CreateTestService(ctx, opt)
		if err != nil {
			// Clean up created services on error
			f.cleanupServices(ctx, services)
			return nil, err
		}
		services = append(services, svc)
	}
	return services, nil
}

// CreateTestRun creates a test run in the database.
func (f *Fixtures) CreateTestRun(ctx context.Context, serviceID uuid.UUID, opts ...FixtureOptions) (*database.TestRun, error) {
	opt := mergeOptions(opts...)

	gitRef := "main"
	gitSHA := fmt.Sprintf("%s%s", uuid.New().String()[:20], uuid.New().String()[:20])
	triggeredBy := opt.TriggeredBy
	if triggeredBy == "" {
		triggeredBy = "test-user"
	}
	triggerType := opt.TriggerType

	run := &database.TestRun{
		ServiceID:   serviceID,
		Status:      database.RunStatusPending,
		GitRef:      &gitRef,
		GitSHA:      &gitSHA,
		TriggerType: &triggerType,
		TriggeredBy: &triggeredBy,
		Priority:    opt.Priority,
	}

	repo := database.NewRunRepo(f.db)
	if err := repo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create test run: %w", err)
	}

	return run, nil
}

// CreateTestRuns creates multiple test runs for a service.
func (f *Fixtures) CreateTestRuns(ctx context.Context, serviceID uuid.UUID, count int, opts ...FixtureOptions) ([]*database.TestRun, error) {
	opt := mergeOptions(opts...)

	runs := make([]*database.TestRun, 0, count)
	for i := 0; i < count; i++ {
		opt.Priority = (count - i) * 10 // Different priorities
		run, err := f.CreateTestRun(ctx, serviceID, opt)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// CreateTestAgent creates a test agent in the database.
func (f *Fixtures) CreateTestAgent(ctx context.Context, opts ...FixtureOptions) (*database.Agent, error) {
	opt := mergeOptions(opts...)

	name := opt.Name
	if name == "" {
		name = fmt.Sprintf("test-agent-%s", uuid.New().String()[:8])
	}

	status := database.AgentStatusIdle
	if opt.Status != "" {
		status = database.AgentStatus(opt.Status)
	}

	version := "1.0.0-test"
	now := time.Now()

	agent := &database.Agent{
		ID:              uuid.New(),
		Name:            name,
		Status:          status,
		Version:         &version,
		NetworkZones:    opt.NetworkZones,
		MaxParallel:     4,
		DockerAvailable: true,
		LastHeartbeat:   &now,
		RegisteredAt:    now,
	}

	repo := database.NewAgentRepo(f.db)
	if err := repo.Create(ctx, agent); err != nil {
		return nil, fmt.Errorf("failed to create test agent: %w", err)
	}

	return agent, nil
}

// CreateTestAgents creates multiple test agents in the database.
func (f *Fixtures) CreateTestAgents(ctx context.Context, count int, opts ...FixtureOptions) ([]*database.Agent, error) {
	opt := mergeOptions(opts...)

	agents := make([]*database.Agent, 0, count)
	for i := 0; i < count; i++ {
		opt.Name = fmt.Sprintf("test-agent-%d-%s", i, uuid.New().String()[:8])
		agent, err := f.CreateTestAgent(ctx, opt)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

// CreateTestResults creates test results for a run in the database.
func (f *Fixtures) CreateTestResults(ctx context.Context, runID uuid.UUID, opts ...FixtureOptions) ([]database.TestResult, error) {
	opt := mergeOptions(opts...)

	count := opt.Count
	if count <= 0 {
		count = 5 // Default to 5 test results
	}

	results := make([]database.TestResult, 0, count)
	statuses := []database.ResultStatus{
		database.ResultStatusPass,
		database.ResultStatusPass,
		database.ResultStatusPass,
		database.ResultStatusFail,
		database.ResultStatusSkip,
	}

	for i := 0; i < count; i++ {
		status := statuses[i%len(statuses)]
		durationMs := int64((i + 1) * 100)
		suiteName := "TestSuite"

		result := database.TestResult{
			RunID:      runID,
			TestName:   fmt.Sprintf("Test%s_%d", uuid.New().String()[:4], i),
			SuiteName:  &suiteName,
			Status:     status,
			DurationMs: &durationMs,
		}

		if status == database.ResultStatusFail {
			errMsg := fmt.Sprintf("assertion failed at test %d", i)
			stackTrace := fmt.Sprintf("at Test%d (test_file.go:%d)", i, i*10+5)
			result.ErrorMessage = &errMsg
			result.StackTrace = &stackTrace
		}

		results = append(results, result)
	}

	repo := database.NewResultRepo(f.db)
	if err := repo.BatchCreate(ctx, results); err != nil {
		return nil, fmt.Errorf("failed to create test results: %w", err)
	}

	return results, nil
}

// CreateTestDefinition creates a test definition for a service.
func (f *Fixtures) CreateTestDefinition(ctx context.Context, serviceID uuid.UUID, opts ...FixtureOptions) (*database.TestDefinition, error) {
	opt := mergeOptions(opts...)

	name := opt.Name
	if name == "" {
		name = fmt.Sprintf("test-definition-%s", uuid.New().String()[:8])
	}

	tags := opt.Tags
	if len(tags) == 0 {
		tags = []string{"unit", "fast"}
	}

	description := "Test definition for testing"

	def := &database.TestDefinition{
		ServiceID:      serviceID,
		Name:           name,
		Description:    &description,
		ExecutionType:  "subprocess",
		Command:        "go",
		Args:           []string{"test", "-v", "./..."},
		TimeoutSeconds: 300,
		Tags:           tags,
		Retries:        1,
		AllowFailure:   false,
	}

	repo := database.NewTestDefinitionRepo(f.db)
	if err := repo.Create(ctx, def); err != nil {
		return nil, fmt.Errorf("failed to create test definition: %w", err)
	}

	return def, nil
}

// CreateTestDefinitions creates multiple test definitions for a service.
func (f *Fixtures) CreateTestDefinitions(ctx context.Context, serviceID uuid.UUID, count int, opts ...FixtureOptions) ([]*database.TestDefinition, error) {
	opt := mergeOptions(opts...)

	defs := make([]*database.TestDefinition, 0, count)
	for i := 0; i < count; i++ {
		opt.Name = fmt.Sprintf("test-definition-%d-%s", i, uuid.New().String()[:8])
		opt.Tags = []string{fmt.Sprintf("tag-%d", i), "common"}
		def, err := f.CreateTestDefinition(ctx, serviceID, opt)
		if err != nil {
			return nil, err
		}
		defs = append(defs, def)
	}
	return defs, nil
}

// CreateNotificationChannel creates a notification channel in the database.
func (f *Fixtures) CreateNotificationChannel(ctx context.Context, channelType database.ChannelType, opts ...FixtureOptions) (*database.NotificationChannel, error) {
	opt := mergeOptions(opts...)

	name := opt.Name
	if name == "" {
		name = fmt.Sprintf("test-channel-%s", uuid.New().String()[:8])
	}

	var config []byte
	switch channelType {
	case database.ChannelTypeSlack:
		cfg := database.SlackChannelConfig{
			WebhookURL: "https://hooks.slack.com/services/test/test/test",
			Channel:    "#test-channel",
		}
		config, _ = json.Marshal(cfg)
	case database.ChannelTypeEmail:
		cfg := database.EmailChannelConfig{
			Recipients:  []string{"test@example.com"},
			IncludeLogs: true,
		}
		config, _ = json.Marshal(cfg)
	case database.ChannelTypeWebhook:
		cfg := database.WebhookChannelConfig{
			URL:    "https://example.com/webhook",
			Method: "POST",
		}
		config, _ = json.Marshal(cfg)
	default:
		config = []byte(`{}`)
	}

	channel := &database.NotificationChannel{
		Name:    name,
		Type:    channelType,
		Config:  config,
		Enabled: true,
	}

	repo := database.NewNotificationRepo(f.db)
	if err := repo.CreateChannel(ctx, channel); err != nil {
		return nil, fmt.Errorf("failed to create notification channel: %w", err)
	}

	return channel, nil
}

// CreateNotificationRule creates a notification rule in the database.
func (f *Fixtures) CreateNotificationRule(ctx context.Context, channelID uuid.UUID, serviceID *uuid.UUID, opts ...FixtureOptions) (*database.NotificationRule, error) {
	rule := &database.NotificationRule{
		ChannelID: channelID,
		ServiceID: serviceID,
		TriggerOn: []database.TriggerEvent{
			database.TriggerEventFailure,
			database.TriggerEventRecovery,
		},
		Enabled: true,
	}

	repo := database.NewNotificationRepo(f.db)
	if err := repo.CreateRule(ctx, rule); err != nil {
		return nil, fmt.Errorf("failed to create notification rule: %w", err)
	}

	return rule, nil
}

// CreateScheduledRun creates a scheduled run for a service.
func (f *Fixtures) CreateScheduledRun(ctx context.Context, serviceID uuid.UUID, opts ...FixtureOptions) (*database.ScheduledRun, error) {
	opt := mergeOptions(opts...)

	name := opt.Name
	if name == "" {
		name = fmt.Sprintf("test-schedule-%s", uuid.New().String()[:8])
	}

	nextRun := time.Now().Add(1 * time.Hour)

	schedule := &database.ScheduledRun{
		ServiceID:      serviceID,
		Name:           name,
		CronExpression: "0 0 * * *", // Daily at midnight
		GitRef:         "main",
		TestFilter:     []string{},
		Enabled:        true,
		NextRunAt:      &nextRun,
	}

	repo := database.NewScheduleRepo(f.db)
	if err := repo.Create(ctx, schedule); err != nil {
		return nil, fmt.Errorf("failed to create scheduled run: %w", err)
	}

	return schedule, nil
}

// CreateCompleteTestScenario creates a complete test scenario with service, agent, run, and results.
func (f *Fixtures) CreateCompleteTestScenario(ctx context.Context, opts ...FixtureOptions) (*TestScenario, error) {
	opt := mergeOptions(opts...)

	// Create service
	service, err := f.CreateTestService(ctx, opt)
	if err != nil {
		return nil, err
	}

	// Create agent
	agent, err := f.CreateTestAgent(ctx, opt)
	if err != nil {
		return nil, err
	}

	// Create test definition
	def, err := f.CreateTestDefinition(ctx, service.ID, opt)
	if err != nil {
		return nil, err
	}

	// Create test run
	run, err := f.CreateTestRun(ctx, service.ID, opt)
	if err != nil {
		return nil, err
	}

	// Start the run with the agent
	runRepo := database.NewRunRepo(f.db)
	if err := runRepo.Start(ctx, run.ID, agent.ID); err != nil {
		return nil, err
	}

	// Refresh the run to get updated state
	run, err = runRepo.Get(ctx, run.ID)
	if err != nil {
		return nil, err
	}

	// Create test results
	results, err := f.CreateTestResults(ctx, run.ID, FixtureOptions{Count: 5})
	if err != nil {
		return nil, err
	}

	return &TestScenario{
		Service:        service,
		Agent:          agent,
		TestDefinition: def,
		Run:            run,
		Results:        results,
	}, nil
}

// TestScenario holds all fixtures for a complete test scenario.
type TestScenario struct {
	Service        *database.Service
	Agent          *database.Agent
	TestDefinition *database.TestDefinition
	Run            *database.TestRun
	Results        []database.TestResult
}

// Cleanup removes all fixtures created in this scenario.
func (s *TestScenario) Cleanup(ctx context.Context, db *database.DB) error {
	pool := db.Pool()

	// Delete in reverse order of dependencies
	if s.Run != nil {
		pool.Exec(ctx, "DELETE FROM test_results WHERE run_id = $1", s.Run.ID)
		pool.Exec(ctx, "DELETE FROM test_runs WHERE id = $1", s.Run.ID)
	}

	if s.TestDefinition != nil {
		pool.Exec(ctx, "DELETE FROM test_definitions WHERE id = $1", s.TestDefinition.ID)
	}

	if s.Agent != nil {
		pool.Exec(ctx, "DELETE FROM agents WHERE id = $1", s.Agent.ID)
	}

	if s.Service != nil {
		pool.Exec(ctx, "DELETE FROM services WHERE id = $1", s.Service.ID)
	}

	return nil
}

// CleanupService removes a service and all related data.
func (f *Fixtures) CleanupService(ctx context.Context, serviceID uuid.UUID) error {
	pool := f.db.Pool()

	// Delete in order of dependencies
	pool.Exec(ctx, "DELETE FROM test_results WHERE run_id IN (SELECT id FROM test_runs WHERE service_id = $1)", serviceID)
	pool.Exec(ctx, "DELETE FROM artifacts WHERE run_id IN (SELECT id FROM test_runs WHERE service_id = $1)", serviceID)
	pool.Exec(ctx, "DELETE FROM test_runs WHERE service_id = $1", serviceID)
	pool.Exec(ctx, "DELETE FROM test_definitions WHERE service_id = $1", serviceID)
	pool.Exec(ctx, "DELETE FROM notification_rules WHERE service_id = $1", serviceID)
	pool.Exec(ctx, "DELETE FROM scheduled_runs WHERE service_id = $1", serviceID)
	pool.Exec(ctx, "DELETE FROM flaky_tests WHERE service_id = $1", serviceID)
	pool.Exec(ctx, "DELETE FROM daily_stats WHERE service_id = $1", serviceID)
	pool.Exec(ctx, "DELETE FROM services WHERE id = $1", serviceID)

	return nil
}

// CleanupAgent removes an agent.
func (f *Fixtures) CleanupAgent(ctx context.Context, agentID uuid.UUID) error {
	_, err := f.db.Pool().Exec(ctx, "DELETE FROM agents WHERE id = $1", agentID)
	return err
}

// CleanupRun removes a run and all related data.
func (f *Fixtures) CleanupRun(ctx context.Context, runID uuid.UUID) error {
	pool := f.db.Pool()

	pool.Exec(ctx, "DELETE FROM test_results WHERE run_id = $1", runID)
	pool.Exec(ctx, "DELETE FROM artifacts WHERE run_id = $1", runID)
	_, err := pool.Exec(ctx, "DELETE FROM test_runs WHERE id = $1", runID)

	return err
}

// CleanupChannel removes a notification channel and all related rules.
func (f *Fixtures) CleanupChannel(ctx context.Context, channelID uuid.UUID) error {
	pool := f.db.Pool()

	pool.Exec(ctx, "DELETE FROM notification_rules WHERE channel_id = $1", channelID)
	_, err := pool.Exec(ctx, "DELETE FROM notification_channels WHERE id = $1", channelID)

	return err
}

// cleanupServices is a helper to clean up services on error.
func (f *Fixtures) cleanupServices(ctx context.Context, services []*database.Service) {
	for _, svc := range services {
		f.CleanupService(ctx, svc.ID)
	}
}

// mergeOptions merges fixture options, with later options taking precedence.
func mergeOptions(opts ...FixtureOptions) FixtureOptions {
	result := DefaultFixtureOptions()

	for _, opt := range opts {
		if opt.Name != "" {
			result.Name = opt.Name
		}
		if opt.Owner != "" {
			result.Owner = opt.Owner
		}
		if len(opt.Tags) > 0 {
			result.Tags = opt.Tags
		}
		if len(opt.NetworkZones) > 0 {
			result.NetworkZones = opt.NetworkZones
		}
		if opt.Status != "" {
			result.Status = opt.Status
		}
		if opt.Count > 0 {
			result.Count = opt.Count
		}
		if opt.Priority != 0 {
			result.Priority = opt.Priority
		}
		if opt.TriggerType != "" {
			result.TriggerType = opt.TriggerType
		}
		if opt.TriggeredBy != "" {
			result.TriggeredBy = opt.TriggeredBy
		}
	}

	return result
}

// RandomName generates a random name with a prefix.
func RandomName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
}

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T {
	return &v
}
