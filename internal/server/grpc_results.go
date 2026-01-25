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

// ResultServiceDeps defines the dependencies for the result service.
type ResultServiceDeps struct {
	// ResultRepo handles result persistence.
	ResultRepo ResultRepository
	// ArtifactRepo handles artifact persistence.
	ArtifactRepo ArtifactRepository
	// RunRepo handles run persistence.
	RunRepo RunRepository
	// ArtifactStorage handles artifact storage operations.
	ArtifactStorage ArtifactStorage
}

// ResultRepository defines the interface for result persistence.
type ResultRepository interface {
	GetByRunID(ctx context.Context, runID uuid.UUID) ([]*database.TestResult, error)
	List(ctx context.Context, runID uuid.UUID, filter ResultFilter, pagination database.Pagination) ([]*database.TestResult, int, error)
	Create(ctx context.Context, result *database.TestResult) error
}

// ResultFilter defines filtering options for listing results.
type ResultFilter struct {
	Statuses    []database.ResultStatus
	SuiteName   string
	NamePattern string
}

// ArtifactRepository defines the interface for artifact persistence.
type ArtifactRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*database.Artifact, error)
	ListByRunID(ctx context.Context, runID uuid.UUID, pagination database.Pagination) ([]*database.Artifact, int, error)
	Create(ctx context.Context, artifact *database.Artifact) error
}

// ArtifactStorage handles artifact storage operations.
type ArtifactStorage interface {
	// GenerateDownloadURL generates a signed download URL for an artifact.
	GenerateDownloadURL(ctx context.Context, path string, expirationSeconds int) (string, time.Time, error)
}

// ResultServiceServer implements the ResultService gRPC service.
type ResultServiceServer struct {
	conductorv1.UnimplementedResultServiceServer

	deps   ResultServiceDeps
	logger zerolog.Logger
}

// NewResultServiceServer creates a new result service server.
func NewResultServiceServer(deps ResultServiceDeps, logger zerolog.Logger) *ResultServiceServer {
	return &ResultServiceServer{
		deps:   deps,
		logger: logger.With().Str("service", "ResultService").Logger(),
	}
}

// GetRunResults retrieves aggregated results for a specific test run.
func (s *ResultServiceServer) GetRunResults(ctx context.Context, req *conductorv1.GetRunResultsRequest) (*conductorv1.GetRunResultsResponse, error) {
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

	// Calculate pass rate
	var passRate float64
	if run.TotalTests > 0 {
		passRate = float64(run.PassedTests) / float64(run.TotalTests) * 100
	}

	resp := &conductorv1.GetRunResultsResponse{
		RunId:  req.RunId,
		Status: runStatusToProto(run.Status),
		Summary: &conductorv1.ResultSummary{
			Total:    int32(run.TotalTests),
			Passed:   int32(run.PassedTests),
			Failed:   int32(run.FailedTests),
			Skipped:  int32(run.SkippedTests),
			PassRate: passRate,
		},
	}

	if run.StartedAt != nil {
		resp.StartedAt = timestamppb.New(*run.StartedAt)
	}
	if run.FinishedAt != nil {
		resp.FinishedAt = timestamppb.New(*run.FinishedAt)
	}
	if run.DurationMs != nil {
		resp.DurationMs = *run.DurationMs
	}
	if run.ErrorMessage != nil {
		resp.ErrorMessage = *run.ErrorMessage
	}

	return resp, nil
}

// ListTestResults returns a paginated list of individual test results.
func (s *ResultServiceServer) ListTestResults(ctx context.Context, req *conductorv1.ListTestResultsRequest) (*conductorv1.ListTestResultsResponse, error) {
	runID, err := uuid.Parse(req.RunId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid run ID: %v", err)
	}

	filter := ResultFilter{
		SuiteName:   req.SuiteName,
		NamePattern: req.NamePattern,
	}

	if len(req.Statuses) > 0 {
		filter.Statuses = make([]database.ResultStatus, len(req.Statuses))
		for i, s := range req.Statuses {
			filter.Statuses[i] = testStatusFromProto(s)
		}
	}

	pagination := paginationFromProto(req.Pagination)
	results, total, err := s.deps.ResultRepo.List(ctx, runID, filter, pagination)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list results: %v", err)
	}

	protoResults := make([]*conductorv1.TestResult, len(results))
	for i, result := range results {
		protoResults[i] = testResultToProto(result)
	}

	return &conductorv1.ListTestResultsResponse{
		Results:    protoResults,
		Pagination: paginationResponseToProto(pagination, total),
	}, nil
}

// GetArtifact retrieves metadata for a specific artifact.
func (s *ResultServiceServer) GetArtifact(ctx context.Context, req *conductorv1.GetArtifactRequest) (*conductorv1.GetArtifactResponse, error) {
	artifactID, err := uuid.Parse(req.ArtifactId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid artifact ID: %v", err)
	}

	artifact, err := s.deps.ArtifactRepo.GetByID(ctx, artifactID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "artifact not found: %s", req.ArtifactId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get artifact: %v", err)
	}

	return &conductorv1.GetArtifactResponse{
		Artifact: artifactToProto(artifact),
	}, nil
}

