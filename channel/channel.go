// Package channel defines the Channel interface for transport layer communication.
//
// Channel is responsible for establishing and maintaining network connections
// and sending/receiving raw byte data.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.4):
// - Channel MUST NOT handle data serialization
// - Channel MUST NOT handle data encoding/encryption
// - Channel MUST NOT handle data fragmentation
// - Channel MUST NOT handle key management
// - Channel.Type() MUST NOT be transmitted over network
package channel

import (
	"errors"
	"io"
	"time"
)

// ChannelType identifies the type of channel.
// This is for internal use only and MUST NOT be transmitted.
type ChannelType string

const (
	// TypeTCP represents TCP channel
	TypeTCP ChannelType = "tcp"
	// TypeUDP represents UDP channel
	TypeUDP ChannelType = "udp"
	// TypeICMP represents ICMP channel
	TypeICMP ChannelType = "icmp"
	// TypeQUIC represents QUIC channel
	TypeQUIC ChannelType = "quic"
)

// Common channel errors
var (
	// ErrChannelClosed indicates the channel is closed
	ErrChannelClosed = errors.New("channel: closed")
	// ErrChannelNotReady indicates the channel is not ready
	ErrChannelNotReady = errors.New("channel: not ready")
	// ErrChannelTimeout indicates operation timeout
	ErrChannelTimeout = errors.New("channel: timeout")
	// ErrChannelDisconnected indicates connection lost
	ErrChannelDisconnected = errors.New("channel: disconnected")
	// ErrChannelSendFailed indicates send operation failed
	ErrChannelSendFailed = errors.New("channel: send failed")
	// ErrChannelRecvFailed indicates receive operation failed
	ErrChannelRecvFailed = errors.New("channel: receive failed")
	// ErrAcceptFailed indicates accept operation failed
	ErrAcceptFailed = errors.New("channel: accept failed")
)

// Channel is the core interface for transport layer communication.
// It is responsible for sending and receiving raw byte data.
// Channel MUST NOT be exposed in metadata protocols.
//
// Responsibilities:
// - Establish and maintain network connections
// - Send raw byte data (Send)
// - Receive raw byte data (Receive)
// - Manage connection lifecycle (Close, IsConnected)
// - Provide channel type for internal management (Type)
//
// NOT Responsible for:
// - Data serialization (handled by Serializer)
// - Data encoding/encryption (handled by Codec)
// - Data fragmentation (handled by Fragment)
// - Key management (handled by KeyProvider)
type Channel interface {
	// Send sends raw byte data over the channel.
	//
	// Parameter Constraints:
	//   - data: MUST be non-nil byte slice
	//   - Data is transmitted as-is without modification
	//
	// Return Guarantees:
	//   - On success: data has been sent to peer
	//   - On failure: connection may be disconnected
	//
	// Error Types:
	//   - ErrChannelClosed: channel is closed
	//   - ErrChannelDisconnected: connection lost
	//   - ErrChannelSendFailed: send failed
	//   - ErrChannelTimeout: send timeout
	Send(data []byte) error

	// Receive receives raw byte data from the channel.
	//
	// Behavior:
	//   - Blocking operation, waits for data
	//   - Returns complete data unit
	//
	// Return Guarantees:
	//   - On success: returns complete received data
	//   - On failure: connection may be disconnected
	//
	// Error Types:
	//   - ErrChannelClosed: channel is closed
	//   - ErrChannelDisconnected: connection lost
	//   - ErrChannelRecvFailed: receive failed
	//   - ErrChannelTimeout: receive timeout
	Receive() ([]byte, error)

	// Close closes the channel and releases all resources.
	//
	// Behavior:
	//   - After Close, channel cannot be used
	//   - Releases all associated resources
	//
	// Return Guarantees:
	//   - On success: resources released
	//   - Multiple calls return ErrChannelClosed
	Close() error

	// IsConnected returns the connection status.
	//
	// Return Guarantees:
	//   - true: channel is available
	//   - false: channel is unavailable (not connected or disconnected)
	IsConnected() bool

	// Type returns the channel type.
	//
	// Return Guarantees:
	//   - Returns ChannelType constant
	//   - For internal management only, MUST NOT be transmitted
	Type() ChannelType
}

// ServerChannel extends Channel with server-side capabilities.
// It can accept incoming connections and create client channels.
type ServerChannel interface {
	Channel

	// Accept waits for and returns the next connection.
	//
	// Behavior:
	//   - Blocking operation, waits for new connection
	//   - Each call returns one new client Channel
	//
	// Return Guarantees:
	//   - On success: returns client Channel instance
	//   - Client Channel is already connected
	//
	// Error Types:
	//   - ErrChannelClosed: server is closed
	//   - ErrAcceptFailed: accept failed
	Accept() (Channel, error)

	// ListenAddress returns the listening address.
	//
	// Return Guarantees:
	//   - Format: "host:port"
	ListenAddress() string

	// ClientCount returns the number of accepted clients.
	ClientCount() int
}

// ChannelConfig provides configuration for channel instances.
type ChannelConfig struct {
	// Address is the network address (e.g., "localhost:8080")
	Address string

	// Timeout is the operation timeout in seconds (0 = no timeout)
	Timeout int

	// BufferSize is the buffer size for send/receive operations
	BufferSize int

	// KeepAlive configuration for connection maintenance
	KeepAlive KeepAliveConfig
}

