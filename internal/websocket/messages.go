// Package websocket provides real-time WebSocket support for the Conductor control plane.
// It enables clients to receive live updates about test runs, agent status changes,
// and streaming logs.
package websocket

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// MessageType defines the type of WebSocket message.
type MessageType string

const (
	// Client -> Server message types
	MessageTypeSubscribe   MessageType = "subscribe"
	MessageTypeUnsubscribe MessageType = "unsubscribe"
	MessageTypePing        MessageType = "ping"

	// Server -> Client message types
	MessageTypePong          MessageType = "pong"
	MessageTypeSubscribed    MessageType = "subscribed"
	MessageTypeUnsubscribed  MessageType = "unsubscribed"
	MessageTypeError         MessageType = "error"
	MessageTypeRunUpdate     MessageType = "run_update"
	MessageTypeAgentUpdate   MessageType = "agent_update"
	MessageTypeLogChunk      MessageType = "log_chunk"
	MessageTypeTestResult    MessageType = "test_result"
	MessageTypeServiceUpdate MessageType = "service_update"
)

// RoomType defines the type of subscription room.
type RoomType string

const (
	RoomTypeRun     RoomType = "run"
	RoomTypeAgent   RoomType = "agent"
	RoomTypeService RoomType = "service"
	RoomTypeGlobal  RoomType = "global" // For system-wide events
)

// Message represents a WebSocket message.
type Message struct {
	// Type is the message type.
	Type MessageType `json:"type"`
	// Room is the target room (optional, used for subscribe/unsubscribe).
	Room string `json:"room,omitempty"`
	// Payload contains the message data.
	Payload json.RawMessage `json:"payload,omitempty"`
	// Timestamp is when the message was created.
	Timestamp time.Time `json:"timestamp"`
	// ID is a unique message identifier.
	ID string `json:"id,omitempty"`
}

// NewMessage creates a new message with the given type and payload.
func NewMessage(msgType MessageType, payload interface{}) (*Message, error) {
	var payloadBytes json.RawMessage
	if payload != nil {
		var err error
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}

	return &Message{
		Type:      msgType,
		Payload:   payloadBytes,
		Timestamp: time.Now().UTC(),
		ID:        uuid.New().String(),
	}, nil
}

// NewRoomMessage creates a new message targeted at a specific room.
func NewRoomMessage(msgType MessageType, room string, payload interface{}) (*Message, error) {
	msg, err := NewMessage(msgType, payload)
	if err != nil {
		return nil, err
	}
	msg.Room = room
	return msg, nil
}

// Bytes serializes the message to JSON bytes.
func (m *Message) Bytes() ([]byte, error) {
	return json.Marshal(m)
}

// ParseMessage deserializes a message from JSON bytes.
func ParseMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// SubscribePayload is the payload for subscribe messages.
type SubscribePayload struct {
	// Room is the room to subscribe to.
	Room string `json:"room"`
}

// UnsubscribePayload is the payload for unsubscribe messages.
type UnsubscribePayload struct {
	// Room is the room to unsubscribe from.
	Room string `json:"room"`
}

// ErrorPayload is the payload for error messages.
type ErrorPayload struct {
	// Code is the error code.
	Code string `json:"code"`
	// Message is a human-readable error message.
	Message string `json:"message"`
}

// RunUpdatePayload is the payload for run update messages.
type RunUpdatePayload struct {
	RunID        uuid.UUID  `json:"run_id"`
	ServiceID    uuid.UUID  `json:"service_id"`
	Status       string     `json:"status"`
	TotalTests   int        `json:"total_tests,omitempty"`
	PassedTests  int        `json:"passed_tests,omitempty"`
	FailedTests  int        `json:"failed_tests,omitempty"`
	SkippedTests int        `json:"skipped_tests,omitempty"`
	DurationMs   *int64     `json:"duration_ms,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// AgentUpdatePayload is the payload for agent update messages.
type AgentUpdatePayload struct {
	AgentID         uuid.UUID  `json:"agent_id"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	LastHeartbeat   *time.Time `json:"last_heartbeat,omitempty"`
	ActiveJobs      int        `json:"active_jobs,omitempty"`
	Version         *string    `json:"version,omitempty"`
	DockerAvailable bool       `json:"docker_available,omitempty"`
}

// LogChunkPayload is the payload for log chunk messages.
type LogChunkPayload struct {
	RunID     uuid.UUID `json:"run_id"`
	Sequence  int64     `json:"sequence"`
	Stream    string    `json:"stream"` // "stdout" or "stderr"
	Data      string    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

// TestResultPayload is the payload for test result messages.
type TestResultPayload struct {
	RunID        uuid.UUID `json:"run_id"`
	TestName     string    `json:"test_name"`
	SuiteName    *string   `json:"suite_name,omitempty"`
	Status       string    `json:"status"`
	DurationMs   *int64    `json:"duration_ms,omitempty"`
	ErrorMessage *string   `json:"error_message,omitempty"`
}

// ServiceUpdatePayload is the payload for service update messages.
type ServiceUpdatePayload struct {
	ServiceID     uuid.UUID  `json:"service_id"`
	Name          string     `json:"name"`
	LastRunStatus *string    `json:"last_run_status,omitempty"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
}

// RoomName creates a standardized room name from type and ID.
func RoomName(roomType RoomType, id string) string {
	return string(roomType) + ":" + id
}

// ParseRoomName extracts the room type and ID from a room name.
func ParseRoomName(room string) (RoomType, string) {
	for i, c := range room {
		if c == ':' {
			return RoomType(room[:i]), room[i+1:]
		}
	}
	return RoomTypeGlobal, room
}
