// Package protocol provides control messages for VoidBus communication.
//
// ControlMessage is used for session management, acknowledgment,
// and heartbeat operations between client and server.
//
// Design Constraints:
// - Control messages are sent outside normal data flow
// - Control messages are NOT encrypted by CodecChain (sent raw)
// - Control messages contain minimal metadata for performance
package protocol

import (
	"encoding/binary"
	"errors"
	"time"
)

// Control message errors
var (
	ErrInvalidControlMessage = errors.New("control: invalid message")
	ErrControlTypeUnknown    = errors.New("control: unknown type")
	ErrControlPayloadInvalid = errors.New("control: invalid payload")
)

// ControlMessageType defines the type of control message.
type ControlMessageType uint8

const (
	// ControlAck indicates acknowledgment of received data/fragment
	ControlAck ControlMessageType = 0
	// ControlNack indicates negative acknowledgment (request retransmit)
	ControlNack ControlMessageType = 1
	// ControlHeartbeat indicates heartbeat/keepalive message
	ControlHeartbeat ControlMessageType = 2
	// ControlDisconnect indicates session disconnect notification
	ControlDisconnect ControlMessageType = 3
	// ControlFragmentAck indicates acknowledgment of complete fragment group
	ControlFragmentAck ControlMessageType = 4
	// ControlFragmentNack indicates request for missing fragments
	ControlFragmentNack ControlMessageType = 5
	// ControlPing indicates ping request (for latency measurement)
	ControlPing ControlMessageType = 6
	// ControlPong indicates ping response
	ControlPong ControlMessageType = 7
)

// String returns string representation of ControlMessageType.
func (t ControlMessageType) String() string {
	switch t {
	case ControlAck:
		return "ack"
	case ControlNack:
		return "nack"
	case ControlHeartbeat:
		return "heartbeat"
	case ControlDisconnect:
		return "disconnect"
	case ControlFragmentAck:
		return "fragment_ack"
	case ControlFragmentNack:
		return "fragment_nack"
	case ControlPing:
		return "ping"
	case ControlPong:
		return "pong"
	default:
		return "unknown"
	}
}

// ControlMessage represents a control message.
type ControlMessage struct {
	// Type is the control message type
	Type ControlMessageType

	// SessionID is the session identifier
	SessionID string

	// SequenceNumber is the message sequence number (for ack/nack)
	SequenceNumber uint64

	// FragmentID is fragment group ID (for fragment_ack/nack)
	FragmentID string

	// MissingIndices is list of missing fragment indices (for fragment_nack)
	MissingIndices []uint16

	// Timestamp is message timestamp (for ping/pong latency calculation)
	Timestamp int64

	// Payload is additional payload data (optional)
	Payload []byte
}

// ControlMessageHeaderSize is the fixed header size for control messages.
const ControlMessageHeaderSize = 32

// NewControlMessage creates a new control message.
func NewControlMessage(typ ControlMessageType, sessionID string) *ControlMessage {
	return &ControlMessage{
		Type:      typ,
		SessionID: sessionID,
		Timestamp: time.Now().UnixNano(),
	}
}

// WithSequenceNumber sets the sequence number.
func (m *ControlMessage) WithSequenceNumber(seq uint64) *ControlMessage {
	m.SequenceNumber = seq
	return m
}

// WithFragmentID sets the fragment group ID.
func (m *ControlMessage) WithFragmentID(id string) *ControlMessage {
	m.FragmentID = id
	return m
}

// WithMissingIndices sets missing fragment indices.
func (m *ControlMessage) WithMissingIndices(indices []uint16) *ControlMessage {
	m.MissingIndices = indices
	return m
}

// WithPayload sets additional payload.
func (m *ControlMessage) WithPayload(payload []byte) *ControlMessage {
	m.Payload = payload
	return m
}

// Ack creates an acknowledgment message.
func Ack(sessionID string, seq uint64) *ControlMessage {
	return NewControlMessage(ControlAck, sessionID).WithSequenceNumber(seq)
}

// Nack creates a negative acknowledgment message.
func Nack(sessionID string, seq uint64) *ControlMessage {
	return NewControlMessage(ControlNack, sessionID).WithSequenceNumber(seq)
}

// Heartbeat creates a heartbeat message.
func Heartbeat(sessionID string) *ControlMessage {
	return NewControlMessage(ControlHeartbeat, sessionID)
}

// Disconnect creates a disconnect notification message.
func Disconnect(sessionID string) *ControlMessage {
	return NewControlMessage(ControlDisconnect, sessionID)
}

// FragmentAck creates a fragment group acknowledgment message.
func FragmentAck(sessionID string, fragmentID string) *ControlMessage {
	return NewControlMessage(ControlFragmentAck, sessionID).WithFragmentID(fragmentID)
}

