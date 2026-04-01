// Package rsa provides RSA encryption codec implementation.
//
// RSA codec performs asymmetric encryption using RSA-OAEP.
// It is primarily used for:
// - Key exchange and distribution
// - Digital signatures (when combined with hash)
// - Encrypting small amounts of data (symmetric keys)
//
// Security Notes (see docs/ARCHITECTURE.md §2.1.2):
// - RSA-OAEP provides confidentiality with SHA-256 hash
// - SecurityLevelHigh (3) - suitable for production use
// - Key size MUST be at least 2048 bits (256 bytes)
// - Maximum plaintext size = key_size - 2*hash_size - 2
// - For 2048-bit key with SHA-256: max ~190 bytes
// - For larger data, use hybrid encryption (RSA + AES/ChaCha20)
//
// Usage Note:
// - RSA is NOT suitable for encrypting large data
// - Use RSA to encrypt a symmetric key, then use AES/ChaCha20 for data
// - This codec is designed for key exchange scenarios
package rsa

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"io"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/keyprovider"
)

const (
	// MinKeySize is the minimum RSA key size in bits (2048 bits)
	MinKeySize = 2048

	// DefaultKeySize is the default RSA key size in bits (2048 bits)
	DefaultKeySize = 2048

	// RecommendedKeySize is the recommended RSA key size in bits (4096 bits)
	RecommendedKeySize = 4096

	// InternalIDValue is the internal identifier for RSA-OAEP
	InternalIDValue = "rsa_oaep_sha256"

	// Algorithm is the algorithm identifier
	Algorithm = "RSA-OAEP-SHA256"
)

// Errors
var (
	// ErrKeyNotSet indicates key provider not set
	ErrKeyNotSet = errors.New("rsa: key provider not set")
	// ErrInvalidKeyType indicates invalid key type
	ErrInvalidKeyType = errors.New("rsa: invalid key type")
	// ErrPlaintextTooLong indicates plaintext is too long
	ErrPlaintextTooLong = errors.New("rsa: plaintext too long for key size")
	// ErrDecryptFailed indicates decryption failed
	ErrDecryptFailed = errors.New("rsa: decryption failed")
	// ErrNoPublicKey indicates public key not available
	ErrNoPublicKey = errors.New("rsa: public key not available")
	// ErrNoPrivateKey indicates private key not available
	ErrNoPrivateKey = errors.New("rsa: private key not available")
)

// Codec implements the codec.Codec and codec.KeyAwareCodec interfaces for RSA-OAEP encryption.
type Codec struct {
	keyProvider keyprovider.KeyProvider
	publicKey   *rsa.PublicKey
	privateKey  *rsa.PrivateKey
}

// New creates a new RSA-OAEP codec instance.
func New() *Codec {
	return &Codec{}
}

// NewWithKeys creates a new RSA codec with explicit keys.
func NewWithKeys(publicKey *rsa.PublicKey, privateKey *rsa.PrivateKey) *Codec {
	return &Codec{
		publicKey:  publicKey,
		privateKey: privateKey,
	}
}

// Encode implements codec.Codec.Encode.
// Encrypts data using RSA-OAEP with the public key.
//
// Parameter Constraints:
//   - data: maximum size depends on key size (for 2048-bit: ~190 bytes)
//
// Return Guarantees:
//   - On success: returns ciphertext (same size as key in bytes)
//   - Output length = key_size / 8 (e.g., 256 bytes for 2048-bit key)
//
// Error Types:
//   - ErrKeyNotSet: no public key available
//   - ErrPlaintextTooLong: data exceeds maximum size
//   - ErrDecryptFailed: encryption failed
func (c *Codec) Encode(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}

	pubKey, err := c.getPublicKey()
	if err != nil {
		return nil, err
	}

	// Check maximum plaintext size
	maxSize := pubKey.Size() - 2*sha256.Size - 2
	if len(data) > maxSize {
		return nil, ErrPlaintextTooLong
	}

	// Encrypt with OAEP using SHA-256
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pubKey, data, nil)
	if err != nil {
		return nil, err
	}

	return ciphertext, nil
}