// KeepAliveConfig configures heartbeat keep-alive mechanism.
type KeepAliveConfig struct {
	// Enable indicates whether heartbeat is enabled
	Enable bool

	// Interval is the heartbeat interval in seconds
	Interval int

	// Timeout is the heartbeat response timeout in seconds
	Timeout int

	// MaxMissed is the maximum missed heartbeats before disconnect
	MaxMissed int
}

// DefaultKeepAliveConfig returns default keep-alive configuration.
func DefaultKeepAliveConfig() KeepAliveConfig {
	return KeepAliveConfig{
		Enable:    true,
		Interval:  5,
		Timeout:   30,
		MaxMissed: 3,
	}
}

// ChannelMetadata contains metadata about a channel instance.
type ChannelMetadata struct {
	Type         ChannelType
	Address      string
	IsServer     bool
	ID           string
	CreatedAt    time.Time
	LastActivity time.Time
}

// ChannelError represents errors specific to channel operations.
type ChannelError struct {
	Op        string // The operation that caused the error
	Err       error  // The underlying error
	Msg       string // Additional context message
	Retryable bool   // Whether the operation can be retried
}

func (e *ChannelError) Error() string {
	return e.Op + ": " + e.Msg + ": " + e.Err.Error()
}

func (e *ChannelError) Unwrap() error {
	return e.Err
}

// IsRetryable checks if the error is retryable.
func (e *ChannelError) IsRetryable() bool {
	return e.Retryable
}

// ChannelReader wraps a Channel to implement io.Reader.
type ChannelReader struct {
	ch     Channel
	buffer []byte
}

// NewChannelReader creates a new ChannelReader.
func NewChannelReader(ch Channel) *ChannelReader {
	return &ChannelReader{ch: ch}
}

func (r *ChannelReader) Read(p []byte) (n int, err error) {
	if len(r.buffer) == 0 {
		data, err := r.ch.Receive()
		if err != nil {
			return 0, err
		}
		r.buffer = data
	}

	n = copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

// ChannelWriter wraps a Channel to implement io.Writer.
type ChannelWriter struct {
	ch Channel
}

// NewChannelWriter creates a new ChannelWriter.
func NewChannelWriter(ch Channel) *ChannelWriter {
	return &ChannelWriter{ch: ch}
}

func (w *ChannelWriter) Write(p []byte) (n int, err error) {
	err = w.ch.Send(p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// ChannelModule is the interface for channel module registration.
type ChannelModule interface {
	// CreateClient creates a client channel instance.
	CreateClient(config ChannelConfig) (Channel, error)

	// CreateServer creates a server channel instance.
	CreateServer(config ChannelConfig) (ServerChannel, error)

	// Type returns the channel type.
	Type() ChannelType
}

// ChannelRegistry manages registered channels.
type ChannelRegistry struct {
	modules map[ChannelType]ChannelModule
}

// NewChannelRegistry creates a new ChannelRegistry.
func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{
		modules: make(map[ChannelType]ChannelModule),
	}
}

// Register registers a channel module.
func (r *ChannelRegistry) Register(module ChannelModule) error {
	if module == nil {
		return errors.New("channel: cannot register nil module")
	}
	r.modules[module.Type()] = module
	return nil
}

// GetClient retrieves a client channel instance.
func (r *ChannelRegistry) GetClient(typ ChannelType, config ChannelConfig) (Channel, error) {
	module, exists := r.modules[typ]
	if !exists {
		return nil, errors.New("channel: type not registered: " + string(typ))
	}
	return module.CreateClient(config)
}

// GetServer retrieves a server channel instance.
func (r *ChannelRegistry) GetServer(typ ChannelType, config ChannelConfig) (ServerChannel, error) {
	module, exists := r.modules[typ]
	if !exists {
		return nil, errors.New("channel: type not registered: " + string(typ))
	}
	return module.CreateServer(config)
}

// List returns all registered channel types.
func (r *ChannelRegistry) List() []ChannelType {
	result := make([]ChannelType, 0, len(r.modules))
	for typ := range r.modules {
		result = append(result, typ)
	}
	return result
}

// Global registry instance
var globalRegistry = NewChannelRegistry()

// Register registers a module to the global registry.
func Register(module ChannelModule) error {
	return globalRegistry.Register(module)
}

// GetClient retrieves a client channel from the global registry.
func GetClient(typ ChannelType, config ChannelConfig) (Channel, error) {
	return globalRegistry.GetClient(typ, config)
}

// GetServer retrieves a server channel from the global registry.
func GetServer(typ ChannelType, config ChannelConfig) (ServerChannel, error) {
	return globalRegistry.GetServer(typ, config)
}

// List returns all channel types from the global registry.
func List() []ChannelType {
	return globalRegistry.List()
}

// GlobalRegistry returns the global registry instance.
func GlobalRegistry() *ChannelRegistry {
	return globalRegistry
}

// Verify interface compliance
var (
	_ io.Reader = (*ChannelReader)(nil)
	_ io.Writer = (*ChannelWriter)(nil)
)
