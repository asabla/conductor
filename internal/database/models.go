package database

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Service represents a registered microservice with test suites.
type Service struct {
	ID            uuid.UUID `json:"id" db:"id"`
	Name          string    `json:"name" db:"name"`
	DisplayName   *string   `json:"display_name,omitempty" db:"display_name"`
	GitURL        string    `json:"git_url" db:"git_url"`
	GitProvider   *string   `json:"git_provider,omitempty" db:"git_provider"`
	DefaultBranch string    `json:"default_branch" db:"default_branch"`
	NetworkZones  []string  `json:"network_zones,omitempty" db:"network_zones"`
	Owner         *string   `json:"owner,omitempty" db:"owner"`
	ContactSlack  *string   `json:"contact_slack,omitempty" db:"contact_slack"`
	ContactEmail  *string   `json:"contact_email,omitempty" db:"contact_email"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// TestDefinition defines an individual test or test suite that can be executed.
type TestDefinition struct {
	ID               uuid.UUID `json:"id" db:"id"`
	ServiceID        uuid.UUID `json:"service_id" db:"service_id"`
	Name             string    `json:"name" db:"name"`
	Description      *string   `json:"description,omitempty" db:"description"`
	ExecutionType    string    `json:"execution_type" db:"execution_type"` // subprocess, container
	Command          string    `json:"command" db:"command"`
	Args             []string  `json:"args,omitempty" db:"args"`
	TimeoutSeconds   int       `json:"timeout_seconds" db:"timeout_seconds"`
	ResultFile       *string   `json:"result_file,omitempty" db:"result_file"`
	ResultFormat     *string   `json:"result_format,omitempty" db:"result_format"` // junit, jest, playwright, go_test, tap, json
	ArtifactPatterns []string  `json:"artifact_patterns,omitempty" db:"artifact_patterns"`
	Tags             []string  `json:"tags,omitempty" db:"tags"`
	DependsOn        []string  `json:"depends_on,omitempty" db:"depends_on"`
	Retries          int       `json:"retries" db:"retries"`
	AllowFailure     bool      `json:"allow_failure" db:"allow_failure"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

// AgentStatus represents the current status of an agent.
type AgentStatus string

const (
	AgentStatusIdle     AgentStatus = "idle"
	AgentStatusBusy     AgentStatus = "busy"
	AgentStatusDraining AgentStatus = "draining"
	AgentStatusOffline  AgentStatus = "offline"
)

// Agent represents a test execution agent.
type Agent struct {
	ID              uuid.UUID   `json:"id" db:"id"`
	Name            string      `json:"name" db:"name"`
	Status          AgentStatus `json:"status" db:"status"`
	Version         *string     `json:"version,omitempty" db:"version"`
	NetworkZones    []string    `json:"network_zones,omitempty" db:"network_zones"`
	MaxParallel     int         `json:"max_parallel" db:"max_parallel"`
	DockerAvailable bool        `json:"docker_available" db:"docker_available"`
	LastHeartbeat   *time.Time  `json:"last_heartbeat,omitempty" db:"last_heartbeat"`
	RegisteredAt    time.Time   `json:"registered_at" db:"registered_at"`
}

// IsOnline returns true if the agent is considered online (received heartbeat within timeout).
func (a *Agent) IsOnline(timeout time.Duration) bool {
	if a.LastHeartbeat == nil {
		return false
	}
	return time.Since(*a.LastHeartbeat) < timeout
}

// RunStatus represents the status of a test run.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusPassed    RunStatus = "passed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusError     RunStatus = "error"
	RunStatusTimeout   RunStatus = "timeout"
	RunStatusCancelled RunStatus = "cancelled"
)

// TriggerType represents what triggered a test run.
type TriggerType string

const (
	TriggerTypeManual   TriggerType = "manual"
	TriggerTypeWebhook  TriggerType = "webhook"
	TriggerTypeSchedule TriggerType = "schedule"
)

