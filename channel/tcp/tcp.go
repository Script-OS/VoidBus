// Package tcp provides a TCP channel implementation.
//
// TCP channel uses standard TCP/IP for reliable data transmission.
// It provides:
// - Reliable, ordered delivery of data
// - Connection-oriented communication
// - Automatic reconnection support
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.4):
// - TCP channel MUST NOT handle serialization/encoding/fragmentation
// - TCP channel MUST NOT be exposed in metadata protocols
// - Data frames use length-prefix protocol for message boundaries
package tcp

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/internal"
)

const (
	// ChannelType is the type identifier for TCP channels
	ChannelType = channel.TypeTCP
	// DefaultBufferSize is the default buffer size
	DefaultBufferSize = 4096
	// MaxFrameSize is the maximum frame size (16MB)
	MaxFrameSize = 16 * 1024 * 1024
	// LengthPrefixSize is the size of length prefix (4 bytes)
	LengthPrefixSize = 4
)

// Frame protocol:
// [4 bytes: length] [N bytes: data]
// Length is uint32 in little-endian format

// ClientChannel implements channel.Channel for TCP client connections.
// Note: net.Conn.Write is NOT thread-safe for concurrent writes, so we use writeMu.
type ClientChannel struct {
	conn         net.Conn
	config       channel.ChannelConfig
	mu           sync.RWMutex // State management
	writeMu      sync.Mutex   // Write serialization
	closed       bool
	connected    bool
	id           string
	lastActivity time.Time
}

// NewClientChannel creates a new TCP client channel.
//
// Parameter Constraints:
//   - config.Address MUST be valid TCP address format (host:port)
//   - config.BufferSize default to 4096 if 0
//
// Return Guarantees:
//   - On success: returns connected Channel instance
//   - On failure: returns nil and error
func NewClientChannel(config channel.ChannelConfig) (*ClientChannel, error) {
	if config.Address == "" {
		return nil, errors.New("tcp: address required")
	}

	bufferSize := config.BufferSize
	if bufferSize <= 0 {
		bufferSize = DefaultBufferSize
	}

	// Establish connection
	timeout := time.Duration(config.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second // default connect timeout
	}

	conn, err := net.DialTimeout("tcp", config.Address, timeout)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "connect",
			Err:       err,
			Msg:       "failed to connect to " + config.Address,
			Retryable: true,
		}
	}

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
// Sends data with length-prefix framing.
//
// Parameter Constraints:
//   - data: MUST be non-nil byte slice
//   - data length MUST be <= MaxFrameSize (16MB)
//
// Return Guarantees:
//   - On success: complete data frame sent to peer
func (c *ClientChannel) Send(data []byte) error {
	// Step 1: Check state
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

	// Step 2: Acquire write mutex
	c.writeMu.Lock()

	// Step 3: Re-check state after acquiring write lock
	c.mu.RLock()
	if c.closed || !c.connected {
		c.mu.RUnlock()
		c.writeMu.Unlock()
		return channel.ErrChannelDisconnected
	}
	conn := c.conn
	c.mu.RUnlock()

	// Step 4: Perform write (length prefix + data atomically)
	if len(data) > MaxFrameSize {
		c.writeMu.Unlock()
		return errors.New("tcp: data exceeds max frame size")
	}

	// Write length prefix
	lengthBuf := make([]byte, LengthPrefixSize)
	binary.LittleEndian.PutUint32(lengthBuf, uint32(len(data)))

	writeErr := func() error {
		if _, err := conn.Write(lengthBuf); err != nil {
			return err
		}
		if len(data) > 0 {
			if _, err := conn.Write(data); err != nil {
				return err
			}
		}
		return nil
	}()

	c.writeMu.Unlock()

	// Step 5: Handle result
	if writeErr != nil {
		c.handleDisconnect()
		return &channel.ChannelError{
			Op:        "send",
			Err:       writeErr,
			Msg:       "failed to write data",
			Retryable: false,
		}
	}

	c.mu.Lock()
	c.lastActivity = time.Now()
	c.mu.Unlock()
	return nil
}

