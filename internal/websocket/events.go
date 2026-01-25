package websocket

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// EventPublisher publishes real-time events to WebSocket clients.
type EventPublisher interface {
	// PublishRunUpdate publishes a test run status update.
	PublishRunUpdate(run RunEvent) error

	// PublishAgentUpdate publishes an agent status update.
	PublishAgentUpdate(agent AgentEvent) error

	// PublishLogChunk publishes a real-time log chunk.
	PublishLogChunk(runID uuid.UUID, chunk LogChunk) error

	// PublishTestResult publishes an individual test result.
	PublishTestResult(runID uuid.UUID, result TestResultEvent) error

	// PublishServiceUpdate publishes a service update event.
	PublishServiceUpdate(service ServiceEvent) error
}

// RunEvent represents a test run event for publishing.
type RunEvent struct {
	RunID        uuid.UUID
	ServiceID    uuid.UUID
	Status       string
	TotalTests   int
	PassedTests  int
	FailedTests  int
	SkippedTests int
	DurationMs   *int64
	ErrorMessage *string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

// AgentEvent represents an agent event for publishing.
type AgentEvent struct {
	AgentID         uuid.UUID
	Name            string
	Status          string
	LastHeartbeat   *time.Time
	ActiveJobs      int
	Version         *string
	DockerAvailable bool
}

// LogChunk represents a chunk of log output.
type LogChunk struct {
	Sequence  int64
	Stream    string // "stdout" or "stderr"
	Data      string
	Timestamp time.Time
}

// TestResultEvent represents a test result for publishing.
type TestResultEvent struct {
	TestName     string
	SuiteName    *string
	Status       string
	DurationMs   *int64
	ErrorMessage *string
}

// ServiceEvent represents a service event for publishing.
type ServiceEvent struct {
	ServiceID     uuid.UUID
	Name          string
	LastRunStatus *string
	LastRunAt     *time.Time
}

// Publisher implements EventPublisher using the WebSocket hub.
type Publisher struct {
	hub    *Hub
	logger zerolog.Logger
}

// NewPublisher creates a new event publisher.
func NewPublisher(hub *Hub, logger zerolog.Logger) *Publisher {
	return &Publisher{
		hub:    hub,
		logger: logger.With().Str("component", "websocket_publisher").Logger(),
	}
}

// PublishRunUpdate publishes a test run status update.
func (p *Publisher) PublishRunUpdate(run RunEvent) error {
	payload := RunUpdatePayload{
		RunID:        run.RunID,
		ServiceID:    run.ServiceID,
		Status:       run.Status,
		TotalTests:   run.TotalTests,
		PassedTests:  run.PassedTests,
		FailedTests:  run.FailedTests,
		SkippedTests: run.SkippedTests,
		DurationMs:   run.DurationMs,
		ErrorMessage: run.ErrorMessage,
		StartedAt:    run.StartedAt,
		FinishedAt:   run.FinishedAt,
	}

	msg, err := NewMessage(MessageTypeRunUpdate, payload)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to create run update message")
		return err
	}

	// Publish to run-specific room
	runRoom := RoomName(RoomTypeRun, run.RunID.String())
	if err := p.hub.BroadcastMessage(runRoom, msg); err != nil {
		p.logger.Error().Err(err).Str("room", runRoom).Msg("failed to broadcast to run room")
	}

	// Also publish to service room for service-wide listeners
	serviceRoom := RoomName(RoomTypeService, run.ServiceID.String())
	if err := p.hub.BroadcastMessage(serviceRoom, msg); err != nil {
		p.logger.Error().Err(err).Str("room", serviceRoom).Msg("failed to broadcast to service room")
	}

	// Publish to global room for dashboard-wide listeners
	globalRoom := RoomName(RoomTypeGlobal, "runs")
	if err := p.hub.BroadcastMessage(globalRoom, msg); err != nil {
		p.logger.Error().Err(err).Str("room", globalRoom).Msg("failed to broadcast to global room")
	}

	p.logger.Debug().
		Str("run_id", run.RunID.String()).
		Str("status", run.Status).
		Msg("published run update")

	return nil
}

