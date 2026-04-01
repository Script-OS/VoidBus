// Package voidbus provides the unified Bus for VoidBus.
package voidbus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/fragment"
	"github.com/Script-OS/VoidBus/internal"
	"github.com/Script-OS/VoidBus/keyprovider/embedded"
	"github.com/Script-OS/VoidBus/negotiate"
	"github.com/Script-OS/VoidBus/protocol"
	"github.com/Script-OS/VoidBus/session"
)

// Dependencies holds injectable module dependencies for Bus.
// Used for dependency injection in testing and custom configurations.
type Dependencies struct {
	CodecManager  *codec.CodecManager
	ChannelPool   *channel.ChannelPool
	FragmentMgr   *fragment.FragmentManager
	SessionMgr    *session.SessionManager
	AdaptiveTimer *internal.AdaptiveTimeout
	KeyProvider   *embedded.Provider
}

// ErrDependencyMissing indicates a required dependency is missing.
var ErrDependencyMissing = errors.New("voidbus: required dependency missing")

// Bus is the unified entry point for VoidBus v2.0.
// Implements Module interface for lifecycle management.
type Bus struct {
	mu     sync.RWMutex
	config *BusConfig

	// Managers (implement Module interface)
	codecManager  *codec.CodecManager
	channelPool   *channel.ChannelPool
	fragmentMgr   *fragment.FragmentManager
	sessionMgr    *session.SessionManager
	adaptiveTimer *internal.AdaptiveTimeout

	// Key provider
	keyProvider *embedded.Provider

	// State (atomic for thread-safe access)
	connected  atomic.Bool
	negotiated atomic.Bool
	running    atomic.Bool
	stopChan   chan struct{}
	wg         sync.WaitGroup // WaitGroup for goroutines

	// Receive
	recvQueue      chan []byte
	messageHandler func([]byte)
	errorHandler   func(error)

	// Channel ID counter
	channelIDCounter int

	// NAK batch queue (P1 optimization)
	nakQueue     map[string][]uint16 // sessionID -> missing indices
	nakQueueMu   sync.Mutex
	nakBatchSize int // Maximum NAK batch size

	// Parallel send semaphore (P1 optimization)
	sendSemaphore chan struct{}
}

// New creates a new Bus instance with default configuration.
// Returns error if configuration validation fails.
func New() (*Bus, error) {
	config := DefaultBusConfig()
	return NewWithConfig(config)
}

// NewWithConfig creates a new Bus with custom config.
// Returns error if configuration validation fails.
func NewWithConfig(config *BusConfig) (*Bus, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, WrapModuleError("NewWithConfig", "bus", err)
	}

	codecMgr := codec.NewCodecManager()
	channelPool := channel.NewChannelPool()
	fragmentMgr := fragment.NewFragmentManager(fragment.DefaultFragmentConfig())
	sessionMgr := session.NewSessionManager(session.DefaultSessionManagerConfig())
	adaptiveTimer := internal.NewAdaptiveTimeout(
		1*time.Second,
		30*time.Second,
	)

	return &Bus{
		config:        config,
		codecManager:  codecMgr,
		channelPool:   channelPool,
		fragmentMgr:   fragmentMgr,
		sessionMgr:    sessionMgr,
		adaptiveTimer: adaptiveTimer,
		recvQueue:     make(chan []byte, config.RecvBufferSize),
		stopChan:      make(chan struct{}),
		nakQueue:      make(map[string][]uint16),
		nakBatchSize:  10,                     // Maximum 10 missing indices per NAK
		sendSemaphore: make(chan struct{}, 8), // Maximum 8 parallel sends
	}, nil
}

