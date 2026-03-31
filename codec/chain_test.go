package codec

import (
	"errors"
	"testing"

	"github.com/Script-OS/VoidBus/keyprovider"
	"github.com/Script-OS/VoidBus/keyprovider/embedded"
)

func TestNewChain(t *testing.T) {
	chain := NewChain()

	if chain == nil {
		t.Error("NewChain() should return non-nil chain")
	}

	if !chain.IsEmpty() {
		t.Error("NewChain() should return empty chain")
	}

	if chain.Length() != 0 {
		t.Errorf("NewChain() Length() = %d, want 0", chain.Length())
	}

	if chain.SecurityLevel() != SecurityLevelNone {
		t.Errorf("Empty chain SecurityLevel() = %d, want None(0)", chain.SecurityLevel())
	}
}

func TestChain_AddCodec(t *testing.T) {
	chain := NewChain()

	plainCodec := &mockCodec{level: SecurityLevelNone}
	chain.AddCodec(plainCodec)

	if chain.Length() != 1 {
		t.Errorf("AddCodec() Length() = %d, want 1", chain.Length())
	}

	if chain.IsEmpty() {
		t.Error("AddCodec() chain should not be empty")
	}
}

func TestChain_AddCodecAt(t *testing.T) {
	chain := NewChain()

	c1 := &mockCodec{id: "codec1", level: SecurityLevelLow}
	c2 := &mockCodec{id: "codec2", level: SecurityLevelMedium}

	chain.AddCodec(c1)
	chain.AddCodecAt(c2, 0)

	ids := chain.InternalIDs()
	if ids[0] != "codec2" {
		t.Errorf("AddCodecAt(0) should insert at beginning, got %s", ids[0])
	}
	if ids[1] != "codec1" {
		t.Errorf("Second codec should be codec1, got %s", ids[1])
	}
}

func TestChain_RemoveCodecAt(t *testing.T) {
	chain := NewChain()

	c1 := &mockCodec{id: "codec1"}
	c2 := &mockCodec{id: "codec2"}
	c3 := &mockCodec{id: "codec3"}

	chain.AddCodec(c1).AddCodec(c2).AddCodec(c3)
	chain.RemoveCodecAt(1)

	ids := chain.InternalIDs()
	if len(ids) != 2 {
		t.Errorf("RemoveCodecAt() Length() = %d, want 2", len(ids))
	}
	if ids[0] != "codec1" || ids[1] != "codec3" {
		t.Errorf("RemoveCodecAt(1) failed, got %v", ids)
	}
}

func TestChain_EncodeDecode(t *testing.T) {
	// Create chain with mock codecs that transform data
	chain := NewChain()

	// Codec that adds prefix "A"
	c1 := &transformCodec{prefix: "A"}
	// Codec that adds prefix "B"
	c2 := &transformCodec{prefix: "B"}

	chain.AddCodec(c1).AddCodec(c2)

	data := []byte("test")
	encoded, err := chain.Encode(data)
	if err != nil {
		t.Errorf("Encode() error = %v", err)
	}

	// Encode order: test -> c1.Encode -> "Atest" -> c2.Encode -> "BAtest"
	if string(encoded) != "BAtest" {
		t.Errorf("Encode() = %s, want BAtest", encoded)
	}

	decoded, err := chain.Decode(encoded)
	if err != nil {
		t.Errorf("Decode() error = %v", err)
	}

	// Decode order (reverse): "BAtest" -> c2.Decode -> "Atest" -> c1.Decode -> "test"
	if string(decoded) != "test" {
		t.Errorf("Decode() = %s, want test", decoded)
	}
}

func TestChain_EncodeDecode_Empty(t *testing.T) {
	chain := NewChain()

	data := []byte("test")
	encoded, err := chain.Encode(data)
	if err != nil {
		t.Errorf("Empty chain Encode() error = %v", err)
	}

	if string(encoded) != string(data) {
		t.Errorf("Empty chain should not transform data")
	}

	decoded, err := chain.Decode(encoded)
	if err != nil {
		t.Errorf("Empty chain Decode() error = %v", err)
	}

	if string(decoded) != string(data) {
		t.Errorf("Empty chain should not transform data")
	}
}

