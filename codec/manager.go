// Package codec provides CodecManager for v2.0 architecture.
// CodecManager handles random codec selection and hash-based matching.
package codec

import (
	"fmt"
	"sync"

	"github.com/Script-OS/VoidBus/internal"
)

// CodecManager manages codec instances and provides random selection
// with hash-based matching for the v2.0 architecture.
type CodecManager struct {
	mu sync.RWMutex

	// codeToCodec maps user-defined codes to codec instances
	// e.g., "A" -> AES-256, "B" -> Base64
	codeToCodec map[string]Codec

	// supportedCodes stores the list of registered codes
	supportedCodes []string

	// hashCache provides O(1) hash lookup (P0 optimization)
	hashCache *internal.HashCache

	// maxDepth is the maximum codec chain depth
	maxDepth int

	// salt for hash computation (security optimization)
	salt []byte

	// negotiated indicates if negotiation has completed
	negotiated bool

	// negotiatedCodes are the common codes after negotiation
	negotiatedCodes []string
}

// NewCodecManager creates a new CodecManager.
func NewCodecManager() *CodecManager {
	return &CodecManager{
		codeToCodec:    make(map[string]Codec),
		supportedCodes: make([]string, 0),
		hashCache:      internal.NewHashCache(),
		maxDepth:       2, // Default
	}
}

// AddCodec registers a codec with a user-defined code.
// The code is a short identifier like "A", "B", "X" that will be used
// in hash computation instead of exposing codec names.
func (m *CodecManager) AddCodec(codec Codec, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if codec == nil {
		return ErrCodecNotFound
	}

	if code == "" {
		return fmt.Errorf("codec code cannot be empty")
	}

	// Check if code already exists
	if _, exists := m.codeToCodec[code]; exists {
		return fmt.Errorf("codec code '%s' already registered", code)
	}

	m.codeToCodec[code] = codec
	m.supportedCodes = append(m.supportedCodes, code)

	// Invalidate cache
	m.hashCache.Clear()

	return nil
}

// SetMaxDepth sets the maximum codec chain depth.
func (m *CodecManager) SetMaxDepth(depth int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if depth < 1 || depth > 5 {
		return fmt.Errorf("max depth must be between 1 and 5")
	}

	m.maxDepth = depth
	m.hashCache.Clear()

	return nil
}

// SetSalt sets the salt for hash computation (security optimization).
// Should be set during negotiation.
func (m *CodecManager) SetSalt(salt []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.salt = salt
	m.hashCache.SetSalt(salt)
	m.hashCache.Clear()
}

// GetSupportedCodes returns the list of registered codec codes.
func (m *CodecManager) GetSupportedCodes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]string, len(m.supportedCodes))
	copy(result, m.supportedCodes)
	return result
}

// GetMaxDepth returns the maximum codec chain depth.
func (m *CodecManager) GetMaxDepth() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maxDepth
}

// RandomSelect randomly selects codec codes and creates a codec chain.
// This is used on the send side for codec chain selection.
func (m *CodecManager) RandomSelect() ([]string, CodecChain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.supportedCodes) == 0 {
		return nil, nil, ErrCodecNotFound
	}

	// Use negotiated codes if available
	codes := m.supportedCodes
	if m.negotiated && len(m.negotiatedCodes) > 0 {
		codes = m.negotiatedCodes
	}

	// Random select depth (between 1 and maxDepth)
	depth := internal.RandomInt(m.maxDepth) + 1

	// Generate random permutation
	selectedCodes := internal.RandomPermutation(codes, depth)
	if len(selectedCodes) == 0 {
		return nil, nil, ErrCodecNotFound
	}

	// Create codec chain
	chain := NewChain()
	for _, code := range selectedCodes {
		codec, exists := m.codeToCodec[code]
		if !exists {
			return nil, nil, fmt.Errorf("codec not found for code: %s", code)
		}
		chain.AddCodec(codec)
	}

	return selectedCodes, chain, nil
}

// ComputeHash computes hash for the given codec codes.
func (m *CodecManager) ComputeHash(codes []string) [32]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.salt) > 0 {
		return internal.ComputeHashWithSalt(codes, m.salt)
	}
	return internal.ComputeHash(codes)
}

