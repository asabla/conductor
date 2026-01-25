package executor

import (
	"context"
	"strings"
	"sync"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
)

type logEntry struct {
	runID  string
	stream conductorv1.LogStream
	data   string
}

type progressEntry struct {
	runID     string
	phase     string
	message   string
	percent   int
	completed int
	total     int
}

type testReporter struct {
	mu       sync.Mutex
	logs     []logEntry
	results  []*conductorv1.TestResultEvent
	progress []progressEntry
}

func newTestReporter() *testReporter {
	return &testReporter{}
}

func (r *testReporter) StreamLogs(ctx context.Context, runID, shardID string, stream conductorv1.LogStream, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, logEntry{runID: runID, stream: stream, data: string(data)})
	return nil
}

func (r *testReporter) ReportTestResult(ctx context.Context, runID, shardID string, result *conductorv1.TestResultEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, result)
	return nil
}

func (r *testReporter) ReportProgress(ctx context.Context, runID, shardID string, phase string, message string, percent int, completed int, total int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.progress = append(r.progress, progressEntry{
		runID:     runID,
		phase:     phase,
		message:   message,
		percent:   percent,
		completed: completed,
		total:     total,
	})
	return nil
}

func (r *testReporter) combinedLogs() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var builder strings.Builder
	for _, entry := range r.logs {
		builder.WriteString(entry.data)
	}
	return builder.String()
}
