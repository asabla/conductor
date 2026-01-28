package server

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/database"
)

// RunServiceDeps defines the dependencies for the run service.
type RunServiceDeps struct {
	// RunRepo handles run persistence.
	RunRepo RunRepository
	// RunShardRepo handles shard persistence.
	RunShardRepo RunShardRepository
	// ServiceRepo handles service persistence.
	ServiceRepo ServiceRepository
	// Scheduler handles work scheduling.
	Scheduler WorkScheduler
}

// RunRepository defines the interface for run persistence.
type RunRepository interface {
	Create(ctx context.Context, run *database.TestRun) error
	Update(ctx context.Context, run *database.TestRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*database.TestRun, error)
	List(ctx context.Context, filter RunFilter, pagination database.Pagination) ([]*database.TestRun, int, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status database.RunStatus, errorMsg *string) error
	Finish(ctx context.Context, id uuid.UUID, status database.RunStatus, results database.RunResults) error
	UpdateShardStats(ctx context.Context, id uuid.UUID, completed int, failed int, results database.RunResults) error
}

// RunShardRepository defines the interface for run shard persistence.
type RunShardRepository interface {
	ListByRun(ctx context.Context, runID uuid.UUID) ([]database.RunShard, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status database.ShardStatus) error
	Finish(ctx context.Context, id uuid.UUID, status database.ShardStatus, results database.RunResults) error
	Reset(ctx context.Context, id uuid.UUID) error
}

// RunFilter defines filtering options for listing runs.
type RunFilter struct {
	ServiceID   *uuid.UUID
	Statuses    []database.RunStatus
	Branch      string
	CommitSHA   string
	TriggerType *database.TriggerType
	Labels      map[string]string
	StartTime   *time.Time
	EndTime     *time.Time
}

// ServiceRepository defines the interface for service persistence.
type ServiceRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*database.Service, error)
	List(ctx context.Context, filter ServiceFilter, pagination database.Pagination) ([]*database.Service, int, error)
}

// ServiceFilter defines filtering options for listing services.
type ServiceFilter struct {
	Owner       string
	NetworkZone string
	Labels      map[string]string
	Query       string
}

// RunServiceServer implements the RunService gRPC service.
type RunServiceServer struct {
	conductorv1.UnimplementedRunServiceServer

	deps   RunServiceDeps
	logger zerolog.Logger
}

// NewRunServiceServer creates a new run service server.
func NewRunServiceServer(deps RunServiceDeps, logger zerolog.Logger) *RunServiceServer {
	return &RunServiceServer{
		deps:   deps,
		logger: logger.With().Str("service", "RunService").Logger(),
	}
}

// CreateRun creates a new test run.
func (s *RunServiceServer) CreateRun(ctx context.Context, req *conductorv1.CreateRunRequest) (*conductorv1.CreateRunResponse, error) {
	serviceID, err := uuid.Parse(req.ServiceId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
	}

	// Verify service exists
	service, err := s.deps.ServiceRepo.GetByID(ctx, serviceID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "service not found: %s", req.ServiceId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get service: %v", err)
	}

	// Create the run
	run := &database.TestRun{
		ID:          uuid.New(),
		ServiceID:   serviceID,
		Status:      database.RunStatusPending,
		GitRef:      database.NullString(req.GetGitRef().GetBranch()),
		GitSHA:      database.NullString(req.GetGitRef().GetCommitSha()),
		TriggerType: triggerTypeFromProto(req.GetTrigger().GetType()),
		TriggeredBy: database.NullString(req.GetTrigger().GetUser()),
		Priority:    int(req.Priority),
		CreatedAt:   time.Now(),
	}

	if err := s.deps.RunRepo.Create(ctx, run); err != nil {
		s.logger.Error().Err(err).Str("service_id", serviceID.String()).Msg("failed to create run")
		return nil, status.Errorf(codes.Internal, "failed to create run: %v", err)
	}

	s.logger.Info().
		Str("run_id", run.ID.String()).
		Str("service_id", serviceID.String()).
		Str("service_name", service.Name).
		Msg("run created")

	return &conductorv1.CreateRunResponse{
		Run: runToProto(run, service),
	}, nil
}

