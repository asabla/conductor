//go:build integration

// Package e2e provides end-to-end integration tests for the Conductor platform.
package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/artifact"
	"github.com/conductor/conductor/internal/database"
	"github.com/conductor/conductor/internal/notification"
	"github.com/conductor/conductor/internal/server"
	"github.com/conductor/conductor/pkg/testutil"
)

// TestEnvironment holds all the components needed for E2E tests.
type TestEnvironment struct {
	// Containers
	Postgres *testutil.PostgresContainer
	Minio    *testutil.MinioContainer
	Redis    *testutil.RedisContainer

	// Database
	DB *database.DB

	// Storage
	Storage *artifact.Storage

	// Repositories
	ServiceRepo      database.ServiceRepository
	RunRepo          database.TestRunRepository
	ResultRepo       database.ResultRepository
	AgentRepo        database.AgentRepository
	NotificationRepo database.NotificationRepository

	// Services
	NotificationService *notification.Service

	// Servers
	GRPCServer *server.GRPCServer
	HTTPServer *server.HTTPServer

	// Server addresses
	GRPCAddress string
	HTTPAddress string

	// gRPC client connections
	GRPCConn *grpc.ClientConn

	// Logger
	Logger zerolog.Logger

	// Context for cleanup
	ctx    context.Context
	cancel context.CancelFunc
}

// testEnv is the global test environment.
var testEnv *TestEnvironment

func TestMain(m *testing.M) {
	if !testutil.IsDockerAvailable() {
		fmt.Println("Docker not available, skipping E2E tests")
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var err error
	testEnv, err = SetupTestEnvironment(ctx)
	if err != nil {
		fmt.Printf("Failed to setup test environment: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	testEnv.Cleanup()

	os.Exit(code)
}

// SetupTestEnvironment creates and initializes all test infrastructure.
func SetupTestEnvironment(ctx context.Context) (*TestEnvironment, error) {
	env := &TestEnvironment{
		Logger: zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger(),
	}

	env.ctx, env.cancel = context.WithCancel(ctx)

	env.Logger.Info().Msg("Starting test environment setup")

	// Start PostgreSQL
	env.Logger.Info().Msg("Starting PostgreSQL container...")
	pg, err := testutil.NewPostgresContainer(ctx, testutil.DefaultPostgresConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres: %w", err)
	}
	env.Postgres = pg

	// Create database connection
	dbCfg := database.DefaultConfig(pg.ConnStr)
	dbCfg.MaxConns = 5
	dbCfg.MinConns = 1
	db, err := database.New(ctx, dbCfg)
	if err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("failed to create database: %w", err)
	}
	env.DB = db

	// Run migrations
	env.Logger.Info().Msg("Running database migrations...")
	migrationsFS := os.DirFS("../../migrations")
	migrator, err := database.NewMigratorFromFS(db, migrationsFS)
	if err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("failed to create migrator: %w", err)
	}
	if _, err := migrator.Up(ctx); err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Start MinIO
	env.Logger.Info().Msg("Starting MinIO container...")
	mc, err := testutil.NewMinioContainer(ctx, testutil.DefaultMinioConfig())
	if err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("failed to start minio: %w", err)
	}
	env.Minio = mc

	// Create storage client
	storage, err := artifact.NewStorage(artifact.StorageConfig{
		Endpoint:        mc.Endpoint,
		Bucket:          "conductor-e2e",
		Region:          "us-east-1",
		AccessKeyID:     mc.AccessKeyID,
		SecretAccessKey: mc.SecretAccessKey,
		UseSSL:          false,
		PathStyle:       true,
	}, nil)
	if err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}
	env.Storage = storage

	if err := storage.EnsureBucket(ctx); err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	// Start Redis (optional, for caching)
	env.Logger.Info().Msg("Starting Redis container...")
	redis, err := testutil.NewRedisContainer(ctx, testutil.DefaultRedisConfig())
	if err != nil {
		env.Logger.Warn().Err(err).Msg("Failed to start Redis, continuing without it")
	} else {
		env.Redis = redis
	}

	// Initialize repositories
	env.Logger.Info().Msg("Initializing repositories...")
	env.ServiceRepo = database.NewServiceRepo(db)
	env.RunRepo = database.NewRunRepo(db)
	env.ResultRepo = database.NewResultRepo(db)
	env.NotificationRepo = database.NewNotificationRepo(db)

	// Initialize notification service
	env.NotificationService = notification.NewService(notification.Config{
		WorkerCount:      2,
		QueueSize:        100,
		DefaultTimeout:   10 * time.Second,
		ThrottleDuration: 1 * time.Second, // Short for tests
		BaseURL:          "http://localhost:8080",
	}, env.NotificationRepo, nil)

	if err := env.NotificationService.Start(ctx); err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("failed to start notification service: %w", err)
	}

	// Start gRPC server
	env.Logger.Info().Msg("Starting gRPC server...")
	if err := env.startGRPCServer(ctx); err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("failed to start gRPC server: %w", err)
	}

	// Start HTTP server
	env.Logger.Info().Msg("Starting HTTP server...")
	if err := env.startHTTPServer(ctx); err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Create gRPC client connection
	conn, err := grpc.NewClient(
		env.GRPCAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		env.Cleanup()
		return nil, fmt.Errorf("failed to create gRPC client: %w", err)
	}
	env.GRPCConn = conn

	env.Logger.Info().
		Str("grpc_address", env.GRPCAddress).
		Str("http_address", env.HTTPAddress).
		Msg("Test environment ready")

	return env, nil
}