// TestRun represents a test execution run.
type TestRun struct {
	ID           uuid.UUID    `json:"id" db:"id"`
	ServiceID    uuid.UUID    `json:"service_id" db:"service_id"`
	AgentID      *uuid.UUID   `json:"agent_id,omitempty" db:"agent_id"`
	Status       RunStatus    `json:"status" db:"status"`
	GitRef       *string      `json:"git_ref,omitempty" db:"git_ref"`
	GitSHA       *string      `json:"git_sha,omitempty" db:"git_sha"`
	TriggerType  *TriggerType `json:"trigger_type,omitempty" db:"trigger_type"`
	TriggeredBy  *string      `json:"triggered_by,omitempty" db:"triggered_by"`
	Priority     int          `json:"priority" db:"priority"`
	CreatedAt    time.Time    `json:"created_at" db:"created_at"`
	StartedAt    *time.Time   `json:"started_at,omitempty" db:"started_at"`
	FinishedAt   *time.Time   `json:"finished_at,omitempty" db:"finished_at"`
	TotalTests   int          `json:"total_tests" db:"total_tests"`
	PassedTests  int          `json:"passed_tests" db:"passed_tests"`
	FailedTests  int          `json:"failed_tests" db:"failed_tests"`
	SkippedTests int          `json:"skipped_tests" db:"skipped_tests"`
	ShardCount   int          `json:"shard_count" db:"shard_count"`
	ShardsDone   int          `json:"shards_completed" db:"shards_completed"`
	ShardsFailed int          `json:"shards_failed" db:"shards_failed"`
	MaxParallel  int          `json:"max_parallel_tests" db:"max_parallel_tests"`
	DurationMs   *int64       `json:"duration_ms,omitempty" db:"duration_ms"`
	ErrorMessage *string      `json:"error_message,omitempty" db:"error_message"`
}

// IsTerminal returns true if the run is in a terminal state.
func (r *TestRun) IsTerminal() bool {
	switch r.Status {
	case RunStatusPassed, RunStatusFailed, RunStatusError, RunStatusTimeout, RunStatusCancelled:
		return true
	default:
		return false
	}
}

// ResultStatus represents the status of an individual test result.
type ResultStatus string

const (
	ResultStatusPass  ResultStatus = "pass"
	ResultStatusFail  ResultStatus = "fail"
	ResultStatusSkip  ResultStatus = "skip"
	ResultStatusError ResultStatus = "error"
)

// TestResult stores individual test case results within a run.
type TestResult struct {
	ID               uuid.UUID    `json:"id" db:"id"`
	RunID            uuid.UUID    `json:"run_id" db:"run_id"`
	ShardID          *uuid.UUID   `json:"shard_id,omitempty" db:"shard_id"`
	TestDefinitionID *uuid.UUID   `json:"test_definition_id,omitempty" db:"test_definition_id"`
	TestName         string       `json:"test_name" db:"test_name"`
	SuiteName        *string      `json:"suite_name,omitempty" db:"suite_name"`
	Status           ResultStatus `json:"status" db:"status"`
	DurationMs       *int64       `json:"duration_ms,omitempty" db:"duration_ms"`
	ErrorMessage     *string      `json:"error_message,omitempty" db:"error_message"`
	StackTrace       *string      `json:"stack_trace,omitempty" db:"stack_trace"`
	Stdout           *string      `json:"stdout,omitempty" db:"stdout"`
	Stderr           *string      `json:"stderr,omitempty" db:"stderr"`
	RetryCount       int          `json:"retry_count" db:"retry_count"`
	CreatedAt        time.Time    `json:"created_at" db:"created_at"`
}

// ShardStatus represents the status of a run shard.
type ShardStatus string

const (
	ShardStatusPending   ShardStatus = "pending"
	ShardStatusRunning   ShardStatus = "running"
	ShardStatusPassed    ShardStatus = "passed"
	ShardStatusFailed    ShardStatus = "failed"
	ShardStatusError     ShardStatus = "error"
	ShardStatusCancelled ShardStatus = "cancelled"
)

