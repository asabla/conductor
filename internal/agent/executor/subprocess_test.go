package executor

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/rs/zerolog"
)

func TestSubprocessExecutorName(t *testing.T) {
	executor := NewSubprocessExecutor("/tmp", zerolog.New(io.Discard))
	if executor.Name() != "subprocess" {
		t.Fatalf("expected name subprocess, got %s", executor.Name())
	}
}

func TestSubprocessExecutorExecuteSuccess(t *testing.T) {
	workDir := t.TempDir()
	executor := NewSubprocessExecutor(workDir, zerolog.New(io.Discard))
	reporter := newTestReporter()

	req := &ExecutionRequest{
		RunID:   "run-success",
		WorkDir: workDir,
		Environment: map[string]string{
			"CUSTOM": "value",
			"SECRET": "s3cret",
		},
		Tests: []*conductorv1.TestToRun{
			{
				TestId:  "test-1",
				Name:    "env test",
				Command: "sh -c \"echo $CUSTOM-$SECRET\"",
			},
		},
	}

	result, err := executor.Execute(context.Background(), req, reporter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("expected no execution error, got %s", result.Error)
	}
	if result.Summary == nil || result.Summary.Passed != 1 || result.Summary.Total != 1 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}

	logs := reporter.combinedLogs()
	if !strings.Contains(logs, "value-s3cret") {
		t.Fatalf("expected logs to contain environment output, got %q", logs)
	}
	if len(reporter.results) != 1 {
		t.Fatalf("expected 1 test result, got %d", len(reporter.results))
	}
	if reporter.results[0].Status != conductorv1.TestStatus_TEST_STATUS_PASS {
		t.Fatalf("expected test status pass, got %v", reporter.results[0].Status)
	}
}

func TestSubprocessExecutorSetupFailure(t *testing.T) {
	workDir := t.TempDir()
	executor := NewSubprocessExecutor(workDir, zerolog.New(io.Discard))
	reporter := newTestReporter()

	req := &ExecutionRequest{
		RunID:            "run-setup-fail",
		WorkDir:          workDir,
		SetupCommands:    []string{"false"},
		TeardownCommands: []string{"echo teardown"},
		Tests: []*conductorv1.TestToRun{
			{
				TestId:  "test-1",
				Name:    "noop",
				Command: "echo should-not-run",
			},
		},
	}

	result, err := executor.Execute(context.Background(), req, reporter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Error == "" {
		t.Fatalf("expected execution error for setup failure")
	}
	if !strings.Contains(result.Error, "setup command 0 failed") {
		t.Fatalf("unexpected error message: %s", result.Error)
	}
}

func TestSubprocessExecutorTestFailureExitCode(t *testing.T) {
	workDir := t.TempDir()
	executor := NewSubprocessExecutor(workDir, zerolog.New(io.Discard))
	reporter := newTestReporter()

	req := &ExecutionRequest{
		RunID:   "run-fail",
		WorkDir: workDir,
		Tests: []*conductorv1.TestToRun{
			{
				TestId:  "test-1",
				Name:    "failure test",
				Command: "sh -c \"echo boom >&2; exit 2\"",
			},
		},
	}

	result, err := executor.Execute(context.Background(), req, reporter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.TestResults) != 1 {
		t.Fatalf("expected one test result")
	}
	testResult := result.TestResults[0]
	if testResult.Status != conductorv1.TestStatus_TEST_STATUS_FAIL {
		t.Fatalf("expected status fail, got %v", testResult.Status)
	}
	if !strings.Contains(testResult.ErrorMessage, "exit code 2") {
		t.Fatalf("expected exit code error, got %s", testResult.ErrorMessage)
	}
	if !strings.Contains(testResult.StackTrace, "boom") {
		t.Fatalf("expected stack trace to include stderr output")
	}
}

func TestSubprocessExecutorRetries(t *testing.T) {
	workDir := t.TempDir()
	executor := NewSubprocessExecutor(workDir, zerolog.New(io.Discard))
	reporter := newTestReporter()

	flagPath := filepath.Join(workDir, "retry-flag")
	cmd := "sh -c \"if [ -f retry-flag ]; then exit 0; else touch retry-flag; exit 1; fi\""

	req := &ExecutionRequest{
		RunID:   "run-retry",
		WorkDir: workDir,
		Tests: []*conductorv1.TestToRun{
			{
				TestId:     "test-1",
				Name:       "retry test",
				Command:    cmd,
				RetryCount: 1,
			},
		},
	}

	result, err := executor.Execute(context.Background(), req, reporter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.TestResults) != 1 {
		t.Fatalf("expected one test result")
	}
	if _, statErr := os.Stat(flagPath); statErr != nil {
		t.Fatalf("expected retry flag file to exist: %v", statErr)
	}

	testResult := result.TestResults[0]
	if testResult.Status != conductorv1.TestStatus_TEST_STATUS_PASS {
		t.Fatalf("expected status pass after retry, got %v", testResult.Status)
	}
	if testResult.RetryAttempt != 1 {
		t.Fatalf("expected retry attempt 1, got %d", testResult.RetryAttempt)
	}
}

func TestSubprocessExecutorTimeout(t *testing.T) {
	workDir := t.TempDir()
	executor := NewSubprocessExecutor(workDir, zerolog.New(io.Discard))
	reporter := newTestReporter()

	req := &ExecutionRequest{
		RunID:   "run-timeout",
		WorkDir: workDir,
		Tests: []*conductorv1.TestToRun{
			{
				TestId:  "test-1",
				Name:    "timeout test",
				Command: "sh -c \"sleep 2\"",
				Timeout: &conductorv1.Duration{Seconds: 1},
			},
		},
	}

	result, err := executor.Execute(context.Background(), req, reporter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.TestResults) != 1 {
		t.Fatalf("expected one test result")
	}
	if result.TestResults[0].Status != conductorv1.TestStatus_TEST_STATUS_ERROR {
		t.Fatalf("expected status error, got %v", result.TestResults[0].Status)
	}
	if result.TestResults[0].ErrorMessage != "test timed out" {
		t.Fatalf("expected timeout error message, got %s", result.TestResults[0].ErrorMessage)
	}
}

func TestParseCommand(t *testing.T) {
	args := parseCommand("echo \"hello world\" 'from conductor'")
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
	if args[1] != "hello world" || args[2] != "from conductor" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestTruncateString(t *testing.T) {
	if truncateString("short", 10) != "short" {
		t.Fatalf("expected string to be unchanged")
	}
	if truncateString("this is long", 7) != "this..." {
		t.Fatalf("unexpected truncation output")
	}
}

func TestDurationToProto(t *testing.T) {
	duration := 1500 * time.Millisecond
	proto := durationToProto(duration)
	if proto.Seconds != 1 {
		t.Fatalf("expected 1 second, got %d", proto.Seconds)
	}
	if proto.Nanos <= 0 {
		t.Fatalf("expected nanos to be set")
	}
}
