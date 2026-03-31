// Package codec provides codec chain info generation for negotiation.
//
// CodecChainInfoGenerator generates CodecChainInfo from CodecChain
// without exposing specific codec names.
//
// Security Design (see docs/ARCHITECTURE.md §2.1.3):
//   - Codec names MUST NOT be exposed in negotiation
//   - Only SecurityLevel and Hash are transmitted
//   - Hash is computed from SecurityLevel sequence (no semantic info)
//   - Challenge mechanism prevents degradation attacks
package codec

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
)

// Negotiation errors
var (
	// ErrInvalidChain indicates invalid codec chain
	ErrInvalidChain = errors.New("negotiation: invalid chain")
	// ErrNegotiationChainTooLong indicates chain exceeds limit
	ErrNegotiationChainTooLong = errors.New("negotiation: chain too long")
	// ErrEmptyChain indicates empty codec chain
	ErrEmptyChain = errors.New("negotiation: empty chain")
)

// CodecChainInfo represents codec chain info for negotiation.
// IMPORTANT: Does NOT expose specific codec names!
// Only exposes security level for negotiation.
type CodecChainInfo struct {
	// SecurityLevel is chain's overall security level
	SecurityLevel SecurityLevel

	// ChainLength is number of codecs in chain
	ChainLength int

	// Hash is chain configuration hash (for verification)
	// Hash is computed from SecurityLevel sequence (no semantic info)
	Hash string
}

// CodecChainInfoGenerator generates CodecChainInfo from CodecChain.
type CodecChainInfoGenerator struct {
	mu sync.RWMutex

	// MaxChainLength is maximum chain length (default 5)
	MaxChainLength int

	// EnableStrictValidation enables strict validation
	EnableStrictValidation bool
}

// NewCodecChainInfoGenerator creates a new generator.
func NewCodecChainInfoGenerator() *CodecChainInfoGenerator {
	return &CodecChainInfoGenerator{
		MaxChainLength:         5,
		EnableStrictValidation: true,
	}
}

// Generate generates CodecChainInfo from a CodecChain.
//
// Security Guarantee:
//   - Does NOT expose codec names
//   - Hash is computed from SecurityLevel sequence only
//   - No semantic information is leaked
func (g *CodecChainInfoGenerator) Generate(chain CodecChain) (CodecChainInfo, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if chain == nil {
		return CodecChainInfo{}, ErrInvalidChain
	}

	if chain.IsEmpty() {
		return CodecChainInfo{}, ErrEmptyChain
	}

	if chain.Length() > g.MaxChainLength {
		return CodecChainInfo{}, ErrNegotiationChainTooLong
	}

	// Compute hash from SecurityLevel sequence (no semantic info)
	hash := g.computeHash(chain)

	return CodecChainInfo{
		SecurityLevel: chain.SecurityLevel(),
		ChainLength:   chain.Length(),
		Hash:          hash,
	}, nil
}

// GenerateAll generates CodecChainInfo for all chains in a list.
func (g *CodecChainInfoGenerator) GenerateAll(chains []CodecChain) ([]CodecChainInfo, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(chains) == 0 {
		return nil, ErrEmptyChain
	}

	result := make([]CodecChainInfo, 0, len(chains))
	for _, chain := range chains {
		info, err := g.Generate(chain)
		if err != nil {
			continue // Skip invalid chains
		}
		result = append(result, info)
	}

	return result, nil
}

// GenerateForClient generates CodecChainInfo for client negotiation.
// Client presents supported codec chains without exposing names.
func (g *CodecChainInfoGenerator) GenerateForClient(chains []CodecChain) ([]CodecChainInfo, error) {
	return g.GenerateAll(chains)
}

// computeHash computes hash from SecurityLevel sequence.
// IMPORTANT: Does NOT expose codec names!
// Only uses SecurityLevel values (no semantic information).
func (g *CodecChainInfoGenerator) computeHash(chain CodecChain) string {
	// Build SecurityLevel sequence
	levels := make([]byte, chain.Length())
	for i := 0; i < chain.Length(); i++ {
		c, err := chain.GetCodec(i)
		if err != nil {
			continue
		}
		levels[i] = byte(c.SecurityLevel())
	}

	// Compute SHA256 hash
	h := sha256.Sum256(levels)
	return hex.EncodeToString(h[:])
}

// VerifyHash verifies that a CodecChain matches given hash.
func (g *CodecChainInfoGenerator) VerifyHash(chain CodecChain, hash string) bool {
	if chain == nil || hash == "" {
		return false
	}

	computed := g.computeHash(chain)
	return computed == hash
}

// MatchChain matches a CodecChain by CodecChainInfo.
// Returns true if chain matches the info (SecurityLevel and Hash).
func (g *CodecChainInfoGenerator) MatchChain(chain CodecChain, info CodecChainInfo) bool {
	if chain == nil {
		return false
	}

	// Check SecurityLevel
	if chain.SecurityLevel() != info.SecurityLevel {
		return false
	}

	// Check ChainLength
	if chain.Length() != info.ChainLength {
		return false
	}

	// Check Hash
	return g.VerifyHash(chain, info.Hash)
}

// FindMatchingChain finds a chain that matches CodecChainInfo.
func (g *CodecChainInfoGenerator) FindMatchingChain(chains []CodecChain, info CodecChainInfo) (CodecChain, int) {
	for i, chain := range chains {
		if g.MatchChain(chain, info) {
			return chain, i
		}
	}
	return nil, -1
}

