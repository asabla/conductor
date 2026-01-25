package server

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/database"
)

// AgentServiceDeps defines the dependencies for the agent service.
type AgentServiceDeps struct {
	// AgentRepo handles agent persistence.
	AgentRepo AgentRepository
	// RunRepo handles run persistence.
	RunRepo RunRepository
	// Scheduler handles work assignment.
	Scheduler WorkScheduler
	// HeartbeatTimeout is the duration after which an agent is considered offline.
	HeartbeatTimeout time.Duration
	// ServerVersion is the version of the control plane server.
	ServerVersion string
}

// AgentRepository defines the interface for agent persistence.
type AgentRepository interface {
	Create(ctx context.Context, agent *database.Agent) error
	Update(ctx context.Context, agent *database.Agent) error
	GetByID(ctx context.Context, id uuid.UUID) (*database.Agent, error)
	UpdateHeartbeat(ctx context.Context, id uuid.UUID, status database.AgentStatus) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status database.AgentStatus) error
	List(ctx context.Context, filter AgentFilter, pagination database.Pagination) ([]*database.Agent, int, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// AgentFilter defines filtering options for listing agents.
type AgentFilter struct {
	Statuses    []database.AgentStatus
	NetworkZone string
	Labels      map[string]string
	Query       string
}

// WorkScheduler handles work assignment to agents.
type WorkScheduler interface {
	// AssignWork finds and assigns pending work to an agent.
	AssignWork(ctx context.Context, agentID uuid.UUID, capabilities *conductorv1.Capabilities) (*conductorv1.AssignWork, error)
	// CancelWork cancels an assigned work item.
	CancelWork(ctx context.Context, runID uuid.UUID, reason string) error
	// HandleWorkAccepted processes a work acceptance from an agent.
	HandleWorkAccepted(ctx context.Context, agentID uuid.UUID, runID uuid.UUID) error
	// HandleWorkRejected processes a work rejection from an agent.
	HandleWorkRejected(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, reason string) error
	// HandleRunComplete processes run completion from an agent.
	HandleRunComplete(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, result *conductorv1.RunComplete) error
}

// connectedAgent represents an agent with an active stream connection.
type connectedAgent struct {
	id           uuid.UUID
	name         string
	capabilities *conductorv1.Capabilities
	labels       map[string]string
	stream       conductorv1.AgentService_WorkStreamServer
	sendMu       sync.Mutex
	lastSeen     time.Time
	cancel       context.CancelFunc
}

// AgentServiceServer implements the AgentService gRPC service.
type AgentServiceServer struct {
	conductorv1.UnimplementedAgentServiceServer

	deps   AgentServiceDeps
	logger zerolog.Logger

	// Connected agents indexed by agent ID
	agents   map[uuid.UUID]*connectedAgent
	agentsMu sync.RWMutex
}

// NewAgentServiceServer creates a new agent service server.
func NewAgentServiceServer(deps AgentServiceDeps, logger zerolog.Logger) *AgentServiceServer {
	return &AgentServiceServer{
		deps:   deps,
		logger: logger.With().Str("service", "AgentService").Logger(),
		agents: make(map[uuid.UUID]*connectedAgent),
	}
}

// WorkStream handles the bidirectional streaming RPC for agent communication.
func (s *AgentServiceServer) WorkStream(stream conductorv1.AgentService_WorkStreamServer) error {
	ctx := stream.Context()

	var agent *connectedAgent

	for {
		select {
		case <-ctx.Done():
			if agent != nil {
				s.disconnectAgent(agent.id)
			}
			return ctx.Err()
		default:
		}

		msg, err := stream.Recv()
		if err == io.EOF {
			if agent != nil {
				s.disconnectAgent(agent.id)
			}
			return nil
		}
		if err != nil {
			if agent != nil {
				s.disconnectAgent(agent.id)
			}
			return status.Errorf(codes.Internal, "failed to receive message: %v", err)
		}

		switch m := msg.Message.(type) {
		case *conductorv1.AgentMessage_Register:
			var regErr error
			agent, regErr = s.handleRegister(ctx, stream, m.Register)
			if regErr != nil {
				return regErr
			}

		case *conductorv1.AgentMessage_Heartbeat:
			if agent == nil {
				return status.Error(codes.FailedPrecondition, "agent not registered")
			}
			if err := s.handleHeartbeat(ctx, agent, m.Heartbeat); err != nil {
				s.logger.Error().Err(err).Str("agent_id", agent.id.String()).Msg("failed to handle heartbeat")
			}

		case *conductorv1.AgentMessage_WorkAccepted:
			if agent == nil {
				return status.Error(codes.FailedPrecondition, "agent not registered")
			}
			if err := s.handleWorkAccepted(ctx, agent, m.WorkAccepted); err != nil {
				s.logger.Error().Err(err).Str("agent_id", agent.id.String()).Msg("failed to handle work accepted")
			}

		case *conductorv1.AgentMessage_WorkRejected:
			if agent == nil {
				return status.Error(codes.FailedPrecondition, "agent not registered")
			}
			if err := s.handleWorkRejected(ctx, agent, m.WorkRejected); err != nil {
				s.logger.Error().Err(err).Str("agent_id", agent.id.String()).Msg("failed to handle work rejected")
			}

		case *conductorv1.AgentMessage_ResultStream:
			if agent == nil {
				return status.Error(codes.FailedPrecondition, "agent not registered")
			}
			if err := s.handleResultStream(ctx, agent, m.ResultStream); err != nil {
				s.logger.Error().Err(err).Str("agent_id", agent.id.String()).Msg("failed to handle result stream")
			}

		default:
			s.logger.Warn().Type("message_type", m).Msg("unknown message type")
		}
	}
}

// handleRegister processes an agent registration request.
func (s *AgentServiceServer) handleRegister(
	ctx context.Context,
	stream conductorv1.AgentService_WorkStreamServer,
	req *conductorv1.RegisterRequest,
) (*connectedAgent, error) {
	agentID, err := uuid.Parse(req.AgentId)
	if err != nil {
		resp := &conductorv1.ControlMessage{
			Message: &conductorv1.ControlMessage_RegisterResponse{
				RegisterResponse: &conductorv1.RegisterResponse{
					Success:      false,
					ErrorMessage: "invalid agent ID format",
				},
			},
		}
		if sendErr := stream.Send(resp); sendErr != nil {
			return nil, status.Errorf(codes.Internal, "failed to send register response: %v", sendErr)
		}
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent ID: %v", err)
	}

	logger := s.logger.With().Str("agent_id", agentID.String()).Str("agent_name", req.Name).Logger()
	logger.Info().Msg("agent registering")

	// Create or update agent in database
	agent := &database.Agent{
		ID:              agentID,
		Name:            req.Name,
		Status:          database.AgentStatusIdle,
		Version:         &req.Version,
		NetworkZones:    req.Capabilities.GetNetworkZones(),
		MaxParallel:     int(req.Capabilities.GetMaxParallel()),
		DockerAvailable: req.Capabilities.GetDockerAvailable(),
		RegisteredAt:    time.Now(),
	}

	// Try to get existing agent
	existing, err := s.deps.AgentRepo.GetByID(ctx, agentID)
	if err != nil && !database.IsNotFound(err) {
		logger.Error().Err(err).Msg("failed to check existing agent")
		return nil, status.Errorf(codes.Internal, "failed to check agent: %v", err)
	}

	if existing != nil {
		// Update existing agent
		if err := s.deps.AgentRepo.Update(ctx, agent); err != nil {
			logger.Error().Err(err).Msg("failed to update agent")
			return nil, status.Errorf(codes.Internal, "failed to update agent: %v", err)
		}
	} else {
		// Create new agent
		if err := s.deps.AgentRepo.Create(ctx, agent); err != nil {
			logger.Error().Err(err).Msg("failed to create agent")
			return nil, status.Errorf(codes.Internal, "failed to create agent: %v", err)
		}
	}

	// Create connected agent
	streamCtx, cancel := context.WithCancel(ctx)
	connAgent := &connectedAgent{
		id:           agentID,
		name:         req.Name,
		capabilities: req.Capabilities,
		labels:       req.Labels,
		stream:       stream,
		lastSeen:     time.Now(),
		cancel:       cancel,
	}

	// Register connected agent
	s.agentsMu.Lock()
	s.agents[agentID] = connAgent
	s.agentsMu.Unlock()

	// Send register response
	resp := &conductorv1.ControlMessage{
		Message: &conductorv1.ControlMessage_RegisterResponse{
			RegisterResponse: &conductorv1.RegisterResponse{
				Success:                  true,
				HeartbeatIntervalSeconds: int32(s.deps.HeartbeatTimeout.Seconds() / 3), // Heartbeat at 1/3 of timeout
				ServerVersion:            s.deps.ServerVersion,
			},
		},
	}
	if err := stream.Send(resp); err != nil {
		s.disconnectAgent(agentID)
		return nil, status.Errorf(codes.Internal, "failed to send register response: %v", err)
	}

	logger.Info().Msg("agent registered successfully")

	// Start work assignment goroutine
	go s.workAssignmentLoop(streamCtx, connAgent)

	return connAgent, nil
}

// workAssignmentLoop periodically checks for work to assign to an agent.
func (s *AgentServiceServer) workAssignmentLoop(ctx context.Context, agent *connectedAgent) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			work, err := s.deps.Scheduler.AssignWork(ctx, agent.id, agent.capabilities)
			if err != nil {
				s.logger.Error().Err(err).Str("agent_id", agent.id.String()).Msg("failed to get work assignment")
				continue
			}
			if work == nil {
				continue
			}

			// Send work assignment to agent
			msg := &conductorv1.ControlMessage{
				Message: &conductorv1.ControlMessage_AssignWork{
					AssignWork: work,
				},
			}

			agent.sendMu.Lock()
			err = agent.stream.Send(msg)
			agent.sendMu.Unlock()

			if err != nil {
				s.logger.Error().Err(err).
					Str("agent_id", agent.id.String()).
					Str("run_id", work.RunId).
					Msg("failed to send work assignment")
			} else {
				s.logger.Info().
					Str("agent_id", agent.id.String()).
					Str("run_id", work.RunId).
					Msg("work assigned to agent")
			}
		}
	}
}

