// Package xor provides a simple XOR encryption codec implementation.
//
// XOR codec performs byte-wise XOR operation with a key for encryption/decryption.
// It is primarily used for:
// - Simple obfuscation of data
// - Testing and educational purposes
// - Combined with other codecs for layered encryption
//
// Security Warning (see docs/ARCHITECTURE.md §2.1.2):
// - XOR codec has SecurityLevelMedium (2)
// - XOR encryption is symmetric and relatively weak
// - Key is repeated if data is longer than key
// - Suitable for obfuscation, not for strong encryption
// - Recommended to use AES or ChaCha20 for production
package xor

import (
	"crypto/rand"
	"errors"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/keyprovider"
)

const (
	// InternalID is the unique identifier for the xor codec
	InternalID = "xor"

	// MinKeySize is the minimum key size in bytes
	MinKeySize = 8

	// MaxKeySize is the maximum key size in bytes
	MaxKeySize = 256

	// DefaultKeySize is the default key size in bytes
	DefaultKeySize = 32
)

// Errors
var (
	// ErrInvalidKeySize indicates invalid key size
	ErrInvalidKeySize = errors.New("xor: invalid key size, must be between 8 and 256 bytes")
	// ErrKeyNotSet indicates key provider not set
	ErrKeyNotSet = errors.New("xor: key provider not set")
	// ErrKeyTooShort indicates key is too short
	ErrKeyTooShort = errors.New("xor: key too short")
)

// Codec implements the codec.Codec and codec.KeyAwareCodec interfaces with XOR encryption.
type Codec struct {
	keyProvider keyprovider.KeyProvider
	keySize     int
}

// New creates a new XOR codec instance with default key size.
func New() *Codec {
	return &Codec{
		keySize: DefaultKeySize,
	}
}

// NewWithKeySize creates a new XOR codec instance with specified key size.
func NewWithKeySize(keySize int) (*Codec, error) {
	if keySize < MinKeySize || keySize > MaxKeySize {
		return nil, ErrInvalidKeySize
	}
	return &Codec{
		keySize: keySize,
	}, nil
}

// Encode implements codec.Codec.Encode.
// Encrypts data using XOR with the key.
//
// Parameter Constraints:
//   - data: can be nil or any byte slice
//
// Return Guarantees:
//   - On success: returns XOR encrypted data
//   - Output length equals input length
//
// Error Types:
//   - ErrKeyNotSet: key provider not configured
//   - codec.ErrKeyRequired: key not available from provider
func (c *Codec) Encode(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}

	key, err := c.getKey()
	if err != nil {
		return nil, err
	}

	return c.xorBytes(data, key), nil
}

// Decode implements codec.Codec.Decode.
// Decrypts data using XOR with the key (same operation as Encode).
//
// Parameter Constraints:
//   - data: MUST be XOR encrypted data
//
// Return Guarantees:
//   - On success: returns original data
//   - XOR is symmetric, so Decode = Encode
//
// Error Types:
//   - ErrKeyNotSet: key provider not configured
//   - codec.ErrKeyRequired: key not available from provider
func (c *Codec) Decode(data []byte) ([]byte, error) {
	// XOR is symmetric: encoding and decoding are the same operation
	return c.Encode(data)
}

// xorBytes performs XOR operation on data with key.
// Key is repeated if data is longer than key.
func (c *Codec) xorBytes(data, key []byte) []byte {
	if len(key) == 0 {
		return data
	}

	result := make([]byte, len(data))
	keyLen := len(key)

	for i, b := range data {
		result[i] = b ^ key[i%keyLen]
	}

	return result
}

// getKey retrieves the key from the key provider.
func (c *Codec) getKey() ([]byte, error) {
	if c.keyProvider == nil {
		return nil, ErrKeyNotSet
	}

	key, err := c.keyProvider.GetKey()
	if err != nil {
		return nil, codec.ErrKeyRequired
	}

	if len(key) < MinKeySize {
		return nil, ErrKeyTooShort
	}

	return key, nil
}

// InternalID implements codec.Codec.InternalID.
// Returns "xor".
func (c *Codec) InternalID() string {
	return InternalID
}

// SecurityLevel implements codec.Codec.SecurityLevel.
// Returns SecurityLevelMedium (2) - simple encryption.
func (c *Codec) SecurityLevel() codec.SecurityLevel {
	return codec.SecurityLevelMedium
}

// SetKeyProvider implements codec.KeyAwareCodec.SetKeyProvider.
// Sets the key provider for this codec.
func (c *Codec) SetKeyProvider(provider keyprovider.KeyProvider) error {
	if provider == nil {
		return codec.ErrInvalidKeyProvider
	}
	c.keyProvider = provider
	return nil
}

// RequiresKey implements codec.KeyAwareCodec.RequiresKey.
// Returns true - XOR codec requires a key.
func (c *Codec) RequiresKey() bool {
	return true
}

// KeyAlgorithm implements codec.KeyAwareCodec.KeyAlgorithm.
// Returns "XOR" as the key algorithm.
func (c *Codec) KeyAlgorithm() string {
	return "XOR"
}

// Module implements codec.CodecModule for registration.
type Module struct {
	keySize int
}

// NewModule creates a new XOR codec module with default key size.
func NewModule() *Module {
	return &Module{
		keySize: DefaultKeySize,
	}
}

// NewModuleWithKeySize creates a new XOR codec module with specified key size.
func NewModuleWithKeySize(keySize int) (*Module, error) {
	if keySize < MinKeySize || keySize > MaxKeySize {
		return nil, ErrInvalidKeySize
	}
	return &Module{
		keySize: keySize,
	}, nil
}

// Create implements codec.CodecModule.Create.
// Creates a new XOR codec instance.
func (m *Module) Create(args interface{}) (codec.Codec, error) {
	return NewWithKeySize(m.keySize)
}

// InternalID implements codec.CodecModule.InternalID.
// Returns "xor".
func (m *Module) InternalID() string {
	return InternalID
}

// SecurityLevel implements codec.CodecModule.SecurityLevel.
// Returns SecurityLevelMedium (2).
func (m *Module) SecurityLevel() codec.SecurityLevel {
	return codec.SecurityLevelMedium
}

// GenerateKey generates a random XOR key of the specified size.
func GenerateKey(size int) ([]byte, error) {
	if size < MinKeySize || size > MaxKeySize {
		return nil, ErrInvalidKeySize
	}

	key := make([]byte, size)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	return key, nil
}
