// Package agent provides the Conductor agent implementation.
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/agent/executor"
	"github.com/conductor/conductor/internal/agent/repo"
	"github.com/conductor/conductor/internal/secrets"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Version is the agent software version.
const Version = "0.1.0"

// Agent is the main agent process that connects to the control plane,
// receives work assignments, executes tests, and reports results.
type Agent struct {
	config   *Config
	logger   zerolog.Logger
	client   *Client
	state    *State
	reporter *Reporter
	repoMgr  *repo.Manager
	monitor  *Monitor
	secrets  secrets.Store

	// Executors for different execution types
	subprocessExecutor executor.Executor
	containerExecutor  executor.Executor

	// Active runs tracking
	activeRuns   map[string]*activeRun
	activeRunsMu sync.RWMutex

	// Agent state
	status       atomic.Value // conductorv1.AgentStatus
	draining     atomic.Bool
	shuttingDown atomic.Bool

	// Channels for coordination
	workChan     chan *conductorv1.AssignWork
	cancelChan   chan string // run IDs to cancel
	shutdownChan chan struct{}
	wg           sync.WaitGroup

	// Heartbeat configuration from control plane
	heartbeatInterval time.Duration
}

// activeRun tracks an in-progress test run.
type activeRun struct {
	runID      string
	work       *conductorv1.AssignWork
	startTime  time.Time
	cancelFunc context.CancelFunc
	executor   executor.Executor
}

// New creates a new Agent instance.
func New(cfg *Config) (*Agent, error) {
	// Setup logger
	logger := setupLogger(cfg)

	// Create state manager for persistence
	state, err := NewState(cfg.StateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create state manager: %w", err)
	}

	// Create repository manager
	repoMgr, err := repo.NewManager(cfg.CacheDir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository manager: %w", err)
	}

	// Create resource monitor
	monitor := NewMonitor(cfg, logger)

	// Create client
	client := NewClient(cfg, logger)

	// Create reporter
	reporter := NewReporter(client, logger)

	// Create secrets store if configured
	var secretsStore secrets.Store
	if cfg.SecretsProvider == "vault" {
		secretsStore, err = secrets.NewVaultStore(secrets.VaultConfig{
			Address:   cfg.VaultAddress,
			Token:     cfg.VaultToken,
			Namespace: cfg.VaultNamespace,
			Mount:     cfg.VaultMount,
			Timeout:   cfg.VaultTimeout,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to configure vault secrets: %w", err)
		}
	}

	// Create subprocess executor
	subprocessExec := executor.NewSubprocessExecutor(cfg.WorkspaceDir, logger)

	// Create container executor if Docker is enabled
	var containerExec executor.Executor
	if cfg.DockerEnabled {
		containerExec, err = executor.NewContainerExecutor(cfg.DockerHost, cfg.WorkspaceDir, logger)
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to create container executor, container mode disabled")
			containerExec = nil
		}
	}

	agent := &Agent{
		config:             cfg,
		logger:             logger,
		client:             client,
		state:              state,
		reporter:           reporter,
		repoMgr:            repoMgr,
		monitor:            monitor,
		secrets:            secretsStore,
		subprocessExecutor: subprocessExec,
		containerExecutor:  containerExec,
		activeRuns:         make(map[string]*activeRun),
		workChan:           make(chan *conductorv1.AssignWork, cfg.MaxParallel),
		cancelChan:         make(chan string, cfg.MaxParallel),
		shutdownChan:       make(chan struct{}),
		heartbeatInterval:  cfg.HeartbeatInterval,
	}

	agent.status.Store(conductorv1.AgentStatus_AGENT_STATUS_IDLE)

	return agent, nil
}

// Start connects to the control plane and begins processing work.
func (a *Agent) Start(ctx context.Context) error {
	a.logger.Info().
		Str("agent_id", a.config.AgentID).
		Str("version", Version).
		Str("control_plane", a.config.ControlPlaneURL).
		Msg("Starting agent")

	// Ensure directories exist
	if err := a.ensureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Start resource monitor
	a.wg.Add(1)
	go a.resourceMonitorLoop(ctx)

	// Recover any pending runs from previous session
	if err := a.recoverPendingRuns(ctx); err != nil {
		a.logger.Warn().Err(err).Msg("Failed to recover pending runs")
	}

	// Start work processor workers
	for i := 0; i < a.config.MaxParallel; i++ {
		a.wg.Add(1)
		go a.workProcessor(ctx, i)
	}

	// Start cancel processor
	a.wg.Add(1)
	go a.cancelProcessor(ctx)

	// Main connection loop with reconnection
	return a.connectionLoop(ctx)
}

