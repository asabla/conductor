package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/agent/executor"
	"github.com/rs/zerolog"
)

// Reporter handles reporting results back to the control plane.
type Reporter struct {
	client   *Client
	logger   zerolog.Logger
	sequence atomic.Int64
	mu       sync.Mutex
}

// NewReporter creates a new result reporter.
func NewReporter(client *Client, logger zerolog.Logger) *Reporter {
	return &Reporter{
		client: client,
		logger: logger.With().Str("component", "reporter").Logger(),
	}
}

// StreamLogs streams log output to the control plane.
func (r *Reporter) StreamLogs(ctx context.Context, runID string, stream conductorv1.LogStream, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	msg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_ResultStream{
			ResultStream: &conductorv1.ResultStream{
				RunId:    runID,
				Sequence: r.sequence.Add(1),
				Payload: &conductorv1.ResultStream_LogChunk{
					LogChunk: &conductorv1.LogChunk{
						Stream: stream,
						Data:   data,
					},
				},
			},
		},
	}

	return r.client.Send(msg)
}

// ReportTestResult reports an individual test result.
func (r *Reporter) ReportTestResult(ctx context.Context, runID string, result *conductorv1.TestResultEvent) error {
	msg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_ResultStream{
			ResultStream: &conductorv1.ResultStream{
				RunId:    runID,
				Sequence: r.sequence.Add(1),
				Payload: &conductorv1.ResultStream_TestResult{
					TestResult: result,
				},
			},
		},
	}

	return r.client.Send(msg)
}

// ReportProgress reports execution progress.
func (r *Reporter) ReportProgress(ctx context.Context, runID string, phase string, message string, percent int, completed int, total int) error {
	msg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_ResultStream{
			ResultStream: &conductorv1.ResultStream{
				RunId:    runID,
				Sequence: r.sequence.Add(1),
				Payload: &conductorv1.ResultStream_Progress{
					Progress: &conductorv1.ProgressUpdate{
						Phase:           phase,
						Message:         message,
						PercentComplete: int32(percent),
						TestsCompleted:  int32(completed),
						TestsTotal:      int32(total),
					},
				},
			},
		},
	}

	return r.client.Send(msg)
}

// ReportComplete reports run completion status.
func (r *Reporter) ReportComplete(ctx context.Context, runID string, status conductorv1.RunStatus, errorMsg string) error {
	msg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_ResultStream{
			ResultStream: &conductorv1.ResultStream{
				RunId:    runID,
				Sequence: r.sequence.Add(1),
				Payload: &conductorv1.ResultStream_RunComplete{
					RunComplete: &conductorv1.RunComplete{
						Status:       status,
						ErrorMessage: errorMsg,
					},
				},
			},
		},
	}

	return r.client.Send(msg)
}

// ReportRunComplete reports full run completion with summary.
func (r *Reporter) ReportRunComplete(ctx context.Context, runID string, result *executor.ExecutionResult) error {
	var summary *conductorv1.RunSummary
	if result.Summary != nil {
		summary = &conductorv1.RunSummary{
			Total:   int32(result.Summary.Total),
			Passed:  int32(result.Summary.Passed),
			Failed:  int32(result.Summary.Failed),
			Skipped: int32(result.Summary.Skipped),
			Errored: int32(result.Summary.Errored),
			Duration: &conductorv1.Duration{
				Seconds: int64(result.Duration.Seconds()),
				Nanos:   int32(result.Duration.Nanoseconds() % 1e9),
			},
		}
	}

	// Determine status
	status := conductorv1.RunStatus_RUN_STATUS_PASSED
	if result.Error != "" {
		status = conductorv1.RunStatus_RUN_STATUS_ERROR
	} else if result.Summary != nil && (result.Summary.Failed > 0 || result.Summary.Errored > 0) {
		status = conductorv1.RunStatus_RUN_STATUS_FAILED
	}

	msg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_ResultStream{
			ResultStream: &conductorv1.ResultStream{
				RunId:    runID,
				Sequence: r.sequence.Add(1),
				Payload: &conductorv1.ResultStream_RunComplete{
					RunComplete: &conductorv1.RunComplete{
						Status:       status,
						Summary:      summary,
						ErrorMessage: result.Error,
						Duration: &conductorv1.Duration{
							Seconds: int64(result.Duration.Seconds()),
							Nanos:   int32(result.Duration.Nanoseconds() % 1e9),
						},
					},
				},
			},
		},
	}

	return r.client.Send(msg)
}

// UploadArtifact uploads an artifact file to storage.
func (r *Reporter) UploadArtifact(ctx context.Context, runID string, artifactPath string) error {
	// TODO: Implement artifact upload to S3/MinIO
	// For now, just log that we would upload
	r.logger.Debug().
		Str("run_id", runID).
		Str("path", artifactPath).
		Msg("Would upload artifact")

	return nil
}

// Ensure Reporter implements executor.ResultReporter
var _ executor.ResultReporter = (*Reporter)(nil)

// timestampNow returns the current time as a protobuf timestamp.
func timestampNow() *time.Time {
	t := time.Now()
	return &t
}
