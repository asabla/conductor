//go:build integration

// Package server provides additional gRPC integration tests.
package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/database"
	"github.com/conductor/conductor/pkg/testutil"
)

// ============================================================================
// AGENT WORK STREAM TESTS
// ============================================================================

func TestGRPCServer_WorkStream_Registration(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupGRPCTestEnv(t)
	defer env.cleanup()

	client := conductorv1.NewAgentServiceClient(env.conn)

	t.Run("successful registration", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		agentID := uuid.New()

		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Register{
				Register: &conductorv1.RegisterRequest{
					AgentId: agentID.String(),
					Name:    "test-agent-registration",
					Version: "1.0.0",
					Capabilities: &conductorv1.Capabilities{
						MaxParallel:     4,
						NetworkZones:    []string{"default", "zone-a"},
						DockerAvailable: true,
					},
					Labels: map[string]string{
						"env":  "test",
						"tier": "integration",
					},
				},
			},
		})
		require.NoError(t, err)

		msg, err := stream.Recv()
		require.NoError(t, err)

		regResp := msg.GetRegisterResponse()
		require.NotNil(t, regResp)
		assert.True(t, regResp.Success)
		assert.NotEmpty(t, regResp.ServerVersion)
		assert.Greater(t, regResp.HeartbeatIntervalSeconds, int32(0))

		stream.CloseSend()
	})

	t.Run("registration with invalid agent ID", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

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

		msg, err := stream.Recv()
		if err == nil {
			regResp := msg.GetRegisterResponse()
			if regResp != nil {
				assert.False(t, regResp.Success)
				assert.Contains(t, regResp.ErrorMessage, "invalid")
			}
		}
		// Either error or unsuccessful response is acceptable

		stream.CloseSend()
	})

	t.Run("registration with empty name", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Register{
				Register: &conductorv1.RegisterRequest{
					AgentId:      uuid.New().String(),
					Name:         "", // Empty name
					Version:      "1.0.0",
					Capabilities: &conductorv1.Capabilities{},
				},
			},
		})
		require.NoError(t, err)

		msg, err := stream.Recv()
		if err == nil {
			regResp := msg.GetRegisterResponse()
			if regResp != nil {
				// Either success with generated name or failure
				// depends on server implementation
				t.Logf("Registration response: success=%v, error=%s", regResp.Success, regResp.ErrorMessage)
			}
		}

		stream.CloseSend()
	})
}

func TestGRPCServer_WorkStream_Heartbeat(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupGRPCTestEnv(t)
	defer env.cleanup()

	client := conductorv1.NewAgentServiceClient(env.conn)

	t.Run("heartbeat after registration", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		agentID := uuid.New()

		// Register first
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Register{
				Register: &conductorv1.RegisterRequest{
					AgentId:      agentID.String(),
					Name:         "heartbeat-test-agent",
					Version:      "1.0.0",
					Capabilities: &conductorv1.Capabilities{MaxParallel: 4},
				},
			},
		})
		require.NoError(t, err)

		msg, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, msg.GetRegisterResponse().GetSuccess())

		// Send multiple heartbeats with different statuses
		statuses := []conductorv1.AgentStatus{
			conductorv1.AgentStatus_AGENT_STATUS_IDLE,
			conductorv1.AgentStatus_AGENT_STATUS_BUSY,
			conductorv1.AgentStatus_AGENT_STATUS_IDLE,
		}

		for _, status := range statuses {
			err = stream.Send(&conductorv1.AgentMessage{
				Message: &conductorv1.AgentMessage_Heartbeat{
					Heartbeat: &conductorv1.Heartbeat{
						Status:       status,
						ActiveRunIds: []string{},
					},
				},
			})
			require.NoError(t, err)
			time.Sleep(50 * time.Millisecond)
		}

		// Verify agent status in repository
		agent, err := env.agentRepo.GetByID(ctx, agentID)
		require.NoError(t, err)
		assert.Equal(t, database.AgentStatusIdle, agent.Status)

		stream.CloseSend()
	})

	t.Run("heartbeat without registration fails", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		// Send heartbeat without registering first
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Heartbeat{
				Heartbeat: &conductorv1.Heartbeat{
					Status: conductorv1.AgentStatus_AGENT_STATUS_IDLE,
				},
			},
		})
		require.NoError(t, err)

		// Should get an error
		_, err = stream.Recv()
		assert.Error(t, err)
	})

	t.Run("heartbeat with active runs", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		agentID := uuid.New()

		// Register
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Register{
				Register: &conductorv1.RegisterRequest{
					AgentId:      agentID.String(),
					Name:         "busy-agent",
					Version:      "1.0.0",
					Capabilities: &conductorv1.Capabilities{MaxParallel: 4},
				},
			},
		})
		require.NoError(t, err)

		msg, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, msg.GetRegisterResponse().GetSuccess())

		// Send heartbeat with active runs
		activeRunIDs := []string{uuid.New().String(), uuid.New().String()}
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Heartbeat{
				Heartbeat: &conductorv1.Heartbeat{
					Status:       conductorv1.AgentStatus_AGENT_STATUS_BUSY,
					ActiveRunIds: activeRunIDs,
				},
			},
		})
		require.NoError(t, err)

		// Verify agent is marked as busy
		time.Sleep(100 * time.Millisecond)
		agent, err := env.agentRepo.GetByID(ctx, agentID)
		require.NoError(t, err)
		assert.Equal(t, database.AgentStatusBusy, agent.Status)

		stream.CloseSend()
	})
}

