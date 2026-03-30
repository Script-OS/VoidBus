// Package core provides MultiBus implementation for multi-channel transmission.
//
// MultiBus implements multi-channel combination strategy: data is fragmented
// and distributed across multiple channels for transmission.
//
// Design Constraints (see docs/ARCHITECTURE.md §3.3):
// - MultiBus manages multiple Channel instances
// - Data fragmentation distributed across channels
// - Channel selection strategy: random or specified
// - Channel config NOT exposed in metadata
// - FragmentInfo CAN be exposed (ID/Index/Total only)
// - Full-duplex on each channel
package core

import (
	"errors"
	"math/rand"
	"sync"
	"time"

	"VoidBus/channel"
	"VoidBus/codec"
	"VoidBus/fragment"
	"VoidBus/internal"
	"VoidBus/keyprovider"
	"VoidBus/serializer"
)

// MultiBus errors
var (
	ErrMultiBusNotRunning     = errors.New("multibus: not running")
	ErrMultiBusAlreadyRunning = errors.New("multibus: already running")
	ErrChannelsRequired       = errors.New("multibus: at least one channel required")
	ErrNoAvailableChannel     = errors.New("multibus: no available channel")
	ErrChannelSelectionFailed = errors.New("multibus: channel selection failed")
)

// MultiBus is the multi-channel bus implementation.
type MultiBus struct {
	mu sync.RWMutex

	// Core modules
	serializer  serializer.Serializer
	codecChain  codec.CodecChain
	fragment    fragment.Fragment
	keyProvider keyprovider.KeyProvider

	// Channel pool
	channels []*ChannelInfo

	// Configuration
	config MultiBusConfig

	// State
	running   bool
	stopCh    chan struct{}
	sessionID string

	// Fragment manager
	fragmentManager *fragment.DefaultFragmentManager

	// Channel selection state
	roundRobinIndex int

	// Handlers
	messageHandler func([]byte)
	errorHandler   func(error)

	// Send queues (per channel)
	sendQueues []chan []byte
}

// NewMultiBus creates a new MultiBus with default configuration.
func NewMultiBus() *MultiBus {
	return NewMultiBusWithConfig(DefaultMultiBusConfig())
}

// NewMultiBusWithConfig creates a new MultiBus with specified configuration.
func NewMultiBusWithConfig(config MultiBusConfig) *MultiBus {
	return &MultiBus{
		config:          config,
		stopCh:          make(chan struct{}),
		sessionID:       internal.GenerateSessionID(),
		channels:        make([]*ChannelInfo, 0),
		fragmentManager: fragment.NewFragmentManager(fragment.DefaultFragmentConfig()),
	}
}

// SetSerializer sets the serializer.
func (m *MultiBus) SetSerializer(s serializer.Serializer) *MultiBus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serializer = s
	return m
}

// SetCodecChain sets the codec chain.
func (m *MultiBus) SetCodecChain(c codec.CodecChain) *MultiBus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.codecChain = c
	return m
}

// SetFragment sets the fragment handler.
func (m *MultiBus) SetFragment(f fragment.Fragment) *MultiBus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fragment = f
	m.config.EnableFragmentation = true
	return m
}

// SetKeyProvider sets the key provider.
func (m *MultiBus) SetKeyProvider(kp keyprovider.KeyProvider) *MultiBus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keyProvider = kp
	if m.codecChain != nil {
		m.codecChain.SetKeyProvider(kp)
	}
	return m
}

// AddChannel adds a channel to the pool.
func (m *MultiBus) AddChannel(ch channel.Channel, weight int) *MultiBus {
	m.mu.Lock()
	defer m.mu.Unlock()

	index := len(m.channels)
	m.channels = append(m.channels, &ChannelInfo{
		Index:        index,
		Channel:      ch,
		Weight:       weight,
		Status:       ChannelStatusActive,
		LastActivity: time.Now().Unix(),
	})

	return m
}

// OnMessage registers message handler.
func (m *MultiBus) OnMessage(handler func([]byte)) *MultiBus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messageHandler = handler
	return m
}

// OnError registers error handler.
func (m *MultiBus) OnError(handler func(error)) *MultiBus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorHandler = handler
	return m
}

