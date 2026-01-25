package agentmgr

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ConnectionState represents the current state of an agent connection.
type ConnectionState int

const (
	ConnectionStateConnecting ConnectionState = iota
	ConnectionStateConnected
	ConnectionStateDisconnecting
	ConnectionStateDisconnected
)

func (s ConnectionState) String() string {
	switch s {
	case ConnectionStateConnecting:
		return "connecting"
	case ConnectionStateConnected:
		return "connected"
	case ConnectionStateDisconnecting:
		return "disconnecting"
	case ConnectionStateDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

// StreamSender defines the interface for sending messages to an agent.
type StreamSender interface {
	// SendAssignment sends a work assignment to the agent.
	SendAssignment(assignment *WorkAssignment) error

	// SendCancel sends a cancellation request to the agent.
	SendCancel(runID uuid.UUID, reason string) error

	// SendDrain sends a drain request to the agent.
	SendDrain(reason string) error

	// SendAck sends an acknowledgement to the agent.
	SendAck(id string, success bool, errorMessage string) error
}

// StreamReceiver defines the interface for receiving messages from an agent.
type StreamReceiver interface {
	// Recv receives the next message from the agent.
	// Returns io.EOF when the stream is closed.
	Recv() (AgentMessage, error)
}

// AgentMessage represents a message received from an agent.
type AgentMessage interface {
	// Type returns the type of message.
	Type() string
}

// AgentConnection manages a bidirectional gRPC stream with an agent.
type AgentConnection struct {
	agentID uuid.UUID
	sender  StreamSender
	logger  *slog.Logger

	mu            sync.RWMutex
	state         ConnectionState
	lastHeartbeat time.Time
	activeRuns    map[uuid.UUID]struct{}
	closeCh       chan struct{}
	closeOnce     sync.Once

	// Callbacks
	onDisconnect func(agentID uuid.UUID)
}

// AgentConnectionConfig holds configuration for an agent connection.
type AgentConnectionConfig struct {
	AgentID      uuid.UUID
	Sender       StreamSender
	Logger       *slog.Logger
	OnDisconnect func(agentID uuid.UUID)
}

// NewAgentConnection creates a new AgentConnection.
func NewAgentConnection(cfg AgentConnectionConfig) *AgentConnection {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &AgentConnection{
		agentID:       cfg.AgentID,
		sender:        cfg.Sender,
		logger:        cfg.Logger.With("agent_id", cfg.AgentID),
		state:         ConnectionStateConnecting,
		lastHeartbeat: time.Now(),
		activeRuns:    make(map[uuid.UUID]struct{}),
		closeCh:       make(chan struct{}),
		onDisconnect:  cfg.OnDisconnect,
	}
}

// AgentID returns the ID of the connected agent.
func (c *AgentConnection) AgentID() uuid.UUID {
	return c.agentID
}

// State returns the current connection state.
func (c *AgentConnection) State() ConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// SetState updates the connection state.
func (c *AgentConnection) SetState(state ConnectionState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = state
	c.logger.Debug("connection state changed", "state", state)
}

// LastHeartbeat returns the timestamp of the last heartbeat.
func (c *AgentConnection) LastHeartbeat() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastHeartbeat
}

// UpdateHeartbeat updates the heartbeat timestamp and active runs.
func (c *AgentConnection) UpdateHeartbeat(status *HeartbeatStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastHeartbeat = time.Now()

	// Update active runs from heartbeat
	c.activeRuns = make(map[uuid.UUID]struct{}, len(status.ActiveRunIDs))
	for _, runID := range status.ActiveRunIDs {
		c.activeRuns[runID] = struct{}{}
	}
}

// ActiveRunCount returns the number of active runs on this agent.
func (c *AgentConnection) ActiveRunCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.activeRuns)
}

// HasActiveRun checks if a specific run is active on this agent.
func (c *AgentConnection) HasActiveRun(runID uuid.UUID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.activeRuns[runID]
	return ok
}

// AddActiveRun adds a run to the active runs set.
func (c *AgentConnection) AddActiveRun(runID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activeRuns[runID] = struct{}{}
}

// RemoveActiveRun removes a run from the active runs set.
func (c *AgentConnection) RemoveActiveRun(runID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.activeRuns, runID)
}

// GetActiveRuns returns a copy of the active run IDs.
func (c *AgentConnection) GetActiveRuns() []uuid.UUID {
	c.mu.RLock()
	defer c.mu.RUnlock()

	runs := make([]uuid.UUID, 0, len(c.activeRuns))
	for runID := range c.activeRuns {
		runs = append(runs, runID)
	}
	return runs
}

