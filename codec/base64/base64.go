// Package base64 provides a Base64 encoding/decoding codec implementation.
//
// Base64 codec performs standard Base64 encoding/decoding without encryption.
// It is primarily used for:
// - Making binary data safe for text-based transmission
// - Encoding data before sending over channels that require text format
//
// Security Note (see docs/ARCHITECTURE.md §2.1.2):
// - Base64 codec has SecurityLevelLow (1)
// - It does NOT provide encryption, only encoding
// - Suitable for debug mode or combined with encryption codecs
package base64

import (
	"encoding/base64"

	"github.com/Script-OS/VoidBus/codec"
)

const (
	// InternalID is the unique identifier for the base64 codec
	InternalID = "base64"
	// SecurityLevelValue is the security level (low - encoding only, no encryption)
	SecurityLevelValue = codec.SecurityLevelLow
)

// Codec implements the codec.Codec interface with Base64 encoding.
type Codec struct{}

// New creates a new base64 codec instance.
func New() *Codec {
	return &Codec{}
}

// Encode implements codec.Codec.Encode.
// Encodes data using standard Base64 encoding.
//
// Parameter Constraints:
//   - data: can be nil or any byte slice
//
// Return Guarantees:
//   - On success: returns base64 encoded data
//   - Output length is approximately 4/3 of input length
func (c *Codec) Encode(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	// Calculate output length: ceil(len/3) * 4
	dst := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
	base64.StdEncoding.Encode(dst, data)
	return dst, nil
}

// Decode implements codec.Codec.Decode.
// Decodes data from standard Base64 format.
//
// Parameter Constraints:
//   - data: MUST be valid base64 encoded data
//
// Return Guarantees:
//   - On success: returns original data
//
// Error Types:
//   - codec.ErrInvalidData: data is not valid base64
//   - codec.ErrDecodingFailed: decoding process failed
func (c *Codec) Decode(data []byte) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	// Calculate max output length
	dst := make([]byte, base64.StdEncoding.DecodedLen(len(data)))
	n, err := base64.StdEncoding.Decode(dst, data)
	if err != nil {
		return nil, codec.ErrDecodingFailed
	}
	return dst[:n], nil
}

// InternalID implements codec.Codec.InternalID.
// Returns "base64".
func (c *Codec) InternalID() string {
	return InternalID
}

// SecurityLevel implements codec.Codec.SecurityLevel.
// Returns SecurityLevelLow (1) - encoding only, no encryption.
func (c *Codec) SecurityLevel() codec.SecurityLevel {
	return SecurityLevelValue
}

// Module implements codec.CodecModule for registration.
type Module struct{}

// NewModule creates a new base64 codec module.
func NewModule() *Module {
	return &Module{}
}

// Create implements codec.CodecModule.Create.
// Creates a new base64 codec instance.
//
// Parameter Constraints:
//   - args: ignored (base64 codec has no configuration)
//
// Return Guarantees:
//   - Always returns a valid Codec instance
//   - Never returns an error
func (m *Module) Create(args interface{}) (codec.Codec, error) {
	return New(), nil
}

// InternalID implements codec.CodecModule.InternalID.
// Returns "base64".
func (m *Module) InternalID() string {
	return InternalID
}

// SecurityLevel implements codec.CodecModule.SecurityLevel.
// Returns SecurityLevelLow (1).
func (m *Module) SecurityLevel() codec.SecurityLevel {
	return SecurityLevelValue
}

// init registers the base64 codec module on package import.
func init() {
	if err := codec.Register(NewModule()); err != nil {
		panic("base64: failed to register codec: " + err.Error())
	}
}