// RunShard represents a shard of a test run.
type RunShard struct {
	ID           uuid.UUID   `json:"id" db:"id"`
	RunID        uuid.UUID   `json:"run_id" db:"run_id"`
	ShardIndex   int         `json:"shard_index" db:"shard_index"`
	ShardCount   int         `json:"shard_count" db:"shard_count"`
	Status       ShardStatus `json:"status" db:"status"`
	AgentID      *uuid.UUID  `json:"agent_id,omitempty" db:"agent_id"`
	TotalTests   int         `json:"total_tests" db:"total_tests"`
	PassedTests  int         `json:"passed_tests" db:"passed_tests"`
	FailedTests  int         `json:"failed_tests" db:"failed_tests"`
	SkippedTests int         `json:"skipped_tests" db:"skipped_tests"`
	ErrorMessage *string     `json:"error_message,omitempty" db:"error_message"`
	StartedAt    *time.Time  `json:"started_at,omitempty" db:"started_at"`
	FinishedAt   *time.Time  `json:"finished_at,omitempty" db:"finished_at"`
	CreatedAt    time.Time   `json:"created_at" db:"created_at"`
}

// Artifact represents a test artifact stored in S3/MinIO.
type Artifact struct {
	ID          uuid.UUID `json:"id" db:"id"`
	RunID       uuid.UUID `json:"run_id" db:"run_id"`
	Name        string    `json:"name" db:"name"`
	Path        string    `json:"path" db:"path"` // S3 object path
	ContentType *string   `json:"content_type,omitempty" db:"content_type"`
	SizeBytes   *int64    `json:"size_bytes,omitempty" db:"size_bytes"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// ChannelType represents the type of notification channel.
type ChannelType string

const (
	ChannelTypeSlack   ChannelType = "slack"
	ChannelTypeEmail   ChannelType = "email"
	ChannelTypeWebhook ChannelType = "webhook"
	ChannelTypeTeams   ChannelType = "teams"
)

// NotificationChannel defines a notification destination.
type NotificationChannel struct {
	ID        uuid.UUID       `json:"id" db:"id"`
	Name      string          `json:"name" db:"name"`
	Type      ChannelType     `json:"type" db:"type"`
	Config    json.RawMessage `json:"config" db:"config"`
	Enabled   bool            `json:"enabled" db:"enabled"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

// SlackChannelConfig holds Slack-specific configuration.
type SlackChannelConfig struct {
	WebhookURL string `json:"webhook_url"`
	Channel    string `json:"channel,omitempty"`
	Username   string `json:"username,omitempty"`
	IconEmoji  string `json:"icon_emoji,omitempty"`
	Token      string `json:"token,omitempty"`
}

// EmailChannelConfig holds email-specific configuration.
type EmailChannelConfig struct {
	Recipients   []string `json:"recipients"`
	CC           []string `json:"cc,omitempty"`
	SMTPConfigID string   `json:"smtp_config_id,omitempty"`
	IncludeLogs  bool     `json:"include_logs,omitempty"`
}

// WebhookChannelConfig holds webhook-specific configuration.
type WebhookChannelConfig struct {
	URL      string            `json:"url"`
	Method   string            `json:"method,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Template string            `json:"template,omitempty"`
}

// TriggerEvent represents events that can trigger notifications.
type TriggerEvent string

const (
	TriggerEventFailure  TriggerEvent = "failure"
	TriggerEventRecovery TriggerEvent = "recovery"
	TriggerEventFlaky    TriggerEvent = "flaky"
	TriggerEventAlways   TriggerEvent = "always"
)

// NotificationRule defines when and what notifications to send.
type NotificationRule struct {
	ID        uuid.UUID      `json:"id" db:"id"`
	ChannelID uuid.UUID      `json:"channel_id" db:"channel_id"`
	ServiceID *uuid.UUID     `json:"service_id,omitempty" db:"service_id"` // NULL means all services
	TriggerOn []TriggerEvent `json:"trigger_on" db:"trigger_on"`
	Enabled   bool           `json:"enabled" db:"enabled"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
}

// ScheduledRun defines a recurring test run schedule.
type ScheduledRun struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	ServiceID      uuid.UUID  `json:"service_id" db:"service_id"`
	Name           string     `json:"name" db:"name"`
	CronExpression string     `json:"cron_expression" db:"cron_expression"`
	GitRef         string     `json:"git_ref" db:"git_ref"`
	TestFilter     []string   `json:"test_filter,omitempty" db:"test_filter"`
	Enabled        bool       `json:"enabled" db:"enabled"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty" db:"last_run_at"`
	NextRunAt      *time.Time `json:"next_run_at,omitempty" db:"next_run_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// DailyStats holds pre-aggregated daily statistics per service.
type DailyStats struct {
	ID            int64         `json:"id" db:"id"`
	ServiceID     uuid.UUID     `json:"service_id" db:"service_id"`
	Date          time.Time     `json:"date" db:"date"`
	TotalRuns     int           `json:"total_runs" db:"total_runs"`
	PassedRuns    int           `json:"passed_runs" db:"passed_runs"`
	FailedRuns    int           `json:"failed_runs" db:"failed_runs"`
	TotalTests    int           `json:"total_tests" db:"total_tests"`
	PassedTests   int           `json:"passed_tests" db:"passed_tests"`
	FailedTests   int           `json:"failed_tests" db:"failed_tests"`
	AvgDurationMs sql.NullInt64 `json:"avg_duration_ms,omitempty" db:"avg_duration_ms"`
	P50DurationMs sql.NullInt64 `json:"p50_duration_ms,omitempty" db:"p50_duration_ms"`
	P95DurationMs sql.NullInt64 `json:"p95_duration_ms,omitempty" db:"p95_duration_ms"`
}

// FlakyTest tracks tests that exhibit flaky behavior.
type FlakyTest struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	ServiceID      uuid.UUID  `json:"service_id" db:"service_id"`
	TestName       string     `json:"test_name" db:"test_name"`
	FlakinessScore float64    `json:"flakiness_score" db:"flakiness_score"`
	TotalRuns      int        `json:"total_runs" db:"total_runs"`
	FlakyRuns      int        `json:"flaky_runs" db:"flaky_runs"`
	FirstDetected  time.Time  `json:"first_detected_at" db:"first_detected_at"`
	LastFlakyAt    *time.Time `json:"last_flaky_at,omitempty" db:"last_flaky_at"`
	Quarantined    bool       `json:"quarantined" db:"quarantined"`
	QuarantinedAt  *time.Time `json:"quarantined_at,omitempty" db:"quarantined_at"`
	QuarantinedBy  *string    `json:"quarantined_by,omitempty" db:"quarantined_by"`
	Notes          *string    `json:"notes,omitempty" db:"notes"`
}

// TestHistory stores recent test execution history for trend analysis.
type TestHistory struct {
	ID         int64        `json:"id" db:"id"`
	ServiceID  uuid.UUID    `json:"service_id" db:"service_id"`
	TestName   string       `json:"test_name" db:"test_name"`
	RunID      uuid.UUID    `json:"run_id" db:"run_id"`
	Status     ResultStatus `json:"status" db:"status"`
	DurationMs *int64       `json:"duration_ms,omitempty" db:"duration_ms"`
	ExecutedAt time.Time    `json:"executed_at" db:"executed_at"`
}

// ServiceHealthSummary provides a quick overview of service health metrics.
type ServiceHealthSummary struct {
	ServiceID       uuid.UUID  `json:"service_id" db:"service_id"`
	ServiceName     string     `json:"service_name" db:"service_name"`
	DisplayName     *string    `json:"display_name,omitempty" db:"display_name"`
	RunsLast7Days   int        `json:"runs_last_7_days" db:"runs_last_7_days"`
	PassedLast7Days int        `json:"passed_last_7_days" db:"passed_last_7_days"`
	FailedLast7Days int        `json:"failed_last_7_days" db:"failed_last_7_days"`
	PassRate7Days   *float64   `json:"pass_rate_7_days,omitempty" db:"pass_rate_7_days"`
	FlakyTestCount  int        `json:"flaky_test_count" db:"flaky_test_count"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty" db:"last_run_at"`
	LastRunStatus   *RunStatus `json:"last_run_status,omitempty" db:"last_run_status"`
}

// Pagination parameters for list operations.
type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// DefaultPagination returns default pagination settings.
func DefaultPagination() Pagination {
	return Pagination{
		Limit:  20,
		Offset: 0,
	}
}

// NullString creates a pointer to a string, returning nil for empty strings.
func NullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// NullUUID creates a pointer to a UUID, returning nil for zero UUIDs.
func NullUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// NullInt64 creates a pointer to an int64, returning nil for zero values.
func NullInt64(n int64) *int64 {
	if n == 0 {
		return nil
	}
	return &n
}

// NullTime creates a pointer to a time.Time, returning nil for zero time.
func NullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
