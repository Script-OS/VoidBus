// Package core provides core interface definitions for VoidBus.
//
// This file defines the core interface contracts for:
// - Bus: Single-channel bus for point-to-point communication
// - ServerBus: Multi-client bus for server-side Hall mode
// - MultiBus: Multi-channel bus for parallel transmission
//
// Design Constraints (see docs/ARCHITECTURE.md):
// - All interfaces follow the Builder pattern for configuration
// - Serializer.Name() CAN be exposed for negotiation
// - CodecChain configuration MUST NOT be exposed
// - Channel configuration MUST NOT be exposed
// - SecurityLevel is used for handshake negotiation
package core

import (
	"errors"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/fragment"
	"github.com/Script-OS/VoidBus/keyprovider"
	"github.com/Script-OS/VoidBus/protocol"
	"github.com/Script-OS/VoidBus/serializer"
)

// BusInterface defines the contract for a single-channel bus.
// Bus coordinates Serializer, CodecChain, Channel, Fragment, and KeyProvider
// for point-to-point communication.
type BusInterface interface {
	// Lifecycle methods
	Start() error
	Stop() error
	IsRunning() bool

	// Communication methods
	Send(data []byte) error
	Receive() ([]byte, error)

	// Accessor methods
	GetSessionID() string
	GetSerializer() serializer.Serializer
	GetCodecChain() codec.CodecChain
	GetChannel() channel.Channel
	GetFragment() fragment.Fragment
	GetKeyProvider() keyprovider.KeyProvider
	GetConfig() BusConfig
	SetConfig(config BusConfig)

	// Security level
	SecurityLevel() codec.SecurityLevel
}

// BusConfigurer defines configuration methods for Bus.
// These methods return concrete type for method chaining.
type BusConfigurer interface {
	SetSerializer(s serializer.Serializer) interface{}
	SetCodecChain(c codec.CodecChain) interface{}
	SetChannel(c channel.Channel) interface{}
	SetFragment(f fragment.Fragment) interface{}
	SetKeyProvider(kp keyprovider.KeyProvider) interface{}
	OnMessage(handler func([]byte)) interface{}
	OnError(handler func(error)) interface{}
}

// ServerBusInterface defines the contract for server-side Hall mode.
// ServerBus listens for incoming connections, performs handshake negotiation,
// and creates individual Bus instances for each accepted client.
type ServerBusInterface interface {
	// Lifecycle methods
	Start() error
	Stop() error
	IsRunning() bool

	// Communication methods
	SendTo(sessionID string, data []byte) error
	Broadcast(data []byte) error

	// Client management methods
	GetClient(sessionID string) (*ClientInfo, error)
	ListClients() []*ClientInfo
	ClientCount() int

	// Accessor methods
	GetConfig() ServerBusConfig
}

// ServerBusConfigurer defines configuration methods for ServerBus.
// These methods return concrete type for method chaining.
type ServerBusConfigurer interface {
	SetServerChannel(ch channel.ServerChannel) interface{}
	SetSerializer(ser serializer.Serializer) interface{}
	SetCodecChain(chain codec.CodecChain) interface{}
	SetKeyProvider(kp keyprovider.KeyProvider) interface{}
	SetBusConfig(config BusConfig) interface{}
}

// MultiBusInterface defines the contract for multi-channel bus.
// MultiBus manages multiple Channel instances and distributes data
// across channels using various selection strategies.
type MultiBusInterface interface {
	// Lifecycle methods
	Start() error
	Stop() error
	IsRunning() bool

	// Communication methods
	Send(data []byte) error

	// Channel management methods
	ChannelCount() int
	ActiveChannelCount() int
	GetChannelInfo(index int) (*ChannelInfo, error)

	// Accessor methods
	GetSessionID() string
	GetConfig() MultiBusConfig
	SecurityLevel() codec.SecurityLevel
}

// MultiBusConfigurer defines configuration methods for MultiBus.
// These methods return concrete type for method chaining.
type MultiBusConfigurer interface {
	SetSerializer(s serializer.Serializer) interface{}
	SetCodecChain(c codec.CodecChain) interface{}
	SetFragment(f fragment.Fragment) interface{}
	SetKeyProvider(kp keyprovider.KeyProvider) interface{}
	AddChannel(ch channel.Channel, weight int) interface{}
}