// Receive implements channel.Channel.Receive.
// Receives data with length-prefix framing.
//
// Return Guarantees:
//   - On success: returns complete data frame
//
// Important: This method does NOT hold the read lock during blocking I/O
// to avoid deadlock with Close(). Close() needs the write lock to proceed.
func (c *ClientChannel) Receive() ([]byte, error) {
	// Check connection status under read lock
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

	// Release read lock before blocking I/O to allow Close() to proceed
	// If connection is closed during read, io.ReadFull will return error

	// Read length prefix (blocking)
	lengthBuf := make([]byte, LengthPrefixSize)
	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		c.handleDisconnect()
		if errors.Is(err, io.EOF) {
			return nil, channel.ErrChannelDisconnected
		}
		// Check if connection was closed
		if errors.Is(err, net.ErrClosed) {
			return nil, channel.ErrChannelClosed
		}
		return nil, &channel.ChannelError{
			Op:        "receive",
			Err:       err,
			Msg:       "failed to read length prefix",
			Retryable: false,
		}
	}

	length := binary.LittleEndian.Uint32(lengthBuf)
	if length > MaxFrameSize {
		return nil, errors.New("tcp: received frame exceeds max size")
	}

	// Read data (blocking)
	data := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(conn, data); err != nil {
			c.handleDisconnect()
			return nil, &channel.ChannelError{
				Op:        "receive",
				Err:       err,
				Msg:       "failed to read data",
				Retryable: false,
			}
		}
	}

	// Update last activity under read lock
	c.mu.RLock()
	c.lastActivity = time.Now()
	c.mu.RUnlock()

	return data, nil
}

// Close implements channel.Channel.Close.
// Closes the TCP connection.
func (c *ClientChannel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return channel.ErrChannelClosed
	}

	c.closed = true
	c.connected = false

	if c.conn != nil {
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
// TCP has no inherent MTU limit, but we use 64KB as a reasonable default.
func (c *ClientChannel) DefaultMTU() int {
	return 64 * 1024 // 64KB
}

// IsReliable implements channel.Channel.IsReliable.
// TCP provides reliable transmission, no need for VoidBus-level ACK.
func (c *ClientChannel) IsReliable() bool {
	return true
}

// AckTimeout implements channel.Channel.AckTimeout.
// TCP is reliable, returns 0 (not used).
func (c *ClientChannel) AckTimeout() time.Duration {
	return 0
}

// handleDisconnect handles connection disconnect.
func (c *ClientChannel) handleDisconnect() {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
}

// ServerChannel implements channel.ServerChannel for TCP server.
type ServerChannel struct {
	listener    net.Listener
	config      channel.ChannelConfig
	mu          sync.RWMutex
	closed      bool
	clients     map[string]*AcceptedChannel
	clientCount int
}

// NewServerChannel creates a new TCP server channel.
//
// Parameter Constraints:
//   - config.Address MUST be valid TCP address format (host:port)
//
// Return Guarantees:
//   - On success: returns listening ServerChannel instance
func NewServerChannel(config channel.ChannelConfig) (*ServerChannel, error) {
	if config.Address == "" {
		return nil, errors.New("tcp: address required")
	}

	listener, err := net.Listen("tcp", config.Address)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "listen",
			Err:       err,
			Msg:       "failed to listen on " + config.Address,
			Retryable: false,
		}
	}

	return &ServerChannel{
		listener: listener,
		config:   config,
		clients:  make(map[string]*AcceptedChannel),
	}, nil
}

// Accept implements channel.ServerChannel.Accept.
// Waits for and accepts a new TCP connection.
//
// Return Guarantees:
//   - On success: returns connected client Channel
func (s *ServerChannel) Accept() (channel.Channel, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, channel.ErrChannelClosed
	}
	s.mu.RUnlock()

	conn, err := s.listener.Accept()
	if err != nil {
		s.mu.RLock()
		if s.closed {
			s.mu.RUnlock()
			return nil, channel.ErrChannelClosed
		}
		s.mu.RUnlock()
		return nil, &channel.ChannelError{
			Op:        "accept",
			Err:       err,
			Msg:       "failed to accept connection",
			Retryable: true,
		}
	}

	client := &AcceptedChannel{
		conn:         conn,
		server:       s,
		id:           internal.GenerateID(),
		lastActivity: time.Now(),
		connected:    true, // Initialize as connected
	}

	s.mu.Lock()
	s.clients[client.id] = client
	s.clientCount++
	s.mu.Unlock()

	return client, nil
}

// Send implements channel.Channel.Send.
func (s *ServerChannel) Send(data []byte) error {
	return errors.New("tcp: server channel cannot send directly, use accepted client channels")
}

// Receive implements channel.Channel.Receive.
func (s *ServerChannel) Receive() ([]byte, error) {
	return nil, errors.New("tcp: server channel cannot receive directly, use accepted client channels")
}

