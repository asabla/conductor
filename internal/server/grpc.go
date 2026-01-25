// Package server provides gRPC and HTTP server implementations for the Conductor control plane.
package server

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/pkg/metrics"
	"github.com/conductor/conductor/pkg/tracing"
)

// GRPCConfig holds configuration for the gRPC server.
type GRPCConfig struct {
	// Port is the port to listen on.
	Port int
	// MaxRecvMsgSize is the maximum message size in bytes the server can receive.
	MaxRecvMsgSize int
	// MaxSendMsgSize is the maximum message size in bytes the server can send.
	MaxSendMsgSize int
	// EnableReflection enables gRPC server reflection for debugging.
	EnableReflection bool
	// EnableTracing enables OpenTelemetry tracing for gRPC calls.
	EnableTracing bool
	// Metrics is the control plane metrics instance for recording gRPC metrics.
	Metrics *metrics.ControlPlaneMetrics
}

// DefaultGRPCConfig returns sensible defaults for gRPC server configuration.
func DefaultGRPCConfig() GRPCConfig {
	return GRPCConfig{
		Port:             9090,
		MaxRecvMsgSize:   16 * 1024 * 1024, // 16MB
		MaxSendMsgSize:   16 * 1024 * 1024, // 16MB
		EnableReflection: true,
		EnableTracing:    false,
		Metrics:          nil,
	}
}

// Services holds all service dependencies required by the gRPC server.
type Services struct {
	AgentService        AgentServiceDeps
	RunService          RunServiceDeps
	ServiceService      ServiceRegistryDeps
	ResultService       ResultServiceDeps
	HealthService       HealthServiceDeps
	NotificationService NotificationServiceDeps
}

// GRPCServer wraps a gRPC server with Conductor services.
type GRPCServer struct {
	config   GRPCConfig
	server   *grpc.Server
	listener net.Listener
	logger   zerolog.Logger

	// Service implementations
	agentService          *AgentServiceServer
	agentManagementServer *AgentManagementServer
	runServer             *RunServiceServer
	serviceRegistryServer *ServiceRegistryServer
	resultServer          *ResultServiceServer
	healthServer          *HealthServiceServer
	notificationServer    *NotificationServiceServer

	// gRPC health server
	grpcHealth *health.Server
}

// NewGRPCServer creates a new gRPC server with the provided configuration and services.
func NewGRPCServer(cfg GRPCConfig, services Services, jwtValidator *JWTValidator, logger zerolog.Logger) *GRPCServer {
	// Create interceptors
	loggingInterceptor := NewLoggingInterceptor(logger)
	recoveryInterceptor := NewRecoveryInterceptor(logger)
	authInterceptor := NewAuthInterceptor(jwtValidator, logger)

	// Build unary interceptor chain
	// Order: recovery -> tracing -> metrics -> logging -> auth
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		recoveryInterceptor.Unary(),
	}

	// Add tracing interceptor if enabled
	if cfg.EnableTracing {
		unaryInterceptors = append(unaryInterceptors, tracing.UnaryServerInterceptor())
	}

	// Add metrics interceptor if metrics are provided
	if cfg.Metrics != nil {
		unaryInterceptors = append(unaryInterceptors, newMetricsUnaryInterceptor(cfg.Metrics))
	}

	unaryInterceptors = append(unaryInterceptors,
		loggingInterceptor.Unary(),
		authInterceptor.Unary(),
	)

	// Build stream interceptor chain
	// Order: recovery -> tracing -> metrics -> logging -> auth
	streamInterceptors := []grpc.StreamServerInterceptor{
		recoveryInterceptor.Stream(),
	}

	// Add tracing interceptor if enabled
	if cfg.EnableTracing {
		streamInterceptors = append(streamInterceptors, tracing.StreamServerInterceptor())
	}

	// Add metrics interceptor if metrics are provided
	if cfg.Metrics != nil {
		streamInterceptors = append(streamInterceptors, newMetricsStreamInterceptor(cfg.Metrics))
	}

	streamInterceptors = append(streamInterceptors,
		loggingInterceptor.Stream(),
		authInterceptor.Stream(),
	)

	// Build server options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(cfg.MaxSendMsgSize),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 5 * time.Minute,
			Time:                  5 * time.Minute,
			Timeout:               20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             1 * time.Minute,
			PermitWithoutStream: true,
		}),
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	}

	server := grpc.NewServer(opts...)

	// Create service implementations
	agentService := NewAgentServiceServer(services.AgentService, logger)
	agentMgmtServer := NewAgentManagementServer(services.AgentService, logger)
	runServer := NewRunServiceServer(services.RunService, logger)
	serviceRegistryServer := NewServiceRegistryServer(services.ServiceService, logger)
	resultServer := NewResultServiceServer(services.ResultService, logger)
	healthServer := NewHealthServiceServer(services.HealthService, logger)
	notificationServer := NewNotificationServiceServer(services.NotificationService, logger)

	// Register services
	conductorv1.RegisterAgentServiceServer(server, agentService)
	conductorv1.RegisterAgentManagementServiceServer(server, agentMgmtServer)
	conductorv1.RegisterRunServiceServer(server, runServer)
	conductorv1.RegisterServiceRegistryServiceServer(server, serviceRegistryServer)
	conductorv1.RegisterResultServiceServer(server, resultServer)
	conductorv1.RegisterHealthServiceServer(server, healthServer)
	conductorv1.RegisterNotificationServiceServer(server, notificationServer)

	// Register gRPC health service
	grpcHealth := health.NewServer()
	healthpb.RegisterHealthServer(server, grpcHealth)

	// Enable reflection if configured
	if cfg.EnableReflection {
		reflection.Register(server)
	}

	return &GRPCServer{
		config:                cfg,
		server:                server,
		logger:                logger.With().Str("component", "grpc_server").Logger(),
		agentService:          agentService,
		agentManagementServer: agentMgmtServer,
		runServer:             runServer,
		serviceRegistryServer: serviceRegistryServer,
		resultServer:          resultServer,
		healthServer:          healthServer,
		notificationServer:    notificationServer,
		grpcHealth:            grpcHealth,
	}
}