// Start starts the multi bus.
func (m *MultiBus) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return ErrMultiBusAlreadyRunning
	}

	// Validate required modules
	if m.serializer == nil {
		return errors.New("multibus: serializer required")
	}
	if m.codecChain == nil {
		return errors.New("multibus: codec chain required")
	}
	if len(m.channels) == 0 {
		return ErrChannelsRequired
	}

	// Set key provider for codec chain
	if m.keyProvider != nil {
		if err := m.codecChain.SetKeyProvider(m.keyProvider); err != nil {
			return err
		}
	}

	// Initialize send queues
	m.sendQueues = make([]chan []byte, len(m.channels))
	for i := range m.channels {
		m.sendQueues[i] = make(chan []byte, m.config.SendQueueSize)
	}

	// Reset stop channel
	m.stopCh = make(chan struct{})
	m.running = true

	// Start send loops for each channel
	for i := range m.channels {
		go m.sendLoop(i)
	}

	// Start receive loops for each channel
	for i := range m.channels {
		for j := 0; j < m.config.ReceiveWorkers; j++ {
			go m.receiveLoop(i)
		}
	}

	return nil
}

// Stop stops the multi bus.
func (m *MultiBus) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return ErrMultiBusNotRunning
	}

	m.running = false
	close(m.stopCh)

	// Close all channels
	for _, info := range m.channels {
		if info.Channel != nil && info.Channel.IsConnected() {
			info.Channel.Close()
		}
	}

	// Clear fragment manager
	m.fragmentManager.ClearAll()

	return nil
}

// Send sends data through the multi bus.
func (m *MultiBus) Send(data []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return ErrMultiBusNotRunning
	}

	return m.sendSync(data)
}

// sendSync sends data synchronously.
func (m *MultiBus) sendSync(data []byte) error {
	var err error
	processed := data

	// 1. Serialize
	processed, err = m.serializer.Serialize(processed)
	if err != nil {
		return m.handleError("serialize", err)
	}

	// 2. Encode
	processed, err = m.codecChain.Encode(processed)
	if err != nil {
		return m.handleError("encode", err)
	}

	// 3. Fragment (if enabled)
	if m.config.EnableFragmentation && m.fragment != nil {
		fragments, err := m.fragment.Split(processed, m.config.MaxFragmentSize)
		if err != nil {
			return m.handleError("fragment", err)
		}

		// Send each fragment through selected channel
		for _, frag := range fragments {
			channelIndex := m.selectChannel()
			if channelIndex < 0 {
				return m.handleError("channel_select", ErrNoAvailableChannel)
			}

			// Queue for async send
			if m.sendQueues[channelIndex] != nil {
				select {
				case m.sendQueues[channelIndex] <- frag:
					m.channels[channelIndex].SendCount++
					continue
				default:
					// Queue full, send synchronously
				}
			}

			// Send directly
			if err := m.sendFragment(channelIndex, frag); err != nil {
				return err
			}
		}
		return nil
	}

	// Send without fragmentation
	channelIndex := m.selectChannel()
	if channelIndex < 0 {
		return m.handleError("channel_select", ErrNoAvailableChannel)
	}

	return m.sendFragment(channelIndex, processed)
}

// sendFragment sends a fragment through specified channel.
func (m *MultiBus) sendFragment(channelIndex int, data []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if channelIndex >= len(m.channels) {
		return ErrChannelSelectionFailed
	}

	info := m.channels[channelIndex]
	if info.Channel == nil || !info.Channel.IsConnected() {
		info.Status = ChannelStatusError
		return ErrNoAvailableChannel
	}

	if err := info.Channel.Send(data); err != nil {
		info.ErrorCount++
		info.Status = ChannelStatusError
		return m.handleError("channel_send", err)
	}

	info.SendCount++
	info.LastActivity = time.Now().Unix()
	return nil
}

