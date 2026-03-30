// Package codec provides the codec registry for VoidBus.
//
// For interface definitions, see interface.go.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.2):
// - Codec.InternalID() MUST NOT be transmitted
// - Codec.SecurityLevel() is used for negotiation (value only, not name)
package codec

import (
	"errors"
)

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
