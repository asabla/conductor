//go:build integration

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/database"
)

// ============================================================================
// AGENT REGISTRATION AND CONNECTION TESTS
// ============================================================================

func TestE2E_AgentRegistrationFlow(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a new gRPC connection for this test
	conn, err := grpc.NewClient(
		testEnv.GRPCAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := conductorv1.NewAgentServiceClient(conn)

	// Start work stream
	stream, err := client.WorkStream(ctx)
	require.NoError(t, err)

	agentID := uuid.New()
	agentName := fmt.Sprintf("test-agent-%s", agentID.String()[:8])

	// Send registration message
	registerMsg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_Register{
			Register: &conductorv1.RegisterRequest{
				AgentId: agentID.String(),
				Name:    agentName,
				Version: "1.0.0-test",
				Capabilities: &conductorv1.Capabilities{
					NetworkZones:    []string{"default"},
					MaxParallel:     2,
					DockerAvailable: true,
				},
				Labels: map[string]string{
					"env":  "test",
					"tier": "integration",
				},
			},
		},
	}

	err = stream.Send(registerMsg)
	require.NoError(t, err)

	// Receive registration response
	resp, err := stream.Recv()
	require.NoError(t, err)

	regResp := resp.GetRegisterResponse()
	require.NotNil(t, regResp, "expected RegisterResponse")
	assert.True(t, regResp.Success, "registration should succeed")
	assert.NotEmpty(t, regResp.ServerVersion, "server version should be provided")
	assert.Greater(t, regResp.HeartbeatIntervalSeconds, int32(0), "heartbeat interval should be positive")

	// Verify agent was stored
	storedAgent, err := testEnv.AgentRepo.Get(ctx, agentID)
	require.NoError(t, err)
	assert.Equal(t, agentName, storedAgent.Name)
	assert.Equal(t, database.AgentStatusIdle, storedAgent.Status)

	// Send heartbeat
	heartbeatMsg := &conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_Heartbeat{
			Heartbeat: &conductorv1.Heartbeat{
				Status:       conductorv1.AgentStatus_AGENT_STATUS_IDLE,
				ActiveRunIds: []string{},
			},
		},
	}

	err = stream.Send(heartbeatMsg)
	require.NoError(t, err)

	// Close the stream gracefully
	err = stream.CloseSend()
	require.NoError(t, err)
}

func TestE2E_MultipleAgentsConcurrent(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const numAgents = 3
	var wg sync.WaitGroup
	errors := make(chan error, numAgents)

	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(agentNum int) {
			defer wg.Done()

			// Create a new gRPC connection
			conn, err := grpc.NewClient(
				testEnv.GRPCAddress,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				errors <- fmt.Errorf("agent %d: failed to create connection: %w", agentNum, err)
				return
			}
			defer conn.Close()

			client := conductorv1.NewAgentServiceClient(conn)

			stream, err := client.WorkStream(ctx)
			if err != nil {
				errors <- fmt.Errorf("agent %d: failed to create stream: %w", agentNum, err)
				return
			}

			agentID := uuid.New()

			// Register
			err = stream.Send(&conductorv1.AgentMessage{
				Message: &conductorv1.AgentMessage_Register{
					Register: &conductorv1.RegisterRequest{
						AgentId: agentID.String(),
						Name:    fmt.Sprintf("concurrent-agent-%d", agentNum),
						Version: "1.0.0",
						Capabilities: &conductorv1.Capabilities{
							NetworkZones: []string{"default"},
							MaxParallel:  1,
						},
					},
				},
			})
			if err != nil {
				errors <- fmt.Errorf("agent %d: failed to send register: %w", agentNum, err)
				return
			}

			resp, err := stream.Recv()
			if err != nil {
				errors <- fmt.Errorf("agent %d: failed to receive response: %w", agentNum, err)
				return
			}

			regResp := resp.GetRegisterResponse()
			if regResp == nil || !regResp.Success {
				errors <- fmt.Errorf("agent %d: registration failed: %v", agentNum, regResp.GetErrorMessage())
				return
			}

			// Send a few heartbeats
			for j := 0; j < 3; j++ {
				err = stream.Send(&conductorv1.AgentMessage{
					Message: &conductorv1.AgentMessage_Heartbeat{
						Heartbeat: &conductorv1.Heartbeat{
							Status: conductorv1.AgentStatus_AGENT_STATUS_IDLE,
						},
					},
				})
				if err != nil {
					// Heartbeat send might fail if server closed, that's okay
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			stream.CloseSend()
		}(i)
	}

	// Wait for all agents to complete
	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}
}

