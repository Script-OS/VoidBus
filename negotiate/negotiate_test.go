// Package negotiate provides tests for negotiation protocol.
package negotiate

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"
)

// ============================================================================
// Codec Bitmap Tests
// ============================================================================

func TestNewCodecBitmap(t *testing.T) {
	// Test with default size
	bitmap := NewCodecBitmap(0)
	if len(bitmap) != DefaultCodecBitmapSize {
		t.Errorf("expected default size %d, got %d", DefaultCodecBitmapSize, len(bitmap))
	}

	// Test with custom size
	bitmap = NewCodecBitmap(4)
	if len(bitmap) != 4 {
		t.Errorf("expected size 4, got %d", len(bitmap))
	}

	// Test with zero size (should use default)
	bitmap = NewCodecBitmap(0)
	if len(bitmap) != DefaultCodecBitmapSize {
		t.Errorf("expected default size for 0, got %d", len(bitmap))
	}
}

func TestCodecBitmap_SetCodec(t *testing.T) {
	bitmap := NewCodecBitmap(2)

	// Set Plain codec (bit 0)
	bitmap.SetCodec(CodecBitPlain)
	if !bitmap.HasCodec(CodecBitPlain) {
		t.Error("expected Plain codec to be set")
	}

	// Set AES codec (bit 2)
	bitmap.SetCodec(CodecBitAES256)
	if !bitmap.HasCodec(CodecBitAES256) {
		t.Error("expected AES codec to be set")
	}

	// Verify expected value: 0b00000101 = 0x05
	if bitmap[0] != 0x05 {
		t.Errorf("expected bitmap[0] = 0x05, got 0x%02x", bitmap[0])
	}
}

func TestCodecBitmap_ClearCodec(t *testing.T) {
	bitmap := NewCodecBitmap(2)
	bitmap.SetCodec(CodecBitPlain)
	bitmap.SetCodec(CodecBitAES256)

	// Clear Plain codec
	bitmap.ClearCodec(CodecBitPlain)
	if bitmap.HasCodec(CodecBitPlain) {
		t.Error("expected Plain codec to be cleared")
	}

	// AES should still be set
	if !bitmap.HasCodec(CodecBitAES256) {
		t.Error("expected AES codec to remain set")
	}
}

func TestCodecBitmap_HasCodec(t *testing.T) {
	bitmap := NewCodecBitmap(2)

	// Test unset codec
	if bitmap.HasCodec(CodecBitPlain) {
		t.Error("expected Plain codec to not be set initially")
	}

	// Set and test
	bitmap.SetCodec(CodecBitPlain)
	if !bitmap.HasCodec(CodecBitPlain) {
		t.Error("expected Plain codec to be set after SetCodec")
	}

	// Test beyond bitmap size
	if bitmap.HasCodec(CodecBit(16)) {
		t.Error("expected codec bit 16 to return false (beyond size)")
	}
}

func TestCodecBitmap_GetCodecIDs(t *testing.T) {
	bitmap := NewCodecBitmap(2)
	bitmap.SetCodec(CodecBitPlain)
	bitmap.SetCodec(CodecBitAES256)
	bitmap.SetCodec(CodecBitChaCha20)

	ids := bitmap.GetCodecIDs()
	expected := []CodecID{CodecIDPlain, CodecIDAES256, CodecIDChaCha20}

	if len(ids) != len(expected) {
		t.Errorf("expected %d IDs, got %d", len(expected), len(ids))
	}

	for _, id := range expected {
		found := false
		for _, gotID := range ids {
			if gotID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected ID %d not found in result", id)
		}
	}
}

func TestCodecBitmapFromIDs(t *testing.T) {
	ids := []CodecID{CodecIDPlain, CodecIDAES256, CodecIDChaCha20}
	bitmap := CodecBitmapFromIDs(ids, 2)

	if !bitmap.HasCodec(CodecBitPlain) {
		t.Error("expected Plain to be set")
	}
	if !bitmap.HasCodec(CodecBitAES256) {
		t.Error("expected AES to be set")
	}
	if !bitmap.HasCodec(CodecBitChaCha20) {
		t.Error("expected ChaCha20 to be set")
	}
}

func TestIntersectCodecBitmaps(t *testing.T) {
	// Create two bitmaps
	a := NewCodecBitmap(2)
	a.SetCodec(CodecBitPlain)
	a.SetCodec(CodecBitAES256)
	a.SetCodec(CodecBitChaCha20)

	b := NewCodecBitmap(2)
	b.SetCodec(CodecBitPlain)
	b.SetCodec(CodecBitXOR)
	b.SetCodec(CodecBitChaCha20)

	// Compute intersection
	result := IntersectCodecBitmaps(a, b)

	// Expected: Plain + ChaCha20
	if !result.HasCodec(CodecBitPlain) {
		t.Error("expected Plain in intersection")
	}
	if result.HasCodec(CodecBitAES256) {
		t.Error("expected AES NOT in intersection")
	}
	if result.HasCodec(CodecBitXOR) {
		t.Error("expected XOR NOT in intersection")
	}
	if !result.HasCodec(CodecBitChaCha20) {
		t.Error("expected ChaCha20 in intersection")
	}
}

func TestIsCodecBitmapEmpty(t *testing.T) {
	// Empty bitmap
	empty := NewCodecBitmap(2)
	if !IsCodecBitmapEmpty(empty) {
		t.Error("expected empty bitmap to be empty")
	}

	// Non-empty bitmap
	nonEmpty := NewCodecBitmap(2)
	nonEmpty.SetCodec(CodecBitPlain)
	if IsCodecBitmapEmpty(nonEmpty) {
		t.Error("expected non-empty bitmap to not be empty")
	}
}

