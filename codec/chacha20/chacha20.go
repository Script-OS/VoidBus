// Package chacha20 provides a ChaCha20-Poly1305 encryption codec implementation.
//
// ChaCha20 codec performs authenticated encryption using ChaCha20-Poly1305 AEAD.
// It is primarily used for:
// - High-performance encryption on software-only platforms
// - Mobile devices where AES hardware acceleration is limited
// - Secure communication requiring both confidentiality and integrity
//
// Security Notes (see docs/ARCHITECTURE.md §2.1.2):
// - ChaCha20-Poly1305 provides confidentiality and integrity
// - SecurityLevelHigh (3) - suitable for production use
// - Key MUST be 32 bytes (256-bit)
// - Nonce is randomly generated per encryption (12 bytes)
// - Output format: nonce (12 bytes) + ciphertext + tag (16 bytes)
// - Key is obtained from KeyProvider, never hardcoded
package chacha20

import (
	"crypto/rand"
	"errors"
	"io"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/keyprovider"
)

const (
	// KeySize is the required key size in bytes (256-bit)
	KeySize = 32

	// NonceSize is the size of ChaCha20-Poly1305 nonce in bytes
	NonceSize = 12

	// TagSize is the size of Poly1305 authentication tag in bytes
	TagSize = 16

	// InternalIDValue is the internal identifier for ChaCha20-Poly1305
	InternalIDValue = "chacha20_poly1305"

	// Algorithm is the algorithm identifier
	Algorithm = "ChaCha20-Poly1305"
)

// Errors
var (
	// ErrInvalidKeySize indicates invalid key size
	ErrInvalidKeySize = errors.New("chacha20: invalid key size, must be 32 bytes")
	// ErrKeyNotSet indicates key provider not set
	ErrKeyNotSet = errors.New("chacha20: key provider not set")
	// ErrNonceGenerationFailed indicates nonce generation failed
	ErrNonceGenerationFailed = errors.New("chacha20: nonce generation failed")
	// ErrCiphertextTooShort indicates ciphertext is too short
	ErrCiphertextTooShort = errors.New("chacha20: ciphertext too short")
	// ErrDecryptFailed indicates decryption failed
	ErrDecryptFailed = errors.New("chacha20: decryption failed")
)

// Codec implements the codec.Codec and codec.KeyAwareCodec interfaces for ChaCha20-Poly1305 encryption.
type Codec struct {
	code        string // User-defined code identifier
	keyProvider keyprovider.KeyProvider
}

// New creates a new ChaCha20-Poly1305 codec instance.
func New() *Codec {
	return &Codec{code: "chacha20"}
}

// Code implements codec.Codec.Code.
// Returns the user-defined code identifier for this codec.
func (c *Codec) Code() string {
	if c.code == "" {
		return "chacha20" // Default value
	}
	return c.code
}

// SetCode sets a custom code identifier for this codec.
func (c *Codec) SetCode(code string) {
	c.code = code
}

// Encode implements codec.Codec.Encode.
// Encrypts data using ChaCha20-Poly1305 with the key.
//
// Parameter Constraints:
//   - data: can be nil or any byte slice
//
// Return Guarantees:
//   - On success: returns nonce (12 bytes) + ciphertext + tag (16 bytes)
//   - Output length = 12 + len(data) + 16
//
// Error Types:
//   - ErrKeyNotSet: key provider not configured
//   - codec.ErrKeyRequired: key not available from provider
//   - ErrInvalidKeySize: key is not 32 bytes
//   - ErrNonceGenerationFailed: failed to generate nonce
func (c *Codec) Encode(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}

	key, err := c.getKey()
	if err != nil {
		return nil, err
	}

	// Create AEAD cipher
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}

	// Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, ErrNonceGenerationFailed
	}

	// Encrypt: nonce + ciphertext + tag
	ciphertext := aead.Seal(nil, nonce, data, nil)

	// Prepend nonce to ciphertext
	result := make([]byte, 0, NonceSize+len(ciphertext))
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// Decode implements codec.Codec.Decode.
// Decrypts data using ChaCha20-Poly1305 with the key.
//
// Parameter Constraints:
//   - data: MUST be nonce (12 bytes) + ciphertext + tag (16 bytes)
//
// Return Guarantees:
//   - On success: returns original plaintext data
//
// Error Types:
//   - ErrKeyNotSet: key provider not configured
//   - codec.ErrKeyRequired: key not available from provider
//   - ErrCiphertextTooShort: data is too short to contain nonce + tag
//   - ErrDecryptFailed: decryption or authentication failed
func (c *Codec) Decode(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}

	// Check minimum length: nonce + tag
	minLen := NonceSize + TagSize
	if len(data) < minLen {
		return nil, ErrCiphertextTooShort
	}

	key, err := c.getKey()
	if err != nil {
		return nil, err
	}

	// Create AEAD cipher
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}

	// Extract nonce and ciphertext
	nonce := data[:NonceSize]
	ciphertext := data[NonceSize:]

	// Decrypt
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}

	return plaintext, nil
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

	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}

	return key, nil
}

// InternalID implements codec.Codec.InternalID.
// Returns "chacha20_poly1305".
func (c *Codec) InternalID() string {
	return InternalIDValue
}

// SecurityLevel implements codec.Codec.SecurityLevel.
// Returns SecurityLevelHigh (3) - authenticated encryption.
func (c *Codec) SecurityLevel() codec.SecurityLevel {
	return codec.SecurityLevelHigh
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
// Returns true - ChaCha20 codec requires a key.
func (c *Codec) RequiresKey() bool {
	return true
}

// KeyAlgorithm implements codec.KeyAwareCodec.KeyAlgorithm.
// Returns "ChaCha20-Poly1305" as the key algorithm.
func (c *Codec) KeyAlgorithm() string {
	return Algorithm
}

// Module implements codec.CodecModule for registration.
type Module struct{}

// NewModule creates a new ChaCha20-Poly1305 codec module.
func NewModule() *Module {
	return &Module{}
}

// Create implements codec.CodecModule.Create.
// Creates a new ChaCha20-Poly1305 codec instance.
func (m *Module) Create(args interface{}) (codec.Codec, error) {
	return New(), nil
}

// InternalID implements codec.CodecModule.InternalID.
// Returns "chacha20_poly1305".
func (m *Module) InternalID() string {
	return InternalIDValue
}

// SecurityLevel implements codec.CodecModule.SecurityLevel.
// Returns SecurityLevelHigh (3).
func (m *Module) SecurityLevel() codec.SecurityLevel {
	return codec.SecurityLevelHigh
}

// GenerateKey generates a random ChaCha20 key (32 bytes).
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}
