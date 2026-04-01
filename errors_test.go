package voidbus

import (
	"errors"
	"testing"
)

// === ErrorSeverity Tests ===

func TestErrorSeverity_String(t *testing.T) {
	tests := []struct {
		severity ErrorSeverity
		expected string
	}{
		{SeverityLow, "LOW"},
		{SeverityMedium, "MEDIUM"},
		{SeverityHigh, "HIGH"},
		{SeverityCritical, "CRITICAL"},
		{ErrorSeverity(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.severity.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.severity.String())
			}
		})
	}
}

// === VoidBusError Tests ===

func TestVoidBusError_Error(t *testing.T) {
	// Test with module
	err := &VoidBusError{
		Op:     "Send",
		Module: "channel",
		Err:    ErrChannelClosed,
		Msg:    "connection lost",
	}

	expected := "[channel] Send: connection lost: channel closed"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}

	// Test without module
	err2 := &VoidBusError{
		Op:  "Connect",
		Err: ErrChannelTimeout,
		Msg: "timeout",
	}

	expected2 := "Connect: timeout: channel timeout"
	if err2.Error() != expected2 {
		t.Errorf("expected '%s', got '%s'", expected2, err2.Error())
	}
}

func TestVoidBusError_Unwrap(t *testing.T) {
	underlying := ErrChannelClosed
	err := &VoidBusError{
		Op:  "Send",
		Err: underlying,
		Msg: "failed",
	}

	unwrapped := err.Unwrap()
	if unwrapped != underlying {
		t.Errorf("expected underlying error, got %v", unwrapped)
	}
}

// === EnhancedVoidBusError Tests ===

func TestEnhancedVoidBusError_Error(t *testing.T) {
	// Test with context
	err := &EnhancedVoidBusError{
		VoidBusError: &VoidBusError{
			Op:     "Send",
			Module: "channel",
			Err:    ErrChannelClosed,
			Msg:    "failed",
		},
		Severity:    SeverityHigh,
		Recoverable: false,
		Context:     map[string]interface{}{"channelID": "tcp-1"},
	}

	expectedPrefix := "[HIGH]"
	if err.Error()[:6] != expectedPrefix {
		t.Errorf("expected prefix '%s', got '%s'", expectedPrefix, err.Error()[:6])
	}

	// Test without context
	err2 := &EnhancedVoidBusError{
		VoidBusError: &VoidBusError{
			Op:     "Send",
			Module: "channel",
			Err:    ErrChannelClosed,
			Msg:    "failed",
		},
		Severity:    SeverityLow,
		Recoverable: true,
	}

	if err2.Error()[:5] != "[LOW]" {
		t.Errorf("expected prefix '[LOW]', got '%s'", err2.Error()[:5])
	}
}

func TestEnhancedVoidBusError_IsRecoverable(t *testing.T) {
	err := &EnhancedVoidBusError{
		VoidBusError: &VoidBusError{},
		Recoverable:  true,
	}

	if !err.IsRecoverable() {
		t.Error("expected IsRecoverable() to return true")
	}

	err2 := &EnhancedVoidBusError{
		VoidBusError: &VoidBusError{},
		Recoverable:  false,
	}

	if err2.IsRecoverable() {
		t.Error("expected IsRecoverable() to return false")
	}
}

func TestEnhancedVoidBusError_GetSeverity(t *testing.T) {
	err := &EnhancedVoidBusError{
		VoidBusError: &VoidBusError{},
		Severity:     SeverityCritical,
	}

	if err.GetSeverity() != SeverityCritical {
		t.Errorf("expected SeverityCritical, got %v", err.GetSeverity())
	}
}

// === Error Wrapping Functions Tests ===

func TestNewError(t *testing.T) {
	err := NewError("Send", "channel", "failed", ErrChannelClosed)

	if err.Op != "Send" {
		t.Errorf("expected Op 'Send', got '%s'", err.Op)
	}

	if err.Module != "channel" {
		t.Errorf("expected Module 'channel', got '%s'", err.Module)
	}
}

func TestWrapError(t *testing.T) {
	// Test wrapping
	err := WrapError("Send", ErrChannelClosed)

	var voidBusErr *VoidBusError
	if !errors.As(err, &voidBusErr) {
		t.Error("expected VoidBusError type")
	}

	if voidBusErr.Op != "Send" {
		t.Errorf("expected Op 'Send', got '%s'", voidBusErr.Op)
	}

	// Test nil input
	nilErr := WrapError("Send", nil)
	if nilErr != nil {
		t.Error("expected nil for nil input")
	}
}