// GetRun retrieves details of a specific test run.
func (s *RunServiceServer) GetRun(ctx context.Context, req *conductorv1.GetRunRequest) (*conductorv1.GetRunResponse, error) {
	runID, err := uuid.Parse(req.RunId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid run ID: %v", err)
	}

	run, err := s.deps.RunRepo.GetByID(ctx, runID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "run not found: %s", req.RunId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get run: %v", err)
	}

	service, err := s.deps.ServiceRepo.GetByID(ctx, run.ServiceID)
	if err != nil {
		s.logger.Error().Err(err).Str("service_id", run.ServiceID.String()).Msg("failed to get service for run")
		// Continue with nil service - non-critical error
	}

	resp := &conductorv1.GetRunResponse{
		Run: runToProto(run, service),
	}

	if req.IncludeShards && s.deps.RunShardRepo != nil {
		shards, err := s.deps.RunShardRepo.ListByRun(ctx, runID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list shards: %v", err)
		}
		resp.Shards = runShardsToProto(shards)
	}

	// TODO: Include results and artifacts if requested

	return resp, nil
}

// ListRuns returns a paginated list of test runs.
func (s *RunServiceServer) ListRuns(ctx context.Context, req *conductorv1.ListRunsRequest) (*conductorv1.ListRunsResponse, error) {
	filter := RunFilter{
		Branch:    req.Branch,
		CommitSHA: req.CommitSha,
		Labels:    req.Labels,
	}

	if req.ServiceId != "" {
		serviceID, err := uuid.Parse(req.ServiceId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
		}
		filter.ServiceID = &serviceID
	}

	if len(req.Statuses) > 0 {
		filter.Statuses = make([]database.RunStatus, len(req.Statuses))
		for i, s := range req.Statuses {
			filter.Statuses[i] = runStatusFromProto(s)
		}
	}

	if req.TriggerType != conductorv1.TriggerType_TRIGGER_TYPE_UNSPECIFIED {
		tt := triggerTypeFromProtoEnum(req.TriggerType)
		filter.TriggerType = &tt
	}

	if req.TimeRange != nil {
		if req.TimeRange.Start != nil {
			t := req.TimeRange.Start.AsTime()
			filter.StartTime = &t
		}
		if req.TimeRange.End != nil {
			t := req.TimeRange.End.AsTime()
			filter.EndTime = &t
		}
	}

	pagination := paginationFromProto(req.Pagination)
	runs, total, err := s.deps.RunRepo.List(ctx, filter, pagination)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list runs: %v", err)
	}

	// Get services for runs
	serviceIDs := make(map[uuid.UUID]bool)
	for _, run := range runs {
		serviceIDs[run.ServiceID] = true
	}

	services := make(map[uuid.UUID]*database.Service)
	for serviceID := range serviceIDs {
		svc, err := s.deps.ServiceRepo.GetByID(ctx, serviceID)
		if err == nil {
			services[serviceID] = svc
		}
	}

	protoRuns := make([]*conductorv1.Run, len(runs))
	for i, run := range runs {
		protoRuns[i] = runToProto(run, services[run.ServiceID])
	}

	return &conductorv1.ListRunsResponse{
		Runs:       protoRuns,
		Pagination: paginationResponseToProto(pagination, total),
	}, nil
}