// Stop gracefully shuts down the agent.
func (a *Agent) Stop(ctx context.Context) error {
	a.logger.Info().Msg("Stopping agent")
	a.shuttingDown.Store(true)

	// Signal shutdown
	close(a.shutdownChan)

	// Cancel all active runs
	a.activeRunsMu.Lock()
	for runID, run := range a.activeRuns {
		a.logger.Info().Str("run_id", runID).Msg("Cancelling active run for shutdown")
		run.cancelFunc()
	}
	a.activeRunsMu.Unlock()

	// Wait for workers with timeout
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		a.logger.Info().Msg("All workers stopped")
	case <-ctx.Done():
		a.logger.Warn().Msg("Shutdown timeout, forcing exit")
	}

	// Close resources
	if err := a.state.Close(); err != nil {
		a.logger.Warn().Err(err).Msg("Error closing state")
	}

	if err := a.client.Close(); err != nil {
		a.logger.Warn().Err(err).Msg("Error closing client")
	}

	return nil
}

// connectionLoop maintains the connection to the control plane.
func (a *Agent) connectionLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.shutdownChan:
			return nil
		default:
		}

		// Connect to control plane
		if err := a.client.Connect(ctx); err != nil {
			a.logger.Error().Err(err).Msg("Failed to connect to control plane")
			a.waitForReconnect(ctx)
			continue
		}

		// Open work stream
		stream, err := a.client.WorkStream(ctx)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to open work stream")
			a.waitForReconnect(ctx)
			continue
		}

		// Register with control plane
		if err := a.register(stream); err != nil {
			a.logger.Error().Err(err).Msg("Failed to register with control plane")
			a.waitForReconnect(ctx)
			continue
		}

		// Start message handler
		if err := a.messageLoop(ctx, stream); err != nil {
			if !errors.Is(err, context.Canceled) {
				a.logger.Error().Err(err).Msg("Message loop error, reconnecting")
			}
		}

		// Check if we should exit
		if a.shuttingDown.Load() {
			return nil
		}

		a.waitForReconnect(ctx)
	}
}

// register sends the registration message to the control plane.
func (a *Agent) register(stream *WorkStream) error {
	capabilities := &conductorv1.Capabilities{
		NetworkZones:    a.config.NetworkZones,
		Runtimes:        a.config.Runtimes,
		MaxParallel:     int32(a.config.MaxParallel),
		DockerAvailable: a.containerExecutor != nil,
		Resources:       a.monitor.GetResources(),
		Os:              runtime.GOOS,
		Arch:            runtime.GOARCH,
	}

	msg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_Register{
			Register: &conductorv1.RegisterRequest{
				AgentId:      a.config.AgentID,
				Name:         a.config.AgentName,
				Version:      Version,
				Capabilities: capabilities,
				Labels:       a.config.Labels,
			},
		},
	}

	if err := stream.Send(msg); err != nil {
		return fmt.Errorf("failed to send register message: %w", err)
	}

	// Wait for register response
	resp, err := stream.Receive()
	if err != nil {
		return fmt.Errorf("failed to receive register response: %w", err)
	}

	registerResp := resp.GetRegisterResponse()
	if registerResp == nil {
		return errors.New("expected register response, got different message type")
	}

	if !registerResp.Success {
		return fmt.Errorf("registration failed: %s", registerResp.ErrorMessage)
	}

	// Update heartbeat interval if provided
	if registerResp.HeartbeatIntervalSeconds > 0 {
		a.heartbeatInterval = time.Duration(registerResp.HeartbeatIntervalSeconds) * time.Second
	}

	a.logger.Info().
		Str("server_version", registerResp.ServerVersion).
		Dur("heartbeat_interval", a.heartbeatInterval).
		Msg("Registered with control plane")

	return nil
}

// messageLoop handles incoming messages from the control plane.
func (a *Agent) messageLoop(ctx context.Context, stream *WorkStream) error {
	// Start heartbeat goroutine
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()

	a.wg.Add(1)
	go a.heartbeatLoop(heartbeatCtx, stream)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.shutdownChan:
			return nil
		default:
		}

		msg, err := stream.Receive()
		if err != nil {
			return fmt.Errorf("receive error: %w", err)
		}

		if err := a.handleControlMessage(msg); err != nil {
			a.logger.Error().Err(err).Msg("Error handling control message")
		}
	}
}

