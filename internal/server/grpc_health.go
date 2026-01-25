package server

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/timestamppb"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/database"
)

// HealthServiceDeps defines the dependencies for the health service.
type HealthServiceDeps struct {
	// DB is the database connection for health checks.
	DB HealthChecker
	// StartTime is when the server started.
	StartTime time.Time
	// Version is the server version info.
	Version VersionInfo
}

// HealthChecker defines the interface for health checking.
type HealthChecker interface {
	Health(ctx context.Context) error
	Stats() database.HealthStats
}

// VersionInfo contains version information about the server.
type VersionInfo struct {
	Version   string
	Commit    string
	BuildTime time.Time
	GoVersion string
	Platform  string
}

// HealthServiceServer implements the HealthService gRPC service.
type HealthServiceServer struct {
	conductorv1.UnimplementedHealthServiceServer

	deps   HealthServiceDeps
	logger zerolog.Logger
}

// NewHealthServiceServer creates a new health service server.
func NewHealthServiceServer(deps HealthServiceDeps, logger zerolog.Logger) *HealthServiceServer {
	return &HealthServiceServer{
		deps:   deps,
		logger: logger.With().Str("service", "HealthService").Logger(),
	}
}

// Check performs a health check on the control plane and its dependencies.
func (s *HealthServiceServer) Check(ctx context.Context, req *conductorv1.HealthCheckRequest) (*conductorv1.HealthCheckResponse, error) {
	resp := &conductorv1.HealthCheckResponse{
		Status:        conductorv1.HealthStatus_HEALTH_STATUS_HEALTHY,
		Message:       "all systems operational",
		Timestamp:     timestamppb.Now(),
		UptimeSeconds: int64(time.Since(s.deps.StartTime).Seconds()),
		Version: &conductorv1.VersionInfo{
			Version:   s.deps.Version.Version,
			Commit:    s.deps.Version.Commit,
			GoVersion: s.deps.Version.GoVersion,
			Platform:  s.deps.Version.Platform,
		},
	}

	if !s.deps.Version.BuildTime.IsZero() {
		resp.Version.BuildTime = timestamppb.New(s.deps.Version.BuildTime)
	}

	// Check components if requested
	if req.IncludeComponents || len(req.Components) > 0 {
		components := s.checkComponents(ctx, req.Components)
		resp.Components = components

		// Update overall status based on component health
		for _, comp := range components {
			if comp.Critical {
				if comp.Status == conductorv1.HealthStatus_HEALTH_STATUS_UNHEALTHY {
					resp.Status = conductorv1.HealthStatus_HEALTH_STATUS_UNHEALTHY
					resp.Message = "critical component unhealthy: " + comp.Name
					break
				} else if comp.Status == conductorv1.HealthStatus_HEALTH_STATUS_DEGRADED {
					resp.Status = conductorv1.HealthStatus_HEALTH_STATUS_DEGRADED
					resp.Message = "critical component degraded: " + comp.Name
				}
			}
		}
	}

	return resp, nil
}

// CheckLiveness returns whether the service is alive.
func (s *HealthServiceServer) CheckLiveness(ctx context.Context, req *conductorv1.LivenessRequest) (*conductorv1.LivenessResponse, error) {
	return &conductorv1.LivenessResponse{
		Alive:     true,
		Timestamp: timestamppb.Now(),
	}, nil
}

// CheckReadiness returns whether the service is ready to accept traffic.
func (s *HealthServiceServer) CheckReadiness(ctx context.Context, req *conductorv1.ReadinessRequest) (*conductorv1.ReadinessResponse, error) {
	notReady := []string{}

	// Check database connection
	if err := s.deps.DB.Health(ctx); err != nil {
		notReady = append(notReady, "database")
	}

	if len(notReady) > 0 {
		return &conductorv1.ReadinessResponse{
			Ready:              false,
			Reason:             "required components not ready",
			NotReadyComponents: notReady,
			Timestamp:          timestamppb.Now(),
		}, nil
	}

	return &conductorv1.ReadinessResponse{
		Ready:     true,
		Timestamp: timestamppb.Now(),
	}, nil
}

// checkComponents checks the health of individual components.
func (s *HealthServiceServer) checkComponents(ctx context.Context, requestedComponents []string) []*conductorv1.ComponentHealth {
	// Define all components
	allComponents := map[string]func(context.Context) *conductorv1.ComponentHealth{
		"database": s.checkDatabase,
		// TODO: Add more components: redis, artifact_storage, etc.
	}

	// Filter components if specific ones were requested
	componentsToCheck := allComponents
	if len(requestedComponents) > 0 {
		componentsToCheck = make(map[string]func(context.Context) *conductorv1.ComponentHealth)
		for _, name := range requestedComponents {
			if checkFn, ok := allComponents[name]; ok {
				componentsToCheck[name] = checkFn
			}
		}
	}

	results := make([]*conductorv1.ComponentHealth, 0, len(componentsToCheck))
	for name, checkFn := range componentsToCheck {
		result := checkFn(ctx)
		result.Name = name
		result.LastChecked = timestamppb.Now()
		results = append(results, result)
	}

	return results
}

// checkDatabase performs a health check on the database.
func (s *HealthServiceServer) checkDatabase(ctx context.Context) *conductorv1.ComponentHealth {
	start := time.Now()

	err := s.deps.DB.Health(ctx)
	latency := time.Since(start)

	component := &conductorv1.ComponentHealth{
		Name:      "database",
		LatencyMs: latency.Milliseconds(),
		Critical:  true,
		Details:   make(map[string]string),
	}

	if err != nil {
		component.Status = conductorv1.HealthStatus_HEALTH_STATUS_UNHEALTHY
		component.Message = err.Error()
		return component
	}

	// Get stats
	stats := s.deps.DB.Stats()
	component.Details["total_conns"] = itoa(int(stats.TotalConns))
	component.Details["idle_conns"] = itoa(int(stats.IdleConns))
	component.Details["acquired_conns"] = itoa(int(stats.AcquiredConns))
	component.Details["max_conns"] = itoa(int(stats.MaxConns))

	// Check if connection pool is saturated
	if stats.AcquiredConns >= stats.MaxConns-1 {
		component.Status = conductorv1.HealthStatus_HEALTH_STATUS_DEGRADED
		component.Message = "connection pool near capacity"
	} else {
		component.Status = conductorv1.HealthStatus_HEALTH_STATUS_HEALTHY
		component.Message = "database responding normally"
	}

	return component
}

// itoa converts an int to a string (simple helper to avoid importing strconv).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	neg := false
	if i < 0 {
		neg = true
		i = -i
	}

	b := make([]byte, 20)
	j := len(b)

	for i > 0 {
		j--
		b[j] = byte('0' + i%10)
		i /= 10
	}

	if neg {
		j--
		b[j] = '-'
	}

	return string(b[j:])
}