// Start starts the gRPC server and blocks until the context is cancelled.
func (s *GRPCServer) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	// Set all services as serving
	s.grpcHealth.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	s.grpcHealth.SetServingStatus("conductor.v1.AgentService", healthpb.HealthCheckResponse_SERVING)
	s.grpcHealth.SetServingStatus("conductor.v1.AgentManagementService", healthpb.HealthCheckResponse_SERVING)
	s.grpcHealth.SetServingStatus("conductor.v1.RunService", healthpb.HealthCheckResponse_SERVING)
	s.grpcHealth.SetServingStatus("conductor.v1.ServiceRegistryService", healthpb.HealthCheckResponse_SERVING)
	s.grpcHealth.SetServingStatus("conductor.v1.ResultService", healthpb.HealthCheckResponse_SERVING)
	s.grpcHealth.SetServingStatus("conductor.v1.HealthService", healthpb.HealthCheckResponse_SERVING)
	s.grpcHealth.SetServingStatus("conductor.v1.NotificationService", healthpb.HealthCheckResponse_SERVING)

	s.logger.Info().
		Str("address", addr).
		Bool("reflection", s.config.EnableReflection).
		Msg("starting gRPC server")

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := s.server.Serve(listener); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info().Msg("context cancelled, stopping gRPC server")
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("gRPC server error: %w", err)
		}
		return nil
	}
}

// Stop gracefully stops the gRPC server.
func (s *GRPCServer) Stop(ctx context.Context) error {
	s.logger.Info().Msg("stopping gRPC server")

	// Set all services as not serving
	s.grpcHealth.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)

	// Create a channel to signal when graceful stop is complete
	done := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(done)
	}()

	// Wait for graceful stop or context timeout
	select {
	case <-done:
		s.logger.Info().Msg("gRPC server stopped gracefully")
		return nil
	case <-ctx.Done():
		s.logger.Warn().Msg("graceful stop timeout, forcing stop")
		s.server.Stop()
		return ctx.Err()
	}
}

// Address returns the address the server is listening on.
// Returns empty string if server is not started.
func (s *GRPCServer) Address() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Server returns the underlying gRPC server.
func (s *GRPCServer) Server() *grpc.Server {
	return s.server
}

// newMetricsUnaryInterceptor creates a unary interceptor that records gRPC metrics.
func newMetricsUnaryInterceptor(m *metrics.ControlPlaneMetrics) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		// Extract method name (e.g., "/conductor.v1.RunService/CreateRun" -> "CreateRun")
		method := info.FullMethod
		if idx := len(method) - 1; idx >= 0 {
			for i := idx; i >= 0; i-- {
				if method[i] == '/' {
					method = method[i+1:]
					break
				}
			}
		}

		// Determine status
		status := "ok"
		if err != nil {
			status = "error"
		}

		m.RecordGRPCRequest(method, status, duration.Seconds())
		return resp, err
	}
}

// newMetricsStreamInterceptor creates a stream interceptor that records gRPC metrics.
func newMetricsStreamInterceptor(m *metrics.ControlPlaneMetrics) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		// Extract method name
		method := info.FullMethod
		if idx := len(method) - 1; idx >= 0 {
			for i := idx; i >= 0; i-- {
				if method[i] == '/' {
					method = method[i+1:]
					break
				}
			}
		}

		// Increment active stream count
		m.IncrementGRPCStream(method)
		defer m.DecrementGRPCStream(method)

		err := handler(srv, ss)
		duration := time.Since(start)

		// Determine status
		status := "ok"
		if err != nil {
			status = "error"
		}

		m.RecordGRPCStream(method, status, duration.Seconds())
		return err
	}
}
