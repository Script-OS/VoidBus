// Package aes provides AES-GCM encryption codec implementation.
//
// This package implements the Codec and KeyAwareCodec interfaces using
// AES in Galois Counter Mode (GCM) for authenticated encryption.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.2):
// - AES-GCM provides confidentiality and integrity
// - Key length determines security level (128-bit: Medium, 256-bit: High)
// - Nonce is randomly generated per encryption (12 bytes for GCM)
// - Output format: nonce (12 bytes) + ciphertext + tag (16 bytes)
// - Key is obtained from KeyProvider, never hardcoded
//
// Security Notes:
// - Each encryption uses a unique random nonce
// - Nonce is prepended to ciphertext for transmission
// - Tag is appended automatically by GCM mode
// - Key MUST be 16 bytes (AES-128) or 32 bytes (AES-256)
package aes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/keyprovider"
)

const (
	// NonceSize is the size of GCM nonce in bytes (recommended: 12 bytes)
	NonceSize = 12

	// TagSize is the size of GCM authentication tag in bytes
	TagSize = 16

	// KeySize128 is the key size for AES-128 in bytes
	KeySize128 = 16

	// KeySize256 is the key size for AES-256 in bytes
	KeySize256 = 32

	// InternalID128 is the internal identifier for AES-128-GCM
	InternalID128 = "codec_aes_128_gcm"

	// InternalID256 is the internal identifier for AES-256-GCM
	InternalID256 = "codec_aes_256_gcm"

	// Algorithm128 is the algorithm identifier for AES-128-GCM
	Algorithm128 = "AES-128-GCM"

	// Algorithm256 is the algorithm identifier for AES-256-GCM
	Algorithm256 = "AES-256-GCM"
)

// Errors
var (
	// ErrInvalidKeySize indicates invalid key size
	ErrInvalidKeySize = errors.New("aes: invalid key size, must be 16 or 32 bytes")
	// ErrKeyNotSet indicates key provider not set
	ErrKeyNotSet = errors.New("aes: key provider not set")
	// ErrNonceGenerationFailed indicates nonce generation failed
	ErrNonceGenerationFailed = errors.New("aes: nonce generation failed")
	// ErrCiphertextTooShort indicates ciphertext is too short
	ErrCiphertextTooShort = errors.New("aes: ciphertext too short")
	// ErrDecryptFailed indicates decryption failed
	ErrDecryptFailed = errors.New("aes: decryption failed")
)

// AESCodec implements Codec and KeyAwareCodec for AES-GCM encryption.
type AESCodec struct {
	keyProvider keyprovider.KeyProvider
	keySize     int // 16 for AES-128, 32 for AES-256
	security    codec.SecurityLevel
	internalID  string
	algorithm   string
	code        string // User-defined code for chain hash, default "aes"
}

// NewAES128Codec creates a new AES-128-GCM codec instance.
//
// Security Level: Medium (AES-128)
// Key Size: 16 bytes
func NewAES128Codec() *AESCodec {
	return &AESCodec{
		keySize:     KeySize128,
		security:    codec.SecurityLevelMedium,
		internalID:  InternalID128,
		algorithm:   Algorithm128,
		keyProvider: nil,
		code:        "aes",
	}
}

// NewAES256Codec creates a new AES-256-GCM codec instance.
//
// Security Level: High (AES-256)
// Key Size: 32 bytes
func NewAES256Codec() *AESCodec {
	return &AESCodec{
		keySize:     KeySize256,
		security:    codec.SecurityLevelHigh,
		internalID:  InternalID256,
		algorithm:   Algorithm256,
		keyProvider: nil,
		code:        "aes",
	}
}

// SetCode sets a custom code for chain hash computation.
// This allows users to define their own code identifiers.
func (c *AESCodec) SetCode(code string) {
	c.code = code
}

// SetKeyProvider sets the key provider for this codec.
//
// This method implements KeyAwareCodec interface.
// The key provider must return a key of correct size (16 or 32 bytes).
func (c *AESCodec) SetKeyProvider(provider keyprovider.KeyProvider) error {
	if provider == nil {
		return codec.ErrInvalidKeyProvider
	}

	// Verify key is available and has correct size
	key, err := provider.GetKey()
	if err != nil {
		return err
	}

	if len(key) != c.keySize {
		return ErrInvalidKeySize
	}

	c.keyProvider = provider
	return nil
}

// RequiresKey returns true as AES codec requires a key.
func (c *AESCodec) RequiresKey() bool {
	return true
}

// KeyAlgorithm returns the required key algorithm.
func (c *AESCodec) KeyAlgorithm() string {
	return c.algorithm
}

