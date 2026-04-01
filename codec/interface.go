// Package codec defines the Codec interface for encoding/decoding operations.
//
// Codec is responsible for encoding/encryption and decoding/decryption.
// Unlike Serializer, Codec MUST NOT be exposed in metadata protocols.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.2):
// - Codec MUST NOT handle data serialization
// - Codec MUST NOT handle data transmission
// - Codec MUST NOT handle data fragmentation
// - Codec.InternalID() MUST NOT be transmitted
// - Codec.SecurityLevel() is used for negotiation (value only, not name)
package codec

import (
	"errors"

	"github.com/Script-OS/VoidBus/keyprovider"
)

// SecurityLevel defines the security level of a codec.
type SecurityLevel int

const (
	// SecurityLevelNone indicates no security (plaintext).
	SecurityLevelNone SecurityLevel = 0

	// SecurityLevelLow indicates low security level (e.g., Base64).
	SecurityLevelLow SecurityLevel = 1

	// SecurityLevelMedium indicates medium security level (e.g., AES-128).
	SecurityLevelMedium SecurityLevel = 2

	// SecurityLevelHigh indicates high security level (e.g., AES-256, RSA).
	SecurityLevelHigh SecurityLevel = 3

	// MaxCodecDepth is the maximum depth of codec chain.
	MaxCodecDepth = 5
)

// String returns the string representation of SecurityLevel.
func (s SecurityLevel) String() string {
	switch s {
	case SecurityLevelNone:
		return "none"
	case SecurityLevelLow:
		return "low"
	case SecurityLevelMedium:
		return "medium"
	case SecurityLevelHigh:
		return "high"
	default:
		return "unknown"
	}
}

// Common codec errors
var (
	ErrInvalidData        = errors.New("codec: invalid data")
	ErrKeyRequired        = errors.New("codec: key required")
	ErrInvalidKey         = errors.New("codec: invalid key")
	ErrInvalidKeyProvider = errors.New("codec: invalid key provider")
	ErrKeyIncompatible    = errors.New("codec: key incompatible")
	ErrEncodingFailed     = errors.New("codec: encoding failed")
	ErrDecodingFailed     = errors.New("codec: decoding failed")
	ErrCodecNotFound      = errors.New("codec: not found")
	ErrCodecConflict      = errors.New("codec: codec conflict")
	ErrCodecChainMismatch = errors.New("codec: chain hash mismatch")
	// Note: ErrChainTooLong and ErrInvalidIndex are defined in chain.go
)

// Codec is the core interface for encoding/decoding operations.
type Codec interface {
	// Encode encodes/encrypts the data.
	Encode(data []byte) ([]byte, error)

	// Decode decodes/decrypts the data.
	Decode(data []byte) ([]byte, error)

	// Code returns the codec identifier for chain hash computation.
	// This code is user-defined and MUST be consistent between sender and receiver.
	// The code is used in the chain hash: Hash = SHA256("code1|code2|code3")
	//
	// Standard codes (for bitmap mapping):
	//   - "plain" or "p" - Plain codec
	//   - "base64" or "b" - Base64 codec
	//   - "aes" or "a" - AES-256-GCM codec
	//   - "xor" or "x" - XOR codec
	//   - "chacha" or "c" - ChaCha20 codec
	//   - "rsa" or "r" - RSA codec
	//   - "gzip" or "g" - GZIP codec
	//   - "zstd" or "z" - ZSTD codec
	//
	// Users can also use custom codes, but they won't be mapped to standard bitmap bits.
	Code() string

	// InternalID returns the internal identifier (NOT for transmission).
	// Deprecated: Use Code() for hash computation.
	InternalID() string

	// SecurityLevel returns the security level.
	SecurityLevel() SecurityLevel
}

// KeyAwareCodec is an extension interface for codecs that require keys.
type KeyAwareCodec interface {
	Codec

	// SetKeyProvider sets the key provider for this codec.
	SetKeyProvider(provider keyprovider.KeyProvider) error

	// RequiresKey returns whether this codec requires a key.
	RequiresKey() bool

	// KeyAlgorithm returns the required key algorithm type.
	KeyAlgorithm() string
}

// CodecModule is the interface for codec module registration.
type CodecModule interface {
	// Create creates a codec instance.
	Create(args interface{}) (Codec, error)

	// InternalID returns the module's internal ID.
	InternalID() string

	// SecurityLevel returns the module's security level.
	SecurityLevel() SecurityLevel
}
