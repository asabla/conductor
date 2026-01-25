package log

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// RequestIDHeader is the HTTP header for request ID.
	RequestIDHeader = "X-Request-ID"
	// CorrelationIDHeader is the HTTP header for correlation ID.
	CorrelationIDHeader = "X-Correlation-ID"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// Flush implements http.Flusher.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// HTTPMiddleware returns an HTTP middleware that logs requests and adds
// request/correlation IDs to the context.
func HTTPMiddleware(log Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Extract or generate request ID
			requestID := r.Header.Get(RequestIDHeader)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Extract or generate correlation ID
			correlationID := r.Header.Get(CorrelationIDHeader)
			if correlationID == "" {
				correlationID = requestID
			}

			// Add IDs to context
			ctx := r.Context()
			ctx = ContextWithRequestID(ctx, requestID)
			ctx = ContextWithCorrelationID(ctx, correlationID)

			// Create request-scoped logger
			reqLog := log.WithContext(ctx)
			ctx = ContextWithLogger(ctx, reqLog)

			// Set response headers
			w.Header().Set(RequestIDHeader, requestID)
			w.Header().Set(CorrelationIDHeader, correlationID)

			// Wrap response writer to capture status code
			rw := newResponseWriter(w)

			// Log request start at debug level
			reqLog.Debug().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remote_addr", r.RemoteAddr).
				Str("user_agent", r.UserAgent()).
				Msg("request started")

			// Process request
			next.ServeHTTP(rw, r.WithContext(ctx))

			// Log request completion
			duration := time.Since(start)
			logEvent := reqLog.Info()

			// Use warn/error level for non-success status codes
			if rw.statusCode >= 500 {
				logEvent = reqLog.Error()
			} else if rw.statusCode >= 400 {
				logEvent = reqLog.Warn()
			}

			logEvent.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", rw.statusCode).
				Int64("bytes", rw.written).
				Dur("duration", duration).
				Msg("request completed")
		})
	}
}

// GRPCUnaryServerInterceptor returns a gRPC unary server interceptor
// that logs requests and adds request/correlation IDs to the context.
func GRPCUnaryServerInterceptor(log Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// Extract or generate request ID from metadata
		requestID := extractMetadataValue(ctx, "x-request-id")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		correlationID := extractMetadataValue(ctx, "x-correlation-id")
		if correlationID == "" {
			correlationID = requestID
		}

		// Add IDs to context
		ctx = ContextWithRequestID(ctx, requestID)
		ctx = ContextWithCorrelationID(ctx, correlationID)

		// Create request-scoped logger
		reqLog := log.WithContext(ctx)
		ctx = ContextWithLogger(ctx, reqLog)

		// Set outgoing metadata
		ctx = metadata.AppendToOutgoingContext(ctx,
			"x-request-id", requestID,
			"x-correlation-id", correlationID,
		)

		// Log request start at debug level
		reqLog.Debug().
			Str("method", info.FullMethod).
			Msg("gRPC request started")

		// Handle request
		resp, err := handler(ctx, req)

		// Log request completion
		duration := time.Since(start)
		statusCode := status.Code(err)

		logEvent := reqLog.Info()
		if err != nil {
			logEvent = reqLog.Error().Err(err)
		}

		logEvent.
			Str("method", info.FullMethod).
			Str("status", statusCode.String()).
			Dur("duration", duration).
			Msg("gRPC request completed")

		return resp, err
	}
}

// GRPCStreamServerInterceptor returns a gRPC stream server interceptor
// that logs stream lifecycle and adds request/correlation IDs to the context.
func GRPCStreamServerInterceptor(log Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		ctx := ss.Context()

		// Extract or generate request ID from metadata
		requestID := extractMetadataValue(ctx, "x-request-id")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		correlationID := extractMetadataValue(ctx, "x-correlation-id")
		if correlationID == "" {
			correlationID = requestID
		}

		// Add IDs to context
		ctx = ContextWithRequestID(ctx, requestID)
		ctx = ContextWithCorrelationID(ctx, correlationID)

		// Create request-scoped logger
		reqLog := log.WithContext(ctx)
		ctx = ContextWithLogger(ctx, reqLog)

		// Log stream start
		reqLog.Debug().
			Str("method", info.FullMethod).
			Bool("client_stream", info.IsClientStream).
			Bool("server_stream", info.IsServerStream).
			Msg("gRPC stream started")

		// Wrap the stream to inject the context
		wrapped := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		// Handle stream
		err := handler(srv, wrapped)

		// Log stream completion
		duration := time.Since(start)
		statusCode := status.Code(err)

		logEvent := reqLog.Info()
		if err != nil {
			logEvent = reqLog.Error().Err(err)
		}

		logEvent.
			Str("method", info.FullMethod).
			Str("status", statusCode.String()).
			Dur("duration", duration).
			Msg("gRPC stream completed")

		return err
	}
}

// wrappedServerStream wraps grpc.ServerStream to override the context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// extractMetadataValue extracts a value from gRPC metadata.
func extractMetadataValue(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// GRPCUnaryClientInterceptor returns a gRPC unary client interceptor
// that propagates request/correlation IDs.
func GRPCUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Propagate IDs to outgoing metadata
		ctx = propagateIDs(ctx)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// GRPCStreamClientInterceptor returns a gRPC stream client interceptor
// that propagates request/correlation IDs.
func GRPCStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// Propagate IDs to outgoing metadata
		ctx = propagateIDs(ctx)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// propagateIDs adds request and correlation IDs to outgoing gRPC metadata.
func propagateIDs(ctx context.Context) context.Context {
	requestID := RequestIDFromContext(ctx)
	correlationID := CorrelationIDFromContext(ctx)

	if requestID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", requestID)
	}
	if correlationID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-correlation-id", correlationID)
	}

	return ctx
}
