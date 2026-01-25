package websocket

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

const (
	// writeWait is the time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// pongWait is the time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// pingPeriod is the period at which pings are sent. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// maxMessageSize is the maximum message size allowed from peer.
	maxMessageSize = 512 * 1024 // 512KB

	// sendBufferSize is the buffer size for the send channel.
	sendBufferSize = 256
)

// Connection wraps a WebSocket connection with read/write pumps and hub integration.
type Connection struct {
	// id is a unique identifier for this connection
	id string

	// hub is the WebSocket hub this connection belongs to
	hub *Hub

	// conn is the underlying WebSocket connection
	conn *websocket.Conn

	// send is the buffered channel for outbound messages
	send chan []byte

	// rooms tracks which rooms this connection is subscribed to
	rooms map[string]struct{}

	// userID is the authenticated user's ID (if any)
	userID string

	// claims holds user authentication claims
	claims map[string]interface{}

	// mu protects connection state
	mu sync.RWMutex

	// closed indicates if the connection is closed
	closed bool

	// logger for this connection
	logger zerolog.Logger

	// connectedAt is when the connection was established
	connectedAt time.Time

	// lastActivity is the time of the last activity on this connection
	lastActivity time.Time
}

// ConnectionOption configures a Connection.
type ConnectionOption func(*Connection)

// WithUserID sets the user ID for the connection.
func WithUserID(userID string) ConnectionOption {
	return func(c *Connection) {
		c.userID = userID
	}
}

// WithClaims sets the authentication claims for the connection.
func WithClaims(claims map[string]interface{}) ConnectionOption {
	return func(c *Connection) {
		c.claims = claims
	}
}

// NewConnection creates a new Connection wrapper.
func NewConnection(ws *websocket.Conn, hub *Hub, logger zerolog.Logger, opts ...ConnectionOption) *Connection {
	now := time.Now()
	c := &Connection{
		id:           uuid.New().String(),
		hub:          hub,
		conn:         ws,
		send:         make(chan []byte, sendBufferSize),
		rooms:        make(map[string]struct{}),
		logger:       logger.With().Str("component", "websocket_conn").Logger(),
		connectedAt:  now,
		lastActivity: now,
	}

	for _, opt := range opts {
		opt(c)
	}

	c.logger = c.logger.With().Str("conn_id", c.id).Logger()
	if c.userID != "" {
		c.logger = c.logger.With().Str("user_id", c.userID).Logger()
	}

	return c
}

// ID returns the connection's unique identifier.
func (c *Connection) ID() string {
	return c.id
}

// UserID returns the authenticated user's ID.
func (c *Connection) UserID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userID
}

// Claims returns the authentication claims.
func (c *Connection) Claims() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.claims
}

// ConnectedAt returns when the connection was established.
func (c *Connection) ConnectedAt() time.Time {
	return c.connectedAt
}

// LastActivity returns the time of the last activity.
func (c *Connection) LastActivity() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastActivity
}

// IsClosed returns true if the connection is closed.
func (c *Connection) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// Rooms returns the list of rooms this connection is subscribed to.
func (c *Connection) Rooms() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	rooms := make([]string, 0, len(c.rooms))
	for room := range c.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

// Send queues a message to be sent to the client.
// Returns false if the connection is closed or the buffer is full.
func (c *Connection) Send(message []byte) bool {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return false
	}
	c.mu.RUnlock()

	select {
	case c.send <- message:
		return true
	default:
		// Buffer full, connection is too slow
		c.logger.Warn().Msg("send buffer full, dropping message")
		return false
	}
}

// Close closes the connection and removes it from the hub.
func (c *Connection) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()

	close(c.send)
	c.conn.Close()

	c.logger.Debug().Msg("connection closed")
}

// ReadPump pumps messages from the WebSocket connection to the hub.
// It runs in its own goroutine and handles incoming messages.
func (c *Connection) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.mu.Lock()
		c.lastActivity = time.Now()
		c.mu.Unlock()
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				c.logger.Debug().Err(err).Msg("unexpected close error")
			}
			break
		}

		c.mu.Lock()
		c.lastActivity = time.Now()
		c.mu.Unlock()

		c.handleMessage(message)
	}
}

// WritePump pumps messages from the send channel to the WebSocket connection.
// It runs in its own goroutine and handles outgoing messages.
func (c *Connection) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Batch additional queued messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes an incoming message from the client.
func (c *Connection) handleMessage(data []byte) {
	msg, err := ParseMessage(data)
	if err != nil {
		c.logger.Debug().Err(err).Msg("failed to parse message")
		c.sendError("invalid_message", "failed to parse message")
		return
	}

	c.logger.Debug().
		Str("type", string(msg.Type)).
		Str("room", msg.Room).
		Msg("received message")

	switch msg.Type {
	case MessageTypeSubscribe:
		c.handleSubscribe(msg)
	case MessageTypeUnsubscribe:
		c.handleUnsubscribe(msg)
	case MessageTypePing:
		c.handlePing()
	default:
		c.logger.Debug().Str("type", string(msg.Type)).Msg("unknown message type")
	}
}

// handleSubscribe handles a subscribe request.
func (c *Connection) handleSubscribe(msg *Message) {
	room := msg.Room
	if room == "" {
		c.sendError("invalid_room", "room is required for subscribe")
		return
	}

	c.mu.Lock()
	c.rooms[room] = struct{}{}
	c.mu.Unlock()

	c.hub.Subscribe(c, room)

	c.logger.Debug().Str("room", room).Msg("subscribed to room")
}

// handleUnsubscribe handles an unsubscribe request.
func (c *Connection) handleUnsubscribe(msg *Message) {
	room := msg.Room
	if room == "" {
		c.sendError("invalid_room", "room is required for unsubscribe")
		return
	}

	c.mu.Lock()
	delete(c.rooms, room)
	c.mu.Unlock()

	c.hub.Unsubscribe(c, room)

	c.logger.Debug().Str("room", room).Msg("unsubscribed from room")
}

// handlePing handles a ping message by sending a pong.
func (c *Connection) handlePing() {
	msg, _ := NewMessage(MessageTypePong, nil)
	if data, err := msg.Bytes(); err == nil {
		c.Send(data)
	}
}

// sendError sends an error message to the client.
func (c *Connection) sendError(code, message string) {
	msg, _ := NewMessage(MessageTypeError, ErrorPayload{
		Code:    code,
		Message: message,
	})
	if data, err := msg.Bytes(); err == nil {
		c.Send(data)
	}
}