func TestGRPCServer_WorkStream_WorkAssignment(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupGRPCTestEnv(t)
	defer env.cleanup()

	client := conductorv1.NewAgentServiceClient(env.conn)

	t.Run("work accepted message", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		agentID := uuid.New()
		runID := uuid.New()

		// Register
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Register{
				Register: &conductorv1.RegisterRequest{
					AgentId:      agentID.String(),
					Name:         "work-accept-agent",
					Version:      "1.0.0",
					Capabilities: &conductorv1.Capabilities{MaxParallel: 4},
				},
			},
		})
		require.NoError(t, err)

		msg, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, msg.GetRegisterResponse().GetSuccess())

		// Send work accepted
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_WorkAccepted{
				WorkAccepted: &conductorv1.WorkAccepted{
					RunId: runID.String(),
				},
			},
		})
		require.NoError(t, err)

		stream.CloseSend()
	})

	t.Run("work rejected message", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		agentID := uuid.New()
		runID := uuid.New()

		// Register
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Register{
				Register: &conductorv1.RegisterRequest{
					AgentId:      agentID.String(),
					Name:         "work-reject-agent",
					Version:      "1.0.0",
					Capabilities: &conductorv1.Capabilities{MaxParallel: 4},
				},
			},
		})
		require.NoError(t, err)

		msg, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, msg.GetRegisterResponse().GetSuccess())

		// Send work rejected
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_WorkRejected{
				WorkRejected: &conductorv1.WorkRejected{
					RunId:  runID.String(),
					Reason: "agent is busy",
				},
			},
		})
		require.NoError(t, err)

		stream.CloseSend()
	})
}

