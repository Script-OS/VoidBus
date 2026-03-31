package plain

import (
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
		{"multiple bytes", []byte("Hello, World!")},
		{"large data", make([]byte, 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := c.Encode(tt.data)
			if err != nil {
				t.Errorf("Encode() error = %v", err)
				return
			}

			// Should return copy of input data (pass-through)
			if tt.data == nil {
				if result != nil {
					t.Errorf("Encode(nil) = %v, want nil", result)
				}
			} else {
				if string(result) != string(tt.data) {
					t.Errorf("Encode() = %v, want %v", result, tt.data)
				}

				// Verify it's a copy
				if len(result) > 0 {
					result[0] = 0xFF
					if len(tt.data) > 0 && tt.data[0] == 0xFF {
						t.Error("Encode() should return a copy")
					}
				}
			}
		})
	}
}

func TestCodec_Decode(t *testing.T) {
	c := New()

	tests := []struct {
		name string
		data []byte
	}{
		{"nil data", nil},
		{"empty data", []byte{}},
		{"single byte", []byte{0x00}},
		{"multiple bytes", []byte("Hello, World!")},
		{"large data", make([]byte, 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := c.Decode(tt.data)
			if err != nil {
				t.Errorf("Decode() error = %v", err)
				return
			}

			// Should return copy of input data (pass-through)
			if tt.data == nil {
				if result != nil {
					t.Errorf("Decode(nil) = %v, want nil", result)
				}
			} else {
				if string(result) != string(tt.data) {
					t.Errorf("Decode() = %v, want %v", result, tt.data)
				}

				// Verify it's a copy
				if len(result) > 0 {
					result[0] = 0xFF
					if len(tt.data) > 0 && tt.data[0] == 0xFF {
						t.Error("Decode() should return a copy")
					}
				}
			}
		})
	}
}

func TestCodec_InternalID(t *testing.T) {
	c := New()

	if c.InternalID() != InternalID {
		t.Errorf("InternalID() = %s, want %s", c.InternalID(), InternalID)
	}

	if c.InternalID() != "plain" {
		t.Errorf("InternalID() should return 'plain'")
	}
}

func TestCodec_SecurityLevel(t *testing.T) {
	c := New()

	if c.SecurityLevel() != SecurityLevelValue {
		t.Errorf("SecurityLevel() = %d, want %d", c.SecurityLevel(), SecurityLevelValue)
	}

	if c.SecurityLevel() != codec.SecurityLevelNone {
		t.Errorf("SecurityLevel() should return SecurityLevelNone (0)")
	}
}

func TestCodec_Roundtrip(t *testing.T) {
	c := New()

	testData := [][]byte{
		[]byte("Hello, World!"),
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
		make([]byte, 256),
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
