// Package serializer defines the Serializer interface and registry for data
// serialization/deserialization operations.
//
// Serializer is responsible for converting data structures to/from byte streams.
// Unlike Codec, Serializer can be exposed in metadata protocols for negotiation.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.1):
// - Serializer MUST NOT handle encoding/encryption
// - Serializer MUST NOT handle data transmission
// - Serializer MUST NOT handle data fragmentation
// - Serializer.Name() CAN be exposed in metadata
// - Serializer.Priority() is used for negotiation ordering
package serializer

import (
	"errors"
)

// Common serializer errors
var (
	// ErrInvalidData indicates the input data is invalid for serialization
	ErrInvalidData = errors.New("serializer: invalid data")
	// ErrSerializationFailed indicates the serialization process failed
	ErrSerializationFailed = errors.New("serializer: serialization failed")
	// ErrDeserializationFailed indicates the deserialization process failed
	ErrDeserializationFailed = errors.New("serializer: deserialization failed")
	// ErrSerializerNotFound indicates the requested serializer is not registered
	ErrSerializerNotFound = errors.New("serializer: not found")
)

// Serializer is the core interface for data serialization/deserialization.
// It is responsible for converting data structures to/from byte streams.
// Serializer CAN be exposed in metadata protocols for negotiation between parties.
//
// Responsibilities:
// - Convert data structures to byte streams (Serialize)
// - Convert byte streams back to data structures (Deserialize)
// - Provide a name for identification (Name)
// - Provide a priority for negotiation ordering (Priority)
//
// NOT Responsible for:
// - Data encoding/encryption (handled by Codec)
// - Data transmission (handled by Channel)
// - Data fragmentation (handled by Fragment)
type Serializer interface {
	// Serialize converts data to a byte stream.
	//
	// Parameter Constraints:
	//   - data: MUST be non-nil byte slice, length >= 0
	//
	// Return Guarantees:
	//   - On success: returns serialized byte stream, length >= 0
	//   - On failure: returns nil and a specific error
	//
	// Error Types:
	//   - ErrInvalidData: input data is invalid
	//   - ErrSerializationFailed: serialization process failed
	Serialize(data []byte) ([]byte, error)

	// Deserialize converts a byte stream back to original data.
	//
	// Parameter Constraints:
	//   - data: MUST be valid serialized format byte stream
	//
	// Return Guarantees:
	//   - On success: returns original data
	//   - On failure: returns nil and a specific error
	//
	// Error Types:
	//   - ErrInvalidData: input data format is invalid
	//   - ErrDeserializationFailed: deserialization process failed
	Deserialize(data []byte) ([]byte, error)

	// Name returns the serializer name.
	//
	// Return Guarantees:
	//   - Returns unique, exposeable name identifier
	//   - Format: lowercase letters + numbers + underscores, e.g. "json", "protobuf_v2"
	//   - CAN be transmitted in metadata protocols
	Name() string

	// Priority returns the priority level for negotiation.
	//
	// Return Guarantees:
	//   - Returns priority value 0-100
	//   - Higher value means higher priority
	//   - Used for ordering during negotiation
	Priority() int
}

// SerializerModule is the interface for serializer module registration.
// Each serializer implementation should provide a Module for registration.
type SerializerModule interface {
	// Create creates a serializer instance with optional configuration.
	//
	// Parameter Constraints:
	//   - args: optional configuration, type defined by specific implementation
	//
	// Return Guarantees:
	//   - On success: returns Serializer instance
	//   - On failure: returns nil and error
	Create(args interface{}) (Serializer, error)

	// Name returns the module name (must match Serializer.Name() from Create)
	Name() string
}

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