// GetArtifactDownloadURL generates a signed download URL for an artifact.
func (s *ResultServiceServer) GetArtifactDownloadURL(ctx context.Context, req *conductorv1.GetArtifactDownloadURLRequest) (*conductorv1.GetArtifactDownloadURLResponse, error) {
	artifactID, err := uuid.Parse(req.ArtifactId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid artifact ID: %v", err)
	}

	artifact, err := s.deps.ArtifactRepo.GetByID(ctx, artifactID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "artifact not found: %s", req.ArtifactId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get artifact: %v", err)
	}

	// Default expiration is 5 minutes, max is 1 hour
	expirationSeconds := int(req.ExpirationSeconds)
	if expirationSeconds <= 0 {
		expirationSeconds = 300
	}
	if expirationSeconds > 3600 {
		expirationSeconds = 3600
	}

	downloadURL, expiresAt, err := s.deps.ArtifactStorage.GenerateDownloadURL(ctx, artifact.Path, expirationSeconds)
	if err != nil {
		s.logger.Error().Err(err).
			Str("artifact_id", artifactID.String()).
			Str("path", artifact.Path).
			Msg("failed to generate download URL")
		return nil, status.Errorf(codes.Internal, "failed to generate download URL: %v", err)
	}

	return &conductorv1.GetArtifactDownloadURLResponse{
		DownloadUrl: downloadURL,
		ExpiresAt:   timestamppb.New(expiresAt),
	}, nil
}

// ListArtifacts returns all artifacts for a specific run.
func (s *ResultServiceServer) ListArtifacts(ctx context.Context, req *conductorv1.ListArtifactsRequest) (*conductorv1.ListArtifactsResponse, error) {
	runID, err := uuid.Parse(req.RunId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid run ID: %v", err)
	}

	pagination := paginationFromProto(req.Pagination)
	artifacts, total, err := s.deps.ArtifactRepo.ListByRunID(ctx, runID, pagination)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list artifacts: %v", err)
	}

	protoArtifacts := make([]*conductorv1.Artifact, len(artifacts))
	for i, artifact := range artifacts {
		protoArtifacts[i] = artifactToProto(artifact)
	}

	return &conductorv1.ListArtifactsResponse{
		Artifacts:  protoArtifacts,
		Pagination: paginationResponseToProto(pagination, total),
	}, nil
}

// Helper functions

func testResultToProto(result *database.TestResult) *conductorv1.TestResult {
	if result == nil {
		return nil
	}

	protoResult := &conductorv1.TestResult{
		Id:           result.ID.String(),
		RunId:        result.RunID.String(),
		TestName:     result.TestName,
		Status:       testStatusToProto(result.Status),
		RetryAttempt: int32(result.RetryCount),
	}

	if result.SuiteName != nil {
		protoResult.SuiteName = *result.SuiteName
	}
	if result.DurationMs != nil {
		protoResult.DurationMs = *result.DurationMs
	}
	if result.ErrorMessage != nil {
		protoResult.ErrorMessage = *result.ErrorMessage
	}
	if result.StackTrace != nil {
		protoResult.StackTrace = *result.StackTrace
	}
	if result.Stdout != nil {
		protoResult.Stdout = *result.Stdout
	}
	if result.Stderr != nil {
		protoResult.Stderr = *result.Stderr
	}

	return protoResult
}

func testStatusToProto(status database.ResultStatus) conductorv1.TestStatus {
	switch status {
	case database.ResultStatusPass:
		return conductorv1.TestStatus_TEST_STATUS_PASS
	case database.ResultStatusFail:
		return conductorv1.TestStatus_TEST_STATUS_FAIL
	case database.ResultStatusSkip:
		return conductorv1.TestStatus_TEST_STATUS_SKIP
	case database.ResultStatusError:
		return conductorv1.TestStatus_TEST_STATUS_ERROR
	default:
		return conductorv1.TestStatus_TEST_STATUS_UNSPECIFIED
	}
}

func testStatusFromProto(status conductorv1.TestStatus) database.ResultStatus {
	switch status {
	case conductorv1.TestStatus_TEST_STATUS_PASS:
		return database.ResultStatusPass
	case conductorv1.TestStatus_TEST_STATUS_FAIL:
		return database.ResultStatusFail
	case conductorv1.TestStatus_TEST_STATUS_SKIP:
		return database.ResultStatusSkip
	case conductorv1.TestStatus_TEST_STATUS_ERROR:
		return database.ResultStatusError
	default:
		return database.ResultStatusError
	}
}

func artifactToProto(artifact *database.Artifact) *conductorv1.Artifact {
	if artifact == nil {
		return nil
	}

	protoArtifact := &conductorv1.Artifact{
		Id:        artifact.ID.String(),
		RunId:     artifact.RunID.String(),
		Name:      artifact.Name,
		Path:      artifact.Path,
		CreatedAt: timestamppb.New(artifact.CreatedAt),
	}

	if artifact.ContentType != nil {
		protoArtifact.ContentType = *artifact.ContentType
	}
	if artifact.SizeBytes != nil {
		protoArtifact.SizeBytes = *artifact.SizeBytes
	}

	return protoArtifact
}
