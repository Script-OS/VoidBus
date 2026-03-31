// Package plain provides a pass-through serializer implementation.
//
// Plain serializer performs no transformation on data, simply passing it through.
// It is primarily used for:
// - Debug scenarios where serialization overhead is unnecessary
// - Cases where data is already in byte stream format
// - Testing and benchmarking purposes
//
// Security Note (see docs/ARCHITECTURE.md §2.1.1):
// - Plain serializer CAN be exposed in metadata protocols
// - It has the lowest priority (0) during negotiation
// - In Release mode, it should only be used with appropriate Codec protection
package plain

import (
	"github.com/Script-OS/VoidBus/serializer"
)

const (
	// Name is the unique identifier for the plain serializer
	Name = "plain"
	// Priority is the negotiation priority (lowest = 0)
	Priority = 0
)

// Serializer implements the serializer.Serializer interface
// with pass-through (no-op) behavior.
type Serializer struct{}

// New creates a new plain serializer instance.
func New() *Serializer {
	return &Serializer{}
}

// Serialize implements serializer.Serializer.Serialize.
// It returns the input data unchanged (pass-through).
//
// Parameter Constraints:
//   - data: can be nil or any byte slice
//
// Return Guarantees:
//   - Always returns input data unchanged
//   - Never returns an error
func (s *Serializer) Serialize(data []byte) ([]byte, error) {
	// Pass-through: return input unchanged
	if data == nil {
		return nil, nil
	}
	// Return a copy to prevent caller from modifying original
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// Deserialize implements serializer.Serializer.Deserialize.
// It returns the input data unchanged (pass-through).
//
// Parameter Constraints:
//   - data: can be nil or any byte slice
//
// Return Guarantees:
//   - Always returns input data unchanged
//   - Never returns an error
func (s *Serializer) Deserialize(data []byte) ([]byte, error) {
	// Pass-through: return input unchanged
	if data == nil {
		return nil, nil
	}
	// Return a copy to prevent caller from modifying original
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// Name implements serializer.Serializer.Name.
// Returns "plain".
func (s *Serializer) Name() string {
	return Name
}

// Priority implements serializer.Serializer.Priority.
// Returns 0 (lowest priority).
func (s *Serializer) Priority() int {
	return Priority
}

// Module implements serializer.SerializerModule for registration.
type Module struct{}

// NewModule creates a new plain serializer module.
func NewModule() *Module {
	return &Module{}
}

// Create implements serializer.SerializerModule.Create.
// Creates a new plain serializer instance.
//
// Parameter Constraints:
//   - args: ignored (plain serializer has no configuration)
//
// Return Guarantees:
//   - Always returns a valid Serializer instance
//   - Never returns an error
func (m *Module) Create(args interface{}) (serializer.Serializer, error) {
	return New(), nil
}

// Name implements serializer.SerializerModule.Name.
// Returns "plain".
func (m *Module) Name() string {
	return Name
}

// init registers the plain serializer module on package import.
func init() {
	if err := serializer.Register(NewModule()); err != nil {
		// Panic on registration failure indicates a programming error
		// (duplicate registration)
		panic("plain: failed to register serializer: " + err.Error())
	}
}
