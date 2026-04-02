// Package udp provides a UDP channel implementation with reliability support.
//
// UDP channel uses UDP protocol for communication.
// Since UDP is unreliable, this implementation provides:
// - ACK/NAK mechanism for reliability
// - Sequence number tracking
// - Retransmission on timeout
// - Maximum 3 retry attempts
//
// Frame format:
// [1 byte: FrameType] [4 bytes: SeqNum] [N bytes: Payload]
//
// Design Constraints:
// - UDP channel MUST NOT handle serialization/encoding/fragmentation
// - UDP channel MUST NOT be exposed in metadata protocols
// - Reliability is implemented at channel level (ACK/NAK)
package udp

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/internal"
)

const (
	// ChannelType is the type identifier for UDP channels
	ChannelType = channel.TypeUDP
	// DefaultBufferSize is the default buffer size
	DefaultBufferSize = 4096
	// MaxUDPPacketSize is the maximum UDP packet size (64KB theoretical, practical ~1400)
	MaxUDPPacketSize = 1400
	// MaxRetries is the maximum retry count
	MaxRetries = 3
	// DefaultAckTimeout is the default ACK timeout (3 seconds)
	DefaultAckTimeout = 3 * time.Second
	// HeaderSize is the frame header size
	HeaderSize = 5 // 1 (type) + 4 (seq)
)

// FrameType defines UDP frame types.
type FrameType byte

const (
	FrameData  FrameType = 0x01 // Data frame
	FrameAck   FrameType = 0x02 // Acknowledgment
	FrameNak   FrameType = 0x03 // Negative acknowledgment
	FrameProbe FrameType = 0x04 // Heartbeat probe
)

// ClientChannel implements channel.Channel for UDP client.
type ClientChannel struct {
	conn         *net.UDPConn
	remoteAddr   *net.UDPAddr
	config       channel.ChannelConfig
	mu           sync.RWMutex
	closed       bool
	connected    bool
	id           string
	lastActivity time.Time

	// Reliability manager
	reliability *ReliabilityManager
}

// ReliabilityManager manages UDP reliability.
type ReliabilityManager struct {
	mu         sync.RWMutex
	sendWindow map[uint32]*PendingFrame // SeqNum -> Frame
	nextSeqNum uint32
	ackTimeout time.Duration
	maxRetries int
}

// PendingFrame represents a frame waiting for ACK.
type PendingFrame struct {
	Data       []byte
	SendTime   time.Time
	RetryCount int
}

// NewReliabilityManager creates a new reliability manager.
func NewReliabilityManager(ackTimeout time.Duration, maxRetries int) *ReliabilityManager {
	return &ReliabilityManager{
		sendWindow: make(map[uint32]*PendingFrame),
		nextSeqNum: 0,
		ackTimeout: ackTimeout,
		maxRetries: maxRetries,
	}
}

// NextSeqNum returns the next sequence number.
func (r *ReliabilityManager) NextSeqNum() uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	seq := r.nextSeqNum
	r.nextSeqNum++
	return seq
}

// AddPending adds a frame to pending window.
func (r *ReliabilityManager) AddPending(seq uint32, data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sendWindow[seq] = &PendingFrame{
		Data:       data,
		SendTime:   time.Now(),
		RetryCount: 0,
	}
}

// AckPending removes a frame from pending window (ACK received).
func (r *ReliabilityManager) AckPending(seq uint32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sendWindow[seq]; exists {
		delete(r.sendWindow, seq)
		return true
	}
	return false
}

// GetTimeoutFrames returns frames that need retry.
func (r *ReliabilityManager) GetTimeoutFrames() []*PendingFrame {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	var timeout []*PendingFrame
	for _, frame := range r.sendWindow {
		if now.Sub(frame.SendTime) > r.ackTimeout && frame.RetryCount < r.maxRetries {
			timeout = append(timeout, frame)
		}
	}
	return timeout
}

