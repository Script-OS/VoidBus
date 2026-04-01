// Package codec provides CodecManager for v2.1 architecture.
//
// CodecManager handles codec chain selection based on user-defined codes.
//
// Key Concepts:
// - Codec Bitmap: Used ONLY for negotiation to indicate which codec types are supported
// - Codec Code: User-defined identifier, must be consistent between sender and receiver
// - Chain Hash: SHA256 of code sequence (e.g., "aes|base64") for matching
package codec

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// CodecManager manages codec instances and provides code-based chain selection.
type CodecManager struct {
	mu sync.RWMutex

	// Registered codecs: code -> Codec
	codecs map[string]Codec

	// Negotiated codec codes (after handshake)
	negotiatedCodes []string

	// Negotiated bitmap (for reference only)
	negotiatedBitmap []byte

	// Maximum codec chain depth
	maxDepth int

	// Salt for hash computation
	salt []byte

	// Hash cache: uint32(hash[:4]) -> code sequence
	hashCache map[uint32][]string

	// Random source
	rand *rand.Rand

	// Negotiation state
	negotiated bool
}

// NewCodecManager creates a new CodecManager.
func NewCodecManager() *CodecManager {
	return &CodecManager{
		codecs:    make(map[string]Codec),
		maxDepth:  5,
		hashCache: make(map[uint32][]string),
		rand:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Name returns the module name.
func (m *CodecManager) Name() string {
	return "CodecManager"
}

// ModuleStats returns module statistics.
func (m *CodecManager) ModuleStats() interface{} {
	return m.Stats()
}

// Stats returns codec manager statistics.
func (m *CodecManager) Stats() CodecManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return CodecManagerStats{
		CodecCount:    len(m.codecs),
		MaxDepth:      m.maxDepth,
		HashCacheSize: len(m.hashCache),
		IsNegotiated:  m.negotiated,
	}
}

// CodecManagerStats holds codec manager statistics.
type CodecManagerStats struct {
	CodecCount    int
	MaxDepth      int
	HashCacheSize int
	IsNegotiated  bool
}

// RegisterCodec registers a codec with its user-defined code.
// The code is used for chain hash computation and MUST be consistent
// between sender and receiver.
//
// Example:
//
//	aesCodec := aes.NewAES256Codec(...)
//	manager.RegisterCodec(aesCodec, "aes")  // code = "aes"
func (m *CodecManager) RegisterCodec(codec Codec, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if codec == nil {
		return errors.New("codec cannot be nil")
	}

	if code == "" {
		return errors.New("code cannot be empty")
	}

	// Check if code is already registered with different codec
	if existing, exists := m.codecs[code]; exists && existing != codec {
		return ErrCodecConflict
	}

	m.codecs[code] = codec
	return nil
}

// SetNegotiatedBitmap sets the negotiated codec bitmap.
// This converts the bitmap to a list of codec codes.
// Called after handshake negotiation.
func (m *CodecManager) SetNegotiatedBitmap(bitmap []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Map bitmap bits to codes
	// Bit position follows negotiate.CodecBit constants:
	// 0=Plain, 1=Base64, 2=AES256, 3=XOR, 4=ChaCha20, 5=RSA, 6=GZIP, 7=ZSTD
	negotiated := make([]string, 0)
	for code := range m.codecs {
		bitPos := codeToBitPosition(code)
		if bitPos >= 0 && isBitSet(bitmap, bitPos) {
			negotiated = append(negotiated, code)
		}
	}

	if len(negotiated) == 0 {
		return errors.New("no available codecs after negotiation")
	}

	m.negotiatedCodes = negotiated
	m.negotiatedBitmap = bitmap
	m.negotiated = true

	// Clear hash cache
	m.hashCache = make(map[uint32][]string)

	return nil
}

// SetNegotiatedCodes directly sets the negotiated codec codes.
// Alternative to SetNegotiatedBitmap when codes are known.
func (m *CodecManager) SetNegotiatedCodes(codes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate codes
	for _, code := range codes {
		if _, exists := m.codecs[code]; !exists {
			return ErrCodecNotFound
		}
	}

	if len(codes) == 0 {
		return errors.New("no available codecs")
	}

	m.negotiatedCodes = codes
	m.negotiated = true
	m.hashCache = make(map[uint32][]string)

	return nil
}

// codeToBitPosition maps codec code to bitmap bit position.
// Follows negotiate.CodecBit constants.
func codeToBitPosition(code string) int {
	// Standard mapping based on negotiate.CodecBit
	// Users should use these standard codes for proper bitmap mapping
	switch strings.ToLower(code) {
	case "plain", "p":
		return 0
	case "base64", "b":
		return 1
	case "aes", "aes256", "a":
		return 2
	case "xor", "x":
		return 3
	case "chacha", "chacha20", "c":
		return 4
	case "rsa", "r":
		return 5
	case "gzip", "g":
		return 6
	case "zstd", "z":
		return 7
	default:
		return -1 // Unknown code, not in standard bitmap
	}
}

// SetMaxDepth sets maximum codec chain depth.
func (m *CodecManager) SetMaxDepth(depth int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if depth < 1 || depth > 5 {
		return errors.New("max depth must be 1-5")
	}

	m.maxDepth = depth
	m.hashCache = make(map[uint32][]string)
	return nil
}

// SetSalt sets salt for hash computation.
func (m *CodecManager) SetSalt(salt []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.salt = salt
	m.hashCache = make(map[uint32][]string)
}

// SelectChain randomly selects a codec chain.
// Returns: codec chain, hash for matching
func (m *CodecManager) SelectChain() (CodecChain, [32]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	codes := m.getAvailableCodes()
	if len(codes) == 0 {
		return nil, [32]byte{}, ErrCodecNotFound
	}

	// Random depth (1 to maxDepth)
	depth := m.rand.Intn(m.maxDepth) + 1

	// Random selection (with possible repetition)
	selectedCodes := make([]string, depth)
	for i := 0; i < depth; i++ {
		selectedCodes[i] = codes[m.rand.Intn(len(codes))]
	}

	// Create chain
	chain := NewChain()
	for _, code := range selectedCodes {
		codec, exists := m.codecs[code]
		if !exists {
			return nil, [32]byte{}, ErrCodecNotFound
		}
		chain.AddCodec(codec)
	}

	// Compute hash based on code sequence
	hash := m.computeChainHash(selectedCodes)

	// Cache for later matching
	m.hashCache[hashToUint32(hash)] = selectedCodes

	return chain, hash, nil
}

// MatchChain finds codec chain by hash.
func (m *CodecManager) MatchChain(hash [32]byte) (CodecChain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check cache first
	hashKey := hashToUint32(hash)
	if codes, exists := m.hashCache[hashKey]; exists {
		return m.createChainFromCodes(codes), nil
	}

	// Try all combinations (fallback - should rarely happen)
	codes := m.getAvailableCodes()
	for depth := 1; depth <= m.maxDepth; depth++ {
		combinations := generateCodeCombinations(codes, depth)
		for _, combo := range combinations {
			expectedHash := m.computeChainHash(combo)
			if expectedHash == hash {
				// Cache for future use
				m.hashCache[hashKey] = combo
				return m.createChainFromCodes(combo), nil
			}
		}
	}

	return nil, ErrCodecChainMismatch
}

// computeChainHash computes SHA256 hash for codec chain based on codes.
// Hash = SHA256("code1|code2|code3" + salt)
func (m *CodecManager) computeChainHash(codes []string) [32]byte {
	// Join codes with separator to avoid collision
	// e.g., ["aes", "base64"] -> "aes|base64"
	joined := strings.Join(codes, "|")

	// Add salt if available
	data := []byte(joined)
	if len(m.salt) > 0 {
		data = append(data, m.salt...)
	}

	// Use SHA256 for [32]byte hash
	return sha256.Sum256(data)
}

// hashToUint32 converts [32]byte to uint32 for cache key.
func hashToUint32(hash [32]byte) uint32 {
	return binary.BigEndian.Uint32(hash[:4])
}

// createChainFromCodes creates chain from code strings.
func (m *CodecManager) createChainFromCodes(codes []string) CodecChain {
	chain := NewChain()
	for _, code := range codes {
		if codec, exists := m.codecs[code]; exists {
			chain.AddCodec(codec)
		}
	}
	return chain
}

// getAvailableCodes returns available codes (negotiated if set, otherwise all).
func (m *CodecManager) getAvailableCodes() []string {
	if m.negotiated && len(m.negotiatedCodes) > 0 {
		return m.negotiatedCodes
	}

	codes := make([]string, 0, len(m.codecs))
	for code := range m.codecs {
		codes = append(codes, code)
	}
	return codes
}

// GetAvailableCodes returns available codec codes.
func (m *CodecManager) GetAvailableCodes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	codes := m.getAvailableCodes()
	result := make([]string, len(codes))
	copy(result, codes)
	return result
}