// CancelRun cancels a pending or running test run.
func (s *RunServiceServer) CancelRun(ctx context.Context, req *conductorv1.CancelRunRequest) (*conductorv1.CancelRunResponse, error) {
	runID, err := uuid.Parse(req.RunId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid run ID: %v", err)
	}

	if req.ShardId != "" {
		return s.cancelShard(ctx, runID, req.ShardId, req.Reason)
	}

	run, err := s.deps.RunRepo.GetByID(ctx, runID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "run not found: %s", req.RunId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get run: %v", err)
	}

	if run.IsTerminal() {
		return nil, status.Errorf(codes.FailedPrecondition, "run is already in terminal state: %s", run.Status)
	}

	// Cancel via scheduler (handles agent notification)
	if err := s.deps.Scheduler.CancelWork(ctx, runID, req.Reason); err != nil {
		s.logger.Error().Err(err).Str("run_id", runID.String()).Msg("failed to cancel work via scheduler")
	}

	// Update run status
	reason := req.Reason
	if err := s.deps.RunRepo.UpdateStatus(ctx, runID, database.RunStatusCancelled, &reason); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update run status: %v", err)
	}

	// Fetch updated run
	run, _ = s.deps.RunRepo.GetByID(ctx, runID)
	service, _ := s.deps.ServiceRepo.GetByID(ctx, run.ServiceID)

	s.logger.Info().
		Str("run_id", runID.String()).
		Str("reason", req.Reason).
		Msg("run cancelled")

	return &conductorv1.CancelRunResponse{
		Run: runToProto(run, service),
	}, nil
}

// RetryRun creates a new run with the same parameters as a previous run.
func (s *RunServiceServer) RetryRun(ctx context.Context, req *conductorv1.RetryRunRequest) (*conductorv1.RetryRunResponse, error) {
	originalRunID, err := uuid.Parse(req.RunId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid run ID: %v", err)
	}

	if req.ShardId != "" {
		return s.retryShard(ctx, originalRunID, req.ShardId)
	}

	originalRun, err := s.deps.RunRepo.GetByID(ctx, originalRunID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "run not found: %s", req.RunId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get run: %v", err)
	}

	// Create new run based on original
	triggerRetry := database.TriggerTypeManual // Default to manual for retries
	newRun := &database.TestRun{
		ID:          uuid.New(),
		ServiceID:   originalRun.ServiceID,
		Status:      database.RunStatusPending,
		GitRef:      originalRun.GitRef,
		GitSHA:      originalRun.GitSHA,
		TriggerType: &triggerRetry,
		Priority:    originalRun.Priority,
		CreatedAt:   time.Now(),
	}

	if err := s.deps.RunRepo.Create(ctx, newRun); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create retry run: %v", err)
	}

	service, _ := s.deps.ServiceRepo.GetByID(ctx, newRun.ServiceID)

	s.logger.Info().
		Str("run_id", newRun.ID.String()).
		Str("original_run_id", originalRunID.String()).
		Msg("retry run created")

	return &conductorv1.RetryRunResponse{
		Run:           runToProto(newRun, service),
		OriginalRunId: originalRunID.String(),
	}, nil
}

// StreamRunLogs streams live logs from a running test.
func (s *RunServiceServer) StreamRunLogs(req *conductorv1.StreamRunLogsRequest, stream conductorv1.RunService_StreamRunLogsServer) error {
	// TODO: Implement log streaming
	return status.Error(codes.Unimplemented, "log streaming not yet implemented")
}

// GetRunLogs retrieves stored logs for a completed run.
func (s *RunServiceServer) GetRunLogs(ctx context.Context, req *conductorv1.GetRunLogsRequest) (*conductorv1.GetRunLogsResponse, error) {
	// TODO: Implement log retrieval
	return nil, status.Error(codes.Unimplemented, "log retrieval not yet implemented")
}

// Helper functions for type conversion

