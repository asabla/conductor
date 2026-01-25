package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/database"
)

// WorkScheduler assigns pending shards to agents.
type WorkScheduler struct {
	runRepo     database.TestRunRepository
	serviceRepo database.ServiceRepository
	testRepo    database.TestDefinitionRepository
	shardRepo   database.RunShardRepository
	logger      *slog.Logger
}

// NewWorkScheduler creates a new WorkScheduler.
func NewWorkScheduler(
	runRepo database.TestRunRepository,
	serviceRepo database.ServiceRepository,
	testRepo database.TestDefinitionRepository,
	shardRepo database.RunShardRepository,
	logger *slog.Logger,
) *WorkScheduler {
	if logger == nil {
		logger = slog.Default()
	}

	return &WorkScheduler{
		runRepo:     runRepo,
		serviceRepo: serviceRepo,
		testRepo:    testRepo,
		shardRepo:   shardRepo,
		logger:      logger.With("component", "work_scheduler"),
	}
}

// AssignWork finds and assigns pending work to an agent.
func (w *WorkScheduler) AssignWork(ctx context.Context, agentID uuid.UUID, capabilities *conductorv1.Capabilities) (*conductorv1.AssignWork, error) {
	if capabilities == nil {
		return nil, nil
	}

	pendingRuns, err := w.runRepo.GetPending(ctx, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending runs: %w", err)
	}

	for _, run := range pendingRuns {
		service, err := w.serviceRepo.Get(ctx, run.ServiceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get service: %w", err)
		}
		if service == nil {
			continue
		}
		if !zonesMatch(service.NetworkZones, capabilities.NetworkZones) {
			continue
		}

		tests, err := w.testRepo.ListByService(ctx, run.ServiceID, database.Pagination{Limit: 1000})
		if err != nil {
			return nil, fmt.Errorf("failed to list tests: %w", err)
		}

		shards, shardTests, err := ensureShards(ctx, &run, tests, w.shardRepo)
		if err != nil {
			return nil, fmt.Errorf("failed to ensure shards: %w", err)
		}

		shard, testsForShard := nextPendingShard(shards, shardTests)
		if shard == nil {
			continue
		}

		assignment := buildAssignWork(service, &run, shard, testsForShard)
		return assignment, nil
	}

	return nil, nil
}

// CancelWork cancels a run.
func (w *WorkScheduler) CancelWork(ctx context.Context, runID uuid.UUID, reason string) error {
	if err := w.runRepo.UpdateStatus(ctx, runID, database.RunStatusCancelled); err != nil {
		return fmt.Errorf("failed to cancel run: %w", err)
	}
	return nil
}

// HandleWorkAccepted marks a run/shard as accepted.
func (w *WorkScheduler) HandleWorkAccepted(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, shardID *uuid.UUID) error {
	if shardID != nil {
		if err := w.shardRepo.Start(ctx, *shardID, agentID); err != nil {
			return fmt.Errorf("failed to start shard: %w", err)
		}
	}

	if err := w.runRepo.Start(ctx, runID, agentID); err != nil {
		return fmt.Errorf("failed to start run: %w", err)
	}
	return nil
}

// HandleWorkRejected marks a shard back to pending on rejection.
func (w *WorkScheduler) HandleWorkRejected(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, shardID *uuid.UUID, reason string) error {
	if shardID != nil {
		if err := w.shardRepo.UpdateStatus(ctx, *shardID, database.ShardStatusPending); err != nil {
			return fmt.Errorf("failed to reset shard: %w", err)
		}
	}
	return nil
}

// HandleRunComplete handles run or shard completion.
func (w *WorkScheduler) HandleRunComplete(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, shardID *uuid.UUID, result *conductorv1.RunComplete) error {
	if result == nil {
		return nil
	}

	if shardID != nil {
		return w.finishShard(ctx, *shardID, result)
	}

	return w.runRepo.Finish(ctx, runID, runStatusFromProto(result.Status), runResultsFromProto(result))
}

func (w *WorkScheduler) finishShard(ctx context.Context, shardID uuid.UUID, result *conductorv1.RunComplete) error {
	status := shardStatusFromProto(result.Status)
	if err := w.shardRepo.Finish(ctx, shardID, status, runResultsFromProto(result)); err != nil {
		return fmt.Errorf("failed to finish shard: %w", err)
	}
	return nil
}

func zonesMatch(serviceZones, agentZones []string) bool {
	if len(serviceZones) == 0 || len(agentZones) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(agentZones))
	for _, zone := range agentZones {
		set[zone] = struct{}{}
	}
	for _, zone := range serviceZones {
		if _, ok := set[zone]; ok {
			return true
		}
	}
	return false
}