// NewWithDependencies creates a new Bus with injected dependencies.
// This is primarily used for testing with mock implementations.
//
// Required dependencies:
//   - CodecManager (must not be nil)
//   - ChannelPool (must not be nil)
//   - FragmentMgr (must not be nil)
//   - SessionMgr (must not be nil)
//   - AdaptiveTimer (must not be nil)
//
// Optional dependencies:
//   - KeyProvider (can be nil, set via SetKey later)
//
// Returns error if any required dependency is missing or config validation fails.
func NewWithDependencies(config *BusConfig, deps *Dependencies) (*Bus, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, WrapModuleError("NewWithDependencies", "bus", err)
	}

	// Validate required dependencies
	if deps == nil {
		return nil, WrapModuleError("NewWithDependencies", "bus", ErrDependencyMissing)
	}

	if deps.CodecManager == nil {
		return nil, WrapModuleError("NewWithDependencies", "bus",
			CriticalError("CodecManager", "dependencies", "CodecManager is required", ErrDependencyMissing))
	}

	if deps.ChannelPool == nil {
		return nil, WrapModuleError("NewWithDependencies", "bus",
			CriticalError("ChannelPool", "dependencies", "ChannelPool is required", ErrDependencyMissing))
	}

	if deps.FragmentMgr == nil {
		return nil, WrapModuleError("NewWithDependencies", "bus",
			CriticalError("FragmentMgr", "dependencies", "FragmentMgr is required", ErrDependencyMissing))
	}

	if deps.SessionMgr == nil {
		return nil, WrapModuleError("NewWithDependencies", "bus",
			CriticalError("SessionMgr", "dependencies", "SessionMgr is required", ErrDependencyMissing))
	}

	if deps.AdaptiveTimer == nil {
		return nil, WrapModuleError("NewWithDependencies", "bus",
			CriticalError("AdaptiveTimer", "dependencies", "AdaptiveTimer is required", ErrDependencyMissing))
	}

	return &Bus{
		config:        config,
		codecManager:  deps.CodecManager,
		channelPool:   deps.ChannelPool,
		fragmentMgr:   deps.FragmentMgr,
		sessionMgr:    deps.SessionMgr,
		adaptiveTimer: deps.AdaptiveTimer,
		keyProvider:   deps.KeyProvider,
		recvQueue:     make(chan []byte, config.RecvBufferSize),
		stopChan:      make(chan struct{}),
		nakQueue:      make(map[string][]uint16),
		nakBatchSize:  10,                     // Maximum 10 missing indices per NAK
		sendSemaphore: make(chan struct{}, 8), // Maximum 8 parallel sends
	}, nil
}

// Name returns the module name (implements Module interface).
func (b *Bus) Name() string {
	return "Bus"
}

// ModuleStats returns bus statistics (implements Module interface).
func (b *Bus) ModuleStats() interface{} {
	return b.statsInternal()
}

// statsInternal returns internal statistics.
func (b *Bus) statsInternal() BusStats {
	return BusStats{
		Connected:     b.connected.Load(),
		Negotiated:    b.negotiated.Load(),
		Running:       b.running.Load(),
		ChannelCount:  b.channelPool.Count(),
		CodecCount:    b.codecManager.CodecCount(),
		SessionStats:  b.sessionMgr.Stats(),
		FragmentStats: b.fragmentMgr.Stats(),
		TimerStats:    b.adaptiveTimer.GetSRTT(),
	}
}

// === Codec Configuration ===

// RegisterCodec registers a codec with its user-defined code.
// The code is used for chain hash computation and MUST be consistent
// between sender and receiver.
//
// Example:
//
//	aesCodec := aes.NewAES256Codec(...)
//	bus.RegisterCodec(aesCodec)  // Uses codec.Code() which should return "aes" or similar
func (b *Bus) RegisterCodec(c codec.Codec) error {
	code := c.Code()
	if code == "" {
		return errors.New("codec code cannot be empty")
	}

	if err := b.codecManager.RegisterCodec(c, code); err != nil {
		return WrapModuleError("RegisterCodec", "codec", err)
	}

	// Set key provider if codec needs it
	if kc, ok := c.(codec.KeyAwareCodec); ok && b.keyProvider != nil {
		kc.SetKeyProvider(b.keyProvider)
	}

	return nil
}

