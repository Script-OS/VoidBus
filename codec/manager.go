// Package codec provides CodecManager for v2.1 architecture.
//
// CodecManager handles random codec selection based on bitmap negotiation.
// After negotiation, both sides have the same CodecBitmap and dynamically
// select codec chains independently.
package codec

import (
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/internal"
)

// CodecID represents a codec identifier (bit position in bitmap).
type CodecID int

// Standard codec IDs (bit positions in CodecBitmap).
const (
	CodecIDPlain    CodecID = 0
	CodecIDBase64   CodecID = 1
	CodecIDAES256   CodecID = 2
	CodecIDXOR      CodecID = 3
	CodecIDChaCha20 CodecID = 4
	CodecIDRSA      CodecID = 5
	CodecIDGZIP     CodecID = 6
	CodecIDZSTD     CodecID = 7
)

// CodecManager manages codec instances and provides bitmap-based selection.
type CodecManager struct {
	mu sync.RWMutex

	// Available codecs after negotiation
	availableCodecs map[CodecID]Codec

	// Negotiated bitmap
	negotiatedBitmap []byte

	// Maximum codec chain depth
	maxDepth int

	// Salt for hash computation
	salt []byte

	// Hash cache for O(1) lookup
	hashCache map[uint32][]CodecID

	// Random source
	rand *rand.Rand

	// Negotiation state
	negotiated bool
}

// NewCodecManager creates a new CodecManager.
func NewCodecManager() *CodecManager {
	return &CodecManager{
		availableCodecs: make(map[CodecID]Codec),
		maxDepth:        5,
		hashCache:       make(map[uint32][]CodecID),
		rand:            rand.New(rand.NewSource(time.Now().UnixNano())),
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
		CodecCount:    len(m.availableCodecs),
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

// RegisterCodec registers a codec with its ID.
// This should be called before negotiation.
func (m *CodecManager) RegisterCodec(codec Codec, id CodecID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if codec == nil {
		return errors.New("codec cannot be nil")
	}

	m.availableCodecs[id] = codec
	return nil
}

// SetNegotiatedBitmap sets the negotiated codec bitmap.
// This is called after handshake negotiation.
func (m *CodecManager) SetNegotiatedBitmap(bitmap []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Filter available codecs by bitmap
	filtered := make(map[CodecID]Codec)
	for id, codec := range m.availableCodecs {
		if isBitSet(bitmap, int(id)) {
			filtered[id] = codec
		}
	}

	if len(filtered) == 0 {
		return errors.New("no available codecs after negotiation")
	}

	m.availableCodecs = filtered
	m.negotiatedBitmap = bitmap
	m.negotiated = true

	// Clear hash cache (will be rebuilt on demand)
	m.hashCache = make(map[uint32][]CodecID)

	return nil
}

// SetMaxDepth sets maximum codec chain depth.
func (m *CodecManager) SetMaxDepth(depth int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if depth < 1 || depth > 5 {
		return errors.New("max depth must be 1-5")
	}

	m.maxDepth = depth
	m.hashCache = make(map[uint32][]CodecID)
	return nil
}

// SetSalt sets salt for hash computation.
func (m *CodecManager) SetSalt(salt []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.salt = salt
	m.hashCache = make(map[uint32][]CodecID)
}

// SelectChain randomly selects a codec chain.
// Returns: codec chain, hash for matching
func (m *CodecManager) SelectChain() (CodecChain, [32]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.availableCodecs) == 0 {
		return nil, [32]byte{}, ErrCodecNotFound
	}

	// Random depth (1 to maxDepth)
	depth := m.rand.Intn(m.maxDepth) + 1

	// Get available codec IDs
	ids := make([]CodecID, 0, len(m.availableCodecs))
	for id := range m.availableCodecs {
		ids = append(ids, id)
	}

	// Random selection (with possible repetition)
	selected := make([]CodecID, depth)
	for i := 0; i < depth; i++ {
		selected[i] = ids[m.rand.Intn(len(ids))]
	}

	// Create chain
	chain := NewChain()
	for _, id := range selected {
		codec, exists := m.availableCodecs[id]
		if !exists {
			return nil, [32]byte{}, ErrCodecNotFound
		}
		chain.AddCodec(codec)
	}

	// Compute hash (SHA256 for [32]byte)
	hash := m.computeChainHash(selected)

	// Cache for later matching
	m.hashCache[hashToUint32(hash)] = selected

	return chain, hash, nil
}

// MatchChain finds codec chain by hash.
func (m *CodecManager) MatchChain(hash [32]byte) (CodecChain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check cache
	hashKey := hashToUint32(hash)
	if ids, exists := m.hashCache[hashKey]; exists {
		return m.createChainFromIDs(ids), nil
	}

	// Try all combinations (fallback)
	for depth := 1; depth <= m.maxDepth; depth++ {
		ids := make([]CodecID, 0, len(m.availableCodecs))
		for id := range m.availableCodecs {
			ids = append(ids, id)
		}

		combinations := generateCombinations(ids, depth)
		for _, combo := range combinations {
			expectedHash := m.computeChainHash(combo)
			if expectedHash == hash {
				// Cache for future use
				m.hashCache[hashKey] = combo
				return m.createChainFromIDs(combo), nil
			}
		}
	}

	return nil, ErrCodecChainMismatch
}

// computeChainHash computes SHA256 hash for codec chain.
func (m *CodecManager) computeChainHash(ids []CodecID) [32]byte {
	// Convert IDs to byte slice
	data := make([]byte, len(ids))
	for i, id := range ids {
		data[i] = byte(id)
	}

	// Use SHA256 for [32]byte hash
	return internal.ComputeHashFromBytes(data, m.salt)
}

// hashToUint32 converts [32]byte to uint32 for cache key.
func hashToUint32(hash [32]byte) uint32 {
	return uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])
}