func TestWrapModuleError(t *testing.T) {
	err := WrapModuleError("Send", "channel", ErrChannelClosed)

	var voidBusErr *VoidBusError
	if !errors.As(err, &voidBusErr) {
		t.Error("expected VoidBusError type")
	}

	if voidBusErr.Module != "channel" {
		t.Errorf("expected Module 'channel', got '%s'", voidBusErr.Module)
	}

	// Test nil input
	nilErr := WrapModuleError("Send", "channel", nil)
	if nilErr != nil {
		t.Error("expected nil for nil input")
	}
}

func TestMustWrap(t *testing.T) {
	err := MustWrap("Send", "channel", ErrChannelClosed)

	var enhancedErr *EnhancedVoidBusError
	if !errors.As(err, &enhancedErr) {
		t.Error("expected EnhancedVoidBusError type")
	}

	if enhancedErr.Severity != SeverityHigh {
		t.Errorf("expected SeverityHigh, got %v", enhancedErr.Severity)
	}

	if enhancedErr.Recoverable {
		t.Error("expected Recoverable=false")
	}
}

func TestSoftWrap(t *testing.T) {
	err := SoftWrap("Send", "channel", ErrChannelClosed)

	var enhancedErr *EnhancedVoidBusError
	if !errors.As(err, &enhancedErr) {
		t.Error("expected EnhancedVoidBusError type")
	}

	if enhancedErr.Severity != SeverityLow {
		t.Errorf("expected SeverityLow, got %v", enhancedErr.Severity)
	}

	if !enhancedErr.Recoverable {
		t.Error("expected Recoverable=true")
	}
}

func TestRecoverableError(t *testing.T) {
	err := RecoverableError("Send", "channel", "test message", ErrChannelClosed)

	if err.Severity != SeverityMedium {
		t.Errorf("expected SeverityMedium, got %v", err.Severity)
	}

	if !err.Recoverable {
		t.Error("expected Recoverable=true")
	}

	if err.Msg != "test message" {
		t.Errorf("expected Msg 'test message', got '%s'", err.Msg)
	}
}

func TestCriticalError(t *testing.T) {
	err := CriticalError("Send", "channel", "critical failure", ErrChannelClosed)

	if err.Severity != SeverityCritical {
		t.Errorf("expected SeverityCritical, got %v", err.Severity)
	}

	if err.Recoverable {
		t.Error("expected Recoverable=false")
	}
}

func TestWrapWithContext(t *testing.T) {
	context := map[string]interface{}{
		"channelID": "tcp-1",
		"retry":     3,
	}

	err := WrapWithContext("Send", "channel", ErrChannelClosed, context)

	var enhancedErr *EnhancedVoidBusError
	if !errors.As(err, &enhancedErr) {
		t.Error("expected EnhancedVoidBusError type")
	}

	if enhancedErr.Context["channelID"] != "tcp-1" {
		t.Errorf("expected context channelID 'tcp-1', got '%v'", enhancedErr.Context["channelID"])
	}
}

// === Error Helper Functions Tests ===

func TestIsVoidBusError(t *testing.T) {
	err := WrapError("Send", ErrChannelClosed)

	if !IsVoidBusError(err) {
		t.Error("expected IsVoidBusError=true")
	}

	regularErr := errors.New("regular error")
	if IsVoidBusError(regularErr) {
		t.Error("expected IsVoidBusError=false for regular error")
	}
}

func TestGetModule(t *testing.T) {
	err := WrapModuleError("Send", "channel", ErrChannelClosed)

	if GetModule(err) != "channel" {
		t.Errorf("expected Module 'channel', got '%s'", GetModule(err))
	}

	regularErr := errors.New("regular error")
	if GetModule(regularErr) != "" {
		t.Errorf("expected empty Module for regular error, got '%s'", GetModule(regularErr))
	}
}

func TestGetOperation(t *testing.T) {
	err := WrapError("Send", ErrChannelClosed)

	if GetOperation(err) != "Send" {
		t.Errorf("expected Op 'Send', got '%s'", GetOperation(err))
	}
}

func TestIsEnhancedError(t *testing.T) {
	err := MustWrap("Send", "channel", ErrChannelClosed)

	if !IsEnhancedError(err) {
		t.Error("expected IsEnhancedError=true")
	}

	regularErr := errors.New("regular error")
	if IsEnhancedError(regularErr) {
		t.Error("expected IsEnhancedError=false for regular error")
	}
}