// PublishAgentUpdate publishes an agent status update.
func (p *Publisher) PublishAgentUpdate(agent AgentEvent) error {
	payload := AgentUpdatePayload{
		AgentID:         agent.AgentID,
		Name:            agent.Name,
		Status:          agent.Status,
		LastHeartbeat:   agent.LastHeartbeat,
		ActiveJobs:      agent.ActiveJobs,
		Version:         agent.Version,
		DockerAvailable: agent.DockerAvailable,
	}

	msg, err := NewMessage(MessageTypeAgentUpdate, payload)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to create agent update message")
		return err
	}

	// Publish to agent-specific room
	agentRoom := RoomName(RoomTypeAgent, agent.AgentID.String())
	if err := p.hub.BroadcastMessage(agentRoom, msg); err != nil {
		p.logger.Error().Err(err).Str("room", agentRoom).Msg("failed to broadcast to agent room")
	}

	// Publish to global agents room
	globalRoom := RoomName(RoomTypeGlobal, "agents")
	if err := p.hub.BroadcastMessage(globalRoom, msg); err != nil {
		p.logger.Error().Err(err).Str("room", globalRoom).Msg("failed to broadcast to global room")
	}

	p.logger.Debug().
		Str("agent_id", agent.AgentID.String()).
		Str("status", agent.Status).
		Msg("published agent update")

	return nil
}

// PublishLogChunk publishes a real-time log chunk.
func (p *Publisher) PublishLogChunk(runID uuid.UUID, chunk LogChunk) error {
	payload := LogChunkPayload{
		RunID:     runID,
		Sequence:  chunk.Sequence,
		Stream:    chunk.Stream,
		Data:      chunk.Data,
		Timestamp: chunk.Timestamp,
	}

	msg, err := NewMessage(MessageTypeLogChunk, payload)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to create log chunk message")
		return err
	}

	// Publish to run-specific room only (logs can be high volume)
	runRoom := RoomName(RoomTypeRun, runID.String())
	if err := p.hub.BroadcastMessage(runRoom, msg); err != nil {
		p.logger.Error().Err(err).Str("room", runRoom).Msg("failed to broadcast log chunk")
		return err
	}

	return nil
}

// PublishTestResult publishes an individual test result.
func (p *Publisher) PublishTestResult(runID uuid.UUID, result TestResultEvent) error {
	payload := TestResultPayload{
		RunID:        runID,
		TestName:     result.TestName,
		SuiteName:    result.SuiteName,
		Status:       result.Status,
		DurationMs:   result.DurationMs,
		ErrorMessage: result.ErrorMessage,
	}

	msg, err := NewMessage(MessageTypeTestResult, payload)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to create test result message")
		return err
	}

	// Publish to run-specific room
	runRoom := RoomName(RoomTypeRun, runID.String())
	if err := p.hub.BroadcastMessage(runRoom, msg); err != nil {
		p.logger.Error().Err(err).Str("room", runRoom).Msg("failed to broadcast test result")
		return err
	}

	p.logger.Debug().
		Str("run_id", runID.String()).
		Str("test_name", result.TestName).
		Str("status", result.Status).
		Msg("published test result")

	return nil
}

// PublishServiceUpdate publishes a service update event.
func (p *Publisher) PublishServiceUpdate(service ServiceEvent) error {
	payload := ServiceUpdatePayload{
		ServiceID:     service.ServiceID,
		Name:          service.Name,
		LastRunStatus: service.LastRunStatus,
		LastRunAt:     service.LastRunAt,
	}

	msg, err := NewMessage(MessageTypeServiceUpdate, payload)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to create service update message")
		return err
	}

	// Publish to service-specific room
	serviceRoom := RoomName(RoomTypeService, service.ServiceID.String())
	if err := p.hub.BroadcastMessage(serviceRoom, msg); err != nil {
		p.logger.Error().Err(err).Str("room", serviceRoom).Msg("failed to broadcast to service room")
	}

	// Publish to global services room
	globalRoom := RoomName(RoomTypeGlobal, "services")
	if err := p.hub.BroadcastMessage(globalRoom, msg); err != nil {
		p.logger.Error().Err(err).Str("room", globalRoom).Msg("failed to broadcast to global room")
	}

	p.logger.Debug().
		Str("service_id", service.ServiceID.String()).
		Str("name", service.Name).
		Msg("published service update")

	return nil
}

// NoopPublisher is a no-op implementation of EventPublisher.
type NoopPublisher struct{}

// PublishRunUpdate does nothing.
func (NoopPublisher) PublishRunUpdate(RunEvent) error { return nil }

// PublishAgentUpdate does nothing.
func (NoopPublisher) PublishAgentUpdate(AgentEvent) error { return nil }

// PublishLogChunk does nothing.
func (NoopPublisher) PublishLogChunk(uuid.UUID, LogChunk) error { return nil }

// PublishTestResult does nothing.
func (NoopPublisher) PublishTestResult(uuid.UUID, TestResultEvent) error { return nil }

// PublishServiceUpdate does nothing.
func (NoopPublisher) PublishServiceUpdate(ServiceEvent) error { return nil }