func buildAssignWork(service *database.Service, run *database.TestRun, shard *database.RunShard, tests []database.TestDefinition) *conductorv1.AssignWork {
	protoTests := make([]*conductorv1.TestToRun, 0, len(tests))
	for _, test := range tests {
		protoTests = append(protoTests, testDefinitionToProto(test))
	}

	execType := determineExecutionType(tests)
	return &conductorv1.AssignWork{
		RunId:            run.ID.String(),
		GitRef:           gitRefFromRun(service, run),
		Tests:            protoTests,
		ExecutionType:    execType,
		Priority:         int32(run.Priority),
		ShardId:          shard.ID.String(),
		ShardIndex:       int32(shard.ShardIndex),
		ShardCount:       int32(shard.ShardCount),
		MaxParallelTests: int32(run.MaxParallel),
	}
}

func gitRefFromRun(service *database.Service, run *database.TestRun) *conductorv1.GitRef {
	ref := &conductorv1.GitRef{
		RepositoryUrl: service.GitURL,
	}
	if run.GitRef != nil {
		ref.Branch = *run.GitRef
	}
	if run.GitSHA != nil {
		ref.CommitSha = *run.GitSHA
	}
	return ref
}

func testDefinitionToProto(def database.TestDefinition) *conductorv1.TestToRun {
	command := def.Command
	if len(def.Args) > 0 {
		command = strings.TrimSpace(command + " " + strings.Join(def.Args, " "))
	}

	proto := &conductorv1.TestToRun{
		TestId:        def.ID.String(),
		Name:          def.Name,
		Command:       command,
		ResultFormat:  resultFormatToProto(def.ResultFormat),
		ArtifactPaths: def.ArtifactPatterns,
		RetryCount:    int32(def.Retries),
	}

	if def.TimeoutSeconds > 0 {
		proto.Timeout = &conductorv1.Duration{Seconds: int64(def.TimeoutSeconds)}
	}

	return proto
}

func resultFormatToProto(format *string) conductorv1.ResultFormat {
	if format == nil {
		return conductorv1.ResultFormat_RESULT_FORMAT_UNSPECIFIED
	}
	switch strings.ToLower(*format) {
	case "junit":
		return conductorv1.ResultFormat_RESULT_FORMAT_JUNIT
	case "jest":
		return conductorv1.ResultFormat_RESULT_FORMAT_JEST
	case "playwright":
		return conductorv1.ResultFormat_RESULT_FORMAT_PLAYWRIGHT
	case "go_test":
		return conductorv1.ResultFormat_RESULT_FORMAT_GO_TEST
	case "tap":
		return conductorv1.ResultFormat_RESULT_FORMAT_TAP
	case "json":
		return conductorv1.ResultFormat_RESULT_FORMAT_JSON
	default:
		return conductorv1.ResultFormat_RESULT_FORMAT_UNSPECIFIED
	}
}

func determineExecutionType(tests []database.TestDefinition) conductorv1.ExecutionType {
	for _, test := range tests {
		if strings.EqualFold(test.ExecutionType, "container") {
			return conductorv1.ExecutionType_EXECUTION_TYPE_CONTAINER
		}
	}
	return conductorv1.ExecutionType_EXECUTION_TYPE_SUBPROCESS
}

func runStatusFromProto(status conductorv1.RunStatus) database.RunStatus {
	switch status {
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
	case conductorv1.RunStatus_RUN_STATUS_RUNNING:
		return database.RunStatusRunning
	default:
		return database.RunStatusPending
	}
}

func shardStatusFromProto(status conductorv1.RunStatus) database.ShardStatus {
	switch status {
	case conductorv1.RunStatus_RUN_STATUS_PASSED:
		return database.ShardStatusPassed
	case conductorv1.RunStatus_RUN_STATUS_FAILED:
		return database.ShardStatusFailed
	case conductorv1.RunStatus_RUN_STATUS_ERROR:
		return database.ShardStatusError
	case conductorv1.RunStatus_RUN_STATUS_CANCELLED:
		return database.ShardStatusCancelled
	case conductorv1.RunStatus_RUN_STATUS_RUNNING:
		return database.ShardStatusRunning
	default:
		return database.ShardStatusPending
	}
}

func runResultsFromProto(result *conductorv1.RunComplete) database.RunResults {
	if result == nil || result.Summary == nil {
		return database.RunResults{}
	}

	return database.RunResults{
		TotalTests:   int(result.Summary.Total),
		PassedTests:  int(result.Summary.Passed),
		FailedTests:  int(result.Summary.Failed),
		SkippedTests: int(result.Summary.Skipped),
		DurationMs:   int64(result.Summary.Duration.GetSeconds() * 1000),
		ErrorMessage: result.ErrorMessage,
	}
}