// createChainFromIDs creates chain from codec IDs.
func (m *CodecManager) createChainFromIDs(ids []CodecID) CodecChain {
	chain := NewChain()
	for _, id := range ids {
		if codec, exists := m.availableCodecs[id]; exists {
			chain.AddCodec(codec)
		}
	}
	return chain
}

// GetAvailableCodecs returns available codec IDs.
func (m *CodecManager) GetAvailableCodecs() []CodecID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]CodecID, 0, len(m.availableCodecs))
	for id := range m.availableCodecs {
		ids = append(ids, id)
	}
	return ids
}

// GetNegotiatedBitmap returns negotiated bitmap.
func (m *CodecManager) GetNegotiatedBitmap() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

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

// GetCodec returns codec by ID.
func (m *CodecManager) GetCodec(id CodecID) (Codec, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	codec, exists := m.availableCodecs[id]
	if !exists {
		return nil, ErrCodecNotFound
	}
	return codec, nil
}

// CodecCount returns available codec count.
func (m *CodecManager) CodecCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.availableCodecs)
}

// PreComputeHashes pre-computes hashes for O(1) lookup.
func (m *CodecManager) PreComputeHashes() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.availableCodecs) == 0 {
		return ErrCodecNotFound
	}

	m.hashCache = make(map[uint32][]CodecID)

	ids := make([]CodecID, 0, len(m.availableCodecs))
	for id := range m.availableCodecs {
		ids = append(ids, id)
	}

	// Compute all combinations up to maxDepth
	for depth := 1; depth <= m.maxDepth; depth++ {
		combinations := generateCombinations(ids, depth)
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
	if bit < 0 || bit >= len(bitmap)*8 {
		return false
	}
	byteIndex := bit / 8
	bitIndex := bit % 8
	return (bitmap[byteIndex] & (1 << bitIndex)) != 0
}

// generateCombinations generates all combinations of given depth.
func generateCombinations(ids []CodecID, depth int) [][]CodecID {
	if depth == 0 {
		return [][]CodecID{}
	}

	if depth == 1 {
		result := make([][]CodecID, len(ids))
		for i, id := range ids {
			result[i] = []CodecID{id}
		}
		return result
	}

	// Recursive combination generation
	result := make([][]CodecID, 0)
	for _, id := range ids {
		subCombos := generateCombinations(ids, depth-1)
		for _, sub := range subCombos {
			combo := append([]CodecID{id}, sub...)
			result = append(result, combo)
		}
	}

	return result
}
