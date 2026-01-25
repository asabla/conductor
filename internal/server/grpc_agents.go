package server

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/database"
)

// AgentManagementServer implements the AgentManagementService gRPC service.
type AgentManagementServer struct {
	conductorv1.UnimplementedAgentManagementServiceServer

	deps   AgentServiceDeps
	logger zerolog.Logger
}

// NewAgentManagementServer creates a new agent management server.
func NewAgentManagementServer(deps AgentServiceDeps, logger zerolog.Logger) *AgentManagementServer {
	return &AgentManagementServer{
		deps:   deps,
		logger: logger.With().Str("service", "AgentManagementService").Logger(),
	}
}

// ListAgents returns a paginated list of all registered agents.
func (s *AgentManagementServer) ListAgents(ctx context.Context, req *conductorv1.ListAgentsRequest) (*conductorv1.ListAgentsResponse, error) {
	filter := AgentFilter{
		NetworkZone: req.NetworkZone,
		Labels:      req.Labels,
		Query:       req.Query,
	}

	if len(req.Statuses) > 0 {
		filter.Statuses = make([]database.AgentStatus, len(req.Statuses))
		for i, st := range req.Statuses {
			filter.Statuses[i] = agentStatusFromProto(st)
		}
	}

	pagination := paginationFromProto(req.Pagination)
	agents, total, err := s.deps.AgentRepo.List(ctx, filter, pagination)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list agents: %v", err)
	}

	protoAgents := make([]*conductorv1.Agent, len(agents))
	for i, agent := range agents {
		protoAgents[i] = agentToProto(agent)
	}

	return &conductorv1.ListAgentsResponse{
		Agents:     protoAgents,
		Pagination: paginationResponseToProto(pagination, total),
	}, nil
}

// GetAgent retrieves details of a specific agent.
func (s *AgentManagementServer) GetAgent(ctx context.Context, req *conductorv1.GetAgentRequest) (*conductorv1.GetAgentResponse, error) {
	agentID, err := uuid.Parse(req.AgentId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent ID: %v", err)
	}

	agent, err := s.deps.AgentRepo.GetByID(ctx, agentID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "agent not found: %s", req.AgentId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get agent: %v", err)
	}

	resp := &conductorv1.GetAgentResponse{
		Agent: agentToProto(agent),
	}

	// TODO: Include current runs if requested

	return resp, nil
}

// DrainAgent puts an agent into draining mode.
func (s *AgentManagementServer) DrainAgent(ctx context.Context, req *conductorv1.DrainAgentRequest) (*conductorv1.DrainAgentResponse, error) {
	agentID, err := uuid.Parse(req.AgentId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent ID: %v", err)
	}

	agent, err := s.deps.AgentRepo.GetByID(ctx, agentID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "agent not found: %s", req.AgentId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get agent: %v", err)
	}

	if agent.Status == database.AgentStatusOffline {
		return nil, status.Error(codes.FailedPrecondition, "cannot drain offline agent")
	}

	// Update status to draining
	if err := s.deps.AgentRepo.UpdateStatus(ctx, agentID, database.AgentStatusDraining); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update agent status: %v", err)
	}

	// Fetch updated agent
	agent, _ = s.deps.AgentRepo.GetByID(ctx, agentID)

	s.logger.Info().
		Str("agent_id", agentID.String()).
		Str("reason", req.Reason).
		Bool("cancel_active", req.CancelActive).
		Msg("agent drain requested")

	return &conductorv1.DrainAgentResponse{
		Agent:         agentToProto(agent),
		CancelledRuns: 0, // TODO: Count cancelled runs if cancel_active is true
	}, nil
}

// UndrainAgent removes an agent from draining mode.
func (s *AgentManagementServer) UndrainAgent(ctx context.Context, req *conductorv1.UndrainAgentRequest) (*conductorv1.UndrainAgentResponse, error) {
	agentID, err := uuid.Parse(req.AgentId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent ID: %v", err)
	}

	agent, err := s.deps.AgentRepo.GetByID(ctx, agentID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "agent not found: %s", req.AgentId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get agent: %v", err)
	}

	if agent.Status != database.AgentStatusDraining {
		return nil, status.Errorf(codes.FailedPrecondition, "agent is not draining, current status: %s", agent.Status)
	}

	// Update status to idle
	if err := s.deps.AgentRepo.UpdateStatus(ctx, agentID, database.AgentStatusIdle); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update agent status: %v", err)
	}

	// Fetch updated agent
	agent, _ = s.deps.AgentRepo.GetByID(ctx, agentID)

	s.logger.Info().
		Str("agent_id", agentID.String()).
		Msg("agent undrained")

	return &conductorv1.UndrainAgentResponse{
		Agent: agentToProto(agent),
	}, nil
}