// ============================================================================
// SERVICE MANAGEMENT TESTS
// ============================================================================

func TestE2E_ServiceLifecycle(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a service
	serviceName := fmt.Sprintf("test-service-%s", uuid.New().String()[:8])
	service := &database.Service{
		Name:          serviceName,
		GitURL:        "https://github.com/test/repo",
		DefaultBranch: "main",
		NetworkZones:  []string{"default"},
		Owner:         Ptr("test-owner"),
		ContactEmail:  Ptr("test@example.com"),
	}

	err := testEnv.ServiceRepo.Create(ctx, service)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, service.ID, "service ID should be assigned")

	// Verify we can retrieve it
	retrieved, err := testEnv.ServiceRepo.Get(ctx, service.ID)
	require.NoError(t, err)
	assert.Equal(t, serviceName, retrieved.Name)
	assert.Equal(t, "https://github.com/test/repo", retrieved.GitURL)

	// Update the service
	retrieved.ContactSlack = Ptr("#test-channel")
	err = testEnv.ServiceRepo.Update(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	updated, err := testEnv.ServiceRepo.Get(ctx, service.ID)
	require.NoError(t, err)
	assert.Equal(t, "#test-channel", *updated.ContactSlack)

	// Delete the service
	err = testEnv.ServiceRepo.Delete(ctx, service.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = testEnv.ServiceRepo.Get(ctx, service.ID)
	assert.Error(t, err)
}

// ============================================================================
// TEST RUN CREATION AND EXECUTION TESTS
// ============================================================================

func TestE2E_CreateTestRun(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First, create a service
	service, err := testEnv.CreateTestService(ctx, fmt.Sprintf("run-test-service-%s", uuid.New().String()[:8]))
	require.NoError(t, err)

	// Create a test run
	triggerType := database.TriggerTypeManual
	run := &database.TestRun{
		ServiceID:   service.ID,
		Status:      database.RunStatusPending,
		GitRef:      Ptr("main"),
		GitSHA:      Ptr("abc123def456"),
		TriggerType: &triggerType,
		TriggeredBy: Ptr("test-user"),
		Priority:    5,
	}

	err = testEnv.RunRepo.Create(ctx, run)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, run.ID)

	// Verify the run was created
	retrieved, err := testEnv.RunRepo.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, database.RunStatusPending, retrieved.Status)
	assert.Equal(t, service.ID, retrieved.ServiceID)
	assert.Equal(t, "main", *retrieved.GitRef)

	// Update run status
	err = testEnv.RunRepo.UpdateStatus(ctx, run.ID, database.RunStatusRunning)
	require.NoError(t, err)

	// Verify status update
	updated, err := testEnv.RunRepo.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, database.RunStatusRunning, updated.Status)

	// Clean up
	testEnv.ServiceRepo.Delete(ctx, service.ID)
}

func TestE2E_TestRunWithResults(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create service and run
	service, err := testEnv.CreateTestService(ctx, fmt.Sprintf("results-test-%s", uuid.New().String()[:8]))
	require.NoError(t, err)
	defer testEnv.ServiceRepo.Delete(ctx, service.ID)

	triggerType := database.TriggerTypeManual
	run := &database.TestRun{
		ServiceID:   service.ID,
		Status:      database.RunStatusRunning,
		TriggerType: &triggerType,
		Priority:    5,
	}
	err = testEnv.RunRepo.Create(ctx, run)
	require.NoError(t, err)

	// Add test results
	results := []database.TestResult{
		{
			RunID:      run.ID,
			TestName:   "TestAddition",
			SuiteName:  Ptr("math"),
			Status:     database.ResultStatusPass,
			DurationMs: Ptr(int64(150)),
		},
		{
			RunID:        run.ID,
			TestName:     "TestDivision",
			SuiteName:    Ptr("math"),
			Status:       database.ResultStatusFail,
			DurationMs:   Ptr(int64(200)),
			ErrorMessage: Ptr("division by zero"),
			StackTrace:   Ptr("at TestDivision (math_test.go:45)"),
		},
		{
			RunID:      run.ID,
			TestName:   "TestSkipped",
			SuiteName:  Ptr("misc"),
			Status:     database.ResultStatusSkip,
			DurationMs: Ptr(int64(0)),
		},
	}

	err = testEnv.ResultRepo.BatchCreate(ctx, results)
	require.NoError(t, err)

	// Retrieve results
	storedResults, err := testEnv.ResultRepo.ListByRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Len(t, storedResults, 3)

	// Count by status
	counts, err := testEnv.ResultRepo.CountByRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), counts[database.ResultStatusPass])
	assert.Equal(t, int64(1), counts[database.ResultStatusFail])
	assert.Equal(t, int64(1), counts[database.ResultStatusSkip])

	// Get failed results only
	failedResults, err := testEnv.ResultRepo.ListByRunAndStatus(ctx, run.ID, database.ResultStatusFail)
	require.NoError(t, err)
	assert.Len(t, failedResults, 1)
	assert.Equal(t, "TestDivision", failedResults[0].TestName)

	// Finish the run
	err = testEnv.RunRepo.Finish(ctx, run.ID, database.RunStatusFailed, database.RunResults{
		TotalTests:   3,
		PassedTests:  1,
		FailedTests:  1,
		SkippedTests: 1,
		DurationMs:   350,
	})
	require.NoError(t, err)

	// Verify final state
	finalRun, err := testEnv.RunRepo.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, database.RunStatusFailed, finalRun.Status)
	assert.True(t, finalRun.IsTerminal())
	assert.Equal(t, 3, finalRun.TotalTests)
	assert.Equal(t, 1, finalRun.FailedTests)
}

