// Package internal provides internal utilities for VoidBus.
package internal

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// GenerateID generates a unique identifier.
// Returns a random UUID v4 format string.
func GenerateID() string {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())
	}

	// Set version (4) and variant bits
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 10

	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

// GenerateShortID generates a short unique identifier.
// Returns first 8 characters of hash-based ID.
func GenerateShortID() string {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, uint64(time.Now().UnixNano()))
	rand.Read(data)

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:4])
}

// GenerateSessionID generates a session identifier.
// Format: "session-{timestamp}-{random}"
func GenerateSessionID() string {
	timestamp := time.Now().Unix()
	random := GenerateShortID()
	return fmt.Sprintf("session-%d-%s", timestamp, random)
}

// GenerateClientID generates a client identifier.
// Format: "client-{random}"
func GenerateClientID() string {
	return "client-" + GenerateShortID()
}

// GenerateFragmentID generates a fragment group identifier.
// Returns a random UUID format string for fragment group tracking.
func GenerateFragmentID() string {
	return GenerateID()
}

// GenerateRandomBytes generates random bytes of specified length.
func GenerateRandomBytes(length int) ([]byte, error) {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

// GenerateChallenge generates a challenge for handshake verification.
// Returns 32 bytes of random data.
func GenerateChallenge() ([]byte, error) {
	return GenerateRandomBytes(32)
}

// EncodeHex encodes bytes to hex string.
func EncodeHex(data []byte) string {
	return hex.EncodeToString(data)
}

// DecodeHex decodes hex string to bytes.
func DecodeHex(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// ParseID extracts components from session ID.
func ParseSessionID(sessionID string) (timestamp int64, random string, ok bool) {
	parts := strings.Split(sessionID, "-")
	if len(parts) != 3 || parts[0] != "session" {
		return 0, "", false
	}

	var ts int64
	fmt.Sscanf(parts[1], "%d", &ts)
	return ts, parts[2], true
}
