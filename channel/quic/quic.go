// Package quic provides a QUIC channel implementation.
//
// QUIC channel uses QUIC protocol (HTTP/3 transport) for communication.
// It provides:
// - Reliable, ordered delivery (built-in)
// - Built-in TLS 1.3 encryption (mandatory)
// - Multiplexed streams (no head-of-line blocking)
// - 0-RTT connection support
// - Fast handshake (1-RTT or 0-RTT)
// - Firewall friendly (UDP-based)
//
// Design Constraints:
// - QUIC channel MUST NOT handle serialization/encoding/fragmentation
// - QUIC channel MUST NOT be exposed in metadata protocols
// - QUIC Stream provides message framing (no length-prefix needed)
// - QUIC is reliable, VoidBus-level ACK is not needed
package quic

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/internal"
)

const (
	// ChannelType is the type identifier for QUIC channels
	ChannelType = channel.TypeQUIC
	// DefaultBufferSize is the default buffer size
	DefaultBufferSize = 4096
	// MaxMessageSize is the maximum message size (16MB)
	MaxMessageSize = 16 * 1024 * 1024
	// DefaultMTU is QUIC's initial MTU (packet size)
	DefaultMTU = 1200
	// DefaultConnectTimeout is the default connection timeout
	DefaultConnectTimeout = 30 * time.Second
	// DefaultIdleTimeout is the default idle timeout for connections
	DefaultIdleTimeout = 60 * time.Second
	// DefaultKeepAliveInterval is the keep-alive interval
	DefaultKeepAliveInterval = 15 * time.Second
)

// ClientChannel implements channel.Channel for QUIC client connections.
type ClientChannel struct {
	conn         *quic.Conn
	stream       *quic.Stream
	config       channel.ChannelConfig
	mu           sync.RWMutex
	closed       bool
	connected    bool
	id           string
	lastActivity time.Time
}

// NewClientChannel creates a new QUIC client channel.
//
// Parameter Constraints:
//   - config.Address MUST be valid QUIC address format (host:port)
//   - config.TLSConfig SHOULD be provided for QUIC (TLS is mandatory)
//   - If config.TLSConfig is nil, uses InsecureSkipVerify (for testing only)
//
// Return Guarantees:
//   - On success: returns connected Channel instance
//   - On failure: returns nil and error
func NewClientChannel(config channel.ChannelConfig) (*ClientChannel, error) {
	if config.Address == "" {
		return nil, errors.New("quic: address required")
	}

	// TLS configuration (QUIC requires TLS)
	tlsConfig := config.TLSConfig
	if tlsConfig == nil {
		// Default TLS config for testing (skip verification)
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"voidbus"},
		}
	}

	// Ensure NextProtos is set
	if len(tlsConfig.NextProtos) == 0 {
		tlsConfig.NextProtos = []string{"voidbus"}
	}

	// QUIC configuration
	quicConfig := &quic.Config{
		MaxIdleTimeout:    DefaultIdleTimeout,
		KeepAlivePeriod:   DefaultKeepAliveInterval,
		InitialPacketSize: DefaultMTU,
	}

	// Connection timeout
	timeout := config.ConnectTimeout
	if timeout <= 0 {
		timeout = DefaultConnectTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Establish QUIC connection
	conn, err := quic.DialAddr(ctx, config.Address, tlsConfig, quicConfig)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "connect",
			Err:       err,
			Msg:       "failed to connect to " + config.Address,
			Retryable: true,
		}
	}

	// Open a bidirectional stream
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		conn.CloseWithError(0, "failed to open stream")
		return nil, &channel.ChannelError{
			Op:        "stream",
			Err:       err,
			Msg:       "failed to open stream",
			Retryable: true,
		}
	}

	ch := &ClientChannel{
		conn:         conn,
		stream:       stream,
		config:       config,
		connected:    true,
		id:           internal.GenerateID(),
		lastActivity: time.Now(),
	}

	return ch, nil
}