// selectChannel selects a channel based on strategy.
func (m *MultiBus) selectChannel() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Filter active channels
	activeChannels := make([]*ChannelInfo, 0)
	for _, info := range m.channels {
		if info.Status == ChannelStatusActive && info.Channel != nil && info.Channel.IsConnected() {
			activeChannels = append(activeChannels, info)
		}
	}

	if len(activeChannels) == 0 {
		return -1
	}

	switch m.config.SelectionStrategy {
	case StrategyRandom:
		// Random selection
		return activeChannels[rand.Intn(len(activeChannels))].Index

	case StrategyRoundRobin:
		// Round-robin selection
		m.roundRobinIndex = (m.roundRobinIndex + 1) % len(activeChannels)
		return activeChannels[m.roundRobinIndex].Index

	case StrategyWeighted:
		// Weighted selection (weighted random)
		totalWeight := 0
		for _, info := range activeChannels {
			totalWeight += info.Weight
		}
		if totalWeight == 0 {
			return activeChannels[rand.Intn(len(activeChannels))].Index
		}
		selectedWeight := rand.Intn(totalWeight)
		for _, info := range activeChannels {
			selectedWeight -= info.Weight
			if selectedWeight <= 0 {
				return info.Index
			}
		}
		return activeChannels[0].Index

	case StrategySpecified:
		// Specified channel
		if m.config.SpecifiedChannelIndex >= 0 && m.config.SpecifiedChannelIndex < len(activeChannels) {
			return activeChannels[m.config.SpecifiedChannelIndex].Index
		}
		return activeChannels[0].Index

	default:
		return activeChannels[rand.Intn(len(activeChannels))].Index
	}
}

// sendLoop handles async send for a channel.
func (m *MultiBus) sendLoop(channelIndex int) {
	for {
		select {
		case <-m.stopCh:
			return
		case data := <-m.sendQueues[channelIndex]:
			if err := m.sendFragment(channelIndex, data); err != nil {
				m.handleError("send_loop", err)
			}
		}
	}
}

// receiveLoop handles receive for a channel.
func (m *MultiBus) receiveLoop(channelIndex int) {
	for {
		select {
		case <-m.stopCh:
			return
		default:
			data, err := m.receiveFromChannel(channelIndex)
			if err != nil {
				m.handleError("receive_loop", err)
				continue
			}
			if data != nil && m.messageHandler != nil {
				m.messageHandler(data)
			}
		}
	}
}

// receiveFromChannel receives and processes data from a channel.
func (m *MultiBus) receiveFromChannel(channelIndex int) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if channelIndex >= len(m.channels) {
		return nil, ErrChannelSelectionFailed
	}

	info := m.channels[channelIndex]
	if info.Channel == nil || !info.Channel.IsConnected() {
		info.Status = ChannelStatusError
		return nil, ErrNoAvailableChannel
	}

	// Receive raw data
	rawData, err := info.Channel.Receive()
	if err != nil {
		info.ErrorCount++
		return nil, err
	}

	info.ReceiveCount++
	info.LastActivity = time.Now().Unix()

	// Check if fragmentation enabled
	if m.config.EnableFragmentation && m.fragment != nil {
		// Extract fragment info
		fragInfo, err := m.fragment.GetFragmentInfo(rawData)
		if err != nil {
			// Not a fragment, process directly
			return m.processData(rawData)
		}

		// Add to fragment manager
		if err := m.fragmentManager.AddFragment(fragInfo.ID, int(fragInfo.Index), rawData); err != nil {
			// State might not exist, create it
			if err == fragment.ErrStateNotFound {
				m.fragmentManager.CreateState(fragInfo.ID, int(fragInfo.Total))
				m.fragmentManager.AddFragment(fragInfo.ID, int(fragInfo.Index), rawData)
			} else {
				return nil, m.handleError("fragment_add", err)
			}
		}

		// Check if complete
		complete, err := m.fragmentManager.IsComplete(fragInfo.ID)
		if err != nil {
			return nil, m.handleError("fragment_check", err)
		}

		if !complete {
			// Wait for more fragments
			return nil, nil
		}

		// Reassemble
		reassembled, err := m.fragmentManager.Reassemble(fragInfo.ID)
		if err != nil {
			return nil, m.handleError("fragment_reassemble", err)
		}

		// Process reassembled data
		return m.processData(reassembled)
	}

	// Process directly without fragmentation
	return m.processData(rawData)
}

// processData processes data through decode and deserialize.
func (m *MultiBus) processData(data []byte) ([]byte, error) {
	// Decode
	decoded, err := m.codecChain.Decode(data)
	if err != nil {
		return nil, m.handleError("decode", err)
	}

	// Deserialize
	result, err := m.serializer.Deserialize(decoded)
	if err != nil {
		return nil, m.handleError("deserialize", err)
	}

	return result, nil
}

// handleError handles errors.
func (m *MultiBus) handleError(op string, err error) error {
	if m.errorHandler != nil {
		m.errorHandler(errors.New(op + ": " + err.Error()))
	}
	return err
}

// IsRunning returns running status.
func (m *MultiBus) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// GetSessionID returns session ID.
func (m *MultiBus) GetSessionID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionID
}

