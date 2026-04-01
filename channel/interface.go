// Package channel defines the Channel interface for transport layer communication.
//
// Channel is responsible for establishing and maintaining network connections
// and sending/receiving raw byte data.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.4):
// - Channel MUST NOT handle data serialization
// - Channel MUST NOT handle data encoding/encryption
// - Channel MUST NOT handle data fragmentation
// - Channel.Type() MUST NOT be transmitted over network
package channel

import (
	"errors"
	"time"
)

// ChannelType identifies the type of channel.
type ChannelType string

const (
	TypeTCP  ChannelType = "tcp"
	TypeUDP  ChannelType = "udp"
	TypeICMP ChannelType = "icmp"
	TypeQUIC ChannelType = "quic"
	TypeWS   ChannelType = "ws"   // WebSocket (default negotiation channel)
	TypeHTTP ChannelType = "http" // HTTP/HTTPS tunnel
	TypeDNS  ChannelType = "dns"  // DNS tunnel
)

// Common channel errors
var (
	ErrChannelClosed       = errors.New("channel: closed")
	ErrChannelNotReady     = errors.New("channel: not ready")
	ErrChannelTimeout      = errors.New("channel: timeout")
	ErrChannelDisconnected = errors.New("channel: disconnected")
	ErrChannelSendFailed   = errors.New("channel: send failed")
	ErrChannelRecvFailed   = errors.New("channel: receive failed")
	ErrAcceptFailed        = errors.New("channel: accept failed")
)

// ChannelError represents a channel error with context.
type ChannelError struct {
	Op        string
	Err       error
	Msg       string
	Retryable bool
}

// Error implements the error interface.
func (e *ChannelError) Error() string {
	return e.Op + ": " + e.Msg + ": " + e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *ChannelError) Unwrap() error {
	return e.Err
}

// Channel is the core interface for transport layer communication.
type Channel interface {
	// Send sends raw byte data over the channel.
	Send(data []byte) error

	// Receive receives raw byte data from the channel.
	Receive() ([]byte, error)

	// Close closes the channel and releases all resources.
	Close() error

	// IsConnected returns the connection status.
	IsConnected() bool

	// Type returns the channel type (NOT for transmission).
	Type() ChannelType

	// DefaultMTU returns the default Maximum Transmission Unit for this channel.
	// Used for adaptive fragmentation in v2.0 architecture.
	// Returns 0 if no specific MTU is known.
	DefaultMTU() int

	// IsReliable returns whether the channel provides reliable transmission.
	// Reliable channels (TCP, QUIC, WebSocket) rely on their own protocols for reliability.
	// Unreliable channels (UDP) require VoidBus-level ACK/NAK mechanism.
	IsReliable() bool

	// AckTimeout returns the ACK timeout duration for unreliable channels.
	// Only used for channels where IsReliable() returns false.
	// Returns 0 for reliable channels (timeout not needed).
	AckTimeout() time.Duration
}

// ServerChannel extends Channel with server-side capabilities.
type ServerChannel interface {
	Channel

	// Accept waits for and returns the next connection.
	Accept() (Channel, error)

	// ListenAddress returns the listening address.
	ListenAddress() string
}

// ChannelConfig provides configuration for channels.
type ChannelConfig struct {
	Address         string
	Timeout         time.Duration
	ConnectTimeout  time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	MaxMessageSize  int
	BufferSize      int
	KeepAlive       bool
	KeepAlivePeriod time.Duration
	ReuseAddr       bool
}

// ChannelModule is the interface for channel module registration.
type ChannelModule interface {
	// CreateClient creates a client channel.
	CreateClient(config ChannelConfig) (Channel, error)

	// CreateServer creates a server channel.
	CreateServer(config ChannelConfig) (ServerChannel, error)

	// Type returns the channel type.
	Type() ChannelType
}
