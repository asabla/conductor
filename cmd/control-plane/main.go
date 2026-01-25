// Package main is the entry point for the Conductor control plane.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/conductor/conductor/internal/artifact"
	"github.com/conductor/conductor/internal/config"
	"github.com/conductor/conductor/internal/database"
	"github.com/conductor/conductor/internal/git"
	"github.com/conductor/conductor/internal/notification"
	"github.com/conductor/conductor/internal/server"
	"github.com/conductor/conductor/internal/websocket"
	"github.com/conductor/conductor/internal/wire"
	"github.com/conductor/conductor/pkg/metrics"
	"github.com/conductor/conductor/pkg/tracing"
)

// Build information, set by ldflags during build.
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	// Initialize logger
	logger := setupLogger()
	log.Logger = logger

	logger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("build_time", buildTime).
		Str("go_version", runtime.Version()).
		Msg("starting Conductor control plane")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load configuration")
	}

	// Create root context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Initialize metrics
	appMetrics := metrics.NewControlPlaneMetrics()
	logger.Info().Msg("metrics initialized")

	// Initialize tracing
	var tracer *tracing.Tracer
	if cfg.Observability.TracingEnabled && cfg.Observability.TracingEndpoint != "" {
		tracingCfg := tracing.Config{
			ServiceName:    "conductor-control-plane",
			ServiceVersion: version,
			Endpoint:       cfg.Observability.TracingEndpoint,
			Insecure:       cfg.Observability.TracingInsecure,
			SampleRate:     cfg.Observability.TracingSampleRate,
			Environment:    cfg.Observability.Environment,
			Enabled:        true,
		}
		tracer, err = tracing.InitTracer(tracingCfg)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to initialize tracing - continuing without tracing")
		} else {
			logger.Info().
				Str("endpoint", cfg.Observability.TracingEndpoint).
				Float64("sample_rate", cfg.Observability.TracingSampleRate).
				Msg("tracing initialized")
		}
	} else {
		logger.Info().Msg("tracing disabled")
	}

	// Initialize database
	logger.Info().Msg("connecting to database")
	db, err := database.New(ctx, database.Config{
		URL:               cfg.Database.URL,
		MaxConns:          int32(cfg.Database.MaxOpenConns),
		MinConns:          int32(cfg.Database.MaxIdleConns),
		MaxConnLifetime:   cfg.Database.ConnMaxLifetime,
		MaxConnIdleTime:   cfg.Database.ConnMaxIdleTime,
		HealthCheckPeriod: time.Minute,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	logger.Info().Msg("database connection established")

	// Create repositories
	repos := database.NewRepositories(db)

	// Create adapted repositories for server interfaces
	agentRepo := wire.NewAgentRepositoryAdapter(repos.Agents)
	runRepo := wire.NewRunRepositoryAdapter(repos.Runs)
	serviceRepo := wire.NewServiceRepositoryAdapter(repos.Services)
	testDefRepo := wire.NewTestDefinitionRepositoryAdapter(repos.TestDefinitions)
	resultRepo := wire.NewResultRepositoryAdapter(repos.Results)
	artifactRepo := wire.NewArtifactRepositoryAdapter(repos.Artifacts)

	// Create scheduler and git syncer
	// TODO: Replace with real scheduler implementation
	scheduler := &wire.NoopScheduler{}

	// Create git syncer (if configured)
	gitSyncer, err := createGitSyncer(cfg, repos.TestDefinitions, logger)
	if err != nil {
		logger.Warn().Err(err).Msg("git syncer not available - sync functionality disabled")
		gitSyncer = &wire.NoopGitSyncer{}
	}

	// Create artifact storage
	artifactStorage, artifactStorageAdapter, err := createArtifactStorage(cfg, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create artifact storage")
	}

	if cfg.Storage.CleanupEnabled {
		cleanupLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})).With("component", "artifact_cleanup")
		cleanupService := artifact.NewCleanupService(
			repos.Artifacts,
			artifactStorage,
			artifact.CleanupConfig{
				Interval:  cfg.Storage.CleanupInterval,
				Retention: cfg.Storage.RetentionPeriod,
				BatchSize: cfg.Storage.CleanupBatchSize,
			},
			cleanupLogger,
		)
		cleanupService.Start(ctx)
	}

	// Create JWT validator
	jwtValidator := server.NewJWTValidator(cfg.Auth.JWTSecret)

	// Create WebSocket hub for real-time updates
	wsHub := websocket.NewHub(logger)

	// Create WebSocket event publisher
	// The publisher is available for services to broadcast real-time updates.
	// It can be injected into services that need to publish events.
	wsPublisher := websocket.NewPublisher(wsHub, logger)
	_ = wsPublisher // Used by services for real-time event publishing

	// Create notification service
	notificationLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("component", "notification_service")

	notificationConfig := notification.DefaultConfig()
	// BaseURL can be set via environment or config if needed for notification links
	// notificationConfig.BaseURL = "http://localhost:8080"
	notificationConfig.Email = notification.EmailSettings{
		SMTPHost:    cfg.Notifications.Email.SMTPHost,
		SMTPPort:    cfg.Notifications.Email.SMTPPort,
		Username:    cfg.Notifications.Email.Username,
		Password:    cfg.Notifications.Email.Password,
		FromAddress: cfg.Notifications.Email.FromAddress,
		FromName:    cfg.Notifications.Email.FromName,
		UseTLS:      cfg.Notifications.Email.UseTLS,
		SkipVerify:  cfg.Notifications.Email.SkipVerify,
		ConnTimeout: cfg.Notifications.Email.ConnTimeout,
	}
	notificationService := notification.NewService(notificationConfig, repos.Notifications, notificationLogger)

	// Create service dependencies with real repositories
	services := server.Services{
		AgentService: server.AgentServiceDeps{
			AgentRepo:        agentRepo,
			RunRepo:          runRepo,
			Scheduler:        scheduler,
			HeartbeatTimeout: cfg.Agent.HeartbeatTimeout,
			ServerVersion:    version,
		},
		RunService: server.RunServiceDeps{
			RunRepo:     runRepo,
			ServiceRepo: serviceRepo,
			Scheduler:   scheduler,
		},
		ServiceService: server.ServiceRegistryDeps{
			ServiceRepo: serviceRepo,
			TestRepo:    testDefRepo,
			GitSyncer:   gitSyncer,
		},
		ResultService: server.ResultServiceDeps{
			ResultRepo:      resultRepo,
			ArtifactRepo:    artifactRepo,
			RunRepo:         runRepo,
			ArtifactStorage: artifactStorageAdapter,
		},
		HealthService: server.HealthServiceDeps{
			DB:        db,
			StartTime: time.Now(),
			Version: server.VersionInfo{
				Version:   version,
				Commit:    commit,
				GoVersion: runtime.Version(),
				Platform:  runtime.GOOS + "/" + runtime.GOARCH,
			},
		},
		NotificationService: server.NotificationServiceDeps{
			Repo:                repos.Notifications,
			NotificationService: notificationService,
		},
	}

	// Parse build time
	if buildTime != "unknown" {
		if t, err := time.Parse(time.RFC3339, buildTime); err == nil {
			services.HealthService.Version.BuildTime = t
		}
	}

	// Create gRPC server
	grpcConfig := server.GRPCConfig{
		Port:             cfg.Server.GRPCPort,
		MaxRecvMsgSize:   16 * 1024 * 1024,
		MaxSendMsgSize:   16 * 1024 * 1024,
		EnableReflection: true,
		EnableTracing:    tracer != nil,
		Metrics:          appMetrics.ControlPlane,
	}
	grpcServer := server.NewGRPCServer(grpcConfig, services, jwtValidator, logger)

	// Create HTTP server with WebSocket support
	httpConfig := server.HTTPConfig{
		Port:           cfg.Server.HTTPPort,
		GRPCAddress:    fmt.Sprintf("localhost:%d", cfg.Server.GRPCPort),
		EnableCORS:     true,
		AllowedOrigins: []string{"*"},
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		WebSocketPath:  "/ws",
		EnableTracing:  tracer != nil,
		Metrics:        appMetrics.ControlPlane,
	}

	// Create WebSocket authenticator that wraps JWT validator
	wsAuth := &jwtWebSocketAuth{validator: jwtValidator}

	httpServer, err := server.NewHTTPServerWithWebSocket(httpConfig, wsHub, wsAuth, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create HTTP server")
	}

	// Create and configure webhook handler if enabled
	if cfg.WebhooksEnabled() {
		webhookCfg := server.WebhookConfig{
			GithubSecret:    cfg.Git.WebhookSecret,
			GitlabSecret:    cfg.Git.GitLabWebhookSecret,
			BitbucketSecret: cfg.Git.BitbucketWebhookSecret,
			BaseURL:         cfg.Webhook.BaseURL,
		}

		// Create service repository adapter for webhook handler
		webhookServiceRepo := &webhookServiceRepoAdapter{repo: repos.Services}

		webhookHandler := server.NewWebhookHandler(
			webhookCfg,
			webhookServiceRepo,
			scheduler, // RunScheduler - currently NoopScheduler
			logger,
		)
		httpServer.SetWebhookHandler(webhookHandler)

		logger.Info().
			Bool("github_secret_set", cfg.Git.WebhookSecret != "").
			Bool("gitlab_secret_set", cfg.Git.GitLabWebhookSecret != "").
			Bool("bitbucket_secret_set", cfg.Git.BitbucketWebhookSecret != "").
			Msg("webhook handler configured")
	}

	// Create metrics server
	metricsServerCfg := server.MetricsServerConfig{
		Port:         cfg.Server.MetricsPort,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Path:         "/metrics",
	}
	metricsServer := server.NewMetricsServer(metricsServerCfg, appMetrics, logger)

	// Channel to collect errors from servers
	errCh := make(chan error, 5)

	// Start WebSocket hub
	go func() {
		wsHub.Run(ctx)
	}()

	// Start notification service
	if err := notificationService.Start(ctx); err != nil {
		logger.Fatal().Err(err).Msg("failed to start notification service")
	}
	logger.Info().Msg("notification service started")

	// Start gRPC server
	go func() {
		if err := grpcServer.Start(ctx); err != nil {
			errCh <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// Wait for gRPC to start before starting HTTP gateway
	time.Sleep(100 * time.Millisecond)

	// Start HTTP server
	go func() {
		if err := httpServer.Start(ctx); err != nil {
			errCh <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Start metrics server
	go func() {
		if err := metricsServer.Start(ctx); err != nil {
			errCh <- fmt.Errorf("metrics server error: %w", err)
		}
	}()

	logger.Info().
		Int("grpc_port", cfg.Server.GRPCPort).
		Int("http_port", cfg.Server.HTTPPort).
		Int("metrics_port", cfg.Server.MetricsPort).
		Msg("Conductor control plane started")

	// Wait for shutdown signal or error
	select {
	case sig := <-sigCh:
		logger.Info().Str("signal", sig.String()).Msg("received shutdown signal")
	case err := <-errCh:
		logger.Error().Err(err).Msg("server error")
	}

	// Initiate graceful shutdown
	logger.Info().Msg("initiating graceful shutdown")
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// Shutdown servers
	var shutdownErr error

	// Shutdown tracer first (to flush any pending spans)
	if tracer != nil {
		if err := tracer.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("tracer shutdown error")
			shutdownErr = err
		} else {
			logger.Info().Msg("tracer shutdown complete")
		}
	}

	// Shutdown metrics server
	if err := metricsServer.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("metrics server shutdown error")
		shutdownErr = err
	}

	// Shutdown HTTP server
	if err := httpServer.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("HTTP server shutdown error")
		shutdownErr = err
	}

	// Shutdown notification service
	if err := notificationService.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("notification service shutdown error")
		shutdownErr = err
	}

	// Shutdown gRPC server
	if err := grpcServer.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("gRPC server shutdown error")
		shutdownErr = err
	}

	if shutdownErr != nil {
		logger.Error().Msg("shutdown completed with errors")
		os.Exit(1)
	}

	logger.Info().Msg("shutdown completed successfully")
}

