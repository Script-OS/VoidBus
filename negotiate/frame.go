// Package negotiate provides negotiation frame structures and encoding/decoding.
//
// Frame format uses custom binary protocol (NOT VoidBus Header format).
// All negotiation data is encoded as bitmap for stealth.
package negotiate

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"time"

	"github.com/Script-OS/VoidBus/internal"
)

// Frame magic numbers for negotiation protocol.
const (
	NegotiateMagicRequest  byte = 0x56 // 'V' VoidBus request
	NegotiateMagicResponse byte = 0x42 // 'B' VoidBus response
	NegotiateVersion       byte = 0x01 // Protocol version
)

// Negotiation status codes.
const (
	NegotiateStatusSuccess byte = 0x00 // Negotiation successful
	NegotiateStatusReject  byte = 0x01 // Rejected (no common channels/codecs)
	NegotiateStatusRetry   byte = 0x02 // Retry needed
)

// Frame size constraints.
const (
	NegotiateMinFrameSize    = 20               // Minimum frame size
	NegotiateMaxFrameSize    = 300              // Maximum frame size
	NegotiateMaxPaddingSize  = 255              // Maximum padding size
	NegotiateNonceSize       = 8                // Session nonce size
	NegotiateSessionIDSize   = 8                // Session ID size
	NegotiateDefaultTimeout  = 10 * time.Second // Negotiation timeout
	NegotiateMaxTimestampAge = 30               // Max timestamp age (seconds)
)

// Frame errors.
var (
	ErrInvalidMagic     = errors.New("negotiate: invalid magic number")
	ErrInvalidVersion   = errors.New("negotiate: invalid protocol version")
	ErrInvalidChecksum  = errors.New("negotiate: invalid checksum")
	ErrInvalidFrameSize = errors.New("negotiate: invalid frame size")
	ErrTimestampExpired = errors.New("negotiate: timestamp expired")
	ErrNoCommonChannels = errors.New("negotiate: no common channels")
	ErrNoCommonCodecs   = errors.New("negotiate: no common codecs")
	ErrNonceSize        = errors.New("negotiate: invalid nonce size")
	ErrSessionIDSize    = errors.New("negotiate: invalid session ID size")
)

// NegotiateRequest represents a negotiation request frame.
//
// Frame format:
// [1 byte:  Magic]           - Fixed 0x56
// [1 byte:  Version]         - Protocol version 0x01
// [1 byte:  ChannelCount]    - Number of channel bitmap bytes
// [N bytes: ChannelBitmap]   - Supported channels bitmap
// [1 byte:  CodecCount]      - Number of codec bitmap bytes
// [N bytes: CodecBitmap]     - Supported codecs bitmap
// [8 bytes: SessionNonce]    - Random nonce for first connection
// [1 byte:  HasSessionID]    - 0 = no SessionID (first), 1 = has SessionID (subsequent)
// [8 bytes: SessionID]       - Optional, present only if HasSessionID = 1
// [4 bytes: Timestamp]       - Unix timestamp (anti-replay)
// [1 byte:  PaddingLen]      - Padding length (0-255)
// [M bytes: Padding]         - Random padding (stealth)
// [2 bytes: Checksum]        - CRC16 checksum
type NegotiateRequest struct {
	ChannelBitmap ChannelBitmap
	CodecBitmap   CodecBitmap
	SessionNonce  []byte // 8 bytes, always present
	SessionID     []byte // 8 bytes, optional (nil for first connection)
	Timestamp     uint32
	Padding       []byte // 0-255 bytes
}

// IsFirstConnection returns true if this is the first channel connection.
func (r *NegotiateRequest) IsFirstConnection() bool {
	return len(r.SessionID) == 0
}

// NegotiateResponse represents a negotiation response frame.
//
// Frame format:
// [1 byte:  Magic]           - Fixed 0x42
// [1 byte:  Version]         - Protocol version 0x01
// [1 byte:  ChannelCount]    - Number of channel bitmap bytes
// [N bytes: ChannelBitmap]   - Available channels (intersection)
// [1 byte:  CodecCount]      - Number of codec bitmap bytes
// [N bytes: CodecBitmap]     - Available codecs (intersection)
// [8 bytes: SessionID]       - Session identifier
// [1 byte:  Status]          - Negotiation status
// [1 byte:  PaddingLen]      - Padding length (0-255)
// [M bytes: Padding]         - Random padding (stealth)
// [2 bytes: Checksum]        - CRC16 checksum
type NegotiateResponse struct {
	ChannelBitmap ChannelBitmap
	CodecBitmap   CodecBitmap
	SessionID     []byte // 8 bytes
	Status        byte
	Padding       []byte // 0-255 bytes
}