func TestGRPCServer_WorkStream_ResultStreaming(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupGRPCTestEnv(t)
	defer env.cleanup()

	client := conductorv1.NewAgentServiceClient(env.conn)

	t.Run("result stream with test results", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		agentID := uuid.New()
		runID := uuid.New()

		// Register
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Register{
				Register: &conductorv1.RegisterRequest{
					AgentId:      agentID.String(),
					Name:         "result-stream-agent",
					Version:      "1.0.0",
					Capabilities: &conductorv1.Capabilities{MaxParallel: 4},
				},
			},
		})
		require.NoError(t, err)

		msg, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, msg.GetRegisterResponse().GetSuccess())

		// Send result stream with test results
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_ResultStream{
				ResultStream: &conductorv1.ResultStream{
					RunId: runID.String(),
					Results: []*conductorv1.TestResult{
						{
							TestName:   "TestAddition",
							SuiteName:  "math",
							Status:     conductorv1.TestStatus_TEST_STATUS_PASS,
							DurationMs: 150,
						},
						{
							TestName:     "TestDivision",
							SuiteName:    "math",
							Status:       conductorv1.TestStatus_TEST_STATUS_FAIL,
							DurationMs:   200,
							ErrorMessage: "division by zero",
							StackTrace:   "at TestDivision (math_test.go:45)",
						},
						{
							TestName:   "TestSkipped",
							SuiteName:  "misc",
							Status:     conductorv1.TestStatus_TEST_STATUS_SKIP,
							DurationMs: 0,
						},
					},
				},
			},
		})
		require.NoError(t, err)

		stream.CloseSend()
	})

	t.Run("run complete message", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		agentID := uuid.New()
		runID := uuid.New()

		// Register
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Register{
				Register: &conductorv1.RegisterRequest{
					AgentId:      agentID.String(),
					Name:         "run-complete-agent",
					Version:      "1.0.0",
					Capabilities: &conductorv1.Capabilities{MaxParallel: 4},
				},
			},
		})
		require.NoError(t, err)

		msg, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, msg.GetRegisterResponse().GetSuccess())

		// Send run complete
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_RunComplete{
				RunComplete: &conductorv1.RunComplete{
					RunId:        runID.String(),
					Status:       conductorv1.RunStatus_RUN_STATUS_PASSED,
					TotalTests:   10,
					PassedTests:  8,
					FailedTests:  1,
					SkippedTests: 1,
					DurationMs:   5000,
				},
			},
		})
		require.NoError(t, err)

		stream.CloseSend()
	})

	t.Run("run complete with error", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		agentID := uuid.New()
		runID := uuid.New()

		// Register
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Register{
				Register: &conductorv1.RegisterRequest{
					AgentId:      agentID.String(),
					Name:         "error-agent",
					Version:      "1.0.0",
					Capabilities: &conductorv1.Capabilities{MaxParallel: 4},
				},
			},
		})
		require.NoError(t, err)

		msg, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, msg.GetRegisterResponse().GetSuccess())

		// Send run complete with error
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_RunComplete{
				RunComplete: &conductorv1.RunComplete{
					RunId:        runID.String(),
					Status:       conductorv1.RunStatus_RUN_STATUS_ERROR,
					ErrorMessage: "failed to clone repository: connection timeout",
					DurationMs:   1500,
				},
			},
		})
		require.NoError(t, err)

		stream.CloseSend()
	})
}

func TestGRPCServer_WorkStream_ConcurrentAgents(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	env := setupGRPCTestEnv(t)
	defer env.cleanup()

	client := conductorv1.NewAgentServiceClient(env.conn)

	const numAgents = 5
	var wg sync.WaitGroup
	errors := make(chan error, numAgents)
	successCount := make(chan int, numAgents)

	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(agentNum int) {
			defer wg.Done()

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
							MaxParallel:  2,
							NetworkZones: []string{"default"},
						},
					},
				},
			})
			if err != nil {
				errors <- fmt.Errorf("agent %d: failed to send register: %w", agentNum, err)
				return
			}

			msg, err := stream.Recv()
			if err != nil {
				errors <- fmt.Errorf("agent %d: failed to receive response: %w", agentNum, err)
				return
			}

			regResp := msg.GetRegisterResponse()
			if regResp == nil || !regResp.Success {
				errors <- fmt.Errorf("agent %d: registration failed", agentNum)
				return
			}

			// Send heartbeats
			for j := 0; j < 5; j++ {
				err = stream.Send(&conductorv1.AgentMessage{
					Message: &conductorv1.AgentMessage_Heartbeat{
						Heartbeat: &conductorv1.Heartbeat{
							Status: conductorv1.AgentStatus_AGENT_STATUS_IDLE,
						},
					},
				})
				if err != nil {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}

			stream.CloseSend()
			successCount <- 1
		}(i)
	}

	wg.Wait()
	close(errors)
	close(successCount)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}

	// Count successes
	total := 0
	for count := range successCount {
		total += count
	}
	assert.Equal(t, numAgents, total, "all agents should register successfully")
}

func TestGRPCServer_WorkStream_Reconnection(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupGRPCTestEnv(t)
	defer env.cleanup()

	client := conductorv1.NewAgentServiceClient(env.conn)
	agentID := uuid.New()

	// First connection
	stream1, err := client.WorkStream(ctx)
	require.NoError(t, err)

	err = stream1.Send(&conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_Register{
			Register: &conductorv1.RegisterRequest{
				AgentId:      agentID.String(),
				Name:         "reconnect-agent",
				Version:      "1.0.0",
				Capabilities: &conductorv1.Capabilities{MaxParallel: 4},
			},
		},
	})
	require.NoError(t, err)

	msg, err := stream1.Recv()
	require.NoError(t, err)
	require.True(t, msg.GetRegisterResponse().GetSuccess())

	// Close first connection
	stream1.CloseSend()
	time.Sleep(100 * time.Millisecond)

	// Second connection with same agent ID (re-registration)
	stream2, err := client.WorkStream(ctx)
	require.NoError(t, err)

	err = stream2.Send(&conductorv1.AgentMessage{
		Message: &conductorv1.AgentMessage_Register{
			Register: &conductorv1.RegisterRequest{
				AgentId:      agentID.String(),
				Name:         "reconnect-agent",
				Version:      "1.0.1", // Updated version
				Capabilities: &conductorv1.Capabilities{MaxParallel: 8},
			},
		},
	})
	require.NoError(t, err)

	msg, err = stream2.Recv()
	require.NoError(t, err)
	require.True(t, msg.GetRegisterResponse().GetSuccess())

	stream2.CloseSend()
}