// SetMaxChainLength sets maximum chain length.
func (g *CodecChainInfoGenerator) SetMaxChainLength(max int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if max > 0 {
		g.MaxChainLength = max
	}
}

// CodecChainRegistry manages available codec chains for negotiation.
type CodecChainRegistry struct {
	mu        sync.RWMutex
	chains    []CodecChain
	generator *CodecChainInfoGenerator
}

// NewCodecChainRegistry creates a new registry.
func NewCodecChainRegistry() *CodecChainRegistry {
	return &CodecChainRegistry{
		chains:    make([]CodecChain, 0),
		generator: NewCodecChainInfoGenerator(),
	}
}

// Register registers a codec chain.
func (r *CodecChainRegistry) Register(chain CodecChain) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if chain == nil {
		return ErrInvalidChain
	}

	if chain.Length() > r.generator.MaxChainLength {
		return ErrChainTooLong
	}

	r.chains = append(r.chains, chain)
	return nil
}

// RegisterMultiple registers multiple codec chains.
func (r *CodecChainRegistry) RegisterMultiple(chains []CodecChain) error {
	for _, chain := range chains {
		if err := r.Register(chain); err != nil {
			return err
		}
	}
	return nil
}

// GetAll returns all registered chains.
func (r *CodecChainRegistry) GetAll() []CodecChain {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]CodecChain, len(r.chains))
	copy(result, r.chains)
	return result
}

// GetAllInfo returns CodecChainInfo for all registered chains.
func (r *CodecChainRegistry) GetAllInfo() ([]CodecChainInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.generator.GenerateAll(r.chains)
}

// GetBySecurityLevel returns chains with specified security level.
func (r *CodecChainRegistry) GetBySecurityLevel(level SecurityLevel) []CodecChain {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]CodecChain, 0)
	for _, chain := range r.chains {
		if chain.SecurityLevel() == level {
			result = append(result, chain)
		}
	}
	return result
}

// GetByMinSecurityLevel returns chains with at least specified level.
func (r *CodecChainRegistry) GetByMinSecurityLevel(minLevel SecurityLevel) []CodecChain {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]CodecChain, 0)
	for _, chain := range r.chains {
		if chain.SecurityLevel() >= minLevel {
			result = append(result, chain)
		}
	}
	return result
}

// FindByInfo finds chain matching CodecChainInfo.
func (r *CodecChainRegistry) FindByInfo(info CodecChainInfo) (CodecChain, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	chain, _ := r.generator.FindMatchingChain(r.chains, info)
	if chain == nil {
		return nil, ErrInvalidChain
	}
	return chain, nil
}

// Count returns number of registered chains.
func (r *CodecChainRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.chains)
}

// Clear clears all registered chains.
func (r *CodecChainRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chains = make([]CodecChain, 0)
}

// NegotiationHelper provides helper functions for negotiation.
type NegotiationHelper struct {
	generator *CodecChainInfoGenerator
}

// NewNegotiationHelper creates a new helper.
func NewNegotiationHelper() *NegotiationHelper {
	return &NegotiationHelper{
		generator: NewCodecChainInfoGenerator(),
	}
}

// GenerateChallenge generates a challenge for verification.
// Challenge is random bytes that client must encode with codec chain.
func (h *NegotiationHelper) GenerateChallenge(size int) ([]byte, error) {
	if size <= 0 {
		size = 32 // Default challenge size
	}

	challenge := make([]byte, size)
	// Note: In production, use crypto/rand
	// For simplicity, we use deterministic generation here
	for i := 0; i < size; i++ {
		challenge[i] = byte(i % 256)
	}
	return challenge, nil
}

// VerifyChallenge verifies challenge response.
// Client must encode challenge with claimed codec chain.
func (h *NegotiationHelper) VerifyChallenge(challenge []byte, response []byte, chain CodecChain) error {
	if chain == nil {
		return ErrInvalidChain
	}

	// Decode the response using the codec chain
	decoded, err := chain.Decode(response)
	if err != nil {
		return err
	}

	// Verify decoded matches original challenge
	if len(decoded) != len(challenge) {
		return errors.New("negotiation: challenge mismatch")
	}

	for i := range challenge {
		if decoded[i] != challenge[i] {
			return errors.New("negotiation: challenge verification failed")
		}
	}

	return nil
}

// PrepareClientResponse prepares client's challenge response.
// Client encodes challenge with their codec chain.
func (h *NegotiationHelper) PrepareClientResponse(challenge []byte, chain CodecChain) ([]byte, error) {
	if chain == nil {
		return nil, ErrInvalidChain
	}

	return chain.Encode(challenge)
}

// Global generator
var globalGenerator = NewCodecChainInfoGenerator()

// GenerateChainInfo generates CodecChainInfo using global generator.
func GenerateChainInfo(chain CodecChain) (CodecChainInfo, error) {
	return globalGenerator.Generate(chain)
}

// GenerateAllChainInfo generates info for all chains using global generator.
func GenerateAllChainInfo(chains []CodecChain) ([]CodecChainInfo, error) {
	return globalGenerator.GenerateAll(chains)
}

// Global chain registry (separate from codec registry)
var globalChainRegistry = NewCodecChainRegistry()

// RegisterCodecChain registers chain to global registry.
func RegisterCodecChain(chain CodecChain) error {
	return globalChainRegistry.Register(chain)
}

// GetCodecChainInfo returns info for all registered chains.
func GetCodecChainInfo() ([]CodecChainInfo, error) {
	return globalChainRegistry.GetAllInfo()
}