// FragmentNack creates a fragment group negative acknowledgment with missing indices.
func FragmentNack(sessionID string, fragmentID string, missing []uint16) *ControlMessage {
	return NewControlMessage(ControlFragmentNack, sessionID).
		WithFragmentID(fragmentID).
		WithMissingIndices(missing)
}

// Ping creates a ping request message.
func Ping(sessionID string) *ControlMessage {
	return NewControlMessage(ControlPing, sessionID)
}

// Pong creates a ping response message (echoes the ping timestamp).
func Pong(sessionID string, pingTimestamp int64) *ControlMessage {
	return NewControlMessage(ControlPong, sessionID).WithPayload(
		binary.BigEndian.AppendUint64(nil, uint64(pingTimestamp)),
	)
}

// Encode encodes the control message to bytes.
// Format:
// [type(1)][session_id_len(2)][session_id][seq(8)][fragment_id_len(2)][fragment_id]
// [timestamp(8)][missing_count(2)][missing_indices...][payload_len(2)][payload]
func (m *ControlMessage) Encode() ([]byte, error) {
	result := make([]byte, 0, ControlMessageHeaderSize)

	// Type (1 byte)
	result = append(result, byte(m.Type))

	// Session ID (2 bytes length + data)
	sessionIDBytes := []byte(m.SessionID)
	result = binary.BigEndian.AppendUint16(result, uint16(len(sessionIDBytes)))
	result = append(result, sessionIDBytes...)

	// Sequence number (8 bytes)
	result = binary.BigEndian.AppendUint64(result, m.SequenceNumber)

	// Fragment ID (2 bytes length + data)
	fragmentIDBytes := []byte(m.FragmentID)
	result = binary.BigEndian.AppendUint16(result, uint16(len(fragmentIDBytes)))
	result = append(result, fragmentIDBytes...)

	// Timestamp (8 bytes)
	result = binary.BigEndian.AppendUint64(result, uint64(m.Timestamp))

	// Missing indices count (2 bytes)
	result = binary.BigEndian.AppendUint16(result, uint16(len(m.MissingIndices)))

	// Missing indices (each 2 bytes)
	for _, idx := range m.MissingIndices {
		result = binary.BigEndian.AppendUint16(result, idx)
	}

	// Payload (2 bytes length + data)
	result = binary.BigEndian.AppendUint16(result, uint16(len(m.Payload)))
	result = append(result, m.Payload...)

	return result, nil
}

