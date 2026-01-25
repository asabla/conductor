//go:build integration

// Package server provides integration tests for gRPC and HTTP servers.
package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/database"
	"github.com/conductor/conductor/pkg/testutil"
)

// testInfra holds shared infrastructure for server integration tests.
var testInfra struct {
	db       *database.DB
	postgres *testutil.PostgresContainer
}

func TestMain(m *testing.M) {
	if !testutil.IsDockerAvailable() {
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start postgres
	pg, err := testutil.NewPostgresContainer(ctx, testutil.DefaultPostgresConfig())
	if err != nil {
		panic("failed to start postgres: " + err.Error())
	}
	testInfra.postgres = pg

	// Create database connection
	dbCfg := database.DefaultConfig(pg.ConnStr)
	dbCfg.MaxConns = 5
	dbCfg.MinConns = 1
	db, err := database.New(ctx, dbCfg)
	if err != nil {
		pg.Terminate(ctx)
		panic("failed to create database: " + err.Error())
	}
	testInfra.db = db

	// Run migrations
	migrationsFS := os.DirFS("../../migrations")
	migrator, err := database.NewMigratorFromFS(db, migrationsFS)
	if err != nil {
		db.Close()
		pg.Terminate(ctx)
		panic("failed to create migrator: " + err.Error())
	}
	if _, err := migrator.Up(ctx); err != nil {
		db.Close()
		pg.Terminate(ctx)
		panic("failed to run migrations: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Cleanup
	db.Close()
	pg.Terminate(context.Background())

	os.Exit(code)
}

// ============================================================================
// WEBHOOK HANDLER TESTS
// ============================================================================

func TestWebhookHandler_GitHub(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	// Create mock service repository
	testOwner := "test-owner"
	serviceRepo := &mockWebhookServiceRepo{
		services: []database.Service{
			{
				ID:     uuid.New(),
				Name:   "test-service",
				GitURL: "https://github.com/test-org/test-repo",
				Owner:  &testOwner,
			},
		},
	}

	// Create mock scheduler
	scheduler := &mockRunScheduler{
		runs: make(map[uuid.UUID]*database.TestRun),
	}

	handler := NewWebhookHandler(WebhookConfig{
		GithubSecret: "test-secret",
		BaseURL:      "https://conductor.example.com",
	}, serviceRepo, scheduler, logger)

	t.Run("valid push event", func(t *testing.T) {
		payload := `{
			"ref": "refs/heads/main",
			"before": "0000000000000000000000000000000000000000",
			"after": "abc123def456",
			"deleted": false,
			"pusher": {
				"name": "test-user",
				"email": "test@example.com"
			},
			"repository": {
				"name": "test-repo",
				"full_name": "test-org/test-repo",
				"owner": {
					"login": "test-org"
				}
			},
			"sender": {
				"login": "test-user"
			}
		}`

		signature := computeGitHubSignature([]byte(payload), "test-secret")

		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-GitHub-Delivery", uuid.New().String())
		req.Header.Set("X-Hub-Signature-256", signature)
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.HandleGitHubWebhook(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]string
		json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.Equal(t, "ok", resp["status"])
	})

	t.Run("invalid signature", func(t *testing.T) {
		payload := `{"ref": "refs/heads/main"}`

		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-Hub-Signature-256", "sha256=invalidsignature")

		rec := httptest.NewRecorder()
		handler.HandleGitHubWebhook(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("ping event", func(t *testing.T) {
		payload := `{"zen": "Design for failure."}`
		signature := computeGitHubSignature([]byte(payload), "test-secret")

		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-GitHub-Event", "ping")
		req.Header.Set("X-Hub-Signature-256", signature)

		rec := httptest.NewRecorder()
		handler.HandleGitHubWebhook(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("pull request event", func(t *testing.T) {
		payload := `{
			"action": "opened",
			"number": 42,
			"pull_request": {
				"head": {
					"ref": "feature-branch",
					"sha": "def789abc123"
				}
			},
			"repository": {
				"name": "test-repo",
				"full_name": "test-org/test-repo",
				"owner": {
					"login": "test-org"
				}
			},
			"sender": {
				"login": "pr-author"
			}
		}`
		signature := computeGitHubSignature([]byte(payload), "test-secret")

		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-GitHub-Event", "pull_request")
		req.Header.Set("X-Hub-Signature-256", signature)

		rec := httptest.NewRecorder()
		handler.HandleGitHubWebhook(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("branch deletion ignored", func(t *testing.T) {
		payload := `{
			"ref": "refs/heads/deleted-branch",
			"deleted": true,
			"pusher": {"name": "test-user"},
			"repository": {
				"name": "test-repo",
				"full_name": "test-org/test-repo",
				"owner": {"login": "test-org"}
			}
		}`
		signature := computeGitHubSignature([]byte(payload), "test-secret")

		initialRunCount := len(scheduler.runs)

		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-Hub-Signature-256", signature)

		rec := httptest.NewRecorder()
		handler.HandleGitHubWebhook(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		// No new runs should be created for branch deletion
		assert.Equal(t, initialRunCount, len(scheduler.runs))
	})
}

func TestWebhookHandler_GitLab(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	gitlabOwner := "test-owner"
	serviceRepo := &mockWebhookServiceRepo{
		services: []database.Service{
			{
				ID:     uuid.New(),
				Name:   "gitlab-service",
				GitURL: "https://gitlab.com/test-group/test-project",
				Owner:  &gitlabOwner,
			},
		},
	}

	scheduler := &mockRunScheduler{
		runs: make(map[uuid.UUID]*database.TestRun),
	}

	handler := NewWebhookHandler(WebhookConfig{
		GitlabSecret: "gitlab-secret-token",
	}, serviceRepo, scheduler, logger)

	t.Run("valid push event", func(t *testing.T) {
		payload := `{
			"ref": "refs/heads/main",
			"before": "0000000000000000000000000000000000000000",
			"after": "abc123def456",
			"user_username": "gitlab-user",
			"project": {
				"path_with_namespace": "test-group/test-project"
			}
		}`

		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-Gitlab-Event", "Push Hook")
		req.Header.Set("X-Gitlab-Token", "gitlab-secret-token")
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.HandleGitLabWebhook(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		payload := `{"ref": "refs/heads/main"}`

		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-Gitlab-Event", "Push Hook")
		req.Header.Set("X-Gitlab-Token", "wrong-token")

		rec := httptest.NewRecorder()
		handler.HandleGitLabWebhook(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("merge request event", func(t *testing.T) {
		payload := `{
			"object_attributes": {
				"iid": 123,
				"action": "open",
				"source_branch": "feature-branch",
				"last_commit": {
					"id": "commit-sha-123"
				}
			},
			"project": {
				"path_with_namespace": "test-group/test-project"
			},
			"user": {
				"username": "mr-author"
			}
		}`

		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
		req.Header.Set("X-Gitlab-Token", "gitlab-secret-token")

		rec := httptest.NewRecorder()
		handler.HandleGitLabWebhook(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestWebhookHandler_Bitbucket(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	bitbucketOwner := "test-owner"
	serviceRepo := &mockWebhookServiceRepo{
		services: []database.Service{
			{
				ID:     uuid.New(),
				Name:   "bitbucket-service",
				GitURL: "https://bitbucket.org/test-workspace/test-repo",
				Owner:  &bitbucketOwner,
			},
		},
	}

	scheduler := &mockRunScheduler{
		runs: make(map[uuid.UUID]*database.TestRun),
	}

	handler := NewWebhookHandler(WebhookConfig{
		BitbucketSecret: "bitbucket-token",
	}, serviceRepo, scheduler, logger)

	t.Run("valid push event", func(t *testing.T) {
		payload := `{
			"push": {
				"changes": [
					{
						"new": {
							"name": "main",
							"target": {
								"hash": "abc123def456"
							}
						},
						"closed": false
					}
				]
			},
			"repository": {
				"full_name": "test-workspace/test-repo"
			},
			"actor": {
				"username": "bitbucket-user"
			}
		}`

		req := httptest.NewRequest(http.MethodPost, "/webhooks/bitbucket", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-Event-Key", "repo:push")
		req.Header.Set("Authorization", "Bearer bitbucket-token")
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.HandleBitbucketWebhook(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("invalid authorization", func(t *testing.T) {
		payload := `{"push": {"changes": []}}`

		req := httptest.NewRequest(http.MethodPost, "/webhooks/bitbucket", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-Event-Key", "repo:push")
		req.Header.Set("Authorization", "Bearer wrong-token")

		rec := httptest.NewRecorder()
		handler.HandleBitbucketWebhook(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("pull request event", func(t *testing.T) {
		payload := `{
			"pullrequest": {
				"id": 456,
				"source": {
					"branch": {
						"name": "feature-branch"
					},
					"commit": {
						"hash": "pr-commit-hash"
					}
				}
			},
			"repository": {
				"full_name": "test-workspace/test-repo"
			},
			"actor": {
				"username": "pr-author"
			}
		}`

		req := httptest.NewRequest(http.MethodPost, "/webhooks/bitbucket", bytes.NewBufferString(payload))
		req = req.WithContext(ctx)
		req.Header.Set("X-Event-Key", "pullrequest:created")
		req.Header.Set("Authorization", "Bearer bitbucket-token")

		rec := httptest.NewRecorder()
		handler.HandleBitbucketWebhook(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// ============================================================================
// HTTP SERVER TESTS
// ============================================================================

func TestHTTPServer_Middleware(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	t.Run("CORS headers", func(t *testing.T) {
		server, err := NewHTTPServer(HTTPConfig{
			Port:           0,
			EnableCORS:     true,
			AllowedOrigins: []string{"https://example.com"},
		}, logger)
		require.NoError(t, err)

		handler := server.buildHandler()

		// Test preflight request
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/services", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
	})

	t.Run("request ID middleware", func(t *testing.T) {
		server, err := NewHTTPServer(HTTPConfig{
			Port: 0,
		}, logger)
		require.NoError(t, err)

		handler := server.buildHandler()

		// Request without X-Request-ID should get one generated
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		requestID := rec.Header().Get("X-Request-ID")
		assert.NotEmpty(t, requestID)

		// Request with X-Request-ID should preserve it
		req = httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Request-ID", "custom-request-id")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, "custom-request-id", rec.Header().Get("X-Request-ID"))
	})

	t.Run("recovery middleware", func(t *testing.T) {
		server, err := NewHTTPServer(HTTPConfig{
			Port: 0,
		}, logger)
		require.NoError(t, err)

		// Create a handler that panics
		panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		})

		handler := server.recoveryMiddleware(panicHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		// Should not panic
		assert.NotPanics(t, func() {
			handler.ServeHTTP(rec, req)
		})

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

// ============================================================================
// GRPC SERVER TESTS
// ============================================================================

func TestGRPCServer_AgentRegistration(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	// Create mock dependencies
	agentRepo := newMockAgentRepository()
	runRepo := newMockRunRepository()
	scheduler := &mockWorkScheduler{}

	deps := AgentServiceDeps{
		AgentRepo:        agentRepo,
		RunRepo:          runRepo,
		Scheduler:        scheduler,
		HeartbeatTimeout: 90 * time.Second,
		ServerVersion:    "test-1.0.0",
	}

	// Create gRPC server
	services := Services{
		AgentService: deps,
	}

	grpcServer := NewGRPCServer(GRPCConfig{
		Port:             0, // Random port
		MaxRecvMsgSize:   16 * 1024 * 1024,
		MaxSendMsgSize:   16 * 1024 * 1024,
		EnableReflection: true,
	}, services, nil, logger)

	// Start server
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	go func() {
		grpcServer.server.Serve(listener)
	}()
	defer grpcServer.server.Stop()

	// Create client
	conn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := conductorv1.NewAgentServiceClient(conn)

	t.Run("agent work stream", func(t *testing.T) {
		stream, err := client.WorkStream(ctx)
		require.NoError(t, err)

		agentID := uuid.New()

		// Send register request
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Register{
				Register: &conductorv1.RegisterRequest{
					AgentId: agentID.String(),
					Name:    "test-agent",
					Version: "1.0.0",
					Capabilities: &conductorv1.Capabilities{
						MaxParallel:     4,
						NetworkZones:    []string{"default"},
						DockerAvailable: true,
					},
				},
			},
		})
		require.NoError(t, err)

		// Receive register response
		msg, err := stream.Recv()
		require.NoError(t, err)

		regResp, ok := msg.Message.(*conductorv1.ControlMessage_RegisterResponse)
		require.True(t, ok)
		assert.True(t, regResp.RegisterResponse.Success)
		assert.Equal(t, "test-1.0.0", regResp.RegisterResponse.ServerVersion)

		// Send heartbeat
		err = stream.Send(&conductorv1.AgentMessage{
			Message: &conductorv1.AgentMessage_Heartbeat{
				Heartbeat: &conductorv1.Heartbeat{
					Status:       conductorv1.AgentStatus_AGENT_STATUS_IDLE,
					ActiveRunIds: []string{},
				},
			},
		})
		require.NoError(t, err)

		// Close stream
		stream.CloseSend()
	})
}

func TestGRPCServer_AgentManagement(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	// Create mock dependencies with pre-populated agent
	agentRepo := newMockAgentRepository()
	testAgentID := uuid.New()
	agentRepo.agents[testAgentID] = &database.Agent{
		ID:              testAgentID,
		Name:            "existing-agent",
		Status:          database.AgentStatusIdle,
		NetworkZones:    []string{"default"},
		MaxParallel:     4,
		DockerAvailable: true,
		RegisteredAt:    time.Now().Add(-1 * time.Hour),
	}

	deps := AgentServiceDeps{
		AgentRepo:        agentRepo,
		HeartbeatTimeout: 90 * time.Second,
	}

	services := Services{
		AgentService: deps,
	}

	grpcServer := NewGRPCServer(GRPCConfig{
		Port:             0,
		EnableReflection: true,
	}, services, nil, logger)

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	go func() {
		grpcServer.server.Serve(listener)
	}()
	defer grpcServer.server.Stop()

	conn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := conductorv1.NewAgentManagementServiceClient(conn)

	t.Run("list agents", func(t *testing.T) {
		resp, err := client.ListAgents(ctx, &conductorv1.ListAgentsRequest{})
		require.NoError(t, err)

		assert.GreaterOrEqual(t, len(resp.Agents), 1)

		// Find our test agent
		var found bool
		for _, agent := range resp.Agents {
			if agent.Id == testAgentID.String() {
				found = true
				assert.Equal(t, "existing-agent", agent.Name)
				assert.Equal(t, conductorv1.AgentStatus_AGENT_STATUS_IDLE, agent.Status)
				break
			}
		}
		assert.True(t, found, "test agent not found in list")
	})

	t.Run("get agent", func(t *testing.T) {
		resp, err := client.GetAgent(ctx, &conductorv1.GetAgentRequest{
			AgentId: testAgentID.String(),
		})
		require.NoError(t, err)

		assert.Equal(t, testAgentID.String(), resp.Agent.Id)
		assert.Equal(t, "existing-agent", resp.Agent.Name)
	})

	t.Run("get non-existent agent", func(t *testing.T) {
		_, err := client.GetAgent(ctx, &conductorv1.GetAgentRequest{
			AgentId: uuid.New().String(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("drain agent", func(t *testing.T) {
		resp, err := client.DrainAgent(ctx, &conductorv1.DrainAgentRequest{
			AgentId: testAgentID.String(),
			Reason:  "maintenance",
		})
		require.NoError(t, err)

		assert.Equal(t, conductorv1.AgentStatus_AGENT_STATUS_DRAINING, resp.Agent.Status)
	})

	t.Run("undrain agent", func(t *testing.T) {
		// Agent is already draining from previous test
		resp, err := client.UndrainAgent(ctx, &conductorv1.UndrainAgentRequest{
			AgentId: testAgentID.String(),
		})
		require.NoError(t, err)

		assert.Equal(t, conductorv1.AgentStatus_AGENT_STATUS_IDLE, resp.Agent.Status)
	})

	t.Run("delete offline agent", func(t *testing.T) {
		// First set agent to offline
		agentRepo.agents[testAgentID].Status = database.AgentStatusOffline

		resp, err := client.DeleteAgent(ctx, &conductorv1.DeleteAgentRequest{
			AgentId: testAgentID.String(),
		})
		require.NoError(t, err)
		assert.True(t, resp.Success)

		// Verify agent is deleted
		_, err = client.GetAgent(ctx, &conductorv1.GetAgentRequest{
			AgentId: testAgentID.String(),
		})
		require.Error(t, err)
	})
}

// ============================================================================
// HELPER FUNCTIONS AND MOCKS
// ============================================================================

func computeGitHubSignature(payload []byte, secret string) string {
	return "sha256=" + computeHMACSHA256(payload, secret)
}

func computeHMACSHA256(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// mockWebhookServiceRepo implements WebhookServiceRepository for testing.
type mockWebhookServiceRepo struct {
	services []database.Service
}

func (m *mockWebhookServiceRepo) List(ctx context.Context, page database.Pagination) ([]database.Service, error) {
	return m.services, nil
}

// mockRunScheduler implements RunScheduler for testing.
type mockRunScheduler struct {
	runs map[uuid.UUID]*database.TestRun
}

func (m *mockRunScheduler) ScheduleRun(ctx context.Context, req ScheduleRunRequest) (*database.TestRun, error) {
	run := &database.TestRun{
		ID:          uuid.New(),
		ServiceID:   req.ServiceID,
		Status:      database.RunStatusPending,
		TriggerType: &req.TriggerType,
		TriggeredBy: &req.TriggeredBy,
		CreatedAt:   time.Now(),
	}
	if req.GitRef != "" {
		run.GitRef = &req.GitRef
	}
	if req.GitSHA != "" {
		run.GitSHA = &req.GitSHA
	}
	m.runs[run.ID] = run
	return run, nil
}

// mockAgentRepository implements AgentRepository for testing.
type mockAgentRepository struct {
	agents map[uuid.UUID]*database.Agent
}

func newMockAgentRepository() *mockAgentRepository {
	return &mockAgentRepository{
		agents: make(map[uuid.UUID]*database.Agent),
	}
}

func (m *mockAgentRepository) Create(ctx context.Context, agent *database.Agent) error {
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentRepository) Update(ctx context.Context, agent *database.Agent) error {
	if _, exists := m.agents[agent.ID]; !exists {
		return database.ErrNotFound
	}
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentRepository) GetByID(ctx context.Context, id uuid.UUID) (*database.Agent, error) {
	agent, exists := m.agents[id]
	if !exists {
		return nil, database.ErrNotFound
	}
	return agent, nil
}

func (m *mockAgentRepository) UpdateHeartbeat(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	agent, exists := m.agents[id]
	if !exists {
		return database.ErrNotFound
	}
	now := time.Now()
	agent.LastHeartbeat = &now
	agent.Status = status
	return nil
}

func (m *mockAgentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	agent, exists := m.agents[id]
	if !exists {
		return database.ErrNotFound
	}
	agent.Status = status
	return nil
}

func (m *mockAgentRepository) List(ctx context.Context, filter AgentFilter, pagination database.Pagination) ([]*database.Agent, int, error) {
	var result []*database.Agent
	for _, agent := range m.agents {
		result = append(result, agent)
	}
	return result, len(result), nil
}

func (m *mockAgentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if _, exists := m.agents[id]; !exists {
		return database.ErrNotFound
	}
	delete(m.agents, id)
	return nil
}

// mockRunRepository implements RunRepository for testing.
type mockRunRepository struct {
	runs map[uuid.UUID]*database.TestRun
	mu   sync.RWMutex
}

func newMockRunRepository() *mockRunRepository {
	return &mockRunRepository{
		runs: make(map[uuid.UUID]*database.TestRun),
	}
}

func (m *mockRunRepository) Create(ctx context.Context, run *database.TestRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[run.ID] = run
	return nil
}

func (m *mockRunRepository) Update(ctx context.Context, run *database.TestRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[run.ID] = run
	return nil
}

func (m *mockRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*database.TestRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if run, ok := m.runs[id]; ok {
		return run, nil
	}
	return nil, fmt.Errorf("run not found: %s", id)
}

func (m *mockRunRepository) List(ctx context.Context, filter RunFilter, pagination database.Pagination) ([]*database.TestRun, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	runs := make([]*database.TestRun, 0, len(m.runs))
	for _, run := range m.runs {
		runs = append(runs, run)
	}
	return runs, len(runs), nil
}

func (m *mockRunRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status database.RunStatus, errorMsg *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run, ok := m.runs[id]; ok {
		run.Status = status
		run.ErrorMessage = errorMsg
		return nil
	}
	return fmt.Errorf("run not found: %s", id)
}

// mockWorkScheduler implements WorkScheduler for testing.
type mockWorkScheduler struct{}

func (m *mockWorkScheduler) AssignWork(ctx context.Context, agentID uuid.UUID, capabilities *conductorv1.Capabilities) (*conductorv1.AssignWork, error) {
	return nil, nil // No work available
}

func (m *mockWorkScheduler) CancelWork(ctx context.Context, runID uuid.UUID, reason string) error {
	return nil
}

func (m *mockWorkScheduler) HandleWorkAccepted(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, shardID *uuid.UUID) error {
	return nil
}

func (m *mockWorkScheduler) HandleWorkRejected(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, shardID *uuid.UUID, reason string) error {
	return nil
}

func (m *mockWorkScheduler) HandleRunComplete(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, shardID *uuid.UUID, result *conductorv1.RunComplete) error {
	return nil
}