// handleControlMessage processes a message from the control plane.
func (a *Agent) handleControlMessage(msg *conductorv1.ControlMessage) error {
	switch m := msg.Message.(type) {
	case *conductorv1.ControlMessage_AssignWork:
		return a.handleAssignWork(m.AssignWork)
	case *conductorv1.ControlMessage_CancelWork:
		return a.handleCancelWork(m.CancelWork)
	case *conductorv1.ControlMessage_Drain:
		return a.handleDrain(m.Drain)
	case *conductorv1.ControlMessage_Ack:
		a.logger.Debug().Str("id", m.Ack.Id).Bool("success", m.Ack.Success).Msg("Received ack")
	default:
		a.logger.Warn().Msg("Received unknown message type")
	}
	return nil
}

// handleAssignWork processes a work assignment.
func (a *Agent) handleAssignWork(work *conductorv1.AssignWork) error {
	a.logger.Info().
		Str("run_id", work.RunId).
		Str("execution_type", work.ExecutionType.String()).
		Int("test_count", len(work.Tests)).
		Msg("Received work assignment")

	// Check if draining
	if a.draining.Load() {
		return a.rejectWork(work.RunId, "agent is draining", true)
	}

	// Check if we can accept more work
	if !a.canAcceptWork() {
		return a.rejectWork(work.RunId, "resource limits exceeded", true)
	}

	// Check if we have the right executor
	if work.ExecutionType == conductorv1.ExecutionType_EXECUTION_TYPE_CONTAINER && a.containerExecutor == nil {
		return a.rejectWork(work.RunId, "container execution not available", false)
	}

	// Accept the work
	select {
	case a.workChan <- work:
		return a.acceptWork(work.RunId)
	default:
		return a.rejectWork(work.RunId, "work queue full", true)
	}
}

// handleCancelWork processes a cancellation request.
func (a *Agent) handleCancelWork(cancel *conductorv1.CancelWork) error {
	a.logger.Info().
		Str("run_id", cancel.RunId).
		Str("reason", cancel.Reason).
		Msg("Received cancel request")

	select {
	case a.cancelChan <- cancel.RunId:
	default:
		a.logger.Warn().Str("run_id", cancel.RunId).Msg("Cancel channel full")
	}

	return nil
}

// handleDrain processes a drain request.
func (a *Agent) handleDrain(drain *conductorv1.Drain) error {
	a.logger.Info().
		Str("reason", drain.Reason).
		Bool("cancel_active", drain.CancelActive).
		Msg("Received drain request")

	a.draining.Store(true)
	a.status.Store(conductorv1.AgentStatus_AGENT_STATUS_DRAINING)

	if drain.CancelActive {
		a.activeRunsMu.Lock()
		for runID, run := range a.activeRuns {
			a.logger.Info().Str("run_id", runID).Msg("Cancelling run due to drain")
			run.cancelFunc()
		}
		a.activeRunsMu.Unlock()
	}

	return nil
}

// acceptWork sends a work accepted message.
func (a *Agent) acceptWork(runID string) error {
	msg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_WorkAccepted{
			WorkAccepted: &conductorv1.WorkAccepted{
				RunId: runID,
			},
		},
	}
	return a.client.Send(msg)
}

// rejectWork sends a work rejected message.
func (a *Agent) rejectWork(runID, reason string, temporary bool) error {
	a.logger.Info().
		Str("run_id", runID).
		Str("reason", reason).
		Bool("temporary", temporary).
		Msg("Rejecting work")

	msg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_WorkRejected{
			WorkRejected: &conductorv1.WorkRejected{
				RunId:     runID,
				Reason:    reason,
				Temporary: temporary,
			},
		},
	}
	return a.client.Send(msg)
}

// workProcessor processes work assignments from the work channel.
func (a *Agent) workProcessor(ctx context.Context, workerID int) {
	defer a.wg.Done()

	logger := a.logger.With().Int("worker_id", workerID).Logger()
	logger.Debug().Msg("Work processor started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.shutdownChan:
			return
		case work := <-a.workChan:
			a.processWork(ctx, work, logger)
		}
	}
}