// AddCodec is deprecated, use RegisterCodec.
// This method attempts to extract code from codec.Code() or falls back to InternalID.
func (b *Bus) AddCodec(c codec.Codec) error {
	return b.RegisterCodec(c)
}

// SetKey sets the key provider with embedded key.
// Returns error if key initialization fails (instead of silently failing).
func (b *Bus) SetKey(key []byte) error {
	provider, err := embedded.New(key, "", "AES-256-GCM")
	if err != nil {
		return WrapModuleError("SetKey", "keyprovider", err)
	}
	b.keyProvider = provider
	return nil
}

// SetKeyProvider sets a custom key provider.
func (b *Bus) SetKeyProvider(provider *embedded.Provider) {
	b.keyProvider = provider
}

// SetMaxCodecDepth sets the maximum codec chain depth.
func (b *Bus) SetMaxCodecDepth(depth int) error {
	return b.codecManager.SetMaxDepth(depth)
}

// === Channel Configuration ===

// AddChannel adds a channel to the pool with auto-generated ID.
func (b *Bus) AddChannel(c channel.Channel) error {
	b.channelIDCounter++
	id := string(c.Type()) + "-" + internal.GenerateShortID()
	return b.AddChannelWithID(c, id)
}

// AddChannelWithID adds a channel with specified ID.
func (b *Bus) AddChannelWithID(c channel.Channel, id string) error {
	if err := b.channelPool.AddChannel(c, id); err != nil {
		return WrapModuleError("AddChannel", "channel", err)
	}
	return nil
}

// SetChannelMTU is deprecated - MTU is now automatically determined.
func (b *Bus) SetChannelMTU(channelID string, mtu int) error {
	// MTU is now automatically determined from channel.DefaultMTU()
	return nil
}

// === Connection & Negotiation ===

// Connect connects to remote.
func (b *Bus) Connect(remoteAddr string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.connected.Load() {
		return ErrBusAlreadyRunning
	}

	b.connected.Store(true)
	return nil
}

// Negotiate performs capability negotiation with remote.
// Deprecated: Use SetNegotiatedBitmap after negotiate module handshake.
func (b *Bus) Negotiate(remoteCodes []string, remoteMaxDepth int, salt []byte) (*NegotiationConfig, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Set negotiated codes directly
	if err := b.codecManager.SetNegotiatedCodes(remoteCodes); err != nil {
		return nil, WrapModuleError("Negotiate", "codec", err)
	}

	// Set max depth
	if remoteMaxDepth > 0 && remoteMaxDepth < b.codecManager.GetMaxDepth() {
		b.codecManager.SetMaxDepth(remoteMaxDepth)
	}

	// Set salt
	if len(salt) > 0 {
		b.codecManager.SetSalt(salt)
	}

	maxDepth := b.codecManager.GetMaxDepth()
	b.negotiated.Store(true)

	return &NegotiationConfig{
		SupportedCodes: b.codecManager.GetAvailableCodes(),
		MaxDepth:       maxDepth,
		NegotiatedAt:   time.Now(),
	}, nil
}

// CreateNegotiateRequest creates a negotiation request with auto-generated bitmaps.
// Bitmaps are automatically generated from registered codecs and channels.
// The request should be sent through the default channel (WebSocket by default).
//
// Usage:
//
//	request, err := bus.CreateNegotiateRequest()
//	encoded, err := request.Encode()
//	// Send encoded bytes through default channel (WebSocket)
func (b *Bus) CreateNegotiateRequest() (*negotiate.NegotiateRequest, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Auto-generate bitmaps from registered codecs and channels
	codecBitmap := b.codecManager.GenerateCodecBitmap()
	channelBitmap := b.channelPool.GenerateChannelBitmap()

	// Validate bitmaps are not empty
	if isBitmapEmpty(codecBitmap) {
		return nil, errors.New("no codecs registered")
	}
	if isBitmapEmpty(channelBitmap) {
		return nil, errors.New("no channels registered")
	}

	// Create negotiate request using negotiate package
	return negotiate.NewNegotiateRequest(channelBitmap, codecBitmap)
}

