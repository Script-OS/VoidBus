package base64

import (
	"encoding/base64"
	"testing"

	"github.com/Script-OS/VoidBus/codec"
)

func TestCodec_Encode(t *testing.T) {
	c := New()

	tests := []struct {
		name string
		data []byte
	}{
		{"nil data", nil},
		{"empty data", []byte{}},
		{"single byte", []byte{0x00}},
		{"hello", []byte("Hello")},
		{"binary data", []byte{0x00, 0x01, 0x02, 0x03, 0xFF}},
		{"large data", make([]byte, 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := c.Encode(tt.data)
			if err != nil {
				t.Errorf("Encode() error = %v", err)
				return
			}

			// Nil data should return nil
			if tt.data == nil {
				if result != nil {
					t.Errorf("Encode(nil) = %v, want nil", result)
				}
				return
			}

			// Verify encoding is correct
			expected := base64.StdEncoding.EncodeToString(tt.data)
			if string(result) != expected {
				t.Errorf("Encode() = %s, want %s", result, expected)
			}

			// Verify output length
			expectedLen := base64.StdEncoding.EncodedLen(len(tt.data))
			if len(result) != expectedLen {
				t.Errorf("Encode() length = %d, want %d", len(result), expectedLen)
			}
		})
	}
}

func TestCodec_Decode(t *testing.T) {
	c := New()

	tests := []struct {
		name     string
		data     []byte
		expected []byte
		hasError bool
	}{
		{"nil data", nil, nil, false},
		{"empty data", []byte{}, []byte{}, false},
		{"valid base64", []byte("SGVsbG8="), []byte("Hello"), false},
		{"valid base64 no padding", []byte("SGVsbG8g"), []byte("Hello "), false}, // Standard padding is needed
		{"binary data", []byte("AAECA/8="), []byte{0x00, 0x01, 0x02, 0x03, 0xFF}, false},
		{"invalid base64", []byte("not-valid-base64!!!"), nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := c.Decode(tt.data)

			if tt.hasError {
				if err == nil {
					t.Error("Decode() should return error for invalid data")
				}
				return
			}

			if err != nil {
				t.Errorf("Decode() error = %v", err)
				return
			}

			// Nil data should return nil
			if tt.data == nil {
				if result != nil {
					t.Errorf("Decode(nil) = %v, want nil", result)
				}
				return
			}

			if string(result) != string(tt.expected) {
				t.Errorf("Decode() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCodec_InternalID(t *testing.T) {
	c := New()

	if c.InternalID() != InternalID {
		t.Errorf("InternalID() = %s, want %s", c.InternalID(), InternalID)
	}

	if c.InternalID() != "base64" {
		t.Errorf("InternalID() should return 'base64'")
	}
}

func TestCodec_SecurityLevel(t *testing.T) {
	c := New()

	if c.SecurityLevel() != SecurityLevelValue {
		t.Errorf("SecurityLevel() = %d, want %d", c.SecurityLevel(), SecurityLevelValue)
	}

	if c.SecurityLevel() != codec.SecurityLevelLow {
		t.Errorf("SecurityLevel() should return SecurityLevelLow (1)")
	}
}

func TestCodec_Roundtrip(t *testing.T) {
	c := New()

	testData := [][]byte{
		[]byte("Hello, World!"),
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
		[]byte("The quick brown fox jumps over the lazy dog"),
		make([]byte, 256),
		make([]byte, 1024),
	}

	for i, data := range testData {
		encoded, err := c.Encode(data)
		if err != nil {
			t.Errorf("Test %d: Encode() error = %v", i, err)
			continue
		}

		decoded, err := c.Decode(encoded)
		if err != nil {
			t.Errorf("Test %d: Decode() error = %v", i, err)
			continue
		}

		if string(decoded) != string(data) {
			t.Errorf("Test %d: roundtrip failed", i)
		}
	}
}

func TestCodec_EncodeLength(t *testing.T) {
	c := New()

	// Test various input lengths
	tests := []struct {
		inputLen  int
		outputLen int
	}{
		{0, 0},
		{1, 4},
		{2, 4},
		{3, 4},
		{4, 8},
		{5, 8},
		{6, 8},
		{7, 12},
		{100, 136},
		{1024, 1368},
	}

	for _, tt := range tests {
		data := make([]byte, tt.inputLen)
		encoded, err := c.Encode(data)
		if err != nil {
			t.Errorf("Encode length %d error = %v", tt.inputLen, err)
			continue
		}

		if len(encoded) != tt.outputLen {
			t.Errorf("Encode(%d bytes) length = %d, want %d", tt.inputLen, len(encoded), tt.outputLen)
		}
	}
}

func TestModule_Create(t *testing.T) {
	m := NewModule()

	codecInst, err := m.Create(nil)
	if err != nil {
		t.Errorf("Create() error = %v", err)
		return
	}

	if codecInst == nil {
		t.Error("Create() should return non-nil codec")
	}

	if codecInst.InternalID() != InternalID {
		t.Errorf("Created codec InternalID() = %s, want %s", codecInst.InternalID(), InternalID)
	}
}

func TestModule_InternalID(t *testing.T) {
	m := NewModule()

	if m.InternalID() != InternalID {
		t.Errorf("Module.InternalID() = %s, want %s", m.InternalID(), InternalID)
	}
}

func TestModule_SecurityLevel(t *testing.T) {
	m := NewModule()

	if m.SecurityLevel() != SecurityLevelValue {
		t.Errorf("Module.SecurityLevel() = %d, want %d", m.SecurityLevel(), SecurityLevelValue)
	}
}
