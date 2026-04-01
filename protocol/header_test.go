// Package protocol_test provides tests for v2.0 protocol header validation.
package protocol_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Script-OS/VoidBus/protocol"
)

// === V2ValidationError Tests ===

func TestV2ValidationError_Error(t *testing.T) {
	err := protocol.NewValidationError(
		"SessionID",
		"sessionID too long",
		128,
		"<= 64",
	)

	expected := "validation error: SessionID - sessionID too long (actual: 128, expected: <= 64)"
	if err.Error() != expected {
		t.Errorf("expected error message '%s', got '%s'", expected, err.Error())
	}
}

// === Encode Tests ===

func TestHeader_Encode(t *testing.T) {
	header := &protocol.Header{
		SessionID:     "test-session-123",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    2,
		CodecHash:     [32]byte{1, 2, 3},
		DataChecksum:  12345,
		DataHash:      [32]byte{4, 5, 6},
		Timestamp:     time.Now().Unix(),
		Flags:         protocol.FlagIsLast,
	}

	data := []byte("test data")
	packet := header.Encode(data)

	// Verify packet size
	expectedLen := 2 + len(header.SessionID) + protocol.HeaderBaseSize + len(data)
	if len(packet) != expectedLen {
		t.Errorf("expected packet length %d, got %d", expectedLen, len(packet))
	}

	// Decode and verify
	decodedHeader, decodedData, err := protocol.DecodeHeader(packet)
	if err != nil {
		t.Fatalf("DecodeHeader failed: %v", err)
	}

	if decodedHeader.SessionID != header.SessionID {
		t.Errorf("expected SessionID '%s', got '%s'", header.SessionID, decodedHeader.SessionID)
	}

	if decodedHeader.FragmentIndex != header.FragmentIndex {
		t.Errorf("expected FragmentIndex %d, got %d", header.FragmentIndex, decodedHeader.FragmentIndex)
	}

	if decodedHeader.FragmentTotal != header.FragmentTotal {
		t.Errorf("expected FragmentTotal %d, got %d", header.FragmentTotal, decodedHeader.FragmentTotal)
	}

	if decodedHeader.CodecDepth != header.CodecDepth {
		t.Errorf("expected CodecDepth %d, got %d", header.CodecDepth, decodedHeader.CodecDepth)
	}

	if decodedHeader.Flags != header.Flags {
		t.Errorf("expected Flags %d, got %d", header.Flags, decodedHeader.Flags)
	}

	if string(decodedData) != string(data) {
		t.Errorf("expected data '%s', got '%s'", string(data), string(decodedData))
	}
}

// === DecodeHeader Security Validation Tests ===

func TestDecodeHeader_ValidPacket(t *testing.T) {
	header := &protocol.Header{
		SessionID:     "valid-session-id",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    2,
		CodecHash:     [32]byte{1, 2, 3},
		DataChecksum:  12345,
		DataHash:      [32]byte{4, 5, 6},
		Timestamp:     time.Now().Unix(),
		Flags:         0,
	}

	data := []byte("valid test data")
	packet := header.Encode(data)

	decodedHeader, decodedData, err := protocol.DecodeHeader(packet)
	if err != nil {
		t.Fatalf("expected valid packet to decode, got error: %v", err)
	}

	if decodedHeader == nil {
		t.Fatal("expected decoded header, got nil")
	}

	if len(decodedData) != len(data) {
		t.Errorf("expected data length %d, got %d", len(data), len(decodedData))
	}
}

func TestDecodeHeader_PacketTooShort(t *testing.T) {
	// Packet less than MinPacketSize
	shortPacket := []byte{0x00, 0x01} // Only 2 bytes

	_, _, err := protocol.DecodeHeader(shortPacket)
	if err == nil {
		t.Fatal("expected error for packet too short, got nil")
	}

	// Verify it's a validation error
	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "PacketSize" {
		t.Errorf("expected Field 'PacketSize', got '%s'", validationErr.Field)
	}

	if validationErr.Msg != "packet too short" {
		t.Errorf("expected Msg 'packet too short', got '%s'", validationErr.Msg)
	}
}