// processWork executes a single work assignment.
func (a *Agent) processWork(ctx context.Context, work *conductorv1.AssignWork, logger zerolog.Logger) {
	runID := work.RunId
	logger = logger.With().Str("run_id", runID).Logger()
	logger.Info().Msg("Starting work execution")

	// Create cancellable context
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Apply timeout if specified
	if work.Timeout != nil && work.Timeout.Seconds > 0 {
		timeout := time.Duration(work.Timeout.Seconds)*time.Second + time.Duration(work.Timeout.Nanos)*time.Nanosecond
		var timeoutCancel context.CancelFunc
		runCtx, timeoutCancel = context.WithTimeout(runCtx, timeout)
		defer timeoutCancel()
	}

	// Select executor
	exec := a.selectExecutor(work.ExecutionType)
	if exec == nil {
		logger.Error().Msg("No executor available for execution type")
		a.reporter.ReportComplete(ctx, runID, conductorv1.RunStatus_RUN_STATUS_ERROR, "no executor available")
		return
	}

	// Track active run
	run := &activeRun{
		runID:      runID,
		work:       work,
		startTime:  time.Now(),
		cancelFunc: cancel,
		executor:   exec,
	}

	a.activeRunsMu.Lock()
	a.activeRuns[runID] = run
	a.activeRunsMu.Unlock()
	a.updateStatus()

	// Save state for recovery
	if err := a.state.SaveRunState(runID, "running", work); err != nil {
		logger.Warn().Err(err).Msg("Failed to save run state")
	}

	defer func() {
		a.activeRunsMu.Lock()
		delete(a.activeRuns, runID)
		a.activeRunsMu.Unlock()
		a.updateStatus()

		// Clear persisted state
		if err := a.state.DeleteRunState(runID); err != nil {
			logger.Warn().Err(err).Msg("Failed to delete run state")
		}
	}()

	// Clone repository
	repoPath, err := a.cloneRepository(runCtx, work, logger)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to clone repository")
		a.reporter.ReportComplete(ctx, runID, conductorv1.RunStatus_RUN_STATUS_ERROR, fmt.Sprintf("clone failed: %v", err))
		return
	}

	// Report progress
	a.reporter.ReportProgress(ctx, runID, "setup", "Repository cloned", 10, 0, len(work.Tests))

	resolvedEnv := make(map[string]string, len(work.Environment))
	for key, value := range work.Environment {
		resolvedEnv[key] = value
	}

	if len(work.Secrets) > 0 {
		secretValues, err := a.resolveSecrets(runCtx, work.Secrets)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to resolve secrets")
			a.reporter.ReportComplete(ctx, runID, conductorv1.RunStatus_RUN_STATUS_ERROR, err.Error())
			return
		}
		for key, value := range secretValues {
			resolvedEnv[key] = value
		}
	}

	// Prepare execution request
	execReq := &executor.ExecutionRequest{
		RunID:            runID,
		WorkDir:          repoPath,
		WorkingDirectory: work.WorkingDirectory,
		Tests:            work.Tests,
		Environment:      resolvedEnv,
		SetupCommands:    work.SetupCommands,
		TeardownCommands: work.TeardownCommands,
		ContainerImage:   work.ContainerImage,
		Timeout:          a.config.DefaultTimeout,
	}

	// Apply work timeout if specified
	if work.Timeout != nil && work.Timeout.Seconds > 0 {
		execReq.Timeout = time.Duration(work.Timeout.Seconds) * time.Second
	}

	// Execute tests
	result, err := exec.Execute(runCtx, execReq, a.reporter)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info().Msg("Run was cancelled")
			a.reporter.ReportComplete(ctx, runID, conductorv1.RunStatus_RUN_STATUS_CANCELLED, "cancelled")
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			logger.Warn().Msg("Run timed out")
			a.reporter.ReportComplete(ctx, runID, conductorv1.RunStatus_RUN_STATUS_TIMEOUT, "timeout exceeded")
			return
		}
		logger.Error().Err(err).Msg("Execution failed")
		a.reporter.ReportComplete(ctx, runID, conductorv1.RunStatus_RUN_STATUS_ERROR, err.Error())
		return
	}

	// Upload artifacts
	for _, artifactPath := range a.collectArtifacts(repoPath, work.Tests) {
		if err := a.reporter.UploadArtifact(ctx, runID, artifactPath); err != nil {
			logger.Warn().Err(err).Str("path", artifactPath).Msg("Failed to upload artifact")
		}
	}

	// Report completion
	finalStatus := a.determineFinalStatus(result)
	logger.Info().
		Str("status", finalStatus.String()).
		Int("total", result.Summary.Total).
		Int("passed", result.Summary.Passed).
		Int("failed", result.Summary.Failed).
		Msg("Run completed")

	a.reporter.ReportRunComplete(ctx, runID, result)
}

