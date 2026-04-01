package voidbus

import (
	"errors"
	"testing"
	"time"

	"github.com/Script-OS/VoidBus/codec/plain"
)

// === New and NewWithConfig Tests ===

func TestNew(t *testing.T) {
	bus, err := New()
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
	bus, err := NewWithConfig(config)
	if err != nil {
		t.Fatalf("expected NewWithConfig() to succeed, got error: %v", err)
	}

	if bus == nil {
		t.Fatal("expected non-nil bus")
	}
}

func TestNewWithConfig_InvalidMaxCodecDepth(t *testing.T) {
	config := DefaultBusConfig()
	config.MaxCodecDepth = 0 // Invalid

	_, err := NewWithConfig(config)
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

	_, err := NewWithConfig(config)
	if err == nil {
		t.Fatal("expected error for invalid MTU")
	}
}

func TestNewWithConfig_InvalidTimeout(t *testing.T) {
	config := DefaultBusConfig()
	config.FragmentTimeout = 0 // Invalid

	_, err := NewWithConfig(config)
	if err == nil {
		t.Fatal("expected error for invalid FragmentTimeout")
	}
}

// === SetKey Tests ===

func TestSetKey_ValidKey(t *testing.T) {
	bus, err := New()
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
	bus, err := New()
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
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = bus.SetKey([]byte{})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestSetKeyProvider(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// SetKeyProvider doesn't return error (direct assignment)
	bus.SetKeyProvider(nil)

	// This is valid - can set nil provider
	if bus.keyProvider != nil {
		t.Error("expected nil keyProvider")
	}
}

// === Codec Tests ===

func TestAddCodec(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Add plain codec (no key required)
	plainCodec := plain.New()
	err = bus.AddCodec(plainCodec, "P")
	if err != nil {
		t.Fatalf("expected AddCodec() to succeed, got error: %v", err)
	}

	// Verify codec was added
	supportedCodes := bus.codecManager.GetSupportedCodes()
	if len(supportedCodes) != 1 {
		t.Errorf("expected 1 codec, got %d", len(supportedCodes))
	}

	if supportedCodes[0] != "P" {
		t.Errorf("expected code 'P', got '%s'", supportedCodes[0])
	}
}

func TestAddCodec_DuplicateCode(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	plainCodec1 := plain.New()
	err = bus.AddCodec(plainCodec1, "P")
	if err != nil {
		t.Fatalf("first AddCodec() failed: %v", err)
	}

	// Add duplicate code
	plainCodec2 := plain.New()
	err = bus.AddCodec(plainCodec2, "P")
	if err == nil {
		t.Fatal("expected error for duplicate codec code")
	}
}

func TestSetMaxCodecDepth(t *testing.T) {
	bus, err := New()
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
	bus, err := New()
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

func TestValidate_NoCodec(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = bus.Validate()
	if err == nil {
		t.Fatal("expected error for no codec")
	}

	if !errors.Is(err, ErrCodecChainRequired) {
		t.Errorf("expected ErrCodecChainRequired, got %v", err)
	}
}

func TestValidate_NoChannel(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Add codec but no channel
	plainCodec := plain.New()
	bus.AddCodec(plainCodec, "P")

	err = bus.Validate()
	if err == nil {
		t.Fatal("expected error for no channel")
	}

	if !errors.Is(err, ErrChannelRequired) {
		t.Errorf("expected ErrChannelRequired, got %v", err)
	}
}

func TestValidate_Valid(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Add codec
	plainCodec := plain.New()
	bus.AddCodec(plainCodec, "P")

	// Add mock channel (we need a real channel implementation)
	// For now, skip this test as it requires actual channel

	// This test would pass if we had a channel
	// err = bus.Validate()
	// if err != nil {
	//     t.Fatalf("expected Validate() to succeed: %v", err)
	// }
}

// === Stats Tests ===

func TestStats(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	stats := bus.Stats()

	if stats.Connected {
		t.Error("expected Connected=false for new bus")
	}

	if stats.Negotiated {
		t.Error("expected Negotiated=false for new bus")
	}

	if stats.Running {
		t.Error("expected Running=false for new bus")
	}

	if stats.ChannelCount != 0 {
		t.Errorf("expected ChannelCount=0, got %d", stats.ChannelCount)
	}

	if stats.CodecCount != 0 {
		t.Errorf("expected CodecCount=0, got %d", stats.CodecCount)
	}
}

func TestModuleStats(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	moduleStats := bus.ModuleStats()

	// ModuleStats returns BusStats
	stats, ok := moduleStats.(BusStats)
	if !ok {
		t.Errorf("expected BusStats type, got %T", moduleStats)
	}

	if stats.Connected {
		t.Error("expected Connected=false")
	}
}

// === Lifecycle Tests ===

func TestBus_Name(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if bus.Name() != "Bus" {
		t.Errorf("expected Name='Bus', got '%s'", bus.Name())
	}
}

func TestBus_IsRunning(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if bus.IsRunning() {
		t.Error("expected IsRunning=false for new bus")
	}
}

func TestBus_IsConnected(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if bus.IsConnected() {
		t.Error("expected IsConnected=false for new bus")
	}
}

func TestBus_GetConfig(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	config := bus.GetConfig()
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	if config.MaxCodecDepth != 2 {
		t.Errorf("expected MaxCodecDepth=2, got %d", config.MaxCodecDepth)
	}
}

// === GetNegotiationInfo Tests ===

func TestBus_GetNegotiationInfo(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Add codec
	plainCodec := plain.New()
	bus.AddCodec(plainCodec, "P")

	codes, depth := bus.GetNegotiationInfo()

	if len(codes) != 1 {
		t.Errorf("expected 1 code, got %d", len(codes))
	}

	if depth != 2 {
		t.Errorf("expected depth=2, got %d", depth)
	}
}

// === Connect Tests ===

func TestBus_Connect(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = bus.Connect("localhost:8080")
	if err != nil {
		t.Fatalf("expected Connect() to succeed, got error: %v", err)
	}

	if !bus.IsConnected() {
		t.Error("expected IsConnected=true after Connect()")
	}
}

func TestBus_Connect_AlreadyConnected(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// First connect
	bus.Connect("localhost:8080")

	// Second connect should fail
	err = bus.Connect("localhost:8080")
	if err == nil {
		t.Fatal("expected error for already connected")
	}

	if !errors.Is(err, ErrBusAlreadyRunning) {
		t.Errorf("expected ErrBusAlreadyRunning, got %v", err)
	}
}

// === Error Handler Tests ===

func TestBus_OnMessage(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	handlerCalled := false
	bus.OnMessage(func(data []byte) {
		handlerCalled = true
	})

	if bus.messageHandler == nil {
		t.Error("expected messageHandler to be set")
	}

	// Call handler
	bus.messageHandler([]byte("test"))
	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestBus_OnError(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	handlerCalled := false
	bus.OnError(func(err error) {
		handlerCalled = true
	})

	if bus.errorHandler == nil {
		t.Error("expected errorHandler to be set")
	}

	// Call handler
	bus.errorHandler(ErrChannelClosed)
	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

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

func TestModuleRegistry_Register(t *testing.T) {
	registry := NewModuleRegistry()

	// Create a simple module
	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = registry.Register(bus)
	if err != nil {
		t.Fatalf("expected Register() to succeed: %v", err)
	}

	// Duplicate registration
	err = registry.Register(bus)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestModuleRegistry_Register_Nil(t *testing.T) {
	registry := NewModuleRegistry()

	err := registry.Register(nil)
	if err == nil {
		t.Error("expected error for nil module")
	}
}

func TestModuleRegistry_Get(t *testing.T) {
	registry := NewModuleRegistry()

	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	registry.Register(bus)

	module, err := registry.Get("Bus")
	if err != nil {
		t.Fatalf("expected Get() to succeed: %v", err)
	}

	if module == nil {
		t.Error("expected non-nil module")
	}

	// Get non-existent
	module, err = registry.Get("NonExistent")
	if err == nil {
		t.Error("expected error for non-existent module")
	}
}

func TestModuleRegistry_List(t *testing.T) {
	registry := NewModuleRegistry()

	bus, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	registry.Register(bus)

	names := registry.List()
	if len(names) != 1 {
		t.Errorf("expected 1 module, got %d", len(names))
	}

	if names[0] != "Bus" {
		t.Errorf("expected name 'Bus', got '%s'", names[0])
	}
}

// === Benchmark Tests ===

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		New()
	}
}

func BenchmarkNewWithConfig(b *testing.B) {
	config := DefaultBusConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewWithConfig(config)
	}
}

func BenchmarkSetKey(b *testing.B) {
	bus, _ := New()
	key := []byte("32-byte-secret-key-for-aes-256!!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.SetKey(key)
	}
}

func BenchmarkAddCodec(b *testing.B) {
	bus, _ := New()
	plainCodec := plain.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.AddCodec(plainCodec, "P")
	}
}

func BenchmarkValidate(b *testing.B) {
	bus, _ := New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Validate()
	}
}

func BenchmarkStats(b *testing.B) {
	bus, _ := New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Stats()
	}
}