// Decode implements codec.Codec.Decode.
// Decrypts data using RSA-OAEP with the private key.
//
// Parameter Constraints:
//   - data: MUST be ciphertext encrypted with matching public key
//
// Return Guarantees:
//   - On success: returns original plaintext
//
// Error Types:
//   - ErrNoPrivateKey: no private key available
//   - ErrDecryptFailed: decryption failed
func (c *Codec) Decode(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}

	privKey, err := c.getPrivateKey()
	if err != nil {
		return nil, err
	}

	// Decrypt with OAEP using SHA-256
	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privKey, data, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}

	return plaintext, nil
}

// getPublicKey returns the public key.
func (c *Codec) getPublicKey() (*rsa.PublicKey, error) {
	if c.publicKey != nil {
		return c.publicKey, nil
	}

	return nil, ErrNoPublicKey
}

// getPrivateKey returns the private key.
func (c *Codec) getPrivateKey() (*rsa.PrivateKey, error) {
	if c.privateKey != nil {
		return c.privateKey, nil
	}

	return nil, ErrNoPrivateKey
}

// InternalID implements codec.Codec.InternalID.
// Returns "rsa_oaep_sha256".
func (c *Codec) InternalID() string {
	return InternalIDValue
}

// SecurityLevel implements codec.Codec.SecurityLevel.
// Returns SecurityLevelHigh (3) - asymmetric encryption.
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

// SetPublicKey sets the public key directly.
func (c *Codec) SetPublicKey(key *rsa.PublicKey) error {
	if key == nil {
		return ErrInvalidKeyType
	}
	c.publicKey = key
	return nil
}

// SetPrivateKey sets the private key directly.
func (c *Codec) SetPrivateKey(key *rsa.PrivateKey) error {
	if key == nil {
		return ErrInvalidKeyType
	}
	c.privateKey = key
	// Also set public key from private key
	c.publicKey = &key.PublicKey
	return nil
}

// RequiresKey implements codec.KeyAwareCodec.RequiresKey.
// Returns true - RSA codec requires keys.
func (c *Codec) RequiresKey() bool {
	return true
}

// KeyAlgorithm implements codec.KeyAwareCodec.KeyAlgorithm.
// Returns "RSA-OAEP-SHA256" as the key algorithm.
func (c *Codec) KeyAlgorithm() string {
	return Algorithm
}

// MaxPlaintextSize returns the maximum plaintext size for encryption.
func (c *Codec) MaxPlaintextSize() (int, error) {
	pubKey, err := c.getPublicKey()
	if err != nil {
		return 0, err
	}
	return pubKey.Size() - 2*sha256.Size - 2, nil
}

// Module implements codec.CodecModule for registration.
type Module struct{}

// NewModule creates a new RSA codec module.
func NewModule() *Module {
	return &Module{}
}

// Create implements codec.CodecModule.Create.
// Creates a new RSA codec instance.
func (m *Module) Create(args interface{}) (codec.Codec, error) {
	return New(), nil
}

// InternalID implements codec.CodecModule.InternalID.
// Returns "rsa_oaep_sha256".
func (m *Module) InternalID() string {
	return InternalIDValue
}

// SecurityLevel implements codec.CodecModule.SecurityLevel.
// Returns SecurityLevelHigh (3).
func (m *Module) SecurityLevel() codec.SecurityLevel {
	return codec.SecurityLevelHigh
}

// GenerateKey generates a new RSA key pair.
func GenerateKey(bits int) (*rsa.PrivateKey, error) {
	if bits < MinKeySize {
		bits = DefaultKeySize
	}

	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}

	return key, nil
}

// GenerateKeyReader generates a new RSA key pair using custom reader.
func GenerateKeyReader(bits int, r io.Reader) (*rsa.PrivateKey, error) {
	if bits < MinKeySize {
		bits = DefaultKeySize
	}

	key, err := rsa.GenerateKey(r, bits)
	if err != nil {
		return nil, err
	}

	return key, nil
}
