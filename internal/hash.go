// Package internal provides hash utilities for VoidBus v2.0.
package internal

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"strings"
)

// ComputeHash computes SHA256 hash of codec chain codes.
// Used for matching codec chain on receive side.
func ComputeHash(codeChain []string) [32]byte {
	concatenated := strings.Join(codeChain, "")
	return sha256.Sum256([]byte(concatenated))
}

// ComputeHashWithSalt computes SHA256 hash with salt for security.
// Prevents pre-computation attacks on codec chain discovery.
func ComputeHashWithSalt(codeChain []string, salt []byte) [32]byte {
	concatenated := strings.Join(codeChain, "")
	data := append(salt, []byte(concatenated)...)
	return sha256.Sum256(data)
}

// GenerateSalt generates random salt for hash computation.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 16)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, err
	}
	return salt, nil
}

// HashToBytes converts hash to byte slice.
func HashToBytes(hash [32]byte) []byte {
	return hash[:]
}

// BytesToHash converts byte slice to hash.
func BytesToHash(data []byte) [32]byte {
	if len(data) < 32 {
		var hash [32]byte
		copy(hash[:], data)
		return hash
	}
	var hash [32]byte
	copy(hash[:], data[:32])
	return hash
}

// HashCache provides pre-computed hash cache for O(1) lookup.
type HashCache struct {
	// cache maps hash to codec chain codes
	cache map[[32]byte][]string

	// reverseCache maps codes to hash (for quick encode)
	reverseCache map[string][32]byte

	// salt for hash computation
	salt []byte
}

// NewHashCache creates a new hash cache.
func NewHashCache() *HashCache {
	return &HashCache{
		cache:        make(map[[32]byte][]string),
		reverseCache: make(map[string][32]byte),
	}
}

// SetSalt sets the salt for hash computation.
func (c *HashCache) SetSalt(salt []byte) {
	c.salt = salt
}

// PreCompute pre-computes hash for all possible codec chain combinations.
// This converts O(n^d) runtime to O(1) lookup.
func (c *HashCache) PreCompute(supportedCodes []string, maxDepth int) {
	c.cache = make(map[[32]byte][]string)
	c.reverseCache = make(map[string][32]byte)

	// Generate all permutations for each depth
	for depth := 1; depth <= maxDepth; depth++ {
		perms := GeneratePermutations(supportedCodes, depth)
		for _, combo := range perms {
			var hash [32]byte
			if len(c.salt) > 0 {
				hash = ComputeHashWithSalt(combo, c.salt)
			} else {
				hash = ComputeHash(combo)
			}
			c.cache[hash] = combo
			c.reverseCache[strings.Join(combo, "")] = hash
		}
	}
}

// Lookup finds codec chain by hash (O(1) operation).
func (c *HashCache) Lookup(hash [32]byte) ([]string, bool) {
	codes, ok := c.cache[hash]
	return codes, ok
}

// GetHash gets pre-computed hash for codec chain (O(1) operation).
func (c *HashCache) GetHash(codes []string) ([32]byte, bool) {
	key := strings.Join(codes, "")
	hash, ok := c.reverseCache[key]
	return hash, ok
}

// Size returns cache size.
func (c *HashCache) Size() int {
	return len(c.cache)
}

// Clear clears the cache.
func (c *HashCache) Clear() {
	c.cache = make(map[[32]byte][]string)
	c.reverseCache = make(map[string][32]byte)
}

// ComputeDataHash computes SHA256 hash of data for integrity verification.
func ComputeDataHash(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// VerifyDataHash verifies data integrity against expected hash.
func VerifyDataHash(data []byte, expected [32]byte) bool {
	hash := ComputeDataHash(data)
	return hash == expected
}

// ComputeHashFromBytes computes SHA256 hash of byte data with optional salt.
func ComputeHashFromBytes(data, salt []byte) [32]byte {
	if len(salt) > 0 {
		combined := append(salt, data...)
		return sha256.Sum256(combined)
	}
	return sha256.Sum256(data)
}

// EncodeHash encodes hash to 4-byte compact format for metadata.
// Uses first 4 bytes of SHA256, sufficient for fragment matching.
func EncodeHashCompact(hash [32]byte) uint32 {
	return binary.BigEndian.Uint32(hash[:4])
}

// DecodeHashCompact decodes 4-byte compact hash.
func DecodeHashCompact(compact uint32) []byte {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, compact)
	return data
}