// DeleteAgent removes an agent from the registry.
func (s *AgentManagementServer) DeleteAgent(ctx context.Context, req *conductorv1.DeleteAgentRequest) (*conductorv1.DeleteAgentResponse, error) {
	agentID, err := uuid.Parse(req.AgentId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent ID: %v", err)
	}

	agent, err := s.deps.AgentRepo.GetByID(ctx, agentID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "agent not found: %s", req.AgentId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get agent: %v", err)
	}

	if agent.Status != database.AgentStatusOffline && !req.Force {
		return nil, status.Error(codes.FailedPrecondition, "cannot delete online agent without force flag")
	}

	if err := s.deps.AgentRepo.Delete(ctx, agentID); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete agent: %v", err)
	}

	s.logger.Info().
		Str("agent_id", agentID.String()).
		Bool("force", req.Force).
		Msg("agent deleted")

	return &conductorv1.DeleteAgentResponse{
		Success: true,
		Message: "agent deleted successfully",
	}, nil
}

// GetAgentStats retrieves statistics for a specific agent.
func (s *AgentManagementServer) GetAgentStats(ctx context.Context, req *conductorv1.GetAgentStatsRequest) (*conductorv1.GetAgentStatsResponse, error) {
	agentID, err := uuid.Parse(req.AgentId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent ID: %v", err)
	}

	// Verify agent exists
	_, err = s.deps.AgentRepo.GetByID(ctx, agentID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "agent not found: %s", req.AgentId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get agent: %v", err)
	}

	// TODO: Implement actual stats collection
	return &conductorv1.GetAgentStatsResponse{
		AgentId:              req.AgentId,
		TotalRuns:            0,
		SuccessfulRuns:       0,
		FailedRuns:           0,
		AvgDurationMs:        0,
		TotalExecutionTimeMs: 0,
		UptimePercent:        0,
	}, nil
}

// Helper functions

func agentToProto(agent *database.Agent) *conductorv1.Agent {
	if agent == nil {
		return nil
	}

	protoAgent := &conductorv1.Agent{
		Id:           agent.ID.String(),
		Name:         agent.Name,
		Status:       agentStatusToProto(agent.Status),
		NetworkZones: agent.NetworkZones,
		MaxParallel:  int32(agent.MaxParallel),
		RegisteredAt: timestamppb.New(agent.RegisteredAt),
		Capabilities: &conductorv1.AgentCapabilities{
			DockerAvailable: agent.DockerAvailable,
		},
	}

	if agent.Version != nil {
		protoAgent.Version = *agent.Version
	}

	if agent.LastHeartbeat != nil {
		protoAgent.LastHeartbeat = timestamppb.New(*agent.LastHeartbeat)
	}

	return protoAgent
}

func agentStatusToProto(status database.AgentStatus) conductorv1.AgentStatus {
	switch status {
	case database.AgentStatusIdle:
		return conductorv1.AgentStatus_AGENT_STATUS_IDLE
	case database.AgentStatusBusy:
		return conductorv1.AgentStatus_AGENT_STATUS_BUSY
	case database.AgentStatusDraining:
		return conductorv1.AgentStatus_AGENT_STATUS_DRAINING
	case database.AgentStatusOffline:
		return conductorv1.AgentStatus_AGENT_STATUS_OFFLINE
	default:
		return conductorv1.AgentStatus_AGENT_STATUS_UNSPECIFIED
	}
}

func agentStatusFromProto(status conductorv1.AgentStatus) database.AgentStatus {
	switch status {
	case conductorv1.AgentStatus_AGENT_STATUS_IDLE:
		return database.AgentStatusIdle
	case conductorv1.AgentStatus_AGENT_STATUS_BUSY:
		return database.AgentStatusBusy
	case conductorv1.AgentStatus_AGENT_STATUS_DRAINING:
		return database.AgentStatusDraining
	case conductorv1.AgentStatus_AGENT_STATUS_OFFLINE:
		return database.AgentStatusOffline
	default:
		return database.AgentStatusOffline
	}
}