func runToProto(run *database.TestRun, service *database.Service) *conductorv1.Run {
	if run == nil {
		return nil
	}

	protoRun := &conductorv1.Run{
		Id:        run.ID.String(),
		ServiceId: run.ServiceID.String(),
		Status:    runStatusToProto(run.Status),
		Priority:  int32(run.Priority),
		CreatedAt: timestamppb.New(run.CreatedAt),
	}

	if service != nil {
		protoRun.ServiceName = service.Name
	}

	if run.GitRef != nil || run.GitSHA != nil {
		protoRun.GitRef = &conductorv1.GitRef{}
		if run.GitRef != nil {
			protoRun.GitRef.Branch = *run.GitRef
		}
		if run.GitSHA != nil {
			protoRun.GitRef.CommitSha = *run.GitSHA
		}
	}

	if run.TriggerType != nil {
		protoRun.Trigger = &conductorv1.RunTrigger{
			Type: triggerTypeToProto(*run.TriggerType),
		}
		if run.TriggeredBy != nil {
			protoRun.Trigger.User = *run.TriggeredBy
		}
	}

	if run.AgentID != nil {
		protoRun.AgentId = run.AgentID.String()
	}

	if run.StartedAt != nil {
		protoRun.StartedAt = timestamppb.New(*run.StartedAt)
	}

	if run.FinishedAt != nil {
		protoRun.FinishedAt = timestamppb.New(*run.FinishedAt)
	}

	if run.ErrorMessage != nil {
		protoRun.ErrorMessage = *run.ErrorMessage
	}

	protoRun.Summary = &conductorv1.RunSummary{
		Total:   int32(run.TotalTests),
		Passed:  int32(run.PassedTests),
		Failed:  int32(run.FailedTests),
		Skipped: int32(run.SkippedTests),
	}

	protoRun.ShardCount = int32(run.ShardCount)
	protoRun.ShardsCompleted = int32(run.ShardsDone)
	protoRun.ShardsFailed = int32(run.ShardsFailed)
	protoRun.MaxParallelTests = int32(run.MaxParallel)

	return protoRun
}

func runStatusToProto(status database.RunStatus) conductorv1.RunStatus {
	switch status {
	case database.RunStatusPending:
		return conductorv1.RunStatus_RUN_STATUS_PENDING
	case database.RunStatusRunning:
		return conductorv1.RunStatus_RUN_STATUS_RUNNING
	case database.RunStatusPassed:
		return conductorv1.RunStatus_RUN_STATUS_PASSED
	case database.RunStatusFailed:
		return conductorv1.RunStatus_RUN_STATUS_FAILED
	case database.RunStatusError:
		return conductorv1.RunStatus_RUN_STATUS_ERROR
	case database.RunStatusTimeout:
		return conductorv1.RunStatus_RUN_STATUS_TIMEOUT
	case database.RunStatusCancelled:
		return conductorv1.RunStatus_RUN_STATUS_CANCELLED
	default:
		return conductorv1.RunStatus_RUN_STATUS_UNSPECIFIED
	}
}

func runStatusFromProto(status conductorv1.RunStatus) database.RunStatus {
	switch status {
	case conductorv1.RunStatus_RUN_STATUS_PENDING:
		return database.RunStatusPending
	case conductorv1.RunStatus_RUN_STATUS_RUNNING:
		return database.RunStatusRunning
	case conductorv1.RunStatus_RUN_STATUS_PASSED:
		return database.RunStatusPassed
	case conductorv1.RunStatus_RUN_STATUS_FAILED:
		return database.RunStatusFailed
	case conductorv1.RunStatus_RUN_STATUS_ERROR:
		return database.RunStatusError
	case conductorv1.RunStatus_RUN_STATUS_TIMEOUT:
		return database.RunStatusTimeout
	case conductorv1.RunStatus_RUN_STATUS_CANCELLED:
		return database.RunStatusCancelled
	default:
		return database.RunStatusPending
	}
}