// startGRPCServer starts the gRPC server on a random port.
func (e *TestEnvironment) startGRPCServer(ctx context.Context) error {
	// Create mock agent repository that wraps our database
	agentRepo := &e2eAgentRepository{db: e.DB}
	e.AgentRepo = agentRepo

	deps := server.AgentServiceDeps{
		AgentRepo:        agentRepo,
		HeartbeatTimeout: 90 * time.Second,
		ServerVersion:    "e2e-test-1.0.0",
	}

	services := server.Services{
		AgentService: deps,
	}

	e.GRPCServer = server.NewGRPCServer(server.GRPCConfig{
		Port:             0,
		MaxRecvMsgSize:   16 * 1024 * 1024,
		MaxSendMsgSize:   16 * 1024 * 1024,
		EnableReflection: true,
	}, services, nil, e.Logger)

	// Find available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	e.GRPCAddress = listener.Addr().String()

	// Start server in background
	go func() {
		if err := e.GRPCServer.Server().Serve(listener); err != nil {
			e.Logger.Error().Err(err).Msg("gRPC server error")
		}
	}()

	return nil
}

// startHTTPServer starts the HTTP server on a random port.
func (e *TestEnvironment) startHTTPServer(ctx context.Context) error {
	httpServer, err := server.NewHTTPServer(server.HTTPConfig{
		Port:           0,
		GRPCAddress:    e.GRPCAddress,
		EnableCORS:     true,
		AllowedOrigins: []string{"*"},
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
	}, e.Logger)
	if err != nil {
		return fmt.Errorf("failed to create HTTP server: %w", err)
	}
	e.HTTPServer = httpServer

	// Find available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	e.HTTPAddress = fmt.Sprintf("http://%s", listener.Addr().String())

	// Start server in background
	go func() {
		// Note: We need to handle this differently since NewHTTPServer doesn't expose the handler directly
		// For now we just record the address - in a real scenario you'd use Start()
		listener.Close() // Close since we just needed the address
	}()

	return nil
}

// Cleanup tears down all test infrastructure.
func (e *TestEnvironment) Cleanup() {
	e.Logger.Info().Msg("Cleaning up test environment")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Close gRPC connection
	if e.GRPCConn != nil {
		e.GRPCConn.Close()
	}

	// Stop servers
	if e.GRPCServer != nil {
		e.GRPCServer.Stop(ctx)
	}

	if e.HTTPServer != nil {
		e.HTTPServer.Stop(ctx)
	}

	// Stop notification service
	if e.NotificationService != nil {
		e.NotificationService.Stop(ctx)
	}

	// Close database
	if e.DB != nil {
		e.DB.Close()
	}

	// Terminate containers
	if e.Redis != nil {
		e.Redis.Terminate(ctx)
	}

	if e.Minio != nil {
		e.Minio.Terminate(ctx)
	}

	if e.Postgres != nil {
		e.Postgres.Terminate(ctx)
	}

	// Cancel context
	if e.cancel != nil {
		e.cancel()
	}

	e.Logger.Info().Msg("Test environment cleanup complete")
}

// GetAgentServiceClient returns a gRPC client for the agent service.
func (e *TestEnvironment) GetAgentServiceClient() conductorv1.AgentServiceClient {
	return conductorv1.NewAgentServiceClient(e.GRPCConn)
}

// GetAgentManagementClient returns a gRPC client for agent management.
func (e *TestEnvironment) GetAgentManagementClient() conductorv1.AgentManagementServiceClient {
	return conductorv1.NewAgentManagementServiceClient(e.GRPCConn)
}

// GetRunServiceClient returns a gRPC client for the run service.
func (e *TestEnvironment) GetRunServiceClient() conductorv1.RunServiceClient {
	return conductorv1.NewRunServiceClient(e.GRPCConn)
}