// PreComputeHashes pre-computes all possible hash values for O(1) lookup.
// This is a P0 performance optimization that should be called after negotiation.
func (m *CodecManager) PreComputeHashes() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	codes := m.supportedCodes
	if m.negotiated && len(m.negotiatedCodes) > 0 {
		codes = m.negotiatedCodes
	}

	if len(codes) == 0 {
		return ErrCodecNotFound
	}

	m.hashCache.SetSalt(m.salt)
	m.hashCache.PreCompute(codes, m.maxDepth)

	return nil
}

// MatchByHash finds codec codes by hash (O(1) with pre-computation).
// This is used on the receive side for codec chain matching.
func (m *CodecManager) MatchByHash(hash [32]byte) ([]string, CodecChain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try cache first (O(1))
	if m.hashCache.Size() > 0 {
		codes, ok := m.hashCache.Lookup(hash)
		if ok {
			chain, err := m.createChainFromCodes(codes)
			if err != nil {
				return nil, nil, err
			}
			return codes, chain, nil
		}
	}

	// Fallback: try all combinations (O(n^d))
	codes := m.supportedCodes
	if m.negotiated && len(m.negotiatedCodes) > 0 {
		codes = m.negotiatedCodes
	}

	for depth := 1; depth <= m.maxDepth; depth++ {
		perms := internal.GeneratePermutations(codes, depth)
		for _, combo := range perms {
			var expectedHash [32]byte
			if len(m.salt) > 0 {
				expectedHash = internal.ComputeHashWithSalt(combo, m.salt)
			} else {
				expectedHash = internal.ComputeHash(combo)
			}

			if expectedHash == hash {
				chain, err := m.createChainFromCodes(combo)
				if err != nil {
					return nil, nil, err
				}
				return combo, chain, nil
			}
		}
	}

	return nil, nil, ErrCodecChainMismatch
}

// createChainFromCodes creates a codec chain from code list.
func (m *CodecManager) createChainFromCodes(codes []string) (CodecChain, error) {
	chain := NewChain()
	for _, code := range codes {
		codec, exists := m.codeToCodec[code]
		if !exists {
			return nil, fmt.Errorf("codec not found for code: %s", code)
		}
		chain.AddCodec(codec)
	}
	return chain, nil
}

// Negotiate performs capability negotiation with remote codes.
// Returns common supported codes and sets up for communication.
func (m *CodecManager) Negotiate(remoteCodes []string, remoteMaxDepth int, salt []byte) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find common codes
	commonCodes := make([]string, 0)
	remoteSet := make(map[string]bool)
	for _, code := range remoteCodes {
		remoteSet[code] = true
	}

	for _, code := range m.supportedCodes {
		if remoteSet[code] {
			commonCodes = append(commonCodes, code)
		}
	}

	if len(commonCodes) == 0 {
		return nil, fmt.Errorf("no common codec codes")
	}

	// Set negotiated state
	m.negotiated = true
	m.negotiatedCodes = commonCodes

	// Use minimum depth
	m.maxDepth = min(m.maxDepth, remoteMaxDepth)

	// Set salt
	m.salt = salt
	m.hashCache.SetSalt(salt)

	// Pre-compute hashes for O(1) lookup
	m.hashCache.PreCompute(commonCodes, m.maxDepth)

	return commonCodes, nil
}

// GetNegotiatedCodes returns negotiated codes after negotiation.
func (m *CodecManager) GetNegotiatedCodes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.negotiated {
		return nil
	}

	result := make([]string, len(m.negotiatedCodes))
	copy(result, m.negotiatedCodes)
	return result
}

// IsNegotiated returns true if negotiation has completed.
func (m *CodecManager) IsNegotiated() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.negotiated
}

// GetCodec returns codec instance by code.
func (m *CodecManager) GetCodec(code string) (Codec, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	codec, exists := m.codeToCodec[code]
	if !exists {
		return nil, fmt.Errorf("codec not found for code: %s", code)
	}
	return codec, nil
}

// GetHashCacheSize returns the number of pre-computed hashes.
func (m *CodecManager) GetHashCacheSize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hashCache.Size()
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
