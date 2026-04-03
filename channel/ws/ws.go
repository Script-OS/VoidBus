// Package ws provides a WebSocket channel implementation.
//
// WebSocket channel uses WebSocket protocol for communication.
// It provides:
// - Reliable, ordered delivery (inherited from TCP)
// - Firewall-friendly (HTTP-based)
// - Message framing built-in (no length-prefix needed)
// - Ping/Pong heartbeat support
//
// Design Constraints:
// - WebSocket channel MUST NOT handle serialization/encoding/fragmentation
// - WebSocket channel MUST NOT be exposed in metadata protocols
// - Message framing is handled by WebSocket protocol
package ws

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/internal"
)

const (
	// ChannelType is the type identifier for WebSocket channels
	ChannelType = channel.TypeWS
	// DefaultBufferSize is the default buffer size
	DefaultBufferSize = 4096
	// MaxMessageSize is the maximum message size (same as TCP)
	MaxMessageSize = 16 * 1024 * 1024
	// DefaultPingInterval is the default ping interval
	DefaultPingInterval = 30 * time.Second
	// DefaultPongTimeout is the default pong wait timeout
	DefaultPongTimeout = 60 * time.Second
)

// ClientChannel implements channel.Channel for WebSocket client connections.
// Note: gorilla/websocket WriteMessage is NOT thread-safe, so we use writeMu for concurrent-safe writes.
type ClientChannel struct {
	conn         *websocket.Conn
	config       channel.ChannelConfig
	mu           sync.RWMutex // State management (closed, connected)
	writeMu      sync.Mutex   // Write serialization (gorilla/websocket is NOT thread-safe)
	closed       bool
	connected    bool
	id           string
	lastActivity time.Time
}

// NewClientChannel creates a new WebSocket client channel.
func NewClientChannel(config channel.ChannelConfig) (*ClientChannel, error) {
	if config.Address == "" {
		return nil, errors.New("ws: address required")
	}

	// WebSocket dialer
	dialer := websocket.DefaultDialer
	timeout := config.ConnectTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	dialer.HandshakeTimeout = timeout

	// Connect to WebSocket server
	conn, _, err := dialer.Dial(config.Address, nil)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "connect",
			Err:       err,
			Msg:       "failed to connect to " + config.Address,
			Retryable: true,
		}
	}

	// Set pong handler
	conn.SetReadDeadline(time.Now().Add(DefaultPongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(DefaultPongTimeout))
		return nil
	})

	ch := &ClientChannel{
		conn:         conn,
		config:       config,
		connected:    true,
		id:           internal.GenerateID(),
		lastActivity: time.Now(),
	}

	return ch, nil
}

// Send implements channel.Channel.Send.
// Uses writeMu for concurrent-safe writes (gorilla/websocket WriteMessage is NOT thread-safe).
func (c *ClientChannel) Send(data []byte) error {
	// Step 1: Check state (quick check without holding write lock)
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return channel.ErrChannelClosed
	}
	if !c.connected {
		c.mu.RUnlock()
		return channel.ErrChannelDisconnected
	}
	c.mu.RUnlock()

	// Step 2: Acquire write mutex for actual write operation
	c.writeMu.Lock()

	// Step 3: Re-check state after acquiring write lock (may have changed during wait)
	c.mu.RLock()
	if c.closed || !c.connected {
		c.mu.RUnlock()
		c.writeMu.Unlock()
		return channel.ErrChannelDisconnected
	}
	conn := c.conn
	c.mu.RUnlock()

	// Step 4: Perform actual write
	if len(data) > MaxMessageSize {
		c.writeMu.Unlock()
		return errors.New("ws: data exceeds max message size")
	}

	err := conn.WriteMessage(websocket.BinaryMessage, data)
	c.writeMu.Unlock()

	// Step 5: Handle result
	if err != nil {
		c.handleDisconnect()
		return &channel.ChannelError{
			Op:        "send",
			Err:       err,
			Msg:       "failed to write message",
			Retryable: false,
		}
	}

	c.mu.Lock()
	c.lastActivity = time.Now()
	c.mu.Unlock()
	return nil
}

