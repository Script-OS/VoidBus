package internal

import (
	"strings"
	"testing"
)

func TestGenerateID(t *testing.T) {
	id := GenerateID()

	// Check format: 8-4-4-4-12 hex characters
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Errorf("GenerateID() should return UUID format, got %s", id)
	}

	expectedLengths := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != expectedLengths[i] {
			t.Errorf("Part %d should have length %d, got %d", i, expectedLengths[i], len(part))
		}
	}

	// Test uniqueness
	id2 := GenerateID()
	if id == id2 {
		t.Error("GenerateID() should return unique IDs")
	}
}

func TestGenerateShortID(t *testing.T) {
	id := GenerateShortID()

	// Should return 8 hex characters
	if len(id) != 8 {
		t.Errorf("GenerateShortID() should return 8 characters, got %d", len(id))
	}

	// Test uniqueness
	id2 := GenerateShortID()
	if id == id2 {
		t.Error("GenerateShortID() should return unique IDs")
	}
}

func TestGenerateSessionID(t *testing.T) {
	id := GenerateSessionID()

	// Format: session-{timestamp}-{random}
	if !strings.HasPrefix(id, "session-") {
		t.Errorf("GenerateSessionID() should start with 'session-', got %s", id)
	}

	parts := strings.Split(id, "-")
	if len(parts) != 3 {
		t.Errorf("GenerateSessionID() should have 3 parts, got %d", len(parts))
	}

	// Test uniqueness
	id2 := GenerateSessionID()
	if id == id2 {
		t.Error("GenerateSessionID() should return unique IDs")
	}
}

func TestGenerateClientID(t *testing.T) {
	id := GenerateClientID()

	if !strings.HasPrefix(id, "client-") {
		t.Errorf("GenerateClientID() should start with 'client-', got %s", id)
	}

	// Test uniqueness
	id2 := GenerateClientID()
	if id == id2 {
		t.Error("GenerateClientID() should return unique IDs")
	}
}

func TestGenerateFragmentID(t *testing.T) {
	id := GenerateFragmentID()

	// Should return UUID format
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Errorf("GenerateFragmentID() should return UUID format, got %s", id)
	}

	// Test uniqueness
	id2 := GenerateFragmentID()
	if id == id2 {
		t.Error("GenerateFragmentID() should return unique IDs")
	}
}

func TestGenerateRandomBytes(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"zero length", 0},
		{"small length", 16},
		{"medium length", 32},
		{"large length", 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bytes, err := GenerateRandomBytes(tt.length)
			if err != nil {
				t.Errorf("GenerateRandomBytes() error = %v", err)
				return
			}
			if len(bytes) != tt.length {
				t.Errorf("GenerateRandomBytes() length = %d, want %d", len(bytes), tt.length)
			}
		})
	}
}

func TestGenerateChallenge(t *testing.T) {
	challenge, err := GenerateChallenge()
	if err != nil {
		t.Errorf("GenerateChallenge() error = %v", err)
		return
	}

	// Challenge should be 32 bytes
	if len(challenge) != 32 {
		t.Errorf("GenerateChallenge() length = %d, want 32", len(challenge))
	}

	// Test uniqueness
	challenge2, _ := GenerateChallenge()
	if string(challenge) == string(challenge2) {
		t.Error("GenerateChallenge() should return unique challenges")
	}
}

func TestEncodeDecodeHex(t *testing.T) {
	data := []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f} // "Hello"

	encoded := EncodeHex(data)
	expected := "48656c6c6f"
	if encoded != expected {
		t.Errorf("EncodeHex() = %s, want %s", encoded, expected)
	}

	decoded, err := DecodeHex(encoded)
	if err != nil {
		t.Errorf("DecodeHex() error = %v", err)
		return
	}

	if string(decoded) != string(data) {
		t.Errorf("DecodeHex() = %v, want %v", decoded, data)
	}
}

func TestDecodeHexInvalid(t *testing.T) {
	_, err := DecodeHex("invalid-hex-string!")
	if err == nil {
		t.Error("DecodeHex() should return error for invalid hex string")
	}
}

func TestParseSessionID(t *testing.T) {
	// Test valid session ID
	sessionID := GenerateSessionID()
	timestamp, random, ok := ParseSessionID(sessionID)
	if !ok {
		t.Errorf("ParseSessionID() should parse valid session ID: %s", sessionID)
	}
	if timestamp == 0 {
		t.Error("ParseSessionID() timestamp should not be zero")
	}
	if random == "" {
		t.Error("ParseSessionID() random should not be empty")
	}

	// Test invalid session IDs
	tests := []struct {
		name string
		id   string
	}{
		{"empty string", ""},
		{"wrong prefix", "client-1234567890-abcd"},
		{"missing parts", "session-1234567890"},
		{"too many parts", "session-1234567890-abcd-extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ok := ParseSessionID(tt.id)
			if ok {
				t.Errorf("ParseSessionID() should fail for invalid ID: %s", tt.id)
			}
		})
	}
}
