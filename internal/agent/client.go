// Package agent provides the Conductor agent implementation.
package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

// Client manages the gRPC connection to the control plane.
type Client struct {
	config *Config
	logger zerolog.Logger

	conn   *grpc.ClientConn
	client conductorv1.AgentServiceClient
	stream conductorv1.AgentService_WorkStreamClient

	mu sync.RWMutex

	// Reconnection state
	reconnectAttempt int
}

// WorkStream wraps the bidirectional gRPC stream for type-safe operations.
type WorkStream struct {
	stream conductorv1.AgentService_WorkStreamClient
	mu     sync.Mutex
}

// NewClient creates a new control plane client.
func NewClient(cfg *Config, logger zerolog.Logger) *Client {
	return &Client{
		config: cfg,
		logger: logger.With().Str("component", "client").Logger(),
	}
}

// Connect establishes the gRPC connection to the control plane.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Close existing connection if any
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.client = nil
	}

	// Configure transport credentials
	var creds credentials.TransportCredentials
	if c.config.TLSEnabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: c.config.TLSInsecureSkipVerify,
		}
		creds = credentials.NewTLS(tlsConfig)
	} else {
		creds = insecure.NewCredentials()
	}

	// Configure keepalive
	keepaliveParams := keepalive.ClientParameters{
		Time:                30 * time.Second,
		Timeout:             10 * time.Second,
		PermitWithoutStream: true,
	}

	// Dial options
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithKeepaliveParams(keepaliveParams),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(16*1024*1024), // 16MB
			grpc.MaxCallSendMsgSize(16*1024*1024), // 16MB
		),
	}

	c.logger.Debug().
		Str("url", c.config.ControlPlaneURL).
		Bool("tls", c.config.TLSEnabled).
		Msg("Connecting to control plane")

	// Create connection with timeout
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(connectCtx, c.config.ControlPlaneURL, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.client = conductorv1.NewAgentServiceClient(conn)
	c.reconnectAttempt = 0

	c.logger.Info().Msg("Connected to control plane")
	return nil
}

// WorkStream opens a bidirectional streaming connection for work assignments.
func (c *Client) WorkStream(ctx context.Context) (*WorkStream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return nil, errors.New("client not connected")
	}

	// Add authentication metadata
	md := metadata.New(map[string]string{
		"authorization": "Bearer " + c.config.AgentToken,
		"agent-id":      c.config.AgentID,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	stream, err := c.client.WorkStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open work stream: %w", err)
	}

	c.stream = stream
	return &WorkStream{stream: stream}, nil
}

// Send sends a message to the control plane.
func (c *Client) Send(msg *conductorv1.AgentMessage) error {
	c.mu.RLock()
	stream := c.stream
	c.mu.RUnlock()

	if stream == nil {
		return errors.New("stream not open")
	}

	return stream.Send(msg)
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stream != nil {
		_ = c.stream.CloseSend()
		c.stream = nil
	}

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.client = nil
		return err
	}

	return nil
}

// NextReconnectInterval returns the next reconnection interval using exponential backoff.
func (c *Client) NextReconnectInterval() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.reconnectAttempt++

	// Exponential backoff with jitter
	baseInterval := float64(c.config.ReconnectMinInterval)
	maxInterval := float64(c.config.ReconnectMaxInterval)

	// Calculate exponential backoff: min * 2^attempt
	interval := baseInterval * math.Pow(2, float64(c.reconnectAttempt-1))

	// Cap at max interval
	if interval > maxInterval {
		interval = maxInterval
	}

	// Add jitter (±10%)
	jitter := interval * 0.1
	interval = interval - jitter + (jitter * 2 * float64(time.Now().UnixNano()%100) / 100)

	return time.Duration(interval)
}

// ResetReconnectInterval resets the reconnection attempt counter.
func (c *Client) ResetReconnectInterval() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reconnectAttempt = 0
}

// Send sends a message on the work stream.
func (w *WorkStream) Send(msg *conductorv1.AgentMessage) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stream.Send(msg)
}

// Receive receives a message from the work stream.
func (w *WorkStream) Receive() (*conductorv1.ControlMessage, error) {
	msg, err := w.stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("stream closed by server: %w", err)
		}
		return nil, err
	}
	return msg, nil
}

// CloseSend closes the send side of the stream.
func (w *WorkStream) CloseSend() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stream.CloseSend()
}
