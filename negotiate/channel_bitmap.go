// Package negotiate provides channel bitmap definitions.
//
// Channel Bitmap uses bit positions to represent supported channel types.
// This provides compact binary encoding for negotiation protocol.
package negotiate

// ChannelBit represents a bit position in Channel Bitmap.
// Each bit corresponds to a specific channel type.
//
// Bitmap format (compact mapping, v3.0+):
// - Byte 0: Basic channels (WS, TCP, UDP, ICMP, DNS, HTTP, Reserved1, Reserved2)
//
// Example: Support WS + TCP + UDP
// Bitmap = 0b00000111 = []byte{0x07}
type ChannelBit int

const (
	// Basic Channels (Byte 0) - Compact mapping (v3.0+)
	// QUIC removed to simplify architecture
	ChannelBitWS        ChannelBit = 0 // WebSocket (default negotiation channel)
	ChannelBitTCP       ChannelBit = 1 // TCP
	ChannelBitUDP       ChannelBit = 2 // UDP (unreliable, needs ACK/NAK)
	ChannelBitICMP      ChannelBit = 3 // ICMP tunnel
	ChannelBitDNS       ChannelBit = 4 // DNS tunnel
	ChannelBitHTTP      ChannelBit = 5 // HTTP/HTTPS tunnel
	ChannelBitReserved  ChannelBit = 6 // Reserved
	ChannelBitReserved7 ChannelBit = 7 // Reserved

	// Reserved Channels (Byte 1)
	ChannelBitReserved8  ChannelBit = 8
	ChannelBitReserved9  ChannelBit = 9
	ChannelBitReserved10 ChannelBit = 10
	ChannelBitReserved11 ChannelBit = 11
	ChannelBitReserved12 ChannelBit = 12
	ChannelBitReserved13 ChannelBit = 13
	ChannelBitReserved14 ChannelBit = 14
	ChannelBitReserved15 ChannelBit = 15
)

// ChannelID represents a channel identifier.
type ChannelID int

const (
	ChannelIDWS        ChannelID = 0
	ChannelIDTCP       ChannelID = 1
	ChannelIDUDP       ChannelID = 2
	ChannelIDICMP      ChannelID = 3
	ChannelIDDNS       ChannelID = 4
	ChannelIDHTTP      ChannelID = 5
	ChannelIDReserved  ChannelID = 6
	ChannelIDReserved7 ChannelID = 7
)

// ChannelBitToID converts ChannelBit to ChannelID.
func ChannelBitToID(bit ChannelBit) ChannelID {
	return ChannelID(bit)
}

// ChannelIDToBit converts ChannelID to ChannelBit.
func ChannelIDToBit(id ChannelID) ChannelBit {
	return ChannelBit(id)
}

// ChannelBitmap represents a set of supported channels.
type ChannelBitmap []byte

// NewChannelBitmap creates a new ChannelBitmap with specified size.
// If size is 0, uses DefaultChannelBitmapSize.
func NewChannelBitmap(size int) ChannelBitmap {
	if size <= 0 {
		size = DefaultChannelBitmapSize
	}
	return ChannelBitmap(make([]byte, size))
}

// DefaultChannelBitmapSize is the default bitmap size (2 bytes for 16 channels).
const DefaultChannelBitmapSize = 2

// SetChannel sets a channel bit in the bitmap.
func (b ChannelBitmap) SetChannel(bit ChannelBit) {
	byteIndex := int(bit) / 8
	bitIndex := int(bit) % 8
	if byteIndex < len(b) {
		b[byteIndex] |= (1 << bitIndex)
	}
}

// ClearChannel clears a channel bit in the bitmap.
func (b ChannelBitmap) ClearChannel(bit ChannelBit) {
	byteIndex := int(bit) / 8
	bitIndex := int(bit) % 8
	if byteIndex < len(b) {
		b[byteIndex] &= byte(^(1 << bitIndex))
	}
}

// HasChannel checks if a channel bit is set.
func (b ChannelBitmap) HasChannel(bit ChannelBit) bool {
	byteIndex := int(bit) / 8
	bitIndex := int(bit) % 8
	if byteIndex >= len(b) {
		return false
	}
	return (b[byteIndex] & (1 << bitIndex)) != 0
}

// GetChannelIDs returns all set channel IDs in the bitmap.
func (b ChannelBitmap) GetChannelIDs() []ChannelID {
	ids := make([]ChannelID, 0)
	for bit := ChannelBitWS; bit <= ChannelBitReserved15; bit++ {
		if b.HasChannel(bit) {
			ids = append(ids, ChannelBitToID(bit))
		}
	}
	return ids
}

// ChannelBitmapFromIDs creates a ChannelBitmap from ChannelID list.
func ChannelBitmapFromIDs(ids []ChannelID, size int) ChannelBitmap {
	bitmap := NewChannelBitmap(size)
	for _, id := range ids {
		bitmap.SetChannel(ChannelIDToBit(id))
	}
	return bitmap
}

// IntersectChannelBitmaps computes the intersection of two ChannelBitmaps.
// Returns a new bitmap with bits set only where both inputs have the bit set.
func IntersectChannelBitmaps(a, b ChannelBitmap) ChannelBitmap {
	result := NewChannelBitmap(min(len(a), len(b)))
	for i := 0; i < len(result); i++ {
		result[i] = a[i] & b[i]
	}
	return result
}

// IsChannelBitmapEmpty checks if the bitmap has no bits set.
func IsChannelBitmapEmpty(b ChannelBitmap) bool {
	for _, byteVal := range b {
		if byteVal != 0 {
			return false
		}
	}
	return true
}

// ChannelCount returns the number of channels set in the bitmap.
func ChannelCount(b ChannelBitmap) int {
	count := 0
	for _, byteVal := range b {
		for i := 0; i < 8; i++ {
			if (byteVal & (1 << i)) != 0 {
				count++
			}
		}
	}
	return count
}

// IsReliable checks if a channel is reliable (TCP, WS, HTTP).
// Unreliable channels (UDP, ICMP, DNS) need ACK/NAK mechanism at VoidBus level.
func (b ChannelBitmap) IsReliable(bit ChannelBit) bool {
	switch bit {
	case ChannelBitTCP, ChannelBitWS, ChannelBitHTTP:
		return true
	case ChannelBitUDP, ChannelBitICMP, ChannelBitDNS:
		return false
	default:
		return true // Assume reliable for unknown/reserved channels
	}
}

// GetReliableChannels returns IDs of reliable channels in the bitmap.
func (b ChannelBitmap) GetReliableChannels() []ChannelID {
	ids := make([]ChannelID, 0)
	for bit := ChannelBitWS; bit <= ChannelBitReserved; bit++ {
		if b.HasChannel(bit) && b.IsReliable(bit) {
			ids = append(ids, ChannelBitToID(bit))
		}
	}
	return ids
}

// GetUnreliableChannels returns IDs of unreliable channels in the bitmap.
func (b ChannelBitmap) GetUnreliableChannels() []ChannelID {
	ids := make([]ChannelID, 0)
	for bit := ChannelBitWS; bit <= ChannelBitReserved; bit++ {
		if b.HasChannel(bit) && !b.IsReliable(bit) {
			ids = append(ids, ChannelBitToID(bit))
		}
	}
	return ids
}