// Receive implements channel.Channel.Receive.
func (c *ClientChannel) Receive() ([]byte, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, channel.ErrChannelClosed
	}
	if !c.connected {
		c.mu.RUnlock()
		return nil, channel.ErrChannelDisconnected
	}
	conn := c.conn
	c.mu.RUnlock()

	messageType, data, err := conn.ReadMessage()
	if err != nil {
		c.handleDisconnect()
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			return nil, channel.ErrChannelDisconnected
		}
		return nil, &channel.ChannelError{
			Op:        "receive",
			Err:       err,
			Msg:       "failed to read message",
			Retryable: false,
		}
	}

	if messageType != websocket.BinaryMessage {
		return nil, errors.New("ws: unexpected message type")
	}

	c.mu.RLock()
	c.lastActivity = time.Now()
	c.mu.RUnlock()

	return data, nil
}

// Close implements channel.Channel.Close.
func (c *ClientChannel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return channel.ErrChannelClosed
	}

	c.closed = true
	c.connected = false

	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		return c.conn.Close()
	}
	return nil
}

// IsConnected implements channel.Channel.IsConnected.
func (c *ClientChannel) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && !c.closed
}

// Type implements channel.Channel.Type.
func (c *ClientChannel) Type() channel.ChannelType {
	return ChannelType
}

// DefaultMTU implements channel.Channel.DefaultMTU.
func (c *ClientChannel) DefaultMTU() int {
	return 64 * 1024
}

// IsReliable implements channel.Channel.IsReliable.
func (c *ClientChannel) IsReliable() bool {
	return true
}

// AckTimeout implements channel.Channel.AckTimeout.
func (c *ClientChannel) AckTimeout() time.Duration {
	return 0
}

// handleDisconnect handles connection disconnect.
func (c *ClientChannel) handleDisconnect() {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
}

// ServerChannel implements channel.ServerChannel for WebSocket server.
type ServerChannel struct {
	upgrader    *websocket.Upgrader
	config      channel.ChannelConfig
	mu          sync.RWMutex
	closed      bool
	clients     map[string]*AcceptedChannel
	clientCount int
	// HTTP server for Accept() support
	httpServer  *http.Server
	acceptChan  chan *AcceptedChannel
	acceptError chan error
	listener    net.Listener
}

// NewServerChannel creates a new WebSocket server channel.
func NewServerChannel(config channel.ChannelConfig) (*ServerChannel, error) {
	if config.Address == "" {
		return nil, errors.New("ws: address required")
	}

	upgrader := &websocket.Upgrader{
		ReadBufferSize:  DefaultBufferSize,
		WriteBufferSize: DefaultBufferSize,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	s := &ServerChannel{
		upgrader:    upgrader,
		config:      config,
		clients:     make(map[string]*AcceptedChannel),
		acceptChan:  make(chan *AcceptedChannel, 10),
		acceptError: make(chan error, 1),
	}

	// Create HTTP handler
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		client, err := s.Upgrade(w, r)
		if err != nil {
			s.acceptError <- err
			return
		}
		s.acceptChan <- client.(*AcceptedChannel)
	})

	// Start HTTP server
	s.httpServer = &http.Server{
		Addr:    config.Address,
		Handler: handler,
	}

	// Start listener
	listener, err := net.Listen("tcp", config.Address)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "listen",
			Err:       err,
			Msg:       "failed to listen on " + config.Address,
			Retryable: false,
		}
	}
	s.listener = listener

	// Start HTTP server in goroutine
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			// Server error (not graceful shutdown)
			s.acceptError <- err
		}
	}()

	return s, nil
}

// Accept implements channel.ServerChannel.Accept.
func (s *ServerChannel) Accept() (channel.Channel, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, channel.ErrChannelClosed
	}
	s.mu.RUnlock()

	// Wait for connection from acceptChan or error from acceptError.
	// When ServerChannel is closed, both channels are closed, which causes
	// reads to return zero values (nil client / nil error). We must detect
	// this via the 'ok' flag and return ErrChannelClosed.
	select {
	case client, ok := <-s.acceptChan:
		if !ok {
			// acceptChan was closed — server is shutting down
			return nil, channel.ErrChannelClosed
		}
		return client, nil
	case err, ok := <-s.acceptError:
		if !ok {
			// acceptError was closed — server is shutting down
			return nil, channel.ErrChannelClosed
		}
		return nil, err
	}
}