// MarkRetried increments retry count.
func (r *ReliabilityManager) MarkRetried(seq uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if frame, exists := r.sendWindow[seq]; exists {
		frame.RetryCount++
		frame.SendTime = time.Now()
	}
}

// RemovePending removes a pending frame.
func (r *ReliabilityManager) RemovePending(seq uint32) {
	r.mu.Lock()
	delete(r.sendWindow, seq)
	r.mu.Unlock()
}

// PendingCount returns number of pending frames.
func (r *ReliabilityManager) PendingCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sendWindow)
}

// NewClientChannel creates a new UDP client channel.
func NewClientChannel(config channel.ChannelConfig) (*ClientChannel, error) {
	if config.Address == "" {
		return nil, errors.New("udp: address required")
	}

	// Parse remote address
	remoteAddr, err := net.ResolveUDPAddr("udp", config.Address)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "resolve",
			Err:       err,
			Msg:       "failed to resolve address " + config.Address,
			Retryable: false,
		}
	}

	// Create local connection (random port)
	localAddr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", localAddr, remoteAddr)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "connect",
			Err:       err,
			Msg:       "failed to connect to " + config.Address,
			Retryable: true,
		}
	}

	ackTimeout := DefaultAckTimeout
	if config.Timeout > 0 {
		ackTimeout = config.Timeout
	}

	ch := &ClientChannel{
		conn:        conn,
		remoteAddr:  remoteAddr,
		config:      config,
		connected:   true,
		id:          internal.GenerateID(),
		reliability: NewReliabilityManager(ackTimeout, MaxRetries),
	}

	return ch, nil
}

// Send implements channel.Channel.Send.
// Sends data with reliability mechanism.
func (c *ClientChannel) Send(data []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return channel.ErrChannelClosed
	}
	if !c.connected {
		return channel.ErrChannelDisconnected
	}

	if len(data) > MaxUDPPacketSize-HeaderSize {
		return errors.New("udp: data exceeds max packet size")
	}

	// Get sequence number
	seq := c.reliability.NextSeqNum()

	// Build frame: [FrameType][SeqNum][Data]
	frame := make([]byte, HeaderSize+len(data))
	frame[0] = byte(FrameData)
	binary.BigEndian.PutUint32(frame[1:5], seq)
	copy(frame[5:], data)

	// Add to pending window
	c.reliability.AddPending(seq, frame)

	// Send
	if _, err := c.conn.Write(frame); err != nil {
		c.reliability.RemovePending(seq)
		c.handleDisconnect()
		return &channel.ChannelError{
			Op:        "send",
			Err:       err,
			Msg:       "failed to send",
			Retryable: true,
		}
	}

	c.lastActivity = time.Now()
	return nil
}

// Receive implements channel.Channel.Receive.
// Loops internally to skip ACK/NAK frames and return only data frames.
func (c *ClientChannel) Receive() ([]byte, error) {
	for {
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

		buf := make([]byte, MaxUDPPacketSize)
		n, err := conn.Read(buf)
		if err != nil {
			c.handleDisconnect()
			return nil, &channel.ChannelError{
				Op:        "receive",
				Err:       err,
				Msg:       "failed to receive",
				Retryable: false,
			}
		}

		if n < HeaderSize {
			return nil, errors.New("udp: invalid frame size")
		}

		frameType := FrameType(buf[0])
		seq := binary.BigEndian.Uint32(buf[1:5])
		data := buf[5:n]

		// Handle frame type
		switch frameType {
		case FrameAck:
			// ACK received, remove from pending, continue loop
			c.reliability.AckPending(seq)
			continue // Skip ACK, wait for next packet

		case FrameNak:
			// NAK received, should retry (handled by retry goroutine), continue loop
			c.reliability.MarkRetried(seq)
			continue // Skip NAK, wait for next packet

		case FrameData:
			// Data received, send ACK and return data
			c.sendAck(seq)
			return data, nil

		case FrameProbe:
			// Heartbeat, ignore and continue
			continue

		default:
			return nil, errors.New("udp: unknown frame type")
		}
	}
}

