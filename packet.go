// Package voidbus provides packet structures for VoidBus communication.
package voidbus

import (
	"encoding/binary"
	"errors"
	"time"

	"VoidBus/fragment"
	"VoidBus/internal"
)

// Packet errors
var (
	ErrInvalidPacket    = errors.New("packet: invalid packet")
	ErrInvalidHeader    = errors.New("packet: invalid header")
	ErrPayloadTooLarge  = errors.New("packet: payload too large")
	ErrChecksumMismatch = errors.New("packet: checksum mismatch")
	ErrTimestampExpired = errors.New("packet: timestamp expired")
)

// Packet header constants
const (
	PacketVersion   = 1
	HeaderMinSize   = 64               // Minimum header size in bytes
	MaxPayloadSize  = 10 * 1024 * 1024 // 10MB max payload
	TimestampExpiry = 5 * time.Minute  // Packet timestamp expiry
)

// Packet represents a data packet transmitted over the bus.
type Packet struct {
	// Header contains metadata (can be partially exposed)
	Header PacketHeader

	// Payload is the actual data (after Serializer + CodecChain processing)
	Payload []byte
}

// PacketHeader contains packet metadata.
// Security Design:
// - SessionID: Random UUID, used as indirect reference to local config
// - SerializerType: CAN be exposed (serializer name only)
// - CodecChain info: NOT exposed (stored locally in SessionRegistry)
// - Channel info: NOT exposed (stored locally in SessionRegistry)
type PacketHeader struct {
	// SessionID is session identifier (random UUID, no semantic info)
	// Used as index into local SessionRegistry
	SessionID string

	// FragmentInfo contains fragmentation metadata
	FragmentInfo fragment.FragmentInfo

	// SerializerType is the serializer name (CAN be exposed)
	SerializerType string

	// PayloadChecksum is CRC32 checksum of payload
	PayloadChecksum uint32

	// Timestamp is packet creation time (for replay attack prevention)
	Timestamp int64

	// Version is protocol version
	Version uint8
}

// NewPacket creates a new packet.
func NewPacket(sessionID, serializerType string, payload []byte) *Packet {
	return &Packet{
		Header: PacketHeader{
			SessionID:       sessionID,
			SerializerType:  serializerType,
			PayloadChecksum: internal.CalculateChecksum(payload),
			Timestamp:       time.Now().Unix(),
			Version:         PacketVersion,
		},
		Payload: payload,
	}
}

// WithFragment sets fragment info for the packet.
func (p *Packet) WithFragment(info fragment.FragmentInfo) *Packet {
	p.Header.FragmentInfo = info
	return p
}

// Verify verifies packet integrity and freshness.
func (p *Packet) Verify() error {
	// Verify version
	if p.Header.Version != PacketVersion {
		return ErrInvalidPacket
	}

	// Verify checksum
	checksum := internal.CalculateChecksum(p.Payload)
	if checksum != p.Header.PayloadChecksum {
		return ErrChecksumMismatch
	}

	// Verify timestamp (replay attack prevention)
	timestamp := time.Unix(p.Header.Timestamp, 0)
	if time.Since(timestamp) > TimestampExpiry {
		return ErrTimestampExpired
	}

	return nil
}

// Encode encodes the packet to bytes for transmission.
// Format: [header_length(4)][header][payload]
func (p *Packet) Encode() ([]byte, error) {
	headerData, err := p.encodeHeader()
	if err != nil {
		return nil, err
	}

	// Total length: 4 (header length) + header + payload
	totalLen := 4 + len(headerData) + len(p.Payload)
	result := make([]byte, totalLen)

	// Write header length (4 bytes, big endian)
	binary.BigEndian.PutUint32(result[0:4], uint32(len(headerData)))

	// Write header
	copy(result[4:4+len(headerData)], headerData)

	// Write payload
	copy(result[4+len(headerData):], p.Payload)

	return result, nil
}