// Send implements channel.Channel.Send.
func (s *ServerChannel) Send(data []byte) error {
	return errors.New("ws: server channel cannot send directly")
}

// Receive implements channel.Channel.Receive.
func (s *ServerChannel) Receive() ([]byte, error) {
	return nil, errors.New("ws: server channel cannot receive directly")
}

// Close implements channel.Channel.Close.
// IMPORTANT: This method follows the "copy-release-operate" pattern to avoid deadlock.
// We copy the client list before releasing the lock, then close clients outside of lock.
func (s *ServerChannel) Close() error {
	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()
		return channel.ErrChannelClosed
	}

	s.closed = true

	// Close HTTP server
	if s.httpServer != nil {
		s.httpServer.Close()
	}

	// Close listener
	if s.listener != nil {
		s.listener.Close()
	}

	// Copy clients to close outside of lock to avoid deadlock
	// client.Close() may call removeClient which tries to acquire s.mu
	clients := make([]*AcceptedChannel, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	s.clients = make(map[string]*AcceptedChannel)

	// Close accept channels
	close(s.acceptChan)
	close(s.acceptError)

	s.mu.Unlock() // Release lock BEFORE closing clients

	// Close all client connections outside of lock
	for _, client := range clients {
		client.Close()
	}

	return nil
}

// IsConnected implements channel.Channel.IsConnected.
func (s *ServerChannel) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.closed
}

// Type implements channel.Channel.Type.
func (s *ServerChannel) Type() channel.ChannelType {
	return ChannelType
}

// DefaultMTU implements channel.Channel.DefaultMTU.
func (s *ServerChannel) DefaultMTU() int {
	return 0
}

// IsReliable implements channel.Channel.IsReliable.
func (s *ServerChannel) IsReliable() bool {
	return true
}

// AckTimeout implements channel.Channel.AckTimeout.
func (s *ServerChannel) AckTimeout() time.Duration {
	return 0
}

// ListenAddress implements channel.ServerChannel.ListenAddress.
func (s *ServerChannel) ListenAddress() string {
	return s.config.Address
}

// Upgrade upgrades HTTP connection to WebSocket.
func (s *ServerChannel) Upgrade(w http.ResponseWriter, r *http.Request) (channel.Channel, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, channel.ErrChannelClosed
	}
	s.mu.RUnlock()

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "upgrade",
			Err:       err,
			Msg:       "failed to upgrade connection",
			Retryable: false,
		}
	}

	conn.SetReadDeadline(time.Now().Add(DefaultPongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(DefaultPongTimeout))
		return nil
	})

	client := &AcceptedChannel{
		conn:         conn,
		server:       s,
		id:           internal.GenerateID(),
		lastActivity: time.Now(),
		connected:    true,
	}

	s.mu.Lock()
	s.clients[client.id] = client
	s.clientCount++
	s.mu.Unlock()

	return client, nil
}

// removeClient removes a client from server.
func (s *ServerChannel) removeClient(id string) {
	s.mu.Lock()
	delete(s.clients, id)
	s.clientCount--
	s.mu.Unlock()
}

// AcceptedChannel represents an accepted WebSocket connection.
// Note: gorilla/websocket WriteMessage is NOT thread-safe, so we use writeMu for concurrent-safe writes.
type AcceptedChannel struct {
	conn         *websocket.Conn
	server       *ServerChannel
	mu           sync.RWMutex // State management
	writeMu      sync.Mutex   // Write serialization
	closed       bool
	connected    bool
	id           string
	lastActivity time.Time
}