func (s *RunServiceServer) cancelShard(ctx context.Context, runID uuid.UUID, shardID string, reason string) (*conductorv1.CancelRunResponse, error) {
	if s.deps.RunShardRepo == nil {
		return nil, status.Error(codes.FailedPrecondition, "shard repository not configured")
	}

	parsed, err := uuid.Parse(shardID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid shard ID: %v", err)
	}

	results := database.RunResults{ErrorMessage: reason}
	if err := s.deps.RunShardRepo.Finish(ctx, parsed, database.ShardStatusCancelled, results); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to cancel shard: %v", err)
	}

	if err := s.updateRunFromShards(ctx, runID); err != nil {
		return nil, err
	}

	run, _ := s.deps.RunRepo.GetByID(ctx, runID)
	service, _ := s.deps.ServiceRepo.GetByID(ctx, run.ServiceID)

	return &conductorv1.CancelRunResponse{Run: runToProto(run, service)}, nil
}

func (s *RunServiceServer) retryShard(ctx context.Context, runID uuid.UUID, shardID string) (*conductorv1.RetryRunResponse, error) {
	if s.deps.RunShardRepo == nil {
		return nil, status.Error(codes.FailedPrecondition, "shard repository not configured")
	}

	parsed, err := uuid.Parse(shardID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid shard ID: %v", err)
	}

	if err := s.deps.RunShardRepo.Reset(ctx, parsed); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to reset shard: %v", err)
	}

	run, _ := s.deps.RunRepo.GetByID(ctx, runID)
	service, _ := s.deps.ServiceRepo.GetByID(ctx, run.ServiceID)

	return &conductorv1.RetryRunResponse{
		Run:           runToProto(run, service),
		OriginalRunId: runID.String(),
	}, nil
}

func (s *RunServiceServer) updateRunFromShards(ctx context.Context, runID uuid.UUID) error {
	if s.deps.RunShardRepo == nil {
		return nil
	}

	shards, err := s.deps.RunShardRepo.ListByRun(ctx, runID)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to list shards: %v", err)
	}

	completed, failed, results, finished := aggregateShardResults(shards)
	if err := s.deps.RunRepo.UpdateShardStats(ctx, runID, completed, failed, results); err != nil {
		return status.Errorf(codes.Internal, "failed to update shard stats: %v", err)
	}

	if finished {
		statusVal := runStatusFromShardStatus(shards)
		if err := s.deps.RunRepo.Finish(ctx, runID, statusVal, results); err != nil {
			return status.Errorf(codes.Internal, "failed to finish run: %v", err)
		}
	}

	return nil
}

func runShardsToProto(shards []database.RunShard) []*conductorv1.RunShard {
	protoShards := make([]*conductorv1.RunShard, 0, len(shards))
	for _, shard := range shards {
		protoShards = append(protoShards, runShardToProto(shard))
	}
	return protoShards
}

func runShardToProto(shard database.RunShard) *conductorv1.RunShard {
	protoShard := &conductorv1.RunShard{
		Id:         shard.ID.String(),
		RunId:      shard.RunID.String(),
		ShardIndex: int32(shard.ShardIndex),
		ShardCount: int32(shard.ShardCount),
		Status:     runStatusToProto(runStatusFromShardStatusValue(shard.Status)),
		Summary: &conductorv1.RunSummary{
			Total:   int32(shard.TotalTests),
			Passed:  int32(shard.PassedTests),
			Failed:  int32(shard.FailedTests),
			Skipped: int32(shard.SkippedTests),
		},
	}

	if shard.AgentID != nil {
		protoShard.AgentId = shard.AgentID.String()
	}
	if shard.ErrorMessage != nil {
		protoShard.ErrorMessage = *shard.ErrorMessage
	}
	if shard.StartedAt != nil {
		protoShard.StartedAt = timestamppb.New(*shard.StartedAt)
	}
	if shard.FinishedAt != nil {
		protoShard.FinishedAt = timestamppb.New(*shard.FinishedAt)
	}

	return protoShard
}