// sendAck sends an ACK frame.
func (c *ClientChannel) sendAck(seq uint32) error {
	frame := make([]byte, HeaderSize)
	frame[0] = byte(FrameAck)
	binary.BigEndian.PutUint32(frame[1:5], seq)
	_, err := c.conn.Write(frame)
	return err
}

// sendNak sends a NAK frame.
func (c *ClientChannel) sendNak(seq uint32) error {
	frame := make([]byte, HeaderSize)
	frame[0] = byte(FrameNak)
	binary.BigEndian.PutUint32(frame[1:5], seq)
	_, err := c.conn.Write(frame)
	return err
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
// UDP MTU is typically 1472 bytes (1500 - 20 IP - 8 UDP).
func (c *ClientChannel) DefaultMTU() int {
	return 1472
}

// IsReliable implements channel.Channel.IsReliable.
// UDP is unreliable, requires ACK/NAK.
func (c *ClientChannel) IsReliable() bool {
	return false
}

// AckTimeout implements channel.Channel.AckTimeout.
func (c *ClientChannel) AckTimeout() time.Duration {
	return DefaultAckTimeout
}

// handleDisconnect handles disconnection.
func (c *ClientChannel) handleDisconnect() {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
}

// udpPacket represents a received UDP packet with sender address.
type udpPacket struct {
	data       []byte
	remoteAddr *net.UDPAddr
}

// ServerChannel implements channel.ServerChannel for UDP server.
// Uses a single dispatcher goroutine to read from the shared UDP socket
// and route packets to per-client queues, eliminating socket contention.
type ServerChannel struct {
	conn   *net.UDPConn
	config channel.ChannelConfig
	mu     sync.RWMutex
	closed bool

	// Client management
	clients map[string]*AcceptedChannel // id -> client

	// Dispatcher: routes packets from shared socket to per-client queues
	clientsByAddr map[string]*AcceptedChannel // "ip:port" -> client
	addrMu        sync.RWMutex                // protects clientsByAddr
	newClientCh   chan *udpPacket             // first packet from unknown client → Accept()
	dispatchOnce  sync.Once                   // ensures dispatcher starts only once
}

// NewServerChannel creates a new UDP server channel.
//
// The server uses a dispatcher pattern:
// - Single goroutine reads from the shared UDP socket
// - Packets are routed to per-client queues by remote address
// - New (unknown) clients are forwarded to Accept() via newClientCh
func NewServerChannel(config channel.ChannelConfig) (*ServerChannel, error) {
	if config.Address == "" {
		return nil, errors.New("udp: address required")
	}

	addr, err := net.ResolveUDPAddr("udp", config.Address)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "resolve",
			Err:       err,
			Msg:       "failed to resolve address",
			Retryable: false,
		}
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "listen",
			Err:       err,
			Msg:       "failed to listen on " + config.Address,
			Retryable: false,
		}
	}

	return &ServerChannel{
		conn:          conn,
		config:        config,
		clients:       make(map[string]*AcceptedChannel),
		clientsByAddr: make(map[string]*AcceptedChannel),
		newClientCh:   make(chan *udpPacket, 16),
	}, nil
}

// startDispatcher starts the single packet dispatcher goroutine.
// It reads all packets from the shared UDP socket and routes them:
// - Known client address → client's recvQueue channel
// - Unknown address → newClientCh for Accept() to pick up
func (s *ServerChannel) startDispatcher() {
	go func() {
		buf := make([]byte, MaxUDPPacketSize)
		for {
			n, remoteAddr, err := s.conn.ReadFromUDP(buf)
			if err != nil {
				s.mu.RLock()
				closed := s.closed
				s.mu.RUnlock()
				if closed {
					return // Server closed, stop dispatcher
				}
				continue // Transient error, keep reading
			}

			if n == 0 {
				continue
			}

			// Make a copy of the packet data (buf is reused)
			pktData := make([]byte, n)
			copy(pktData, buf[:n])

			pkt := &udpPacket{
				data:       pktData,
				remoteAddr: remoteAddr,
			}

			// Look up client by address
			addrKey := remoteAddr.String()
			s.addrMu.RLock()
			client, exists := s.clientsByAddr[addrKey]
			s.addrMu.RUnlock()

			if exists {
				// Known client: route to its queue (non-blocking)
				// Check if client is still open to avoid send-on-closed-channel panic
				client.mu.RLock()
				closed := client.closed
				client.mu.RUnlock()
				if !closed {
					select {
					case client.recvQueue <- pkt:
					default:
						// Queue full, drop packet (UDP semantics allow this)
					}
				}
			} else {
				// Unknown client: forward to Accept()
				select {
				case s.newClientCh <- pkt:
				default:
					// Accept backpressure, drop packet
				}
			}
		}
	}()
}

