package voidbus

import (
	"errors"
	"testing"
	"time"

	"github.com/Script-OS/VoidBus/codec/plain"
)

// === New and NewWithConfig Tests ===

func TestNew(t *testing.T) {
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("expected New() to succeed, got error: %v", err)
	}

	if bus == nil {
		t.Fatal("expected non-nil bus")
	}

	if bus.config == nil {
		t.Error("expected config to be set")
	}
}

func TestNewWithConfig_Valid(t *testing.T) {
	config := DefaultBusConfig()
	bus, err := New(config)
	if err != nil {
		t.Fatalf("expected New(config) to succeed, got error: %v", err)
	}

	if bus == nil {
		t.Fatal("expected non-nil bus")
	}
}

func TestNewWithConfig_InvalidMaxCodecDepth(t *testing.T) {
	config := DefaultBusConfig()
	config.MaxCodecDepth = 0 // Invalid

	_, err := New(config)
	if err == nil {
		t.Fatal("expected error for invalid MaxCodecDepth")
	}

	// Verify error type
	var voidBusErr *VoidBusError
	if !errors.As(err, &voidBusErr) {
		t.Errorf("expected VoidBusError type, got %T", err)
	}
}

func TestNewWithConfig_InvalidMTU(t *testing.T) {
	config := DefaultBusConfig()
	config.DefaultMTU = 10 // Less than MinMTU

	_, err := New(config)
	if err == nil {
		t.Fatal("expected error for invalid MTU")
	}
}

func TestNewWithConfig_InvalidTimeout(t *testing.T) {
	config := DefaultBusConfig()
	config.FragmentTimeout = 0 // Invalid

	_, err := New(config)
	if err == nil {
		t.Fatal("expected error for invalid FragmentTimeout")
	}
}

// === SetKey Tests ===

func TestSetKey_ValidKey(t *testing.T) {
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// 32-byte key for AES-256-GCM
	key := []byte("32-byte-secret-key-for-aes-256!!")
	err = bus.SetKey(key)
	if err != nil {
		t.Fatalf("expected SetKey() to succeed, got error: %v", err)
	}

	if bus.keyProvider == nil {
		t.Error("expected keyProvider to be set")
	}
}