// ============================================================================
// ARTIFACT STORAGE TESTS
// ============================================================================

func TestE2E_ArtifactStorage(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runID := uuid.New()
	artifactName := "coverage.html"
	artifactContent := []byte("<html><body>Coverage Report: 85%</body></html>")

	// Upload artifact
	artifactPath, err := testEnv.Storage.Upload(ctx, runID, artifactName, bytes.NewReader(artifactContent))
	require.NoError(t, err)
	assert.NotEmpty(t, artifactPath)

	// Download and verify
	reader, err := testEnv.Storage.Download(ctx, artifactPath)
	require.NoError(t, err)
	downloaded, err := io.ReadAll(reader)
	reader.Close()
	require.NoError(t, err)
	assert.Equal(t, artifactContent, downloaded)

	// Check existence via GetMetadata
	metadata, err := testEnv.Storage.GetMetadata(ctx, artifactPath)
	require.NoError(t, err)
	assert.NotNil(t, metadata)

	// List artifacts
	artifacts, err := testEnv.Storage.List(ctx, runID)
	require.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, artifactPath, artifacts[0].Path)

	// Get presigned URL
	url, err := testEnv.Storage.GetPresignedURL(ctx, artifactPath, 5*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, url)
	assert.Contains(t, url, "coverage.html")

	// Delete artifact
	err = testEnv.Storage.Delete(ctx, artifactPath)
	require.NoError(t, err)

	// Verify deletion - GetMetadata should return error
	_, err = testEnv.Storage.GetMetadata(ctx, artifactPath)
	assert.Error(t, err)
}

func TestE2E_MultipleArtifacts(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runID := uuid.New()

	// Upload multiple artifacts for the same run
	artifactNames := []string{
		"screenshot-1.png",
		"screenshot-2.png",
		"logs/stdout.log",
		"logs/stderr.log",
		"reports/junit.xml",
		"reports/coverage.xml",
	}

	var uploadedPaths []string
	for _, name := range artifactNames {
		content := []byte(fmt.Sprintf("Content for %s", name))
		path, err := testEnv.Storage.Upload(ctx, runID, name, bytes.NewReader(content))
		require.NoError(t, err)
		uploadedPaths = append(uploadedPaths, path)
	}

	// List all artifacts for the run
	listed, err := testEnv.Storage.List(ctx, runID)
	require.NoError(t, err)
	assert.Len(t, listed, 6)

	// Delete all artifacts for this run
	err = testEnv.Storage.DeleteByRun(ctx, runID)
	require.NoError(t, err)

	// Verify all deleted
	remaining, err := testEnv.Storage.List(ctx, runID)
	require.NoError(t, err)
	assert.Len(t, remaining, 0)
}

// ============================================================================
// NOTIFICATION TESTS
// ============================================================================

