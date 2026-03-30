// Package core provides the core Bus implementation for VoidBus.
//
// Bus is the central component that coordinates all modules:
// - Serializer: Data serialization (can be exposed in metadata)
// - CodecChain: Encoding/encryption chain (NOT exposed)
// - Channel: Transport layer (NOT exposed)
// - Fragment: Data fragmentation (optional)
// - KeyProvider: Key management (optional)
//
// Data Flow (Send):
//
//	data -> Serializer.Serialize -> CodecChain.Encode -> [Fragment.Split] -> Channel.Send
//
// Data Flow (Receive):
//
//	Channel.Receive -> [Fragment.Reassemble] -> CodecChain.Decode -> Serializer.Deserialize -> data
package core

import (
	"errors"
	"sync"
	"time"

	"VoidBus/channel"
	"VoidBus/codec"
	"VoidBus/fragment"
	"VoidBus/internal"
	"VoidBus/keyprovider"
	"VoidBus/serializer"
)

// Bus errors are defined in VoidBus/errors.go

// BusConfig provides configuration for Bus.
type BusConfig struct {
	// AsyncReceive enables asynchronous receive mode
	AsyncReceive bool

	// EnableFragment enables data fragmentation
	EnableFragment bool

	// MaxFragmentSize is maximum fragment size in bytes
	MaxFragmentSize int

	// SendQueueSize is send queue buffer size
	SendQueueSize int

	// RecvQueueSize is receive queue buffer size
	RecvQueueSize int

	// FragmentTimeout is fragment reassembly timeout in seconds
	FragmentTimeout int

	// AutoReconnect enables automatic reconnection
	AutoReconnect bool

	// ReconnectDelay is delay between reconnection attempts in seconds
	ReconnectDelay int

	// MaxReconnectAttempts is maximum reconnection attempts (0 = unlimited)
	MaxReconnectAttempts int
}

// DefaultBusConfig returns default bus configuration.
func DefaultBusConfig() BusConfig {
	return BusConfig{
		AsyncReceive:         true,
		EnableFragment:       false,
		MaxFragmentSize:      1024,
		SendQueueSize:        100,
		RecvQueueSize:        100,
		FragmentTimeout:      60,
		AutoReconnect:        true,
		ReconnectDelay:       3,
		MaxReconnectAttempts: 0, // unlimited
	}
}

// Bus is the core structure that coordinates all modules.
type Bus struct {
	mu sync.RWMutex

	// Core modules
	serializer serializer.Serializer
	codecChain codec.CodecChain
	channel    channel.Channel

	// Optional modules
	fragment    fragment.Fragment
	keyProvider keyprovider.KeyProvider

	// Configuration
	config BusConfig

	// State
	running   bool
	stopCh    chan struct{}
	sessionID string

	// Handlers
	messageHandler func([]byte)
	errorHandler   func(error)

	// Queues (for async mode)
	sendQueue chan []byte

	// Fragment manager (for reassembly)
	fragmentManager *fragment.DefaultFragmentManager
}

// New creates a new Bus with default configuration.
func New() *Bus {
	return NewWithConfig(DefaultBusConfig())
}

// NewWithConfig creates a new Bus with specified configuration.
func NewWithConfig(config BusConfig) *Bus {
	return &Bus{
		config:          config,
		stopCh:          make(chan struct{}),
		sessionID:       internal.GenerateSessionID(),
		fragmentManager: fragment.NewFragmentManager(fragment.DefaultFragmentConfig()),
	}
}

// SetSerializer sets the serializer.
func (b *Bus) SetSerializer(s serializer.Serializer) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.serializer = s
	return b
}

// SetCodecChain sets the codec chain.
func (b *Bus) SetCodecChain(c codec.CodecChain) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.codecChain = c
	return b
}

// SetChannel sets the channel.
func (b *Bus) SetChannel(c channel.Channel) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.channel = c
	return b
}

// SetFragment sets the fragment handler.
func (b *Bus) SetFragment(f fragment.Fragment) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fragment = f
	b.config.EnableFragment = true
	return b
}

// SetKeyProvider sets the key provider.
func (b *Bus) SetKeyProvider(kp keyprovider.KeyProvider) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.keyProvider = kp

	// Set key provider for codec chain if it needs keys
	if b.codecChain != nil {
		b.codecChain.SetKeyProvider(kp)
	}
	return b
}

// OnMessage registers message handler.
func (b *Bus) OnMessage(handler func([]byte)) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messageHandler = handler
	return b
}

// OnError registers error handler.
func (b *Bus) OnError(handler func(error)) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.errorHandler = handler
	return b
}