func TestGetSeverity(t *testing.T) {
	err := CriticalError("Send", "channel", "test", ErrChannelClosed)

	if GetSeverity(err) != SeverityCritical {
		t.Errorf("expected SeverityCritical, got %v", GetSeverity(err))
	}

	regularErr := errors.New("regular error")
	if GetSeverity(regularErr) != SeverityLow {
		t.Errorf("expected SeverityLow for regular error, got %v", GetSeverity(regularErr))
	}
}

func TestIsRecoverableFunc(t *testing.T) {
	err := RecoverableError("Send", "channel", "test", ErrChannelClosed)

	if !IsRecoverable(err) {
		t.Error("expected IsRecoverable=true")
	}

	err2 := CriticalError("Send", "channel", "test", ErrChannelClosed)

	if IsRecoverable(err2) {
		t.Error("expected IsRecoverable=false for critical error")
	}
}

func TestGetContextFunc(t *testing.T) {
	context := map[string]interface{}{"key": "value"}
	err := WrapWithContext("Send", "channel", ErrChannelClosed, context)

	ctx := GetContext(err)
	if ctx == nil {
		t.Error("expected non-nil context")
	}

	if ctx["key"] != "value" {
		t.Errorf("expected context value 'value', got '%v'", ctx["key"])
	}
}

func TestIsCritical(t *testing.T) {
	err := CriticalError("Send", "channel", "test", ErrChannelClosed)

	if !IsCritical(err) {
		t.Error("expected IsCritical=true")
	}

	err2 := SoftWrap("Send", "channel", ErrChannelClosed)

	if IsCritical(err2) {
		t.Error("expected IsCritical=false for soft wrap")
	}
}

func TestIsHighSeverity(t *testing.T) {
	// Critical severity
	err1 := CriticalError("Send", "channel", "test", ErrChannelClosed)
	if !IsHighSeverity(err1) {
		t.Error("expected IsHighSeverity=true for Critical")
	}

	// High severity
	err2 := MustWrap("Send", "channel", ErrChannelClosed)
	if !IsHighSeverity(err2) {
		t.Error("expected IsHighSeverity=true for High")
	}

	// Medium severity
	err3 := RecoverableError("Send", "channel", "test", ErrChannelClosed)
	if IsHighSeverity(err3) {
		t.Error("expected IsHighSeverity=false for Medium")
	}

	// Low severity
	err4 := SoftWrap("Send", "channel", ErrChannelClosed)
	if IsHighSeverity(err4) {
		t.Error("expected IsHighSeverity=false for Low")
	}
}

// === Error Chaining Tests ===

func TestErrorChaining(t *testing.T) {
	// Create nested error chain
	baseErr := errors.New("base error")
	voidBusErr := WrapModuleError("Operation1", "module1", baseErr)
	enhancedErr := MustWrap("Operation2", "module2", voidBusErr)

	// Verify chain - extract VoidBusError
	var extractedVoidBus *VoidBusError
	if !errors.As(enhancedErr, &extractedVoidBus) {
		t.Error("expected to extract VoidBusError from chain")
	}

	// Verify we can extract EnhancedVoidBusError
	var extractedEnhanced *EnhancedVoidBusError
	if !errors.As(enhancedErr, &extractedEnhanced) {
		t.Error("expected to extract EnhancedVoidBusError from chain")
	}

	// Verify enhanced error properties
	if extractedEnhanced.Severity != SeverityHigh {
		t.Errorf("expected SeverityHigh, got %v", extractedEnhanced.Severity)
	}

	// Verify unwrap chain
	unwrapped := extractedEnhanced.Unwrap()
	if unwrapped == nil {
		t.Error("expected non-nil unwrapped error")
	}
}

// === Benchmark Tests ===

func BenchmarkWrapError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		WrapError("Send", ErrChannelClosed)
	}
}

func BenchmarkWrapModuleError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		WrapModuleError("Send", "channel", ErrChannelClosed)
	}
}

func BenchmarkMustWrap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		MustWrap("Send", "channel", ErrChannelClosed)
	}
}

func BenchmarkIsVoidBusError(b *testing.B) {
	err := WrapError("Send", ErrChannelClosed)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsVoidBusError(err)
	}
}

func BenchmarkGetSeverity(b *testing.B) {
	err := CriticalError("Send", "channel", "test", ErrChannelClosed)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetSeverity(err)
	}
}