// ApplyNegotiateResponse applies server's negotiation response.
// This sets the negotiated bitmaps and marks the bus as negotiated.
//
// Usage:
//
//	response, err := negotiate.DecodeNegotiateResponse(serverData)
//	err = bus.ApplyNegotiateResponse(response)
func (b *Bus) ApplyNegotiateResponse(response *negotiate.NegotiateResponse) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Apply negotiated bitmaps
	if err := b.codecManager.SetNegotiatedBitmap(response.CodecBitmap); err != nil {
		return WrapModuleError("ApplyNegotiateResponse", "codec", err)
	}

	// Set negotiated channel types
	b.channelPool.SetNegotiatedChannelBitmap(response.ChannelBitmap)

	b.negotiated.Store(true)
	return nil
}

// isBitmapEmpty checks if a bitmap has no bits set.
func isBitmapEmpty(bitmap []byte) bool {
	for _, b := range bitmap {
		if b != 0 {
			return false
		}
	}
	return true
}

// SetNegotiatedBitmap sets the negotiated codec bitmap.
func (b *Bus) SetNegotiatedBitmap(bitmap []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.codecManager.SetNegotiatedBitmap(bitmap); err != nil {
		return WrapModuleError("SetNegotiatedBitmap", "codec", err)
	}

	b.negotiated.Store(true)
	return nil
}

// GetNegotiationInfo returns negotiation info for sending to remote.
func (b *Bus) GetNegotiationInfo() ([]string, int) {
	return b.codecManager.GetAvailableCodes(), b.codecManager.GetMaxDepth()
}

// === Sending (P1: Parallel Send Optimization) ===

// Send sends data through the bus with parallel fragment distribution.
func (b *Bus) Send(data []byte) error {
	return b.SendWithContext(context.Background(), data)
}

// SendWithContext sends data with context for timeout/cancellation.
func (b *Bus) SendWithContext(ctx context.Context, data []byte) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.connected.Load() {
		return ErrChannelNotReady
	}

	if !b.negotiated.Load() {
		return ErrNegotiationFailed
	}

	// Acquire semaphore for parallel send control
	select {
	case b.sendSemaphore <- struct{}{}:
		defer func() { <-b.sendSemaphore }()
	case <-ctx.Done():
		return ctx.Err()
	case <-b.stopChan:
		return ErrBusNotRunning
	}

	// 1. Randomly select codec chain
	chain, codecHash, err := b.codecManager.SelectChain()
	if err != nil {
		return WrapModuleError("SelectChain", "codec", err)
	}

	// 2. Compute data hash
	dataHash := internal.ComputeDataHash(data)

	// 3. Create session
	sess := b.sessionMgr.CreateSendSession(nil, codecHash, 0, dataHash)

	// 4. Encode data
	encodedData, err := chain.Encode(data)
	if err != nil {
		b.sessionMgr.RemoveSendSession(sess.ID)
		return WrapModuleError("Encode", "codec", err)
	}

	// 5. Get adaptive MTU
	mtu := b.channelPool.GetAdaptiveMTU()
	if mtu == 0 {
		mtu = b.config.DefaultMTU
	}

	// 7. Adaptive split
	fragments, checksums, err := b.fragmentMgr.AdaptiveSplit(encodedData, mtu)
	if err != nil {
		b.sessionMgr.RemoveSendSession(sess.ID)
		return WrapModuleError("AdaptiveSplit", "fragment", err)
	}

	// 8. Create send buffer
	sendBuf := b.fragmentMgr.CreateSendBuffer(sess.ID, data)
	sendBuf.SetCodecInfo(nil, codecHash)
	sendBuf.SetEncodedData(encodedData)
	sess.SetTotalFragments(uint16(len(fragments)))

	// 9. Parallel fragment distribution (P1 optimization)
	errChan := make(chan error, len(fragments))
	var sendWg sync.WaitGroup

	for i, fragData := range fragments {
		sendWg.Add(1)
		go func(index int, fragment []byte, checksum uint32) {
			defer sendWg.Done()

			// Select channel for this fragment
			chInfo, err := b.channelPool.SelectChannel(nil)
			if err != nil {
				errChan <- ErrNoHealthyChannel
				return
			}

			if chInfo == nil {
				errChan <- ErrNoHealthyChannel
				return
			}

			// Build Header
			header := &protocol.Header{
				SessionID:     sess.ID,
				FragmentIndex: uint16(index),
				FragmentTotal: uint16(len(fragments)),
				CodecDepth:    0, // Deprecated in v2.1
				CodecHash:     codecHash,
				DataChecksum:  checksum,
				DataHash:      dataHash,
				Flags:         0,
			}
			if index == len(fragments)-1 {
				header.Flags |= protocol.FlagIsLast
			}

			// Encode header + fragment
			packet := header.Encode(fragment)

			// Send
			if err := chInfo.Channel.Send(packet); err != nil {
				b.channelPool.RecordError(chInfo.ID)
				errChan <- err
				return
			}

			// Record success
			b.channelPool.RecordSend(chInfo.ID, time.Millisecond)
			sendBuf.AddFragment(uint16(index), fragment, checksum, chInfo.ID)
			sess.IncrementSent()
		}(i, fragData, checksums[i])
	}

	// Wait for all sends to complete
	sendWg.Wait()

	// Check for errors
	select {
	case err := <-errChan:
		// At least one send failed, but we continue
		// The receiver will request NAK for missing fragments
		if b.errorHandler != nil {
			b.errorHandler(WrapModuleError("ParallelSend", "channel", err))
		}
	default:
		// All sends successful
	}

	// Update adaptive timer
	b.adaptiveTimer.AddMeasurement(time.Since(sess.CreatedAt))

	return nil
}