// handleHeartbeat processes an agent heartbeat.
func (s *AgentServiceServer) handleHeartbeat(ctx context.Context, agent *connectedAgent, hb *conductorv1.Heartbeat) error {
	agent.lastSeen = time.Now()

	// Map proto status to database status
	var dbStatus database.AgentStatus
	switch hb.Status {
	case conductorv1.AgentStatus_AGENT_STATUS_IDLE:
		dbStatus = database.AgentStatusIdle
	case conductorv1.AgentStatus_AGENT_STATUS_BUSY:
		dbStatus = database.AgentStatusBusy
	case conductorv1.AgentStatus_AGENT_STATUS_DRAINING:
		dbStatus = database.AgentStatusDraining
	default:
		dbStatus = database.AgentStatusIdle
	}

	if err := s.deps.AgentRepo.UpdateHeartbeat(ctx, agent.id, dbStatus); err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	s.logger.Debug().
		Str("agent_id", agent.id.String()).
		Strs("active_runs", hb.ActiveRunIds).
		Msg("heartbeat received")

	return nil
}

// handleWorkAccepted processes a work acceptance from an agent.
func (s *AgentServiceServer) handleWorkAccepted(ctx context.Context, agent *connectedAgent, wa *conductorv1.WorkAccepted) error {
	runID, err := uuid.Parse(wa.RunId)
	if err != nil {
		return fmt.Errorf("invalid run ID: %w", err)
	}

	if err := s.deps.Scheduler.HandleWorkAccepted(ctx, agent.id, runID); err != nil {
		return fmt.Errorf("failed to handle work accepted: %w", err)
	}

	s.logger.Info().
		Str("agent_id", agent.id.String()).
		Str("run_id", wa.RunId).
		Msg("work accepted by agent")

	// Send acknowledgement
	ack := &conductorv1.ControlMessage{
		Message: &conductorv1.ControlMessage_Ack{
			Ack: &conductorv1.Ack{
				Id:      wa.RunId,
				Success: true,
			},
		},
	}

	agent.sendMu.Lock()
	err = agent.stream.Send(ack)
	agent.sendMu.Unlock()

	return err
}

