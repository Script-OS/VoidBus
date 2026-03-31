// Package codec provides CodecChain for chaining multiple codecs.
//
// CodecChain allows combining multiple codecs in sequence:
// Encode: data -> Codec[0].Encode -> Codec[1].Encode -> ... -> output
// Decode: data -> Codec[n].Decode -> ... -> Codec[1].Decode -> Codec[0].Decode -> output
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.3):
// - CodecChain MUST encode in forward order
// - CodecChain MUST decode in reverse order
// - CodecChain.SecurityLevel() returns the minimum level (security短板原则)
// - CodecChain MUST NOT exceed NegotiationPolicy.MaxCodecChainLength
package codec

import (
	"errors"
	"sync"

	"github.com/Script-OS/VoidBus/keyprovider"
)

// Chain errors
var (
	// ErrChainTooLong indicates the chain exceeds maximum length
	ErrChainTooLong = errors.New("codec chain: too long")
	// ErrChainEmpty indicates the chain is empty
	ErrChainEmpty = errors.New("codec chain: empty")
	// ErrInvalidIndex indicates invalid index for operation
	ErrInvalidIndex = errors.New("codec chain: invalid index")
)

// CodecChain interface for chaining multiple codecs.
type CodecChain interface {
	// AddCodec adds a codec to the end of the chain.
	//
	// Parameter Constraints:
	//   - codec: MUST be valid Codec instance
	//   - Chain length limited by NegotiationPolicy.MaxCodecChainLength (default 5)
	//
	// Return Guarantees:
	//   - Returns updated CodecChain (supports fluent API)
	//
	// Error Types:
	//   - ErrChainTooLong: chain exceeds limit
	//   - ErrCodecConflict: codec conflicts with existing
	AddCodec(codec Codec) CodecChain

	// AddCodecAt adds a codec at specified position.
	//
	// Parameter Constraints:
	//   - codec: MUST be valid Codec instance
	//   - index: 0 to current chain length
	//
	// Return Guarantees:
	//   - Codec inserted at specified position
	AddCodecAt(codec Codec, index int) CodecChain

	// RemoveCodecAt removes codec at specified position.
	//
	// Parameter Constraints:
	//   - index: valid index position
	RemoveCodecAt(index int) CodecChain

	// Encode encodes data through the chain in forward order.
	//
	// Processing Order:
	//   data -> Codec[0].Encode -> Codec[1].Encode -> ... -> Codec[n].Encode -> output
	//
	// Return Guarantees:
	//   - Applies all codecs' Encode methods in sequence
	Encode(data []byte) ([]byte, error)

	// Decode decodes data through the chain in reverse order.
	//
	// Processing Order:
	//   data -> Codec[n].Decode -> ... -> Codec[1].Decode -> Codec[0].Decode -> output
	//
	// Return Guarantees:
	//   - Applies all codecs' Decode methods in reverse sequence
	Decode(data []byte) ([]byte, error)

	// SecurityLevel returns the overall chain security level.
	//
	// Return Guarantees:
	//   - Returns minimum SecurityLevel in chain (security短板原则)
	//   - Empty chain returns SecurityLevelNone
	SecurityLevel() SecurityLevel

	// Length returns the number of codecs in chain.
	Length() int

	// IsEmpty returns whether the chain is empty.
	IsEmpty() bool

	// SetKeyProvider sets key provider for all key-aware codecs.
	//
	// Processing Logic:
	//   - Iterates all codecs in chain
	//   - Calls SetKeyProvider on KeyAwareCodec implementations
	SetKeyProvider(provider keyprovider.KeyProvider) error

	// Clone creates an independent copy of the chain.
	//
	// Return Guarantees:
	//   - Returns independent copy, does not affect original
	Clone() CodecChain

	// InternalIDs returns internal IDs of all codecs.
	//
	// Return Guarantees:
	//   - Returns list of InternalIDs (for internal use only)
	//   - MUST NOT be transmitted
	InternalIDs() []string

	// GetCodec returns the codec at specified index.
	//
	// Parameter Constraints:
	//   - index: valid position in chain (0 to Length()-1)
	//
	// Return Guarantees:
	//   - Returns codec at index
	//   - Returns ErrInvalidIndex if index out of bounds
	//
	// Note: For internal use only, not exposed in protocol.
	GetCodec(index int) (Codec, error)
}

// DefaultChain is the default implementation of CodecChain.
type DefaultChain struct {
	mu     sync.RWMutex
	codecs []Codec
}

