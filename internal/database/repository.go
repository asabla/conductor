package database

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ServiceRepository defines the interface for service data operations.
type ServiceRepository interface {
	// Create creates a new service.
	Create(ctx context.Context, svc *Service) error

	// Get retrieves a service by ID.
	Get(ctx context.Context, id uuid.UUID) (*Service, error)

	// GetByName retrieves a service by name.
	GetByName(ctx context.Context, name string) (*Service, error)

	// Update updates an existing service.
	Update(ctx context.Context, svc *Service) error

	// Delete deletes a service by ID.
	Delete(ctx context.Context, id uuid.UUID) error

	// List returns services with pagination.
	List(ctx context.Context, page Pagination) ([]Service, error)

	// Count returns the total number of services.
	Count(ctx context.Context) (int64, error)

	// ListByOwner returns services owned by a specific owner.
	ListByOwner(ctx context.Context, owner string, page Pagination) ([]Service, error)

	// Search searches services by name pattern.
	Search(ctx context.Context, query string, page Pagination) ([]Service, error)
}

// TestDefinitionRepository defines the interface for test definition data operations.
type TestDefinitionRepository interface {
	// Create creates a new test definition.
	Create(ctx context.Context, def *TestDefinition) error

	// Get retrieves a test definition by ID.
	Get(ctx context.Context, id uuid.UUID) (*TestDefinition, error)

	// Update updates a test definition.
	Update(ctx context.Context, def *TestDefinition) error

	// Delete deletes a test definition.
	Delete(ctx context.Context, id uuid.UUID) error

	// ListByService returns test definitions for a service.
	ListByService(ctx context.Context, serviceID uuid.UUID, page Pagination) ([]TestDefinition, error)

	// ListByTags returns test definitions matching any of the given tags.
	ListByTags(ctx context.Context, serviceID uuid.UUID, tags []string, page Pagination) ([]TestDefinition, error)
}

// AgentRepository defines the interface for agent data operations.
type AgentRepository interface {
	// Create creates a new agent.
	Create(ctx context.Context, agent *Agent) error

	// Get retrieves an agent by ID.
	Get(ctx context.Context, id uuid.UUID) (*Agent, error)

	// GetByName retrieves an agent by name.
	GetByName(ctx context.Context, name string) (*Agent, error)

	// Update updates an agent.
	Update(ctx context.Context, agent *Agent) error

	// Delete deletes an agent.
	Delete(ctx context.Context, id uuid.UUID) error

	// List returns agents with pagination.
	List(ctx context.Context, page Pagination) ([]Agent, error)

	// ListByStatus returns agents with a specific status.
	ListByStatus(ctx context.Context, status AgentStatus, page Pagination) ([]Agent, error)

	// UpdateStatus updates only the agent's status.
	UpdateStatus(ctx context.Context, id uuid.UUID, status AgentStatus) error

	// UpdateHeartbeat updates the agent's heartbeat time and status.
	UpdateHeartbeat(ctx context.Context, id uuid.UUID, status AgentStatus) error

	// GetAvailable returns agents available to run tests for services in the given zones.
	GetAvailable(ctx context.Context, zones []string, limit int) ([]Agent, error)

	// MarkOfflineAgents marks agents as offline if they haven't sent a heartbeat recently.
	MarkOfflineAgents(ctx context.Context) (int64, error)

	// CountByStatus returns the count of agents grouped by status.
	CountByStatus(ctx context.Context) (map[AgentStatus]int64, error)
}

