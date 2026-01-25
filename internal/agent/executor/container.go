package executor

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/rs/zerolog"
)

// ContainerExecutor runs tests inside Docker containers for isolation.
type ContainerExecutor struct {
	client       *client.Client
	workspaceDir string
	logger       zerolog.Logger
}

// NewContainerExecutor creates a new container executor.
func NewContainerExecutor(dockerHost, workspaceDir string, logger zerolog.Logger) (*ContainerExecutor, error) {
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	if dockerHost != "" {
		opts = append(opts, client.WithHost(dockerHost))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := cli.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	return &ContainerExecutor{
		client:       cli,
		workspaceDir: workspaceDir,
		logger:       logger.With().Str("executor", "container").Logger(),
	}, nil
}

// Name returns the executor name.
func (e *ContainerExecutor) Name() string {
	return "container"
}

// Execute runs tests inside a Docker container.
func (e *ContainerExecutor) Execute(ctx context.Context, req *ExecutionRequest, reporter ResultReporter) (*ExecutionResult, error) {
	startTime := time.Now()

	// Determine container image
	containerImage := req.ContainerImage
	if containerImage == "" {
		containerImage = "ubuntu:22.04" // Default image
	}

	e.logger.Info().
		Str("image", containerImage).
		Str("run_id", req.RunID).
		Msg("Starting container execution")

	// Pull image
	if err := reporter.ReportProgress(ctx, req.RunID, "setup", "Pulling container image", 5, 0, len(req.Tests)); err != nil {
		e.logger.Warn().Err(err).Msg("Failed to report progress")
	}

	if err := e.pullImage(ctx, containerImage); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("failed to pull image: %v", err),
			Duration: time.Since(startTime),
		}, nil
	}

	// Create container
	if err := reporter.ReportProgress(ctx, req.RunID, "setup", "Creating container", 10, 0, len(req.Tests)); err != nil {
		e.logger.Warn().Err(err).Msg("Failed to report progress")
	}

	containerID, err := e.createContainer(ctx, req, containerImage)
	if err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("failed to create container: %v", err),
			Duration: time.Since(startTime),
		}, nil
	}

	// Ensure cleanup
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := e.cleanup(cleanupCtx, containerID); err != nil {
			e.logger.Warn().Err(err).Str("container_id", containerID).Msg("Failed to cleanup container")
		}
	}()

	// Start container
	if err := e.startContainer(ctx, containerID); err != nil {
		return &ExecutionResult{
			Error:    fmt.Sprintf("failed to start container: %v", err),
			Duration: time.Since(startTime),
		}, nil
	}

	// Run setup commands
	if err := reporter.ReportProgress(ctx, req.RunID, "setup", "Running setup commands", 15, 0, len(req.Tests)); err != nil {
		e.logger.Warn().Err(err).Msg("Failed to report progress")
	}

	for i, cmd := range req.SetupCommands {
		e.logger.Debug().Str("command", cmd).Int("index", i).Msg("Running setup command in container")
		if err := e.execInContainer(ctx, req.RunID, containerID, cmd, reporter); err != nil {
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

	for i, test := range req.Tests {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		progress := int(float64(i+1)/float64(len(req.Tests))*70) + 20 // 20-90%
		if err := reporter.ReportProgress(ctx, req.RunID, "testing", fmt.Sprintf("Running test: %s", test.Name), progress, i, len(req.Tests)); err != nil {
			e.logger.Warn().Err(err).Msg("Failed to report progress")
		}

		testResult := e.executeTest(ctx, req.RunID, containerID, test, reporter)
		result.TestResults = append(result.TestResults, testResult)

		// Update summary
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

		// Report test result
		if err := reporter.ReportTestResult(ctx, req.RunID, &conductorv1.TestResultEvent{
			TestId:       test.TestId,
			TestName:     testResult.TestName,
			Status:       testResult.Status,
			Duration:     durationToProto(testResult.Duration),
			ErrorMessage: testResult.ErrorMessage,
			StackTrace:   testResult.StackTrace,
			RetryAttempt: int32(testResult.RetryAttempt),
			Metadata:     testResult.Metadata,
		}); err != nil {
			e.logger.Warn().Err(err).Msg("Failed to report test result")
		}
	}

	// Run teardown commands
	if err := reporter.ReportProgress(ctx, req.RunID, "teardown", "Running teardown commands", 95, len(req.Tests), len(req.Tests)); err != nil {
		e.logger.Warn().Err(err).Msg("Failed to report progress")
	}

	for i, cmd := range req.TeardownCommands {
		e.logger.Debug().Str("command", cmd).Int("index", i).Msg("Running teardown command in container")
		if err := e.execInContainer(ctx, req.RunID, containerID, cmd, reporter); err != nil {
			e.logger.Warn().Err(err).Str("command", cmd).Msg("Teardown command failed")
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// pullImage pulls the container image if not already present.
func (e *ContainerExecutor) pullImage(ctx context.Context, imageName string) error {
	e.logger.Debug().Str("image", imageName).Msg("Pulling image")

	reader, err := e.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	// Consume the output to complete the pull
	_, err = io.Copy(io.Discard, reader)
	return err
}

// createContainer creates a Docker container for test execution.
func (e *ContainerExecutor) createContainer(ctx context.Context, req *ExecutionRequest, imageName string) (string, error) {
	// Build environment variables
	env := make([]string, 0, len(req.Environment)+2)
	env = append(env, fmt.Sprintf("CONDUCTOR_RUN_ID=%s", req.RunID))
	env = append(env, "CONDUCTOR_WORKSPACE=/workspace")

	for k, v := range req.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Determine working directory in container
	workDir := "/workspace"
	if req.WorkingDirectory != "" {
		workDir = filepath.Join("/workspace", req.WorkingDirectory)
	}

	// Container configuration
	containerConfig := &container.Config{
		Image:      imageName,
		Env:        env,
		WorkingDir: workDir,
		Cmd:        []string{"sleep", "infinity"}, // Keep container running
		Labels: map[string]string{
			"conductor.run_id": req.RunID,
			"conductor.agent":  "true",
		},
	}

	// Host configuration with resource limits
	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   req.WorkDir,
				Target:   "/workspace",
				ReadOnly: false,
			},
		},
		Resources: container.Resources{
			Memory:     4 * 1024 * 1024 * 1024, // 4GB
			MemorySwap: 4 * 1024 * 1024 * 1024, // Disable swap
			NanoCPUs:   2 * 1e9,                // 2 CPUs
		},
		AutoRemove:  false, // We'll handle cleanup manually
		NetworkMode: container.NetworkMode("bridge"),
	}

	// Network configuration
	networkConfig := &network.NetworkingConfig{}

	resp, err := e.client.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, "")
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	e.logger.Debug().Str("container_id", resp.ID).Msg("Container created")
	return resp.ID, nil
}