// handleWorkRejected processes a work rejection from an agent.
func (s *AgentServiceServer) handleWorkRejected(ctx context.Context, agent *connectedAgent, wr *conductorv1.WorkRejected) error {
	runID, err := uuid.Parse(wr.RunId)
	if err != nil {
		return fmt.Errorf("invalid run ID: %w", err)
	}

	if err := s.deps.Scheduler.HandleWorkRejected(ctx, agent.id, runID, wr.Reason); err != nil {
		return fmt.Errorf("failed to handle work rejected: %w", err)
	}

	s.logger.Info().
		Str("agent_id", agent.id.String()).
		Str("run_id", wr.RunId).
		Str("reason", wr.Reason).
		Bool("temporary", wr.Temporary).
		Msg("work rejected by agent")

	return nil
}

// handleResultStream processes streaming results from an agent.
func (s *AgentServiceServer) handleResultStream(ctx context.Context, agent *connectedAgent, rs *conductorv1.ResultStream) error {
	logger := s.logger.With().
		Str("agent_id", agent.id.String()).
		Str("run_id", rs.RunId).
		Int64("sequence", rs.Sequence).
		Logger()

	switch p := rs.Payload.(type) {
	case *conductorv1.ResultStream_LogChunk:
		logger.Debug().
			Str("stream", p.LogChunk.Stream.String()).
			Int("bytes", len(p.LogChunk.Data)).
			Msg("log chunk received")
		// TODO: Store log chunk

	case *conductorv1.ResultStream_TestResult:
		logger.Info().
			Str("test_name", p.TestResult.TestName).
			Str("status", p.TestResult.Status.String()).
			Msg("test result received")
		// TODO: Store test result

	case *conductorv1.ResultStream_Artifact:
		logger.Info().
			Str("artifact_name", p.Artifact.Name).
			Int64("size", p.Artifact.Size).
			Msg("artifact uploaded")
		// TODO: Record artifact metadata

	case *conductorv1.ResultStream_RunComplete:
		runID, err := uuid.Parse(rs.RunId)
		if err != nil {
			return fmt.Errorf("invalid run ID: %w", err)
		}

		if err := s.deps.Scheduler.HandleRunComplete(ctx, agent.id, runID, p.RunComplete); err != nil {
			return fmt.Errorf("failed to handle run complete: %w", err)
		}

		logger.Info().
			Str("status", p.RunComplete.Status.String()).
			Int32("passed", p.RunComplete.Summary.GetPassed()).
			Int32("failed", p.RunComplete.Summary.GetFailed()).
			Msg("run completed")

	case *conductorv1.ResultStream_Progress:
		logger.Debug().
			Str("phase", p.Progress.Phase).
			Int32("percent", p.Progress.PercentComplete).
			Msg("progress update")
	}

	return nil
}

