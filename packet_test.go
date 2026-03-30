package voidbus

import (
	"testing"
	"time"

	"VoidBus/fragment"
	"VoidBus/internal"
)

func TestNewPacket(t *testing.T) {
	sessionID := "test-session-123"
	serializerType := "plain"
	payload := []byte("Hello, World!")

	packet := NewPacket(sessionID, serializerType, payload)

	if packet.Header.SessionID != sessionID {
		t.Errorf("SessionID = %s, want %s", packet.Header.SessionID, sessionID)
	}

	if packet.Header.SerializerType != serializerType {
		t.Errorf("SerializerType = %s, want %s", packet.Header.SerializerType, serializerType)
	}

	if string(packet.Payload) != string(payload) {
		t.Errorf("Payload = %v, want %v", packet.Payload, payload)
	}

	if packet.Header.Version != PacketVersion {
		t.Errorf("Version = %d, want %d", packet.Header.Version, PacketVersion)
	}

	expectedChecksum := internal.CalculateChecksum(payload)
	if packet.Header.PayloadChecksum != expectedChecksum {
		t.Errorf("PayloadChecksum = %d, want %d", packet.Header.PayloadChecksum, expectedChecksum)
	}
}

func TestPacket_WithFragment(t *testing.T) {
	packet := NewPacket("session", "plain", []byte("test"))

	fragInfo := fragment.FragmentInfo{
		ID:       "frag-123",
		Index:    1,
		Total:    3,
		IsLast:   false,
		Checksum: 12345,
	}

	packet.WithFragment(fragInfo)

	if packet.Header.FragmentInfo.ID != "frag-123" {
		t.Errorf("FragmentInfo.ID = %s, want frag-123", packet.Header.FragmentInfo.ID)
	}

	if packet.Header.FragmentInfo.Index != 1 {
		t.Errorf("FragmentInfo.Index = %d, want 1", packet.Header.FragmentInfo.Index)
	}
}

func TestPacket_Verify(t *testing.T) {
	packet := NewPacket("session", "plain", []byte("test"))

	// Fresh packet should verify
	if err := packet.Verify(); err != nil {
		t.Errorf("Fresh packet should verify, got error: %v", err)
	}

	// Modified version should fail
	packet.Header.Version = 99
	if err := packet.Verify(); err != ErrInvalidPacket {
		t.Errorf("Invalid version should return ErrInvalidPacket, got %v", err)
	}

	// Reset version
	packet.Header.Version = PacketVersion

	// Modified checksum should fail
	packet.Header.PayloadChecksum = 0
	if err := packet.Verify(); err != ErrChecksumMismatch {
		t.Errorf("Invalid checksum should return ErrChecksumMismatch, got %v", err)
	}
}

func TestPacket_Verify_TimestampExpired(t *testing.T) {
	packet := NewPacket("session", "plain", []byte("test"))

	// Old timestamp should fail
	packet.Header.Timestamp = time.Now().Add(-10 * time.Minute).Unix()
	if err := packet.Verify(); err != ErrTimestampExpired {
		t.Errorf("Expired timestamp should return ErrTimestampExpired, got %v", err)
	}
}

func TestPacket_EncodeDecode(t *testing.T) {
	sessionID := "test-session-123"
	serializerType := "json"
	payload := []byte("Hello, World!")

	packet := NewPacket(sessionID, serializerType, payload)

	encoded, err := packet.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("DecodePacket() error = %v", err)
	}

	// Verify decoded packet
	if decoded.Header.SessionID != sessionID {
		t.Errorf("Decoded SessionID = %s, want %s", decoded.Header.SessionID, sessionID)
	}

	if decoded.Header.SerializerType != serializerType {
		t.Errorf("Decoded SerializerType = %s, want %s", decoded.Header.SerializerType, serializerType)
	}

	if string(decoded.Payload) != string(payload) {
		t.Errorf("Decoded Payload = %v, want %v", decoded.Payload, payload)
	}

	if decoded.Header.Version != PacketVersion {
		t.Errorf("Decoded Version = %d, want %d", decoded.Header.Version, PacketVersion)
	}

	// Verify checksum
	expectedChecksum := internal.CalculateChecksum(payload)
	if decoded.Header.PayloadChecksum != expectedChecksum {
		t.Errorf("Decoded PayloadChecksum = %d, want %d", decoded.Header.PayloadChecksum, expectedChecksum)
	}
}

