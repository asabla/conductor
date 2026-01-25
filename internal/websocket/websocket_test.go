package websocket

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func TestNewMessage(t *testing.T) {
	payload := map[string]string{"key": "value"}
	msg, err := NewMessage(MessageTypeRunUpdate, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Type != MessageTypeRunUpdate {
		t.Errorf("expected type %s, got %s", MessageTypeRunUpdate, msg.Type)
	}

	if msg.ID == "" {
		t.Error("expected non-empty ID")
	}

	if msg.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	var decoded map[string]string
	if err := json.Unmarshal(msg.Payload, &decoded); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}

	if decoded["key"] != "value" {
		t.Errorf("expected payload key='value', got %s", decoded["key"])
	}
}

func TestNewRoomMessage(t *testing.T) {
	msg, err := NewRoomMessage(MessageTypeSubscribed, "run:123", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Room != "run:123" {
		t.Errorf("expected room 'run:123', got '%s'", msg.Room)
	}
}

func TestMessageBytes(t *testing.T) {
	msg, _ := NewMessage(MessageTypePong, nil)
	data, err := msg.Bytes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	if parsed.Type != msg.Type {
		t.Errorf("expected type %s, got %s", msg.Type, parsed.Type)
	}
}

func TestParseMessage_Invalid(t *testing.T) {
	_, err := ParseMessage([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRoomName(t *testing.T) {
	tests := []struct {
		roomType RoomType
		id       string
		expected string
	}{
		{RoomTypeRun, "123", "run:123"},
		{RoomTypeAgent, "abc", "agent:abc"},
		{RoomTypeService, "svc-1", "service:svc-1"},
		{RoomTypeGlobal, "all", "global:all"},
	}

	for _, tt := range tests {
		got := RoomName(tt.roomType, tt.id)
		if got != tt.expected {
			t.Errorf("RoomName(%s, %s) = %s, expected %s", tt.roomType, tt.id, got, tt.expected)
		}
	}
}

func TestParseRoomName(t *testing.T) {
	tests := []struct {
		room         string
		expectedType RoomType
		expectedID   string
	}{
		{"run:123", RoomTypeRun, "123"},
		{"agent:abc", RoomTypeAgent, "abc"},
		{"service:svc-1", RoomTypeService, "svc-1"},
		{"global:all", RoomTypeGlobal, "all"},
		{"invalid", RoomTypeGlobal, "invalid"},
	}

	for _, tt := range tests {
		gotType, gotID := ParseRoomName(tt.room)
		if gotType != tt.expectedType {
			t.Errorf("ParseRoomName(%s) type = %s, expected %s", tt.room, gotType, tt.expectedType)
		}
		if gotID != tt.expectedID {
			t.Errorf("ParseRoomName(%s) id = %s, expected %s", tt.room, gotID, tt.expectedID)
		}
	}
}

func TestHub_BasicOperations(t *testing.T) {
	logger := zerolog.Nop()
	hub := NewHub(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start hub
	go hub.Run(ctx)

	// Give hub time to start
	time.Sleep(10 * time.Millisecond)

	// Check initial state
	if hub.ConnectionCount() != 0 {
		t.Errorf("expected 0 connections, got %d", hub.ConnectionCount())
	}

	if hub.RoomCount() != 0 {
		t.Errorf("expected 0 rooms, got %d", hub.RoomCount())
	}

	stats := hub.Stats()
	if stats.ActiveConnections != 0 {
		t.Errorf("expected 0 active connections in stats, got %d", stats.ActiveConnections)
	}
}

func TestHub_IsHealthy(t *testing.T) {
	logger := zerolog.Nop()
	hub := NewHub(logger)

	if !hub.IsHealthy() {
		t.Error("expected hub to be healthy")
	}
}

func TestRunUpdatePayload_JSON(t *testing.T) {
	runID := uuid.New()
	serviceID := uuid.New()
	payload := RunUpdatePayload{
		RunID:       runID,
		ServiceID:   serviceID,
		Status:      "running",
		TotalTests:  10,
		PassedTests: 5,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded RunUpdatePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.RunID != runID {
		t.Errorf("expected RunID %s, got %s", runID, decoded.RunID)
	}

	if decoded.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", decoded.Status)
	}
}

func TestAgentUpdatePayload_JSON(t *testing.T) {
	agentID := uuid.New()
	now := time.Now()
	version := "1.0.0"
	payload := AgentUpdatePayload{
		AgentID:         agentID,
		Name:            "test-agent",
		Status:          "idle",
		LastHeartbeat:   &now,
		ActiveJobs:      2,
		Version:         &version,
		DockerAvailable: true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded AgentUpdatePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got '%s'", decoded.Name)
	}

	if !decoded.DockerAvailable {
		t.Error("expected DockerAvailable to be true")
	}
}

func TestLogChunkPayload_JSON(t *testing.T) {
	runID := uuid.New()
	payload := LogChunkPayload{
		RunID:     runID,
		Sequence:  42,
		Stream:    "stdout",
		Data:      "test output line",
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded LogChunkPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Sequence != 42 {
		t.Errorf("expected sequence 42, got %d", decoded.Sequence)
	}

	if decoded.Stream != "stdout" {
		t.Errorf("expected stream 'stdout', got '%s'", decoded.Stream)
	}
}
