// Package rsa_test provides tests for RSA codec implementation.
package rsa_test

import (
	"bytes"
	"testing"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/codec/rsa"
)

func TestRSACodec_EncodeDecode(t *testing.T) {
	// Generate key pair
	privKey, err := rsa.GenerateKey(rsa.DefaultKeySize)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Create codec with keys
	c := rsa.NewWithKeys(&privKey.PublicKey, privKey)

	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x42}},
		{"short data", []byte("hello")},
		{"medium data", []byte("The quick brown fox jumps")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := c.Encode(tt.data)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			// Ciphertext should be different from plaintext (unless empty)
			if len(tt.data) > 0 && bytes.Equal(tt.data, encoded) {
				t.Error("Encoded data should not equal original")
			}

			// Ciphertext size should match key size
			if len(encoded) != privKey.Size() && len(tt.data) > 0 {
				t.Errorf("Encoded length mismatch: got %d, want %d", len(encoded), privKey.Size())
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

func TestRSACodec_PlaintextTooLong(t *testing.T) {
	privKey, err := rsa.GenerateKey(rsa.DefaultKeySize)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	c := rsa.NewWithKeys(&privKey.PublicKey, privKey)

	// Get max plaintext size
	maxSize, err := c.MaxPlaintextSize()
	if err != nil {
		t.Fatalf("MaxPlaintextSize failed: %v", err)
	}

	// Create data larger than max
	largeData := make([]byte, maxSize+1)

	_, err = c.Encode(largeData)
	if err != rsa.ErrPlaintextTooLong {
		t.Errorf("Expected ErrPlaintextTooLong, got: %v", err)
	}
}

func TestRSACodec_NoKey(t *testing.T) {
	c := rsa.New()

	_, err := c.Encode([]byte("test"))
	if err == nil {
		t.Error("Encode should fail without key")
	}

	_, err = c.Decode([]byte("test"))
	if err == nil {
		t.Error("Decode should fail without key")
	}
}

func TestRSACodec_PublicKeyOnly(t *testing.T) {
	privKey, err := rsa.GenerateKey(rsa.DefaultKeySize)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Create codec with only public key
	c := rsa.New()
	c.SetPublicKey(&privKey.PublicKey)

	// Encode should work
	_, err = c.Encode([]byte("test"))
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode should fail (no private key)
	_, err = c.Decode([]byte("test"))
	if err == nil {
		t.Error("Decode should fail without private key")
	}
}

func TestRSACodec_Interface(t *testing.T) {
	c := rsa.New()

	// Test Codec interface
	var _ codec.Codec = c

	// Test InternalID
	if c.InternalID() != "rsa_oaep_sha256" {
		t.Errorf("InternalID mismatch: got %s, want rsa_oaep_sha256", c.InternalID())
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

	if c.KeyAlgorithm() != rsa.Algorithm {
		t.Errorf("KeyAlgorithm mismatch: got %s, want %s", c.KeyAlgorithm(), rsa.Algorithm)
	}
}

func TestRSACodec_Module(t *testing.T) {
	m := rsa.NewModule()

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
	if m.InternalID() != "rsa_oaep_sha256" {
		t.Errorf("InternalID mismatch: got %s, want rsa_oaep_sha256", m.InternalID())
	}

	// Test SecurityLevel
	if m.SecurityLevel() != codec.SecurityLevelHigh {
		t.Errorf("SecurityLevel mismatch: got %d, want %d", m.SecurityLevel(), codec.SecurityLevelHigh)
	}
}

func TestGenerateKey(t *testing.T) {
	key, err := rsa.GenerateKey(rsa.DefaultKeySize)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if key == nil {
		t.Fatal("Key should not be nil")
	}

	// Check key size
	expectedBits := rsa.DefaultKeySize
	actualBits := key.N.BitLen()
	if actualBits != expectedBits {
		t.Errorf("Key size mismatch: got %d bits, want %d bits", actualBits, expectedBits)
	}

	// Verify key is valid
	err = key.Validate()
	if err != nil {
		t.Errorf("Key validation failed: %v", err)
	}
}

func TestGenerateKey_Sizes(t *testing.T) {
	sizes := []int{rsa.DefaultKeySize, 3072, rsa.RecommendedKeySize}

	for _, size := range sizes {
		t.Run(string(rune(size)), func(t *testing.T) {
			key, err := rsa.GenerateKey(size)
			if err != nil {
				t.Fatalf("GenerateKey failed: %v", err)
			}

			if key.N.BitLen() != size {
				t.Errorf("Key size mismatch: got %d bits, want %d bits", key.N.BitLen(), size)
			}
		})
	}
}