// DecodeControlMessage decodes bytes to a ControlMessage.
func DecodeControlMessage(data []byte) (*ControlMessage, error) {
	if len(data) < 1 {
		return nil, ErrInvalidControlMessage
	}

	msg := &ControlMessage{}
	offset := 0

	// Type (1 byte)
	msg.Type = ControlMessageType(data[offset])
	offset++

	// Validate type
	if msg.Type > ControlPong {
		return nil, ErrControlTypeUnknown
	}

	// Session ID (2 bytes length + data)
	if len(data) < offset+2 {
		return nil, ErrInvalidControlMessage
	}
	sessionIDLen := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2
	if len(data) < offset+int(sessionIDLen) {
		return nil, ErrInvalidControlMessage
	}
	msg.SessionID = string(data[offset : offset+int(sessionIDLen)])
	offset += int(sessionIDLen)

	// Sequence number (8 bytes)
	if len(data) < offset+8 {
		return nil, ErrInvalidControlMessage
	}
	msg.SequenceNumber = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Fragment ID (2 bytes length + data)
	if len(data) < offset+2 {
		return nil, ErrInvalidControlMessage
	}
	fragmentIDLen := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2
	if len(data) < offset+int(fragmentIDLen) {
		return nil, ErrInvalidControlMessage
	}
	msg.FragmentID = string(data[offset : offset+int(fragmentIDLen)])
	offset += int(fragmentIDLen)

	// Timestamp (8 bytes)
	if len(data) < offset+8 {
		return nil, ErrInvalidControlMessage
	}
	msg.Timestamp = int64(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8

	// Missing indices count (2 bytes)
	if len(data) < offset+2 {
		return nil, ErrInvalidControlMessage
	}
	missingCount := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Missing indices (each 2 bytes)
	msg.MissingIndices = make([]uint16, missingCount)
	for i := 0; i < int(missingCount); i++ {
		if len(data) < offset+2 {
			return nil, ErrInvalidControlMessage
		}
		msg.MissingIndices[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	// Payload (2 bytes length + data)
	if len(data) < offset+2 {
		return nil, ErrInvalidControlMessage
	}
	payloadLen := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2
	if len(data) < offset+int(payloadLen) {
		return nil, ErrInvalidControlMessage
	}
	msg.Payload = make([]byte, payloadLen)
	copy(msg.Payload, data[offset:offset+int(payloadLen)])

	return msg, nil
}

// IsAck returns whether this is an acknowledgment message.
func (m *ControlMessage) IsAck() bool {
	return m.Type == ControlAck || m.Type == ControlFragmentAck
}

// IsNack returns whether this is a negative acknowledgment message.
func (m *ControlMessage) IsNack() bool {
	return m.Type == ControlNack || m.Type == ControlFragmentNack
}

// IsHeartbeat returns whether this is a heartbeat message.
func (m *ControlMessage) IsHeartbeat() bool {
	return m.Type == ControlHeartbeat
}

// IsPing returns whether this is a ping message.
func (m *ControlMessage) IsPing() bool {
	return m.Type == ControlPing
}

// IsPong returns whether this is a pong message.
func (m *ControlMessage) IsPong() bool {
	return m.Type == ControlPong
}

// CalculateLatency calculates latency from pong message (nanoseconds).
func (m *ControlMessage) CalculateLatency() (int64, error) {
	if m.Type != ControlPong || len(m.Payload) < 8 {
		return 0, ErrControlPayloadInvalid
	}

	pingTimestamp := int64(binary.BigEndian.Uint64(m.Payload[:8]))
	return m.Timestamp - pingTimestamp, nil
}

// ControlMessageHandler handles received control messages.
type ControlMessageHandler interface {
	// HandleAck handles acknowledgment message
	HandleAck(msg *ControlMessage) error

	// HandleNack handles negative acknowledgment message
	HandleNack(msg *ControlMessage) error

	// HandleHeartbeat handles heartbeat message
	HandleHeartbeat(msg *ControlMessage) error

	// HandleDisconnect handles disconnect notification
	HandleDisconnect(msg *ControlMessage) error

	// HandleFragmentAck handles fragment group acknowledgment
	HandleFragmentAck(msg *ControlMessage) error

	// HandleFragmentNack handles fragment group negative acknowledgment
	HandleFragmentNack(msg *ControlMessage) error

	// HandlePing handles ping request
	HandlePing(msg *ControlMessage) error

	// HandlePong handles ping response
	HandlePong(msg *ControlMessage) error
}

// DefaultControlMessageHandler is a default implementation.
type DefaultControlMessageHandler struct {
	OnAck          func(msg *ControlMessage) error
	OnNack         func(msg *ControlMessage) error
	OnHeartbeat    func(msg *ControlMessage) error
	OnDisconnect   func(msg *ControlMessage) error
	OnFragmentAck  func(msg *ControlMessage) error
	OnFragmentNack func(msg *ControlMessage) error
	OnPing         func(msg *ControlMessage) error
	OnPong         func(msg *ControlMessage) error
}

// HandleAck handles acknowledgment.
func (h *DefaultControlMessageHandler) HandleAck(msg *ControlMessage) error {
	if h.OnAck != nil {
		return h.OnAck(msg)
	}
	return nil
}

// HandleNack handles negative acknowledgment.
func (h *DefaultControlMessageHandler) HandleNack(msg *ControlMessage) error {
	if h.OnNack != nil {
		return h.OnNack(msg)
	}
	return nil
}

// HandleHeartbeat handles heartbeat.
func (h *DefaultControlMessageHandler) HandleHeartbeat(msg *ControlMessage) error {
	if h.OnHeartbeat != nil {
		return h.OnHeartbeat(msg)
	}
	return nil
}

// HandleDisconnect handles disconnect.
func (h *DefaultControlMessageHandler) HandleDisconnect(msg *ControlMessage) error {
	if h.OnDisconnect != nil {
		return h.OnDisconnect(msg)
	}
	return nil
}

// HandleFragmentAck handles fragment acknowledgment.
func (h *DefaultControlMessageHandler) HandleFragmentAck(msg *ControlMessage) error {
	if h.OnFragmentAck != nil {
		return h.OnFragmentAck(msg)
	}
	return nil
}

// HandleFragmentNack handles fragment negative acknowledgment.
func (h *DefaultControlMessageHandler) HandleFragmentNack(msg *ControlMessage) error {
	if h.OnFragmentNack != nil {
		return h.OnFragmentNack(msg)
	}
	return nil
}

// HandlePing handles ping request.
func (h *DefaultControlMessageHandler) HandlePing(msg *ControlMessage) error {
	if h.OnPing != nil {
		return h.OnPing(msg)
	}
	return nil
}

// HandlePong handles ping response.
func (h *DefaultControlMessageHandler) HandlePong(msg *ControlMessage) error {
	if h.OnPong != nil {
		return h.OnPong(msg)
	}
	return nil
}
