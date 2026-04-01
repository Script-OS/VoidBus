// Package negotiate provides negotiation protocol for VoidBus.
//
// Negotiation uses bitmap-based protocol for exchanging supported
// channels and codecs between client and server.
//
// Design Constraints:
// - Negotiation data MUST NOT use plaintext (use bitmap)
// - Negotiation MUST NOT use VoidBus Header format
// - Channel types MUST NOT be exposed in normal transmission
// - Codec IDs CAN be exposed during negotiation
package negotiate

// CodecBit represents a bit position in Codec Bitmap.
// Each bit corresponds to a specific codec type.
//
// Bitmap format:
// - Byte 0: Basic codecs (Plain, Base64, AES, XOR, ChaCha20, RSA, GZIP, ZSTD)
// - Byte 1: Reserved for future extensions
//
// Example: Support Plain + AES-256-GCM + ChaCha20
// Bitmap = 0b00010101 = []byte{0x15}
type CodecBit int

const (
	// Basic Codecs (Byte 0)
	CodecBitPlain    CodecBit = 0 // PlainCodec (no transformation)
	CodecBitBase64   CodecBit = 1 // Base64Codec
	CodecBitAES256   CodecBit = 2 // AES-256-GCM
	CodecBitXOR      CodecBit = 3 // XORCodec
	CodecBitChaCha20 CodecBit = 4 // ChaCha20-Poly1305
	CodecBitRSA      CodecBit = 5 // RSACodec (asymmetric)
	CodecBitGZIP     CodecBit = 6 // GZIP compression
	CodecBitZSTD     CodecBit = 7 // ZSTD compression

	// Reserved Codecs (Byte 1)
	CodecBitReserved8  CodecBit = 8
	CodecBitReserved9  CodecBit = 9
	CodecBitReserved10 CodecBit = 10
	CodecBitReserved11 CodecBit = 11
	CodecBitReserved12 CodecBit = 12
	CodecBitReserved13 CodecBit = 13
	CodecBitReserved14 CodecBit = 14
	CodecBitReserved15 CodecBit = 15
)

// CodecID represents a codec identifier.
type CodecID int

const (
	CodecIDPlain    CodecID = 0
	CodecIDBase64   CodecID = 1
	CodecIDAES256   CodecID = 2
	CodecIDXOR      CodecID = 3
	CodecIDChaCha20 CodecID = 4
	CodecIDRSA      CodecID = 5
	CodecIDGZIP     CodecID = 6
	CodecIDZSTD     CodecID = 7
)

// CodecBitToID converts CodecBit to CodecID.
func CodecBitToID(bit CodecBit) CodecID {
	return CodecID(bit)
}

// CodecIDToBit converts CodecID to CodecBit.
func CodecIDToBit(id CodecID) CodecBit {
	return CodecBit(id)
}

// CodecBitmap represents a set of supported codecs.
type CodecBitmap []byte

// NewCodecBitmap creates a new CodecBitmap with specified size.
// If size is 0, uses DefaultCodecBitmapSize.
func NewCodecBitmap(size int) CodecBitmap {
	if size <= 0 {
		size = DefaultCodecBitmapSize
	}
	return CodecBitmap(make([]byte, size))
}

// DefaultCodecBitmapSize is the default bitmap size (2 bytes for 16 codecs).
const DefaultCodecBitmapSize = 2

// SetCodec sets a codec bit in the bitmap.
func (b CodecBitmap) SetCodec(bit CodecBit) {
	byteIndex := int(bit) / 8
	bitIndex := int(bit) % 8
	if byteIndex < len(b) {
		b[byteIndex] |= (1 << bitIndex)
	}
}

// ClearCodec clears a codec bit in the bitmap.
func (b CodecBitmap) ClearCodec(bit CodecBit) {
	byteIndex := int(bit) / 8
	bitIndex := int(bit) % 8
	if byteIndex < len(b) {
		b[byteIndex] &= byte(^(1 << bitIndex))
	}
}

// HasCodec checks if a codec bit is set.
func (b CodecBitmap) HasCodec(bit CodecBit) bool {
	byteIndex := int(bit) / 8
	bitIndex := int(bit) % 8
	if byteIndex >= len(b) {
		return false
	}
	return (b[byteIndex] & (1 << bitIndex)) != 0
}

// GetCodecIDs returns all set codec IDs in the bitmap.
func (b CodecBitmap) GetCodecIDs() []CodecID {
	ids := make([]CodecID, 0)
	for bit := CodecBitPlain; bit <= CodecBitReserved15; bit++ {
		if b.HasCodec(bit) {
			ids = append(ids, CodecBitToID(bit))
		}
	}
	return ids
}

// CodecBitmapFromIDs creates a CodecBitmap from CodecID list.
func CodecBitmapFromIDs(ids []CodecID, size int) CodecBitmap {
	bitmap := NewCodecBitmap(size)
	for _, id := range ids {
		bitmap.SetCodec(CodecIDToBit(id))
	}
	return bitmap
}

// IntersectCodecBitmaps computes the intersection of two CodecBitmaps.
// Returns a new bitmap with bits set only where both inputs have the bit set.
func IntersectCodecBitmaps(a, b CodecBitmap) CodecBitmap {
	result := NewCodecBitmap(min(len(a), len(b)))
	for i := 0; i < len(result); i++ {
		result[i] = a[i] & b[i]
	}
	return result
}

// IsCodecBitmapEmpty checks if the bitmap has no bits set.
func IsCodecBitmapEmpty(b CodecBitmap) bool {
	for _, byteVal := range b {
		if byteVal != 0 {
			return false
		}
	}
	return true
}

// CodecCount returns the number of codecs set in the bitmap.
func CodecCount(b CodecBitmap) int {
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

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
