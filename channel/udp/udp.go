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
func (c *ClientChannel) Receive() ([]byte, error) {
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
		// ACK received, remove from pending
		c.reliability.AckPending(seq)
		return nil, nil // No data to return

	case FrameNak:
		// NAK received, should retry (handled by retry goroutine)
		c.reliability.MarkRetried(seq)
		return nil, nil

	case FrameData:
		// Data received, send ACK
		c.sendAck(seq)
		return data, nil

	case FrameProbe:
		// Heartbeat, ignore
		return nil, nil

	default:
		return nil, errors.New("udp: unknown frame type")
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

// ServerChannel implements channel.ServerChannel for UDP server.
type ServerChannel struct {
	conn        *net.UDPConn
	config      channel.ChannelConfig
	mu          sync.RWMutex
	closed      bool
	clients     map[string]*AcceptedChannel
	reliability *ReliabilityManager
}

// NewServerChannel creates a new UDP server channel.
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
		conn:        conn,
		config:      config,
		clients:     make(map[string]*AcceptedChannel),
		reliability: NewReliabilityManager(DefaultAckTimeout, MaxRetries),
	}, nil
}

// Accept implements channel.ServerChannel.Accept.
func (s *ServerChannel) Accept() (channel.Channel, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, channel.ErrChannelClosed
	}
	s.mu.RUnlock()

	// Read first packet to get client address
	buf := make([]byte, MaxUDPPacketSize)
	n, remoteAddr, err := s.conn.ReadFromUDP(buf)
	if err != nil {
		return nil, &channel.ChannelError{
			Op:        "accept",
			Err:       err,
			Msg:       "failed to accept",
			Retryable: true,
		}
	}

	// Create accepted channel for this client
	client := &AcceptedChannel{
		serverConn:   s.conn,
		remoteAddr:   remoteAddr,
		id:           internal.GenerateID(),
		lastActivity: time.Now(),
		connected:    true,
		reliability:  s.reliability,
	}

	s.mu.Lock()
	s.clients[client.id] = client
	s.mu.Unlock()

	// Process first packet
	if n >= HeaderSize {
		frameType := FrameType(buf[0])
		seq := binary.BigEndian.Uint32(buf[1:5])
		if frameType == FrameData {
			client.sendAck(seq)
			// Store data for first receive
			client.firstData = buf[5:n]
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
	for _, client := range s.clients {
		client.Close()
	}

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

// AcceptedChannel represents an accepted UDP client.
type AcceptedChannel struct {
	serverConn   *net.UDPConn
	remoteAddr   *net.UDPAddr
	mu           sync.RWMutex
	closed       bool
	connected    bool
	id           string
	lastActivity time.Time
	reliability  *ReliabilityManager
	firstData    []byte // First packet data
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

	if _, err := a.serverConn.WriteToUDP(frame, a.remoteAddr); err != nil {
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
func (a *AcceptedChannel) Receive() ([]byte, error) {
	// Return first data if available
	if a.firstData != nil {
		data := a.firstData
		a.firstData = nil
		return data, nil
	}

	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()
		return nil, channel.ErrChannelClosed
	}
	a.mu.RUnlock()

	buf := make([]byte, MaxUDPPacketSize)
	n, addr, err := a.serverConn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}

	// Check if from correct client
	if !addr.IP.Equal(a.remoteAddr.IP) || addr.Port != a.remoteAddr.Port {
		return nil, errors.New("udp: packet from wrong client")
	}

	if n < HeaderSize {
		return nil, errors.New("udp: invalid frame size")
	}

	frameType := FrameType(buf[0])
	seq := binary.BigEndian.Uint32(buf[1:5])
	data := buf[5:n]

	switch frameType {
	case FrameAck:
		a.reliability.AckPending(seq)
		return nil, nil
	case FrameNak:
		a.reliability.MarkRetried(seq)
		return nil, nil
	case FrameData:
		a.sendAck(seq)
		return data, nil
	default:
		return nil, nil
	}
}

// sendAck sends ACK.
func (a *AcceptedChannel) sendAck(seq uint32) error {
	frame := make([]byte, HeaderSize)
	frame[0] = byte(FrameAck)
	binary.BigEndian.PutUint32(frame[1:5], seq)
	_, err := a.serverConn.WriteToUDP(frame, a.remoteAddr)
	return err
}

// Close implements channel.Channel.Close.
func (a *AcceptedChannel) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.closed = true
	a.connected = false
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
