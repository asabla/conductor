package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/websocket"
	"github.com/conductor/conductor/pkg/metrics"
	"github.com/conductor/conductor/pkg/tracing"
)

// HTTPConfig holds configuration for the HTTP server.
type HTTPConfig struct {
	// Port is the port to listen on.
	Port int
	// GRPCAddress is the address of the gRPC server to proxy to.
	GRPCAddress string
	// EnableCORS enables CORS support.
	EnableCORS bool
	// AllowedOrigins is the list of allowed CORS origins.
	AllowedOrigins []string
	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration
	// IdleTimeout is the maximum amount of time to wait for the next request.
	IdleTimeout time.Duration
	// WebSocketPath is the path for WebSocket connections (default: /ws).
	WebSocketPath string
	// EnableTracing enables OpenTelemetry tracing for HTTP requests.
	EnableTracing bool
	// Metrics is the control plane metrics instance for recording HTTP metrics.
	Metrics *metrics.ControlPlaneMetrics
}

// DefaultHTTPConfig returns sensible defaults for HTTP server configuration.
func DefaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		Port:           8080,
		GRPCAddress:    "localhost:9090",
		EnableCORS:     true,
		AllowedOrigins: []string{"*"},
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		WebSocketPath:  "/ws",
		EnableTracing:  false,
		Metrics:        nil,
	}
}

// HTTPServer wraps an HTTP server with grpc-gateway.
type HTTPServer struct {
	config         HTTPConfig
	server         *http.Server
	mux            *runtime.ServeMux
	wsHandler      *websocket.Handler
	webhookHandler *WebhookHandler
	logger         zerolog.Logger
}

// NewHTTPServer creates a new HTTP server with grpc-gateway.
func NewHTTPServer(cfg HTTPConfig, logger zerolog.Logger) (*HTTPServer, error) {
	// Create gRPC-Gateway mux
	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(customHeaderMatcher),
		runtime.WithErrorHandler(customErrorHandler),
	)

	return &HTTPServer{
		config: cfg,
		mux:    mux,
		logger: logger.With().Str("component", "http_server").Logger(),
	}, nil
}

// NewHTTPServerWithWebSocket creates a new HTTP server with grpc-gateway and WebSocket support.
func NewHTTPServerWithWebSocket(cfg HTTPConfig, wsHub *websocket.Hub, wsAuth websocket.Authenticator, logger zerolog.Logger) (*HTTPServer, error) {
	// Create gRPC-Gateway mux
	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(customHeaderMatcher),
		runtime.WithErrorHandler(customErrorHandler),
	)

	// Create WebSocket handler
	wsCfg := websocket.HandlerConfig{
		AllowedOrigins:  cfg.AllowedOrigins,
		RequireAuth:     false,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	wsHandler := websocket.NewHandlerWithConfig(wsHub, wsCfg, wsAuth, logger)

	return &HTTPServer{
		config:    cfg,
		mux:       mux,
		wsHandler: wsHandler,
		logger:    logger.With().Str("component", "http_server").Logger(),
	}, nil
}

// SetWebhookHandler sets the webhook handler for the HTTP server.
// This must be called before Start() to enable webhook endpoints.
func (s *HTTPServer) SetWebhookHandler(handler *WebhookHandler) {
	s.webhookHandler = handler
}

// Start starts the HTTP server and blocks until the context is cancelled.
func (s *HTTPServer) Start(ctx context.Context) error {
	// Connect to gRPC server
	grpcConn, err := grpc.NewClient(
		s.config.GRPCAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server: %w", err)
	}
	defer grpcConn.Close()

	// Register gRPC-Gateway handlers
	if err := s.registerHandlers(ctx, grpcConn); err != nil {
		return fmt.Errorf("failed to register handlers: %w", err)
	}

	// Create HTTP handler with middleware
	handler := s.buildHandler()

	// Create HTTP server
	addr := fmt.Sprintf(":%d", s.config.Port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	s.logger.Info().
		Str("address", addr).
		Str("grpc_address", s.config.GRPCAddress).
		Bool("cors_enabled", s.config.EnableCORS).
		Msg("starting HTTP server")

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info().Msg("context cancelled, stopping HTTP server")
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("HTTP server error: %w", err)
		}
		return nil
	}
}