// ClientInfo contains information about a connected client (ServerBus).
type ClientInfo struct {
	// SessionID is the unique session identifier
	SessionID string

	// ClientID is the client's identifier (from handshake)
	ClientID string

	// ConnectedAt is connection timestamp
	ConnectedAt int64 // Unix timestamp

	// LastActivity is last activity timestamp
	LastActivity int64 // Unix timestamp

	// Serializer is the negotiated serializer name (CAN be exposed)
	Serializer string

	// SecurityLevel is the negotiated security level
	SecurityLevel codec.SecurityLevel

	// RemoteAddress is the client's remote address
	RemoteAddress string

	// Bus is the client's Bus instance
	Bus *Bus

	// Status is the client's connection status
	Status ClientStatus
}

// ClientStatus represents client connection status.
type ClientStatus int

const (
	// ClientStatusConnecting indicates client is connecting
	ClientStatusConnecting ClientStatus = iota
	// ClientStatusHandshaking indicates client is handshaking
	ClientStatusHandshaking
	// ClientStatusActive indicates client is active
	ClientStatusActive
	// ClientStatusDisconnected indicates client is disconnected
	ClientStatusDisconnected
)

// String returns string representation of ClientStatus.
func (s ClientStatus) String() string {
	switch s {
	case ClientStatusConnecting:
		return "connecting"
	case ClientStatusHandshaking:
		return "handshaking"
	case ClientStatusActive:
		return "active"
	case ClientStatusDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

// ChannelInfo contains information about a channel in MultiBus pool.
type ChannelInfo struct {
	// Index is the channel index in the pool
	Index int

	// Channel is the channel instance
	Channel channel.Channel

	// Weight is the weight for weighted selection (0-100)
	Weight int

	// Status is the channel status
	Status ChannelStatus

	// LastActivity is last activity timestamp
	LastActivity int64 // Unix timestamp

	// SendCount is number of packets sent through this channel
	SendCount int64

	// ReceiveCount is number of packets received through this channel
	ReceiveCount int64

	// ErrorCount is number of errors encountered
	ErrorCount int64
}

// ChannelStatus represents channel status in MultiBus.
type ChannelStatus int

const (
	// ChannelStatusActive indicates channel is active
	ChannelStatusActive ChannelStatus = iota
	// ChannelStatusInactive indicates channel is inactive
	ChannelStatusInactive
	// ChannelStatusError indicates channel has error
	ChannelStatusError
)

// String returns string representation of ChannelStatus.
func (s ChannelStatus) String() string {
	switch s {
	case ChannelStatusActive:
		return "active"
	case ChannelStatusInactive:
		return "inactive"
	case ChannelStatusError:
		return "error"
	default:
		return "unknown"
	}
}

// ChannelSelectionStrategy defines how channels are selected for sending.
type ChannelSelectionStrategy int

const (
	// StrategyRandom randomly selects channels for each packet
	StrategyRandom ChannelSelectionStrategy = iota
	// StrategyRoundRobin selects channels in round-robin order
	StrategyRoundRobin
	// StrategyWeighted selects channels based on weights
	StrategyWeighted
	// StrategySpecified uses a specified channel index
	StrategySpecified
)

// String returns string representation of ChannelSelectionStrategy.
func (s ChannelSelectionStrategy) String() string {
	switch s {
	case StrategyRandom:
		return "random"
	case StrategyRoundRobin:
		return "round_robin"
	case StrategyWeighted:
		return "weighted"
	case StrategySpecified:
		return "specified"
	default:
		return "unknown"
	}
}

// ToDistributionStrategy converts to protocol.DistributionStrategy.
func (s ChannelSelectionStrategy) ToDistributionStrategy() protocol.DistributionStrategy {
	switch s {
	case StrategyRandom:
		return protocol.DistributeAllRandom
	case StrategyRoundRobin:
		return protocol.DistributeRoundRobin
	case StrategyWeighted:
		return protocol.DistributeWeighted
	case StrategySpecified:
		return protocol.DistributeGrouped
	default:
		return protocol.DistributeAllRandom
	}
}

// ServerBusConfig provides configuration for ServerBus.
type ServerBusConfig struct {
	// MaxClients is maximum number of concurrent clients (0 = unlimited)
	MaxClients int

	// HandshakeTimeout is handshake timeout in seconds
	HandshakeTimeout int

	// ClientTimeout is client idle timeout in seconds (0 = no timeout)
	ClientTimeout int

	// AutoCleanup enables automatic cleanup of disconnected clients
	AutoCleanup bool

	// CleanupInterval is cleanup interval in seconds
	CleanupInterval int
}

// DefaultServerBusConfig returns default server bus configuration.
func DefaultServerBusConfig() ServerBusConfig {
	return ServerBusConfig{
		MaxClients:       100,
		HandshakeTimeout: 30,
		ClientTimeout:    300,
		AutoCleanup:      true,
		CleanupInterval:  60,
	}
}

// Validate validates the ServerBusConfig.
// Returns error if configuration contains invalid values.
func (c ServerBusConfig) Validate() error {
	if c.MaxClients < 0 {
		return errors.New("serverbus: MaxClients must be non-negative")
	}
	if c.HandshakeTimeout < 1 {
		return errors.New("serverbus: HandshakeTimeout must be at least 1 second")
	}
	if c.ClientTimeout < 0 {
		return errors.New("serverbus: ClientTimeout must be non-negative")
	}
	if c.CleanupInterval < 0 {
		return errors.New("serverbus: CleanupInterval must be non-negative")
	}
	return nil
}

// MultiBusConfig provides configuration for MultiBus.
type MultiBusConfig struct {
	// SelectionStrategy is the channel selection strategy
	SelectionStrategy ChannelSelectionStrategy

	// SpecifiedChannelIndex is the specified channel index (for StrategySpecified)
	SpecifiedChannelIndex int

	// MaxFragmentSize is maximum fragment size in bytes
	MaxFragmentSize int

	// EnableFragmentation enables data fragmentation (default true)
	EnableFragmentation bool

	// FragmentTimeout is reassembly timeout in seconds
	FragmentTimeout int

	// SendQueueSize is send queue buffer size per channel
	SendQueueSize int

	// ReceiveWorkers is number of receive workers per channel
	ReceiveWorkers int

	// AutoReconnect enables automatic channel reconnection
	AutoReconnect bool

	// ReconnectDelay is delay between reconnection attempts in seconds
	ReconnectDelay int

	// MaxReconnectAttempts is maximum reconnection attempts (0 = unlimited)
	MaxReconnectAttempts int

	// EnableChecksum enables fragment checksum verification
	EnableChecksum bool
}

// DefaultMultiBusConfig returns default multi bus configuration.
func DefaultMultiBusConfig() MultiBusConfig {
	return MultiBusConfig{
		SelectionStrategy:     StrategyRandom,
		SpecifiedChannelIndex: 0,
		MaxFragmentSize:       1024,
		EnableFragmentation:   true,
		FragmentTimeout:       60,
		SendQueueSize:         100,
		ReceiveWorkers:        1,
		AutoReconnect:         true,
		ReconnectDelay:        3,
		MaxReconnectAttempts:  0,
		EnableChecksum:        true,
	}
}

// Validate validates the MultiBusConfig.
// Returns error if configuration contains invalid values.
func (c MultiBusConfig) Validate() error {
	if c.MaxFragmentSize < 64 || c.MaxFragmentSize > 65536 {
		return errors.New("multibus: MaxFragmentSize must be between 64 and 65536")
	}
	if c.SendQueueSize < 0 {
		return errors.New("multibus: SendQueueSize must be positive")
	}
	if c.ReceiveWorkers < 1 {
		return errors.New("multibus: ReceiveWorkers must be at least 1")
	}
	if c.FragmentTimeout < 0 {
		return errors.New("multibus: FragmentTimeout must be positive")
	}
	if c.ReconnectDelay < 0 {
		return errors.New("multibus: ReconnectDelay must be positive")
	}
	if c.SpecifiedChannelIndex < 0 {
		return errors.New("multibus: SpecifiedChannelIndex must be non-negative")
	}
	return nil
}