// Send implements channel.Channel.Send.
// Uses writeMu for concurrent-safe writes (gorilla/websocket WriteMessage is NOT thread-safe).
func (a *AcceptedChannel) Send(data []byte) error {
	// Step 1: Check state
	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()
		return channel.ErrChannelClosed
	}
	if !a.connected {
		a.mu.RUnlock()
		return channel.ErrChannelDisconnected
	}
	a.mu.RUnlock()

	// Step 2: Acquire write mutex
	a.writeMu.Lock()

	// Step 3: Re-check state after acquiring write lock
	a.mu.RLock()
	if a.closed || !a.connected {
		a.mu.RUnlock()
		a.writeMu.Unlock()
		return channel.ErrChannelDisconnected
	}
	conn := a.conn
	a.mu.RUnlock()

	// Step 4: Perform write
	if len(data) > MaxMessageSize {
		a.writeMu.Unlock()
		return errors.New("ws: data exceeds max message size")
	}

	err := conn.WriteMessage(websocket.BinaryMessage, data)
	a.writeMu.Unlock()

	// Step 5: Handle result
	if err != nil {
		a.handleDisconnect()
		return &channel.ChannelError{
			Op:        "send",
			Err:       err,
			Msg:       "failed to write message",
			Retryable: false,
		}
	}

	a.mu.Lock()
	a.lastActivity = time.Now()
	a.mu.Unlock()
	return nil
}

// Receive implements channel.Channel.Receive.
func (a *AcceptedChannel) Receive() ([]byte, error) {
	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()
		return nil, channel.ErrChannelClosed
	}
	if !a.connected {
		a.mu.RUnlock()
		return nil, channel.ErrChannelDisconnected
	}
	conn := a.conn
	a.mu.RUnlock()

	messageType, data, err := conn.ReadMessage()
	if err != nil {
		a.handleDisconnect()
		return nil, &channel.ChannelError{
			Op:        "receive",
			Err:       err,
			Msg:       "failed to read message",
			Retryable: false,
		}
	}

	if messageType != websocket.BinaryMessage {
		return nil, errors.New("ws: unexpected message type")
	}

	a.mu.RLock()
	a.lastActivity = time.Now()
	a.mu.RUnlock()

	return data, nil
}

// Close implements channel.Channel.Close.
// IMPORTANT: This method follows the "copy-release-operate" pattern to avoid deadlock.
// Lock order must be consistent: server.mu → client.mu (acquired by removeClient).
// If we hold client.mu and call server.removeClient(), we reverse this order → deadlock.
func (a *AcceptedChannel) Close() error {
	a.mu.Lock()

	if a.closed {
		a.mu.Unlock()
		return channel.ErrChannelClosed
	}

	a.closed = true
	a.connected = false

	// Copy references before releasing lock to avoid deadlock
	server := a.server
	id := a.id
	conn := a.conn

	a.mu.Unlock() // Release lock BEFORE calling server.removeClient()

	// Now operate outside of lock - safe from deadlock
	if server != nil {
		server.removeClient(id) // This acquires server.mu, no client.mu held
	}

	if conn != nil {
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		return conn.Close()
	}
	return nil
}

// IsConnected implements channel.Channel.IsConnected.
func (a *AcceptedChannel) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected && !a.closed
}

// Type implements channel.Channel.Type.
func (a *AcceptedChannel) Type() channel.ChannelType {
	return ChannelType
}

// DefaultMTU implements channel.Channel.DefaultMTU.
func (a *AcceptedChannel) DefaultMTU() int {
	return 64 * 1024
}

// IsReliable implements channel.Channel.IsReliable.
func (a *AcceptedChannel) IsReliable() bool {
	return true
}

// AckTimeout implements channel.Channel.AckTimeout.
func (a *AcceptedChannel) AckTimeout() time.Duration {
	return 0
}

// handleDisconnect handles connection disconnect.
func (a *AcceptedChannel) handleDisconnect() {
	a.mu.Lock()
	a.connected = false
	a.mu.Unlock()
}

// Module implements channel.ChannelModule.
type Module struct{}

// NewModule creates a new WebSocket module.
func NewModule() *Module {
	return &Module{}
}

// CreateClient implements channel.ChannelModule.CreateClient.
func (m *Module) CreateClient(config channel.ChannelConfig) (channel.Channel, error) {
	return NewClientChannel(config)
}

// CreateServer implements channel.ChannelModule.CreateServer.
func (m *Module) CreateServer(config channel.ChannelConfig) (channel.ServerChannel, error) {
	return NewServerChannel(config)
}

// Type implements channel.ChannelModule.Type.
func (m *Module) Type() channel.ChannelType {
	return ChannelType
}