// setupLogger initializes the zerolog logger.
func setupLogger() zerolog.Logger {
	// Default to JSON logging for production
	format := os.Getenv("CONDUCTOR_LOG_FORMAT")
	level := os.Getenv("CONDUCTOR_LOG_LEVEL")

	// Set log level
	var logLevel zerolog.Level
	switch level {
	case "debug":
		logLevel = zerolog.DebugLevel
	case "warn":
		logLevel = zerolog.WarnLevel
	case "error":
		logLevel = zerolog.ErrorLevel
	default:
		logLevel = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(logLevel)

	// Set output format
	var logger zerolog.Logger
	if format == "console" {
		logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		})
	} else {
		logger = zerolog.New(os.Stdout)
	}

	return logger.With().
		Timestamp().
		Str("service", "control-plane").
		Logger()
}

// createArtifactStorage creates the artifact storage backend.
func createArtifactStorage(cfg *config.Config, logger zerolog.Logger) (*artifact.Storage, server.ArtifactStorage, error) {
	// Create slog adapter for artifact storage
	slogHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slogLogger := slog.New(slogHandler).With("component", "artifact_storage")

	// Create S3/MinIO storage
	storageCfg := artifact.StorageConfig{
		Endpoint:        cfg.Storage.Endpoint,
		Bucket:          cfg.Storage.Bucket,
		Region:          cfg.Storage.Region,
		AccessKeyID:     cfg.Storage.AccessKeyID,
		SecretAccessKey: cfg.Storage.SecretAccessKey,
		UseSSL:          cfg.Storage.UseSSL,
		PathStyle:       cfg.Storage.PathStyle,
	}

	storage, err := artifact.NewStorage(storageCfg, slogLogger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	// Ensure bucket exists
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := storage.EnsureBucket(ctx); err != nil {
		logger.Warn().Err(err).Msg("failed to ensure bucket exists - artifact storage may not work")
	}

	// Check storage health
	if err := storage.HealthCheck(ctx); err != nil {
		logger.Warn().Err(err).Msg("artifact storage health check failed")
	} else {
		logger.Info().
			Str("bucket", cfg.Storage.Bucket).
			Str("endpoint", cfg.Storage.Endpoint).
			Msg("artifact storage initialized")
	}

	return storage, wire.NewArtifactStorageAdapter(storage), nil
}

// createGitSyncer creates the git syncer if configured.
func createGitSyncer(cfg *config.Config, testRepo database.TestDefinitionRepository, logger zerolog.Logger) (server.GitSyncer, error) {
	if !cfg.GitEnabled() {
		logger.Info().Msg("git provider not configured - using noop syncer")
		return nil, fmt.Errorf("git token not configured")
	}

	// Create slog adapter for git package
	slogHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slogLogger := slog.New(slogHandler).With("component", "git_syncer")

	// Create git provider
	gitCfg := git.Config{
		Provider: cfg.Git.Provider,
		Token:    cfg.Git.Token,
		BaseURL:  cfg.Git.BaseURL,
	}

	provider, err := git.NewProvider(gitCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create git provider: %w", err)
	}

	// Create syncer
	syncer := git.NewSyncer(provider, testRepo, slogLogger)

	logger.Info().
		Str("provider", cfg.Git.Provider).
		Msg("git syncer initialized")

	return wire.NewGitSyncerAdapter(syncer), nil
}

// jwtWebSocketAuth adapts the JWT validator to the WebSocket authenticator interface.
type jwtWebSocketAuth struct {
	validator *server.JWTValidator
}

// Authenticate validates the token from the request.
func (a *jwtWebSocketAuth) Authenticate(r *http.Request) (string, map[string]interface{}, error) {
	// Try Authorization header first
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		const bearerPrefix = "Bearer "
		if len(authHeader) > len(bearerPrefix) && authHeader[:len(bearerPrefix)] == bearerPrefix {
			token := authHeader[len(bearerPrefix):]
			claims, err := a.validator.Validate(token)
			if err != nil {
				return "", nil, err
			}
			return claims.UserID, map[string]interface{}{
				"email": claims.Email,
				"name":  claims.Name,
				"roles": claims.Roles,
			}, nil
		}
	}

	// Fall back to query parameter (useful for browser WebSocket connections)
	token := r.URL.Query().Get("token")
	if token != "" {
		claims, err := a.validator.Validate(token)
		if err != nil {
			return "", nil, err
		}
		return claims.UserID, map[string]interface{}{
			"email": claims.Email,
			"name":  claims.Name,
			"roles": claims.Roles,
		}, nil
	}

	// No token provided - allow anonymous connection
	return "", nil, nil
}

// webhookServiceRepoAdapter adapts database.ServiceRepository to server.WebhookServiceRepository.
type webhookServiceRepoAdapter struct {
	repo database.ServiceRepository
}

func (a *webhookServiceRepoAdapter) List(ctx context.Context, page database.Pagination) ([]database.Service, error) {
	return a.repo.List(ctx, page)
}