// ChannelCount returns number of channels.
func (m *MultiBus) ChannelCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.channels)
}

// ActiveChannelCount returns number of active channels.
func (m *MultiBus) ActiveChannelCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, info := range m.channels {
		if info.Status == ChannelStatusActive && info.Channel != nil && info.Channel.IsConnected() {
			count++
		}
	}
	return count
}

// GetChannelInfo returns channel info by index.
func (m *MultiBus) GetChannelInfo(index int) (*ChannelInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if index < 0 || index >= len(m.channels) {
		return nil, ErrChannelSelectionFailed
	}

	return m.channels[index], nil
}

// GetConfig returns current configuration.
func (m *MultiBus) GetConfig() MultiBusConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// SecurityLevel returns overall security level.
func (m *MultiBus) SecurityLevel() codec.SecurityLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.codecChain == nil {
		return codec.SecurityLevelNone
	}
	return m.codecChain.SecurityLevel()
}

// MultiBusBuilder provides fluent API for building MultiBus.
type MultiBusBuilder struct {
	multiBus *MultiBus
}

// NewMultiBusBuilder creates a new MultiBusBuilder.
func NewMultiBusBuilder() *MultiBusBuilder {
	return &MultiBusBuilder{
		multiBus: NewMultiBus(),
	}
}

// UseSerializerInstance sets the serializer instance.
func (b *MultiBusBuilder) UseSerializerInstance(s serializer.Serializer) *MultiBusBuilder {
	b.multiBus.SetSerializer(s)
	return b
}

// UseSerializerFromRegistry sets the serializer from registry.
func (b *MultiBusBuilder) UseSerializerFromRegistry(name string) *MultiBusBuilder {
	s, err := serializer.Get(name)
	if err == nil {
		b.multiBus.SetSerializer(s)
	}
	return b
}

// UseCodecChain sets the codec chain.
func (b *MultiBusBuilder) UseCodecChain(c codec.CodecChain) *MultiBusBuilder {
	b.multiBus.SetCodecChain(c)
	return b
}

// UseCodec adds a codec to the chain.
func (b *MultiBusBuilder) UseCodec(c codec.Codec) *MultiBusBuilder {
	if b.multiBus.codecChain == nil {
		b.multiBus.codecChain = codec.NewChain()
	}
	b.multiBus.codecChain.AddCodec(c)
	return b
}

// UseFragment sets the fragment handler.
func (b *MultiBusBuilder) UseFragment(f fragment.Fragment) *MultiBusBuilder {
	b.multiBus.SetFragment(f)
	return b
}

// UseKeyProvider sets the key provider.
func (b *MultiBusBuilder) UseKeyProvider(kp keyprovider.KeyProvider) *MultiBusBuilder {
	b.multiBus.SetKeyProvider(kp)
	return b
}

// AddChannel adds a channel to the pool.
func (b *MultiBusBuilder) AddChannel(ch channel.Channel, weight int) *MultiBusBuilder {
	b.multiBus.AddChannel(ch, weight)
	return b
}

// WithConfig sets the configuration.
func (b *MultiBusBuilder) WithConfig(config MultiBusConfig) *MultiBusBuilder {
	b.multiBus.config = config
	return b
}

// WithStrategy sets the channel selection strategy.
func (b *MultiBusBuilder) WithStrategy(strategy ChannelSelectionStrategy) *MultiBusBuilder {
	b.multiBus.config.SelectionStrategy = strategy
	return b
}

// WithFragmentation configures fragmentation.
func (b *MultiBusBuilder) WithFragmentation(maxSize int, enabled bool) *MultiBusBuilder {
	b.multiBus.config.MaxFragmentSize = maxSize
	b.multiBus.config.EnableFragmentation = enabled
	return b
}

// OnMessage sets message handler.
func (b *MultiBusBuilder) OnMessage(handler func([]byte)) *MultiBusBuilder {
	b.multiBus.OnMessage(handler)
	return b
}

// OnError sets error handler.
func (b *MultiBusBuilder) OnError(handler func(error)) *MultiBusBuilder {
	b.multiBus.OnError(handler)
	return b
}

// Build builds and returns the MultiBus.
func (b *MultiBusBuilder) Build() *MultiBus {
	return b.multiBus
}

// BuildAndStart builds and starts the MultiBus.
func (b *MultiBusBuilder) BuildAndStart() (*MultiBus, error) {
	mb := b.Build()
	err := mb.Start()
	return mb, err
}