func TestDecodeHeader_PacketTooLarge(t *testing.T) {
	// Create packet larger than MaxPacketSize
	largePacket := make([]byte, protocol.MaxPacketSize+1)

	_, _, err := protocol.DecodeHeader(largePacket)
	if err == nil {
		t.Fatal("expected error for packet too large, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "PacketSize" {
		t.Errorf("expected Field 'PacketSize', got '%s'", validationErr.Field)
	}
}

func TestDecodeHeader_SessionIDTooShort(t *testing.T) {
	// Create packet with SessionID length = 0
	packet := make([]byte, protocol.MinPacketSize)
	packet[0] = 0x00
	packet[1] = 0x00 // SessionID length = 0

	_, _, err := protocol.DecodeHeader(packet)
	if err == nil {
		t.Fatal("expected error for SessionID too short, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "SessionID" {
		t.Errorf("expected Field 'SessionID', got '%s'", validationErr.Field)
	}
}

func TestDecodeHeader_SessionIDTooLong(t *testing.T) {
	// Create packet with SessionID length > MaxSessionIDLength
	sessionIDLen := protocol.MaxSessionIDLength + 10
	packet := make([]byte, 2+sessionIDLen+protocol.HeaderBaseSize)
	packet[0] = byte(sessionIDLen >> 8)
	packet[1] = byte(sessionIDLen)

	_, _, err := protocol.DecodeHeader(packet)
	if err == nil {
		t.Fatal("expected error for SessionID too long, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "SessionID" {
		t.Errorf("expected Field 'SessionID', got '%s'", validationErr.Field)
	}

	if validationErr.Msg != "sessionID too long" {
		t.Errorf("expected Msg 'sessionID too long', got '%s'", validationErr.Msg)
	}
}

func TestDecodeHeader_FragmentTotalExceedsLimit(t *testing.T) {
	header := &protocol.Header{
		SessionID:     "test-session",
		FragmentIndex: 0,
		FragmentTotal: protocol.MaxFragmentTotal + 1000, // Exceeds limit
		CodecDepth:    2,
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix(),
		Flags:         0,
	}

	packet := header.Encode([]byte("test"))

	_, _, err := protocol.DecodeHeader(packet)
	if err == nil {
		t.Fatal("expected error for FragmentTotal exceeds limit, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "FragmentTotal" {
		t.Errorf("expected Field 'FragmentTotal', got '%s'", validationErr.Field)
	}
}

func TestDecodeHeader_FragmentIndexOutOfRange(t *testing.T) {
	header := &protocol.Header{
		SessionID:     "test-session",
		FragmentIndex: 10, // Index >= Total
		FragmentTotal: 10,
		CodecDepth:    2,
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix(),
		Flags:         0,
	}

	packet := header.Encode([]byte("test"))

	_, _, err := protocol.DecodeHeader(packet)
	if err == nil {
		t.Fatal("expected error for FragmentIndex out of range, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "FragmentIndex" {
		t.Errorf("expected Field 'FragmentIndex', got '%s'", validationErr.Field)
	}
}

func TestDecodeHeader_CodecDepthTooSmall(t *testing.T) {
	header := &protocol.Header{
		SessionID:     "test-session",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    0, // Less than MinCodecDepth
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix(),
		Flags:         0,
	}

	packet := header.Encode([]byte("test"))

	_, _, err := protocol.DecodeHeader(packet)
	if err == nil {
		t.Fatal("expected error for CodecDepth too small, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "CodecDepth" {
		t.Errorf("expected Field 'CodecDepth', got '%s'", validationErr.Field)
	}
}

func TestDecodeHeader_CodecDepthTooLarge(t *testing.T) {
	header := &protocol.Header{
		SessionID:     "test-session",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    protocol.MaxCodecDepth + 10, // Exceeds limit
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix(),
		Flags:         0,
	}

	packet := header.Encode([]byte("test"))

	_, _, err := protocol.DecodeHeader(packet)
	if err == nil {
		t.Fatal("expected error for CodecDepth too large, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "CodecDepth" {
		t.Errorf("expected Field 'CodecDepth', got '%s'", validationErr.Field)
	}
}

func TestDecodeHeader_TimestampTooOld(t *testing.T) {
	header := &protocol.Header{
		SessionID:     "test-session",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    2,
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix() - protocol.MaxTimestampAge - 1000, // Too old
		Flags:         0,
	}

	packet := header.Encode([]byte("test"))

	_, _, err := protocol.DecodeHeader(packet)
	if err == nil {
		t.Fatal("expected error for timestamp too old, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "Timestamp" {
		t.Errorf("expected Field 'Timestamp', got '%s'", validationErr.Field)
	}

	if validationErr.Msg != "packet timestamp too old (potential replay attack)" {
		t.Errorf("expected replay attack message, got '%s'", validationErr.Msg)
	}
}

func TestDecodeHeader_TimestampInFuture(t *testing.T) {
	header := &protocol.Header{
		SessionID:     "test-session",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    2,
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix() + 1000, // Too far in future (beyond -300 tolerance)
		Flags:         0,
	}

	packet := header.Encode([]byte("test"))

	_, _, err := protocol.DecodeHeader(packet)
	if err == nil {
		t.Fatal("expected error for timestamp in future, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "Timestamp" {
		t.Errorf("expected Field 'Timestamp', got '%s'", validationErr.Field)
	}
}

func TestDecodeHeader_InvalidFlags(t *testing.T) {
	header := &protocol.Header{
		SessionID:     "test-session",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    2,
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix(),
		Flags:         0xFF, // Invalid flags (bits outside valid range)
	}

	packet := header.Encode([]byte("test"))

	_, _, err := protocol.DecodeHeader(packet)
	if err == nil {
		t.Fatal("expected error for invalid flags, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "Flags" {
		t.Errorf("expected Field 'Flags', got '%s'", validationErr.Field)
	}
}

func TestDecodeHeader_ValidFlags(t *testing.T) {
	// Test all valid flag combinations
	validFlags := []uint8{
		0,
		protocol.FlagIsLast,
		protocol.FlagRetransmit,
		protocol.FlagIsNAK,
		protocol.FlagIsENDACK,
		protocol.FlagIsLast | protocol.FlagRetransmit,
	}

	for _, flags := range validFlags {
		header := &protocol.Header{
			SessionID:     "test-session",
			FragmentIndex: 0,
			FragmentTotal: 10,
			CodecDepth:    2,
			CodecHash:     [32]byte{},
			DataChecksum:  0,
			DataHash:      [32]byte{},
			Timestamp:     time.Now().Unix(),
			Flags:         flags,
		}

		packet := header.Encode([]byte("test"))

		_, _, err := protocol.DecodeHeader(packet)
		if err != nil {
			t.Errorf("expected valid flags 0x%02x to pass validation, got error: %v", flags, err)
		}
	}
}

func TestDecodeHeader_NAKPacket(t *testing.T) {
	// NAK packets may have FragmentTotal=0
	header := &protocol.Header{
		SessionID:     "test-session",
		FragmentIndex: 0,
		FragmentTotal: 0, // NAK packet
		CodecDepth:    2,
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix(),
		Flags:         protocol.FlagIsNAK,
	}

	packet := header.Encode([]byte{})

	decodedHeader, _, err := protocol.DecodeHeader(packet)
	if err != nil {
		t.Fatalf("expected NAK packet to decode, got error: %v", err)
	}

	if !decodedHeader.IsNAK() {
		t.Error("expected IsNAK() to return true")
	}
}

func TestDecodeHeader_ENDACKPacket(t *testing.T) {
	// END_ACK packets may have FragmentTotal=0
	header := &protocol.Header{
		SessionID:     "test-session",
		FragmentIndex: 0,
		FragmentTotal: 0,
		CodecDepth:    2,
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix(),
		Flags:         protocol.FlagIsENDACK,
	}

	packet := header.Encode([]byte{})

	decodedHeader, _, err := protocol.DecodeHeader(packet)
	if err != nil {
		t.Fatalf("expected END_ACK packet to decode, got error: %v", err)
	}

	if !decodedHeader.IsEND_ACK() {
		t.Error("expected IsEND_ACK() to return true")
	}
}

func TestDecodeHeader_TruncatedPacket(t *testing.T) {
	// Packet with SessionID length indicating more data than available
	sessionIDLen := 20
	packet := make([]byte, 2+10) // Only 12 bytes, but SessionID expects 20
	packet[0] = byte(sessionIDLen >> 8)
	packet[1] = byte(sessionIDLen)

	_, _, err := protocol.DecodeHeader(packet)
	if err == nil {
		t.Fatal("expected error for truncated packet, got nil")
	}

	var validationErr *protocol.V2ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("expected V2ValidationError, got %T", err)
	}

	if validationErr.Field != "PacketSize" {
		t.Errorf("expected Field 'PacketSize', got '%s'", validationErr.Field)
	}
}

// === Flag Helper Methods Tests ===

func TestHeader_FlagMethods(t *testing.T) {
	header := &protocol.Header{Flags: 0}

	// Test SetIsLast
	header.SetIsLast(true)
	if !header.IsLastFragment() {
		t.Error("expected IsLastFragment() to return true after SetIsLast(true)")
	}

	header.SetIsLast(false)
	if header.IsLastFragment() {
		t.Error("expected IsLastFragment() to return false after SetIsLast(false)")
	}

	// Test SetRetransmit
	header.SetRetransmit(true)
	if !header.IsRetransmit() {
		t.Error("expected IsRetransmit() to return true after SetRetransmit(true)")
	}

	header.SetRetransmit(false)
	if header.IsRetransmit() {
		t.Error("expected IsRetransmit() to return false after SetRetransmit(false)")
	}
}

// === Benchmark Tests ===

func BenchmarkEncodeHeader(b *testing.B) {
	header := &protocol.Header{
		SessionID:     "benchmark-session",
		FragmentIndex: 0,
		FragmentTotal: 100,
		CodecDepth:    2,
		CodecHash:     [32]byte{1, 2, 3},
		DataChecksum:  12345,
		DataHash:      [32]byte{4, 5, 6},
		Timestamp:     time.Now().Unix(),
		Flags:         0,
	}

	data := make([]byte, 512)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		header.Encode(data)
	}
}

func BenchmarkDecodeHeader_Valid(b *testing.B) {
	header := &protocol.Header{
		SessionID:     "benchmark-session",
		FragmentIndex: 0,
		FragmentTotal: 100,
		CodecDepth:    2,
		CodecHash:     [32]byte{1, 2, 3},
		DataChecksum:  12345,
		DataHash:      [32]byte{4, 5, 6},
		Timestamp:     time.Now().Unix(),
		Flags:         0,
	}

	data := make([]byte, 512)
	packet := header.Encode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protocol.DecodeHeader(packet)
	}
}

func BenchmarkDecodeHeader_Invalid(b *testing.B) {
	shortPacket := []byte{0x00, 0x01}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protocol.DecodeHeader(shortPacket)
	}
}