// SendAssignment sends a work assignment to the agent.
func (c *AgentConnection) SendAssignment(ctx context.Context, assignment *WorkAssignment) error {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()

	if state != ConnectionStateConnected {
		return fmt.Errorf("connection not in connected state: %s", state)
	}

	select {
	case <-c.closeCh:
		return fmt.Errorf("connection closed")
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := c.sender.SendAssignment(assignment); err != nil {
		return fmt.Errorf("failed to send assignment: %w", err)
	}

	// Track as active run
	c.AddActiveRun(assignment.Run.ID)

	c.logger.Info("sent work assignment",
		"run_id", assignment.Run.ID,
		"service_id", assignment.Service.ID,
	)

	return nil
}

// SendCancel sends a cancellation request to the agent.
func (c *AgentConnection) SendCancel(ctx context.Context, runID uuid.UUID, reason string) error {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()

	if state != ConnectionStateConnected {
		return fmt.Errorf("connection not in connected state: %s", state)
	}

	select {
	case <-c.closeCh:
		return fmt.Errorf("connection closed")
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := c.sender.SendCancel(runID, reason); err != nil {
		return fmt.Errorf("failed to send cancel: %w", err)
	}

	c.logger.Info("sent cancel request",
		"run_id", runID,
		"reason", reason,
	)

	return nil
}

// SendDrain sends a drain request to the agent.
func (c *AgentConnection) SendDrain(ctx context.Context, reason string) error {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()

	if state != ConnectionStateConnected {
		return fmt.Errorf("connection not in connected state: %s", state)
	}

	select {
	case <-c.closeCh:
		return fmt.Errorf("connection closed")
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := c.sender.SendDrain(reason); err != nil {
		return fmt.Errorf("failed to send drain: %w", err)
	}

	c.logger.Info("sent drain request", "reason", reason)

	return nil
}

// Close closes the connection.
func (c *AgentConnection) Close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.state = ConnectionStateDisconnected
		c.mu.Unlock()

		close(c.closeCh)

		if c.onDisconnect != nil {
			c.onDisconnect(c.agentID)
		}

		c.logger.Info("connection closed")
	})
}

// Done returns a channel that's closed when the connection is closed.
func (c *AgentConnection) Done() <-chan struct{} {
	return c.closeCh
}

// IsClosed returns true if the connection is closed.
func (c *AgentConnection) IsClosed() bool {
	select {
	case <-c.closeCh:
		return true
	default:
		return false
	}
}

// ConnectionPool manages a pool of agent connections.
type ConnectionPool struct {
	mu          sync.RWMutex
	connections map[uuid.UUID]*AgentConnection
	logger      *slog.Logger
}

// NewConnectionPool creates a new ConnectionPool.
func NewConnectionPool(logger *slog.Logger) *ConnectionPool {
	if logger == nil {
		logger = slog.Default()
	}

	return &ConnectionPool{
		connections: make(map[uuid.UUID]*AgentConnection),
		logger:      logger.With("component", "connection_pool"),
	}
}

// Add adds a connection to the pool.
func (p *ConnectionPool) Add(conn *AgentConnection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Close existing connection if any
	if existing, ok := p.connections[conn.agentID]; ok {
		existing.Close()
	}

	p.connections[conn.agentID] = conn
	p.logger.Debug("connection added", "agent_id", conn.agentID)
}

// Get returns a connection by agent ID.
func (p *ConnectionPool) Get(agentID uuid.UUID) (*AgentConnection, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	conn, ok := p.connections[agentID]
	return conn, ok
}

// Remove removes a connection from the pool.
func (p *ConnectionPool) Remove(agentID uuid.UUID) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if conn, ok := p.connections[agentID]; ok {
		conn.Close()
		delete(p.connections, agentID)
		p.logger.Debug("connection removed", "agent_id", agentID)
	}
}

// Size returns the number of connections in the pool.
func (p *ConnectionPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.connections)
}

// GetAll returns all connections.
func (p *ConnectionPool) GetAll() []*AgentConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	conns := make([]*AgentConnection, 0, len(p.connections))
	for _, conn := range p.connections {
		conns = append(conns, conn)
	}
	return conns
}

// CloseAll closes all connections in the pool.
func (p *ConnectionPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, conn := range p.connections {
		conn.Close()
		delete(p.connections, id)
	}

	p.logger.Info("closed all connections")
}

// HandleReceive processes messages received from an agent connection.
// This should be called in a goroutine for each connection.
func (c *AgentConnection) HandleReceive(ctx context.Context, receiver StreamReceiver, handler MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.closeCh:
			return nil
		default:
		}

		msg, err := receiver.Recv()
		if err == io.EOF {
			c.logger.Info("stream closed by agent")
			return nil
		}
		if err != nil {
			c.logger.Error("error receiving message", "error", err)
			return err
		}

		if err := handler.HandleMessage(ctx, c.agentID, msg); err != nil {
			c.logger.Error("error handling message",
				"type", msg.Type(),
				"error", err,
			)
			// Continue processing other messages
		}
	}
}

// MessageHandler processes messages from agents.
type MessageHandler interface {
	HandleMessage(ctx context.Context, agentID uuid.UUID, msg AgentMessage) error
}
