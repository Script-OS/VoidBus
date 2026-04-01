// Package plain provides a pass-through codec implementation.
//
// Plain codec performs no transformation on data.
// It is primarily used for:
// - Debug scenarios where encoding overhead is unnecessary
// - Cases where data is already in desired format
// - Testing and benchmarking purposes
//
// Security Warning (see docs/ARCHITECTURE.md §2.1.2):
// - Plain codec has SecurityLevelNone (0)
// - MUST NOT be used in release mode (blocked by NegotiationPolicy)
// - Data is transmitted without any encoding or encryption
package plain

import (
	"github.com/Script-OS/VoidBus/codec"
)

const (
	// InternalID is the unique identifier for the plain codec
	InternalID = "plain"
	// SecurityLevelValue is the security level (none - no encoding/encryption)
	SecurityLevelValue = codec.SecurityLevelNone
)

// Codec implements the codec.Codec interface with pass-through behavior.
type Codec struct {
	code string // User-defined code for chain hash, default "plain"
}

// New creates a new plain codec instance.
func New() *Codec {
	return &Codec{
		code: "plain",
	}
}

// SetCode sets a custom code for chain hash computation.
// This allows users to define their own code identifiers.
func (c *Codec) SetCode(code string) {
	c.code = code
}

// Encode implements codec.Codec.Encode.
// Returns the input data unchanged (pass-through).
//
// Parameter Constraints:
//   - data: can be nil or any byte slice
//
// Return Guarantees:
//   - Always returns input data unchanged
//   - Never returns an error
func (c *Codec) Encode(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	// Return a copy to prevent caller from modifying original
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// Decode implements codec.Codec.Decode.
// Returns the input data unchanged (pass-through).
//
// Parameter Constraints:
//   - data: can be nil or any byte slice
//
// Return Guarantees:
//   - Always returns input data unchanged
//   - Never returns an error
func (c *Codec) Decode(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	// Return a copy to prevent caller from modifying original
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// InternalID implements codec.Codec.InternalID.
// Returns "plain".
func (c *Codec) InternalID() string {
	return InternalID
}

// SecurityLevel implements codec.Codec.SecurityLevel.
// Returns SecurityLevelNone (0) - no encoding/encryption.
func (c *Codec) SecurityLevel() codec.SecurityLevel {
	return SecurityLevelValue
}

// Code implements codec.Codec.Code.
// Returns the user-defined code (default "plain").
func (c *Codec) Code() string {
	if c.code == "" {
		return "plain"
	}
	return c.code
}

// Module implements codec.CodecModule for registration.
type Module struct{}

// NewModule creates a new plain codec module.
func NewModule() *Module {
	return &Module{}
}

// Create implements codec.CodecModule.Create.
// Creates a new plain codec instance.
//
// Parameter Constraints:
//   - args: ignored (plain codec has no configuration)
//
// Return Guarantees:
//   - Always returns a valid Codec instance
//   - Never returns an error
func (m *Module) Create(args interface{}) (codec.Codec, error) {
	return New(), nil
}

// InternalID implements codec.CodecModule.InternalID.
// Returns "plain".
func (m *Module) InternalID() string {
	return InternalID
}

// SecurityLevel implements codec.CodecModule.SecurityLevel.
// Returns SecurityLevelNone (0).
func (m *Module) SecurityLevel() codec.SecurityLevel {
	return SecurityLevelValue
}

// init registers the plain codec module on package import.
func init() {
	if err := codec.Register(NewModule()); err != nil {
		panic("plain: failed to register codec: " + err.Error())
	}
}
