package server

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// LoggingInterceptor provides logging for gRPC calls.
type LoggingInterceptor struct {
	logger zerolog.Logger
}

// NewLoggingInterceptor creates a new logging interceptor.
func NewLoggingInterceptor(logger zerolog.Logger) *LoggingInterceptor {
	return &LoggingInterceptor{
		logger: logger.With().Str("component", "grpc_logging").Logger(),
	}
}

// Unary returns a unary server interceptor for logging.
func (l *LoggingInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		requestID := getOrCreateRequestID(ctx)

		// Add request ID to context
		ctx = context.WithValue(ctx, requestIDKey{}, requestID)

		// Execute handler
		resp, err := handler(ctx, req)

		// Log the request
		duration := time.Since(start)
		logEvent := l.logger.Info()

		if err != nil {
			st, _ := status.FromError(err)
			logEvent = l.logger.Error().
				Str("error", st.Message()).
				Str("code", st.Code().String())
		}

		logEvent.
			Str("request_id", requestID).
			Str("method", info.FullMethod).
			Dur("duration", duration).
			Msg("unary request completed")

		return resp, err
	}
}

// Stream returns a stream server interceptor for logging.
func (l *LoggingInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		requestID := getOrCreateRequestID(ss.Context())

		// Wrap stream with request ID in context
		wrapped := &wrappedServerStream{
			ServerStream: ss,
			ctx:          context.WithValue(ss.Context(), requestIDKey{}, requestID),
		}

		l.logger.Info().
			Str("request_id", requestID).
			Str("method", info.FullMethod).
			Msg("stream started")

		// Execute handler
		err := handler(srv, wrapped)

		// Log completion
		duration := time.Since(start)
		logEvent := l.logger.Info()

		if err != nil {
			st, _ := status.FromError(err)
			logEvent = l.logger.Error().
				Str("error", st.Message()).
				Str("code", st.Code().String())
		}

		logEvent.
			Str("request_id", requestID).
			Str("method", info.FullMethod).
			Dur("duration", duration).
			Msg("stream completed")

		return err
	}
}

// RecoveryInterceptor handles panics in gRPC handlers.
type RecoveryInterceptor struct {
	logger zerolog.Logger
}

// NewRecoveryInterceptor creates a new recovery interceptor.
func NewRecoveryInterceptor(logger zerolog.Logger) *RecoveryInterceptor {
	return &RecoveryInterceptor{
		logger: logger.With().Str("component", "grpc_recovery").Logger(),
	}
}

// Unary returns a unary server interceptor for panic recovery.
func (r *RecoveryInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if p := recover(); p != nil {
				r.logger.Error().
					Interface("panic", p).
					Str("stack", string(debug.Stack())).
					Str("method", info.FullMethod).
					Msg("recovered from panic")

				err = status.Error(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}

// Stream returns a stream server interceptor for panic recovery.
func (r *RecoveryInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if p := recover(); p != nil {
				r.logger.Error().
					Interface("panic", p).
					Str("stack", string(debug.Stack())).
					Str("method", info.FullMethod).
					Msg("recovered from panic")

				err = status.Error(codes.Internal, "internal server error")
			}
		}()

		return handler(srv, ss)
	}
}

// AuthInterceptor handles authentication for gRPC calls.
type AuthInterceptor struct {
	validator *JWTValidator
	logger    zerolog.Logger

	// Methods that don't require authentication
	publicMethods map[string]bool
}

// NewAuthInterceptor creates a new auth interceptor.
func NewAuthInterceptor(validator *JWTValidator, logger zerolog.Logger) *AuthInterceptor {
	return &AuthInterceptor{
		validator: validator,
		logger:    logger.With().Str("component", "grpc_auth").Logger(),
		publicMethods: map[string]bool{
			"/conductor.v1.HealthService/Check":          true,
			"/conductor.v1.HealthService/CheckLiveness":  true,
			"/conductor.v1.HealthService/CheckReadiness": true,
			"/conductor.v1.AgentService/WorkStream":      true, // Agents use their own auth
			"/grpc.health.v1.Health/Check":               true,
			"/grpc.health.v1.Health/Watch":               true,
		},
	}
}

// Unary returns a unary server interceptor for authentication.
func (a *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip auth for public methods
		if a.publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		// Extract and validate token
		claims, err := a.authenticate(ctx)
		if err != nil {
			return nil, err
		}

		// Add claims to context
		ctx = context.WithValue(ctx, userClaimsKey{}, claims)

		return handler(ctx, req)
	}
}

// Stream returns a stream server interceptor for authentication.
func (a *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Skip auth for public methods
		if a.publicMethods[info.FullMethod] {
			return handler(srv, ss)
		}

		// Extract and validate token
		claims, err := a.authenticate(ss.Context())
		if err != nil {
			return err
		}

		// Wrap stream with authenticated context
		wrapped := &wrappedServerStream{
			ServerStream: ss,
			ctx:          context.WithValue(ss.Context(), userClaimsKey{}, claims),
		}

		return handler(srv, wrapped)
	}
}

// authenticate extracts and validates the JWT token from the context.
func (a *AuthInterceptor) authenticate(ctx context.Context) (*UserClaims, error) {
	token, err := extractToken(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "missing or invalid authorization: %v", err)
	}

	claims, err := a.validator.Validate(token)
	if err != nil {
		a.logger.Debug().Err(err).Msg("token validation failed")
		return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}

	return claims, nil
}

// wrappedServerStream wraps a grpc.ServerStream to override the context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context.
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// Context keys
type requestIDKey struct{}
type userClaimsKey struct{}

// getOrCreateRequestID extracts the request ID from metadata or creates a new one.
func getOrCreateRequestID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if values := md.Get("x-request-id"); len(values) > 0 {
			return values[0]
		}
	}
	return uuid.New().String()
}

// extractToken extracts the bearer token from the authorization header.
func extractToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no metadata in context")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "authorization header not provided")
	}

	// Expect "Bearer <token>"
	auth := values[0]
	const prefix = "Bearer "
	if len(auth) < len(prefix) || auth[:len(prefix)] != prefix {
		return "", status.Error(codes.Unauthenticated, "invalid authorization header format")
	}

	return auth[len(prefix):], nil
}

// GetRequestID returns the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// GetUserFromContext returns the user claims from the context.
func GetUserFromContext(ctx context.Context) *UserClaims {
	if claims, ok := ctx.Value(userClaimsKey{}).(*UserClaims); ok {
		return claims
	}
	return nil
}