func aggregateShardResults(shards []database.RunShard) (completed int, failed int, results database.RunResults, finished bool) {
	finished = true

	for _, shard := range shards {
		switch shard.Status {
		case database.ShardStatusPending, database.ShardStatusRunning:
			finished = false
			continue
		default:
			completed++
		}

		if shard.Status == database.ShardStatusFailed || shard.Status == database.ShardStatusError || shard.Status == database.ShardStatusCancelled {
			failed++
			if results.ErrorMessage == "" && shard.ErrorMessage != nil {
				results.ErrorMessage = *shard.ErrorMessage
			}
		}

		results.TotalTests += shard.TotalTests
		results.PassedTests += shard.PassedTests
		results.FailedTests += shard.FailedTests
		results.SkippedTests += shard.SkippedTests
	}

	return completed, failed, results, finished
}

func runStatusFromShardStatus(shards []database.RunShard) database.RunStatus {
	var hasFailed bool
	var hasError bool
	var hasCancelled bool

	for _, shard := range shards {
		switch shard.Status {
		case database.ShardStatusError:
			hasError = true
		case database.ShardStatusFailed:
			hasFailed = true
		case database.ShardStatusCancelled:
			hasCancelled = true
		case database.ShardStatusPending, database.ShardStatusRunning:
			return database.RunStatusRunning
		}
	}

	if hasError {
		return database.RunStatusError
	}
	if hasFailed {
		return database.RunStatusFailed
	}
	if hasCancelled {
		return database.RunStatusCancelled
	}
	return database.RunStatusPassed
}

func runStatusFromShardStatusValue(status database.ShardStatus) database.RunStatus {
	switch status {
	case database.ShardStatusPassed:
		return database.RunStatusPassed
	case database.ShardStatusFailed:
		return database.RunStatusFailed
	case database.ShardStatusError:
		return database.RunStatusError
	case database.ShardStatusCancelled:
		return database.RunStatusCancelled
	case database.ShardStatusRunning:
		return database.RunStatusRunning
	default:
		return database.RunStatusPending
	}
}

func triggerTypeFromProto(trigger conductorv1.TriggerType) *database.TriggerType {
	tt := triggerTypeFromProtoEnum(trigger)
	if tt == "" {
		return nil
	}
	return &tt
}

func triggerTypeFromProtoEnum(trigger conductorv1.TriggerType) database.TriggerType {
	switch trigger {
	case conductorv1.TriggerType_TRIGGER_TYPE_MANUAL:
		return database.TriggerTypeManual
	case conductorv1.TriggerType_TRIGGER_TYPE_WEBHOOK:
		return database.TriggerTypeWebhook
	case conductorv1.TriggerType_TRIGGER_TYPE_SCHEDULED:
		return database.TriggerTypeSchedule
	default:
		return ""
	}
}

func triggerTypeToProto(tt database.TriggerType) conductorv1.TriggerType {
	switch tt {
	case database.TriggerTypeManual:
		return conductorv1.TriggerType_TRIGGER_TYPE_MANUAL
	case database.TriggerTypeWebhook:
		return conductorv1.TriggerType_TRIGGER_TYPE_WEBHOOK
	case database.TriggerTypeSchedule:
		return conductorv1.TriggerType_TRIGGER_TYPE_SCHEDULED
	default:
		return conductorv1.TriggerType_TRIGGER_TYPE_UNSPECIFIED
	}
}

func paginationFromProto(p *conductorv1.Pagination) database.Pagination {
	if p == nil {
		return database.DefaultPagination()
	}
	pagination := database.Pagination{
		Limit:  int(p.PageSize),
		Offset: 0,
	}
	if pagination.Limit <= 0 {
		pagination.Limit = 50
	}
	if pagination.Limit > 100 {
		pagination.Limit = 100
	}
	// TODO: Handle page token for cursor-based pagination
	return pagination
}

func paginationResponseToProto(p database.Pagination, total int) *conductorv1.PaginationResponse {
	hasMore := p.Offset+p.Limit < total
	return &conductorv1.PaginationResponse{
		TotalCount: int64(total),
		HasMore:    hasMore,
		// TODO: Generate next page token
	}
}