// startContainer starts the container.
func (e *ContainerExecutor) startContainer(ctx context.Context, containerID string) error {
	if err := e.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	e.logger.Debug().Str("container_id", containerID).Msg("Container started")
	return nil
}

// execInContainer executes a command inside the container.
func (e *ContainerExecutor) execInContainer(ctx context.Context, runID, containerID, command string, reporter ResultReporter) error {
	execConfig := container.ExecOptions{
		Cmd:          []string{"/bin/sh", "-c", command},
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := e.client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec: %w", err)
	}

	attachResp, err := e.client.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer attachResp.Close()

	// Stream output
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.streamContainerOutput(ctx, runID, attachResp.Reader, reporter)
	}()

	wg.Wait()

	// Check exit code
	inspectResp, err := e.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return fmt.Errorf("failed to inspect exec: %w", err)
	}

	if inspectResp.ExitCode != 0 {
		return fmt.Errorf("command exited with code %d", inspectResp.ExitCode)
	}

	return nil
}

// executeTest runs a single test with optional retries inside the container.
func (e *ContainerExecutor) executeTest(ctx context.Context, runID, containerID string, test *conductorv1.TestToRun, reporter ResultReporter) *TestResult {
	maxAttempts := int(test.RetryCount) + 1
	if maxAttempts < 1 {
		maxAttempts = 1
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
			Msg("Executing test in container")

		lastResult = e.runTestInContainer(testCtx, runID, containerID, test, reporter, attempt)

		// If passed or skipped, don't retry
		if lastResult.Status == conductorv1.TestStatus_TEST_STATUS_PASS ||
			lastResult.Status == conductorv1.TestStatus_TEST_STATUS_SKIP {
			break
		}

		// If context cancelled, don't retry
		if testCtx.Err() != nil {
			break
		}
	}

	return lastResult
}