// encodeHeader encodes header to bytes.
func (p *Packet) encodeHeader() ([]byte, error) {
	// Simple encoding: fixed-size fields + variable-length strings
	// Format:
	// [version(1)][timestamp(8)][checksum(4)]
	// [session_id_len(2)][session_id]
	// [serializer_len(2)][serializer]
	// [fragment_info]

	result := make([]byte, 0, HeaderMinSize)

	// Version
	result = append(result, p.Header.Version)

	// Timestamp (8 bytes)
	timestamp := make([]byte, 8)
	binary.BigEndian.PutUint64(timestamp, uint64(p.Header.Timestamp))
	result = append(result, timestamp...)

	// Checksum (4 bytes)
	checksum := make([]byte, 4)
	binary.BigEndian.PutUint32(checksum, p.Header.PayloadChecksum)
	result = append(result, checksum...)

	// Session ID
	sessionID := []byte(p.Header.SessionID)
	sessionIDLen := make([]byte, 2)
	binary.BigEndian.PutUint16(sessionIDLen, uint16(len(sessionID)))
	result = append(result, sessionIDLen...)
	result = append(result, sessionID...)

	// Serializer type
	serializer := []byte(p.Header.SerializerType)
	serializerLen := make([]byte, 2)
	binary.BigEndian.PutUint16(serializerLen, uint16(len(serializer)))
	result = append(result, serializerLen...)
	result = append(result, serializer...)

	// Fragment info (simplified: ID + Index + Total + IsLast + Checksum)
	fragID := []byte(p.Header.FragmentInfo.ID)
	fragIDLen := make([]byte, 2)
	binary.BigEndian.PutUint16(fragIDLen, uint16(len(fragID)))
	result = append(result, fragIDLen...)
	result = append(result, fragID...)

	// Fragment index (2 bytes)
	fragIndex := make([]byte, 2)
	binary.BigEndian.PutUint16(fragIndex, p.Header.FragmentInfo.Index)
	result = append(result, fragIndex...)

	// Fragment total (2 bytes)
	fragTotal := make([]byte, 2)
	binary.BigEndian.PutUint16(fragTotal, p.Header.FragmentInfo.Total)
	result = append(result, fragTotal...)

	// Is last (1 byte)
	if p.Header.FragmentInfo.IsLast {
		result = append(result, 1)
	} else {
		result = append(result, 0)
	}

	// Fragment checksum (4 bytes)
	fragChecksum := make([]byte, 4)
	binary.BigEndian.PutUint32(fragChecksum, p.Header.FragmentInfo.Checksum)
	result = append(result, fragChecksum...)

	return result, nil
}

// DecodePacket decodes bytes to a Packet.
func DecodePacket(data []byte) (*Packet, error) {
	if len(data) < 4 {
		return nil, ErrInvalidPacket
	}

	// Read header length
	headerLen := binary.BigEndian.Uint32(data[0:4])
	if len(data) < int(4+headerLen) {
		return nil, ErrInvalidPacket
	}

	// Decode header
	header, err := decodeHeader(data[4 : 4+headerLen])
	if err != nil {
		return nil, err
	}

	// Extract payload
	payload := make([]byte, len(data)-int(4+headerLen))
	copy(payload, data[4+headerLen:])

	return &Packet{
		Header:  *header,
		Payload: payload,
	}, nil
}

// decodeHeader decodes bytes to PacketHeader.
func decodeHeader(data []byte) (*PacketHeader, error) {
	if len(data) < 15 { // Minimum: version(1) + timestamp(8) + checksum(4) + lengths(2)
		return nil, ErrInvalidHeader
	}

	offset := 0
	header := &PacketHeader{}

	// Version
	header.Version = data[offset]
	offset++

	// Timestamp
	header.Timestamp = int64(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8

	// Checksum
	header.PayloadChecksum = binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	// Session ID
	sessionIDLen := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2
	if len(data) < offset+int(sessionIDLen) {
		return nil, ErrInvalidHeader
	}
	header.SessionID = string(data[offset : offset+int(sessionIDLen)])
	offset += int(sessionIDLen)

	// Serializer type
	serializerLen := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2
	if len(data) < offset+int(serializerLen) {
		return nil, ErrInvalidHeader
	}
	header.SerializerType = string(data[offset : offset+int(serializerLen)])
	offset += int(serializerLen)

	// Fragment info
	fragIDLen := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2
	if len(data) < offset+int(fragIDLen) {
		return nil, ErrInvalidHeader
	}
	header.FragmentInfo.ID = string(data[offset : offset+int(fragIDLen)])
	offset += int(fragIDLen)

	// Fragment index
	header.FragmentInfo.Index = binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Fragment total
	header.FragmentInfo.Total = binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Is last
	header.FragmentInfo.IsLast = data[offset] == 1
	offset++

	// Fragment checksum
	header.FragmentInfo.Checksum = binary.BigEndian.Uint32(data[offset : offset+4])

	return header, nil
}

// IsFragment returns true if packet is a fragment.
func (p *Packet) IsFragment() bool {
	return p.Header.FragmentInfo.ID != "" && p.Header.FragmentInfo.Total > 0
}

// IsLastFragment returns true if packet is the last fragment.
func (p *Packet) IsLastFragment() bool {
	return p.Header.FragmentInfo.IsLast
}