// Accept implements channel.ServerChannel.Accept.
// Blocks until a packet arrives from a new (unknown) client address.
// Uses the dispatcher to receive first packets from new clients.
func (s *ServerChannel) Accept() (channel.Channel, error) {
	// Start dispatcher on first Accept() call
	s.dispatchOnce.Do(s.startDispatcher)

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, channel.ErrChannelClosed
	}
	s.mu.RUnlock()

	// Wait for a packet from a new client
	pkt, ok := <-s.newClientCh
	if !ok {
		return nil, channel.ErrChannelClosed
	}

	ackTimeout := DefaultAckTimeout
	if s.config.Timeout > 0 {
		ackTimeout = s.config.Timeout
	}

	// Create accepted channel with its own ReliabilityManager and receive queue
	client := &AcceptedChannel{
		sendConn:     s.conn,
		remoteAddr:   pkt.remoteAddr,
		id:           internal.GenerateID(),
		lastActivity: time.Now(),
		connected:    true,
		reliability:  NewReliabilityManager(ackTimeout, MaxRetries),
		recvQueue:    make(chan *udpPacket, 256),
	}

	// Register client by address for dispatcher routing
	addrKey := pkt.remoteAddr.String()
	s.addrMu.Lock()
	s.clientsByAddr[addrKey] = client
	s.addrMu.Unlock()

	s.mu.Lock()
	s.clients[client.id] = client
	s.mu.Unlock()

	// Process first packet: extract data or handle control frame
	if len(pkt.data) >= HeaderSize {
		frameType := FrameType(pkt.data[0])
		seq := binary.BigEndian.Uint32(pkt.data[1:5])
		if frameType == FrameData {
			client.sendAck(seq)
			// Store payload for first Receive() call
			client.firstData = make([]byte, len(pkt.data)-HeaderSize)
			copy(client.firstData, pkt.data[5:])
		}
	}

	return client, nil
}

// Send implements channel.Channel.Send.
func (s *ServerChannel) Send(data []byte) error {
	return errors.New("udp: server cannot send directly")
}

// Receive implements channel.Channel.Receive.
func (s *ServerChannel) Receive() ([]byte, error) {
	return nil, errors.New("udp: server cannot receive directly")
}

// Close implements channel.Channel.Close.
func (s *ServerChannel) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return channel.ErrChannelClosed
	}

	s.closed = true

	// Close newClientCh to unblock Accept()
	close(s.newClientCh)

	// Close all client channels
	for _, client := range s.clients {
		client.Close()
	}

	// Clear address routing table
	s.addrMu.Lock()
	s.clientsByAddr = make(map[string]*AcceptedChannel)
	s.addrMu.Unlock()

	if s.conn != nil {
		return s.conn.Close()
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
	return false
}

// AckTimeout implements channel.Channel.AckTimeout.
func (s *ServerChannel) AckTimeout() time.Duration {
	return DefaultAckTimeout
}

// ListenAddress implements channel.ServerChannel.ListenAddress.
func (s *ServerChannel) ListenAddress() string {
	return s.config.Address
}