// Start starts the bus.
func (b *Bus) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return errors.New("bus: already running")
	}

	// Validate required modules
	if b.serializer == nil {
		return errors.New("bus: serializer required")
	}
	if b.codecChain == nil {
		return errors.New("bus: codec chain required")
	}
	if b.channel == nil {
		return errors.New("bus: channel required")
	}

	// Set key provider for codec chain
	if b.keyProvider != nil {
		if err := b.codecChain.SetKeyProvider(b.keyProvider); err != nil {
			return err
		}
	}

	// Initialize queues
	b.sendQueue = make(chan []byte, b.config.SendQueueSize)

	// Reset stop channel
	b.stopCh = make(chan struct{})
	b.running = true

	// Start background routines
	go b.sendLoop()
	if b.config.AsyncReceive {
		go b.receiveLoop()
	}

	return nil
}

// Stop stops the bus.
func (b *Bus) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return errors.New("bus: not running")
	}

	b.running = false
	close(b.stopCh)

	if b.channel != nil {
		return b.channel.Close()
	}

	return nil
}

// Send sends data through the bus.
// Processing: data -> Serializer.Serialize -> CodecChain.Encode -> [Fragment.Split] -> Channel.Send
func (b *Bus) Send(data []byte) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.running {
		return errors.New("bus: not running")
	}

	// Queue for async send
	if b.sendQueue != nil {
		select {
		case b.sendQueue <- data:
			return nil
		default:
			// Queue full, send synchronously
		}
	}

	return b.sendSync(data)
}

// sendSync sends data synchronously.
func (b *Bus) sendSync(data []byte) error {
	var err error
	processed := data

	// 1. Serialize
	processed, err = b.serializer.Serialize(processed)
	if err != nil {
		return b.handleError("serialize", err)
	}

	// 2. Encode
	processed, err = b.codecChain.Encode(processed)
	if err != nil {
		return b.handleError("encode", err)
	}

	// 3. Fragment (if enabled)
	if b.config.EnableFragment && b.fragment != nil {
		fragments, err := b.fragment.Split(processed, b.config.MaxFragmentSize)
		if err != nil {
			return b.handleError("fragment", err)
		}

		// Send each fragment
		for _, frag := range fragments {
			if err := b.sendRaw(frag); err != nil {
				return err
			}
		}
		return nil
	}

	// 3. Send directly
	return b.sendRaw(processed)
}

// sendRaw sends raw data through channel.
func (b *Bus) sendRaw(data []byte) error {
	if err := b.channel.Send(data); err != nil {
		return b.handleError("channel_send", err)
	}
	return nil
}

// sendLoop handles async send.
func (b *Bus) sendLoop() {
	for {
		select {
		case <-b.stopCh:
			return
		case data := <-b.sendQueue:
			if err := b.sendSync(data); err != nil {
				b.handleError("send_loop", err)
			}
		}
	}
}

// Receive receives data from the bus.
// Processing: Channel.Receive -> [Fragment.Reassemble] -> CodecChain.Decode -> Serializer.Deserialize -> data
func (b *Bus) Receive() ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.running {
		return nil, errors.New("bus: not running")
	}

	return b.receiveSync()
}

// receiveSync receives data synchronously.
func (b *Bus) receiveSync() ([]byte, error) {
	// 1. Receive from channel
	data, err := b.channel.Receive()
	if err != nil {
		return nil, b.handleError("channel_receive", err)
	}

	// 2. Reassemble (if fragmentation enabled)
	if b.config.EnableFragment && b.fragment != nil {
		// Extract fragment info
		info, err := b.fragment.GetFragmentInfo(data)
		if err != nil {
			return nil, b.handleError("fragment_info", err)
		}

		// Add to fragment manager
		if err := b.fragmentManager.AddFragment(info.ID, int(info.Index), data); err != nil {
			return nil, b.handleError("fragment_add", err)
		}

		// Check if complete
		complete, err := b.fragmentManager.IsComplete(info.ID)
		if err != nil {
			return nil, b.handleError("fragment_check", err)
		}

		if !complete {
			// Wait for more fragments
			return nil, nil
		}

		// Reassemble
		data, err = b.fragmentManager.Reassemble(info.ID)
		if err != nil {
			return nil, b.handleError("fragment_reassemble", err)
		}
	}

	// 3. Decode
	decoded, err := b.codecChain.Decode(data)
	if err != nil {
		return nil, b.handleError("decode", err)
	}

	// 4. Deserialize
	result, err := b.serializer.Deserialize(decoded)
	if err != nil {
		return nil, b.handleError("deserialize", err)
	}

	return result, nil
}