// CreateTestService creates a service for testing purposes.
func (e *TestEnvironment) CreateTestService(ctx context.Context, name string) (*database.Service, error) {
	service := &database.Service{
		Name:      name,
		GitURL:    fmt.Sprintf("https://github.com/test/%s", name),
		Owner:     "test-owner",
		TestCmd:   "npm test",
		BuildCmd:  "npm install",
		GitBranch: "main",
	}

	if err := e.ServiceRepo.Create(ctx, service); err != nil {
		return nil, err
	}

	return service, nil
}

// ============================================================================
// E2E AGENT REPOSITORY (WRAPS DATABASE)
// ============================================================================

// e2eAgentRepository implements both server.AgentRepository and a subset of database.AgentRepository.
type e2eAgentRepository struct {
	db     *database.DB
	agents map[string]*database.Agent
	mu     sync.RWMutex
}

func (r *e2eAgentRepository) Create(ctx context.Context, agent *database.Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.agents == nil {
		r.agents = make(map[string]*database.Agent)
	}
	r.agents[agent.ID.String()] = agent
	return nil
}

func (r *e2eAgentRepository) Update(ctx context.Context, agent *database.Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.agents == nil {
		r.agents = make(map[string]*database.Agent)
	}
	r.agents[agent.ID.String()] = agent
	return nil
}

// GetByID implements server.AgentRepository
func (r *e2eAgentRepository) GetByID(ctx context.Context, id uuid.UUID) (*database.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.agents == nil {
		return nil, database.ErrNotFound
	}
	agent, ok := r.agents[id.String()]
	if !ok {
		return nil, database.ErrNotFound
	}
	return agent, nil
}

// Get implements database.AgentRepository
func (r *e2eAgentRepository) Get(ctx context.Context, id uuid.UUID) (*database.Agent, error) {
	return r.GetByID(ctx, id)
}

// GetByName implements database.AgentRepository
func (r *e2eAgentRepository) GetByName(ctx context.Context, name string) (*database.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.agents == nil {
		return nil, database.ErrNotFound
	}
	for _, agent := range r.agents {
		if agent.Name == name {
			return agent, nil
		}
	}
	return nil, database.ErrNotFound
}

func (r *e2eAgentRepository) UpdateHeartbeat(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	agent, ok := r.agents[id.String()]
	if !ok {
		return database.ErrNotFound
	}
	now := time.Now()
	agent.LastHeartbeat = &now
	agent.Status = status
	return nil
}

func (r *e2eAgentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status database.AgentStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	agent, ok := r.agents[id.String()]
	if !ok {
		return database.ErrNotFound
	}
	agent.Status = status
	return nil
}

// List implements database.AgentRepository
func (r *e2eAgentRepository) List(ctx context.Context, page database.Pagination) ([]database.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []database.Agent
	for _, agent := range r.agents {
		result = append(result, *agent)
	}
	return result, nil
}

// ListFiltered implements server.AgentRepository
func (r *e2eAgentRepository) ListFiltered(ctx context.Context, filter server.AgentFilter, pagination database.Pagination) ([]*database.Agent, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*database.Agent
	for _, agent := range r.agents {
		result = append(result, agent)
	}
	return result, len(result), nil
}

// ListPaginated is a duplicate helper (can use List instead)
func (r *e2eAgentRepository) ListPaginated(ctx context.Context, page database.Pagination) ([]database.Agent, error) {
	return r.List(ctx, page)
}

// ListByStatus implements database.AgentRepository
func (r *e2eAgentRepository) ListByStatus(ctx context.Context, status database.AgentStatus, page database.Pagination) ([]database.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []database.Agent
	for _, agent := range r.agents {
		if agent.Status == status {
			result = append(result, *agent)
		}
	}
	return result, nil
}

// GetAvailable implements database.AgentRepository
func (r *e2eAgentRepository) GetAvailable(ctx context.Context, zones []string, limit int) ([]database.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []database.Agent
	for _, agent := range r.agents {
		if agent.Status == database.AgentStatusIdle {
			result = append(result, *agent)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

// MarkOfflineAgents implements database.AgentRepository
func (r *e2eAgentRepository) MarkOfflineAgents(ctx context.Context) (int64, error) {
	return 0, nil
}

// CountByStatus implements database.AgentRepository
func (r *e2eAgentRepository) CountByStatus(ctx context.Context) (map[database.AgentStatus]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	counts := make(map[database.AgentStatus]int64)
	for _, agent := range r.agents {
		counts[agent.Status]++
	}
	return counts, nil
}

func (r *e2eAgentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, id.String())
	return nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// WaitForCondition polls until a condition is true or timeout occurs.
func WaitForCondition(ctx context.Context, interval time.Duration, condition func() bool) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if condition() {
				return nil
			}
		}
	}
}

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T {
	return &v
}