// ============================================================================
// AGENT MANAGEMENT SERVICE TESTS
// ============================================================================

func TestGRPCServer_AgentManagement_ListAgents(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupGRPCTestEnv(t)
	defer env.cleanup()

	// Pre-populate with agents
	for i := 0; i < 3; i++ {
		agent := &database.Agent{
			ID:              uuid.New(),
			Name:            fmt.Sprintf("list-test-agent-%d", i),
			Status:          database.AgentStatusIdle,
			NetworkZones:    []string{"default"},
			MaxParallel:     4,
			DockerAvailable: true,
			RegisteredAt:    time.Now(),
		}
		env.agentRepo.Create(ctx, agent)
	}

	client := conductorv1.NewAgentManagementServiceClient(env.conn)

	t.Run("list all agents", func(t *testing.T) {
		resp, err := client.ListAgents(ctx, &conductorv1.ListAgentsRequest{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(resp.Agents), 3)
	})

	t.Run("list with pagination", func(t *testing.T) {
		resp, err := client.ListAgents(ctx, &conductorv1.ListAgentsRequest{
			PageSize: 2,
		})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(resp.Agents), 2)
	})

	t.Run("list with status filter", func(t *testing.T) {
		resp, err := client.ListAgents(ctx, &conductorv1.ListAgentsRequest{
			StatusFilter: conductorv1.AgentStatus_AGENT_STATUS_IDLE,
		})
		require.NoError(t, err)
		for _, agent := range resp.Agents {
			assert.Equal(t, conductorv1.AgentStatus_AGENT_STATUS_IDLE, agent.Status)
		}
	})
}

func TestGRPCServer_AgentManagement_DrainUndrain(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupGRPCTestEnv(t)
	defer env.cleanup()

	// Create a test agent
	agentID := uuid.New()
	agent := &database.Agent{
		ID:              agentID,
		Name:            "drain-test-agent",
		Status:          database.AgentStatusIdle,
		NetworkZones:    []string{"default"},
		MaxParallel:     4,
		DockerAvailable: true,
		RegisteredAt:    time.Now(),
	}
	env.agentRepo.Create(ctx, agent)

	client := conductorv1.NewAgentManagementServiceClient(env.conn)

	t.Run("drain agent", func(t *testing.T) {
		resp, err := client.DrainAgent(ctx, &conductorv1.DrainAgentRequest{
			AgentId: agentID.String(),
			Reason:  "scheduled maintenance",
		})
		require.NoError(t, err)
		assert.Equal(t, conductorv1.AgentStatus_AGENT_STATUS_DRAINING, resp.Agent.Status)

		// Verify in repository
		stored, err := env.agentRepo.GetByID(ctx, agentID)
		require.NoError(t, err)
		assert.Equal(t, database.AgentStatusDraining, stored.Status)
	})

	t.Run("undrain agent", func(t *testing.T) {
		resp, err := client.UndrainAgent(ctx, &conductorv1.UndrainAgentRequest{
			AgentId: agentID.String(),
		})
		require.NoError(t, err)
		assert.Equal(t, conductorv1.AgentStatus_AGENT_STATUS_IDLE, resp.Agent.Status)
	})

	t.Run("drain non-existent agent", func(t *testing.T) {
		_, err := client.DrainAgent(ctx, &conductorv1.DrainAgentRequest{
			AgentId: uuid.New().String(),
			Reason:  "test",
		})
		require.Error(t, err)
		assert.Equal(t, codes.NotFound, status.Code(err))
	})
}

