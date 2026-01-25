package health

import (
	"context"
	"testing"
)

// mockWebSocketHub implements WebSocketHub for testing.
type mockWebSocketHub struct {
	healthy   bool
	connCount int
	roomCount int
}

func (m *mockWebSocketHub) IsHealthy() bool      { return m.healthy }
func (m *mockWebSocketHub) ConnectionCount() int { return m.connCount }
func (m *mockWebSocketHub) RoomCount() int       { return m.roomCount }

func TestWebSocketCheck_Name(t *testing.T) {
	hub := &mockWebSocketHub{healthy: true}
	check := NewWebSocketCheck(hub)

	if check.Name() != "websocket" {
		t.Errorf("expected name 'websocket', got '%s'", check.Name())
	}
}

func TestWebSocketCheck_Healthy(t *testing.T) {
	hub := &mockWebSocketHub{healthy: true, connCount: 5, roomCount: 3}
	check := NewWebSocketCheck(hub)

	err := check.Check(context.Background())
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestWebSocketCheck_Unhealthy(t *testing.T) {
	hub := &mockWebSocketHub{healthy: false}
	check := NewWebSocketCheck(hub)

	err := check.Check(context.Background())
	if err == nil {
		t.Error("expected error for unhealthy hub")
	}
}

func TestWebSocketCheck_CheckDetailed_Healthy(t *testing.T) {
	hub := &mockWebSocketHub{healthy: true, connCount: 5, roomCount: 3}
	check := NewWebSocketCheck(hub)

	result := check.CheckDetailed(context.Background())

	if result.Status != StatusHealthy {
		t.Errorf("expected status healthy, got %s", result.Status)
	}

	if result.Details["connections"] != "5" {
		t.Errorf("expected connections=5, got %s", result.Details["connections"])
	}

	if result.Details["rooms"] != "3" {
		t.Errorf("expected rooms=3, got %s", result.Details["rooms"])
	}
}

func TestWebSocketCheck_CheckDetailed_Unhealthy(t *testing.T) {
	hub := &mockWebSocketHub{healthy: false}
	check := NewWebSocketCheck(hub)

	result := check.CheckDetailed(context.Background())

	if result.Status != StatusUnhealthy {
		t.Errorf("expected status unhealthy, got %s", result.Status)
	}
}

func TestWebSocketCheck_CheckDetailed_Degraded(t *testing.T) {
	hub := &mockWebSocketHub{healthy: true, connCount: 15000, roomCount: 100}
	check := NewWebSocketCheck(hub, WithMaxConnectionsThreshold(10000))

	result := check.CheckDetailed(context.Background())

	if result.Status != StatusDegraded {
		t.Errorf("expected status degraded, got %s", result.Status)
	}
}

func TestWebSocketCheck_WithOptions(t *testing.T) {
	hub := &mockWebSocketHub{healthy: true, connCount: 500}
	check := NewWebSocketCheck(hub, WithMaxConnectionsThreshold(100))

	result := check.CheckDetailed(context.Background())

	if result.Status != StatusDegraded {
		t.Errorf("expected status degraded with custom threshold, got %s", result.Status)
	}
}
