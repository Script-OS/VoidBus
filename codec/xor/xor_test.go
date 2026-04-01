// Package xor_test provides tests for XOR codec implementation.
package xor_test

import (
	"bytes"
	"testing"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/codec/xor"
	"github.com/Script-OS/VoidBus/keyprovider/embedded"
)

func TestXORCodec_EncodeDecode(t *testing.T) {
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

	key, err := xor.GenerateKey(32)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	kp, err := embedded.New(key, "", "XOR")
	if err != nil {
		t.Fatalf("Failed to create key provider: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := xor.New()
			if err := c.SetKeyProvider(kp); err != nil {
				t.Fatalf("SetKeyProvider failed: %v", err)
			}

			encoded, err := c.Encode(tt.data)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			// XOR should change the data (unless key is all zeros, which is unlikely)
			if len(tt.data) > 0 && bytes.Equal(tt.data, encoded) {
				// This is possible but extremely unlikely with random key
				t.Logf("Warning: encoded data equals original (possible but unlikely)")
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

func TestXORCodec_NoKey(t *testing.T) {
	c := xor.New()

	_, err := c.Encode([]byte("test"))
	if err == nil {
		t.Error("Encode should fail without key")
	}

	_, err = c.Decode([]byte("test"))
	if err == nil {
		t.Error("Decode should fail without key")
	}
}

func TestXORCodec_Interface(t *testing.T) {
	c := xor.New()

	// Test Codec interface
	var _ codec.Codec = c

	// Test InternalID
	if c.InternalID() != "xor" {
		t.Errorf("InternalID mismatch: got %s, want xor", c.InternalID())
	}

	// Test SecurityLevel
	if c.SecurityLevel() != codec.SecurityLevelMedium {
		t.Errorf("SecurityLevel mismatch: got %d, want %d", c.SecurityLevel(), codec.SecurityLevelMedium)
	}

	// Test KeyAwareCodec interface
	var _ codec.KeyAwareCodec = c

	if !c.RequiresKey() {
		t.Error("RequiresKey should return true")
	}

	if c.KeyAlgorithm() != "XOR" {
		t.Errorf("KeyAlgorithm mismatch: got %s, want XOR", c.KeyAlgorithm())
	}
}

func TestXORCodec_KeySize(t *testing.T) {
	// Test valid key sizes
	for _, size := range []int{8, 16, 32, 64, 128, 256} {
		t.Run("valid_"+string(rune(size)), func(t *testing.T) {
			c, err := xor.NewWithKeySize(size)
			if err != nil {
				t.Fatalf("NewWithKeySize failed: %v", err)
			}
			if c == nil {
				t.Error("Codec should not be nil")
			}
		})
	}

	// Test invalid key sizes
	for _, size := range []int{0, 1, 7, 257, 1000} {
		t.Run("invalid_"+string(rune(size)), func(t *testing.T) {
			_, err := xor.NewWithKeySize(size)
			if err == nil {
				t.Error("NewWithKeySize should fail for invalid size")
			}
		})
	}
}

func TestXORCodec_Module(t *testing.T) {
	m := xor.NewModule()

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
	if m.InternalID() != "xor" {
		t.Errorf("InternalID mismatch: got %s, want xor", m.InternalID())
	}

	// Test SecurityLevel
	if m.SecurityLevel() != codec.SecurityLevelMedium {
		t.Errorf("SecurityLevel mismatch: got %d, want %d", m.SecurityLevel(), codec.SecurityLevelMedium)
	}
}

func TestGenerateKey(t *testing.T) {
	// Test valid sizes
	for _, size := range []int{8, 16, 32, 64} {
		key, err := xor.GenerateKey(size)
		if err != nil {
			t.Fatalf("GenerateKey failed: %v", err)
		}
		if len(key) != size {
			t.Errorf("Key size mismatch: got %d, want %d", len(key), size)
		}
	}

	// Test that keys are different
	key1, _ := xor.GenerateKey(32)
	key2, _ := xor.GenerateKey(32)
	if bytes.Equal(key1, key2) {
		t.Error("Generated keys should be different")
	}
}