// Send implements channel.Channel.Send.
// Sends data directly to QUIC stream (no framing needed).
//
// Parameter Constraints:
//   - data: MUST be non-nil byte slice
//   - data length MUST be <= MaxMessageSize (16MB)
//
// Return Guarantees:
//   - On success: complete data sent to peer
func (c *ClientChannel) Send(data []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return channel.ErrChannelClosed
	}
	if !c.connected {
		return channel.ErrChannelDisconnected
	}

	if len(data) > MaxMessageSize {
		return errors.New("quic: data exceeds max message size")
	}

	// Write data directly (QUIC stream handles framing)
	_, err := c.stream.Write(data)
	if err != nil {
		c.handleDisconnect()
		return &channel.ChannelError{
			Op:        "send",
			Err:       err,
			Msg:       "failed to write data",
			Retryable: false,
		}
	}

	c.lastActivity = time.Now()
	return nil
}

// Receive implements channel.Channel.Receive.
// Receives data from QUIC stream.
//
// Return Guarantees:
//   - On success: returns complete data
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
	stream := c.stream
	c.mu.RUnlock()

	// Read data (QUIC stream read until EOF or error)
	buffer := make([]byte, DefaultBufferSize)
	data := make([]byte, 0, DefaultBufferSize)

	for {
		n, err := stream.Read(buffer)
		if n > 0 {
			data = append(data, buffer[:n]...)
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				if len(data) > 0 {
					// Return partial data
					c.mu.RLock()
					c.lastActivity = time.Now()
					c.mu.RUnlock()
					return data, nil
				}
				c.handleDisconnect()
				return nil, channel.ErrChannelDisconnected
			}
			// Stream closed or error
			c.handleDisconnect()
			return nil, &channel.ChannelError{
				Op:        "receive",
				Err:       err,
				Msg:       "failed to read data",
				Retryable: false,
			}
		}
		if len(data) > MaxMessageSize {
			return nil, errors.New("quic: message exceeds max size")
		}
		// Continue reading until stream EOF or error
	}
}

// Close implements channel.Channel.Close.
// Closes the QUIC stream and connection.
func (c *ClientChannel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return channel.ErrChannelClosed
	}

	c.closed = true
	c.connected = false

	if c.stream != nil {
		c.stream.Close()
	}
	if c.conn != nil {
		c.conn.CloseWithError(0, "normal closure")
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
// QUIC's initial MTU is 1200 bytes (initial packet size).
func (c *ClientChannel) DefaultMTU() int {
	return DefaultMTU
}

// IsReliable implements channel.Channel.IsReliable.
// QUIC provides reliable transmission, no need for VoidBus-level ACK.
func (c *ClientChannel) IsReliable() bool {
	return true
}

// AckTimeout implements channel.Channel.AckTimeout.
// QUIC is reliable, returns 0 (not used).
func (c *ClientChannel) AckTimeout() time.Duration {
	return 0
}

// handleDisconnect handles connection disconnect.
func (c *ClientChannel) handleDisconnect() {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
}

// ServerChannel implements channel.ServerChannel for QUIC server.
type ServerChannel struct {
	listener    *quic.Listener
	config      channel.ChannelConfig
	tlsConfig   *tls.Config
	mu          sync.RWMutex
	closed      bool
	clients     map[string]*AcceptedChannel
	clientCount int
}

// NewServerChannel creates a new QUIC server channel.
//
// Parameter Constraints:
//   - config.Address MUST be valid QUIC address format (host:port)
//   - config.TLSConfig MUST be provided for QUIC server (TLS is mandatory)
//   - If config.TLSConfig is nil, generates self-signed cert (for testing only)
//
// Return Guarantees:
//   - On success: returns listening ServerChannel instance
func NewServerChannel(config channel.ChannelConfig) (*ServerChannel, error) {
	if config.Address == "" {
		return nil, errors.New("quic: address required")
	}

	tlsConfig := config.TLSConfig
	if tlsConfig == nil {
		// Generate self-signed certificate for testing
		cert, err := generateSelfSignedCert()
		if err != nil {
			return nil, &channel.ChannelError{
				Op:        "tls",
				Err:       err,
				Msg:       "failed to generate self-signed certificate",
				Retryable: false,
			}
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"voidbus"},
		}
	}

	// Ensure NextProtos is set
	if len(tlsConfig.NextProtos) == 0 {
		tlsConfig.NextProtos = []string{"voidbus"}
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:    DefaultIdleTimeout,
		InitialPacketSize: DefaultMTU,
	}

	listener, err := quic.ListenAddr(config.Address, tlsConfig, quicConfig)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "listen",
			Err:       err,
			Msg:       "failed to listen on " + config.Address,
			Retryable: false,
		}
	}

	return &ServerChannel{
		listener:  listener,
		config:    config,
		tlsConfig: tlsConfig,
		clients:   make(map[string]*AcceptedChannel),
	}, nil
}

