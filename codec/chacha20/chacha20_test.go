// Package chacha20_test provides tests for ChaCha20-Poly1305 codec implementation.
package chacha20_test

import (
	"bytes"
	"testing"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/codec/chacha20"
	"github.com/Script-OS/VoidBus/keyprovider/embedded"
)

func TestChaCha20Codec_EncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x42}},
		{"short data", []byte("hello")},
		{"medium data", []byte("The quick brown fox jumps over the lazy dog")},
		{"long data", make([]byte, 1000)},
	}

	key, err := chacha20.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	kp, err := embedded.New(key, "", chacha20.Algorithm)
	if err != nil {
		t.Fatalf("Failed to create key provider: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := chacha20.New()
			if err := c.SetKeyProvider(kp); err != nil {
				t.Fatalf("SetKeyProvider failed: %v", err)
			}

			encoded, err := c.Encode(tt.data)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			// Ciphertext should be different from plaintext
			if len(tt.data) > 0 && bytes.Equal(tt.data, encoded) {
				t.Error("Encoded data should not equal original")
			}

			// Ciphertext should include overhead (nonce + tag)
			expectedLen := len(tt.data) + chacha20.NonceSize + chacha20.TagSize
			if len(encoded) != expectedLen {
				t.Errorf("Encoded length mismatch: got %d, want %d", len(encoded), expectedLen)
			}

			decoded, err := c.Decode(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			if !bytes.Equal(tt.data, decoded) {
				t.Errorf("Decode mismatch: got %v, want %v", decoded, tt.data)
			}
		})
	}
}

func TestChaCha20Codec_NoKey(t *testing.T) {
	c := chacha20.New()

	_, err := c.Encode([]byte("test"))
	if err == nil {
		t.Error("Encode should fail without key")
	}

	_, err = c.Decode([]byte("test"))
	if err == nil {
		t.Error("Decode should fail without key")
	}
}

func TestChaCha20Codec_InvalidKeySize(t *testing.T) {
	key := []byte("short") // Invalid key size

	kp, err := embedded.New(key, "", chacha20.Algorithm)
	if err != nil {
		t.Fatalf("Failed to create key provider: %v", err)
	}

	c := chacha20.New()
	if err := c.SetKeyProvider(kp); err != nil {
		t.Fatalf("SetKeyProvider failed: %v", err)
	}

	_, err = c.Encode([]byte("test"))
	if err == nil {
		t.Error("Encode should fail with invalid key size")
	}
}

func TestChaCha20Codec_Interface(t *testing.T) {
	c := chacha20.New()

	// Test Codec interface
	var _ codec.Codec = c

	// Test InternalID
	if c.InternalID() != "chacha20_poly1305" {
		t.Errorf("InternalID mismatch: got %s, want chacha20_poly1305", c.InternalID())
	}

	// Test SecurityLevel
	if c.SecurityLevel() != codec.SecurityLevelHigh {
		t.Errorf("SecurityLevel mismatch: got %d, want %d", c.SecurityLevel(), codec.SecurityLevelHigh)
	}

	// Test KeyAwareCodec interface
	var _ codec.KeyAwareCodec = c

	if !c.RequiresKey() {
		t.Error("RequiresKey should return true")
	}

	if c.KeyAlgorithm() != chacha20.Algorithm {
		t.Errorf("KeyAlgorithm mismatch: got %s, want %s", c.KeyAlgorithm(), chacha20.Algorithm)
	}
}

func TestChaCha20Codec_Module(t *testing.T) {
	m := chacha20.NewModule()

	// Test CodecModule interface
	var _ codec.CodecModule = m

	// Test Create
	c, err := m.Create(nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if c == nil {
		t.Error("Codec should not be nil")
	}

	// Test InternalID
	if m.InternalID() != "chacha20_poly1305" {
		t.Errorf("InternalID mismatch: got %s, want chacha20_poly1305", m.InternalID())
	}

	// Test SecurityLevel
	if m.SecurityLevel() != codec.SecurityLevelHigh {
		t.Errorf("SecurityLevel mismatch: got %d, want %d", m.SecurityLevel(), codec.SecurityLevelHigh)
	}
}

func TestGenerateKey(t *testing.T) {
	key, err := chacha20.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if len(key) != chacha20.KeySize {
		t.Errorf("Key size mismatch: got %d, want %d", len(key), chacha20.KeySize)
	}

	// Test that keys are different
	key2, _ := chacha20.GenerateKey()
	if bytes.Equal(key, key2) {
		t.Error("Generated keys should be different")
	}
}
