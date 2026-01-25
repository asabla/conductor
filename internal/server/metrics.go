package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/conductor/conductor/pkg/metrics"
)

// MetricsServerConfig holds configuration for the metrics server.
type MetricsServerConfig struct {
	// Port is the port to listen on for metrics.
	Port int
	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration
	// Path is the path for the metrics endpoint (default: /metrics).
	Path string
}

// DefaultMetricsServerConfig returns sensible defaults for metrics server configuration.
func DefaultMetricsServerConfig() MetricsServerConfig {
	return MetricsServerConfig{
		Port:         9091,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Path:         "/metrics",
	}
}

// MetricsServer serves Prometheus metrics over HTTP.
type MetricsServer struct {
	config  MetricsServerConfig
	metrics *metrics.Metrics
	server  *http.Server
	logger  zerolog.Logger
}

// NewMetricsServer creates a new metrics server.
func NewMetricsServer(cfg MetricsServerConfig, m *metrics.Metrics, logger zerolog.Logger) *MetricsServer {
	return &MetricsServer{
		config:  cfg,
		metrics: m,
		logger:  logger.With().Str("component", "metrics_server").Logger(),
	}
}

// Start starts the metrics HTTP server.
func (s *MetricsServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Mount metrics handler
	path := s.config.Path
	if path == "" {
		path = "/metrics"
	}
	mux.Handle(path, s.metrics.Handler())

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf(":%d", s.config.Port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	s.logger.Info().
		Str("address", addr).
		Str("path", path).
		Msg("starting metrics server")

	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		s.logger.Info().Msg("context cancelled, stopping metrics server")
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("metrics server error: %w", err)
		}
		return nil
	}
}

// Stop gracefully shuts down the metrics server.
func (s *MetricsServer) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	s.logger.Info().Msg("stopping metrics server")

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Warn().Err(err).Msg("metrics server shutdown error")
		return err
	}

	s.logger.Info().Msg("metrics server stopped")
	return nil
}

// MetricsMiddleware returns an HTTP middleware that records request metrics.
func MetricsMiddleware(m *metrics.ControlPlaneMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &metricsResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Call next handler
			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			path := normalizePath(r.URL.Path)
			status := strconv.Itoa(wrapped.statusCode)

			m.RecordAPIRequest(r.Method, path, status, duration)
		})
	}
}

// metricsResponseWriter wraps http.ResponseWriter to capture the status code.
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code.
func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RecordAPIRequest is a convenience method to record an API request.
func RecordAPIRequest(m *metrics.ControlPlaneMetrics, method, path string, statusCode int, duration time.Duration) {
	if m == nil {
		return
	}
	normalizedPath := normalizePath(path)
	status := strconv.Itoa(statusCode)
	m.RecordAPIRequest(method, normalizedPath, status, duration.Seconds())
}

// RecordGRPCRequest is a convenience method to record a gRPC request.
func RecordGRPCRequest(m *metrics.ControlPlaneMetrics, method string, code string, duration time.Duration) {
	if m == nil {
		return
	}
	m.RecordGRPCRequest(method, code, duration.Seconds())
}

// normalizePath normalizes URL paths to reduce cardinality.
// It replaces UUIDs and numeric IDs with placeholders.
func normalizePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		// Check if this part looks like a UUID
		if isUUID(part) {
			parts[i] = ":id"
			continue
		}
		// Check if this part is a numeric ID
		if isNumericID(part) {
			parts[i] = ":id"
			continue
		}
	}
	return strings.Join(parts, "/")
}

// isUUID checks if a string looks like a UUID.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	// Check format: 8-4-4-4-12
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !isHexDigit(byte(c)) {
			return false
		}
	}
	return true
}

// isHexDigit checks if a byte is a hexadecimal digit.
func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// isNumericID checks if a string is a numeric ID.
func isNumericID(s string) bool {
	if len(s) == 0 || len(s) > 20 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// GRPCMetricsInterceptorConfig configures the gRPC metrics interceptor.
type GRPCMetricsInterceptorConfig struct {
	// Metrics is the control plane metrics instance.
	Metrics *metrics.ControlPlaneMetrics
	// ExcludeMethods is a list of methods to exclude from metrics.
	ExcludeMethods []string
}
