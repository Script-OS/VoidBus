// Package voidbus provides the unified Bus for VoidBus.
//
// VoidBus provides a message-oriented communication layer with:
// - net.Conn/net.Listener style API (Dial/Listen)
// - Automatic negotiation (codec/channel matching)
// - Message fragmentation and reassembly
// - Multi-channel distribution
// - Codec chain encoding/decoding
//
// Basic usage (Client):
//
//	bus := voidbus.New(nil)
//	bus.RegisterCodec(base64.New())
//	bus.AddChannel(tcp.NewClientChannel(config))
//	conn, _ := bus.Dial(channel)
//	conn.Write([]byte("Hello"))
//	buf := make([]byte, 1024)
//	n, _ := conn.Read(buf)
//	conn.Close()
//
// Basic usage (Server):
//
//	bus := voidbus.New(nil)
//	bus.RegisterCodec(base64.New())
//	bus.AddChannel(tcp.NewServerChannel(config))
//	listener, _ := bus.Listen(channel)
//	conn, _ := listener.Accept()
//	n, _ := conn.Read(buf)
//	conn.Write([]byte("Echo"))
//	conn.Close()
package voidbus

import (
	"context"
	"errors"
	"fmt"
	"net"
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

// Bus is the unified entry point for VoidBus.
// Use Dial for client mode, Listen for server mode.
type Bus struct {
	mu     sync.RWMutex
	config *BusConfig

	// Managers
	codecManager  *codec.CodecManager
	channelPool   *channel.ChannelPool
	fragmentMgr   *fragment.FragmentManager
	sessionMgr    *session.SessionManager
	adaptiveTimer *internal.AdaptiveTimeout

	// Key provider
	keyProvider *embedded.Provider

	// Server channels (for Listen aggregation)
	serverChannels map[string]channel.ServerChannel

	// State
	connected  atomic.Bool
	negotiated atomic.Bool
	running    atomic.Bool
	stopChan   chan struct{}
	wg         sync.WaitGroup

	// Receive
	recvQueue chan []byte

	// Channel ID counter
	channelIDCounter int

	// NAK batch queue
	nakQueue     map[string][]uint16
	nakQueueMu   sync.Mutex
	nakBatchSize int

	// Send semaphore
	sendSemaphore chan struct{}
}

// New creates a new Bus instance with default configuration.
func New(config *BusConfig) (*Bus, error) {
	if config == nil {
		config = DefaultBusConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, WrapModuleError("New", "bus", err)
	}

	return &Bus{
		config:         config,
		codecManager:   codec.NewCodecManager(),
		channelPool:    channel.NewChannelPool(),
		fragmentMgr:    fragment.NewFragmentManager(fragment.DefaultFragmentConfig()),
		sessionMgr:     session.NewSessionManager(session.DefaultSessionManagerConfig()),
		adaptiveTimer:  internal.NewAdaptiveTimeout(1*time.Second, 30*time.Second),
		recvQueue:      make(chan []byte, config.RecvBufferSize),
		stopChan:       make(chan struct{}),
		nakQueue:       make(map[string][]uint16),
		nakBatchSize:   10,
		sendSemaphore:  make(chan struct{}, 8),
		serverChannels: make(map[string]channel.ServerChannel),
	}, nil
}

// === Codec Configuration ===

// RegisterCodec registers a codec with its user-defined code.
func (b *Bus) RegisterCodec(c codec.Codec) error {
	code := c.Code()
	if code == "" {
		return errors.New("codec code cannot be empty")
	}
	if err := b.codecManager.RegisterCodec(c, code); err != nil {
		return WrapModuleError("RegisterCodec", "codec", err)
	}
	if kc, ok := c.(codec.KeyAwareCodec); ok && b.keyProvider != nil {
		kc.SetKeyProvider(b.keyProvider)
	}
	return nil
}

// SetKey sets the key provider with embedded key.
func (b *Bus) SetKey(key []byte) error {
	provider, err := embedded.New(key, "", "AES-256-GCM")
	if err != nil {
		return WrapModuleError("SetKey", "keyprovider", err)
	}
	b.keyProvider = provider
	return nil
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
// If the channel is a ServerChannel, it's also stored for Listen aggregation.
func (b *Bus) AddChannelWithID(c channel.Channel, id string) error {
	if err := b.channelPool.AddChannel(c, id); err != nil {
		return WrapModuleError("AddChannel", "channel", err)
	}

	// Check if this is a ServerChannel
	if serverCh, ok := c.(channel.ServerChannel); ok {
		b.serverChannels[id] = serverCh
	}

	return nil
}

// === Dial/Listen API ===

// Dial initiates a client connection using the default channel (first registered).
// Returns net.Conn for subsequent Read/Write operations.
//
// After initial negotiation, all registered channels are associated to the same session.
func (b *Bus) Dial() (net.Conn, error) {
	return b.dialWithChannel(nil)
}

// DialChannel initiates a client connection using a specific channel.
// Returns net.Conn for subsequent Read/Write operations.
//
// After initial negotiation, all registered channels are associated to the same session.
func (b *Bus) DialChannel(ch channel.Channel) (net.Conn, error) {
	return b.dialWithChannel(ch)
}

// dialWithChannel is the internal implementation.
// If ch is nil, uses the first registered channel.
func (b *Bus) dialWithChannel(ch channel.Channel) (net.Conn, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.connected.Load() {
		return nil, ErrBusAlreadyRunning
	}

	// Get all channel IDs
	channelIDs := b.channelPool.GetChannelIDs()
	if len(channelIDs) == 0 {
		return nil, errors.New("no channels registered")
	}

	// Determine which channel to use for initial negotiation
	var negotiateChID string
	var negotiateCh channel.Channel

	if ch != nil {
		// Find the specified channel
		for _, id := range channelIDs {
			info, err := b.channelPool.GetChannel(id)
			if err != nil {
				continue
			}
			if info.Channel == ch {
				negotiateChID = id
				negotiateCh = ch
				break
			}
		}
		if negotiateCh == nil {
			return nil, ErrChannelNotRegistered
		}
	} else {
		// Use first channel as default
		firstChID := channelIDs[0]
		firstChInfo, err := b.channelPool.GetChannel(firstChID)
		if err != nil {
			return nil, WrapModuleError("GetChannel", "channel", err)
		}
		negotiateChID = firstChID
		negotiateCh = firstChInfo.Channel
	}

	// Mark as connected
	b.connected.Store(true)

	// Create negotiation request
	codecBitmap := b.codecManager.GenerateCodecBitmap()
	channelBitmap := b.channelPool.GenerateChannelBitmap()
	if isBitmapEmpty(codecBitmap) {
		b.connected.Store(false)
		return nil, errors.New("no codecs registered")
	}
	if isBitmapEmpty(channelBitmap) {
		b.connected.Store(false)
		return nil, errors.New("no channels registered")
	}

	// First connection: sessionID is nil
	request, err := negotiate.NewNegotiateRequest(channelBitmap, codecBitmap, nil)
	if err != nil {
		b.connected.Store(false)
		return nil, WrapModuleError("NewNegotiateRequest", "negotiate", err)
	}

	// Send negotiation request through selected channel
	requestData, err := request.Encode()
	if err != nil {
		b.connected.Store(false)
		return nil, WrapModuleError("EncodeNegotiateRequest", "negotiate", err)
	}

	if err := negotiateCh.Send(requestData); err != nil {
		b.connected.Store(false)
		return nil, WrapModuleError("SendNegotiateRequest", "channel", err)
	}

	// Receive negotiation response
	responseData, err := negotiateCh.Receive()
	if err != nil {
		b.connected.Store(false)
		return nil, WrapModuleError("ReceiveNegotiateResponse", "channel", err)
	}

	response, err := negotiate.DecodeNegotiateResponse(responseData)
	if err != nil {
		b.connected.Store(false)
		return nil, WrapModuleError("DecodeNegotiateResponse", "negotiate", err)
	}

	// Apply negotiation response
	if err := b.codecManager.SetNegotiatedBitmap(response.CodecBitmap); err != nil {
		b.connected.Store(false)
		return nil, WrapModuleError("SetNegotiatedBitmap", "codec", err)
	}
	b.channelPool.SetNegotiatedChannelBitmap(response.ChannelBitmap)
	b.negotiated.Store(true)

	// Negotiate remaining channels with SessionID in parallel
	// Note: Each negotiateChannel will start its own receiveLoop after negotiation completes
	sessionID := response.SessionID
	for _, chID := range channelIDs {
		if chID == negotiateChID {
			continue // Skip the channel used for initial negotiation
		}
		chInfo, err := b.channelPool.GetChannel(chID)
		if err != nil {
			continue
		}
		b.wg.Add(1)
		go b.negotiateChannelAndStartReceive(chID, chInfo.Channel, sessionID, channelBitmap, codecBitmap)
	}

	// Start receive loop only for the initial negotiation channel
	b.running.Store(true)
	go b.nakBatchLoop()
	info, err := b.channelPool.GetChannel(negotiateChID)
	if err == nil {
		b.wg.Add(1)
		go b.receiveLoop(info)
	}

	// Create receive channel for complete messages
	recvChan := make(chan []byte, 100)
	go b.bridgeReceiveToChan(recvChan)

	// Create and return VoidBusConn
	conn := newVoidBusConn(b, negotiateChID, negotiateCh.Type(), recvChan)
	return conn, nil
}

// negotiateChannel negotiates a single channel with existing SessionID.
// Called in background for additional channels.
// If negotiation fails, the channel is removed from the pool.
func (b *Bus) negotiateChannel(channelID string, ch channel.Channel, sessionID, channelBitmap, codecBitmap []byte) bool {
	// Create negotiation request with SessionID
	request, err := negotiate.NewNegotiateRequest(channelBitmap, codecBitmap, sessionID)
	if err != nil {
		if b.config.DebugMode {
			println("[DEBUG] negotiateChannel: failed to create request:", err.Error())
		}
		b.channelPool.RemoveChannel(channelID)
		ch.Close()
		return false
	}

	// Send negotiation request
	requestData, err := request.Encode()
	if err != nil {
		if b.config.DebugMode {
			println("[DEBUG] negotiateChannel: failed to encode request:", err.Error())
		}
		b.channelPool.RemoveChannel(channelID)
		ch.Close()
		return false
	}

	if err := ch.Send(requestData); err != nil {
		if b.config.DebugMode {
			println("[DEBUG] negotiateChannel: failed to send request:", err.Error())
		}
		b.channelPool.RemoveChannel(channelID)
		ch.Close()
		return false
	}

	// Receive negotiation response
	responseData, err := ch.Receive()
	if err != nil {
		if b.config.DebugMode {
			println("[DEBUG] negotiateChannel: failed to receive response:", err.Error())
		}
		b.channelPool.RemoveChannel(channelID)
		ch.Close()
		return false
	}

	response, err := negotiate.DecodeNegotiateResponse(responseData)
	if err != nil {
		if b.config.DebugMode {
			println("[DEBUG] negotiateChannel: failed to decode response:", err.Error())
		}
		b.channelPool.RemoveChannel(channelID)
		ch.Close()
		return false
	}

	if response.Status != negotiate.NegotiateStatusSuccess {
		if b.config.DebugMode {
			println("[DEBUG] negotiateChannel: negotiation failed, status:", response.Status)
		}
		b.channelPool.RemoveChannel(channelID)
		ch.Close()
		return false
	}

	if b.config.DebugMode {
		println("[DEBUG] negotiateChannel: channel", channelID, "negotiated successfully")
	}
	return true
}

// negotiateChannelAndStartReceive negotiates a single channel with existing SessionID,
// then starts its receive loop if successful. Called in background for additional channels.
func (b *Bus) negotiateChannelAndStartReceive(channelID string, ch channel.Channel, sessionID, channelBitmap, codecBitmap []byte) {
	defer b.wg.Done()

	// First negotiate the channel
	success := b.negotiateChannel(channelID, ch, sessionID, channelBitmap, codecBitmap)

	// Only start receive loop if negotiation was successful
	if success && b.running.Load() {
		info, err := b.channelPool.GetChannel(channelID)
		if err == nil {
			b.wg.Add(1)
			go b.receiveLoop(info)
		}
	}
}

// Listen starts a server listener.
// Returns net.Listener for accepting client connections.
//
// Listen aggregates all registered ServerChannels and waits for multi-channel sessions.
// Each Accept returns a net.Conn when all negotiated channels are connected.
func (b *Bus) Listen() (net.Listener, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if any server channels are registered
	if len(b.serverChannels) == 0 {
		return nil, errors.New("no server channels registered")
	}

	// Mark as running
	b.running.Store(true)
	b.connected.Store(true)

	// Create listener with session timeout from config
	sessionTimeout := b.config.NegotiateTimeout
	if sessionTimeout <= 0 {
		sessionTimeout = 30 * time.Second
	}

	listener := newVoidBusListener(b, sessionTimeout)
	return listener, nil
}

// === Internal Methods ===

// sendInternal sends data through the bus (used by VoidBusConn.Write).
// Deprecated: Use sendInternalWithInfo for detailed send information.
func (b *Bus) sendInternal(ctx context.Context, data []byte) error {
	_, err := b.sendInternalWithInfo(ctx, data)
	return err
}

// sendInternalWithInfo sends data and returns detailed send information.
// Returns SendInfo with channels used, codec chain, fragment count, and data size.
func (b *Bus) sendInternalWithInfo(ctx context.Context, data []byte) (*SendInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.connected.Load() || !b.negotiated.Load() {
		return nil, ErrChannelNotReady
	}

	if b.config.DebugMode {
		println("[DEBUG] sendInternal: starting, data size:", len(data))
	}

	// Acquire semaphore
	select {
	case b.sendSemaphore <- struct{}{}:
		defer func() { <-b.sendSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.stopChan:
		return nil, ErrBusNotRunning
	}

	// Select codec chain
	chain, codecHash, err := b.codecManager.SelectChain()
	if err != nil {
		return nil, WrapModuleError("SelectChain", "codec", err)
	}

	// Get codec chain codes for SendInfo
	codecCodes := b.codecManager.GetChainCodes(codecHash)

	if b.config.DebugMode {
		fmt.Printf("[DEBUG] sendInternal: selected codec chain, hash: %x, depth: %d, codes: %v\n", codecHash[:4], chain.Length(), codecCodes)
	}

	// Compute data hash
	dataHash := internal.ComputeDataHash(data)

	// Get codec depth for header
	codecDepth := uint8(chain.Length())
	if codecDepth < 1 {
		codecDepth = 1 // Minimum required by protocol
	}

	// Create session
	sess := b.sessionMgr.CreateSendSession(nil, codecHash, 0, dataHash)

	// Encode data
	encodedData, err := chain.Encode(data)
	if err != nil {
		b.sessionMgr.RemoveSendSession(sess.ID)
		return nil, WrapModuleError("Encode", "codec", err)
	}

	if b.config.DebugMode {
		println("[DEBUG] sendInternal: encoded data size:", len(encodedData))
	}

	// Get adaptive MTU
	mtu := b.channelPool.GetAdaptiveMTU()
	if mtu == 0 {
		mtu = b.config.DefaultMTU
	}

	// Adaptive split
	fragments, checksums, err := b.fragmentMgr.AdaptiveSplit(encodedData, mtu)
	if err != nil {
		b.sessionMgr.RemoveSendSession(sess.ID)
		return nil, WrapModuleError("AdaptiveSplit", "fragment", err)
	}

	if b.config.DebugMode {
		println("[DEBUG] sendInternal: split into", len(fragments), "fragments")
	}

	// Create send buffer
	sendBuf := b.fragmentMgr.CreateSendBuffer(sess.ID, data)
	sendBuf.SetCodecInfo(nil, codecHash)
	sendBuf.SetEncodedData(encodedData)
	sess.SetTotalFragments(uint16(len(fragments)))

	// Parallel fragment distribution
	errChan := make(chan error, len(fragments))
	var sendWg sync.WaitGroup

	for i, fragData := range fragments {
		sendWg.Add(1)
		go func(index int, fragment []byte, checksum uint32) {
			defer sendWg.Done()

			chInfo, err := b.channelPool.SelectChannel(nil)
			if err != nil || chInfo == nil {
				if b.config.DebugMode {
					println("[DEBUG] sendInternal: SelectChannel failed:", err)
				}
				errChan <- ErrNoHealthyChannel
				return
			}

			if b.config.DebugMode {
				println("[DEBUG] sendInternal: sending fragment", index, "on channel", chInfo.ID)
			}

			header := &protocol.Header{
				SessionID:     sess.ID,
				FragmentIndex: uint16(index),
				FragmentTotal: uint16(len(fragments)),
				CodecDepth:    codecDepth,
				CodecHash:     codecHash,
				DataChecksum:  checksum,
				DataHash:      dataHash,
				Timestamp:     time.Now().Unix(),
			}
			if index == len(fragments)-1 {
				header.Flags |= protocol.FlagIsLast
			}

			packet := header.Encode(fragment)
			if err := chInfo.Channel.Send(packet); err != nil {
				b.channelPool.RecordError(chInfo.ID)
				errChan <- err
				return
			}

			if b.config.DebugMode {
				println("[DEBUG] sendInternal: sent fragment", index, "size:", len(packet))
			}

			b.channelPool.RecordSend(chInfo.ID, time.Millisecond)
			sendBuf.AddFragment(uint16(index), fragment, checksum, chInfo.ID)
			sess.IncrementSent()
		}(i, fragData, checksums[i])
	}

	sendWg.Wait()

	// Check for any send errors
	select {
	case err := <-errChan:
		b.sessionMgr.RemoveSendSession(sess.ID)
		return nil, WrapModuleError("SendFragments", "channel", err)
	default:
		// All fragments sent successfully
	}

	b.adaptiveTimer.AddMeasurement(time.Since(sess.CreatedAt))

	// Only return SendInfo in debug mode (reduces overhead in production)
	if b.config.DebugMode {
		// Collect channel IDs used for sending
		channelIDs := sendBuf.GetFragmentChannelIDs()

		return &SendInfo{
			Channels:    channelIDs,
			CodecChain:  codecCodes,
			CodecHash:   codecHash,
			FragmentCnt: len(fragments),
			DataSize:    len(data),
		}, nil
	}

	return nil, nil
}

// bridgeReceiveToChan bridges bus receive queue to conn recvChan.
func (b *Bus) bridgeReceiveToChan(recvChan chan []byte) {
	for {
		select {
		case <-b.stopChan:
			close(recvChan)
			return
		case data, ok := <-b.recvQueue:
			if !ok {
				close(recvChan)
				return
			}
			select {
			case recvChan <- data:
			default:
			}
		}
	}
}

// receiveLoop handles receiving from a channel.
// It exits when stopChan is closed or when the underlying channel is closed
// (which causes Receive() to return an error).
func (b *Bus) receiveLoop(info *channel.ChannelInfo) {
	defer b.wg.Done()

	for {
		select {
		case <-b.stopChan:
			return
		default:
		}

		data, err := info.Channel.Receive()
		if err != nil {
			// Check if bus is stopping — if so, exit cleanly.
			select {
			case <-b.stopChan:
				return
			default:
			}
			// Channel error while bus is still running — record and retry.
			b.channelPool.RecordError(info.ID)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Debug: print received raw data size
		if b.config.DebugMode {
			println("[DEBUG] receiveLoop: received", len(data), "bytes from channel", info.ID)
		}

		b.channelPool.RecordSend(info.ID, time.Millisecond)
		if err := b.processReceivedPacket(data, info); err != nil {
			// Log error internally
			if b.config.DebugMode {
				println("[DEBUG] processReceivedPacket error:", err.Error())
			}
		}
	}
}

// processReceivedPacket processes a received packet.
func (b *Bus) processReceivedPacket(data []byte, info *channel.ChannelInfo) error {
	header, fragmentData, err := protocol.DecodeHeader(data)
	if err != nil {
		return WrapModuleError("DecodeHeader", "protocol", err)
	}

	if header.IsNAK() {
		return b.handleNAK(header, fragmentData)
	}
	if header.IsEND_ACK() {
		return b.handleEND_ACK(header)
	}
	return b.handleFragment(header, fragmentData, info)
}

// handleFragment handles a regular data fragment.
func (b *Bus) handleFragment(header *protocol.Header, fragmentData []byte, info *channel.ChannelInfo) error {
	buf, err := b.fragmentMgr.GetRecvBuffer(header.SessionID)
	if err != nil {
		buf = b.fragmentMgr.CreateRecvBuffer(
			header.SessionID,
			header.FragmentTotal,
			header.CodecDepth,
			header.CodecHash,
			header.DataHash,
		)
		b.sessionMgr.CreateRecvSession(
			header.SessionID,
			nil,
			header.CodecHash,
			int(header.CodecDepth),
			header.DataHash,
		)
	}

	if !buf.AddFragment(header.FragmentIndex, fragmentData, header.DataChecksum) {
		return nil
	}

	if !buf.IsComplete() {
		missing := buf.GetMissing()
		if len(missing) > 0 {
			b.queueNAK(header.SessionID, missing)
		}
		return nil
	}

	encodedData, err := buf.Reassemble()
	if err != nil {
		return WrapModuleError("Reassemble", "fragment", err)
	}

	chain, err := b.codecManager.MatchChain(header.CodecHash)
	if err != nil {
		return WrapModuleError("MatchChain", "codec", err)
	}

	decodedData, err := chain.Decode(encodedData)
	if err != nil {
		return WrapModuleError("Decode", "codec", err)
	}

	if !internal.VerifyDataHash(decodedData, header.DataHash) {
		return ErrFragmentCorrupted
	}

	b.sendEND_ACK(header.SessionID)
	b.sessionMgr.CompleteRecvSession(header.SessionID)
	b.fragmentMgr.RemoveRecvBuffer(header.SessionID)

	select {
	case b.recvQueue <- decodedData:
	default:
	}
	return nil
}

// handleNAK handles a NAK message.
func (b *Bus) handleNAK(header *protocol.Header, extraData []byte) error {
	missing := decodeNAKIndices(extraData)
	buf, err := b.fragmentMgr.GetSendBuffer(header.SessionID)
	if err != nil {
		return err
	}
	sess, err := b.sessionMgr.GetSendSession(header.SessionID)
	if err != nil {
		return err
	}
	if !sess.IncrementRetransmit() {
		sess.MarkExpired()
		b.fragmentMgr.RemoveSendBuffer(header.SessionID)
		return ErrRetransmitExceeded
	}

	fragments := buf.GetMissingFragments(missing)
	var retransmitWg sync.WaitGroup
	for _, frag := range fragments {
		retransmitWg.Add(1)
		go func(f *fragment.FragmentEntry) {
			defer retransmitWg.Done()
			chInfo, err := b.channelPool.SelectChannel(nil)
			if err != nil {
				return
			}
			_, totalFragments := sess.GetProgress()
			retransmitHeader := &protocol.Header{
				SessionID:     header.SessionID,
				FragmentIndex: f.Index,
				FragmentTotal: totalFragments,
				CodecDepth:    1, // Minimum required
				CodecHash:     buf.CodecHash,
				DataChecksum:  f.Checksum,
				DataHash:      buf.DataHash,
				Flags:         protocol.FlagRetransmit,
				Timestamp:     time.Now().Unix(),
			}
			packet := retransmitHeader.Encode(f.Data)
			chInfo.Channel.Send(packet)
			b.channelPool.RecordSend(chInfo.ID, time.Millisecond)
		}(frag)
	}
	retransmitWg.Wait()
	return nil
}

// handleEND_ACK handles an END_ACK message.
func (b *Bus) handleEND_ACK(header *protocol.Header) error {
	b.sessionMgr.CompleteSendSession(header.SessionID)
	b.fragmentMgr.RemoveSendBuffer(header.SessionID)
	return nil
}

// queueNAK queues NAK request for batch processing.
func (b *Bus) queueNAK(sessionID string, missing []uint16) {
	b.nakQueueMu.Lock()
	defer b.nakQueueMu.Unlock()
	existing := b.nakQueue[sessionID]
	b.nakQueue[sessionID] = append(existing, missing...)
	if len(b.nakQueue[sessionID]) > b.nakBatchSize {
		b.nakQueue[sessionID] = b.nakQueue[sessionID][:b.nakBatchSize]
	}
}

// nakBatchLoop periodically sends batched NAK requests.
func (b *Bus) nakBatchLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
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
		b.sendNAK(sessionID, missing)
	}
}

// sendNAK sends a NAK message for missing fragments.
func (b *Bus) sendNAK(sessionID string, missing []uint16) error {
	chInfo, err := b.channelPool.SelectChannel(nil)
	if err != nil {
		return ErrNoHealthyChannel
	}
	header := &protocol.Header{
		SessionID:  sessionID,
		Flags:      protocol.FlagIsNAK,
		CodecDepth: 1, // Minimum required
		Timestamp:  time.Now().Unix(),
	}
	extra := encodeNAKIndices(missing)
	packet := header.Encode(extra)
	return chInfo.Channel.Send(packet)
}

// sendEND_ACK sends an END_ACK message.
func (b *Bus) sendEND_ACK(sessionID string) error {
	chInfo, err := b.channelPool.SelectChannel(nil)
	if err != nil {
		return ErrNoHealthyChannel
	}
	header := &protocol.Header{
		SessionID:  sessionID,
		Flags:      protocol.FlagIsENDACK,
		CodecDepth: 1, // Minimum required
		Timestamp:  time.Now().Unix(),
	}
	packet := header.Encode(nil)
	return chInfo.Channel.Send(packet)
}

// Stop stops the bus and all goroutines.
// It closes all channels first to unblock any goroutines waiting on Receive(),
// then waits for all goroutines to exit cleanly.
func (b *Bus) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running.Load() {
		return ErrBusNotRunning
	}

	b.running.Store(false)
	close(b.stopChan)

	// Close all channels to unblock receiveLoop goroutines waiting on Receive().
	// Without this, receiveLoop blocks forever on channel.Receive() and wg.Wait() deadlocks.
	b.channelPool.CloseAll()

	b.wg.Wait()
	b.fragmentMgr.Stop()
	b.sessionMgr.Stop()
	return nil
}

// SetDebugMode enables or disables debug output.
func (b *Bus) SetDebugMode(enable bool) {
	b.config.DebugMode = enable
}

// === Helper Functions ===

func isBitmapEmpty(bitmap []byte) bool {
	for _, b := range bitmap {
		if b != 0 {
			return false
		}
	}
	return true
}

func encodeNAKIndices(indices []uint16) []byte {
	result := make([]byte, len(indices)*2)
	for i, idx := range indices {
		result[i*2] = byte(idx >> 8)
		result[i*2+1] = byte(idx)
	}
	return result
}

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