func TestCodecCount(t *testing.T) {
	bitmap := NewCodecBitmap(2)
	bitmap.SetCodec(CodecBitPlain)
	bitmap.SetCodec(CodecBitAES256)
	bitmap.SetCodec(CodecBitChaCha20)

	count := CodecCount(bitmap)
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestCodecBitmap_Clone(t *testing.T) {
	bitmap := NewCodecBitmap(2)
	bitmap.SetCodec(CodecBitPlain)
	bitmap.SetCodec(CodecBitAES256)

	cloned := bitmap.Clone()

	// Verify clone has same values
	if !bytes.Equal(bitmap, cloned) {
		t.Error("expected clone to be equal to original")
	}

	// Modify clone should not affect original
	cloned.SetCodec(CodecBitChaCha20)
	if bitmap.HasCodec(CodecBitChaCha20) {
		t.Error("expected original to not be affected by clone modification")
	}
}

// ============================================================================
// Channel Bitmap Tests
// ============================================================================

func TestNewChannelBitmap(t *testing.T) {
	bitmap := NewChannelBitmap(0)
	if len(bitmap) != DefaultChannelBitmapSize {
		t.Errorf("expected default size %d, got %d", DefaultChannelBitmapSize, len(bitmap))
	}

	bitmap = NewChannelBitmap(4)
	if len(bitmap) != 4 {
		t.Errorf("expected size 4, got %d", len(bitmap))
	}
}

func TestChannelBitmap_SetChannel(t *testing.T) {
	bitmap := NewChannelBitmap(2)

	bitmap.SetChannel(ChannelBitWS)
	if !bitmap.HasChannel(ChannelBitWS) {
		t.Error("expected WS channel to be set")
	}

	bitmap.SetChannel(ChannelBitTCP)
	if !bitmap.HasChannel(ChannelBitTCP) {
		t.Error("expected TCP channel to be set")
	}

	// Expected: 0b00000011 = 0x03
	if bitmap[0] != 0x03 {
		t.Errorf("expected bitmap[0] = 0x03, got 0x%02x", bitmap[0])
	}
}

func TestChannelBitmap_ClearChannel(t *testing.T) {
	bitmap := NewChannelBitmap(2)
	bitmap.SetChannel(ChannelBitWS)
	bitmap.SetChannel(ChannelBitTCP)

	bitmap.ClearChannel(ChannelBitWS)
	if bitmap.HasChannel(ChannelBitWS) {
		t.Error("expected WS channel to be cleared")
	}
	if !bitmap.HasChannel(ChannelBitTCP) {
		t.Error("expected TCP channel to remain set")
	}
}

func TestChannelBitmap_HasChannel(t *testing.T) {
	bitmap := NewChannelBitmap(2)

	if bitmap.HasChannel(ChannelBitWS) {
		t.Error("expected WS to not be set initially")
	}

	bitmap.SetChannel(ChannelBitWS)
	if !bitmap.HasChannel(ChannelBitWS) {
		t.Error("expected WS to be set after SetChannel")
	}
}

func TestChannelBitmap_GetChannelIDs(t *testing.T) {
	bitmap := NewChannelBitmap(2)
	bitmap.SetChannel(ChannelBitWS)
	bitmap.SetChannel(ChannelBitTCP)
	bitmap.SetChannel(ChannelBitUDP)

	ids := bitmap.GetChannelIDs()
	expected := []ChannelID{ChannelIDWS, ChannelIDTCP, ChannelIDUDP}

	if len(ids) != len(expected) {
		t.Errorf("expected %d IDs, got %d", len(expected), len(ids))
	}

	for _, id := range expected {
		found := false
		for _, gotID := range ids {
			if gotID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected ChannelID %d not found", id)
		}
	}
}

func TestChannelBitmapFromIDs(t *testing.T) {
	ids := []ChannelID{ChannelIDWS, ChannelIDTCP, ChannelIDUDP}
	bitmap := ChannelBitmapFromIDs(ids, 2)

	if !bitmap.HasChannel(ChannelBitWS) {
		t.Error("expected WS to be set")
	}
	if !bitmap.HasChannel(ChannelBitTCP) {
		t.Error("expected TCP to be set")
	}
	if !bitmap.HasChannel(ChannelBitUDP) {
		t.Error("expected UDP to be set")
	}
}

func TestIntersectChannelBitmaps(t *testing.T) {
	a := NewChannelBitmap(2)
	a.SetChannel(ChannelBitWS)
	a.SetChannel(ChannelBitTCP)
	a.SetChannel(ChannelBitUDP)

	b := NewChannelBitmap(2)
	b.SetChannel(ChannelBitWS)
	b.SetChannel(ChannelBitICMP) // Use ICMP instead of QUIC (UDP already in a)
	b.SetChannel(ChannelBitUDP)

	result := IntersectChannelBitmaps(a, b)

	if !result.HasChannel(ChannelBitWS) {
		t.Error("expected WS in intersection")
	}
	if result.HasChannel(ChannelBitTCP) {
		t.Error("expected TCP NOT in intersection")
	}
	if !result.HasChannel(ChannelBitUDP) {
		t.Error("expected UDP in intersection")
	}
	if result.HasChannel(ChannelBitICMP) {
		t.Error("expected ICMP NOT in intersection")
	}
}

func TestIsChannelBitmapEmpty(t *testing.T) {
	empty := NewChannelBitmap(2)
	if !IsChannelBitmapEmpty(empty) {
		t.Error("expected empty bitmap to be empty")
	}

	nonEmpty := NewChannelBitmap(2)
	nonEmpty.SetChannel(ChannelBitWS)
	if IsChannelBitmapEmpty(nonEmpty) {
		t.Error("expected non-empty bitmap to not be empty")
	}
}

func TestChannelCount(t *testing.T) {
	bitmap := NewChannelBitmap(2)
	bitmap.SetChannel(ChannelBitWS)
	bitmap.SetChannel(ChannelBitTCP)
	bitmap.SetChannel(ChannelBitUDP)

	count := ChannelCount(bitmap)
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestChannelBitmap_IsReliable(t *testing.T) {
	bitmap := NewChannelBitmap(2)

	// Reliable channels
	if !bitmap.IsReliable(ChannelBitWS) {
		t.Error("expected WS to be reliable")
	}
	if !bitmap.IsReliable(ChannelBitTCP) {
		t.Error("expected TCP to be reliable")
	}
	// UDP is unreliable
	if bitmap.IsReliable(ChannelBitUDP) {
		t.Error("expected UDP to be unreliable")
	}

	// Unreliable channels
	if bitmap.IsReliable(ChannelBitUDP) {
		t.Error("expected UDP to be unreliable")
	}
	if bitmap.IsReliable(ChannelBitICMP) {
		t.Error("expected ICMP to be unreliable")
	}
	if bitmap.IsReliable(ChannelBitDNS) {
		t.Error("expected DNS to be unreliable")
	}
}

func TestChannelBitmap_GetReliableChannels(t *testing.T) {
	bitmap := NewChannelBitmap(2)
	bitmap.SetChannel(ChannelBitWS)
	bitmap.SetChannel(ChannelBitTCP)
	bitmap.SetChannel(ChannelBitUDP) // unreliable

	reliable := bitmap.GetReliableChannels()
	expected := []ChannelID{ChannelIDWS, ChannelIDTCP} // UDP is unreliable

	if len(reliable) != len(expected) {
		t.Errorf("expected %d reliable channels, got %d", len(expected), len(reliable))
	}

	for _, id := range expected {
		found := false
		for _, gotID := range reliable {
			if gotID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected reliable ChannelID %d not found", id)
		}
	}
}

func TestChannelBitmap_GetUnreliableChannels(t *testing.T) {
	bitmap := NewChannelBitmap(2)
	bitmap.SetChannel(ChannelBitWS)   // reliable
	bitmap.SetChannel(ChannelBitUDP)  // unreliable
	bitmap.SetChannel(ChannelBitICMP) // unreliable

	unreliable := bitmap.GetUnreliableChannels()
	expected := []ChannelID{ChannelIDUDP, ChannelIDICMP}

	if len(unreliable) != len(expected) {
		t.Errorf("expected %d unreliable channels, got %d", len(expected), len(unreliable))
	}

	for _, id := range expected {
		found := false
		for _, gotID := range unreliable {
			if gotID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected unreliable ChannelID %d not found", id)
		}
	}
}

func TestChannelBitmap_Clone(t *testing.T) {
	bitmap := NewChannelBitmap(2)
	bitmap.SetChannel(ChannelBitWS)
	bitmap.SetChannel(ChannelBitTCP)

	cloned := bitmap.Clone()

	if !bytes.Equal(bitmap, cloned) {
		t.Error("expected clone to be equal to original")
	}

	cloned.SetChannel(ChannelBitUDP)
	if bitmap.HasChannel(ChannelBitUDP) {
		t.Error("expected original to not be affected by clone modification")
	}
}

// ============================================================================
// Frame Tests
// ============================================================================

func TestNewNegotiateRequest(t *testing.T) {
	chBitmap := NewChannelBitmap(2)
	chBitmap.SetChannel(ChannelBitWS)
	chBitmap.SetChannel(ChannelBitTCP)

	codecBitmap := NewCodecBitmap(2)
	codecBitmap.SetCodec(CodecBitPlain)
	codecBitmap.SetCodec(CodecBitAES256)

	req, err := NewNegotiateRequest(chBitmap, codecBitmap, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if len(req.SessionNonce) != NegotiateNonceSize {
		t.Errorf("expected nonce size %d, got %d", NegotiateNonceSize, len(req.SessionNonce))
	}

	if len(req.Padding) > NegotiateMaxPaddingSize {
		t.Errorf("padding exceeds max size: %d", len(req.Padding))
	}

	// Timestamp should be recent
	now := uint32(time.Now().Unix())
	if req.Timestamp > now || req.Timestamp < now-5 {
		t.Errorf("timestamp not recent: %d (now: %d)", req.Timestamp, now)
	}
}

func TestNegotiateRequest_EncodeDecode(t *testing.T) {
	chBitmap := NewChannelBitmap(2)
	chBitmap.SetChannel(ChannelBitWS)
	chBitmap.SetChannel(ChannelBitTCP)
	chBitmap.SetChannel(ChannelBitUDP)

	codecBitmap := NewCodecBitmap(2)
	codecBitmap.SetCodec(CodecBitPlain)
	codecBitmap.SetCodec(CodecBitAES256)
	codecBitmap.SetCodec(CodecBitChaCha20)

	req, err := NewNegotiateRequest(chBitmap, codecBitmap, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Encode
	encoded, err := req.Encode()
	if err != nil {
		t.Fatalf("failed to encode request: %v", err)
	}

	if len(encoded) < NegotiateMinFrameSize {
		t.Errorf("encoded frame too small: %d", len(encoded))
	}
	if len(encoded) > NegotiateMaxFrameSize {
		t.Errorf("encoded frame too large: %d", len(encoded))
	}

	// Verify magic and version
	if encoded[0] != NegotiateMagicRequest {
		t.Errorf("expected magic 0x%02x, got 0x%02x", NegotiateMagicRequest, encoded[0])
	}
	if encoded[1] != NegotiateVersion {
		t.Errorf("expected version 0x%02x, got 0x%02x", NegotiateVersion, encoded[1])
	}

	// Decode
	decoded, err := DecodeNegotiateRequest(encoded)
	if err != nil {
		t.Fatalf("failed to decode request: %v", err)
	}

	// Verify decoded values
	if !bytes.Equal(decoded.ChannelBitmap, chBitmap) {
		t.Error("channel bitmap mismatch")
	}
	if !bytes.Equal(decoded.CodecBitmap, codecBitmap) {
		t.Error("codec bitmap mismatch")
	}
	if !bytes.Equal(decoded.SessionNonce, req.SessionNonce) {
		t.Error("nonce mismatch")
	}
	if decoded.Timestamp != req.Timestamp {
		t.Errorf("timestamp mismatch: %d vs %d", decoded.Timestamp, req.Timestamp)
	}
}

func TestNegotiateRequest_Encode_InvalidNonceSize(t *testing.T) {
	req := &NegotiateRequest{
		ChannelBitmap: NewChannelBitmap(2),
		CodecBitmap:   NewCodecBitmap(2),
		SessionNonce:  []byte{1, 2, 3}, // Invalid size (should be 8)
		Timestamp:     uint32(time.Now().Unix()),
		Padding:       []byte{},
	}

	_, err := req.Encode()
	if err != ErrNonceSize {
		t.Errorf("expected ErrNonceSize, got %v", err)
	}
}

func TestNegotiateRequest_Encode_BitmapTooLarge(t *testing.T) {
	req := &NegotiateRequest{
		ChannelBitmap: make([]byte, 300), // Too large (>255)
		CodecBitmap:   NewCodecBitmap(2),
		SessionNonce:  make([]byte, 8),
		Timestamp:     uint32(time.Now().Unix()),
		Padding:       []byte{},
	}

	_, err := req.Encode()
	if err != ErrInvalidFrameSize {
		t.Errorf("expected ErrInvalidFrameSize, got %v", err)
	}
}

func TestDecodeNegotiateRequest_InvalidMagic(t *testing.T) {
	// Create valid request
	req, _ := NewNegotiateRequest(NewChannelBitmap(2), NewCodecBitmap(2), nil)
	encoded, _ := req.Encode()

	// Corrupt magic
	encoded[0] = 0xFF

	_, err := DecodeNegotiateRequest(encoded)
	if err != ErrInvalidMagic {
		t.Errorf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestDecodeNegotiateRequest_InvalidVersion(t *testing.T) {
	req, _ := NewNegotiateRequest(NewChannelBitmap(2), NewCodecBitmap(2), nil)
	encoded, _ := req.Encode()

	// Corrupt version
	encoded[1] = 0xFF

	_, err := DecodeNegotiateRequest(encoded)
	if err != ErrInvalidVersion {
		t.Errorf("expected ErrInvalidVersion, got %v", err)
	}
}

func TestDecodeNegotiateRequest_InvalidChecksum(t *testing.T) {
	req, _ := NewNegotiateRequest(NewChannelBitmap(2), NewCodecBitmap(2), nil)
	encoded, _ := req.Encode()

	// Corrupt checksum
	encoded[len(encoded)-1] = 0xFF

	_, err := DecodeNegotiateRequest(encoded)
	if err != ErrInvalidChecksum {
		t.Errorf("expected ErrInvalidChecksum, got %v", err)
	}
}

func TestDecodeNegotiateRequest_FrameTooSmall(t *testing.T) {
	data := []byte{NegotiateMagicRequest, NegotiateVersion}

	_, err := DecodeNegotiateRequest(data)
	if err != ErrInvalidFrameSize {
		t.Errorf("expected ErrInvalidFrameSize, got %v", err)
	}
}

func TestDecodeNegotiateRequest_FrameTooLarge(t *testing.T) {
	data := make([]byte, NegotiateMaxFrameSize+100)
	data[0] = NegotiateMagicRequest
	data[1] = NegotiateVersion

	_, err := DecodeNegotiateRequest(data)
	if err != ErrInvalidFrameSize {
		t.Errorf("expected ErrInvalidFrameSize, got %v", err)
	}
}

func TestNegotiateResponse_EncodeDecode(t *testing.T) {
	chBitmap := NewChannelBitmap(2)
	chBitmap.SetChannel(ChannelBitWS)
	chBitmap.SetChannel(ChannelBitTCP)

	codecBitmap := NewCodecBitmap(2)
	codecBitmap.SetCodec(CodecBitPlain)
	codecBitmap.SetCodec(CodecBitAES256)

	nonce := make([]byte, 8)
	rand.Read(nonce)

	resp, err := NewNegotiateResponse(chBitmap, codecBitmap, nonce, NegotiateStatusSuccess)
	if err != nil {
		t.Fatalf("failed to create response: %v", err)
	}

	// Encode
	encoded, err := resp.Encode()
	if err != nil {
		t.Fatalf("failed to encode response: %v", err)
	}

	// Verify magic and version
	if encoded[0] != NegotiateMagicResponse {
		t.Errorf("expected magic 0x%02x, got 0x%02x", NegotiateMagicResponse, encoded[0])
	}
	if encoded[1] != NegotiateVersion {
		t.Errorf("expected version 0x%02x, got 0x%02x", NegotiateVersion, encoded[1])
	}

	// Decode
	decoded, err := DecodeNegotiateResponse(encoded)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify decoded values
	if !bytes.Equal(decoded.ChannelBitmap, chBitmap) {
		t.Error("channel bitmap mismatch")
	}
	if !bytes.Equal(decoded.CodecBitmap, codecBitmap) {
		t.Error("codec bitmap mismatch")
	}
	if decoded.Status != NegotiateStatusSuccess {
		t.Errorf("expected status %d, got %d", NegotiateStatusSuccess, decoded.Status)
	}
}

func TestNegotiateResponse_Encode_InvalidSessionIDSize(t *testing.T) {
	resp := &NegotiateResponse{
		ChannelBitmap: NewChannelBitmap(2),
		CodecBitmap:   NewCodecBitmap(2),
		SessionID:     []byte{1, 2, 3}, // Invalid size (should be 8)
		Status:        NegotiateStatusSuccess,
		Padding:       []byte{},
	}

	_, err := resp.Encode()
	if err != ErrSessionIDSize {
		t.Errorf("expected ErrSessionIDSize, got %v", err)
	}
}

func TestDecodeNegotiateResponse_InvalidMagic(t *testing.T) {
	nonce := make([]byte, 8)
	rand.Read(nonce)
	resp, _ := NewNegotiateResponse(NewChannelBitmap(2), NewCodecBitmap(2), nonce, NegotiateStatusSuccess)
	encoded, _ := resp.Encode()

	// Corrupt magic (should be 0x42 for response)
	encoded[0] = NegotiateMagicRequest

	_, err := DecodeNegotiateResponse(encoded)
	if err != ErrInvalidMagic {
		t.Errorf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestNegotiateResponse_RejectStatus(t *testing.T) {
	nonce := make([]byte, 8)
	rand.Read(nonce)

	resp, err := NewNegotiateResponse([]byte{0}, []byte{0}, nonce, NegotiateStatusReject)
	if err != nil {
		t.Fatalf("failed to create reject response: %v", err)
	}

	encoded, err := resp.Encode()
	if err != nil {
		t.Fatalf("failed to encode reject response: %v", err)
	}

	decoded, err := DecodeNegotiateResponse(encoded)
	if err != nil {
		t.Fatalf("failed to decode reject response: %v", err)
	}

	if decoded.Status != NegotiateStatusReject {
		t.Errorf("expected reject status, got %d", decoded.Status)
	}
}

// ============================================================================
// Client Negotiator Tests
// ============================================================================

func TestNewClientNegotiator(t *testing.T) {
	config := &NegotiatorConfig{
		Timeout:       5 * time.Second,
		ChannelBitmap: NewChannelBitmap(2),
		CodecBitmap:   NewCodecBitmap(2),
		MaxRetryCount: 3,
	}

	negotiator := NewClientNegotiator(config)

	if negotiator.GetTimeout() != config.Timeout {
		t.Errorf("expected timeout %v, got %v", config.Timeout, negotiator.GetTimeout())
	}

	if negotiator.GetChannelBitmap() == nil {
		t.Error("expected channel bitmap to be set")
	}

	if negotiator.GetCodecBitmap() == nil {
		t.Error("expected codec bitmap to be set")
	}
}

func TestNewClientNegotiator_DefaultConfig(t *testing.T) {
	negotiator := NewClientNegotiator(nil)

	if negotiator.GetTimeout() != NegotiateDefaultTimeout {
		t.Errorf("expected default timeout %v, got %v", NegotiateDefaultTimeout, negotiator.GetTimeout())
	}
}

func TestClientNegotiator_SetTimeout(t *testing.T) {
	negotiator := NewClientNegotiator(nil)
	newTimeout := 15 * time.Second

	negotiator.SetTimeout(newTimeout)

	if negotiator.GetTimeout() != newTimeout {
		t.Errorf("expected timeout %v, got %v", newTimeout, negotiator.GetTimeout())
	}
}

func TestClientNegotiator_SetBitmaps(t *testing.T) {
	negotiator := NewClientNegotiator(nil)

	chBitmap := NewChannelBitmap(2)
	chBitmap.SetChannel(ChannelBitWS)
	chBitmap.SetChannel(ChannelBitTCP)

	codecBitmap := NewCodecBitmap(2)
	codecBitmap.SetCodec(CodecBitPlain)
	codecBitmap.SetCodec(CodecBitAES256)

	negotiator.SetChannelBitmap(chBitmap)
	negotiator.SetCodecBitmap(codecBitmap)

	if !negotiator.GetChannelBitmap().HasChannel(ChannelBitWS) {
		t.Error("expected WS channel to be set")
	}
	if !negotiator.GetCodecBitmap().HasCodec(CodecBitAES256) {
		t.Error("expected AES codec to be set")
	}
}

func TestClientNegotiator_CreateRequest(t *testing.T) {
	chBitmap := NewChannelBitmap(2)
	chBitmap.SetChannel(ChannelBitWS)
	chBitmap.SetChannel(ChannelBitTCP)

	codecBitmap := NewCodecBitmap(2)
	codecBitmap.SetCodec(CodecBitPlain)
	codecBitmap.SetCodec(CodecBitAES256)

	config := &NegotiatorConfig{
		ChannelBitmap: chBitmap,
		CodecBitmap:   codecBitmap,
	}

	negotiator := NewClientNegotiator(config)

	req, err := negotiator.CreateRequest()
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if !bytes.Equal(req.ChannelBitmap, chBitmap) {
		t.Error("channel bitmap mismatch in request")
	}
	if !bytes.Equal(req.CodecBitmap, codecBitmap) {
		t.Error("codec bitmap mismatch in request")
	}
}

func TestClientNegotiator_ProcessResponse(t *testing.T) {
	chBitmap := NewChannelBitmap(2)
	chBitmap.SetChannel(ChannelBitWS)
	chBitmap.SetChannel(ChannelBitTCP)

	codecBitmap := NewCodecBitmap(2)
	codecBitmap.SetCodec(CodecBitPlain)
	codecBitmap.SetCodec(CodecBitAES256)

	config := &NegotiatorConfig{
		ChannelBitmap: chBitmap,
		CodecBitmap:   codecBitmap,
	}

	negotiator := NewClientNegotiator(config)

	// Create mock response (success with intersection)
	resp := &NegotiateResponse{
		ChannelBitmap: chBitmap,
		CodecBitmap:   codecBitmap,
		SessionID:     make([]byte, 8),
		Status:        NegotiateStatusSuccess,
	}
	rand.Read(resp.SessionID)

	result, err := negotiator.ProcessResponse(resp)
	if err != nil {
		t.Fatalf("failed to process response: %v", err)
	}

	if !result.IsSuccess() {
		t.Error("expected result to be successful")
	}
	if !result.HasCommonChannels() {
		t.Error("expected common channels")
	}
	if !result.HasCommonCodecs() {
		t.Error("expected common codecs")
	}
	if len(result.SessionID) != 8 {
		t.Errorf("expected SessionID length 8, got %d", len(result.SessionID))
	}
}

func TestClientNegotiator_ProcessResponse_Reject(t *testing.T) {
	config := DefaultNegotiatorConfig()
	negotiator := NewClientNegotiator(config)

	resp := &NegotiateResponse{
		ChannelBitmap: []byte{0},
		CodecBitmap:   []byte{0},
		SessionID:     make([]byte, 8),
		Status:        NegotiateStatusReject,
	}

	_, err := negotiator.ProcessResponse(resp)
	if err != ErrNoCommonChannels {
		t.Errorf("expected ErrNoCommonChannels, got %v", err)
	}
}

func TestClientNegotiator_Negotiate_EmptyBitmap(t *testing.T) {
	config := &NegotiatorConfig{
		ChannelBitmap: NewChannelBitmap(2), // Empty
		CodecBitmap:   NewCodecBitmap(2),   // Empty
	}

	negotiator := NewClientNegotiator(config)

	_, err := negotiator.Negotiate(ChannelBitWS)
	if err != ErrNoCommonChannels {
		t.Errorf("expected ErrNoCommonChannels for empty channel bitmap, got %v", err)
	}
}

func TestClientNegotiator_Negotiate_DefaultChannelNotSupported(t *testing.T) {
	chBitmap := NewChannelBitmap(2)
	chBitmap.SetChannel(ChannelBitTCP) // WS not set

	codecBitmap := NewCodecBitmap(2)
	codecBitmap.SetCodec(CodecBitPlain)

	config := &NegotiatorConfig{
		ChannelBitmap: chBitmap,
		CodecBitmap:   codecBitmap,
	}

	negotiator := NewClientNegotiator(config)

	_, err := negotiator.Negotiate(ChannelBitWS) // Requesting WS as default
	if err == nil {
		t.Error("expected error when default channel not supported")
	}
}

// ============================================================================
// Server Negotiator Tests
// ============================================================================

func TestNewServerNegotiator(t *testing.T) {
	config := &NegotiatorConfig{
		Timeout:       5 * time.Second,
		ChannelBitmap: NewChannelBitmap(2),
		CodecBitmap:   NewCodecBitmap(2),
		MaxRetryCount: 3,
	}

	negotiator := NewServerNegotiator(config)

	if negotiator.GetTimeout() != config.Timeout {
		t.Errorf("expected timeout %v, got %v", config.Timeout, negotiator.GetTimeout())
	}
}

func TestServerNegotiator_HandleRequest(t *testing.T) {
	// Server supports WS + TCP + ICMP (UDP already used in client)
	serverChBitmap := NewChannelBitmap(2)
	serverChBitmap.SetChannel(ChannelBitWS)
	serverChBitmap.SetChannel(ChannelBitTCP)
	serverChBitmap.SetChannel(ChannelBitICMP) // Use ICMP instead of QUIC

	serverCodecBitmap := NewCodecBitmap(2)
	serverCodecBitmap.SetCodec(CodecBitPlain)
	serverCodecBitmap.SetCodec(CodecBitAES256)
	serverCodecBitmap.SetCodec(CodecBitChaCha20)

	serverConfig := &NegotiatorConfig{
		ChannelBitmap: serverChBitmap,
		CodecBitmap:   serverCodecBitmap,
	}

	server := NewServerNegotiator(serverConfig)

	// Client supports WS + TCP + UDP
	clientChBitmap := NewChannelBitmap(2)
	clientChBitmap.SetChannel(ChannelBitWS)
	clientChBitmap.SetChannel(ChannelBitTCP)
	clientChBitmap.SetChannel(ChannelBitUDP)

	clientCodecBitmap := NewCodecBitmap(2)
	clientCodecBitmap.SetCodec(CodecBitPlain)
	clientCodecBitmap.SetCodec(CodecBitAES256)

	req, _ := NewNegotiateRequest(clientChBitmap, clientCodecBitmap, nil)

	// Handle request
	resp, err := server.HandleRequest(req)
	if err != nil {
		t.Fatalf("failed to handle request: %v", err)
	}

	// Verify intersection
	if !resp.ChannelBitmap.HasChannel(ChannelBitWS) {
		t.Error("expected WS in intersection")
	}
	if !resp.ChannelBitmap.HasChannel(ChannelBitTCP) {
		t.Error("expected TCP in intersection")
	}
	if resp.ChannelBitmap.HasChannel(ChannelBitICMP) {
		t.Error("expected ICMP NOT in intersection (client doesn't support)")
	}
	if resp.ChannelBitmap.HasChannel(ChannelBitUDP) {
		t.Error("expected UDP NOT in intersection (server doesn't support)")
	}

	if resp.Status != NegotiateStatusSuccess {
		t.Errorf("expected success status, got %d", resp.Status)
	}

	if len(resp.SessionID) != NegotiateSessionIDSize {
		t.Errorf("expected SessionID size %d, got %d", NegotiateSessionIDSize, len(resp.SessionID))
	}
}

func TestServerNegotiator_HandleRequest_NoCommonChannels(t *testing.T) {
	// Server supports only UDP
	serverChBitmap := NewChannelBitmap(2)
	serverChBitmap.SetChannel(ChannelBitUDP)

	serverCodecBitmap := NewCodecBitmap(2)
	serverCodecBitmap.SetCodec(CodecBitPlain)

	serverConfig := &NegotiatorConfig{
		ChannelBitmap: serverChBitmap,
		CodecBitmap:   serverCodecBitmap,
	}

	server := NewServerNegotiator(serverConfig)

	// Client supports only WS + TCP (no UDP)
	clientChBitmap := NewChannelBitmap(2)
	clientChBitmap.SetChannel(ChannelBitWS)
	clientChBitmap.SetChannel(ChannelBitTCP)

	clientCodecBitmap := NewCodecBitmap(2)
	clientCodecBitmap.SetCodec(CodecBitPlain)

	req, _ := NewNegotiateRequest(clientChBitmap, clientCodecBitmap, nil)

	// Handle request - should reject
	resp, err := server.HandleRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != NegotiateStatusReject {
		t.Errorf("expected reject status, got %d", resp.Status)
	}
}

func TestServerNegotiator_HandleRequest_NoCommonCodecs(t *testing.T) {
	// Server supports WS + TCP
	serverChBitmap := NewChannelBitmap(2)
	serverChBitmap.SetChannel(ChannelBitWS)
	serverChBitmap.SetChannel(ChannelBitTCP)

	// Server supports only AES
	serverCodecBitmap := NewCodecBitmap(2)
	serverCodecBitmap.SetCodec(CodecBitAES256)

	serverConfig := &NegotiatorConfig{
		ChannelBitmap: serverChBitmap,
		CodecBitmap:   serverCodecBitmap,
	}

	server := NewServerNegotiator(serverConfig)

	// Client supports WS + TCP
	clientChBitmap := NewChannelBitmap(2)
	clientChBitmap.SetChannel(ChannelBitWS)
	clientChBitmap.SetChannel(ChannelBitTCP)

	// Client supports only Plain (no AES)
	clientCodecBitmap := NewCodecBitmap(2)
	clientCodecBitmap.SetCodec(CodecBitPlain)

	req, _ := NewNegotiateRequest(clientChBitmap, clientCodecBitmap, nil)

	// Handle request - should reject
	resp, err := server.HandleRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != NegotiateStatusReject {
		t.Errorf("expected reject status, got %d", resp.Status)
	}
}

func TestServerNegotiator_HandleRawRequest(t *testing.T) {
	serverChBitmap := NewChannelBitmap(2)
	serverChBitmap.SetChannel(ChannelBitWS)
	serverChBitmap.SetChannel(ChannelBitTCP)

	serverCodecBitmap := NewCodecBitmap(2)
	serverCodecBitmap.SetCodec(CodecBitPlain)
	serverCodecBitmap.SetCodec(CodecBitAES256)

	serverConfig := &NegotiatorConfig{
		ChannelBitmap: serverChBitmap,
		CodecBitmap:   serverCodecBitmap,
	}

	server := NewServerNegotiator(serverConfig)

	// Client request
	clientChBitmap := NewChannelBitmap(2)
	clientChBitmap.SetChannel(ChannelBitWS)

	clientCodecBitmap := NewCodecBitmap(2)
	clientCodecBitmap.SetCodec(CodecBitAES256)

	req, _ := NewNegotiateRequest(clientChBitmap, clientCodecBitmap, nil)
	encoded, _ := req.Encode()

	// Handle raw request
	resp, err := server.HandleRawRequest(encoded)
	if err != nil {
		t.Fatalf("failed to handle raw request: %v", err)
	}

	if resp.Status != NegotiateStatusSuccess {
		t.Errorf("expected success status, got %d", resp.Status)
	}
}

func TestServerNegotiator_GetAvailableChannels(t *testing.T) {
	serverChBitmap := NewChannelBitmap(2)
	serverChBitmap.SetChannel(ChannelBitWS)
	serverChBitmap.SetChannel(ChannelBitTCP)

	serverCodecBitmap := NewCodecBitmap(2)
	serverCodecBitmap.SetCodec(CodecBitPlain)

	serverConfig := &NegotiatorConfig{
		ChannelBitmap: serverChBitmap,
		CodecBitmap:   serverCodecBitmap,
	}

	server := NewServerNegotiator(serverConfig)

	ids := server.GetAvailableChannels()
	expected := []ChannelID{ChannelIDWS, ChannelIDTCP}

	if len(ids) != len(expected) {
		t.Errorf("expected %d channel IDs, got %d", len(expected), len(ids))
	}
}

func TestServerNegotiator_GetAvailableCodecs(t *testing.T) {
	serverCodecBitmap := NewCodecBitmap(2)
	serverCodecBitmap.SetCodec(CodecBitPlain)
	serverCodecBitmap.SetCodec(CodecBitAES256)

	serverConfig := &NegotiatorConfig{
		CodecBitmap: serverCodecBitmap,
	}

	server := NewServerNegotiator(serverConfig)

	ids := server.GetAvailableCodecs()
	expected := []CodecID{CodecIDPlain, CodecIDAES256}

	if len(ids) != len(expected) {
		t.Errorf("expected %d codec IDs, got %d", len(expected), len(ids))
	}
}

// ============================================================================
// Session Manager Tests
// ============================================================================

func TestNewSessionManager(t *testing.T) {
	sm := NewSessionManager()

	if sm.SessionCount() != 0 {
		t.Error("expected empty session manager")
	}
}

func TestSessionManager_AddSession(t *testing.T) {
	sm := NewSessionManager()

	sessionID := make([]byte, 8)
	rand.Read(sessionID)

	result := &Result{
		AvailableChannels: NewChannelBitmap(2),
		AvailableCodecs:   NewCodecBitmap(2),
		SessionID:         sessionID,
		Status:            NegotiateStatusSuccess,
	}

	sm.AddSession(sessionID, result)

	if sm.SessionCount() != 1 {
		t.Errorf("expected 1 session, got %d", sm.SessionCount())
	}

	if !sm.HasSession(sessionID) {
		t.Error("expected session to exist")
	}
}

func TestSessionManager_GetSession(t *testing.T) {
	sm := NewSessionManager()

	sessionID := make([]byte, 8)
	rand.Read(sessionID)

	result := &Result{
		SessionID: sessionID,
		Status:    NegotiateStatusSuccess,
	}

	sm.AddSession(sessionID, result)

	got, ok := sm.GetSession(sessionID)
	if !ok {
		t.Error("expected to find session")
	}
	if got.Status != result.Status {
		t.Error("session status mismatch")
	}

	// Test non-existent session
	_, ok = sm.GetSession([]byte{0, 0, 0, 0, 0, 0, 0, 1})
	if ok {
		t.Error("expected to not find non-existent session")
	}
}

func TestSessionManager_RemoveSession(t *testing.T) {
	sm := NewSessionManager()

	sessionID := make([]byte, 8)
	rand.Read(sessionID)

	result := &Result{
		SessionID: sessionID,
	}

	sm.AddSession(sessionID, result)
	sm.RemoveSession(sessionID)

	if sm.HasSession(sessionID) {
		t.Error("expected session to be removed")
	}

	if sm.SessionCount() != 0 {
		t.Errorf("expected 0 sessions, got %d", sm.SessionCount())
	}
}

func TestSessionManager_SessionCount(t *testing.T) {
	sm := NewSessionManager()

	// Add multiple sessions
	for i := 0; i < 5; i++ {
		sessionID := make([]byte, 8)
		sessionID[0] = byte(i)
		sm.AddSession(sessionID, &Result{})
	}

	if sm.SessionCount() != 5 {
		t.Errorf("expected 5 sessions, got %d", sm.SessionCount())
	}
}

// ============================================================================
// Result Tests
// ============================================================================

func TestResult_IsSuccess(t *testing.T) {
	successResult := &Result{Status: NegotiateStatusSuccess}
	if !successResult.IsSuccess() {
		t.Error("expected success result to be successful")
	}

	rejectResult := &Result{Status: NegotiateStatusReject}
	if rejectResult.IsSuccess() {
		t.Error("expected reject result to not be successful")
	}
}

func TestResult_HasCommonChannels(t *testing.T) {
	chBitmap := NewChannelBitmap(2)
	chBitmap.SetChannel(ChannelBitWS)

	result := &Result{AvailableChannels: chBitmap}
	if !result.HasCommonChannels() {
		t.Error("expected to have common channels")
	}

	emptyResult := &Result{AvailableChannels: NewChannelBitmap(2)}
	if emptyResult.HasCommonChannels() {
		t.Error("expected empty result to not have common channels")
	}
}

func TestResult_HasCommonCodecs(t *testing.T) {
	codecBitmap := NewCodecBitmap(2)
	codecBitmap.SetCodec(CodecBitPlain)

	result := &Result{AvailableCodecs: codecBitmap}
	if !result.HasCommonCodecs() {
		t.Error("expected to have common codecs")
	}

	emptyResult := &Result{AvailableCodecs: NewCodecBitmap(2)}
	if emptyResult.HasCommonCodecs() {
		t.Error("expected empty result to not have common codecs")
	}
}

func TestResult_GetAvailableChannelIDs(t *testing.T) {
	chBitmap := NewChannelBitmap(2)
	chBitmap.SetChannel(ChannelBitWS)
	chBitmap.SetChannel(ChannelBitTCP)

	result := &Result{AvailableChannels: chBitmap}
	ids := result.GetAvailableChannelIDs()

	expected := []ChannelID{ChannelIDWS, ChannelIDTCP}
	if len(ids) != len(expected) {
		t.Errorf("expected %d IDs, got %d", len(expected), len(ids))
	}
}

func TestResult_GetAvailableCodecIDs(t *testing.T) {
	codecBitmap := NewCodecBitmap(2)
	codecBitmap.SetCodec(CodecBitPlain)
	codecBitmap.SetCodec(CodecBitAES256)

	result := &Result{AvailableCodecs: codecBitmap}
	ids := result.GetAvailableCodecIDs()

	expected := []CodecID{CodecIDPlain, CodecIDAES256}
	if len(ids) != len(expected) {
		t.Errorf("expected %d IDs, got %d", len(expected), len(ids))
	}
}

// ============================================================================
// NegotiatorConfig Tests
// ============================================================================

func TestDefaultNegotiatorConfig(t *testing.T) {
	config := DefaultNegotiatorConfig()

	if config.Timeout != NegotiateDefaultTimeout {
		t.Errorf("expected default timeout %v, got %v", NegotiateDefaultTimeout, config.Timeout)
	}

	if config.MaxRetryCount != 3 {
		t.Errorf("expected max retry count 3, got %d", config.MaxRetryCount)
	}

	if config.ChannelBitmap == nil {
		t.Error("expected channel bitmap to be set")
	}

	if config.CodecBitmap == nil {
		t.Error("expected codec bitmap to be set")
	}
}

// ============================================================================
// Codec/Channel ID Conversion Tests
// ============================================================================

func TestCodecBitToID(t *testing.T) {
	if CodecBitToID(CodecBitPlain) != CodecIDPlain {
		t.Error("CodecBit to ID conversion failed for Plain")
	}
	if CodecBitToID(CodecBitAES256) != CodecIDAES256 {
		t.Error("CodecBit to ID conversion failed for AES")
	}
}

func TestCodecIDToBit(t *testing.T) {
	if CodecIDToBit(CodecIDPlain) != CodecBitPlain {
		t.Error("CodecID to Bit conversion failed for Plain")
	}
	if CodecIDToBit(CodecIDAES256) != CodecBitAES256 {
		t.Error("CodecID to Bit conversion failed for AES")
	}
}

func TestChannelBitToID(t *testing.T) {
	if ChannelBitToID(ChannelBitWS) != ChannelIDWS {
		t.Error("ChannelBit to ID conversion failed for WS")
	}
	if ChannelBitToID(ChannelBitTCP) != ChannelIDTCP {
		t.Error("ChannelBit to ID conversion failed for TCP")
	}
}

func TestChannelIDToBit(t *testing.T) {
	if ChannelIDToBit(ChannelIDWS) != ChannelBitWS {
		t.Error("ChannelID to Bit conversion failed for WS")
	}
	if ChannelIDToBit(ChannelIDTCP) != ChannelBitTCP {
		t.Error("ChannelID to Bit conversion failed for TCP")
	}
}
