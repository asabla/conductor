// Package result provides test result collection and processing services.
package result

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// ResultCollector defines the interface for collecting and processing test results.
type ResultCollector interface {
	// ProcessResults stores test results for a run.
	ProcessResults(ctx context.Context, runID uuid.UUID, results []TestResult) error

	// ProcessArtifact stores an artifact for a run.
	ProcessArtifact(ctx context.Context, runID uuid.UUID, artifact ArtifactInfo) error

	// UpdateRunSummary calculates and updates the run summary from stored results.
	UpdateRunSummary(ctx context.Context, runID uuid.UUID) error

	// CompleteRun marks a run as completed with the given status.
	CompleteRun(ctx context.Context, runID uuid.UUID, status database.RunStatus, errorMsg string) error

	// GetRunResults retrieves all results for a run.
	GetRunResults(ctx context.Context, runID uuid.UUID) ([]database.TestResult, error)

	// GetRunArtifacts retrieves all artifacts for a run.
	GetRunArtifacts(ctx context.Context, runID uuid.UUID) ([]database.Artifact, error)
}

// TestResult represents a single test result received from an agent.
type TestResult struct {
	TestID       string
	TestName     string
	SuiteName    string
	Status       string // pass, fail, skip, error
	DurationMs   int64
	ErrorMessage string
	StackTrace   string
	Stdout       string
	Stderr       string
	RetryCount   int
	Metadata     map[string]string
}

// ArtifactInfo contains information about an uploaded artifact.
type ArtifactInfo struct {
	Name        string
	Path        string // Storage path
	ContentType string
	SizeBytes   int64
	Checksum    string
}

// ArtifactStorage defines the interface for artifact storage operations.
type ArtifactStorage interface {
	// Upload uploads an artifact and returns the storage path.
	Upload(ctx context.Context, runID uuid.UUID, name string, reader io.Reader) (string, error)

	// GetPresignedURL generates a presigned URL for downloading an artifact.
	GetPresignedURL(ctx context.Context, path string, expires time.Duration) (string, error)

	// Delete deletes an artifact from storage.
	Delete(ctx context.Context, path string) error
}

// Collector implements the ResultCollector interface.
type Collector struct {
	runRepo      database.TestRunRepository
	resultRepo   database.ResultRepository
	artifactRepo database.ArtifactRepository
	storage      ArtifactStorage
	logger       *slog.Logger
}

// NewCollector creates a new Collector instance.
func NewCollector(
	runRepo database.TestRunRepository,
	resultRepo database.ResultRepository,
	artifactRepo database.ArtifactRepository,
	storage ArtifactStorage,
	logger *slog.Logger,
) *Collector {
	if logger == nil {
		logger = slog.Default()
	}

	return &Collector{
		runRepo:      runRepo,
		resultRepo:   resultRepo,
		artifactRepo: artifactRepo,
		storage:      storage,
		logger:       logger.With("component", "result_collector"),
	}
}

// ProcessResults stores test results for a run.
func (c *Collector) ProcessResults(ctx context.Context, runID uuid.UUID, results []TestResult) error {
	if len(results) == 0 {
		return nil
	}

	c.logger.Debug("processing results",
		"run_id", runID,
		"count", len(results),
	)

	dbResults := make([]database.TestResult, 0, len(results))
	for _, r := range results {
		dbResult := database.TestResult{
			ID:           uuid.New(),
			RunID:        runID,
			TestName:     r.TestName,
			SuiteName:    database.NullString(r.SuiteName),
			Status:       mapResultStatus(r.Status),
			DurationMs:   database.NullInt64(r.DurationMs),
			ErrorMessage: database.NullString(r.ErrorMessage),
			StackTrace:   database.NullString(r.StackTrace),
			Stdout:       database.NullString(r.Stdout),
			Stderr:       database.NullString(r.Stderr),
			RetryCount:   r.RetryCount,
			CreatedAt:    time.Now().UTC(),
		}

		// Parse test definition ID if provided
		if r.TestID != "" {
			if testID, err := uuid.Parse(r.TestID); err == nil {
				dbResult.TestDefinitionID = &testID
			}
		}

		dbResults = append(dbResults, dbResult)
	}

	if err := c.resultRepo.BatchCreate(ctx, dbResults); err != nil {
		return fmt.Errorf("failed to store results: %w", err)
	}

	c.logger.Info("stored results",
		"run_id", runID,
		"count", len(dbResults),
	)

	return nil
}

// ProcessArtifact stores an artifact for a run.
func (c *Collector) ProcessArtifact(ctx context.Context, runID uuid.UUID, artifact ArtifactInfo) error {
	c.logger.Debug("processing artifact",
		"run_id", runID,
		"name", artifact.Name,
		"size", artifact.SizeBytes,
	)

	dbArtifact := &database.Artifact{
		ID:          uuid.New(),
		RunID:       runID,
		Name:        artifact.Name,
		Path:        artifact.Path,
		ContentType: database.NullString(artifact.ContentType),
		SizeBytes:   database.NullInt64(artifact.SizeBytes),
		CreatedAt:   time.Now().UTC(),
	}

	if err := c.artifactRepo.Create(ctx, dbArtifact); err != nil {
		return fmt.Errorf("failed to store artifact: %w", err)
	}

	c.logger.Info("stored artifact",
		"run_id", runID,
		"artifact_id", dbArtifact.ID,
		"name", artifact.Name,
	)

	return nil
}