// runTestInContainer executes a single test command inside the container.
func (e *ContainerExecutor) runTestInContainer(ctx context.Context, runID, containerID string, test *conductorv1.TestToRun, reporter ResultReporter, attempt int) *TestResult {
	startTime := time.Now()

	result := &TestResult{
		TestID:       test.TestId,
		TestName:     test.Name,
		Status:       conductorv1.TestStatus_TEST_STATUS_ERROR,
		RetryAttempt: attempt,
		Metadata:     make(map[string]string),
	}

	// Build command with test-specific environment
	var envPrefix string
	for k, v := range test.Environment {
		envPrefix += fmt.Sprintf("export %s=%q; ", k, v)
	}
	command := envPrefix + test.Command

	execConfig := container.ExecOptions{
		Cmd:          []string{"/bin/sh", "-c", command},
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := e.client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to create exec: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	attachResp, err := e.client.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to attach to exec: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	defer attachResp.Close()

	// Stream and capture output
	var outputBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.streamContainerOutputWithCapture(ctx, runID, attachResp.Reader, reporter, &outputBuf)
	}()

	wg.Wait()
	result.Duration = time.Since(startTime)

	// Check exit code
	inspectResp, err := e.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to inspect exec: %v", err)
		return result
	}

	if inspectResp.ExitCode == 0 {
		result.Status = conductorv1.TestStatus_TEST_STATUS_PASS
	} else {
		result.Status = conductorv1.TestStatus_TEST_STATUS_FAIL
		result.ErrorMessage = fmt.Sprintf("exit code %d", inspectResp.ExitCode)
		result.StackTrace = truncateString(outputBuf.String(), 4096)
	}

	return result
}

// streamContainerOutput streams container output to the reporter.
func (e *ContainerExecutor) streamContainerOutput(ctx context.Context, runID string, reader io.Reader, reporter ResultReporter) {
	// Docker multiplexes stdout/stderr in the stream
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		_, err := stdcopy.StdCopy(pw, pw, reader)
		if err != nil && err != io.EOF {
			e.logger.Debug().Err(err).Msg("Error copying container output")
		}
	}()

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := pr.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			if reportErr := reporter.StreamLogs(ctx, runID, conductorv1.LogStream_LOG_STREAM_STDOUT, data); reportErr != nil {
				e.logger.Debug().Err(reportErr).Msg("Failed to stream logs")
			}
		}
		if err != nil {
			return
		}
	}
}

// streamContainerOutputWithCapture streams and captures container output.
func (e *ContainerExecutor) streamContainerOutputWithCapture(ctx context.Context, runID string, reader io.Reader, reporter ResultReporter, capture *strings.Builder) {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		_, err := stdcopy.StdCopy(pw, pw, reader)
		if err != nil && err != io.EOF {
			e.logger.Debug().Err(err).Msg("Error copying container output")
		}
	}()

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := pr.Read(buf)
		if n > 0 {
			data := buf[:n]
			capture.Write(data)

			dataCopy := make([]byte, n)
			copy(dataCopy, data)
			if reportErr := reporter.StreamLogs(ctx, runID, conductorv1.LogStream_LOG_STREAM_STDOUT, dataCopy); reportErr != nil {
				e.logger.Debug().Err(reportErr).Msg("Failed to stream logs")
			}
		}
		if err != nil {
			return
		}
	}
}

// waitContainer waits for the container to stop.
func (e *ContainerExecutor) waitContainer(ctx context.Context, containerID string) (int64, error) {
	statusCh, errCh := e.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	select {
	case err := <-errCh:
		return -1, err
	case status := <-statusCh:
		return status.StatusCode, nil
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

// collectLogs retrieves all logs from the container.
func (e *ContainerExecutor) collectLogs(ctx context.Context, containerID string) (string, error) {
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     false,
	}

	reader, err := e.client.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	defer reader.Close()

	var buf strings.Builder
	_, err = stdcopy.StdCopy(&buf, &buf, reader)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return buf.String(), nil
}

// cleanup removes the container.
func (e *ContainerExecutor) cleanup(ctx context.Context, containerID string) error {
	e.logger.Debug().Str("container_id", containerID).Msg("Cleaning up container")

	// Stop container first
	timeout := 10
	if err := e.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		e.logger.Debug().Err(err).Msg("Failed to stop container")
	}

	// Remove container
	opts := container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	}

	if err := e.client.ContainerRemove(ctx, containerID, opts); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	return nil
}

// Close closes the Docker client.
func (e *ContainerExecutor) Close() error {
	if e.client != nil {
		return e.client.Close()
	}
	return nil
}