func TestE2E_NotificationChannelManagement(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a webhook notification channel
	channel := &database.NotificationChannel{
		Name:    fmt.Sprintf("test-webhook-%s", uuid.New().String()[:8]),
		Type:    database.ChannelTypeWebhook,
		Config:  []byte(`{"url": "https://example.com/webhook", "method": "POST"}`),
		Enabled: true,
	}

	err := testEnv.NotificationRepo.CreateChannel(ctx, channel)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, channel.ID)

	// Retrieve the channel
	retrieved, err := testEnv.NotificationRepo.GetChannel(ctx, channel.ID)
	require.NoError(t, err)
	assert.Equal(t, channel.Name, retrieved.Name)
	assert.Equal(t, database.ChannelTypeWebhook, retrieved.Type)
	assert.True(t, retrieved.Enabled)

	// Create a notification rule
	rule := &database.NotificationRule{
		ChannelID: channel.ID,
		ServiceID: nil, // Global rule
		TriggerOn: []database.TriggerEvent{database.TriggerEventFailure, database.TriggerEventRecovery},
		Enabled:   true,
	}

	err = testEnv.NotificationRepo.CreateRule(ctx, rule)
	require.NoError(t, err)

	// List enabled channels
	enabledChannels, err := testEnv.NotificationRepo.ListEnabledChannels(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(enabledChannels), 1)

	// Disable the channel
	channel.Enabled = false
	err = testEnv.NotificationRepo.UpdateChannel(ctx, channel)
	require.NoError(t, err)

	// Clean up
	testEnv.NotificationRepo.DeleteRule(ctx, rule.ID)
	testEnv.NotificationRepo.DeleteChannel(ctx, channel.ID)
}

// ============================================================================
// FULL FLOW TESTS
// ============================================================================

func TestE2E_FullTestExecutionFlow(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 1. Create a service
	service, err := testEnv.CreateTestService(ctx, fmt.Sprintf("full-flow-%s", uuid.New().String()[:8]))
	require.NoError(t, err)
	defer testEnv.ServiceRepo.Delete(ctx, service.ID)

	t.Logf("Created service: %s (ID: %s)", service.Name, service.ID)

	// 2. Create a test run
	triggerType := database.TriggerTypeManual
	run := &database.TestRun{
		ServiceID:   service.ID,
		Status:      database.RunStatusPending,
		GitRef:      Ptr("main"),
		GitSHA:      Ptr("abc123"),
		TriggerType: &triggerType,
		TriggeredBy: Ptr("e2e-test"),
		Priority:    10,
	}
	err = testEnv.RunRepo.Create(ctx, run)
	require.NoError(t, err)

	t.Logf("Created run: %s", run.ID)

	// 3. Start an agent and register it
	conn, err := grpc.NewClient(
		testEnv.GRPCAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	agentClient := conductorv1.NewAgentServiceClient(conn)
	stream, err := agentClient.WorkStream(ctx)
	require.NoError(t, err)

	agentID := uuid.New()
	err = stream.Send(&conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_Register{
			Register: &conductorv1.RegisterRequest{
				AgentId: agentID.String(),
				Name:    "e2e-test-agent",
				Version: "1.0.0",
				Capabilities: &conductorv1.Capabilities{
					NetworkZones:    []string{"default"},
					MaxParallel:     2,
					DockerAvailable: true,
				},
			},
		},
	})
	require.NoError(t, err)

	// Wait for registration response
	resp, err := stream.Recv()
	require.NoError(t, err)
	require.True(t, resp.GetRegisterResponse().GetSuccess())

	t.Logf("Agent registered: %s", agentID)

	// 4. Simulate receiving work (in a real scenario, the server would assign work)
	// For this test, we'll manually update the run to simulate assignment

	// Start the run
	err = testEnv.RunRepo.Start(ctx, run.ID, agentID)
	require.NoError(t, err)

	// 5. Add test results as if the agent executed tests
	results := []database.TestResult{
		{RunID: run.ID, TestName: "TestE2E_Feature1", Status: database.ResultStatusPass, DurationMs: Ptr(int64(100))},
		{RunID: run.ID, TestName: "TestE2E_Feature2", Status: database.ResultStatusPass, DurationMs: Ptr(int64(150))},
		{RunID: run.ID, TestName: "TestE2E_Feature3", Status: database.ResultStatusFail, DurationMs: Ptr(int64(200)), ErrorMessage: Ptr("assertion failed")},
	}
	err = testEnv.ResultRepo.BatchCreate(ctx, results)
	require.NoError(t, err)

	// 6. Upload test artifacts
	_, err = testEnv.Storage.Upload(ctx, run.ID, "test-output.log", bytes.NewReader([]byte("Test execution log...")))
	require.NoError(t, err)

	// 7. Finish the run
	err = testEnv.RunRepo.Finish(ctx, run.ID, database.RunStatusFailed, database.RunResults{
		TotalTests:  3,
		PassedTests: 2,
		FailedTests: 1,
		DurationMs:  450,
	})
	require.NoError(t, err)

	// 8. Verify final state
	finalRun, err := testEnv.RunRepo.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, database.RunStatusFailed, finalRun.Status)
	assert.True(t, finalRun.IsTerminal())
	assert.NotNil(t, finalRun.StartedAt)
	assert.NotNil(t, finalRun.FinishedAt)
	assert.Equal(t, 3, finalRun.TotalTests)
	assert.Equal(t, 2, finalRun.PassedTests)
	assert.Equal(t, 1, finalRun.FailedTests)

	// Verify results are stored
	storedResults, err := testEnv.ResultRepo.ListByRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Len(t, storedResults, 3)

	// Verify artifact exists via List
	artifacts, err := testEnv.Storage.List(ctx, run.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, artifacts)

	// Cleanup artifacts
	err = testEnv.Storage.DeleteByRun(ctx, run.ID)
	require.NoError(t, err)

	// Close agent stream
	stream.CloseSend()

	t.Log("Full test execution flow completed successfully")
}

