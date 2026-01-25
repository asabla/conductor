package tracing

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// httpTracer is the tracer for HTTP operations.
var httpTracer = otel.Tracer("conductor/http")

// Middleware returns an HTTP middleware that traces requests.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract trace context from incoming headers
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Start span
		spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		ctx, span := httpTracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
				attribute.String("http.target", r.URL.Path),
				attribute.String("http.host", r.Host),
				attribute.String("http.scheme", getScheme(r)),
				attribute.String("http.user_agent", r.UserAgent()),
				attribute.String("http.client_ip", getClientIP(r)),
			),
		)
		defer span.End()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Update request with traced context
		r = r.WithContext(ctx)

		// Call next handler
		start := time.Now()
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start)

		// Record result
		span.SetAttributes(
			attribute.Int("http.status_code", wrapped.statusCode),
			attribute.Int64("http.response_content_length", int64(wrapped.bytesWritten)),
			attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
		)

		if wrapped.statusCode >= 400 {
			span.SetStatus(codes.Error, http.StatusText(wrapped.statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}
	})
}

// RoundTripper returns an HTTP RoundTripper that traces outgoing requests.
func RoundTripper(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &tracingRoundTripper{next: next}
}

// tracingRoundTripper wraps http.RoundTripper with tracing.
type tracingRoundTripper struct {
	next http.RoundTripper
}

// RoundTrip executes the request with tracing.
func (t *tracingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()

	// Start span
	spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	ctx, span := httpTracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
			attribute.String("http.target", r.URL.Path),
			attribute.String("http.host", r.Host),
			attribute.String("http.scheme", r.URL.Scheme),
		),
	)
	defer span.End()

	// Inject trace context into outgoing headers
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(r.Header))

	// Update request with traced context
	r = r.WithContext(ctx)

	// Execute request
	start := time.Now()
	resp, err := t.next.RoundTrip(r)
	duration := time.Since(start)

	span.SetAttributes(attribute.Float64("http.duration_ms", float64(duration.Milliseconds())))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Record response
	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.Int64("http.response_content_length", resp.ContentLength),
	)

	if resp.StatusCode >= 400 {
		span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return resp, nil
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

// WriteHeader captures the status code.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the bytes written.
func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// Flush implements http.Flusher.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// getScheme returns the request scheme.
func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	// Check X-Forwarded-Proto header
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}

// getClientIP returns the client IP address.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

// MiddlewareWithConfig returns an HTTP middleware with custom configuration.
type MiddlewareConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper func(r *http.Request) bool
	// SpanNameFormatter formats the span name.
	SpanNameFormatter func(r *http.Request) string
}

// MiddlewareWithConfig returns a configured tracing middleware.
func MiddlewareWithConfig(cfg MiddlewareConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check skipper
			if cfg.Skipper != nil && cfg.Skipper(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Extract trace context from incoming headers
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Format span name
			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			if cfg.SpanNameFormatter != nil {
				spanName = cfg.SpanNameFormatter(r)
			}

			// Start span
			ctx, span := httpTracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.url", r.URL.String()),
					attribute.String("http.target", r.URL.Path),
					attribute.String("http.host", r.Host),
					attribute.String("http.scheme", getScheme(r)),
					attribute.String("http.user_agent", r.UserAgent()),
					attribute.String("http.client_ip", getClientIP(r)),
				),
			)
			defer span.End()

			// Wrap response writer
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Update request with traced context
			r = r.WithContext(ctx)

			// Call next handler
			start := time.Now()
			next.ServeHTTP(wrapped, r)
			duration := time.Since(start)

			// Record result
			span.SetAttributes(
				attribute.Int("http.status_code", wrapped.statusCode),
				attribute.Int64("http.response_content_length", int64(wrapped.bytesWritten)),
				attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
			)

			if wrapped.statusCode >= 400 {
				span.SetStatus(codes.Error, http.StatusText(wrapped.statusCode))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}
