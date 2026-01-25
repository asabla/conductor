package executor

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/rs/zerolog"
)

func TestContainerExecutorName(t *testing.T) {
	executor := &ContainerExecutor{}
	if executor.Name() != "container" {
		t.Fatalf("expected name container, got %s", executor.Name())
	}
}

func TestContainerExecutorExecute(t *testing.T) {
	workDir := t.TempDir()
	logger := zerolog.New(io.Discard)

	executor, err := NewContainerExecutor("", workDir, logger)
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	defer executor.Close()

	reporter := newTestReporter()
	req := &ExecutionRequest{
		RunID:          "run-container",
		WorkDir:        workDir,
		ContainerImage: "ubuntu:22.04",
		Tests: []*conductorv1.TestToRun{
			{
				TestId:  "test-1",
				Name:    "container test",
				Command: "echo hello from container",
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, execErr := executor.Execute(ctx, req, reporter)
	if execErr != nil {
		t.Fatalf("unexpected execute error: %v", execErr)
	}
	if result == nil || result.Summary == nil {
		t.Fatalf("expected result summary")
	}
	if result.Summary.Passed != 1 || result.Summary.Total != 1 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}
	if len(result.TestResults) != 1 {
		t.Fatalf("expected one test result")
	}
	if result.TestResults[0].Status != conductorv1.TestStatus_TEST_STATUS_PASS {
		t.Fatalf("expected status pass, got %v", result.TestResults[0].Status)
	}

	logs := reporter.combinedLogs()
	if !strings.Contains(logs, "hello from container") {
		t.Fatalf("expected logs to contain command output, got %q", logs)
	}
}
