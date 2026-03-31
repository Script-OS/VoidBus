// Package protocol provides transport layer for VoidBus communication.
//
// TransportSender handles the complete sending flow:
//
//	RawData -> Serialize -> CodecChain.Encode -> Fragment.Split -> Packet.Wrap -> Channel.Send
//
// TransportReceiver handles the complete receiving flow:
//
//	Channel.Receive -> Packet.Decode -> Fragment.Reassemble -> CodecChain.Decode -> Deserialize -> RawData
//
// Design Constraints:
//   - Transport coordinates all modules but does NOT own them
//   - Transport is stateless (state managed by Session)
//   - Transport errors are propagated to caller
package protocol

import (
	"errors"
	"sync"

	"github.com/Script-OS/VoidBus/fragment"
	"github.com/Script-OS/VoidBus/internal"
)

// Transport errors
var (
	ErrTransportNotReady      = errors.New("transport: not ready")
	ErrTransportSendFailed    = errors.New("transport: send failed")
	ErrTransportReceiveFailed = errors.New("transport: receive failed")
	ErrFragmentIncomplete     = errors.New("transport: fragment incomplete")
	ErrSessionRequired        = errors.New("transport: session required")
)

// TransportConfig provides configuration for transport.
type TransportConfig struct {
	// EnableFragment enables data fragmentation
	EnableFragment bool

	// MaxFragmentSize is maximum fragment size in bytes
	MaxFragmentSize int

	// EnableChecksum enables checksum verification
	EnableChecksum bool

	// FragmentTimeout is fragment reassembly timeout in seconds
	FragmentTimeout int

	// SendQueueSize is send queue buffer size
	SendQueueSize int

	// ReceiveQueueSize is receive queue buffer size
	ReceiveQueueSize int
}

// DefaultTransportConfig returns default transport configuration.
func DefaultTransportConfig() TransportConfig {
	return TransportConfig{
		EnableFragment:   true,
		MaxFragmentSize:  1024,
		EnableChecksum:   true,
		FragmentTimeout:  60,
		SendQueueSize:    100,
		ReceiveQueueSize: 100,
	}
}

// TransportSender handles the sending flow.
type TransportSender struct {
	mu     sync.RWMutex
	config TransportConfig
}

// NewTransportSender creates a new TransportSender.
func NewTransportSender(config TransportConfig) *TransportSender {
	return &TransportSender{
		config: config,
	}
}