// === Receiving ===

// Receive receives data (blocking mode).
func (b *Bus) Receive() ([]byte, error) {
	select {
	case data := <-b.recvQueue:
		return data, nil
	case <-b.stopChan:
		return nil, ErrBusNotRunning
	}
}

// ReceiveWithContext receives data with context.
func (b *Bus) ReceiveWithContext(ctx context.Context) ([]byte, error) {
	select {
	case data := <-b.recvQueue:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.stopChan:
		return nil, ErrBusNotRunning
	}
}

// OnMessage sets message handler (callback mode).
func (b *Bus) OnMessage(handler func([]byte)) {
	b.messageHandler = handler
}

// OnError sets error handler.
func (b *Bus) OnError(handler func(error)) {
	b.errorHandler = handler
}

// StartReceive starts the background receive loop for all channels.
// This is the P0 fix - now actually starts receive goroutines.
func (b *Bus) StartReceive() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running.Load() {
		return ErrBusAlreadyRunning
	}

	if !b.connected.Load() {
		return ErrChannelNotReady
	}

	b.running.Store(true)

	// Start NAK batch timer (P1 optimization)
	go b.nakBatchLoop()

	// Start receive loop for each channel
	channelIDs := b.channelPool.GetChannelIDs()
	for _, chID := range channelIDs {
		info, err := b.channelPool.GetChannel(chID)
		if err != nil {
			continue
		}

		b.wg.Add(1)
		go b.receiveLoop(info)
	}

	return nil
}

// receiveLoop handles receiving from a channel.
func (b *Bus) receiveLoop(info *channel.ChannelInfo) {
	defer b.wg.Done()

	for {
		select {
		case <-b.stopChan:
			return
		default:
			data, err := info.Channel.Receive()
			if err != nil {
				b.channelPool.RecordError(info.ID)
				if b.errorHandler != nil {
					b.errorHandler(WrapModuleError("Receive", "channel", err))
				}
				// Brief pause before retry
				time.Sleep(100 * time.Millisecond)
				continue
			}

			b.channelPool.RecordSend(info.ID, time.Millisecond)

			// Process received packet
			if err := b.processReceivedPacket(data, info); err != nil {
				if b.errorHandler != nil {
					b.errorHandler(err)
				}
			}
		}
	}
}

