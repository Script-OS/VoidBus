// Package serializer provides the serializer registry for VoidBus.
//
// For interface definitions, see interface.go.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.1):
// - Serializer.Name() CAN be exposed in metadata
// - Serializer.Priority() is used for negotiation ordering
package serializer

import (
	"errors"
)

// Registry errors
var (
	// ErrModuleAlreadyRegistered indicates module already registered
	ErrModuleAlreadyRegistered = errors.New("serializer: module already registered")
	// ErrNilModule indicates nil module
	ErrNilModule = errors.New("serializer: cannot register nil module")
)

// SerializerRegistry manages registered serializers.
type SerializerRegistry struct {
	modules map[string]SerializerModule
}

// NewRegistry creates a new SerializerRegistry.
func NewRegistry() *SerializerRegistry {
	return &SerializerRegistry{
		modules: make(map[string]SerializerModule),
	}
}

// Register registers a serializer module.
//
// Parameter Constraints:
//   - module: MUST be valid SerializerModule implementation
//   - module.Name() MUST be unique in registry
//
// Return Guarantees:
//   - Returns nil on success
//   - Returns error if name already registered
func (r *SerializerRegistry) Register(module SerializerModule) error {
	if module == nil {
		return errors.New("serializer: cannot register nil module")
	}
	name := module.Name()
	if _, exists := r.modules[name]; exists {
		return errors.New("serializer: module already registered: " + name)
	}
	r.modules[name] = module
	return nil
}

// Get retrieves a serializer instance by name.
//
// Parameter Constraints:
//   - name: MUST be non-empty string
//
// Return Guarantees:
//   - On success: returns Serializer instance
//   - On failure: returns nil and ErrSerializerNotFound
func (r *SerializerRegistry) Get(name string) (Serializer, error) {
	module, exists := r.modules[name]
	if !exists {
		return nil, ErrSerializerNotFound
	}
	return module.Create(nil)
}

// GetWithArgs retrieves a serializer instance with configuration.
func (r *SerializerRegistry) GetWithArgs(name string, args interface{}) (Serializer, error) {
	module, exists := r.modules[name]
	if !exists {
		return nil, ErrSerializerNotFound
	}
	return module.Create(args)
}

// List returns all registered serializer names.
func (r *SerializerRegistry) List() []string {
	result := make([]string, 0, len(r.modules))
	for name := range r.modules {
		result = append(result, name)
	}
	return result
}

// Exists checks if a serializer is registered.
func (r *SerializerRegistry) Exists(name string) bool {
	_, exists := r.modules[name]
	return exists
}

// Count returns the number of registered serializers.
func (r *SerializerRegistry) Count() int {
	return len(r.modules)
}

// Global registry instance
var globalRegistry = NewRegistry()

// Register registers a module to the global registry.
func Register(module SerializerModule) error {
	return globalRegistry.Register(module)
}

// Get retrieves a serializer from the global registry.
func Get(name string) (Serializer, error) {
	return globalRegistry.Get(name)
}

// List returns all names from the global registry.
func List() []string {
	return globalRegistry.List()
}

// GlobalRegistry returns the global registry instance.
func GlobalRegistry() *SerializerRegistry {
	return globalRegistry
}
