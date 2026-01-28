package executor

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/rs/zerolog"
)

// SubprocessExecutor runs tests as subprocesses on the agent host.
type SubprocessExecutor struct {
	workspaceDir string
	logger       zerolog.Logger
}

// NewSubprocessExecutor creates a new subprocess executor.
func NewSubprocessExecutor(workspaceDir string, logger zerolog.Logger) *SubprocessExecutor {
	return &SubprocessExecutor{
		workspaceDir: workspaceDir,
		logger:       logger.With().Str("executor", "subprocess").Logger(),
	}
}

// Name returns the executor name.
func (e *SubprocessExecutor) Name() string {
	return "subprocess"
}

// Execute runs tests as subprocesses.
func (e *SubprocessExecutor) Execute(ctx context.Context, req *ExecutionRequest, reporter ResultReporter) (*ExecutionResult, error) {
	startTime := time.Now()

	// Determine working directory
	workDir := req.WorkDir
	if req.WorkingDirectory != "" {
		workDir = filepath.Join(req.WorkDir, req.WorkingDirectory)
	}

	// Verify working directory exists
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("working directory does not exist: %s", workDir)
	}

	// Setup environment
	env := e.setupEnvironment(req)

	// Run setup commands
	if err := reporter.ReportProgress(ctx, req.RunID, req.ShardID, "setup", "Running setup commands", 5, 0, len(req.Tests)); err != nil {
		e.logger.Warn().Err(err).Msg("Failed to report progress")
	}

	for i, cmd := range req.SetupCommands {
		e.logger.Debug().Str("command", cmd).Int("index", i).Msg("Running setup command")
		if err := e.runCommand(ctx, req.RunID, req.ShardID, workDir, cmd, env, reporter); err != nil {
			return &ExecutionResult{
				Error:    fmt.Sprintf("setup command %d failed: %v", i, err),
				Duration: time.Since(startTime),
			}, nil
		}
	}

	// Execute tests
	result := &ExecutionResult{
		Summary:     &ExecutionSummary{Total: len(req.Tests)},
		TestResults: make([]*TestResult, 0, len(req.Tests)),
		Duration:    0,
	}

	if err := e.executeTests(ctx, req, workDir, env, reporter, result); err != nil {
		return nil, err
	}

	// Run teardown commands
	if err := reporter.ReportProgress(ctx, req.RunID, req.ShardID, "teardown", "Running teardown commands", 95, len(req.Tests), len(req.Tests)); err != nil {
		e.logger.Warn().Err(err).Msg("Failed to report progress")
	}

	for i, cmd := range req.TeardownCommands {
		e.logger.Debug().Str("command", cmd).Int("index", i).Msg("Running teardown command")
		// Don't fail on teardown errors, just log them
		if err := e.runCommand(ctx, req.RunID, req.ShardID, workDir, cmd, env, reporter); err != nil {
			e.logger.Warn().Err(err).Str("command", cmd).Msg("Teardown command failed")
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// executeTest runs a single test with optional retries.
func (e *SubprocessExecutor) executeTest(ctx context.Context, runID, shardIDValue, workDir string, test *conductorv1.TestToRun, env []string, reporter ResultReporter) *TestResult {
	maxAttempts := int(test.RetryCount) + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	// Merge test-specific environment
	testEnv := env
	if len(test.Environment) > 0 {
		testEnv = make([]string, len(env), len(env)+len(test.Environment))
		copy(testEnv, env)
		for k, v := range test.Environment {
			testEnv = append(testEnv, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Apply test-specific timeout
	testCtx := ctx
	if test.Timeout != nil && test.Timeout.Seconds > 0 {
		var cancel context.CancelFunc
		timeout := time.Duration(test.Timeout.Seconds)*time.Second + time.Duration(test.Timeout.Nanos)*time.Nanosecond
		testCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var lastResult *TestResult

	for attempt := 0; attempt < maxAttempts; attempt++ {
		e.logger.Debug().
			Str("test_id", test.TestId).
			Str("test_name", test.Name).
			Int("attempt", attempt+1).
			Int("max_attempts", maxAttempts).
			Msg("Executing test")

		lastResult = e.runTest(testCtx, runID, shardIDValue, workDir, test, testEnv, reporter, attempt)

		// If passed, don't retry
		if lastResult.Status == conductorv1.TestStatus_TEST_STATUS_PASS {
			break
		}

		// If skipped, don't retry
		if lastResult.Status == conductorv1.TestStatus_TEST_STATUS_SKIP {
			break
		}

		// If context cancelled, don't retry
		if testCtx.Err() != nil {
			break
		}
	}

	return lastResult
}

func (e *SubprocessExecutor) executeTests(ctx context.Context, req *ExecutionRequest, workDir string, env []string, reporter ResultReporter, result *ExecutionResult) error {
	maxParallel := req.MaxParallelTests
	if maxParallel <= 0 {
		maxParallel = 1
	}
	if maxParallel > len(req.Tests) {
		maxParallel = len(req.Tests)
	}

	if maxParallel <= 1 {
		for i, test := range req.Tests {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			progress := int(float64(i+1)/float64(len(req.Tests))*80) + 10 // 10-90%
			if err := reporter.ReportProgress(ctx, req.RunID, req.ShardID, "testing", fmt.Sprintf("Running test: %s", test.Name), progress, i, len(req.Tests)); err != nil {
				e.logger.Warn().Err(err).Msg("Failed to report progress")
			}

			testResult := e.executeTest(ctx, req.RunID, req.ShardID, workDir, test, env, reporter)
			result.TestResults = append(result.TestResults, testResult)
			e.updateSummary(result, testResult)
			if err := reportTestResult(ctx, reporter, req, test, testResult); err != nil {
				e.logger.Warn().Err(err).Msg("Failed to report test result")
			}
		}
		return nil
	}

	type job struct {
		index int
		test  *conductorv1.TestToRun
	}
	type jobResult struct {
		test   *conductorv1.TestToRun
		result *TestResult
	}

	jobs := make(chan job)
	results := make(chan jobResult)

	var wg sync.WaitGroup
	for i := 0; i < maxParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}
				res := e.executeTest(ctx, req.RunID, req.ShardID, workDir, j.test, env, reporter)
				select {
				case results <- jobResult{test: j.test, result: res}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		for i, test := range req.Tests {
			jobs <- job{index: i, test: test}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	completed := 0
	for res := range results {
		completed++
		result.TestResults = append(result.TestResults, res.result)
		e.updateSummary(result, res.result)
		progress := int(float64(completed)/float64(len(req.Tests))*80) + 10
		if err := reporter.ReportProgress(ctx, req.RunID, req.ShardID, "testing", fmt.Sprintf("Completed test: %s", res.test.Name), progress, completed, len(req.Tests)); err != nil {
			e.logger.Warn().Err(err).Msg("Failed to report progress")
		}
		if err := reportTestResult(ctx, reporter, req, res.test, res.result); err != nil {
			e.logger.Warn().Err(err).Msg("Failed to report test result")
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func (e *SubprocessExecutor) updateSummary(result *ExecutionResult, testResult *TestResult) {
	switch testResult.Status {
	case conductorv1.TestStatus_TEST_STATUS_PASS:
		result.Summary.Passed++
	case conductorv1.TestStatus_TEST_STATUS_FAIL:
		result.Summary.Failed++
	case conductorv1.TestStatus_TEST_STATUS_SKIP:
		result.Summary.Skipped++
	case conductorv1.TestStatus_TEST_STATUS_ERROR:
		result.Summary.Errored++
	}
}

func reportTestResult(ctx context.Context, reporter ResultReporter, req *ExecutionRequest, test *conductorv1.TestToRun, testResult *TestResult) error {
	return reporter.ReportTestResult(ctx, req.RunID, req.ShardID, &conductorv1.TestResultEvent{
		TestId:       test.TestId,
		TestName:     testResult.TestName,
		Status:       testResult.Status,
		Duration:     durationToProto(testResult.Duration),
		ErrorMessage: testResult.ErrorMessage,
		StackTrace:   testResult.StackTrace,
		RetryAttempt: int32(testResult.RetryAttempt),
		Metadata:     testResult.Metadata,
	})
}

// runTest executes a single test command.
func (e *SubprocessExecutor) runTest(ctx context.Context, runID, shardID, workDir string, test *conductorv1.TestToRun, env []string, reporter ResultReporter, attempt int) *TestResult {
	startTime := time.Now()

	result := &TestResult{
		TestID:       test.TestId,
		TestName:     test.Name,
		Status:       conductorv1.TestStatus_TEST_STATUS_ERROR,
		RetryAttempt: attempt,
		Metadata:     make(map[string]string),
	}

	// Parse command
	args := parseCommand(test.Command)
	if len(args) == 0 {
		result.ErrorMessage = "empty command"
		result.Duration = time.Since(startTime)
		return result
	}

	// Create command
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = workDir
	cmd.Env = env

	// Setup pipes for output capture
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to create stdout pipe: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to create stderr pipe: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Start command
	if err := cmd.Start(); err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to start command: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Capture output concurrently
	var wg sync.WaitGroup
	var stdoutBuf, stderrBuf bytes.Buffer

	wg.Add(2)
	go func() {
		defer wg.Done()
		e.captureOutput(ctx, runID, shardID, stdout, conductorv1.LogStream_LOG_STREAM_STDOUT, reporter, &stdoutBuf)
	}()
	go func() {
		defer wg.Done()
		e.captureOutput(ctx, runID, shardID, stderr, conductorv1.LogStream_LOG_STREAM_STDERR, reporter, &stderrBuf)
	}()

	// Wait for output capture
	wg.Wait()

	// Wait for command to complete
	err = cmd.Wait()
	result.Duration = time.Since(startTime)

	// Determine result status
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Status = conductorv1.TestStatus_TEST_STATUS_ERROR
			result.ErrorMessage = "test timed out"
		} else if errors.Is(ctx.Err(), context.Canceled) {
			result.Status = conductorv1.TestStatus_TEST_STATUS_ERROR
			result.ErrorMessage = "test cancelled"
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.Status = conductorv1.TestStatus_TEST_STATUS_FAIL
			result.ErrorMessage = fmt.Sprintf("exit code %d", exitErr.ExitCode())
			if stderrBuf.Len() > 0 {
				result.StackTrace = truncateString(stderrBuf.String(), 4096)
			}
		} else {
			result.Status = conductorv1.TestStatus_TEST_STATUS_ERROR
			result.ErrorMessage = err.Error()
		}
	} else {
		result.Status = conductorv1.TestStatus_TEST_STATUS_PASS
	}

	return result
}

// runCommand executes a setup/teardown command.
func (e *SubprocessExecutor) runCommand(ctx context.Context, runID, shardID, workDir, command string, env []string, reporter ResultReporter) error {
	args := parseCommand(command)
	if len(args) == 0 {
		return errors.New("empty command")
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = workDir
	cmd.Env = env

	// Setup process group for proper cleanup
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Capture output
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		e.captureOutput(ctx, runID, shardID, stdout, conductorv1.LogStream_LOG_STREAM_STDOUT, reporter, nil)
	}()
	go func() {
		defer wg.Done()
		e.captureOutput(ctx, runID, shardID, stderr, conductorv1.LogStream_LOG_STREAM_STDERR, reporter, nil)
	}()

	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

// setupEnvironment prepares the environment variables for execution.
func (e *SubprocessExecutor) setupEnvironment(req *ExecutionRequest) []string {
	// Start with current environment
	env := os.Environ()

	// Add conductor-specific variables
	env = append(env,
		fmt.Sprintf("CONDUCTOR_RUN_ID=%s", req.RunID),
		fmt.Sprintf("CONDUCTOR_WORKSPACE=%s", req.WorkDir),
	)

	// Add request environment variables
	for k, v := range req.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}

// captureOutput reads from a pipe and streams it to the reporter.
func (e *SubprocessExecutor) captureOutput(ctx context.Context, runID, shardID string, r io.Reader, stream conductorv1.LogStream, reporter ResultReporter, buf *bytes.Buffer) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 64KB buffer, 1MB max line

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Bytes()

		// Write to buffer if provided
		if buf != nil {
			buf.Write(line)
			buf.WriteByte('\n')
		}

		// Stream to reporter
		data := make([]byte, len(line)+1)
		copy(data, line)
		data[len(line)] = '\n'

		if err := reporter.StreamLogs(ctx, runID, shardID, stream, data); err != nil {
			e.logger.Debug().Err(err).Msg("Failed to stream logs")
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		e.logger.Debug().Err(err).Msg("Scanner error")
	}
}

// parseCommand splits a command string into arguments.
// Handles basic quoting.
func parseCommand(command string) []string {
	var args []string
	var current strings.Builder
	var inQuote bool
	var quoteChar rune

	for _, r := range command {
		switch {
		case inQuote:
			if r == quoteChar {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			inQuote = true
			quoteChar = r
		case r == ' ' || r == '\t':
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// truncateString truncates a string to maxLen.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// durationToProto converts a time.Duration to a protobuf Duration.
func durationToProto(d time.Duration) *conductorv1.Duration {
	return &conductorv1.Duration{
		Seconds: int64(d.Seconds()),
		Nanos:   int32(d.Nanoseconds() % 1e9),
	}
}