func TestChain_SecurityLevel(t *testing.T) {
	tests := []struct {
		name     string
		codecs   []SecurityLevel
		expected SecurityLevel
	}{
		{"empty chain", []SecurityLevel{}, SecurityLevelNone},
		{"single low", []SecurityLevel{SecurityLevelLow}, SecurityLevelLow},
		{"single medium", []SecurityLevel{SecurityLevelMedium}, SecurityLevelMedium},
		{"single high", []SecurityLevel{SecurityLevelHigh}, SecurityLevelHigh},
		{"mixed - low min", []SecurityLevel{SecurityLevelHigh, SecurityLevelLow, SecurityLevelMedium}, SecurityLevelLow},
		{"mixed - none min", []SecurityLevel{SecurityLevelHigh, SecurityLevelNone, SecurityLevelMedium}, SecurityLevelNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := NewChain()
			for _, level := range tt.codecs {
				chain.AddCodec(&mockCodec{level: level})
			}

			result := chain.SecurityLevel()
			if result != tt.expected {
				t.Errorf("SecurityLevel() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestChain_SetKeyProvider(t *testing.T) {
	chain := NewChain()

	// Add key-aware codec
	keyAwareCodec := &mockKeyAwareCodec{requireKey: true}
	// Add non-key-aware codec
	plainCodec := &mockCodec{id: "plain"}

	chain.AddCodec(keyAwareCodec).AddCodec(plainCodec)

	key := []byte("test-key-16-byte")
	kp, _ := embedded.New(key, "", "")

	err := chain.SetKeyProvider(kp)
	if err != nil {
		t.Errorf("SetKeyProvider() error = %v", err)
	}

	if keyAwareCodec.keyProvider == nil {
		t.Error("SetKeyProvider() should set key provider on key-aware codec")
	}
}

func TestChain_SetKeyProvider_Nil(t *testing.T) {
	chain := NewChain()
	chain.AddCodec(&mockCodec{})

	err := chain.SetKeyProvider(nil)
	if !errors.Is(err, ErrInvalidKeyProvider) {
		t.Errorf("SetKeyProvider(nil) should return ErrInvalidKeyProvider, got %v", err)
	}
}

func TestChain_Clone(t *testing.T) {
	chain := NewChain()
	c1 := &mockCodec{id: "codec1"}
	c2 := &mockCodec{id: "codec2"}
	chain.AddCodec(c1).AddCodec(c2)

	clone := chain.Clone()

	if clone.Length() != chain.Length() {
		t.Errorf("Clone() Length() = %d, want %d", clone.Length(), chain.Length())
	}

	ids := clone.InternalIDs()
	origIDs := chain.InternalIDs()
	if ids[0] != origIDs[0] || ids[1] != origIDs[1] {
		t.Errorf("Clone() should preserve codec order")
	}

	// Modify clone should not affect original
	clone.AddCodec(&mockCodec{id: "codec3"})
	if chain.Length() != 2 {
		t.Errorf("Modifying clone should not affect original")
	}
}

func TestChain_InternalIDs(t *testing.T) {
	chain := NewChain()
	c1 := &mockCodec{id: "codec1"}
	c2 := &mockCodec{id: "codec2"}
	c3 := &mockCodec{id: "codec3"}

	chain.AddCodec(c1).AddCodec(c2).AddCodec(c3)

	ids := chain.InternalIDs()
	expected := []string{"codec1", "codec2", "codec3"}

	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("InternalIDs()[%d] = %s, want %s", i, id, expected[i])
		}
	}
}

func TestChain_GetCodec(t *testing.T) {
	chain := NewChain()
	c1 := &mockCodec{id: "codec1"}
	c2 := &mockCodec{id: "codec2"}

	chain.AddCodec(c1).AddCodec(c2)

	tests := []struct {
		index    int
		wantID   string
		hasError bool
	}{
		{0, "codec1", false},
		{1, "codec2", false},
		{-1, "", true},
		{2, "", true},
		{100, "", true},
	}

	for _, tt := range tests {
		codec, err := chain.GetCodec(tt.index)

		if tt.hasError {
			if err == nil {
				t.Errorf("GetCodec(%d) should return error", tt.index)
			}
		} else {
			if err != nil {
				t.Errorf("GetCodec(%d) error = %v", tt.index, err)
			}
			if codec.InternalID() != tt.wantID {
				t.Errorf("GetCodec(%d) ID = %s, want %s", tt.index, codec.InternalID(), tt.wantID)
			}
		}
	}
}

func TestChain_MaxLength(t *testing.T) {
	chain := NewChain()

	// Add 5 codecs (should succeed)
	for i := 0; i < 5; i++ {
		chain.AddCodec(&mockCodec{id: string(rune('a' + i))})
	}

	if chain.Length() != 5 {
		t.Errorf("Chain should allow 5 codecs, got %d", chain.Length())
	}

	// Try to add 6th codec (should be ignored)
	chain.AddCodec(&mockCodec{id: "extra"})
	if chain.Length() != 5 {
		t.Errorf("Chain should limit to 5 codecs, got %d", chain.Length())
	}
}

func TestNewChainWithCodecs(t *testing.T) {
	c1 := &mockCodec{id: "codec1"}
	c2 := &mockCodec{id: "codec2"}

	chain := NewChainWithCodecs(c1, c2)

	if chain.Length() != 2 {
		t.Errorf("NewChainWithCodecs() Length() = %d, want 2", chain.Length())
	}

	ids := chain.InternalIDs()
	if ids[0] != "codec1" || ids[1] != "codec2" {
		t.Errorf("NewChainWithCodecs() codec order incorrect")
	}
}

// Mock implementations for testing

type mockCodec struct {
	id    string
	level SecurityLevel
}

func (m *mockCodec) Encode(data []byte) ([]byte, error) {
	return data, nil
}

func (m *mockCodec) Decode(data []byte) ([]byte, error) {
	return data, nil
}

func (m *mockCodec) InternalID() string {
	return m.id
}

func (m *mockCodec) SecurityLevel() SecurityLevel {
	return m.level
}

type transformCodec struct {
	prefix string
}

func (t *transformCodec) Encode(data []byte) ([]byte, error) {
	return append([]byte(t.prefix), data...), nil
}

func (t *transformCodec) Decode(data []byte) ([]byte, error) {
	if len(data) < len(t.prefix) {
		return nil, errors.New("data too short")
	}
	return data[len(t.prefix):], nil
}

func (t *transformCodec) InternalID() string {
	return "transform_" + t.prefix
}

func (t *transformCodec) SecurityLevel() SecurityLevel {
	return SecurityLevelLow
}

type mockKeyAwareCodec struct {
	requireKey    bool
	keyProvider   keyprovider.KeyProvider
	keyAlgorithm_ string
}

func (m *mockKeyAwareCodec) Encode(data []byte) ([]byte, error) {
	return data, nil
}

func (m *mockKeyAwareCodec) Decode(data []byte) ([]byte, error) {
	return data, nil
}

func (m *mockKeyAwareCodec) InternalID() string {
	return "keyaware"
}

func (m *mockKeyAwareCodec) SecurityLevel() SecurityLevel {
	return SecurityLevelMedium
}

func (m *mockKeyAwareCodec) SetKeyProvider(provider keyprovider.KeyProvider) error {
	m.keyProvider = provider
	return nil
}

func (m *mockKeyAwareCodec) RequiresKey() bool {
	return m.requireKey
}

func (m *mockKeyAwareCodec) KeyAlgorithm() string {
	return m.keyAlgorithm_
}