// TestRunRepository defines the interface for test run data operations.
type TestRunRepository interface {
	// Create creates a new test run.
	Create(ctx context.Context, run *TestRun) error

	// Get retrieves a test run by ID.
	Get(ctx context.Context, id uuid.UUID) (*TestRun, error)

	// Update updates a test run.
	Update(ctx context.Context, run *TestRun) error

	// UpdateStatus updates only the run's status.
	UpdateStatus(ctx context.Context, id uuid.UUID, status RunStatus) error

	// Start marks a run as started with the given agent.
	Start(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error

	// Finish marks a run as finished with results.
	Finish(ctx context.Context, id uuid.UUID, status RunStatus, results RunResults) error

	// List returns test runs with pagination.
	List(ctx context.Context, page Pagination) ([]TestRun, error)

	// ListByService returns test runs for a service.
	ListByService(ctx context.Context, serviceID uuid.UUID, page Pagination) ([]TestRun, error)

	// ListByStatus returns test runs with a specific status.
	ListByStatus(ctx context.Context, status RunStatus, page Pagination) ([]TestRun, error)

	// ListByServiceAndStatus returns test runs for a service with a specific status.
	ListByServiceAndStatus(ctx context.Context, serviceID uuid.UUID, status RunStatus, page Pagination) ([]TestRun, error)

	// ListByDateRange returns test runs within a date range.
	ListByDateRange(ctx context.Context, start, end time.Time, page Pagination) ([]TestRun, error)

	// GetPending returns pending runs ordered by priority.
	GetPending(ctx context.Context, limit int) ([]TestRun, error)

	// GetRunning returns currently running tests.
	GetRunning(ctx context.Context) ([]TestRun, error)

	// Count returns the total number of test runs.
	Count(ctx context.Context) (int64, error)

	// CountByStatus returns the count of runs grouped by status.
	CountByStatus(ctx context.Context) (map[RunStatus]int64, error)
}

// RunResults holds the summary results for a completed test run.
type RunResults struct {
	TotalTests   int
	PassedTests  int
	FailedTests  int
	SkippedTests int
	DurationMs   int64
	ErrorMessage string
}

// ResultRepository defines the interface for test result data operations.
type ResultRepository interface {
	// Create creates a new test result.
	Create(ctx context.Context, result *TestResult) error

	// BatchCreate creates multiple test results in a single operation.
	BatchCreate(ctx context.Context, results []TestResult) error

	// Get retrieves a test result by ID.
	Get(ctx context.Context, id uuid.UUID) (*TestResult, error)

	// ListByRun returns all results for a test run.
	ListByRun(ctx context.Context, runID uuid.UUID) ([]TestResult, error)

	// ListByRunAndStatus returns results for a run with a specific status.
	ListByRunAndStatus(ctx context.Context, runID uuid.UUID, status ResultStatus) ([]TestResult, error)

	// CountByRun returns the count of results grouped by status for a run.
	CountByRun(ctx context.Context, runID uuid.UUID) (map[ResultStatus]int64, error)

	// DeleteByRun deletes all results for a run.
	DeleteByRun(ctx context.Context, runID uuid.UUID) error
}

// ArtifactRepository defines the interface for artifact data operations.
type ArtifactRepository interface {
	// Create creates a new artifact record.
	Create(ctx context.Context, artifact *Artifact) error

	// Get retrieves an artifact by ID.
	Get(ctx context.Context, id uuid.UUID) (*Artifact, error)

	// ListByRun returns all artifacts for a test run.
	ListByRun(ctx context.Context, runID uuid.UUID) ([]Artifact, error)

	// ListOlderThan returns artifacts older than a timestamp.
	ListOlderThan(ctx context.Context, before time.Time, limit int) ([]Artifact, error)

	// Delete deletes an artifact record.
	Delete(ctx context.Context, id uuid.UUID) error

	// DeleteByRun deletes all artifact records for a run.
	DeleteByRun(ctx context.Context, runID uuid.UUID) error
}

// RunShardRepository defines operations for run shard data.
type RunShardRepository interface {
	// Create creates a new shard record.
	Create(ctx context.Context, shard *RunShard) error

	// Get retrieves a shard by ID.
	Get(ctx context.Context, id uuid.UUID) (*RunShard, error)

	// ListByRun lists shards for a run.
	ListByRun(ctx context.Context, runID uuid.UUID) ([]RunShard, error)

	// UpdateStatus updates a shard status.
	UpdateStatus(ctx context.Context, id uuid.UUID, status ShardStatus) error

	// Start marks a shard as started.
	Start(ctx context.Context, id uuid.UUID, agentID uuid.UUID) error

	// Finish marks a shard as finished with counts.
	Finish(ctx context.Context, id uuid.UUID, status ShardStatus, results RunResults) error

	// DeleteByRun deletes shard records for a run.
	DeleteByRun(ctx context.Context, runID uuid.UUID) error
}

// NotificationRepository defines the interface for notification data operations.
type NotificationRepository interface {
	// CreateChannel creates a new notification channel.
	CreateChannel(ctx context.Context, channel *NotificationChannel) error

	// GetChannel retrieves a channel by ID.
	GetChannel(ctx context.Context, id uuid.UUID) (*NotificationChannel, error)

	// UpdateChannel updates a notification channel.
	UpdateChannel(ctx context.Context, channel *NotificationChannel) error

	// DeleteChannel deletes a notification channel.
	DeleteChannel(ctx context.Context, id uuid.UUID) error

	// ListChannels returns all channels with pagination.
	ListChannels(ctx context.Context, page Pagination) ([]NotificationChannel, error)

	// ListEnabledChannels returns all enabled channels.
	ListEnabledChannels(ctx context.Context) ([]NotificationChannel, error)

	// CreateRule creates a new notification rule.
	CreateRule(ctx context.Context, rule *NotificationRule) error

	// GetRule retrieves a rule by ID.
	GetRule(ctx context.Context, id uuid.UUID) (*NotificationRule, error)

	// UpdateRule updates a notification rule.
	UpdateRule(ctx context.Context, rule *NotificationRule) error

	// DeleteRule deletes a notification rule.
	DeleteRule(ctx context.Context, id uuid.UUID) error

	// ListRulesByService returns rules for a service (including global rules).
	ListRulesByService(ctx context.Context, serviceID uuid.UUID) ([]NotificationRule, error)

	// ListRulesByChannel returns rules for a channel.
	ListRulesByChannel(ctx context.Context, channelID uuid.UUID) ([]NotificationRule, error)
}

// ScheduleRepository defines the interface for scheduled run data operations.
type ScheduleRepository interface {
	// Create creates a new scheduled run.
	Create(ctx context.Context, schedule *ScheduledRun) error

	// Get retrieves a scheduled run by ID.
	Get(ctx context.Context, id uuid.UUID) (*ScheduledRun, error)

	// Update updates a scheduled run.
	Update(ctx context.Context, schedule *ScheduledRun) error

	// Delete deletes a scheduled run.
	Delete(ctx context.Context, id uuid.UUID) error

	// ListByService returns schedules for a service.
	ListByService(ctx context.Context, serviceID uuid.UUID) ([]ScheduledRun, error)

	// ListDue returns schedules that are due to run.
	ListDue(ctx context.Context) ([]ScheduledRun, error)

	// UpdateAfterRun updates a schedule after it has executed.
	UpdateAfterRun(ctx context.Context, id uuid.UUID, nextRunAt time.Time) error
}

// AnalyticsRepository defines the interface for analytics data operations.
type AnalyticsRepository interface {
	// UpsertDailyStats inserts or updates daily statistics.
	UpsertDailyStats(ctx context.Context, stats *DailyStats) error

	// GetDailyStats retrieves daily stats for a service within a date range.
	GetDailyStats(ctx context.Context, serviceID uuid.UUID, start, end time.Time) ([]DailyStats, error)

	// UpsertFlakyTest inserts or updates a flaky test record.
	UpsertFlakyTest(ctx context.Context, serviceID uuid.UUID, testName string, score float64, runs, flakyRuns int) error

	// ListFlakyTests returns flaky tests for a service.
	ListFlakyTests(ctx context.Context, serviceID uuid.UUID, page Pagination) ([]FlakyTest, error)

	// QuarantineTest quarantines a flaky test.
	QuarantineTest(ctx context.Context, id uuid.UUID, by string) error

	// UnquarantineTest removes quarantine from a test.
	UnquarantineTest(ctx context.Context, id uuid.UUID) error

	// RecordTestHistory records a test execution in history.
	RecordTestHistory(ctx context.Context, history *TestHistory) error

	// GetTestHistory retrieves recent history for a test.
	GetTestHistory(ctx context.Context, serviceID uuid.UUID, testName string, limit int) ([]TestHistory, error)

	// GetServiceHealthSummary retrieves health summaries for all services.
	GetServiceHealthSummary(ctx context.Context) ([]ServiceHealthSummary, error)

	// GetServiceHealthSummaryByID retrieves health summary for a specific service.
	GetServiceHealthSummaryByID(ctx context.Context, serviceID uuid.UUID) (*ServiceHealthSummary, error)
}

// Repositories aggregates all repository interfaces.
type Repositories struct {
	Services        ServiceRepository
	TestDefinitions TestDefinitionRepository
	Agents          AgentRepository
	Runs            TestRunRepository
	RunShards       RunShardRepository
	Results         ResultRepository
	Artifacts       ArtifactRepository
	Notifications   NotificationRepository
	Schedules       ScheduleRepository
	Analytics       AnalyticsRepository
}

// NewRepositories creates all repository implementations backed by the given database.
func NewRepositories(db *DB) *Repositories {
	return &Repositories{
		Services:        NewServiceRepo(db),
		TestDefinitions: NewTestDefinitionRepo(db),
		Agents:          NewAgentRepo(db),
		Runs:            NewRunRepo(db),
		RunShards:       NewRunShardRepo(db),
		Results:         NewResultRepo(db),
		Artifacts:       NewArtifactRepo(db),
		Notifications:   NewNotificationRepo(db),
		Schedules:       NewScheduleRepo(db),
		Analytics:       NewAnalyticsRepo(db),
	}
}