// AcceptedChannel represents an accepted UDP client connection.
// Each AcceptedChannel has:
// - Its own ReliabilityManager (independent sequence numbers)
// - A dedicated recvQueue (fed by the server's dispatcher goroutine)
// - A reference to the shared sendConn (UDP write is thread-safe)
type AcceptedChannel struct {
	sendConn     *net.UDPConn // shared server socket, used only for WriteToUDP (thread-safe)
	remoteAddr   *net.UDPAddr
	mu           sync.RWMutex
	closed       bool
	connected    bool
	id           string
	lastActivity time.Time
	reliability  *ReliabilityManager // per-client, NOT shared
	recvQueue    chan *udpPacket     // fed by dispatcher goroutine
	firstData    []byte              // first packet data (from Accept)
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

	if len(data) > MaxUDPPacketSize-HeaderSize {
		return errors.New("udp: data exceeds max packet size")
	}

	seq := a.reliability.NextSeqNum()
	frame := make([]byte, HeaderSize+len(data))
	frame[0] = byte(FrameData)
	binary.BigEndian.PutUint32(frame[1:5], seq)
	copy(frame[5:], data)

	a.reliability.AddPending(seq, frame)

	if _, err := a.sendConn.WriteToUDP(frame, a.remoteAddr); err != nil {
		a.reliability.RemovePending(seq)
		return &channel.ChannelError{
			Op:        "send",
			Err:       err,
			Msg:       "failed to send",
			Retryable: true,
		}
	}

	a.lastActivity = time.Now()
	return nil
}

// Receive implements channel.Channel.Receive.
// Reads from the per-client recvQueue (populated by dispatcher).
// Loops internally to skip ACK/NAK/Probe frames and return only data frames.
func (a *AcceptedChannel) Receive() ([]byte, error) {
	// Return first data if available (from Accept handshake)
	if a.firstData != nil {
		data := a.firstData
		a.firstData = nil
		return data, nil
	}

	for {
		a.mu.RLock()
		if a.closed {
			a.mu.RUnlock()
			return nil, channel.ErrChannelClosed
		}
		a.mu.RUnlock()

		// Read from per-client queue (blocks until packet available or channel closed)
		pkt, ok := <-a.recvQueue
		if !ok {
			return nil, channel.ErrChannelClosed
		}

		if len(pkt.data) < HeaderSize {
			continue // Invalid frame, skip
		}

		frameType := FrameType(pkt.data[0])
		seq := binary.BigEndian.Uint32(pkt.data[1:5])
		data := pkt.data[5:]

		switch frameType {
		case FrameAck:
			a.reliability.AckPending(seq)
			continue // Skip ACK, wait for next packet
		case FrameNak:
			a.reliability.MarkRetried(seq)
			continue // Skip NAK, wait for next packet
		case FrameData:
			a.sendAck(seq)
			// Make a copy since pkt.data may be from dispatcher buffer
			result := make([]byte, len(data))
			copy(result, data)
			return result, nil
		case FrameProbe:
			continue // Skip heartbeat
		default:
			continue // Skip unknown frames
		}
	}
}

// sendAck sends ACK.
func (a *AcceptedChannel) sendAck(seq uint32) error {
	frame := make([]byte, HeaderSize)
	frame[0] = byte(FrameAck)
	binary.BigEndian.PutUint32(frame[1:5], seq)
	_, err := a.sendConn.WriteToUDP(frame, a.remoteAddr)
	return err
}

// Close implements channel.Channel.Close.
func (a *AcceptedChannel) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil // Idempotent close
	}

	a.closed = true
	a.connected = false

	// Close recvQueue to unblock Receive()
	close(a.recvQueue)
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
	return 1472
}

// IsReliable implements channel.Channel.IsReliable.
func (a *AcceptedChannel) IsReliable() bool {
	return false
}

// AckTimeout implements channel.Channel.AckTimeout.
func (a *AcceptedChannel) AckTimeout() time.Duration {
	return DefaultAckTimeout
}

// Module implements channel.ChannelModule.
type Module struct{}

// NewModule creates a new UDP module.
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