// NewNegotiateRequest creates a new negotiation request.
// If sessionID is nil, this is the first connection and a new nonce is generated.
// If sessionID is provided, this is a subsequent connection for the same session.
func NewNegotiateRequest(channelBitmap, codecBitmap []byte, sessionID []byte) (*NegotiateRequest, error) {
	// Generate random nonce
	nonce := make([]byte, NegotiateNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	// Validate sessionID if provided
	if len(sessionID) > 0 && len(sessionID) != NegotiateSessionIDSize {
		return nil, ErrSessionIDSize
	}

	// Generate random padding (0-127 bytes for stealth)
	paddingLen := internal.RandomIntRange(0, 127)
	padding := make([]byte, paddingLen)
	if _, err := rand.Read(padding); err != nil {
		return nil, err
	}

	return &NegotiateRequest{
		ChannelBitmap: channelBitmap,
		CodecBitmap:   codecBitmap,
		SessionNonce:  nonce,
		SessionID:     sessionID, // nil for first, or existing SessionID
		Timestamp:     uint32(time.Now().Unix()),
		Padding:       padding,
	}, nil
}

// Encode encodes NegotiateRequest to binary frame.
func (r *NegotiateRequest) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Magic + Version
	buf.WriteByte(NegotiateMagicRequest)
	buf.WriteByte(NegotiateVersion)

	// Channel Bitmap
	if len(r.ChannelBitmap) > 255 {
		return nil, ErrInvalidFrameSize
	}
	buf.WriteByte(byte(len(r.ChannelBitmap)))
	buf.Write(r.ChannelBitmap)

	// Codec Bitmap
	if len(r.CodecBitmap) > 255 {
		return nil, ErrInvalidFrameSize
	}
	buf.WriteByte(byte(len(r.CodecBitmap)))
	buf.Write(r.CodecBitmap)

	// SessionNonce (8 bytes, always present)
	if len(r.SessionNonce) != NegotiateNonceSize {
		return nil, ErrNonceSize
	}
	buf.Write(r.SessionNonce)

	// HasSessionID flag + optional SessionID
	if len(r.SessionID) > 0 {
		buf.WriteByte(1)
		buf.Write(r.SessionID)
	} else {
		buf.WriteByte(0)
	}

	// Timestamp
	binary.Write(buf, binary.BigEndian, r.Timestamp)

	// Padding
	if len(r.Padding) > NegotiateMaxPaddingSize {
		return nil, ErrInvalidFrameSize
	}
	buf.WriteByte(byte(len(r.Padding)))
	buf.Write(r.Padding)

	// CRC16 Checksum
	data := buf.Bytes()
	checksum := internal.ComputeChecksumCRC16(data)
	binary.Write(buf, binary.BigEndian, checksum)

	result := buf.Bytes()
	if len(result) > NegotiateMaxFrameSize {
		return nil, ErrInvalidFrameSize
	}

	return result, nil
}

// DecodeNegotiateRequest decodes binary frame to NegotiateRequest.
func DecodeNegotiateRequest(data []byte) (*NegotiateRequest, error) {
	if len(data) < NegotiateMinFrameSize || len(data) > NegotiateMaxFrameSize {
		return nil, ErrInvalidFrameSize
	}

	buf := bytes.NewReader(data)

	// Magic
	magic, err := buf.ReadByte()
	if err != nil || magic != NegotiateMagicRequest {
		return nil, ErrInvalidMagic
	}

	// Version
	version, err := buf.ReadByte()
	if err != nil || version != NegotiateVersion {
		return nil, ErrInvalidVersion
	}

	// Channel Bitmap
	chCount, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	chBitmap := make([]byte, chCount)
	if _, err := buf.Read(chBitmap); err != nil {
		return nil, err
	}

	// Codec Bitmap
	codecCount, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	codecBitmap := make([]byte, codecCount)
	if _, err := buf.Read(codecBitmap); err != nil {
		return nil, err
	}

	// SessionNonce (8 bytes)
	nonce := make([]byte, NegotiateNonceSize)
	if _, err := buf.Read(nonce); err != nil {
		return nil, err
	}

	// HasSessionID flag
	hasSessionID, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	var sessionID []byte
	if hasSessionID == 1 {
		sessionID = make([]byte, NegotiateSessionIDSize)
		if _, err := buf.Read(sessionID); err != nil {
			return nil, err
		}
	}

	// Timestamp
	var timestamp uint32
	if err := binary.Read(buf, binary.BigEndian, &timestamp); err != nil {
		return nil, err
	}

	// Padding
	paddingLen, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	padding := make([]byte, paddingLen)
	if _, err := buf.Read(padding); err != nil {
		return nil, err
	}

	// Checksum
	var checksum uint16
	if err := binary.Read(buf, binary.BigEndian, &checksum); err != nil {
		return nil, err
	}

	// Verify checksum (exclude last 2 bytes)
	dataWithoutChecksum := data[:len(data)-2]
	expectedChecksum := internal.ComputeChecksumCRC16(dataWithoutChecksum)
	if checksum != expectedChecksum {
		return nil, ErrInvalidChecksum
	}

	// Verify timestamp (anti-replay)
	now := uint32(time.Now().Unix())
	age := now - timestamp
	if age > NegotiateMaxTimestampAge {
		return nil, ErrTimestampExpired
	}

	return &NegotiateRequest{
		ChannelBitmap: chBitmap,
		CodecBitmap:   codecBitmap,
		SessionNonce:  nonce,
		SessionID:     sessionID,
		Timestamp:     timestamp,
		Padding:       padding,
	}, nil
}

