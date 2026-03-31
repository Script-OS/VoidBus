// Package embedded provides a compile-time embedded key provider implementation.
//
// Embedded key provider stores keys that are embedded at compile time using go:embed.
// It is primarily used for:
// - Pre-configured keys that are known at build time
// - Development and testing scenarios
// - Simple deployment without external key sources
//
// Security Note (see docs/ARCHITECTURE.md §2.1.6):
// - Embedded keys MUST be handled carefully in production
// - Key is compiled into binary, accessible through memory inspection
// - Recommended to combine with secure deployment practices
// - MUST NOT expose key information in metadata protocols
package embedded

import (
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/internal"
	"github.com/Script-OS/VoidBus/keyprovider"
)

// Provider implements keyprovider.KeyProvider with embedded key storage.
type Provider struct {
	key       []byte
	keyID     string
	algorithm string
	mu        sync.RWMutex
	metadata  keyprovider.KeyMetadata
}

// New creates a new embedded key provider.
//
// Parameter Constraints:
//   - key: MUST be non-nil byte slice, length determined by algorithm
//   - keyID: optional identifier, auto-generated if empty
//   - algorithm: optional algorithm identifier (e.g., "AES-256-GCM")
//
// Return Guarantees:
//   - On success: returns valid Provider instance
//   - Key is stored in memory, not refreshed
func New(key []byte, keyID string, algorithm string) (*Provider, error) {
	if key == nil || len(key) == 0 {
		return nil, keyprovider.ErrKeyInvalid
	}

	// Auto-generate key ID if not provided
	if keyID == "" {
		keyID = internal.GenerateID()
	}

	// Create metadata
	metadata := keyprovider.KeyMetadata{
		ID:            keyID,
		Algorithm:     algorithm,
		CreatedAt:     time.Now().Unix(),
		ExpiresAt:     0, // Never expires
		Source:        keyprovider.TypeEmbedded,
		RotationCount: 0,
	}

	return &Provider{
		key:       key,
		keyID:     keyID,
		algorithm: algorithm,
		metadata:  metadata,
	}, nil
}

// NewWithConfig creates a new embedded key provider from config.
func NewWithConfig(config keyprovider.KeyProviderConfig) (*Provider, error) {
	return New(config.Key, config.KeyID, config.Algorithm)
}

// GetKey implements keyprovider.KeyProvider.GetKey.
// Returns the embedded key.
//
// Return Guarantees:
//   - Always returns the same key (embedded)
//   - Never returns error (key is always available)
func (p *Provider) GetKey() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to prevent modification
	result := make([]byte, len(p.key))
	copy(result, p.key)
	return result, nil
}

// RefreshKey implements keyprovider.KeyProvider.RefreshKey.
// Embedded keys cannot be refreshed.
//
// Return Guarantees:
//   - Always returns ErrNotImplemented
func (p *Provider) RefreshKey() error {
	return keyprovider.ErrNotImplemented
}

// SupportsRefresh implements keyprovider.KeyProvider.SupportsRefresh.
// Embedded keys do not support refresh.
//
// Return Guarantees:
//   - Always returns false
func (p *Provider) SupportsRefresh() bool {
	return false
}

// Type implements keyprovider.KeyProvider.Type.
func (p *Provider) Type() keyprovider.KeyProviderType {
	return keyprovider.TypeEmbedded
}

// GetKeyMetadata implements keyprovider.KeyProviderWithMetadata.GetKeyMetadata.
func (p *Provider) GetKeyMetadata() (keyprovider.KeyMetadata, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.metadata, nil
}

// GetKeyHistory implements keyprovider.KeyProviderWithMetadata.GetKeyKeyHistory.
func (p *Provider) GetKeyHistory() ([]keyprovider.KeyMetadata, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	// Embedded keys have no history
	return []keyprovider.KeyMetadata{p.metadata}, nil
}

// Module implements keyprovider.KeyProviderModule for registration.
type Module struct{}

// NewModule creates a new embedded key provider module.
func NewModule() *Module {
	return &Module{}
}

// Create implements keyprovider.KeyProviderModule.Create.
func (m *Module) Create(config keyprovider.KeyProviderConfig) (keyprovider.KeyProvider, error) {
	return NewWithConfig(config)
}

// Type implements keyprovider.KeyProviderModule.Type.
func (m *Module) Type() keyprovider.KeyProviderType {
	return keyprovider.TypeEmbedded
}

// init registers the embedded key provider module.
func init() {
	if err := keyprovider.Register(NewModule()); err != nil {
		panic("embedded: failed to register key provider: " + err.Error())
	}
}