// Encode encrypts data using AES-GCM.
//
// Process:
// 1. Get key from KeyProvider
// 2. Generate random 12-byte nonce
// 3. Encrypt with AES-GCM
// 4. Return nonce + ciphertext + tag
//
// Output Format:
//
//	[nonce (12 bytes)][ciphertext][tag (16 bytes)]
//
// The tag is automatically appended by GCM mode.
func (c *AESCodec) Encode(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, codec.ErrInvalidData
	}

	// Check key provider
	if c.keyProvider == nil {
		return nil, ErrKeyNotSet
	}

	// Get key
	key, err := c.keyProvider.GetKey()
	if err != nil {
		return nil, err
	}

	if len(key) != c.keySize {
		return nil, ErrInvalidKeySize
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, codec.ErrEncodingFailed
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, codec.ErrEncodingFailed
	}

	// Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, ErrNonceGenerationFailed
	}

	// Encrypt: Seal appends tag to ciphertext
	// Output: nonce + ciphertext + tag
	ciphertext := gcm.Seal(nil, nonce, data, nil)
	output := make([]byte, NonceSize+len(ciphertext))
	copy(output[:NonceSize], nonce)
	copy(output[NonceSize:], ciphertext)

	return output, nil
}

// Decode decrypts data using AES-GCM.
//
// Process:
// 1. Extract nonce (first 12 bytes)
// 2. Extract ciphertext + tag (remaining bytes)
// 3. Get key from KeyProvider
// 4. Decrypt with AES-GCM
// 5. Return plaintext
//
// Input Format:
//
//	[nonce (12 bytes)][ciphertext][tag (16 bytes)]
func (c *AESCodec) Decode(data []byte) ([]byte, error) {
	if len(data) < NonceSize+TagSize {
		return nil, ErrCiphertextTooShort
	}

	// Check key provider
	if c.keyProvider == nil {
		return nil, ErrKeyNotSet
	}

	// Get key
	key, err := c.keyProvider.GetKey()
	if err != nil {
		return nil, err
	}

	if len(key) != c.keySize {
		return nil, ErrInvalidKeySize
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, codec.ErrDecodingFailed
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, codec.ErrDecodingFailed
	}

	// Extract nonce and ciphertext
	nonce := data[:NonceSize]
	ciphertext := data[NonceSize:]

	// Decrypt: Open verifies tag and returns plaintext
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}

	return plaintext, nil
}

// InternalID returns the internal identifier.
func (c *AESCodec) InternalID() string {
	return c.internalID
}

// SecurityLevel returns the security level.
func (c *AESCodec) SecurityLevel() codec.SecurityLevel {
	return c.security
}

// Code returns the codec code for chain hash computation.
func (c *AESCodec) Code() string {
	if c.code == "" {
		return "aes"
	}
	return c.code
}

// AES128Module implements CodecModule for AES-128-GCM.
type AES128Module struct{}

// NewAES128Module creates a new AES-128-GCM module.
func NewAES128Module() *AES128Module {
	return &AES128Module{}
}

// Create creates an AES-128-GCM codec instance.
func (m *AES128Module) Create(args interface{}) (codec.Codec, error) {
	return NewAES128Codec(), nil
}

// InternalID returns the module's internal ID.
func (m *AES128Module) InternalID() string {
	return InternalID128
}

// SecurityLevel returns the module's security level.
func (m *AES128Module) SecurityLevel() codec.SecurityLevel {
	return codec.SecurityLevelMedium
}

// AES256Module implements CodecModule for AES-256-GCM.
type AES256Module struct{}

// NewAES256Module creates a new AES-256-GCM module.
func NewAES256Module() *AES256Module {
	return &AES256Module{}
}

// Create creates an AES-256-GCM codec instance.
func (m *AES256Module) Create(args interface{}) (codec.Codec, error) {
	return NewAES256Codec(), nil
}

// InternalID returns the module's internal ID.
func (m *AES256Module) InternalID() string {
	return InternalID256
}

// SecurityLevel returns the module's security level.
func (m *AES256Module) SecurityLevel() codec.SecurityLevel {
	return codec.SecurityLevelHigh
}

// init registers AES codec modules to global registry.
func init() {
	// Register AES-128-GCM
	if err := codec.Register(NewAES128Module()); err != nil {
		panic("failed to register AES-128-GCM codec: " + err.Error())
	}

	// Register AES-256-GCM
	if err := codec.Register(NewAES256Module()); err != nil {
		panic("failed to register AES-256-GCM codec: " + err.Error())
	}
}