// processReceivedPacket processes a received packet.
func (b *Bus) processReceivedPacket(data []byte, info *channel.ChannelInfo) error {
	// Decode header
	header, fragmentData, err := protocol.DecodeHeader(data)
	if err != nil {
		return WrapModuleError("DecodeHeader", "protocol", err)
	}

	// Check packet type
	if header.IsNAK() {
		return b.handleNAK(header, fragmentData)
	}

	if header.IsEND_ACK() {
		return b.handleEND_ACK(header)
	}

	// Regular fragment
	return b.handleFragment(header, fragmentData, info)
}

// handleFragment handles a regular data fragment.
func (b *Bus) handleFragment(header *protocol.Header, fragmentData []byte, info *channel.ChannelInfo) error {
	// Get or create receive buffer
	buf, err := b.fragmentMgr.GetRecvBuffer(header.SessionID)
	if err != nil {
		// Create new receive buffer
		buf = b.fragmentMgr.CreateRecvBuffer(
			header.SessionID,
			header.FragmentTotal,
			header.CodecDepth,
			header.CodecHash,
			header.DataHash,
		)

		// Create receive session
		b.sessionMgr.CreateRecvSession(
			header.SessionID,
			nil,
			header.CodecHash,
			int(header.CodecDepth),
			header.DataHash,
		)
	}

	// Add fragment
	if !buf.AddFragment(header.FragmentIndex, fragmentData, header.DataChecksum) {
		return nil
	}

	// Check if complete
	if !buf.IsComplete() {
		// Queue NAK for missing fragments (P1 batch optimization)
		missing := buf.GetMissing()
		if len(missing) > 0 {
			b.queueNAK(header.SessionID, missing)
		}
		return nil
	}

	// Reassemble
	encodedData, err := buf.Reassemble()
	if err != nil {
		return WrapModuleError("Reassemble", "fragment", err)
	}

	// Match codec chain by hash
	chain, err := b.codecManager.MatchChain(header.CodecHash)
	if err != nil {
		return WrapModuleError("MatchChain", "codec", err)
	}

	// Decode
	decodedData, err := chain.Decode(encodedData)
	if err != nil {
		return WrapModuleError("Decode", "codec", err)
	}

	// Verify data hash
	if !internal.VerifyDataHash(decodedData, header.DataHash) {
		return ErrFragmentCorrupted
	}

	// Send END_ACK
	if err := b.sendEND_ACK(header.SessionID); err != nil {
		return err
	}

	// Complete session
	b.sessionMgr.CompleteRecvSession(header.SessionID)
	b.fragmentMgr.RemoveRecvBuffer(header.SessionID)

	// Deliver to application
	if b.config.ReceiveMode == ReceiveModeBlocking {
		select {
		case b.recvQueue <- decodedData:
		default:
			// Queue full, drop
		}
	} else if b.messageHandler != nil {
		b.messageHandler(decodedData)
	}

	return nil
}

// === NAK Handling (P1: Batch Optimization) ===

// queueNAK queues NAK request for batch processing.
func (b *Bus) queueNAK(sessionID string, missing []uint16) {
	b.nakQueueMu.Lock()
	defer b.nakQueueMu.Unlock()

	// Append to existing queue or create new
	existing := b.nakQueue[sessionID]
	b.nakQueue[sessionID] = append(existing, missing...)

	// Limit batch size
	if len(b.nakQueue[sessionID]) > b.nakBatchSize {
		b.nakQueue[sessionID] = b.nakQueue[sessionID][:b.nakBatchSize]
	}
}

// nakBatchLoop periodically sends batched NAK requests.
func (b *Bus) nakBatchLoop() {
	ticker := time.NewTicker(500 * time.Millisecond) // Batch NAK every 500ms
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.sendBatchedNAKs()
		case <-b.stopChan:
			return
		}
	}
}

