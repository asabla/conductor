// Package notification provides notification services for alerting users
// about test run outcomes and system events.
package notification

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// NotificationType represents the type of notification event.
type NotificationType string

const (
	// NotificationTypeRunStarted indicates a test run has started.
	NotificationTypeRunStarted NotificationType = "run_started"
	// NotificationTypeRunPassed indicates a test run passed.
	NotificationTypeRunPassed NotificationType = "run_passed"
	// NotificationTypeRunFailed indicates a test run failed.
	NotificationTypeRunFailed NotificationType = "run_failed"
	// NotificationTypeRunRecovered indicates a test run recovered after failure.
	NotificationTypeRunRecovered NotificationType = "run_recovered"
	// NotificationTypeFlakyDetected indicates a flaky test was detected.
	NotificationTypeFlakyDetected NotificationType = "flaky_detected"
	// NotificationTypeTestQuarantined indicates a test was quarantined.
	NotificationTypeTestQuarantined NotificationType = "test_quarantined"
	// NotificationTypeRunTimeout indicates a test run timed out.
	NotificationTypeRunTimeout NotificationType = "run_timeout"
	// NotificationTypeRunError indicates a test run encountered an error.
	NotificationTypeRunError NotificationType = "run_error"
	// NotificationTypeAgentOffline indicates an agent went offline.
	NotificationTypeAgentOffline NotificationType = "agent_offline"
	// NotificationTypeAgentOnline indicates an agent came online.
	NotificationTypeAgentOnline NotificationType = "agent_online"
	// NotificationTypeTest indicates a test notification.
	NotificationTypeTest NotificationType = "test"
)

// Notification represents a notification to be sent.
type Notification struct {
	// ID is the unique identifier for this notification.
	ID uuid.UUID
	// Type is the notification type.
	Type NotificationType
	// ServiceID is the ID of the related service (if applicable).
	ServiceID *uuid.UUID
	// ServiceName is the name of the related service.
	ServiceName string
	// RunID is the ID of the related run (if applicable).
	RunID *uuid.UUID
	// Message is the notification message.
	Message string
	// Title is the notification title/subject.
	Title string
	// Summary contains run summary information.
	Summary *RunSummary
	// URL is a link to more details.
	URL string
	// CreatedAt is when the notification was created.
	CreatedAt time.Time
	// Metadata contains additional key-value data.
	Metadata map[string]string
}

// RunSummary contains summary information about a test run.
type RunSummary struct {
	TotalTests   int
	PassedTests  int
	FailedTests  int
	SkippedTests int
	DurationMs   int64
	Branch       string
	CommitSHA    string
	ErrorMessage string
}

// Channel defines the interface for notification channels.
type Channel interface {
	// Type returns the channel type.
	Type() database.ChannelType
	// Send sends a notification through the channel.
	Send(ctx context.Context, notification *Notification) error
	// Validate validates the channel configuration.
	Validate() error
}

// SendResult represents the result of sending a notification.
type SendResult struct {
	// ChannelID is the channel that was used.
	ChannelID uuid.UUID
	// ChannelType is the type of channel.
	ChannelType database.ChannelType
	// Success indicates if the send was successful.
	Success bool
	// Error contains the error message if failed.
	Error string
	// LatencyMs is the time taken to send in milliseconds.
	LatencyMs int64
	// SentAt is when the notification was sent.
	SentAt time.Time
}

// Event represents an event that may trigger notifications.
type Event struct {
	// Type is the event type.
	Type NotificationType
	// ServiceID is the ID of the related service.
	ServiceID uuid.UUID
	// ServiceName is the name of the service.
	ServiceName string
	// RunID is the ID of the related run (if applicable).
	RunID *uuid.UUID
	// Run contains the test run data (if applicable).
	Run *database.TestRun
	// PreviousRun contains the previous run for comparison (for recovery detection).
	PreviousRun *database.TestRun
	// Agent contains agent data (for agent events).
	Agent *database.Agent
	// FlakyTest contains flaky test data (for flaky detection).
	FlakyTest *database.FlakyTest
	// Timestamp is when the event occurred.
	Timestamp time.Time
	// Metadata contains additional event data.
	Metadata map[string]string
}

// mapTriggerEvent maps a NotificationType to a database TriggerEvent.
func mapTriggerEvent(nt NotificationType) database.TriggerEvent {
	switch nt {
	case NotificationTypeRunFailed, NotificationTypeRunError, NotificationTypeRunTimeout:
		return database.TriggerEventFailure
	case NotificationTypeRunRecovered:
		return database.TriggerEventRecovery
	case NotificationTypeFlakyDetected:
		return database.TriggerEventFlaky
	case NotificationTypeTestQuarantined:
		return database.TriggerEventFlaky
	default:
		return database.TriggerEventAlways
	}
}