// PrepareData prepares data for sending through the transport flow.
// This is the synchronous preparation step before actual channel send.
//
// Flow: data -> serialize -> encode -> fragment -> wrap
//
// Returns list of packets ready for channel transmission.
func (s *TransportSender) PrepareData(session *Session, data []byte) ([]*Packet, error) {
	if session == nil {
		return nil, ErrSessionRequired
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	// Validate session
	if session.Serializer == nil || session.CodecChain == nil {
		return nil, ErrTransportNotReady
	}

	// Step 1: Serialize
	serialized, err := session.Serializer.Serialize(data)
	if err != nil {
		return nil, err
	}

	// Step 2: Encode
	encoded, err := session.CodecChain.Encode(serialized)
	if err != nil {
		return nil, err
	}

	// Step 3: Fragment (if enabled)
	if s.config.EnableFragment && session.Fragment != nil {
		fragments, err := session.Fragment.Split(encoded, s.config.MaxFragmentSize)
		if err != nil {
			return nil, err
		}

		// Step 4: Wrap each fragment in a packet
		packets := make([]*Packet, len(fragments))
		groupID := internal.GenerateID()

		for i, frag := range fragments {
			packet := NewPacket(session.ID, session.SerializerType, frag)
			fragInfo := fragment.FragmentInfo{
				ID:     groupID,
				Index:  uint16(i),
				Total:  uint16(len(fragments)),
				IsLast: i == len(fragments)-1,
			}
			if s.config.EnableChecksum {
				fragInfo.Checksum = internal.CalculateChecksum(frag)
			}
			packet.WithFragment(fragInfo)
			packets[i] = packet
		}

		return packets, nil
	}

	// No fragmentation - wrap as single packet
	packet := NewPacket(session.ID, session.SerializerType, encoded)
	packet.Header.FragmentInfo = fragment.FragmentInfo{
		ID:     internal.GenerateID(),
		Index:  0,
		Total:  1,
		IsLast: true,
	}
	return []*Packet{packet}, nil
}

// EncodePacket encodes a packet for transmission.
func (s *TransportSender) EncodePacket(packet *Packet) ([]byte, error) {
	return packet.Encode()
}

// SendPackets sends packets through the session's channel.
func (s *TransportSender) SendPackets(session *Session, packets []*Packet) error {
	if session == nil {
		return ErrSessionRequired
	}

	for _, packet := range packets {
		encoded, err := packet.Encode()
		if err != nil {
			session.IncrementErrorCount()
			return err
		}

		if err := session.Send(encoded); err != nil {
			return err
		}
	}

	return nil
}

// Send sends data through the complete transport flow.
// Convenience method that combines PrepareData, EncodePacket, and SendPackets.
func (s *TransportSender) Send(session *Session, data []byte) error {
	packets, err := s.PrepareData(session, data)
	if err != nil {
		return err
	}
	return s.SendPackets(session, packets)
}

// TransportReceiver handles the receiving flow.
type TransportReceiver struct {
	mu              sync.RWMutex
	config          TransportConfig
	fragmentManager *fragment.DefaultFragmentManager
}

// NewTransportReceiver creates a new TransportReceiver.
func NewTransportReceiver(config TransportConfig) *TransportReceiver {
	fragConfig := fragment.DefaultFragmentConfig()
	fragConfig.MaxFragmentSize = config.MaxFragmentSize
	fragConfig.EnableChecksum = config.EnableChecksum
	fragConfig.Timeout = config.FragmentTimeout

	return &TransportReceiver{
		config:          config,
		fragmentManager: fragment.NewFragmentManager(fragConfig),
	}
}

// ReceiveAndProcess receives and processes data from the channel.
//
// Flow: receive -> decode_packet -> check_fragment -> reassemble -> decode -> deserialize
//
// Returns complete data when all fragments received, nil when waiting for more fragments.
func (r *TransportReceiver) ReceiveAndProcess(session *Session) ([]byte, error) {
	if session == nil {
		return nil, ErrSessionRequired
	}

	// Step 1: Receive raw data
	rawData, err := session.Receive()
	if err != nil {
		return nil, err
	}

	// Step 2: Decode packet
	packet, err := DecodePacket(rawData)
	if err != nil {
		return nil, err
	}

	// Step 3: Verify packet
	if err := packet.Verify(); err != nil {
		return nil, err
	}

	// Step 4: Check if fragment
	if packet.IsFragment() {
		return r.processFragment(session, packet)
	}

	// No fragmentation - process directly
	return r.processData(session, packet.Payload)
}

// processFragment handles fragment processing and reassembly.
func (r *TransportReceiver) processFragment(session *Session, packet *Packet) ([]byte, error) {
	fragInfo := packet.Header.FragmentInfo

	// Create state if not exists
	if err := r.fragmentManager.CreateState(fragInfo.ID, int(fragInfo.Total)); err != nil {
		// State may already exist, ignore
	}

	// Add fragment to manager
	if err := r.fragmentManager.AddFragment(fragInfo.ID, int(fragInfo.Index), packet.Payload); err != nil {
		return nil, err
	}

	// Check if complete
	complete, err := r.fragmentManager.IsComplete(fragInfo.ID)
	if err != nil {
		return nil, err
	}

	if !complete {
		// Wait for more fragments
		return nil, nil
	}

	// Reassemble
	reassembled, err := r.fragmentManager.Reassemble(fragInfo.ID)
	if err != nil {
		return nil, err
	}

	// Process reassembled data
	return r.processData(session, reassembled)
}

// processData processes complete data through decode and deserialize.
func (r *TransportReceiver) processData(session *Session, data []byte) ([]byte, error) {
	session.mu.RLock()
	defer session.mu.RUnlock()

	// Step 1: Decode
	decoded, err := session.CodecChain.Decode(data)
	if err != nil {
		return nil, err
	}

	// Step 2: Deserialize
	result, err := session.Serializer.Deserialize(decoded)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ProcessRawData processes raw received data (already received from channel).
// Use this when you want to control the channel receive yourself.
func (r *TransportReceiver) ProcessRawData(session *Session, rawData []byte) ([]byte, error) {
	if session == nil {
		return nil, ErrSessionRequired
	}

	// Decode packet
	packet, err := DecodePacket(rawData)
	if err != nil {
		return nil, err
	}

	// Verify packet
	if err := packet.Verify(); err != nil {
		return nil, err
	}

	// Check if fragment
	if packet.IsFragment() {
		return r.processFragment(session, packet)
	}

	return r.processData(session, packet.Payload)
}

// GetMissingFragments returns missing fragment indices for a fragment group.
func (r *TransportReceiver) GetMissingFragments(fragmentID string) ([]int, error) {
	return r.fragmentManager.GetMissingIndices(fragmentID)
}

// ClearFragmentState clears fragment state for a fragment group.
func (r *TransportReceiver) ClearFragmentState(fragmentID string) error {
	return r.fragmentManager.ClearState(fragmentID)
}

// ClearAllFragmentStates clears all fragment states.
func (r *TransportReceiver) ClearAllFragmentStates() error {
	return r.fragmentManager.ClearAll()
}

// FragmentCount returns number of active fragment groups being reassembled.
func (r *TransportReceiver) FragmentCount() int {
	return r.fragmentManager.Count()
}

// CleanupExpiredFragments removes expired fragment groups.
func (r *TransportReceiver) CleanupExpiredFragments(timeout int) []string {
	duration := timeout
	if duration == 0 {
		duration = r.config.FragmentTimeout
	}
	return r.fragmentManager.GetTimeoutIDs(
		0, // We'll implement proper timeout checking later
	)
}

// Transport provides a combined sender and receiver.
type Transport struct {
	sender   *TransportSender
	receiver *TransportReceiver
	config   TransportConfig
}

// NewTransport creates a new Transport with sender and receiver.
func NewTransport(config TransportConfig) *Transport {
	return &Transport{
		sender:   NewTransportSender(config),
		receiver: NewTransportReceiver(config),
		config:   config,
	}
}

// Send sends data through the transport.
func (t *Transport) Send(session *Session, data []byte) error {
	return t.sender.Send(session, data)
}

// Receive receives and processes data from the transport.
func (t *Transport) Receive(session *Session) ([]byte, error) {
	return t.receiver.ReceiveAndProcess(session)
}

// ProcessRawData processes raw received data.
func (t *Transport) ProcessRawData(session *Session, rawData []byte) ([]byte, error) {
	return t.receiver.ProcessRawData(session, rawData)
}

// PrepareData prepares data for sending (without actual transmission).
func (t *Transport) PrepareData(session *Session, data []byte) ([]*Packet, error) {
	return t.sender.PrepareData(session, data)
}

// Sender returns the transport sender.
func (t *Transport) Sender() *TransportSender {
	return t.sender
}

// Receiver returns the transport receiver.
func (t *Transport) Receiver() *TransportReceiver {
	return t.receiver
}

// Config returns the transport configuration.
func (t *Transport) Config() TransportConfig {
	return t.config
}