// NewChain creates a new empty CodecChain.
func NewChain() *DefaultChain {
	return &DefaultChain{
		codecs: make([]Codec, 0),
	}
}

// NewChainWithCodecs creates a chain with initial codecs.
func NewChainWithCodecs(codecs ...Codec) *DefaultChain {
	chain := NewChain()
	for _, c := range codecs {
		chain.AddCodec(c)
	}
	return chain
}

// AddCodec adds a codec to the end of the chain.
func (c *DefaultChain) AddCodec(codec Codec) CodecChain {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check length limit (default max 5)
	if len(c.codecs) >= 5 {
		// Note: actual limit should be from NegotiationPolicy
		// This is a default safeguard
		return c
	}

	c.codecs = append(c.codecs, codec)
	return c
}

// AddCodecAt adds a codec at specified position.
func (c *DefaultChain) AddCodecAt(codec Codec, index int) CodecChain {
	c.mu.Lock()
	defer c.mu.Unlock()

	if index < 0 || index > len(c.codecs) {
		return c
	}

	// Check length limit
	if len(c.codecs) >= 5 {
		return c
	}

	// Insert at position
	c.codecs = append(c.codecs[:index], append([]Codec{codec}, c.codecs[index:]...)...)
	return c
}

// RemoveCodecAt removes codec at specified position.
func (c *DefaultChain) RemoveCodecAt(index int) CodecChain {
	c.mu.Lock()
	defer c.mu.Unlock()

	if index < 0 || index >= len(c.codecs) {
		return c
	}

	c.codecs = append(c.codecs[:index], c.codecs[index+1:]...)
	return c
}

// Encode encodes data through the chain in forward order.
func (c *DefaultChain) Encode(data []byte) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.codecs) == 0 {
		return data, nil // Empty chain returns unchanged data
	}

	result := data
	for i, codec := range c.codecs {
		encoded, err := codec.Encode(result)
		if err != nil {
			return nil, errors.New("codec chain encode at index " + string(rune('0'+i)) + ": " + err.Error())
		}
		result = encoded
	}
	return result, nil
}

// Decode decodes data through the chain in reverse order.
func (c *DefaultChain) Decode(data []byte) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.codecs) == 0 {
		return data, nil // Empty chain returns unchanged data
	}

	result := data
	for i := len(c.codecs) - 1; i >= 0; i-- {
		decoded, err := c.codecs[i].Decode(result)
		if err != nil {
			return nil, errors.New("codec chain decode at index " + string(rune('0'+i)) + ": " + err.Error())
		}
		result = decoded
	}
	return result, nil
}

// SecurityLevel returns the minimum security level in chain.
func (c *DefaultChain) SecurityLevel() SecurityLevel {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.codecs) == 0 {
		return SecurityLevelNone
	}

	minLevel := SecurityLevelHigh // Start with highest
	for _, codec := range c.codecs {
		level := codec.SecurityLevel()
		if level < minLevel {
			minLevel = level
		}
	}
	return minLevel
}

// Length returns the number of codecs.
func (c *DefaultChain) Length() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.codecs)
}

// IsEmpty returns whether the chain is empty.
func (c *DefaultChain) IsEmpty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.codecs) == 0
}

// SetKeyProvider sets key provider for all key-aware codecs.
func (c *DefaultChain) SetKeyProvider(provider keyprovider.KeyProvider) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if provider == nil {
		return ErrInvalidKeyProvider
	}

	for _, codec := range c.codecs {
		if ka, ok := codec.(KeyAwareCodec); ok {
			if err := ka.SetKeyProvider(provider); err != nil {
				return err
			}
		}
	}
	return nil
}

// Clone creates an independent copy of the chain.
func (c *DefaultChain) Clone() CodecChain {
	c.mu.RLock()
	defer c.mu.RUnlock()

	clone := NewChain()
	clone.codecs = make([]Codec, len(c.codecs))
	copy(clone.codecs, c.codecs)
	return clone
}

// InternalIDs returns internal IDs of all codecs.
func (c *DefaultChain) InternalIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ids := make([]string, len(c.codecs))
	for i, codec := range c.codecs {
		ids[i] = codec.InternalID()
	}
	return ids
}

// GetCodec returns the codec at specified index (for internal use).
func (c *DefaultChain) GetCodec(index int) (Codec, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if index < 0 || index >= len(c.codecs) {
		return nil, ErrInvalidIndex
	}
	return c.codecs[index], nil
}