// Accept implements channel.ServerChannel.Accept.
// Accepts a new QUIC connection and opens a stream.
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

	// Accept QUIC connection
	conn, err := s.listener.Accept(context.Background())
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

	// Accept stream from connection
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		conn.CloseWithError(0, "failed to accept stream")
		return nil, &channel.ChannelError{
			Op:        "stream",
			Err:       err,
			Msg:       "failed to accept stream",
			Retryable: true,
		}
	}

	client := &AcceptedChannel{
		conn:         conn,
		stream:       stream,
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

// Send implements channel.Channel.Send.
func (s *ServerChannel) Send(data []byte) error {
	return errors.New("quic: server channel cannot send directly, use accepted client channels")
}

// Receive implements channel.Channel.Receive.
func (s *ServerChannel) Receive() ([]byte, error) {
	return nil, errors.New("quic: server channel cannot receive directly, use accepted client channels")
}

// Close implements channel.Channel.Close.
func (s *ServerChannel) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return channel.ErrChannelClosed
	}

	s.closed = true

	// Close all clients
	for _, client := range s.clients {
		client.Close()
	}
	s.clients = make(map[string]*AcceptedChannel)

	if s.listener != nil {
		return s.listener.Close()
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

// removeClient removes a client from server.
func (s *ServerChannel) removeClient(id string) {
	s.mu.Lock()
	delete(s.clients, id)
	s.clientCount--
	s.mu.Unlock()
}

// AcceptedChannel represents an accepted QUIC connection on server.
type AcceptedChannel struct {
	conn         *quic.Conn
	stream       *quic.Stream
	server       *ServerChannel
	mu           sync.RWMutex
	closed       bool
	connected    bool
	id           string
	lastActivity time.Time
}

// Send implements channel.Channel.Send.
func (a *AcceptedChannel) Send(data []byte) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return channel.ErrChannelClosed
	}
	if !a.connected {
		return channel.ErrChannelDisconnected
	}

	if len(data) > MaxMessageSize {
		return errors.New("quic: data exceeds max message size")
	}

	_, err := a.stream.Write(data)
	if err != nil {
		a.handleDisconnect()
		return &channel.ChannelError{
			Op:        "send",
			Err:       err,
			Msg:       "failed to write data",
			Retryable: false,
		}
	}

	a.lastActivity = time.Now()
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
	stream := a.stream
	a.mu.RUnlock()

	buffer := make([]byte, DefaultBufferSize)
	data := make([]byte, 0, DefaultBufferSize)

	for {
		n, err := stream.Read(buffer)
		if n > 0 {
			data = append(data, buffer[:n]...)
		}
		if err != nil {
			a.handleDisconnect()
			return nil, &channel.ChannelError{
				Op:        "receive",
				Err:       err,
				Msg:       "failed to read data",
				Retryable: false,
			}
		}
		if len(data) > MaxMessageSize {
			return nil, errors.New("quic: message exceeds max size")
		}
	}
}

// Close implements channel.Channel.Close.
func (a *AcceptedChannel) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return channel.ErrChannelClosed
	}

	a.closed = true
	a.connected = false

	if a.server != nil {
		a.server.removeClient(a.id)
	}

	if a.stream != nil {
		a.stream.Close()
	}
	if a.conn != nil {
		a.conn.CloseWithError(0, "normal closure")
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
	return DefaultMTU
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

// Module implements channel.ChannelModule for registration.
type Module struct{}

// NewModule creates a new QUIC channel module.
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

// generateSelfSignedCert generates a self-signed TLS certificate for testing.
//
// This function creates a temporary self-signed certificate for testing purposes only.
// Production deployments MUST provide proper TLS certificates via config.TLSConfig.
//
// Certificate Properties:
//   - RSA 2048-bit key
//   - 24-hour validity period
//   - DNS name: localhost
//   - Usage: server authentication
func generateSelfSignedCert() (tls.Certificate, error) {
	// Generate RSA private key (2048-bit)
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Create certificate serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Certificate template
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"VoidBus Test Certificate"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour), // 24-hour validity for testing
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}

	// Create self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Build tls.Certificate
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
		Leaf:        template,
	}, nil
}