func TestGRPCServer_AgentManagement_DeleteAgent(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupGRPCTestEnv(t)
	defer env.cleanup()

	client := conductorv1.NewAgentManagementServiceClient(env.conn)

	t.Run("delete offline agent", func(t *testing.T) {
		agentID := uuid.New()
		agent := &database.Agent{
			ID:           agentID,
			Name:         "delete-offline-agent",
			Status:       database.AgentStatusOffline,
			NetworkZones: []string{"default"},
			MaxParallel:  4,
			RegisteredAt: time.Now(),
		}
		env.agentRepo.Create(ctx, agent)

		resp, err := client.DeleteAgent(ctx, &conductorv1.DeleteAgentRequest{
			AgentId: agentID.String(),
		})
		require.NoError(t, err)
		assert.True(t, resp.Success)

		// Verify deletion
		_, err = env.agentRepo.GetByID(ctx, agentID)
		assert.Error(t, err)
	})

	t.Run("delete idle agent with force", func(t *testing.T) {
		agentID := uuid.New()
		agent := &database.Agent{
			ID:           agentID,
			Name:         "delete-idle-agent",
			Status:       database.AgentStatusIdle,
			NetworkZones: []string{"default"},
			MaxParallel:  4,
			RegisteredAt: time.Now(),
		}
		env.agentRepo.Create(ctx, agent)

		resp, err := client.DeleteAgent(ctx, &conductorv1.DeleteAgentRequest{
			AgentId: agentID.String(),
			Force:   true,
		})
		require.NoError(t, err)
		assert.True(t, resp.Success)
	})

	t.Run("delete busy agent without force fails", func(t *testing.T) {
		agentID := uuid.New()
		agent := &database.Agent{
			ID:           agentID,
			Name:         "delete-busy-agent",
			Status:       database.AgentStatusBusy,
			NetworkZones: []string{"default"},
			MaxParallel:  4,
			RegisteredAt: time.Now(),
		}
		env.agentRepo.Create(ctx, agent)

		_, err := client.DeleteAgent(ctx, &conductorv1.DeleteAgentRequest{
			AgentId: agentID.String(),
			Force:   false,
		})
		require.Error(t, err)
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	})
}

// ============================================================================
// RUN SERVICE TESTS
// ============================================================================