// cloneRepository clones the repository for the work assignment.
func (a *Agent) cloneRepository(ctx context.Context, work *conductorv1.AssignWork, logger zerolog.Logger) (string, error) {
	if work.GitRef == nil {
		return "", errors.New("git ref is required")
	}

	// Create unique workspace directory
	workspaceID := fmt.Sprintf("%s-%s", work.RunId, uuid.New().String()[:8])
	workspacePath := fmt.Sprintf("%s/%s", a.config.WorkspaceDir, workspaceID)

	// Clone with caching
	opts := &repo.CloneOptions{
		URL:       work.GitRef.RepositoryUrl,
		Branch:    work.GitRef.Branch,
		CommitSHA: work.GitRef.CommitSha,
		Depth:     1,
	}

	if err := a.repoMgr.Clone(ctx, opts, workspacePath); err != nil {
		return "", err
	}

	return workspacePath, nil
}

// selectExecutor returns the appropriate executor for the execution type.
func (a *Agent) selectExecutor(execType conductorv1.ExecutionType) executor.Executor {
	switch execType {
	case conductorv1.ExecutionType_EXECUTION_TYPE_CONTAINER:
		if a.containerExecutor != nil {
			return a.containerExecutor
		}
		// Fall back to subprocess
		return a.subprocessExecutor
	default:
		return a.subprocessExecutor
	}
}

func (a *Agent) resolveSecrets(ctx context.Context, refs []*conductorv1.Secret) (map[string]string, error) {
	if a.secrets == nil {
		return nil, errors.New("secrets provider is not configured")
	}

	values := make(map[string]string, len(refs))
	for _, ref := range refs {
		if ref == nil {
			continue
		}

		provider := ref.Provider.String()
		if ref.Provider == conductorv1.SecretProvider_SECRET_PROVIDER_UNSPECIFIED {
			provider = "vault"
		}
		if provider != "vault" {
			return nil, fmt.Errorf("unsupported secret provider: %s", provider)
		}

		value, err := a.secrets.Resolve(ctx, secrets.Reference{
			Name:     ref.Name,
			Provider: secrets.ProviderVault,
			Path:     ref.Path,
			Key:      ref.Key,
			Version:  int(ref.Version),
		})
		if err != nil {
			return nil, fmt.Errorf("resolve secret %s: %w", ref.Name, err)
		}
		values[ref.Name] = value
	}

	return values, nil
}

// collectArtifacts collects artifact paths from the workspace.
func (a *Agent) collectArtifacts(workspacePath string, tests []*conductorv1.TestToRun) []string {
	var artifacts []string
	for _, test := range tests {
		for _, pattern := range test.ArtifactPaths {
			// Glob for matching files
			matches, err := a.repoMgr.Glob(workspacePath, pattern)
			if err != nil {
				a.logger.Debug().Err(err).Str("pattern", pattern).Msg("Artifact glob failed")
				continue
			}
			artifacts = append(artifacts, matches...)
		}
	}
	return artifacts
}

// determineFinalStatus determines the final run status from execution results.
func (a *Agent) determineFinalStatus(result *executor.ExecutionResult) conductorv1.RunStatus {
	if result.Error != "" {
		return conductorv1.RunStatus_RUN_STATUS_ERROR
	}
	if result.Summary.Failed > 0 || result.Summary.Errored > 0 {
		return conductorv1.RunStatus_RUN_STATUS_FAILED
	}
	return conductorv1.RunStatus_RUN_STATUS_PASSED
}

// cancelProcessor processes cancellation requests.
func (a *Agent) cancelProcessor(ctx context.Context) {
	defer a.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.shutdownChan:
			return
		case runID := <-a.cancelChan:
			a.activeRunsMu.RLock()
			run, ok := a.activeRuns[runID]
			a.activeRunsMu.RUnlock()

			if ok {
				a.logger.Info().Str("run_id", runID).Msg("Cancelling run")
				run.cancelFunc()
			} else {
				a.logger.Debug().Str("run_id", runID).Msg("Run not found for cancellation")
			}
		}
	}
}