// receiveLoop handles async receive.
func (b *Bus) receiveLoop() {
	for {
		select {
		case <-b.stopCh:
			return
		default:
			data, err := b.receiveSync()
			if err != nil {
				b.handleError("receive_loop", err)
				continue
			}
			if data != nil && b.messageHandler != nil {
				b.messageHandler(data)
			}
		}
	}
}

// handleError handles errors with error handler.
func (b *Bus) handleError(op string, err error) error {
	if b.errorHandler != nil {
		b.errorHandler(errors.New(op + ": " + err.Error()))
	}
	return err
}

// IsRunning returns running status.
func (b *Bus) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.running
}

// GetSessionID returns session ID.
func (b *Bus) GetSessionID() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.sessionID
}

// GetSerializer returns current serializer.
func (b *Bus) GetSerializer() serializer.Serializer {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.serializer
}

// GetCodecChain returns current codec chain.
func (b *Bus) GetCodecChain() codec.CodecChain {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.codecChain
}

// GetChannel returns current channel.
func (b *Bus) GetChannel() channel.Channel {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.channel
}

// GetFragment returns current fragment handler.
func (b *Bus) GetFragment() fragment.Fragment {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.fragment
}

// GetKeyProvider returns current key provider.
func (b *Bus) GetKeyProvider() keyprovider.KeyProvider {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.keyProvider
}

// GetConfig returns current configuration.
func (b *Bus) GetConfig() BusConfig {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.config
}

// SetConfig sets configuration.
func (b *Bus) SetConfig(config BusConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.config = config
}

// SecurityLevel returns the overall security level.
func (b *Bus) SecurityLevel() codec.SecurityLevel {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.codecChain == nil {
		return codec.SecurityLevelNone
	}
	return b.codecChain.SecurityLevel()
}

// BusBuilder provides fluent API for building Bus.
type BusBuilder struct {
	bus *Bus
}

// NewBuilder creates a new BusBuilder.
func NewBuilder() *BusBuilder {
	return &BusBuilder{
		bus: New(),
	}
}

// UseSerializerInstance sets the serializer instance.
func (b *BusBuilder) UseSerializerInstance(s serializer.Serializer) *BusBuilder {
	b.bus.SetSerializer(s)
	return b
}

// UseSerializer sets the serializer by name from global registry.
func (b *BusBuilder) UseSerializer(name string) *BusBuilder {
	s, err := serializer.Get(name)
	if err == nil {
		b.bus.SetSerializer(s)
	}
	return b
}

// UseCodecChain sets the codec chain.
func (b *BusBuilder) UseCodecChain(c codec.CodecChain) *BusBuilder {
	b.bus.SetCodecChain(c)
	return b
}

// UseCodec adds a codec to the chain.
func (b *BusBuilder) UseCodec(c codec.Codec) *BusBuilder {
	if b.bus.codecChain == nil {
		b.bus.codecChain = codec.NewChain()
	}
	b.bus.codecChain.AddCodec(c)
	return b
}

// UseChannel sets the channel.
func (b *BusBuilder) UseChannel(c channel.Channel) *BusBuilder {
	b.bus.SetChannel(c)
	return b
}

// UseKeyProvider sets the key provider.
func (b *BusBuilder) UseKeyProvider(kp keyprovider.KeyProvider) *BusBuilder {
	b.bus.SetKeyProvider(kp)
	return b
}

// UseFragment sets the fragment handler.
func (b *BusBuilder) UseFragment(f fragment.Fragment) *BusBuilder {
	b.bus.SetFragment(f)
	return b
}

// WithConfig sets the configuration.
func (b *BusBuilder) WithConfig(config BusConfig) *BusBuilder {
	b.bus.config = config
	return b
}

// OnMessage sets message handler.
func (b *BusBuilder) OnMessage(handler func([]byte)) *BusBuilder {
	b.bus.OnMessage(handler)
	return b
}

// OnError sets error handler.
func (b *BusBuilder) OnError(handler func(error)) *BusBuilder {
	b.bus.OnError(handler)
	return b
}

// Build builds and returns the Bus.
func (b *BusBuilder) Build() *Bus {
	return b.bus
}

// BuildAndStart builds and starts the Bus.
func (b *BusBuilder) BuildAndStart() (*Bus, error) {
	bus := b.Build()
	err := bus.Start()
	return bus, err
}

// Verify interface compliance
var (
	_ = time.Duration(0)
)
