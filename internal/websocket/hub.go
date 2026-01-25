package websocket

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Hub manages all WebSocket connections and message broadcasting.
type Hub struct {
	// connections holds all active connections
	connections map[*Connection]struct{}

	// rooms maps room names to connections subscribed to that room
	rooms map[string]map[*Connection]struct{}

	// register channel for new connections
	register chan *Connection

	// unregister channel for removing connections
	unregister chan *Connection

	// subscribe channel for room subscriptions
	subscribe chan *subscriptionRequest

	// unsubscribeCh channel for room unsubscriptions
	unsubscribeCh chan *subscriptionRequest

	// broadcast channel for room-targeted messages
	broadcast chan *broadcastRequest

	// broadcastAll channel for messages to all connections
	broadcastAll chan []byte

	// mutex for thread-safe operations
	mu sync.RWMutex

	// logger for the hub
	logger zerolog.Logger

	// metrics
	totalConnections   int64
	totalBroadcasts    int64
	totalSubscriptions int64
}

// subscriptionRequest represents a request to subscribe/unsubscribe to a room.
type subscriptionRequest struct {
	conn *Connection
	room string
}

// broadcastRequest represents a request to broadcast a message to a room.
type broadcastRequest struct {
	room    string
	message []byte
}

// HubConfig holds configuration for the WebSocket hub.
type HubConfig struct {
	// MaxConnectionsPerRoom limits connections per room (0 = unlimited)
	MaxConnectionsPerRoom int
	// BroadcastBufferSize is the buffer size for broadcast channels
	BroadcastBufferSize int
}

// DefaultHubConfig returns sensible defaults for hub configuration.
func DefaultHubConfig() HubConfig {
	return HubConfig{
		MaxConnectionsPerRoom: 0,
		BroadcastBufferSize:   256,
	}
}

// NewHub creates a new WebSocket hub.
func NewHub(logger zerolog.Logger) *Hub {
	return NewHubWithConfig(DefaultHubConfig(), logger)
}

// NewHubWithConfig creates a new WebSocket hub with custom configuration.
func NewHubWithConfig(cfg HubConfig, logger zerolog.Logger) *Hub {
	bufferSize := cfg.BroadcastBufferSize
	if bufferSize <= 0 {
		bufferSize = 256
	}

	return &Hub{
		connections:   make(map[*Connection]struct{}),
		rooms:         make(map[string]map[*Connection]struct{}),
		register:      make(chan *Connection, bufferSize),
		unregister:    make(chan *Connection, bufferSize),
		subscribe:     make(chan *subscriptionRequest, bufferSize),
		unsubscribeCh: make(chan *subscriptionRequest, bufferSize),
		broadcast:     make(chan *broadcastRequest, bufferSize),
		broadcastAll:  make(chan []byte, bufferSize),
		logger:        logger.With().Str("component", "websocket_hub").Logger(),
	}
}

// Run starts the hub's main event loop. It blocks until the context is cancelled.
func (h *Hub) Run(ctx context.Context) {
	h.logger.Info().Msg("starting WebSocket hub")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info().Msg("stopping WebSocket hub")
			h.closeAllConnections()
			return

		case conn := <-h.register:
			h.handleRegister(conn)

		case conn := <-h.unregister:
			h.handleUnregister(conn)

		case req := <-h.subscribe:
			h.handleSubscribe(req)

		case req := <-h.unsubscribeCh:
			h.handleUnsubscribe(req)

		case req := <-h.broadcast:
			h.handleBroadcast(req)

		case message := <-h.broadcastAll:
			h.handleBroadcastAll(message)

		case <-ticker.C:
			h.logStats()
		}
	}
}

// Register registers a new connection with the hub.
func (h *Hub) Register(conn *Connection) {
	h.register <- conn
}

// Unregister removes a connection from the hub.
func (h *Hub) Unregister(conn *Connection) {
	h.unregister <- conn
}

// Subscribe subscribes a connection to a room.
func (h *Hub) Subscribe(conn *Connection, room string) {
	h.subscribe <- &subscriptionRequest{conn: conn, room: room}
}

// Unsubscribe unsubscribes a connection from a room.
func (h *Hub) Unsubscribe(conn *Connection, room string) {
	h.unsubscribeCh <- &subscriptionRequest{conn: conn, room: room}
}

// Broadcast sends a message to all connections in a room.
func (h *Hub) Broadcast(room string, message []byte) {
	h.broadcast <- &broadcastRequest{room: room, message: message}
}

// BroadcastAll sends a message to all connected clients.
func (h *Hub) BroadcastAll(message []byte) {
	h.broadcastAll <- message
}

// BroadcastMessage creates and broadcasts a Message to a room.
func (h *Hub) BroadcastMessage(room string, msg *Message) error {
	data, err := msg.Bytes()
	if err != nil {
		return err
	}
	h.Broadcast(room, data)
	return nil
}

// BroadcastMessageAll creates and broadcasts a Message to all connections.
func (h *Hub) BroadcastMessageAll(msg *Message) error {
	data, err := msg.Bytes()
	if err != nil {
		return err
	}
	h.BroadcastAll(data)
	return nil
}

// ConnectionCount returns the current number of connections.
func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}

// RoomCount returns the current number of active rooms.
func (h *Hub) RoomCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms)
}

