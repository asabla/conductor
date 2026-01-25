// Package executor provides test execution drivers for the agent.
package executor

import (
	"context"
	"time"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
)

// Executor is the interface for test execution drivers.
type Executor interface {
	// Execute runs the given tests and reports results via the reporter.
	Execute(ctx context.Context, req *ExecutionRequest, reporter ResultReporter) (*ExecutionResult, error)

	// Name returns the executor name for logging.
	Name() string
}

// ResultReporter is the interface for reporting execution progress and results.
type ResultReporter interface {
	// StreamLogs streams log output from the execution.
	StreamLogs(ctx context.Context, runID string, stream conductorv1.LogStream, data []byte) error

	// ReportTestResult reports an individual test result.
	ReportTestResult(ctx context.Context, runID string, result *conductorv1.TestResultEvent) error

	// ReportProgress reports execution progress.
	ReportProgress(ctx context.Context, runID string, phase string, message string, percent int, completed int, total int) error
}

// ExecutionRequest contains all information needed to execute tests.
type ExecutionRequest struct {
	// RunID is the unique identifier for this run.
	RunID string

	// WorkDir is the absolute path to the repository workspace.
	WorkDir string

	// WorkingDirectory is the relative path within the workspace to run tests.
	WorkingDirectory string

	// Tests to execute.
	Tests []*conductorv1.TestToRun

	// Environment variables to set.
	Environment map[string]string

	// Secrets to inject.
	Secrets []*conductorv1.Secret

	// SetupCommands to run before tests.
	SetupCommands []string

	// TeardownCommands to run after tests.
	TeardownCommands []string

	// ContainerImage for container execution.
	ContainerImage string

	// Timeout for the entire execution.
	Timeout time.Duration
}

// ExecutionResult contains the results of test execution.
type ExecutionResult struct {
	// Summary of all test results.
	Summary *ExecutionSummary

	// TestResults contains individual test results.
	TestResults []*TestResult

	// Duration of the entire execution.
	Duration time.Duration

	// Error message if execution failed due to infrastructure issues.
	Error string
}

// ExecutionSummary provides aggregate statistics.
type ExecutionSummary struct {
	Total   int
	Passed  int
	Failed  int
	Skipped int
	Errored int
}

// TestResult represents the outcome of a single test.
type TestResult struct {
	TestID       string
	TestName     string
	Status       conductorv1.TestStatus
	Duration     time.Duration
	ErrorMessage string
	StackTrace   string
	RetryAttempt int
	Metadata     map[string]string
}

// Factory returns the appropriate executor for the given execution type.
func Factory(execType conductorv1.ExecutionType, subprocess Executor, container Executor) Executor {
	switch execType {
	case conductorv1.ExecutionType_EXECUTION_TYPE_CONTAINER:
		if container != nil {
			return container
		}
		// Fall back to subprocess if container not available
		return subprocess
	default:
		return subprocess
	}
}
