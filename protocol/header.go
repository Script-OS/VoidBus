// Package protocol provides v2.0 metadata structures for VoidBus.
// These structures are specific to the v2.0 architecture with codec hash matching.
package protocol

import (
	"time"

	"github.com/Script-OS/VoidBus/internal"
)

// Header is the v2.0 fragment header metadata.
// Attached to each fragment for routing and decoding.
type Header struct {
	// === 核心字段（必须） ===
	SessionID     string // UUID，标识本次发送
	FragmentIndex uint16 // 分片序号（0-based）
	FragmentTotal uint16 // 总分片数

	// === Codec 信息 ===
	CodecDepth uint8    // Codec 链深度（层数）
	CodecHash  [32]byte // SHA256(代号组合)，用于匹配解码链

	// === 数据完整性 ===
	DataChecksum uint32   // CRC32(分片数据)
	DataHash     [32]byte // SHA256(原始完整数据)，用于重组验证

	// === 时间戳 ===
	Timestamp int64 // 发送时间，用于超时判断

	// === 标志位 ===
	Flags uint8 // 标志位集合
}

// Flag constants for Header
const (
	FlagIsLast     uint8 = 0x01 // 是否最后一个分片
	FlagRetransmit uint8 = 0x02 // 是否重传分片
	FlagIsNAK      uint8 = 0x04 // 是否NAK请求
	FlagIsENDACK  uint8 = 0x08 // 是否END_ACK确认
)

// IsLastFragment returns true if this is the last fragment.
func (h *Header) IsLastFragment() bool {
	return (h.Flags & FlagIsLast) != 0
}

// SetIsLast sets the IsLast flag.
func (h *Header) SetIsLast(isLast bool) {
	if isLast {
		h.Flags |= FlagIsLast
	} else {
		h.Flags &= ^FlagIsLast
	}
}

// IsRetransmit returns true if this is a retransmitted fragment.
func (h *Header) IsRetransmit() bool {
	return (h.Flags & FlagRetransmit) != 0
}

// SetRetransmit sets the Retransmit flag.
func (h *Header) SetRetransmit(isRetransmit bool) {
	if isRetransmit {
		h.Flags |= FlagRetransmit
	} else {
		h.Flags &= ^FlagRetransmit
	}
}

// IsNAK returns true if this is a NAK message.
func (h *Header) IsNAK() bool {
	return (h.Flags & FlagIsNAK) != 0
}

// IsEND_ACK returns true if this is an END_ACK message.
func (h *Header) IsEND_ACK() bool {
	return (h.Flags & FlagIsENDACK) != 0
}

// NewHeader creates new v2.0 fragment header.
func NewHeader(sessionID string, index, total uint16, codecDepth uint8, codecHash [32]byte) *Header {
	return &Header{
		SessionID:     sessionID,
		FragmentIndex: index,
		FragmentTotal: total,
		CodecDepth:    codecDepth,
		CodecHash:     codecHash,
		Timestamp:     time.Now().Unix(),
		Flags:         0,
	}
}

// SetDataChecksum calculates and sets checksum for fragment data.
func (h *Header) SetDataChecksum(data []byte) {
	h.DataChecksum = internal.CalculateChecksum(data)
}

// VerifyDataChecksum verifies fragment data against checksum.
func (h *Header) VerifyDataChecksum(data []byte) bool {
	return internal.VerifyChecksum(data, h.DataChecksum)
}

// SetDataHash sets the overall data hash for integrity verification.
func (h *Header) SetDataHash(data []byte) {
	h.DataHash = internal.ComputeDataHash(data)
}

// VerifyDataHash verifies overall data integrity after reassembly.
func (h *Header) VerifyDataHash(data []byte) bool {
	return internal.VerifyDataHash(data, h.DataHash)
}

// NAKMessage represents a negative acknowledgment message.
// Sent when fragments are missing or corrupted.
type NAKMessage struct {
	SessionID      string   // Session identifier
	MissingIndices []uint16 // List of missing fragment indices
	Timestamp      int64    // Request timestamp
}

// NewNAKMessage creates new NAK message.
func NewNAKMessage(sessionID string, missingIndices []uint16) *NAKMessage {
	return &NAKMessage{
		SessionID:      sessionID,
		MissingIndices: missingIndices,
		Timestamp:      time.Now().Unix(),
	}
}

// END_ACKMessage represents end acknowledgment message.
// Sent when all fragments are received successfully.
type END_ACKMessage struct {
	SessionID string   // Session identifier
	Status    string   // Status: "COMPLETE" or "FAILED"
	DataHash  [32]byte // Hash of received data for verification
	Timestamp int64    // Acknowledgment timestamp
}

const (
	StatusComplete = "COMPLETE"
	StatusFailed   = "FAILED"
)

// NewEND_ACKMessage creates new END_ACK message.
func NewEND_ACKMessage(sessionID string, dataHash [32]byte) *END_ACKMessage {
	return &END_ACKMessage{
		SessionID: sessionID,
		Status:    StatusComplete,
		DataHash:  dataHash,
		Timestamp: time.Now().Unix(),
	}
}

// NegotiationRequest represents capability negotiation request.
type NegotiationRequest struct {
	ClientID       string   // Client identifier
	SupportedCodes []string // Supported codec codes (user-defined identifiers)
	MaxDepth       int      // Maximum codec chain depth supported
	Timestamp      int64    // Negotiation timestamp
	Salt           []byte   // Salt for hash computation
}