// Close implements channel.Channel.Close.
func (s *ServerChannel) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return channel.ErrChannelClosed
	}

	s.closed = true

	// Copy clients to close outside of lock to avoid deadlock
	// client.Close() may call removeClient which tries to acquire s.mu
	clients := make([]*AcceptedChannel, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	s.clients = make(map[string]*AcceptedChannel)
	s.mu.Unlock()

	// Close all client connections outside of lock
	for _, client := range clients {
		client.Close()
	}

	// Close listener
	s.mu.RLock()
	listener := s.listener
	s.mu.RUnlock()

	if listener != nil {
		return listener.Close()
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
// TCP server channel does not have MTU, returns 0.
func (s *ServerChannel) DefaultMTU() int {
	return 0 // Server channel doesn't have MTU
}

// IsReliable implements channel.Channel.IsReliable.
// TCP server is reliable.
func (s *ServerChannel) IsReliable() bool {
	return true
}

// AckTimeout implements channel.Channel.AckTimeout.
// TCP is reliable, returns 0.
func (s *ServerChannel) AckTimeout() time.Duration {
	return 0
}

// ListenAddress implements channel.ServerChannel.ListenAddress.
func (s *ServerChannel) ListenAddress() string {
	return s.config.Address
}

// ClientCount implements channel.ServerChannel.ClientCount.
func (s *ServerChannel) ClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientCount
}

// removeClient removes a client from the server's client map.
func (s *ServerChannel) removeClient(id string) {
	s.mu.Lock()
	delete(s.clients, id)
	s.clientCount--
	s.mu.Unlock()
}

// AcceptedChannel represents an accepted client connection on server.
// AcceptedChannel represents an accepted TCP connection.
// Note: net.Conn.Write is NOT thread-safe for concurrent writes, so we use writeMu.
type AcceptedChannel struct {
	conn         net.Conn
	server       *ServerChannel
	mu           sync.RWMutex // State management
	writeMu      sync.Mutex   // Write serialization
	closed       bool
	connected    bool
	id           string
	lastActivity time.Time
}

// Send implements channel.Channel.Send.
// Uses writeMu for concurrent-safe writes (net.Conn.Write is NOT thread-safe).
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

	// Step 4: Perform write (length prefix + data atomically)
	if len(data) > MaxFrameSize {
		a.writeMu.Unlock()
		return errors.New("tcp: data exceeds max frame size")
	}

	// Write length prefix
	lengthBuf := make([]byte, LengthPrefixSize)
	binary.LittleEndian.PutUint32(lengthBuf, uint32(len(data)))

	writeErr := func() error {
		if _, err := conn.Write(lengthBuf); err != nil {
			return err
		}
		if len(data) > 0 {
			if _, err := conn.Write(data); err != nil {
				return err
			}
		}
		return nil
	}()

	a.writeMu.Unlock()

	// Step 5: Handle result
	if writeErr != nil {
		a.handleDisconnect()
		return &channel.ChannelError{
			Op:        "send",
			Err:       writeErr,
			Msg:       "failed to write data",
			Retryable: false,
		}
	}

	a.mu.Lock()
	a.lastActivity = time.Now()
	a.mu.Unlock()
	return nil
}

// Receive implements channel.Channel.Receive.
// Important: This method does NOT hold the read lock during blocking I/O
// to avoid deadlock with Close(). Close() needs the write lock to proceed.
func (a *AcceptedChannel) Receive() ([]byte, error) {
	// Check connection status under read lock
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

	// Release read lock before blocking I/O to allow Close() to proceed

	// Read length prefix (blocking)
	lengthBuf := make([]byte, LengthPrefixSize)
	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		a.handleDisconnect()
		if errors.Is(err, io.EOF) {
			return nil, channel.ErrChannelDisconnected
		}
		if errors.Is(err, net.ErrClosed) {
			return nil, channel.ErrChannelClosed
		}
		return nil, &channel.ChannelError{
			Op:        "receive",
			Err:       err,
			Msg:       "failed to read length prefix",
			Retryable: false,
		}
	}

	length := binary.LittleEndian.Uint32(lengthBuf)
	if length > MaxFrameSize {
		return nil, errors.New("tcp: received frame exceeds max size")
	}

	// Read data (blocking)
	data := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(conn, data); err != nil {
			a.handleDisconnect()
			return nil, &channel.ChannelError{
				Op:        "receive",
				Err:       err,
				Msg:       "failed to read data",
				Retryable: false,
			}
		}
	}

	// Update last activity under read lock
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
// TCP has no inherent MTU limit, but we use 64KB as a reasonable default.
func (a *AcceptedChannel) DefaultMTU() int {
	return 64 * 1024 // 64KB
}

// IsReliable implements channel.Channel.IsReliable.
// TCP accepted connection is reliable.
func (a *AcceptedChannel) IsReliable() bool {
	return true
}

// AckTimeout implements channel.Channel.AckTimeout.
// TCP is reliable, returns 0.
func (a *AcceptedChannel) AckTimeout() time.Duration {
	return 0
}

// handleDisconnect handles connection disconnect.
func (a *AcceptedChannel) handleDisconnect() {
	a.mu.Lock()
	a.connected = false
	a.mu.Unlock()
}

// Module implements channel.ChannelModule for registration.
type Module struct{}

// NewModule creates a new TCP channel module.
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
