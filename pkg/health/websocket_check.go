// Package health provides health check implementations for various components.
package health

import (
	"context"
	"fmt"
)

// Check represents a health check.
type Check interface {
	// Name returns the name of the health check.
	Name() string
	// Check performs the health check and returns an error if unhealthy.
	Check(ctx context.Context) error
}

// Status represents the status of a health check.
type Status string

const (
	// StatusHealthy indicates the component is healthy.
	StatusHealthy Status = "healthy"
	// StatusUnhealthy indicates the component is unhealthy.
	StatusUnhealthy Status = "unhealthy"
	// StatusDegraded indicates the component is working but degraded.
	StatusDegraded Status = "degraded"
)

// Result represents the result of a health check.
type Result struct {
	Name    string            `json:"name"`
	Status  Status            `json:"status"`
	Message string            `json:"message,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

// WebSocketHub defines the interface for WebSocket hub health checks.
type WebSocketHub interface {
	// IsHealthy returns true if the hub is running.
	IsHealthy() bool
	// ConnectionCount returns the number of active connections.
	ConnectionCount() int
	// RoomCount returns the number of active rooms.
	RoomCount() int
}

// WebSocketCheck checks the health of the WebSocket hub.
type WebSocketCheck struct {
	hub                     WebSocketHub
	maxConnectionsThreshold int
}

// WebSocketCheckOption configures a WebSocketCheck.
type WebSocketCheckOption func(*WebSocketCheck)

// WithMaxConnectionsThreshold sets the threshold above which the check reports degraded status.
func WithMaxConnectionsThreshold(threshold int) WebSocketCheckOption {
	return func(c *WebSocketCheck) {
		c.maxConnectionsThreshold = threshold
	}
}

// NewWebSocketCheck creates a new WebSocket health check.
func NewWebSocketCheck(hub WebSocketHub, opts ...WebSocketCheckOption) *WebSocketCheck {
	c := &WebSocketCheck{
		hub:                     hub,
		maxConnectionsThreshold: 10000, // Default: warn if > 10k connections
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name returns the name of the health check.
func (c *WebSocketCheck) Name() string {
	return "websocket"
}

// Check performs the WebSocket health check.
func (c *WebSocketCheck) Check(ctx context.Context) error {
	if !c.hub.IsHealthy() {
		return fmt.Errorf("WebSocket hub is not running")
	}
	return nil
}

// CheckDetailed performs a detailed health check and returns a Result.
func (c *WebSocketCheck) CheckDetailed(ctx context.Context) Result {
	if !c.hub.IsHealthy() {
		return Result{
			Name:    c.Name(),
			Status:  StatusUnhealthy,
			Message: "WebSocket hub is not running",
		}
	}

	connCount := c.hub.ConnectionCount()
	roomCount := c.hub.RoomCount()

	details := map[string]string{
		"connections": fmt.Sprintf("%d", connCount),
		"rooms":       fmt.Sprintf("%d", roomCount),
	}

	// Check if we're approaching connection limits
	if c.maxConnectionsThreshold > 0 && connCount > c.maxConnectionsThreshold {
		return Result{
			Name:    c.Name(),
			Status:  StatusDegraded,
			Message: fmt.Sprintf("high connection count: %d", connCount),
			Details: details,
		}
	}

	return Result{
		Name:    c.Name(),
		Status:  StatusHealthy,
		Message: "WebSocket hub is running",
		Details: details,
	}
}
