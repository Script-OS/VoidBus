// Package serializer defines the Serializer interface for data serialization/deserialization.
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
	Serialize(data []byte) ([]byte, error)

	// Deserialize converts a byte stream back to original data.
	Deserialize(data []byte) ([]byte, error)

	// Name returns the serializer name (CAN be exposed).
	Name() string

	// Priority returns the priority level for negotiation.
	Priority() int
}

// SerializerModule is the interface for serializer module registration.
type SerializerModule interface {
	// Create creates a serializer instance with optional configuration.
	Create(args interface{}) (Serializer, error)

	// Name returns the module name.
	Name() string
}