// ============================================================================
// AGENT STREAM MESSAGE HANDLING TESTS
// ============================================================================

func TestE2E_AgentHeartbeatStatusUpdates(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		testEnv.GRPCAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := conductorv1.NewAgentServiceClient(conn)
	stream, err := client.WorkStream(ctx)
	require.NoError(t, err)

	agentID := uuid.New()

	// Register
	err = stream.Send(&conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_Register{
			Register: &conductorv1.RegisterRequest{
				AgentId:      agentID.String(),
				Name:         "heartbeat-test-agent",
				Version:      "1.0.0",
				Capabilities: &conductorv1.Capabilities{MaxParallel: 1},
			},
		},
	})
	require.NoError(t, err)

	resp, err := stream.Recv()
	require.NoError(t, err)
	require.True(t, resp.GetRegisterResponse().GetSuccess())

	// Test status transitions through heartbeats
	statuses := []conductorv1.AgentStatus{
		conductorv1.AgentStatus_AGENT_STATUS_IDLE,
		conductorv1.AgentStatus_AGENT_STATUS_BUSY,
		conductorv1.AgentStatus_AGENT_STATUS_IDLE,
		conductorv1.AgentStatus_AGENT_STATUS_DRAINING,
	}

	for _, status := range statuses {
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Heartbeat{
				Heartbeat: &conductorv1.Heartbeat{
					Status: status,
				},
			},
		})
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond) // Allow processing
	}

	// Verify final status
	agent, err := testEnv.AgentRepo.Get(ctx, agentID)
	require.NoError(t, err)
	assert.Equal(t, database.AgentStatusDraining, agent.Status)

	stream.CloseSend()
}

// ============================================================================
// ERROR HANDLING TESTS
// ============================================================================

func TestE2E_InvalidAgentID(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		testEnv.GRPCAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := conductorv1.NewAgentServiceClient(conn)
	stream, err := client.WorkStream(ctx)
	require.NoError(t, err)

	// Try to register with invalid agent ID
	err = stream.Send(&conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_Register{
			Register: &conductorv1.RegisterRequest{
				AgentId:      "not-a-valid-uuid",
				Name:         "invalid-agent",
				Version:      "1.0.0",
				Capabilities: &conductorv1.Capabilities{},
			},
		},
	})
	require.NoError(t, err)

	// Should receive error response
	resp, err := stream.Recv()
	if err == nil {
		// If we got a response, it should indicate failure
		regResp := resp.GetRegisterResponse()
		if regResp != nil {
			assert.False(t, regResp.Success)
			assert.Contains(t, regResp.ErrorMessage, "invalid")
		}
	}
	// The stream might also just error out, which is also acceptable
}

func TestE2E_HeartbeatWithoutRegistration(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		testEnv.GRPCAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := conductorv1.NewAgentServiceClient(conn)
	stream, err := client.WorkStream(ctx)
	require.NoError(t, err)

	// Try to send heartbeat without registering first
	err = stream.Send(&conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_Heartbeat{
			Heartbeat: &conductorv1.Heartbeat{
				Status: conductorv1.AgentStatus_AGENT_STATUS_IDLE,
			},
		},
	})
	require.NoError(t, err)

	// Try to receive - should get an error
	_, err = stream.Recv()
	assert.Error(t, err, "should get error when sending heartbeat without registration")
}

// ============================================================================
// CONCURRENT RUN TESTS
// ============================================================================

