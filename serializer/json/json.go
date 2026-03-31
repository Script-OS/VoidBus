// Package json provides a JSON serializer implementation.
//
// JSONSerializer serializes/deserializes data as JSON.
// It is primarily used for structured data and handshake protocols.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.1):
// - JSONSerializer.Name() CAN be exposed
// - Priority = 5 (lower than protobuf, higher than plain)
package json

import (
	"encoding/json"

	"VoidBus/serializer"
)

const (
	// Name is the unique identifier
	Name = "json"
	// Priority for negotiation
	Priority = 5
)

// Serializer implements serializer.Serializer with JSON encoding.
type Serializer struct{}

// New creates a new JSON serializer.
func New() *Serializer { return &Serializer{} }

// Serialize implements serializer.Serializer.Serialize.
// Encodes data as JSON (pass-through for []byte, JSON encode for others).
func (s *Serializer) Serialize(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	// For raw bytes, wrap in JSON array
	var wrapper struct {
		Data []byte `json:"data"`
	}
	wrapper.Data = data
	return json.Marshal(wrapper)
}

// Deserialize implements serializer.Serializer.Deserialize.
func (s *Serializer) Deserialize(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	var wrapper struct {
		Data []byte `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		// Try direct pass-through for compatibility
		return data, nil
	}
	return wrapper.Data, nil
}

// Name returns "json".
func (s *Serializer) Name() string { return Name }

// Priority returns 5.
func (s *Serializer) Priority() int { return Priority }

// Module implements serializer.SerializerModule.
type Module struct{}

// NewModule creates a new module.
func NewModule() *Module { return &Module{} }

// Create creates a serializer instance.
func (m *Module) Create(args interface{}) (serializer.Serializer, error) {
	return New(), nil
}

// Name returns "json".
func (m *Module) Name() string { return Name }

func init() {
	serializer.Register(NewModule())
}