// sendBatchedNAKs sends all queued NAK requests.
func (b *Bus) sendBatchedNAKs() {
	b.nakQueueMu.Lock()
	queued := b.nakQueue
	b.nakQueue = make(map[string][]uint16)
	b.nakQueueMu.Unlock()

	for sessionID, missing := range queued {
		if len(missing) == 0 {
			continue
		}

		if err := b.sendNAK(sessionID, missing); err != nil {
			if b.errorHandler != nil {
				b.errorHandler(WrapModuleError("sendBatchedNAKs", "channel", err))
			}
		}
	}
}

// handleNAK handles a NAK message.
func (b *Bus) handleNAK(header *protocol.Header, extraData []byte) error {
	// Decode missing indices from extra data
	missing := decodeNAKIndices(extraData)

	// Get send buffer
	buf, err := b.fragmentMgr.GetSendBuffer(header.SessionID)
	if err != nil {
		return err
	}

	// Get session
	sess, err := b.sessionMgr.GetSendSession(header.SessionID)
	if err != nil {
		return err
	}

	// Check retransmit limit
	if !sess.IncrementRetransmit() {
		sess.MarkExpired()
		b.fragmentMgr.RemoveSendBuffer(header.SessionID)
		return ErrRetransmitExceeded
	}

	// Get fragments for retransmission
	fragments := buf.GetMissingFragments(missing)

	// Parallel retransmit (P1 optimization)
	var retransmitWg sync.WaitGroup
	errChan := make(chan error, len(fragments))

	for _, frag := range fragments {
		retransmitWg.Add(1)
		go func(f *fragment.FragmentEntry) {
			defer retransmitWg.Done()

			// Select healthy channel for retransmit
			chInfo, err := b.channelPool.SelectChannel(nil)
			if err != nil {
				errChan <- ErrNoHealthyChannel
				return
			}

			// Build header
			_, totalFragments := sess.GetProgress()
			retransmitHeader := &protocol.Header{
				SessionID:     header.SessionID,
				FragmentIndex: f.Index,
				FragmentTotal: totalFragments,
				CodecDepth:    uint8(len(buf.CodecCodes)),
				CodecHash:     buf.CodecHash,
				DataChecksum:  f.Checksum,
				DataHash:      buf.DataHash,
				Flags:         protocol.FlagRetransmit,
			}

			packet := retransmitHeader.Encode(f.Data)

			if err := chInfo.Channel.Send(packet); err != nil {
				b.channelPool.RecordError(chInfo.ID)
				errChan <- err
				return
			}

			b.channelPool.RecordSend(chInfo.ID, time.Millisecond)
		}(frag)
	}

	retransmitWg.Wait()

	// Check for errors (non-blocking)
	select {
	case err := <-errChan:
		if b.errorHandler != nil {
			b.errorHandler(WrapModuleError("Retransmit", "channel", err))
		}
	default:
	}

	return nil
}

// handleEND_ACK handles an END_ACK message.
func (b *Bus) handleEND_ACK(header *protocol.Header) error {
	// Complete send session
	if err := b.sessionMgr.CompleteSendSession(header.SessionID); err != nil {
		return err
	}

	// Remove send buffer
	b.fragmentMgr.RemoveSendBuffer(header.SessionID)

	return nil
}

// sendNAK sends a NAK message for missing fragments.
func (b *Bus) sendNAK(sessionID string, missing []uint16) error {
	// Select channel
	chInfo, err := b.channelPool.SelectChannel(nil)
	if err != nil {
		return ErrNoHealthyChannel
	}

	// Build NAK header
	header := &protocol.Header{
		SessionID: sessionID,
		Flags:     protocol.FlagIsNAK,
	}

	// Encode missing indices into extra data
	extra := encodeNAKIndices(missing)
	packet := header.Encode(extra)

	return chInfo.Channel.Send(packet)
}

