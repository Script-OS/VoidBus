// Package codec defines the Codec interface and registry for data
// encoding/decoding operations.
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

	"VoidBus/keyprovider"
)

// SecurityLevel defines the security level of a codec.
// Higher values indicate stronger security.
type SecurityLevel int

const (
	// SecurityLevelNone indicates no security (plaintext).
	// ONLY allowed in debug mode, MUST NOT be used in release builds.
	SecurityLevelNone SecurityLevel = 0

	// SecurityLevelLow indicates low security level.
	// Used for encoding-only operations like Base64.
	SecurityLevelLow SecurityLevel = 1

	// SecurityLevelMedium indicates medium security level.
	// Used for symmetric encryption like AES-128.
	SecurityLevelMedium SecurityLevel = 2

	// SecurityLevelHigh indicates high security level.
	// Used for strong encryption like AES-256, RSA-2048+.
	SecurityLevelHigh SecurityLevel = 3
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
	// ErrInvalidData indicates the input data is invalid
	ErrInvalidData = errors.New("codec: invalid data")
	// ErrKeyRequired indicates a key is required but not set
	ErrKeyRequired = errors.New("codec: key required")
	// ErrInvalidKey indicates the provided key is invalid
	ErrInvalidKey = errors.New("codec: invalid key")
	// ErrInvalidKeyProvider indicates the key provider is invalid
	ErrInvalidKeyProvider = errors.New("codec: invalid key provider")
	// ErrKeyIncompatible indicates the key type is incompatible
	ErrKeyIncompatible = errors.New("codec: key incompatible")
	// ErrEncodingFailed indicates the encoding process failed
	ErrEncodingFailed = errors.New("codec: encoding failed")
	// ErrDecodingFailed indicates the decoding process failed
	ErrDecodingFailed = errors.New("codec: decoding failed")
	// ErrCodecNotFound indicates the codec is not found in registry
	ErrCodecNotFound = errors.New("codec: not found")
	// ErrCodecConflict indicates codecs conflict in chain
	ErrCodecConflict = errors.New("codec: codec conflict")
)

// Codec is the core interface for encoding/decoding operations.
// It handles encoding/encryption and decoding/decryption.
// Codec MUST NOT be exposed in metadata protocols.
//
// Responsibilities:
// - Encode/encrypt data (Encode)
// - Decode/decrypt data (Decode)
// - Provide internal identifier (InternalID) - NOT for transmission
// - Provide security level (SecurityLevel) - for negotiation
//
// NOT Responsible for:
// - Data serialization (handled by Serializer)
// - Data transmission (handled by Channel)
// - Key acquisition (handled by KeyProvider)
type Codec interface {
	// Encode encodes/encrypts the data.
	//
	// Parameter Constraints:
	//   - data: MUST be non-nil byte slice
	//
	// Return Guarantees:
	//   - On success: returns encoded data
	//   - Output length may be larger than input (e.g., Base64 adds overhead)
	//
	// Error Types:
	//   - ErrKeyRequired: key needed but not set
	//   - ErrInvalidKey: key is invalid
	//   - ErrInvalidData: data is invalid
	//   - ErrEncodingFailed: encoding process failed
	Encode(data []byte) ([]byte, error)

	// Decode decodes/decrypts the data.
	//
	// Parameter Constraints:
	//   - data: MUST be valid encoded format data
	//
	// Return Guarantees:
	//   - On success: returns original data
	//
	// Error Types:
	//   - ErrKeyRequired: key needed but not set
	//   - ErrInvalidKey: key is invalid
	//   - ErrInvalidData: data format invalid or corrupted
	//   - ErrDecodingFailed: decoding process failed
	Decode(data []byte) ([]byte, error)

	// InternalID returns the internal identifier.
	//
	// Return Guarantees:
	//   - Returns unique identifier for internal management only
	//   - MUST NOT be transmitted over network
	//   - Format: internal encoding, e.g., "codec_aes_256_gcm"
	InternalID() string

	// SecurityLevel returns the security level.
	//
	// Return Guarantees:
	//   - Returns SecurityLevel constant value
	//   - Used for security negotiation (value only, not name)
	SecurityLevel() SecurityLevel
}

// KeyAwareCodec is an extension interface for codecs that require keys.
// Encryption codecs (AES, RSA) should implement this interface.
type KeyAwareCodec interface {
	Codec

	// SetKeyProvider sets the key provider for this codec.
	//
	// Parameter Constraints:
	//   - provider: MUST be valid KeyProvider instance
	//
	// Return Guarantees:
	//   - After success, Encode/Decode can use the key
	//
	// Error Types:
	//   - ErrInvalidKeyProvider: provider is invalid
	//   - ErrKeyIncompatible: key type is incompatible
	SetKeyProvider(provider keyprovider.KeyProvider) error

	// RequiresKey returns whether this codec requires a key.
	//
	// Return Guarantees:
	//   - Encryption codecs return true
	//   - Encoding-only codecs (like Base64) return false
	RequiresKey() bool

	// KeyAlgorithm returns the required key algorithm type.
	//
	// Return Guarantees:
	//   - Returns algorithm identifier, e.g., "AES-256-GCM"
	//   - Non-encryption codecs return empty string
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

// CodecRegistry manages registered codecs.
type CodecRegistry struct {
	modules map[string]CodecModule
}

// NewRegistry creates a new CodecRegistry.
func NewRegistry() *CodecRegistry {
	return &CodecRegistry{
		modules: make(map[string]CodecModule),
	}
}

// Register registers a codec module.
func (r *CodecRegistry) Register(module CodecModule) error {
	if module == nil {
		return errors.New("codec: cannot register nil module")
	}
	id := module.InternalID()
	if _, exists := r.modules[id]; exists {
		return errors.New("codec: module already registered: " + id)
	}
	r.modules[id] = module
	return nil
}

// Get retrieves a codec instance by internal ID.
func (r *CodecRegistry) Get(internalID string) (Codec, error) {
	module, exists := r.modules[internalID]
	if !exists {
		return nil, ErrCodecNotFound
	}
	return module.Create(nil)
}

// GetWithArgs retrieves a codec instance with configuration.
func (r *CodecRegistry) GetWithArgs(internalID string, args interface{}) (Codec, error) {
	module, exists := r.modules[internalID]
	if !exists {
		return nil, ErrCodecNotFound
	}
	return module.Create(args)
}

// List returns all registered codec internal IDs.
func (r *CodecRegistry) List() []string {
	result := make([]string, 0, len(r.modules))
	for id := range r.modules {
		result = append(result, id)
	}
	return result
}

// ListBySecurityLevel returns codecs filtered by minimum security level.
func (r *CodecRegistry) ListBySecurityLevel(minLevel SecurityLevel) []string {
	result := make([]string, 0)
	for id, module := range r.modules {
		if module.SecurityLevel() >= minLevel {
			result = append(result, id)
		}
	}
	return result
}

// Exists checks if a codec is registered.
func (r *CodecRegistry) Exists(internalID string) bool {
	_, exists := r.modules[internalID]
	return exists
}

// Global registry instance
var globalRegistry = NewRegistry()

// Register registers a module to the global registry.
func Register(module CodecModule) error {
	return globalRegistry.Register(module)
}

// Get retrieves a codec from the global registry.
func Get(internalID string) (Codec, error) {
	return globalRegistry.Get(internalID)
}

// List returns all internal IDs from the global registry.
func List() []string {
	return globalRegistry.List()
}

// GlobalRegistry returns the global registry instance.
func GlobalRegistry() *CodecRegistry {
	return globalRegistry
}
