package voidbus

import (
	"errors"
	"fmt"
)

// ErrorSeverity represents the severity level of an error.
type ErrorSeverity int

const (
	// SeverityLow indicates a recoverable error that can be safely ignored.
	SeverityLow ErrorSeverity = iota

	// SeverityMedium indicates an error that requires handling but doesn't break functionality.
	SeverityMedium

	// SeverityHigh indicates a serious error that affects core functionality.
	SeverityHigh

	// SeverityCritical indicates a fatal error that prevents the system from operating.
	SeverityCritical
)

func (s ErrorSeverity) String() string {
	switch s {
	case SeverityLow:
		return "LOW"
	case SeverityMedium:
		return "MEDIUM"
	case SeverityHigh:
		return "HIGH"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

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

// EnhancedVoidBusError represents an enhanced error with severity and context.
// Used for critical operations where error severity matters.
type EnhancedVoidBusError struct {
	*VoidBusError
	Severity    ErrorSeverity
	Recoverable bool
	Context     map[string]interface{}
}

func (e *EnhancedVoidBusError) Error() string {
	base := e.VoidBusError.Error()
	if len(e.Context) > 0 {
		return fmt.Sprintf("[%s] %s (context: %v)", e.Severity.String(), base, e.Context)
	}
	return fmt.Sprintf("[%s] %s", e.Severity.String(), base)
}

func (e *EnhancedVoidBusError) Unwrap() error {
	return e.Err
}

// IsRecoverable returns whether the error is recoverable.
func (e *EnhancedVoidBusError) IsRecoverable() bool {
	return e.Recoverable
}

// GetSeverity returns the error severity level.
func (e *EnhancedVoidBusError) GetSeverity() ErrorSeverity {
	return e.Severity
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

// === Enhanced Error Wrapping Functions ===

// MustWrap wraps a critical error with module and operation context.
// Used for operations where failure is severe (returns EnhancedVoidBusError with SeverityHigh).
func MustWrap(op, module string, err error) error {
	if err == nil {
		return nil
	}
	return &EnhancedVoidBusError{
		VoidBusError: &VoidBusError{
			Op:     op,
			Module: module,
			Err:    err,
			Msg:    "critical operation failed",
		},
		Severity:    SeverityHigh,
		Recoverable: false,
	}
}

// SoftWrap wraps an error for optional paths.
// Used for operations where failure is acceptable (returns EnhancedVoidBusError with SeverityLow).
func SoftWrap(op, module string, err error) error {
	if err == nil {
		return nil
	}
	return &EnhancedVoidBusError{
		VoidBusError: &VoidBusError{
			Op:     op,
			Module: module,
			Err:    err,
			Msg:    "optional operation failed",
		},
		Severity:    SeverityLow,
		Recoverable: true,
	}
}

// RecoverableError creates a recoverable enhanced error.
func RecoverableError(op, module, msg string, err error) *EnhancedVoidBusError {
	return &EnhancedVoidBusError{
		VoidBusError: &VoidBusError{
			Op:     op,
			Module: module,
			Err:    err,
			Msg:    msg,
		},
		Severity:    SeverityMedium,
		Recoverable: true,
	}
}

// CriticalError creates a critical enhanced error.
func CriticalError(op, module, msg string, err error) *EnhancedVoidBusError {
	return &EnhancedVoidBusError{
		VoidBusError: &VoidBusError{
			Op:     op,
			Module: module,
			Err:    err,
			Msg:    msg,
		},
		Severity:    SeverityCritical,
		Recoverable: false,
	}
}

// WrapWithContext wraps an error with additional context.
func WrapWithContext(op, module string, err error, context map[string]interface{}) error {
	if err == nil {
		return nil
	}
	return &EnhancedVoidBusError{
		VoidBusError: &VoidBusError{
			Op:     op,
			Module: module,
			Err:    err,
			Msg:    "operation failed",
		},
		Severity:    SeverityMedium,
		Recoverable: false,
		Context:     context,
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

// === Enhanced Error Helper Functions ===

// IsEnhancedError checks if an error is an EnhancedVoidBusError
func IsEnhancedError(err error) bool {
	var enhancedErr *EnhancedVoidBusError
	return errors.As(err, &enhancedErr)
}

// GetSeverity extracts the severity from an error if it's an EnhancedVoidBusError
func GetSeverity(err error) ErrorSeverity {
	var enhancedErr *EnhancedVoidBusError
	if errors.As(err, &enhancedErr) {
		return enhancedErr.Severity
	}
	return SeverityLow
}

// IsRecoverable checks if an error is recoverable
func IsRecoverable(err error) bool {
	var enhancedErr *EnhancedVoidBusError
	if errors.As(err, &enhancedErr) {
		return enhancedErr.Recoverable
	}
	return false // Default to non-recoverable
}

// GetContext extracts the context map from an error
func GetContext(err error) map[string]interface{} {
	var enhancedErr *EnhancedVoidBusError
	if errors.As(err, &enhancedErr) {
		return enhancedErr.Context
	}
	return nil
}

// IsCritical checks if an error is critical severity
func IsCritical(err error) bool {
	return GetSeverity(err) == SeverityCritical
}

// IsHighSeverity checks if an error is high or critical severity
func IsHighSeverity(err error) bool {
	s := GetSeverity(err)
	return s == SeverityHigh || s == SeverityCritical
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

	// === v2.0 新增错误类型 ===

	// Codec 管理错误
	ErrCodecNotFound      = errors.New("codec not found")
	ErrCodecChainMismatch = errors.New("codec chain hash mismatch")
	ErrCodecDepthExceeded = errors.New("codec depth exceeded")
	ErrCodecCodeExists    = errors.New("codec code already exists")

	// Channel Pool 错误
	ErrNoHealthyChannel = errors.New("no healthy channel available")
	ErrChannelPoolEmpty = errors.New("channel pool empty")
	ErrChannelNotFound  = errors.New("channel not found in pool")

	// Fragment v2.0 错误
	ErrFragmentIncomplete  = errors.New("fragment incomplete")
	ErrFragmentMismatch    = errors.New("fragment metadata mismatch")
	ErrInvalidMetadata     = errors.New("invalid fragment metadata")
	ErrAdaptiveSplitFailed = errors.New("adaptive split failed")

	// Session 错误
	ErrSessionNotFound        = errors.New("session not found")
	ErrSessionExpired         = errors.New("session expired")
	ErrSessionAlreadyComplete = errors.New("session already complete")
	ErrRetransmitExceeded     = errors.New("retransmit count exceeded")

	// 协商错误
	ErrNegotiationFailed  = errors.New("negotiation failed")
	ErrNoCommonCodec      = errors.New("no common codec codes")
	ErrNegotiationTimeout = errors.New("negotiation timeout")
)