// NewNegotiationRequest creates new negotiation request.
func NewNegotiationRequest(clientID string, supportedCodes []string, maxDepth int) *NegotiationRequest {
	salt, _ := internal.GenerateSalt()
	return &NegotiationRequest{
		ClientID:       clientID,
		SupportedCodes: supportedCodes,
		MaxDepth:       maxDepth,
		Timestamp:      time.Now().Unix(),
		Salt:           salt,
	}
}

// NegotiationResponse represents negotiation response.
type NegotiationResponse struct {
	Accepted     bool     // Whether negotiation is accepted
	RejectReason string   // Reason for rejection (if not accepted)
	CommonCodes  []string // Common supported codec codes
	MaxDepth     int      // Agreed maximum depth
	Salt         []byte   // Salt for hash computation (must match request)
	Timestamp    int64    // Response timestamp
}

// NewNegotiationResponse creates new negotiation response.
func NewNegotiationResponse(accepted bool, commonCodes []string, maxDepth int, salt []byte, rejectReason string) *NegotiationResponse {
	return &NegotiationResponse{
		Accepted:     accepted,
		CommonCodes:  commonCodes,
		MaxDepth:     maxDepth,
		Salt:         salt,
		RejectReason: rejectReason,
		Timestamp:    time.Now().Unix(),
	}
}

// === Binary Encoding/Decoding ===

// HeaderSize is the fixed size of Header in binary form (excluding SessionID).
const HeaderBaseSize = 2 + 2 + 1 + 32 + 4 + 32 + 8 + 1 // 82 bytes

// Encode encodes the header and data into a binary packet.
// Format: [SessionIDLen:2][SessionID:N][HeaderBase:82][Data:M]
func (h *Header) Encode(data []byte) []byte {
	sessionIDBytes := []byte(h.SessionID)
	sessionIDLen := len(sessionIDBytes)

	totalLen := 2 + sessionIDLen + HeaderBaseSize + len(data)
	result := make([]byte, totalLen)

	offset := 0

	// SessionID length (2 bytes)
	result[offset] = byte(sessionIDLen >> 8)
	result[offset+1] = byte(sessionIDLen)
	offset += 2

	// SessionID
	copy(result[offset:], sessionIDBytes)
	offset += sessionIDLen

	// FragmentIndex (2 bytes)
	result[offset] = byte(h.FragmentIndex >> 8)
	result[offset+1] = byte(h.FragmentIndex)
	offset += 2

	// FragmentTotal (2 bytes)
	result[offset] = byte(h.FragmentTotal >> 8)
	result[offset+1] = byte(h.FragmentTotal)
	offset += 2

	// CodecDepth (1 byte)
	result[offset] = byte(h.CodecDepth)
	offset += 1

	// CodecHash (32 bytes)
	copy(result[offset:], h.CodecHash[:])
	offset += 32

	// DataChecksum (4 bytes)
	result[offset] = byte(h.DataChecksum >> 24)
	result[offset+1] = byte(h.DataChecksum >> 16)
	result[offset+2] = byte(h.DataChecksum >> 8)
	result[offset+3] = byte(h.DataChecksum)
	offset += 4

	// DataHash (32 bytes)
	copy(result[offset:], h.DataHash[:])
	offset += 32

	// Timestamp (8 bytes)
	for i := 0; i < 8; i++ {
		result[offset+i] = byte(h.Timestamp >> (56 - i*8))
	}
	offset += 8

	// Flags (1 byte)
	result[offset] = h.Flags
	offset += 1

	// Data
	copy(result[offset:], data)

	return result
}

// DecodeHeader decodes a binary packet into header and data.
func DecodeHeader(packet []byte) (*Header, []byte, error) {
	if len(packet) < 2 {
		return nil, nil, ErrV2InvalidPacket
	}

	offset := 0

	// SessionID length
	sessionIDLen := int(packet[offset])<<8 | int(packet[offset+1])
	offset += 2

	if len(packet) < offset+sessionIDLen+HeaderBaseSize {
		return nil, nil, ErrV2InvalidPacket
	}

	// SessionID
	sessionID := string(packet[offset : offset+sessionIDLen])
	offset += sessionIDLen

	header := &Header{
		SessionID: sessionID,
	}

	// FragmentIndex
	header.FragmentIndex = uint16(packet[offset])<<8 | uint16(packet[offset+1])
	offset += 2

	// FragmentTotal
	header.FragmentTotal = uint16(packet[offset])<<8 | uint16(packet[offset+1])
	offset += 2

	// CodecDepth
	header.CodecDepth = packet[offset]
	offset += 1

	// CodecHash
	copy(header.CodecHash[:], packet[offset:offset+32])
	offset += 32

	// DataChecksum
	header.DataChecksum = uint32(packet[offset])<<24 | uint32(packet[offset+1])<<16 |
		uint32(packet[offset+2])<<8 | uint32(packet[offset+3])
	offset += 4

	// DataHash
	copy(header.DataHash[:], packet[offset:offset+32])
	offset += 32

	// Timestamp
	header.Timestamp = 0
	for i := 0; i < 8; i++ {
		header.Timestamp |= int64(packet[offset+i]) << (56 - i*8)
	}
	offset += 8

	// Flags
	header.Flags = packet[offset]
	offset += 1

	// Data
	data := packet[offset:]

	return header, data, nil
}

// ErrV2InvalidPacket is returned when v2 packet decoding fails.
var ErrV2InvalidPacket = v2errorf("invalid packet format")

func v2errorf(msg string) error {
	return &V2ProtocolError{Msg: msg}
}

// V2ProtocolError represents a v2 protocol error.
type V2ProtocolError struct {
	Msg string
}

func (e *V2ProtocolError) Error() string {
	return e.Msg
}
