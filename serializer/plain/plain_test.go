package plain

import (
	"testing"
)

func TestSerializer_Serialize(t *testing.T) {
	s := New()

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
			result, err := s.Serialize(tt.data)
			if err != nil {
				t.Errorf("Serialize() error = %v", err)
				return
			}

			// Should return copy of input data
			if tt.data == nil {
				if result != nil {
					t.Errorf("Serialize(nil) = %v, want nil", result)
				}
			} else {
				if string(result) != string(tt.data) {
					t.Errorf("Serialize() = %v, want %v", result, tt.data)
				}

				// Verify it's a copy (modification should not affect original)
				if len(result) > 0 {
					result[0] = 0xFF
					if len(tt.data) > 0 && tt.data[0] == 0xFF {
						t.Error("Serialize() should return a copy, not reference")
					}
				}
			}
		})
	}
}

func TestSerializer_Deserialize(t *testing.T) {
	s := New()

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
			result, err := s.Deserialize(tt.data)
			if err != nil {
				t.Errorf("Deserialize() error = %v", err)
				return
			}

			// Should return copy of input data
			if tt.data == nil {
				if result != nil {
					t.Errorf("Deserialize(nil) = %v, want nil", result)
				}
			} else {
				if string(result) != string(tt.data) {
					t.Errorf("Deserialize() = %v, want %v", result, tt.data)
				}

				// Verify it's a copy (modification should not affect original)
				if len(result) > 0 {
					result[0] = 0xFF
					if len(tt.data) > 0 && tt.data[0] == 0xFF {
						t.Error("Deserialize() should return a copy, not reference")
					}
				}
			}
		})
	}
}

func TestSerializer_Name(t *testing.T) {
	s := New()

	if s.Name() != Name {
		t.Errorf("Name() = %s, want %s", s.Name(), Name)
	}

	if s.Name() != "plain" {
		t.Errorf("Name() should return 'plain'")
	}
}

func TestSerializer_Priority(t *testing.T) {
	s := New()

	if s.Priority() != Priority {
		t.Errorf("Priority() = %d, want %d", s.Priority(), Priority)
	}

	if s.Priority() != 0 {
		t.Errorf("Priority() should return 0 (lowest)")
	}
}

func TestSerializer_Roundtrip(t *testing.T) {
	s := New()

	testData := [][]byte{
		[]byte("Hello, World!"),
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
		make([]byte, 256),
	}

	for i, data := range testData {
		serialized, err := s.Serialize(data)
		if err != nil {
			t.Errorf("Test %d: Serialize() error = %v", i, err)
			continue
		}

		deserialized, err := s.Deserialize(serialized)
		if err != nil {
			t.Errorf("Test %d: Deserialize() error = %v", i, err)
			continue
		}

		if string(deserialized) != string(data) {
			t.Errorf("Test %d: roundtrip failed", i)
		}
	}
}

func TestModule_Create(t *testing.T) {
	m := NewModule()

	serializer, err := m.Create(nil)
	if err != nil {
		t.Errorf("Create() error = %v", err)
		return
	}

	if serializer == nil {
		t.Error("Create() should return non-nil serializer")
	}

	if serializer.Name() != Name {
		t.Errorf("Created serializer Name() = %s, want %s", serializer.Name(), Name)
	}
}

func TestModule_Name(t *testing.T) {
	m := NewModule()

	if m.Name() != Name {
		t.Errorf("Module.Name() = %s, want %s", m.Name(), Name)
	}
}
