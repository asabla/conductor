package websocket

import (
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// Handler handles WebSocket upgrade requests and connection management.
type Handler struct {
	hub      *Hub
	upgrader websocket.Upgrader
	auth     Authenticator
	logger   zerolog.Logger
}

// Authenticator validates WebSocket connection authentication.
type Authenticator interface {
	// Authenticate validates the request and returns user info.
	// Returns nil claims and nil error to allow anonymous connections.
	// Returns error to reject the connection.
	Authenticate(r *http.Request) (userID string, claims map[string]interface{}, err error)
}

// NoopAuthenticator allows all connections without authentication.
type NoopAuthenticator struct{}

// Authenticate always returns success with empty claims.
func (NoopAuthenticator) Authenticate(r *http.Request) (string, map[string]interface{}, error) {
	return "", nil, nil
}

// TokenAuthenticator validates Bearer tokens from the Authorization header or query param.
type TokenAuthenticator struct {
	validator TokenValidator
}

// TokenValidator validates authentication tokens.
type TokenValidator interface {
	ValidateToken(token string) (userID string, claims map[string]interface{}, err error)
}

// NewTokenAuthenticator creates a new token-based authenticator.
func NewTokenAuthenticator(validator TokenValidator) *TokenAuthenticator {
	return &TokenAuthenticator{validator: validator}
}

// Authenticate extracts and validates the token from the request.
func (a *TokenAuthenticator) Authenticate(r *http.Request) (string, map[string]interface{}, error) {
	// Try Authorization header first
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		const bearerPrefix = "Bearer "
		if strings.HasPrefix(authHeader, bearerPrefix) {
			token := strings.TrimPrefix(authHeader, bearerPrefix)
			return a.validator.ValidateToken(token)
		}
	}

	// Fall back to query parameter (useful for browser WebSocket connections)
	token := r.URL.Query().Get("token")
	if token != "" {
		return a.validator.ValidateToken(token)
	}

	// No token provided - allow anonymous connection
	return "", nil, nil
}

// HandlerConfig configures the WebSocket handler.
type HandlerConfig struct {
	// AllowedOrigins is a list of allowed origins. Use "*" to allow all.
	AllowedOrigins []string
	// RequireAuth requires authentication for all connections.
	RequireAuth bool
	// ReadBufferSize is the buffer size for reading messages.
	ReadBufferSize int
	// WriteBufferSize is the buffer size for writing messages.
	WriteBufferSize int
}

// DefaultHandlerConfig returns sensible defaults for handler configuration.
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		AllowedOrigins:  []string{"*"},
		RequireAuth:     false,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
}

// NewHandler creates a new WebSocket handler.
func NewHandler(hub *Hub, logger zerolog.Logger) *Handler {
	return NewHandlerWithConfig(hub, DefaultHandlerConfig(), nil, logger)
}

// NewHandlerWithConfig creates a new WebSocket handler with custom configuration.
func NewHandlerWithConfig(hub *Hub, cfg HandlerConfig, auth Authenticator, logger zerolog.Logger) *Handler {
	if auth == nil {
		auth = NoopAuthenticator{}
	}

	h := &Handler{
		hub:    hub,
		auth:   auth,
		logger: logger.With().Str("component", "websocket_handler").Logger(),
	}

	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  cfg.ReadBufferSize,
		WriteBufferSize: cfg.WriteBufferSize,
		CheckOrigin:     h.makeOriginChecker(cfg.AllowedOrigins),
	}

	return h
}

// makeOriginChecker creates an origin checking function based on allowed origins.
func (h *Handler) makeOriginChecker(allowedOrigins []string) func(*http.Request) bool {
	// If wildcard is present, allow all origins
	for _, origin := range allowedOrigins {
		if origin == "*" {
			return func(r *http.Request) bool {
				return true
			}
		}
	}

	// Build a set for O(1) lookup
	allowed := make(map[string]bool)
	for _, origin := range allowedOrigins {
		allowed[origin] = true
	}

	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // Allow requests without origin (e.g., native apps)
		}
		return allowed[origin]
	}
}

// ServeHTTP upgrades HTTP connections to WebSocket and handles messages.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Authenticate the request
	userID, claims, err := h.auth.Authenticate(r)
	if err != nil {
		h.logger.Debug().Err(err).Msg("authentication failed")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Upgrade the HTTP connection to WebSocket
	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Debug().Err(err).Msg("failed to upgrade connection")
		return
	}

	// Create connection with options
	opts := []ConnectionOption{}
	if userID != "" {
		opts = append(opts, WithUserID(userID))
	}
	if claims != nil {
		opts = append(opts, WithClaims(claims))
	}

	conn := NewConnection(ws, h.hub, h.logger, opts...)

	// Register connection with hub
	h.hub.Register(conn)

	h.logger.Info().
		Str("conn_id", conn.ID()).
		Str("remote_addr", r.RemoteAddr).
		Str("user_id", userID).
		Msg("WebSocket connection established")

	// Start pumps in goroutines
	go conn.WritePump()
	go conn.ReadPump()
}

// Handler returns the http.Handler interface for the WebSocket handler.
func (h *Handler) Handler() http.Handler {
	return h
}