// disconnectAgent removes an agent from the connected agents map and updates its status.
func (s *AgentServiceServer) disconnectAgent(agentID uuid.UUID) {
	s.agentsMu.Lock()
	agent, ok := s.agents[agentID]
	if ok {
		delete(s.agents, agentID)
		agent.cancel()
	}
	s.agentsMu.Unlock()

	if ok {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.deps.AgentRepo.UpdateStatus(ctx, agentID, database.AgentStatusOffline); err != nil {
			s.logger.Error().Err(err).Str("agent_id", agentID.String()).Msg("failed to update agent status to offline")
		}

		s.logger.Info().Str("agent_id", agentID.String()).Msg("agent disconnected")
	}
}

// SendToAgent sends a control message to a connected agent.
func (s *AgentServiceServer) SendToAgent(agentID uuid.UUID, msg *conductorv1.ControlMessage) error {
	s.agentsMu.RLock()
	agent, ok := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not connected", agentID)
	}

	agent.sendMu.Lock()
	defer agent.sendMu.Unlock()

	return agent.stream.Send(msg)
}

// CancelWork sends a cancel work message to an agent.
func (s *AgentServiceServer) CancelWork(agentID uuid.UUID, runID string, reason string, gracePeriod time.Duration) error {
	msg := &conductorv1.ControlMessage{
		Message: &conductorv1.ControlMessage_CancelWork{
			CancelWork: &conductorv1.CancelWork{
				RunId:  runID,
				Reason: reason,
				GracePeriod: &conductorv1.Duration{
					Seconds: int64(gracePeriod.Seconds()),
				},
			},
		},
	}
	return s.SendToAgent(agentID, msg)
}

// DrainAgent sends a drain message to an agent.
func (s *AgentServiceServer) DrainAgent(agentID uuid.UUID, reason string, cancelActive bool, deadline time.Time) error {
	msg := &conductorv1.ControlMessage{
		Message: &conductorv1.ControlMessage_Drain{
			Drain: &conductorv1.Drain{
				Reason:       reason,
				CancelActive: cancelActive,
				Deadline:     timestamppb.New(deadline),
			},
		},
	}
	return s.SendToAgent(agentID, msg)
}

// GetConnectedAgents returns a list of connected agent IDs.
func (s *AgentServiceServer) GetConnectedAgents() []uuid.UUID {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()

	ids := make([]uuid.UUID, 0, len(s.agents))
	for id := range s.agents {
		ids = append(ids, id)
	}
	return ids
}

// IsAgentConnected returns true if the agent is currently connected.
func (s *AgentServiceServer) IsAgentConnected(agentID uuid.UUID) bool {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()
	_, ok := s.agents[agentID]
	return ok
}
