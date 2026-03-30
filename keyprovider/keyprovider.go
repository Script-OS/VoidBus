// Package keyprovider defines the KeyProvider interface for key management.
//
// KeyProvider is responsible for providing keys for encryption/decryption.
// It abstracts key retrieval from various sources.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.6):
// - KeyProvider MUST NOT use keys for encryption (handled by Codec)
// - KeyProvider MUST NOT generate keys (handled by external tools)
// - KeyProvider MUST NOT expose key information in metadata
// - KeyProvider MUST support future refresh/rotation (architecture compatibility)
package keyprovider

import (
	"errors"
)

// KeyProviderType identifies the type of key provider.
type KeyProviderType string

const (
	// TypeURL represents URL-based key loading (future implementation)
	TypeURL KeyProviderType = "url"
	// TypeEmbedded represents compile-time embedded keys
	TypeEmbedded KeyProviderType = "embedded"
	// TypeFile represents file-based key loading
	TypeFile KeyProviderType = "file"
	// TypeEnv represents environment variable keys
	TypeEnv KeyProviderType = "env"
)

// Common key provider errors
var (
	// ErrKeyNotFound indicates key not found
	ErrKeyNotFound = errors.New("keyprovider: key not found")
	// ErrKeyExpired indicates key has expired
	ErrKeyExpired = errors.New("keyprovider: key expired")
	// ErrKeyInvalid indicates key is invalid
	ErrKeyInvalid = errors.New("keyprovider: key invalid")
	// ErrKeyFetchFailed indicates key fetch failed
	ErrKeyFetchFailed = errors.New("keyprovider: fetch failed")
	// ErrKeyRefreshFailed indicates key refresh failed
	ErrKeyRefreshFailed = errors.New("keyprovider: refresh failed")
	// ErrNotImplemented indicates feature not yet implemented
	ErrNotImplemented = errors.New("keyprovider: not implemented")
	// ErrProviderNotFound indicates provider not registered
	ErrProviderNotFound = errors.New("keyprovider: not found")
)

// KeyProvider is the core interface for key management.
//
// Responsibilities:
// - Provide keys for encryption/decryption (GetKey)
// - Support future key refresh/rotation (RefreshKey, SupportsRefresh)
// - Identify key source type (Type)
//
// NOT Responsible for:
// - Using keys for encryption (handled by Codec)
// - Key generation (handled by external tools)
// - Key storage security (handled by implementation)
//
// Architecture Compatibility:
// - RefreshKey currently returns ErrNotImplemented
// - SupportsRefresh currently returns false
// - Interface reserved for future key rotation support
type KeyProvider interface {
	// GetKey returns the current key.
	//
	// Return Guarantees:
	//   - On success: returns valid key bytes
	//   - Key format defined by Codec (e.g., AES needs 16/32 bytes)
	//
	// Error Types:
	//   - ErrKeyNotFound: key not found
	//   - ErrKeyFetchFailed: key acquisition failed
	//   - ErrKeyExpired: key has expired
	GetKey() ([]byte, error)

	// RefreshKey refreshes the key.
	//
	// Current Implementation:
	//   - Returns ErrNotImplemented (future support)
	//
	// Future Implementation:
	//   - Re-fetch key from source
	//   - Support key rotation
	RefreshKey() error

	// SupportsRefresh returns whether refresh is supported.
	//
	// Return Guarantees:
	//   - Current implementations return false
	//   - Future dynamic sources (URL) will return true
	SupportsRefresh() bool

	// Type returns the provider type.
	//
	// Return Guarantees:
	//   - Returns KeyProviderType constant
	Type() KeyProviderType
}

// KeyProviderWithMetadata extends KeyProvider with metadata capabilities.
type KeyProviderWithMetadata interface {
	KeyProvider

	// GetKeyMetadata returns metadata about current key.
	GetKeyMetadata() (KeyMetadata, error)

	// GetKeyHistory returns key history (future feature).
	GetKeyHistory() ([]KeyMetadata, error)
}

// KeyMetadata contains metadata about a key.
// MUST NOT be transmitted over network.
type KeyMetadata struct {
	// ID is unique key identifier
	ID string

	// Algorithm is target algorithm (e.g., "AES-256-GCM")
	Algorithm string

	// CreatedAt is creation timestamp
	CreatedAt int64

	// ExpiresAt is expiration timestamp (0 = never expires)
	ExpiresAt int64

	// Source is key source type
	Source KeyProviderType

	// RotationCount is rotation count (future feature)
	RotationCount int
}

// KeyProviderConfig provides configuration for key providers.
type KeyProviderConfig struct {
	// Type is provider type
	Type KeyProviderType

	// URL is URL to fetch key (for URL type)
	URL string

	// Key is embedded key data (for Embedded type)
	Key []byte

	// KeyID is key identifier
	KeyID string

	// Algorithm is target algorithm
	Algorithm string

	// FilePath is file path (for File type)
	FilePath string

	// EnvVar is environment variable name (for Env type)
	EnvVar string

	// RefreshInterval is auto-refresh interval in seconds (future)
	RefreshInterval int

	// Timeout is operation timeout in seconds
	Timeout int

	// RetryCount is retry count for failed operations
	RetryCount int
}

// KeyProviderModule is the interface for key provider module registration.
type KeyProviderModule interface {
	// Create creates a KeyProvider instance.
	Create(config KeyProviderConfig) (KeyProvider, error)

	// Type returns the provider type.
	Type() KeyProviderType
}

// KeyProviderRegistry manages registered key providers.
type KeyProviderRegistry struct {
	modules map[KeyProviderType]KeyProviderModule
}

// NewKeyProviderRegistry creates a new registry.
func NewKeyProviderRegistry() *KeyProviderRegistry {
	return &KeyProviderRegistry{
		modules: make(map[KeyProviderType]KeyProviderModule),
	}
}

// Register registers a key provider module.
func (r *KeyProviderRegistry) Register(module KeyProviderModule) error {
	if module == nil {
		return errors.New("keyprovider: cannot register nil module")
	}
	r.modules[module.Type()] = module
	return nil
}

// Get retrieves a KeyProvider instance.
func (r *KeyProviderRegistry) Get(typ KeyProviderType, config KeyProviderConfig) (KeyProvider, error) {
	module, exists := r.modules[typ]
	if !exists {
		return nil, ErrProviderNotFound
	}
	return module.Create(config)
}

// List returns all registered provider types.
func (r *KeyProviderRegistry) List() []KeyProviderType {
	result := make([]KeyProviderType, 0, len(r.modules))
	for typ := range r.modules {
		result = append(result, typ)
	}
	return result
}

// Global registry
var globalRegistry = NewKeyProviderRegistry()

// Register registers a module to the global registry.
func Register(module KeyProviderModule) error {
	return globalRegistry.Register(module)
}

// Get retrieves a KeyProvider from the global registry.
func Get(typ KeyProviderType, config KeyProviderConfig) (KeyProvider, error) {
	return globalRegistry.Get(typ, config)
}

// List returns all provider types from the global registry.
func List() []KeyProviderType {
	return globalRegistry.List()
}

// GlobalRegistry returns the global registry instance.
func GlobalRegistry() *KeyProviderRegistry {
	return globalRegistry
}