func TestE2E_ConcurrentRunCreation(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create a service for the runs
	service, err := testEnv.CreateTestService(ctx, fmt.Sprintf("concurrent-runs-%s", uuid.New().String()[:8]))
	require.NoError(t, err)
	defer testEnv.ServiceRepo.Delete(ctx, service.ID)

	const numRuns = 10
	var wg sync.WaitGroup
	runIDs := make(chan uuid.UUID, numRuns)
	errors := make(chan error, numRuns)

	// Create runs concurrently
	for i := 0; i < numRuns; i++ {
		wg.Add(1)
		go func(runNum int) {
			defer wg.Done()

			triggerType := database.TriggerTypeManual
			run := &database.TestRun{
				ServiceID:   service.ID,
				Status:      database.RunStatusPending,
				GitRef:      Ptr(fmt.Sprintf("branch-%d", runNum)),
				TriggerType: &triggerType,
				Priority:    runNum,
			}

			if err := testEnv.RunRepo.Create(ctx, run); err != nil {
				errors <- fmt.Errorf("run %d: %w", runNum, err)
				return
			}
			runIDs <- run.ID
		}(i)
	}

	wg.Wait()
	close(runIDs)
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}

	// Verify all runs were created
	var createdIDs []uuid.UUID
	for id := range runIDs {
		createdIDs = append(createdIDs, id)
	}
	assert.Len(t, createdIDs, numRuns)

	// Verify we can list runs by service
	runs, err := testEnv.RunRepo.ListByService(ctx, service.ID, database.Pagination{Limit: 20, Offset: 0})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(runs), numRuns)
}

// ============================================================================
// DATABASE TRANSACTION TESTS
// ============================================================================

func TestE2E_DatabaseHealth(t *testing.T) {
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify database connection is healthy
	err := testEnv.DB.Health(ctx)
	require.NoError(t, err)
}

// ============================================================================
// CLEANUP VERIFICATION TEST
// ============================================================================

func TestE2E_EnvironmentCleanup(t *testing.T) {
	// This test should run last to verify the test environment is still functional
	if testEnv == nil {
		t.Skip("Test environment not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Quick health checks
	err := testEnv.DB.Health(ctx)
	assert.NoError(t, err, "database should still be healthy")

	// Verify we can still create a service
	service, err := testEnv.CreateTestService(ctx, fmt.Sprintf("cleanup-test-%s", uuid.New().String()[:8]))
	assert.NoError(t, err)

	if service != nil {
		testEnv.ServiceRepo.Delete(ctx, service.ID)
	}
}

// ============================================================================
// BENCHMARK TESTS
// ============================================================================

func BenchmarkE2E_ServiceCreation(b *testing.B) {
	if testEnv == nil {
		b.Skip("Test environment not available")
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service := &database.Service{
			Name:          fmt.Sprintf("bench-service-%d", i),
			GitURL:        "https://github.com/bench/repo",
			DefaultBranch: "main",
		}
		_ = testEnv.ServiceRepo.Create(ctx, service)
		_ = testEnv.ServiceRepo.Delete(ctx, service.ID)
	}
}

func BenchmarkE2E_RunCreation(b *testing.B) {
	if testEnv == nil {
		b.Skip("Test environment not available")
	}

	ctx := context.Background()

	// Create a service for the benchmark
	service := &database.Service{
		Name:          "bench-run-service",
		GitURL:        "https://github.com/bench/repo",
		DefaultBranch: "main",
	}
	_ = testEnv.ServiceRepo.Create(ctx, service)
	defer testEnv.ServiceRepo.Delete(ctx, service.ID)

	triggerType := database.TriggerTypeManual

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		run := &database.TestRun{
			ServiceID:   service.ID,
			Status:      database.RunStatusPending,
			TriggerType: &triggerType,
		}
		_ = testEnv.RunRepo.Create(ctx, run)
	}
}

// ============================================================================
// HELPER: Stream receiver with timeout
// ============================================================================

func recvWithTimeout(stream conductorv1.AgentService_WorkStreamClient, timeout time.Duration) (*conductorv1.ControlMessage, error) {
	msgCh := make(chan *conductorv1.ControlMessage, 1)
	errCh := make(chan error, 1)

	go func() {
		msg, err := stream.Recv()
		if err != nil {
			errCh <- err
			return
		}
		msgCh <- msg
	}()

	select {
	case msg := <-msgCh:
		return msg, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(timeout):
		return nil, io.ErrNoProgress
	}
}