// GetNegotiatedBitmap returns negotiated bitmap.
func (m *CodecManager) GetNegotiatedBitmap() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.negotiatedBitmap == nil {
		return nil
	}
	result := make([]byte, len(m.negotiatedBitmap))
	copy(result, m.negotiatedBitmap)
	return result
}

// IsNegotiated returns negotiation state.
func (m *CodecManager) IsNegotiated() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.negotiated
}

// GetMaxDepth returns maximum depth.
func (m *CodecManager) GetMaxDepth() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maxDepth
}

// GetCodec returns codec by code.
func (m *CodecManager) GetCodec(code string) (Codec, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	codec, exists := m.codecs[code]
	if !exists {
		return nil, ErrCodecNotFound
	}
	return codec, nil
}

// CodecCount returns registered codec count.
func (m *CodecManager) CodecCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.codecs)
}

// PreComputeHashes pre-computes hashes for O(1) lookup.
func (m *CodecManager) PreComputeHashes() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	codes := m.getAvailableCodes()
	if len(codes) == 0 {
		return ErrCodecNotFound
	}

	m.hashCache = make(map[uint32][]string)

	// Compute all combinations up to maxDepth
	for depth := 1; depth <= m.maxDepth; depth++ {
		combinations := generateCodeCombinations(codes, depth)
		for _, combo := range combinations {
			hash := m.computeChainHash(combo)
			m.hashCache[hashToUint32(hash)] = combo
		}
	}

	return nil
}

// Helper functions

// isBitSet checks if bit is set in bitmap.
func isBitSet(bitmap []byte, bit int) bool {
	if bitmap == nil || bit < 0 || bit >= len(bitmap)*8 {
		return false
	}
	byteIndex := bit / 8
	bitIndex := bit % 8
	return (bitmap[byteIndex] & (1 << bitIndex)) != 0
}

// generateCodeCombinations generates all code combinations of given depth.
func generateCodeCombinations(codes []string, depth int) [][]string {
	if depth == 0 || len(codes) == 0 {
		return [][]string{}
	}

	if depth == 1 {
		result := make([][]string, len(codes))
		for i, code := range codes {
			result[i] = []string{code}
		}
		return result
	}

	// Recursive combination generation (with repetition allowed)
	result := make([][]string, 0)
	for _, code := range codes {
		subCombos := generateCodeCombinations(codes, depth-1)
		for _, sub := range subCombos {
			combo := append([]string{code}, sub...)
			result = append(result, combo)
		}
	}

	return result
}