// sendEND_ACK sends an END_ACK message.
func (b *Bus) sendEND_ACK(sessionID string) error {
	// Select channel
	chInfo, err := b.channelPool.SelectChannel(nil)
	if err != nil {
		return ErrNoHealthyChannel
	}

	header := &protocol.Header{
		SessionID: sessionID,
		Flags:     protocol.FlagIsENDACK,
	}

	packet := header.Encode(nil)

	return chInfo.Channel.Send(packet)
}

// encodeNAKIndices encodes missing indices for NAK packet.
func encodeNAKIndices(indices []uint16) []byte {
	result := make([]byte, len(indices)*2)
	for i, idx := range indices {
		result[i*2] = byte(idx >> 8)
		result[i*2+1] = byte(idx)
	}
	return result
}

// decodeNAKIndices decodes missing indices from NAK packet.
func decodeNAKIndices(data []byte) []uint16 {
	if len(data) == 0 || len(data)%2 != 0 {
		return nil
	}

	indices := make([]uint16, len(data)/2)
	for i := 0; i < len(indices); i++ {
		indices[i] = uint16(data[i*2])<<8 | uint16(data[i*2+1])
	}
	return indices
}

// === Lifecycle ===

// Start starts the bus.
func (b *Bus) Start() error {
	return b.StartReceive()
}

// Stop stops the bus and all goroutines.
func (b *Bus) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running.Load() {
		return ErrBusNotRunning
	}

	b.running.Store(false)
	close(b.stopChan)

	// Wait for all goroutines to finish
	b.wg.Wait()

	// Stop managers
	b.fragmentMgr.Stop()
	b.sessionMgr.Stop()

	return nil
}

// Close closes the bus and releases all resources.
func (b *Bus) Close() error {
	return b.Stop()
}

// IsRunning returns whether the bus is running.
func (b *Bus) IsRunning() bool {
	return b.running.Load()
}

// IsConnected returns whether the bus is connected.
func (b *Bus) IsConnected() bool {
	return b.connected.Load()
}

// === Statistics ===

// Stats returns bus statistics.
func (b *Bus) Stats() BusStats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.statsInternal()
}

// BusStats holds bus statistics.
type BusStats struct {
	Connected     bool
	Negotiated    bool
	Running       bool
	ChannelCount  int
	CodecCount    int
	SessionStats  session.SessionManagerStats
	FragmentStats fragment.FragmentStats
	TimerStats    time.Duration
}

// === Debug ===

// SetDebugMode enables debug mode.
func (b *Bus) SetDebugMode(enable bool) {
	b.config.DebugMode = enable
}

// GetConfig returns current configuration.
func (b *Bus) GetConfig() *BusConfig {
	return b.config
}

// GetCodecManager returns codec manager.
func (b *Bus) GetCodecManager() *codec.CodecManager {
	return b.codecManager
}

// GetChannelPool returns channel pool.
func (b *Bus) GetChannelPool() *channel.ChannelPool {
	return b.channelPool
}

// GetSessionManager returns session manager.
func (b *Bus) GetSessionManager() *session.SessionManager {
	return b.sessionMgr
}

// GetFragmentManager returns fragment manager.
func (b *Bus) GetFragmentManager() *fragment.FragmentManager {
	return b.fragmentMgr
}

// Validate validates bus configuration.
func (b *Bus) Validate() error {
	if b.codecManager.CodecCount() == 0 {
		return ErrCodecChainRequired
	}
	if b.channelPool.Count() == 0 {
		return ErrChannelRequired
	}
	return b.config.Validate()
}

// uint32ToBytes32 converts uint32 to [32]byte.
func uint32ToBytes32(h uint32) [32]byte {
	var result [32]byte
	result[0] = byte(h >> 24)
	result[1] = byte(h >> 16)
	result[2] = byte(h >> 8)
	result[3] = byte(h)
	return result
}

// bytes32ToUint32 converts [32]byte to uint32 (first 4 bytes).
func bytes32ToUint32(h [32]byte) uint32 {
	return uint32(h[0])<<24 | uint32(h[1])<<16 | uint32(h[2])<<8 | uint32(h[3])
}