func TestGRPCServer_RunService_GetRun(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupGRPCTestEnv(t)
	defer env.cleanup()

	client := conductorv1.NewRunServiceClient(env.conn)

	t.Run("get non-existent run", func(t *testing.T) {
		_, err := client.GetRun(ctx, &conductorv1.GetRunRequest{
			RunId: uuid.New().String(),
		})
		require.Error(t, err)
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("get run with invalid ID", func(t *testing.T) {
		_, err := client.GetRun(ctx, &conductorv1.GetRunRequest{
			RunId: "not-a-valid-uuid",
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

// ============================================================================
// HELPER FUNCTIONS AND TYPES
// ============================================================================

type grpcTestEnv struct {
	server    *GRPCServer
	listener  net.Listener
	conn      *grpc.ClientConn
	agentRepo *grpcMockAgentRepository
	runRepo   *grpcMockRunRepository
	scheduler *grpcMockWorkScheduler
	cleanup   func()
}

func setupGRPCTestEnv(t *testing.T) *grpcTestEnv {
	t.Helper()

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	agentRepo := newGRPCMockAgentRepository()
	runRepo := newGRPCMockRunRepository()
	scheduler := &grpcMockWorkScheduler{}

	deps := AgentServiceDeps{
		AgentRepo:        agentRepo,
		RunRepo:          runRepo,
		Scheduler:        scheduler,
		HeartbeatTimeout: 90 * time.Second,
		ServerVersion:    "test-1.0.0",
	}

	services := Services{
		AgentService: deps,
	}

	grpcServer := NewGRPCServer(GRPCConfig{
		Port:             0,
		MaxRecvMsgSize:   16 * 1024 * 1024,
		MaxSendMsgSize:   16 * 1024 * 1024,
		EnableReflection: true,
	}, services, nil, logger)

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	go func() {
		grpcServer.server.Serve(listener)
	}()

	conn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	return &grpcTestEnv{
		server:    grpcServer,
		listener:  listener,
		conn:      conn,
		agentRepo: agentRepo,
		runRepo:   runRepo,
		scheduler: scheduler,
		cleanup: func() {
			conn.Close()
			grpcServer.server.Stop()
		},
	}
}

// grpcMockAgentRepository implements AgentRepository for testing.
type grpcMockAgentRepository struct {
	agents map[uuid.UUID]*database.Agent
	mu     sync.RWMutex
}

func newGRPCMockAgentRepository() *grpcMockAgentRepository {
	return &grpcMockAgentRepository{
		agents: make(map[uuid.UUID]*database.Agent),
	}
}

func (m *grpcMockAgentRepository) Create(ctx context.Context, agent *database.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[agent.ID] = agent
	return nil
}

func (m *grpcMockAgentRepository) Update(ctx context.Context, agent *database.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.agents[agent.ID]; !exists {
		return database.ErrNotFound
	}
	m.agents[agent.ID] = agent
	return nil
}

func (m *grpcMockAgentRepository) GetByID(ctx context.Context, id uuid.UUID) (*database.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agent, exists := m.agents[id]
	if !exists {
		return nil, database.ErrNotFound
	}
	return agent, nil
}

func (m *grpcMockAgentRepository) UpdateHeartbeat(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	agent, exists := m.agents[id]
	if !exists {
		return database.ErrNotFound
	}
	now := time.Now()
	agent.LastHeartbeat = &now
	agent.Status = status
	return nil
}

func (m *grpcMockAgentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	agent, exists := m.agents[id]
	if !exists {
		return database.ErrNotFound
	}
	agent.Status = status
	return nil
}

func (m *grpcMockAgentRepository) List(ctx context.Context, filter AgentFilter, pagination database.Pagination) ([]*database.Agent, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*database.Agent
	for _, agent := range m.agents {
		if filter.Status != "" && agent.Status != database.AgentStatus(filter.Status) {
			continue
		}
		result = append(result, agent)
	}
	return result, len(result), nil
}

func (m *grpcMockAgentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.agents[id]; !exists {
		return database.ErrNotFound
	}
	delete(m.agents, id)
	return nil
}

// grpcMockRunRepository implements RunRepository for testing.
type grpcMockRunRepository struct {
	runs map[uuid.UUID]*database.TestRun
	mu   sync.RWMutex
}

func newGRPCMockRunRepository() *grpcMockRunRepository {
	return &grpcMockRunRepository{
		runs: make(map[uuid.UUID]*database.TestRun),
	}
}

func (m *grpcMockRunRepository) Create(ctx context.Context, run *database.TestRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[run.ID] = run
	return nil
}

func (m *grpcMockRunRepository) Update(ctx context.Context, run *database.TestRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[run.ID] = run
	return nil
}

func (m *grpcMockRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*database.TestRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if run, ok := m.runs[id]; ok {
		return run, nil
	}
	return nil, database.ErrNotFound
}

func (m *grpcMockRunRepository) List(ctx context.Context, filter RunFilter, pagination database.Pagination) ([]*database.TestRun, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	runs := make([]*database.TestRun, 0, len(m.runs))
	for _, run := range m.runs {
		runs = append(runs, run)
	}
	return runs, len(runs), nil
}

func (m *grpcMockRunRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status database.RunStatus, errorMsg *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run, ok := m.runs[id]; ok {
		run.Status = status
		run.ErrorMessage = errorMsg
		return nil
	}
	return database.ErrNotFound
}

// grpcMockWorkScheduler implements WorkScheduler for testing.
type grpcMockWorkScheduler struct{}

func (m *grpcMockWorkScheduler) AssignWork(ctx context.Context, agentID uuid.UUID, capabilities *conductorv1.Capabilities) (*conductorv1.AssignWork, error) {
	return nil, nil
}

func (m *grpcMockWorkScheduler) CancelWork(ctx context.Context, runID uuid.UUID, reason string) error {
	return nil
}

func (m *grpcMockWorkScheduler) HandleWorkAccepted(ctx context.Context, agentID uuid.UUID, runID uuid.UUID) error {
	return nil
}

func (m *grpcMockWorkScheduler) HandleWorkRejected(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, reason string) error {
	return nil
}

func (m *grpcMockWorkScheduler) HandleRunComplete(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, result *conductorv1.RunComplete) error {
	return nil
}

// recvWithTimeoutGRPC receives a message with timeout.
func recvWithTimeoutGRPC(stream conductorv1.AgentService_WorkStreamClient, timeout time.Duration) (*conductorv1.ControlMessage, error) {
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