// RoomConnectionCount returns the number of connections in a specific room.
func (h *Hub) RoomConnectionCount(room string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if conns, ok := h.rooms[room]; ok {
		return len(conns)
	}
	return 0
}

// GetRooms returns a list of all active room names.
func (h *Hub) GetRooms() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	rooms := make([]string, 0, len(h.rooms))
	for room := range h.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

// handleRegister handles a new connection registration.
func (h *Hub) handleRegister(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.connections[conn] = struct{}{}
	h.totalConnections++

	h.logger.Debug().
		Str("conn_id", conn.ID()).
		Int("total_connections", len(h.connections)).
		Msg("connection registered")
}

// handleUnregister handles a connection unregistration.
func (h *Hub) handleUnregister(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.connections[conn]; !ok {
		return
	}

	// Remove from all rooms
	for room, conns := range h.rooms {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.rooms, room)
		}
	}

	delete(h.connections, conn)
	conn.Close()

	h.logger.Debug().
		Str("conn_id", conn.ID()).
		Int("total_connections", len(h.connections)).
		Msg("connection unregistered")
}

// handleSubscribe handles a room subscription request.
func (h *Hub) handleSubscribe(req *subscriptionRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Ensure connection is registered
	if _, ok := h.connections[req.conn]; !ok {
		return
	}

	// Create room if it doesn't exist
	if _, ok := h.rooms[req.room]; !ok {
		h.rooms[req.room] = make(map[*Connection]struct{})
	}

	h.rooms[req.room][req.conn] = struct{}{}
	h.totalSubscriptions++

	h.logger.Debug().
		Str("conn_id", req.conn.ID()).
		Str("room", req.room).
		Int("room_connections", len(h.rooms[req.room])).
		Msg("connection subscribed to room")

	// Send confirmation
	msg, _ := NewRoomMessage(MessageTypeSubscribed, req.room, nil)
	if data, err := msg.Bytes(); err == nil {
		req.conn.Send(data)
	}
}

// handleUnsubscribe handles a room unsubscription request.
func (h *Hub) handleUnsubscribe(req *subscriptionRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conns, ok := h.rooms[req.room]; ok {
		delete(conns, req.conn)
		if len(conns) == 0 {
			delete(h.rooms, req.room)
		}
	}

	h.logger.Debug().
		Str("conn_id", req.conn.ID()).
		Str("room", req.room).
		Msg("connection unsubscribed from room")

	// Send confirmation
	msg, _ := NewRoomMessage(MessageTypeUnsubscribed, req.room, nil)
	if data, err := msg.Bytes(); err == nil {
		req.conn.Send(data)
	}
}

// handleBroadcast handles a room broadcast request.
func (h *Hub) handleBroadcast(req *broadcastRequest) {
	h.mu.RLock()
	conns, ok := h.rooms[req.room]
	if !ok {
		h.mu.RUnlock()
		return
	}

	// Copy connections to avoid holding lock during sends
	targets := make([]*Connection, 0, len(conns))
	for conn := range conns {
		targets = append(targets, conn)
	}
	h.mu.RUnlock()

	h.totalBroadcasts++

	for _, conn := range targets {
		conn.Send(req.message)
	}
}

// handleBroadcastAll handles a broadcast to all connections.
func (h *Hub) handleBroadcastAll(message []byte) {
	h.mu.RLock()
	targets := make([]*Connection, 0, len(h.connections))
	for conn := range h.connections {
		targets = append(targets, conn)
	}
	h.mu.RUnlock()

	h.totalBroadcasts++

	for _, conn := range targets {
		conn.Send(message)
	}
}

// closeAllConnections closes all active connections.
func (h *Hub) closeAllConnections() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for conn := range h.connections {
		conn.Close()
	}

	h.connections = make(map[*Connection]struct{})
	h.rooms = make(map[string]map[*Connection]struct{})

	h.logger.Info().Msg("all connections closed")
}

// logStats logs current hub statistics.
func (h *Hub) logStats() {
	h.mu.RLock()
	connCount := len(h.connections)
	roomCount := len(h.rooms)
	h.mu.RUnlock()

	h.logger.Debug().
		Int("connections", connCount).
		Int("rooms", roomCount).
		Int64("total_connections", h.totalConnections).
		Int64("total_broadcasts", h.totalBroadcasts).
		Int64("total_subscriptions", h.totalSubscriptions).
		Msg("hub statistics")
}

// Stats returns current hub statistics.
func (h *Hub) Stats() HubStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return HubStats{
		ActiveConnections:  len(h.connections),
		ActiveRooms:        len(h.rooms),
		TotalConnections:   h.totalConnections,
		TotalBroadcasts:    h.totalBroadcasts,
		TotalSubscriptions: h.totalSubscriptions,
	}
}

// HubStats holds hub statistics.
type HubStats struct {
	ActiveConnections  int   `json:"active_connections"`
	ActiveRooms        int   `json:"active_rooms"`
	TotalConnections   int64 `json:"total_connections"`
	TotalBroadcasts    int64 `json:"total_broadcasts"`
	TotalSubscriptions int64 `json:"total_subscriptions"`
}

// IsHealthy returns true if the hub is running and healthy.
func (h *Hub) IsHealthy() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.connections != nil
}