// UpdateRunSummary calculates and updates the run summary from stored results.
func (c *Collector) UpdateRunSummary(ctx context.Context, runID uuid.UUID) error {
	c.logger.Debug("updating run summary", "run_id", runID)

	// Get result counts by status
	counts, err := c.resultRepo.CountByRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to count results: %w", err)
	}

	// Calculate totals
	var total, passed, failed, skipped int
	for status, count := range counts {
		total += int(count)
		switch status {
		case database.ResultStatusPass:
			passed = int(count)
		case database.ResultStatusFail, database.ResultStatusError:
			failed += int(count)
		case database.ResultStatusSkip:
			skipped = int(count)
		}
	}

	// Get run to update
	run, err := c.runRepo.Get(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found: %s", runID)
	}

	// Update run with summary
	run.TotalTests = total
	run.PassedTests = passed
	run.FailedTests = failed
	run.SkippedTests = skipped

	if err := c.runRepo.Update(ctx, run); err != nil {
		return fmt.Errorf("failed to update run: %w", err)
	}

	c.logger.Info("updated run summary",
		"run_id", runID,
		"total", total,
		"passed", passed,
		"failed", failed,
		"skipped", skipped,
	)

	return nil
}

// CompleteRun marks a run as completed with the given status.
func (c *Collector) CompleteRun(ctx context.Context, runID uuid.UUID, status database.RunStatus, errorMsg string) error {
	c.logger.Info("completing run",
		"run_id", runID,
		"status", status,
	)

	run, err := c.runRepo.Get(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found: %s", runID)
	}

	// Calculate duration
	var durationMs int64
	if run.StartedAt != nil {
		durationMs = time.Since(*run.StartedAt).Milliseconds()
	}

	// Prepare results
	results := database.RunResults{
		TotalTests:   run.TotalTests,
		PassedTests:  run.PassedTests,
		FailedTests:  run.FailedTests,
		SkippedTests: run.SkippedTests,
		DurationMs:   durationMs,
		ErrorMessage: errorMsg,
	}

	if err := c.runRepo.Finish(ctx, runID, status, results); err != nil {
		return fmt.Errorf("failed to finish run: %w", err)
	}

	return nil
}

// GetRunResults retrieves all results for a run.
func (c *Collector) GetRunResults(ctx context.Context, runID uuid.UUID) ([]database.TestResult, error) {
	return c.resultRepo.ListByRun(ctx, runID)
}

// GetRunArtifacts retrieves all artifacts for a run.
func (c *Collector) GetRunArtifacts(ctx context.Context, runID uuid.UUID) ([]database.Artifact, error) {
	return c.artifactRepo.ListByRun(ctx, runID)
}

// DetermineRunStatus determines the final run status based on test results.
func DetermineRunStatus(passed, failed, total int, hasError bool) database.RunStatus {
	if hasError {
		return database.RunStatusError
	}
	if total == 0 {
		return database.RunStatusError // No tests ran - something went wrong
	}
	if failed > 0 {
		return database.RunStatusFailed
	}
	return database.RunStatusPassed
}

// mapResultStatus maps a string status to database.ResultStatus.
func mapResultStatus(status string) database.ResultStatus {
	switch status {
	case "pass", "passed", "success":
		return database.ResultStatusPass
	case "fail", "failed", "failure":
		return database.ResultStatusFail
	case "skip", "skipped", "pending":
		return database.ResultStatusSkip
	case "error":
		return database.ResultStatusError
	default:
		return database.ResultStatusError
	}
}

// StreamingCollector handles streaming results from agents.
type StreamingCollector struct {
	*Collector
	bufferSize int
}

// NewStreamingCollector creates a new streaming result collector.
func NewStreamingCollector(
	runRepo database.TestRunRepository,
	resultRepo database.ResultRepository,
	artifactRepo database.ArtifactRepository,
	storage ArtifactStorage,
	logger *slog.Logger,
	bufferSize int,
) *StreamingCollector {
	if bufferSize <= 0 {
		bufferSize = 100
	}

	return &StreamingCollector{
		Collector:  NewCollector(runRepo, resultRepo, artifactRepo, storage, logger),
		bufferSize: bufferSize,
	}
}

// ResultChannel creates a channel for streaming results.
type ResultChannel struct {
	Results   chan TestResult
	Artifacts chan ArtifactInfo
	Done      chan struct{}
	Errors    chan error
}

// NewResultChannel creates a new result channel.
func NewResultChannel(bufferSize int) *ResultChannel {
	return &ResultChannel{
		Results:   make(chan TestResult, bufferSize),
		Artifacts: make(chan ArtifactInfo, bufferSize),
		Done:      make(chan struct{}),
		Errors:    make(chan error, 10),
	}
}

// ProcessStream processes results from a streaming channel.
func (c *StreamingCollector) ProcessStream(ctx context.Context, runID uuid.UUID, ch *ResultChannel) error {
	var resultBatch []TestResult
	flushTicker := time.NewTicker(5 * time.Second)
	defer flushTicker.Stop()

	flush := func() error {
		if len(resultBatch) == 0 {
			return nil
		}
		if err := c.ProcessResults(ctx, runID, resultBatch); err != nil {
			return err
		}
		resultBatch = resultBatch[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case result, ok := <-ch.Results:
			if !ok {
				// Channel closed, flush remaining
				return flush()
			}
			resultBatch = append(resultBatch, result)
			if len(resultBatch) >= c.bufferSize {
				if err := flush(); err != nil {
					ch.Errors <- err
				}
			}

		case artifact, ok := <-ch.Artifacts:
			if !ok {
				continue
			}
			if err := c.ProcessArtifact(ctx, runID, artifact); err != nil {
				ch.Errors <- err
			}

		case <-flushTicker.C:
			if err := flush(); err != nil {
				ch.Errors <- err
			}

		case <-ch.Done:
			// Final flush
			if err := flush(); err != nil {
				return err
			}
			// Update summary
			return c.UpdateRunSummary(ctx, runID)
		}
	}
}