// heartbeatLoop sends periodic heartbeats to the control plane.
func (a *Agent) heartbeatLoop(ctx context.Context, stream *WorkStream) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.shutdownChan:
			return
		case <-ticker.C:
			if err := a.sendHeartbeat(stream); err != nil {
				a.logger.Error().Err(err).Msg("Failed to send heartbeat")
				return
			}
		}
	}
}

// sendHeartbeat sends a heartbeat message to the control plane.
func (a *Agent) sendHeartbeat(stream *WorkStream) error {
	a.activeRunsMu.RLock()
	activeRunIDs := make([]string, 0, len(a.activeRuns))
	for runID := range a.activeRuns {
		activeRunIDs = append(activeRunIDs, runID)
	}
	a.activeRunsMu.RUnlock()

	msg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_Heartbeat{
			Heartbeat: &conductorv1.Heartbeat{
				Status:        a.status.Load().(conductorv1.AgentStatus),
				ActiveRunIds:  activeRunIDs,
				ResourceUsage: a.monitor.GetUsage(),
			},
		},
	}

	return stream.Send(msg)
}

// resourceMonitorLoop periodically updates resource usage.
func (a *Agent) resourceMonitorLoop(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.ResourceCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.shutdownChan:
			return
		case <-ticker.C:
			a.monitor.Update()
		}
	}
}

// canAcceptWork checks if the agent can accept more work.
func (a *Agent) canAcceptWork() bool {
	a.activeRunsMu.RLock()
	activeCount := len(a.activeRuns)
	a.activeRunsMu.RUnlock()

	if activeCount >= a.config.MaxParallel {
		return false
	}

	return a.monitor.CanAcceptWork()
}

// updateStatus updates the agent status based on active runs.
func (a *Agent) updateStatus() {
	if a.draining.Load() {
		a.status.Store(conductorv1.AgentStatus_AGENT_STATUS_DRAINING)
		return
	}

	a.activeRunsMu.RLock()
	activeCount := len(a.activeRuns)
	a.activeRunsMu.RUnlock()

	if activeCount > 0 {
		a.status.Store(conductorv1.AgentStatus_AGENT_STATUS_BUSY)
	} else {
		a.status.Store(conductorv1.AgentStatus_AGENT_STATUS_IDLE)
	}
}

// recoverPendingRuns attempts to recover runs from a previous session.
func (a *Agent) recoverPendingRuns(ctx context.Context) error {
	runs, err := a.state.GetPendingRuns()
	if err != nil {
		return err
	}

	for _, run := range runs {
		a.logger.Info().Str("run_id", run.RunID).Msg("Found pending run from previous session")
		// Mark as error since we can't resume
		a.reporter.ReportComplete(ctx, run.RunID, conductorv1.RunStatus_RUN_STATUS_ERROR, "agent restarted during execution")
		if err := a.state.DeleteRunState(run.RunID); err != nil {
			a.logger.Warn().Err(err).Str("run_id", run.RunID).Msg("Failed to delete recovered run state")
		}
	}

	return nil
}

// waitForReconnect waits before attempting to reconnect.
func (a *Agent) waitForReconnect(ctx context.Context) {
	interval := a.client.NextReconnectInterval()
	a.logger.Info().Dur("interval", interval).Msg("Waiting before reconnect")

	select {
	case <-ctx.Done():
	case <-a.shutdownChan:
	case <-time.After(interval):
	}
}

// ensureDirectories creates required directories.
func (a *Agent) ensureDirectories() error {
	dirs := []string{
		a.config.WorkspaceDir,
		a.config.CacheDir,
		a.config.StateDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// setupLogger creates the logger based on configuration.
func setupLogger(cfg *Config) zerolog.Logger {
	var logger zerolog.Logger

	if cfg.LogFormat == "console" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
			With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}

	switch cfg.LogLevel {
	case "debug":
		logger = logger.Level(zerolog.DebugLevel)
	case "info":
		logger = logger.Level(zerolog.InfoLevel)
	case "warn":
		logger = logger.Level(zerolog.WarnLevel)
	case "error":
		logger = logger.Level(zerolog.ErrorLevel)
	default:
		logger = logger.Level(zerolog.InfoLevel)
	}

	return logger.With().Str("component", "agent").Logger()
}