func TestSetKey_InvalidKey(t *testing.T) {
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Test nil key
	err = bus.SetKey(nil)
	if err == nil {
		t.Fatal("expected error for nil key")
	}

	// Verify error is wrapped
	var voidBusErr *VoidBusError
	if !errors.As(err, &voidBusErr) {
		t.Errorf("expected VoidBusError type, got %T", err)
	}

	if GetModule(err) != "keyprovider" {
		t.Errorf("expected module 'keyprovider', got '%s'", GetModule(err))
	}

	// Test empty key
	err = bus.SetKey([]byte{})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestSetKey_EmptyKey(t *testing.T) {
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = bus.SetKey([]byte{})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestSetKeyProvider(t *testing.T) {
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// SetKey now accepts a byte slice directly
	err = bus.SetKey(nil)
	if err == nil {
		t.Fatal("expected error for nil key")
	}
}

// === Codec Tests ===

func TestAddCodec(t *testing.T) {
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Add plain codec (no key required)
	plainCodec := plain.New()
	err = bus.RegisterCodec(plainCodec)
	if err != nil {
		t.Fatalf("expected RegisterCodec() to succeed, got error: %v", err)
	}

	// Verify codec was added
	availableCodes := bus.codecManager.GetAvailableCodes()
	if len(availableCodes) != 1 {
		t.Errorf("expected 1 codec, got %d", len(availableCodes))
	}

	if availableCodes[0] != "plain" {
		t.Errorf("expected code 'plain', got '%s'", availableCodes[0])
	}
}

func TestAddCodec_DuplicateCode(t *testing.T) {
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	plainCodec1 := plain.New()
	err = bus.RegisterCodec(plainCodec1)
	if err != nil {
		t.Fatalf("first RegisterCodec() failed: %v", err)
	}

	// Add duplicate code (same codec instance is ok, different instance with same code is error)
	plainCodec2 := plain.New()
	plainCodec2.SetCode("plain") // Same code as plainCodec1
	err = bus.RegisterCodec(plainCodec2)
	if err == nil {
		t.Fatal("expected error for duplicate codec code")
	}
}

func TestSetMaxCodecDepth(t *testing.T) {
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = bus.SetMaxCodecDepth(3)
	if err != nil {
		t.Fatalf("expected SetMaxCodecDepth() to succeed, got error: %v", err)
	}

	if bus.codecManager.GetMaxDepth() != 3 {
		t.Errorf("expected max depth 3, got %d", bus.codecManager.GetMaxDepth())
	}
}

func TestSetMaxCodecDepth_Invalid(t *testing.T) {
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Invalid depth (too large)
	err = bus.SetMaxCodecDepth(100)
	if err == nil {
		t.Fatal("expected error for invalid max depth")
	}
}

// === Validate Tests ===

// TestValidate_NoCodec - Validate method removed
// TestValidate_NoChannel - Validate method removed
// TestValidate_Valid - Validate method removed

// === Stats Tests ===
// TestStats - Stats method removed
// TestModuleStats - ModuleStats method removed

// === Lifecycle Tests ===
// TestBus_Name - Name method removed
// TestBus_IsRunning - IsRunning method removed
// TestBus_IsConnected - IsConnected method removed
// TestBus_GetConfig - GetConfig method removed

// === GetNegotiationInfo Tests ===
// TestBus_GetNegotiationInfo - GetNegotiationInfo method removed

// === Connect Tests ===
// TestBus_Connect - Connect method removed
// TestBus_Connect_AlreadyConnected - Connect method removed

// === Error Handler Tests ===
// TestBus_OnMessage - OnMessage method removed
// TestBus_OnError - OnError method removed

// === Config Tests ===

func TestDefaultBusConfig(t *testing.T) {
	config := DefaultBusConfig()

	if config.MaxCodecDepth != 2 {
		t.Errorf("expected MaxCodecDepth=2, got %d", config.MaxCodecDepth)
	}

	if config.DefaultMTU != 1024 {
		t.Errorf("expected DefaultMTU=1024, got %d", config.DefaultMTU)
	}

	if config.ReceiveMode != ReceiveModeBlocking {
		t.Errorf("expected ReceiveModeBlocking, got %d", config.ReceiveMode)
	}
}

func TestBusConfig_Validate(t *testing.T) {
	// Valid config
	validConfig := DefaultBusConfig()
	err := validConfig.Validate()
	if err != nil {
		t.Fatalf("expected valid config to pass: %v", err)
	}

	// Invalid MaxCodecDepth (too small)
	invalidConfig1 := DefaultBusConfig()
	invalidConfig1.MaxCodecDepth = 0
	err = invalidConfig1.Validate()
	if err == nil {
		t.Error("expected error for MaxCodecDepth=0")
	}

	// Invalid MaxCodecDepth (too large)
	invalidConfig2 := DefaultBusConfig()
	invalidConfig2.MaxCodecDepth = 10
	err = invalidConfig2.Validate()
	if err == nil {
		t.Error("expected error for MaxCodecDepth=10")
	}

	// Invalid MTU
	invalidConfig3 := DefaultBusConfig()
	invalidConfig3.DefaultMTU = 10 // Less than MinMTU
	err = invalidConfig3.Validate()
	if err == nil {
		t.Error("expected error for invalid MTU")
	}

	// Invalid FragmentTimeout
	invalidConfig4 := DefaultBusConfig()
	invalidConfig4.FragmentTimeout = 0
	err = invalidConfig4.Validate()
	if err == nil {
		t.Error("expected error for invalid FragmentTimeout")
	}

	// Invalid MaxRetransmit
	invalidConfig5 := DefaultBusConfig()
	invalidConfig5.MaxRetransmit = -1
	err = invalidConfig5.Validate()
	if err == nil {
		t.Error("expected error for negative MaxRetransmit")
	}
}

func TestNegotiationConfig_Validate(t *testing.T) {
	// Valid config
	validConfig := &NegotiationConfig{
		SupportedCodes: []string{"A", "B"},
		MaxDepth:       2,
		NegotiatedAt:   time.Now(),
	}
	err := validConfig.Validate()
	if err != nil {
		t.Fatalf("expected valid config to pass: %v", err)
	}

	// Invalid - empty codes
	invalidConfig1 := &NegotiationConfig{
		SupportedCodes: []string{},
		MaxDepth:       2,
	}
	err = invalidConfig1.Validate()
	if err == nil {
		t.Error("expected error for empty SupportedCodes")
	}

	// Invalid - depth too small
	invalidConfig2 := &NegotiationConfig{
		SupportedCodes: []string{"A"},
		MaxDepth:       0,
	}
	err = invalidConfig2.Validate()
	if err == nil {
		t.Error("expected error for MaxDepth=0")
	}
}

// === Module Registry Tests ===
// All module registry tests removed - Bus no longer implements Module interface

// === Benchmark Tests ===

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		New(nil)
	}
}

func BenchmarkSetKey(b *testing.B) {
	bus, _ := New(nil)
	key := []byte("32-byte-secret-key-for-aes-256!!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.SetKey(key)
	}
}

func BenchmarkAddCodec(b *testing.B) {
	bus, _ := New(nil)
	plainCodec := plain.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.RegisterCodec(plainCodec)
	}
}