// Encode encodes NegotiateResponse to binary frame.
func (r *NegotiateResponse) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Magic + Version
	buf.WriteByte(NegotiateMagicResponse)
	buf.WriteByte(NegotiateVersion)

	// Channel Bitmap
	if len(r.ChannelBitmap) > 255 {
		return nil, ErrInvalidFrameSize
	}
	buf.WriteByte(byte(len(r.ChannelBitmap)))
	buf.Write(r.ChannelBitmap)

	// Codec Bitmap
	if len(r.CodecBitmap) > 255 {
		return nil, ErrInvalidFrameSize
	}
	buf.WriteByte(byte(len(r.CodecBitmap)))
	buf.Write(r.CodecBitmap)

	// SessionID (8 bytes)
	if len(r.SessionID) != NegotiateSessionIDSize {
		return nil, ErrSessionIDSize
	}
	buf.Write(r.SessionID)

	// Status
	buf.WriteByte(r.Status)

	// Padding
	if len(r.Padding) > NegotiateMaxPaddingSize {
		return nil, ErrInvalidFrameSize
	}
	buf.WriteByte(byte(len(r.Padding)))
	buf.Write(r.Padding)

	// CRC16 Checksum
	data := buf.Bytes()
	checksum := internal.ComputeChecksumCRC16(data)
	binary.Write(buf, binary.BigEndian, checksum)

	result := buf.Bytes()
	if len(result) > NegotiateMaxFrameSize {
		return nil, ErrInvalidFrameSize
	}

	return result, nil
}

// DecodeNegotiateResponse decodes binary frame to NegotiateResponse.
func DecodeNegotiateResponse(data []byte) (*NegotiateResponse, error) {
	if len(data) < NegotiateMinFrameSize || len(data) > NegotiateMaxFrameSize {
		return nil, ErrInvalidFrameSize
	}

	buf := bytes.NewReader(data)

	// Magic
	magic, err := buf.ReadByte()
	if err != nil || magic != NegotiateMagicResponse {
		return nil, ErrInvalidMagic
	}

	// Version
	version, err := buf.ReadByte()
	if err != nil || version != NegotiateVersion {
		return nil, ErrInvalidVersion
	}

	// Channel Bitmap
	chCount, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	chBitmap := make([]byte, chCount)
	if _, err := buf.Read(chBitmap); err != nil {
		return nil, err
	}

	// Codec Bitmap
	codecCount, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	codecBitmap := make([]byte, codecCount)
	if _, err := buf.Read(codecBitmap); err != nil {
		return nil, err
	}

	// SessionID (8 bytes)
	sessionID := make([]byte, NegotiateSessionIDSize)
	if _, err := buf.Read(sessionID); err != nil {
		return nil, err
	}

	// Status
	status, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	// Padding
	paddingLen, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	padding := make([]byte, paddingLen)
	if _, err := buf.Read(padding); err != nil {
		return nil, err
	}

	// Checksum
	var checksum uint16
	if err := binary.Read(buf, binary.BigEndian, &checksum); err != nil {
		return nil, err
	}

	// Verify checksum (exclude last 2 bytes)
	dataWithoutChecksum := data[:len(data)-2]
	expectedChecksum := internal.ComputeChecksumCRC16(dataWithoutChecksum)
	if checksum != expectedChecksum {
		return nil, ErrInvalidChecksum
	}

	return &NegotiateResponse{
		ChannelBitmap: chBitmap,
		CodecBitmap:   codecBitmap,
		SessionID:     sessionID,
		Status:        status,
		Padding:       padding,
	}, nil
}

// NewNegotiateResponse creates a new negotiation response.
// For first connection: generates SessionID from nonce.
// For subsequent connection: uses provided sessionID.
func NewNegotiateResponse(channelBitmap, codecBitmap []byte, sessionID []byte, status byte) (*NegotiateResponse, error) {
	// Generate random padding
	paddingLen := internal.RandomIntRange(0, 127)
	padding := make([]byte, paddingLen)
	if _, err := rand.Read(padding); err != nil {
		return nil, err
	}

	return &NegotiateResponse{
		ChannelBitmap: channelBitmap,
		CodecBitmap:   codecBitmap,
		SessionID:     sessionID,
		Status:        status,
		Padding:       padding,
	}, nil
}

// GenerateSessionID generates a new SessionID from nonce.
func GenerateSessionID(nonce []byte) []byte {
	hash := internal.ComputeDataHash(nonce)
	return hash[:NegotiateSessionIDSize]
}