func TestPacket_EncodeDecode_WithFragment(t *testing.T) {
	packet := NewPacket("session", "plain", []byte("test data"))

	fragInfo := fragment.FragmentInfo{
		ID:       "fragment-group-123",
		Index:    2,
		Total:    5,
		IsLast:   false,
		Checksum: 98765,
	}
	packet.WithFragment(fragInfo)

	encoded, err := packet.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("DecodePacket() error = %v", err)
	}

	if decoded.Header.FragmentInfo.ID != fragInfo.ID {
		t.Errorf("Decoded FragmentInfo.ID = %s, want %s", decoded.Header.FragmentInfo.ID, fragInfo.ID)
	}

	if decoded.Header.FragmentInfo.Index != fragInfo.Index {
		t.Errorf("Decoded FragmentInfo.Index = %d, want %d", decoded.Header.FragmentInfo.Index, fragInfo.Index)
	}

	if decoded.Header.FragmentInfo.Total != fragInfo.Total {
		t.Errorf("Decoded FragmentInfo.Total = %d, want %d", decoded.Header.FragmentInfo.Total, fragInfo.Total)
	}

	if decoded.Header.FragmentInfo.IsLast != fragInfo.IsLast {
		t.Errorf("Decoded FragmentInfo.IsLast = %v, want %v", decoded.Header.FragmentInfo.IsLast, fragInfo.IsLast)
	}

	if decoded.Header.FragmentInfo.Checksum != fragInfo.Checksum {
		t.Errorf("Decoded FragmentInfo.Checksum = %d, want %d", decoded.Header.FragmentInfo.Checksum, fragInfo.Checksum)
	}
}

func TestDecodePacket_InvalidData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty data", []byte{}},
		{"too short", []byte{0x00, 0x01}},
		{"invalid header length", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x00}}, // headerLen > data length
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodePacket(tt.data)
			if err != ErrInvalidPacket {
				t.Errorf("DecodePacket() should return ErrInvalidPacket, got %v", err)
			}
		})
	}
}

func TestPacket_IsFragment(t *testing.T) {
	// Non-fragment packet
	packet1 := NewPacket("session", "plain", []byte("test"))
	if packet1.IsFragment() {
		t.Error("Packet without fragment info should not be fragment")
	}

	// Fragment packet
	packet2 := NewPacket("session", "plain", []byte("test"))
	packet2.WithFragment(fragment.FragmentInfo{
		ID:    "frag-123",
		Total: 3,
	})
	if !packet2.IsFragment() {
		t.Error("Packet with fragment info should be fragment")
	}

	// Fragment with empty ID
	packet3 := NewPacket("session", "plain", []byte("test"))
	packet3.WithFragment(fragment.FragmentInfo{
		ID:    "",
		Total: 3,
	})
	if packet3.IsFragment() {
		t.Error("Packet with empty fragment ID should not be fragment")
	}
}

func TestPacket_IsLastFragment(t *testing.T) {
	packet := NewPacket("session", "plain", []byte("test"))

	// Not last fragment
	packet.WithFragment(fragment.FragmentInfo{
		ID:     "frag-123",
		IsLast: false,
	})
	if packet.IsLastFragment() {
		t.Error("IsLast=false should not be last fragment")
	}

	// Last fragment
	packet.Header.FragmentInfo.IsLast = true
	if !packet.IsLastFragment() {
		t.Error("IsLast=true should be last fragment")
	}
}

func TestPacket_EncodeDecode_LargePayload(t *testing.T) {
	// Large payload (1MB)
	payload := make([]byte, 1024*1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	packet := NewPacket("large-session", "binary", payload)

	encoded, err := packet.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("DecodePacket() error = %v", err)
	}

	// Verify payload
	if len(decoded.Payload) != len(payload) {
		t.Errorf("Decoded payload length = %d, want %d", len(decoded.Payload), len(payload))
	}

	// Verify checksum
	expectedChecksum := internal.CalculateChecksum(payload)
	if decoded.Header.PayloadChecksum != expectedChecksum {
		t.Errorf("Decoded PayloadChecksum mismatch")
	}
}

func TestPacket_EncodeDecode_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		sessionID      string
		serializerType string
		payload        []byte
	}{
		{"empty payload", "session", "plain", []byte{}},
		{"nil payload fields", "s", "p", []byte("x")},
		{"long session ID", "very-long-session-id-with-many-characters-1234567890", "plain", []byte("test")},
		{"long serializer type", "session", "very-long-serializer-type-name", []byte("test")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet := NewPacket(tt.sessionID, tt.serializerType, tt.payload)

			encoded, err := packet.Encode()
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			decoded, err := DecodePacket(encoded)
			if err != nil {
				t.Fatalf("DecodePacket() error = %v", err)
			}

			if decoded.Header.SessionID != tt.sessionID {
				t.Errorf("SessionID mismatch")
			}

			if decoded.Header.SerializerType != tt.serializerType {
				t.Errorf("SerializerType mismatch")
			}
		})
	}
}
