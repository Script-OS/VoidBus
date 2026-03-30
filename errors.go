package voidbus

import (
	"errors"
	"fmt"
)

// VoidBusError represents a general error in VoidBus operations
type VoidBusError struct {
	Op     string // The operation that caused the error
	Module string // The module that caused the error (channel, encoding, fragment, etc.)
	Err    error  // The underlying error
	Msg    string // Additional context message
}

func (e *VoidBusError) Error() string {
	if e.Module != "" {
		return fmt.Sprintf("[%s] %s: %s: %v", e.Module, e.Op, e.Msg, e.Err)
	}
	return fmt.Sprintf("%s: %s: %v", e.Op, e.Msg, e.Err)
}

func (e *VoidBusError) Unwrap() error {
	return e.Err
}

// NewError creates a new VoidBusError
func NewError(op, module, msg string, err error) *VoidBusError {
	return &VoidBusError{
		Op:     op,
		Module: module,
		Err:    err,
		Msg:    msg,
	}
}

// WrapError wraps an error with operation context
func WrapError(op string, err error) error {
	if err == nil {
		return nil
	}
	return &VoidBusError{
		Op:  op,
		Err: err,
	}
}

// WrapModuleError wraps an error with module and operation context
func WrapModuleError(op, module string, err error) error {
	if err == nil {
		return nil
	}
	return &VoidBusError{
		Op:     op,
		Module: module,
		Err:    err,
	}
}

// IsVoidBusError checks if an error is a VoidBusError
func IsVoidBusError(err error) bool {
	var voidBusErr *VoidBusError
	return errors.As(err, &voidBusErr)
}

// GetModule extracts the module name from an error if it's a VoidBusError
func GetModule(err error) string {
	var voidBusErr *VoidBusError
	if errors.As(err, &voidBusErr) {
		return voidBusErr.Module
	}
	return ""
}

// GetOperation extracts the operation name from an error if it's a VoidBusError
func GetOperation(err error) string {
	var voidBusErr *VoidBusError
	if errors.As(err, &voidBusErr) {
		return voidBusErr.Op
	}
	return ""
}

// Common error types
var (
	// Channel errors
	ErrChannelClosed       = errors.New("channel closed")
	ErrChannelNotReady     = errors.New("channel not ready")
	ErrChannelSendFailed   = errors.New("channel send failed")
	ErrChannelRecvFailed   = errors.New("channel receive failed")
	ErrChannelTimeout      = errors.New("channel timeout")
	ErrChannelDisconnected = errors.New("channel disconnected")

	// Encoding errors
	ErrEncodingFailed  = errors.New("encoding failed")
	ErrDecodingFailed  = errors.New("decoding failed")
	ErrKeyRequired     = errors.New("key required")
	ErrInvalidKey      = errors.New("invalid key")
	ErrUnsupportedType = errors.New("unsupported encoding type")

	// Fragment errors
	ErrFragmentFailed    = errors.New("fragmentation failed")
	ErrReassemblyFailed  = errors.New("reassembly failed")
	ErrInvalidFragment   = errors.New("invalid fragment")
	ErrFragmentMissing   = errors.New("fragment missing")
	ErrFragmentTimeout   = errors.New("fragment timeout")
	ErrFragmentCorrupted = errors.New("fragment corrupted")

	// KeyProvider errors
	ErrKeyProviderFailed = errors.New("key provider failed")
	ErrKeyNotFound       = errors.New("key not found")
	ErrKeyExpired        = errors.New("key expired")
	ErrKeyRefreshFailed  = errors.New("key refresh failed")

	// Bus errors
	ErrBusConfig          = errors.New("bus configuration error")
	ErrModuleNotSet       = errors.New("module not set")
	ErrHandlerNotSet      = errors.New("handler not set")
	ErrBusAlreadyRunning  = errors.New("bus already running")
	ErrBusNotRunning      = errors.New("bus not running")
	ErrSerializerRequired = errors.New("serializer required")
	ErrCodecChainRequired = errors.New("codec chain required")
	ErrChannelRequired    = errors.New("channel required")
	ErrSendFailed         = errors.New("send failed")
	ErrReceiveFailed      = errors.New("receive failed")

	// Policy errors
	ErrInvalidPolicy = errors.New("invalid policy configuration")
)