// Stop gracefully stops the HTTP server.
func (s *HTTPServer) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	s.logger.Info().Msg("stopping HTTP server")

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Warn().Err(err).Msg("HTTP server shutdown error")
		return err
	}

	s.logger.Info().Msg("HTTP server stopped")
	return nil
}

// registerHandlers registers all gRPC-Gateway handlers.
func (s *HTTPServer) registerHandlers(ctx context.Context, conn *grpc.ClientConn) error {
	// Register all service handlers
	handlers := []func(context.Context, *runtime.ServeMux, *grpc.ClientConn) error{
		conductorv1.RegisterRunServiceHandler,
		conductorv1.RegisterServiceRegistryServiceHandler,
		conductorv1.RegisterAgentManagementServiceHandler,
		conductorv1.RegisterResultServiceHandler,
		conductorv1.RegisterHealthServiceHandler,
	}

	for _, handler := range handlers {
		if err := handler(ctx, s.mux, conn); err != nil {
			return fmt.Errorf("failed to register handler: %w", err)
		}
	}

	return nil
}

// buildHandler builds the HTTP handler with all middleware.
func (s *HTTPServer) buildHandler() http.Handler {
	// Create a new ServeMux to combine gRPC-Gateway and WebSocket handlers
	rootMux := http.NewServeMux()

	// Mount WebSocket handler if configured
	if s.wsHandler != nil {
		wsPath := s.config.WebSocketPath
		if wsPath == "" {
			wsPath = "/ws"
		}
		rootMux.Handle(wsPath, s.wsHandler)
		s.logger.Info().Str("path", wsPath).Msg("WebSocket handler mounted")
	}

	// Mount webhook handlers if configured
	if s.webhookHandler != nil {
		s.webhookHandler.RegisterRoutes(rootMux)
		s.logger.Info().Msg("webhook handlers mounted")
	}

	// Mount gRPC-Gateway handler for all other paths
	rootMux.Handle("/", s.mux)

	var handler http.Handler = rootMux

	// Add request ID middleware
	handler = s.requestIDMiddleware(handler)

	// Add logging middleware
	handler = s.loggingMiddleware(handler)

	// Add metrics middleware if configured
	if s.config.Metrics != nil {
		handler = s.metricsMiddleware(handler)
	}

	// Add tracing middleware if enabled
	if s.config.EnableTracing {
		handler = tracing.Middleware(handler)
	}

	// Add CORS middleware if enabled
	if s.config.EnableCORS {
		handler = s.corsMiddleware(handler)
	}

	// Add recovery middleware
	handler = s.recoveryMiddleware(handler)

	return handler
}

// corsMiddleware adds CORS headers to responses.
func (s *HTTPServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, o := range s.config.AllowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// requestIDMiddleware adds a request ID to the request context.
func (s *HTTPServer) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loggingMiddleware logs HTTP requests.
func (s *HTTPServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		requestID := GetRequestID(r.Context())

		logEvent := s.logger.Info()
		if wrapped.statusCode >= 400 {
			logEvent = s.logger.Warn()
		}
		if wrapped.statusCode >= 500 {
			logEvent = s.logger.Error()
		}

		logEvent.
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.statusCode).
			Dur("duration", duration).
			Str("remote_addr", r.RemoteAddr).
			Msg("HTTP request")
	})
}

// recoveryMiddleware recovers from panics.
func (s *HTTPServer) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				s.logger.Error().
					Interface("panic", p).
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Msg("recovered from panic")

				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "internal server error"}`))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// metricsMiddleware records HTTP request metrics.
func (s *HTTPServer) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		// Normalize path to reduce cardinality (replace UUIDs and numeric IDs with :id)
		path := normalizePath(r.URL.Path)

		// Record metrics
		s.config.Metrics.RecordAPIRequest(
			r.Method,
			path,
			fmt.Sprintf("%d", wrapped.statusCode),
			duration.Seconds(),
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// customHeaderMatcher allows custom headers to pass through to gRPC.
func customHeaderMatcher(key string) (string, bool) {
	switch strings.ToLower(key) {
	case "x-request-id":
		return key, true
	case "authorization":
		return key, true
	default:
		return runtime.DefaultHeaderMatcher(key)
	}
}

// customErrorHandler handles errors from gRPC and formats them for HTTP.
func customErrorHandler(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
	// Use the default error handler for now
	runtime.DefaultHTTPErrorHandler(ctx, mux, marshaler, w, r, err)
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
