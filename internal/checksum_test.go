package internal

import (
	"testing"
)

func TestCalculateChecksum(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty data", []byte{}},
		{"single byte", []byte{0x00}},
		{"hello", []byte("hello")},
		{"Hello", []byte("Hello")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that checksum is deterministic
			result1 := CalculateChecksum(tt.data)
			result2 := CalculateChecksum(tt.data)
			if result1 != result2 {
				t.Errorf("CalculateChecksum() should be deterministic")
			}

			// Test that different data produces different checksum
			if len(tt.data) > 0 {
				modified := make([]byte, len(tt.data))
				copy(modified, tt.data)
				modified[0] ^= 0xFF
				modifiedResult := CalculateChecksum(modified)
				if modifiedResult == result1 {
					t.Errorf("Different data should produce different checksum")
				}
			}
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("test data")
	checksum := CalculateChecksum(data)

	if !VerifyChecksum(data, checksum) {
		t.Error("VerifyChecksum() should return true for valid checksum")
	}

	// Test with wrong checksum
	if VerifyChecksum(data, checksum+1) {
		t.Error("VerifyChecksum() should return false for invalid checksum")
	}

	// Test with modified data
	modifiedData := []byte("test datA")
	if VerifyChecksum(modifiedData, checksum) {
		t.Error("VerifyChecksum() should return false for modified data")
	}
}

func TestCalculateChecksumBytes(t *testing.T) {
	data := []byte("test")
	bytes := CalculateChecksumBytes(data)

	// Should return 4 bytes
	if len(bytes) != 4 {
		t.Errorf("CalculateChecksumBytes() length = %d, want 4", len(bytes))
	}

	// Verify the bytes can be converted back to checksum
	checksum := BytesToChecksum(bytes)
	expected := CalculateChecksum(data)
	if checksum != expected {
		t.Errorf("BytesToChecksum() = 0x%08X, want 0x%08X", checksum, expected)
	}
}

func TestBytesToChecksum(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint32
	}{
		{"zero bytes", []byte{}, 0},
		{"one byte", []byte{0x12}, 0},
		{"two bytes", []byte{0x12, 0x34}, 0},
		{"three bytes", []byte{0x12, 0x34, 0x56}, 0},
		{"four bytes", []byte{0x12, 0x34, 0x56, 0x78}, 0x12345678},
		{"five bytes", []byte{0x12, 0x34, 0x56, 0x78, 0x9A}, 0x12345678},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BytesToChecksum(tt.data)
			if result != tt.expected {
				t.Errorf("BytesToChecksum() = 0x%08X, want 0x%08X", result, tt.expected)
			}
		})
	}
}

func TestChecksumRoundtrip(t *testing.T) {
	testData := [][]byte{
		{},
		{0x00},
		{0xFF},
		[]byte("Hello, World!"),
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09},
		make([]byte, 1024), // 1KB of zeros
	}

	for i, data := range testData {
		checksum := CalculateChecksum(data)
		bytes := CalculateChecksumBytes(data)
		recovered := BytesToChecksum(bytes)

		if checksum != recovered {
			t.Errorf("Test %d: checksum roundtrip failed: 0x%08X != 0x%08X", i, checksum, recovered)
		}
	}
}
