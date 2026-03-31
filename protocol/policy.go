// Package protocol provides negotiation policy for VoidBus handshake.
//
// NegotiationPolicy controls security requirements and allowed algorithms:
//   - DebugMode: WARNING: Should NEVER be enabled in release builds
//   - MinSecurityLevel: Release MUST use SecurityLevelMedium or higher
//   - AllowedSerializers: Whitelist of allowed serializers
//   - PreferredCodecChainSecurity: Preferred security level for codec chain
//
// Security Design:
//   - Release mode MUST reject plaintext codec
//   - Challenge mechanism prevents degradation attacks
//   - Session timeout prevents stale connections
package protocol

import (
	"errors"
	"time"

	"github.com/Script-OS/VoidBus/codec"
)

// Policy errors
var (
	ErrInvalidPolicy = errors.New("policy: invalid configuration")
)

// NegotiationPolicy defines the security policy for handshake negotiation.
type NegotiationPolicy struct {
	// DebugMode indicates whether debug mode is enabled.
	// In debug mode, plaintext codec is allowed.
	// WARNING: Debug mode should NEVER be enabled in release builds.
	DebugMode bool

	// MinSecurityLevel is the minimum required security level.
	// Release mode MUST use SecurityLevelMedium or higher.
	MinSecurityLevel codec.SecurityLevel

	// AllowedSerializers is the whitelist of allowed serializers.
	// Empty list means all registered serializers are allowed.
	AllowedSerializers []string

	// PreferredSerializer is the preferred serializer name.
	// Used when both parties support multiple serializers.
	PreferredSerializer string

	// PreferredCodecChainSecurity is the preferred codec chain security level.
	// Used when both parties support multiple chains.
	PreferredCodecChainSecurity codec.SecurityLevel

	// MaxCodecChainLength is maximum allowed codec chain length.
	// Prevents excessively long chains that could degrade performance.
	MaxCodecChainLength int

	// ChallengeTimeout is timeout for challenge verification.
	ChallengeTimeout time.Duration

	// HandshakeTimeout is overall handshake timeout.
	HandshakeTimeout time.Duration

	// RejectOnMismatch controls behavior when security level doesn't match.
	// true: Reject connection if security level doesn't match
	// false: Select highest matching security level
	RejectOnMismatch bool

	// AllowPlaintextInDebug allows plaintext codec in debug mode.
	// This should only be true for development/testing.
	AllowPlaintextInDebug bool

	// SessionTimeout is session inactivity timeout.
	SessionTimeout time.Duration

	// MaxSessions is maximum number of concurrent sessions (0 = unlimited).
	MaxSessions int
}

// DefaultNegotiationPolicy returns the default negotiation policy for release builds.
// This policy enforces minimum security requirements.
func DefaultNegotiationPolicy() NegotiationPolicy {
	return NegotiationPolicy{
		DebugMode:                   false,
		MinSecurityLevel:            codec.SecurityLevelMedium,
		AllowedSerializers:          []string{}, // Allow all
		PreferredSerializer:         "",
		PreferredCodecChainSecurity: codec.SecurityLevelHigh,
		MaxCodecChainLength:         5,
		ChallengeTimeout:            30 * time.Second,
		HandshakeTimeout:            60 * time.Second,
		RejectOnMismatch:            true,
		AllowPlaintextInDebug:       false,
		SessionTimeout:              30 * time.Minute,
		MaxSessions:                 1000,
	}
}

// DebugNegotiationPolicy returns a negotiation policy for debug/development mode.
// WARNING: This policy allows insecure configurations and should NOT be used in production.
func DebugNegotiationPolicy() NegotiationPolicy {
	return NegotiationPolicy{
		DebugMode:                   true,
		MinSecurityLevel:            codec.SecurityLevelNone,
		AllowedSerializers:          []string{"plain", "json"},
		PreferredSerializer:         "plain",
		PreferredCodecChainSecurity: codec.SecurityLevelNone,
		MaxCodecChainLength:         3,
		ChallengeTimeout:            60 * time.Second,
		HandshakeTimeout:            120 * time.Second,
		RejectOnMismatch:            false,
		AllowPlaintextInDebug:       true,
		SessionTimeout:              2 * time.Hour,
		MaxSessions:                 100,
	}
}

// HighSecurityNegotiationPolicy returns a high-security policy for sensitive applications.
func HighSecurityNegotiationPolicy() NegotiationPolicy {
	return NegotiationPolicy{
		DebugMode:                   false,
		MinSecurityLevel:            codec.SecurityLevelHigh,
		AllowedSerializers:          []string{"protobuf", "json"},
		PreferredSerializer:         "protobuf",
		PreferredCodecChainSecurity: codec.SecurityLevelHigh,
		MaxCodecChainLength:         3,
		ChallengeTimeout:            15 * time.Second,
		HandshakeTimeout:            30 * time.Second,
		RejectOnMismatch:            true,
		AllowPlaintextInDebug:       false,
		SessionTimeout:              10 * time.Minute,
		MaxSessions:                 100,
	}
}

// Validate validates and normalizes the policy configuration.
// Uses pointer receiver to allow modification of fields with invalid values.
func (p *NegotiationPolicy) Validate() error {
	if p == nil {
		return ErrInvalidPolicy
	}

	// Security level must be valid
	if p.MinSecurityLevel < codec.SecurityLevelNone || p.MinSecurityLevel > codec.SecurityLevelHigh {
		return ErrInvalidPolicy
	}

	// Chain length must be positive
	if p.MaxCodecChainLength < 1 {
		p.MaxCodecChainLength = 5 // Default
	}

	// Timeouts must be positive
	if p.ChallengeTimeout <= 0 {
		p.ChallengeTimeout = 30 * time.Second
	}
	if p.HandshakeTimeout <= 0 {
		p.HandshakeTimeout = 60 * time.Second
	}
	if p.SessionTimeout <= 0 {
		p.SessionTimeout = 30 * time.Minute
	}

	return nil
}

// IsCodecAllowed checks if a codec security level is allowed by the policy.
func (p NegotiationPolicy) IsCodecAllowed(level codec.SecurityLevel) bool {
	// Debug mode allows all
	if p.DebugMode && p.AllowPlaintextInDebug {
		return true
	}

	// Must meet minimum
	if level < p.MinSecurityLevel {
		return false
	}

	// Reject plaintext in non-debug mode
	if level == codec.SecurityLevelNone && !p.DebugMode {
		return false
	}

	return true
}

// IsSerializerAllowed checks if a serializer is allowed by the policy.
func (p NegotiationPolicy) IsSerializerAllowed(name string) bool {
	// Empty whitelist means all allowed
	if len(p.AllowedSerializers) == 0 {
		return true
	}

	for _, allowed := range p.AllowedSerializers {
		if allowed == name {
			return true
		}
	}
	return false
}

// SelectBestSecurityLevel selects the best security level from client options.
func (p NegotiationPolicy) SelectBestSecurityLevel(clientLevels []codec.SecurityLevel) (codec.SecurityLevel, bool) {
	var best codec.SecurityLevel
	found := false

	for _, level := range clientLevels {
		// Must be allowed
		if !p.IsCodecAllowed(level) {
			continue
		}

		// Must meet minimum
		if level < p.MinSecurityLevel {
			continue
		}

		// Prefer higher
		if !found || level > best {
			best = level
			found = true
		}
	}

	// Prefer preferred level if available
	if p.PreferredCodecChainSecurity >= p.MinSecurityLevel {
		for _, level := range clientLevels {
			if level == p.PreferredCodecChainSecurity {
				return level, true
			}
		}
	}

	return best, found
}

// NegotiationPolicyBuilder provides a fluent API for building policies.
type NegotiationPolicyBuilder struct {
	policy NegotiationPolicy
}

// NewPolicyBuilder creates a new policy builder.
func NewPolicyBuilder() *NegotiationPolicyBuilder {
	return &NegotiationPolicyBuilder{
		policy: DefaultNegotiationPolicy(),
	}
}

// WithDebugMode sets debug mode.
func (b *NegotiationPolicyBuilder) WithDebugMode(debug bool) *NegotiationPolicyBuilder {
	b.policy.DebugMode = debug
	return b
}

// WithMinSecurityLevel sets minimum security level.
func (b *NegotiationPolicyBuilder) WithMinSecurityLevel(level codec.SecurityLevel) *NegotiationPolicyBuilder {
	b.policy.MinSecurityLevel = level
	return b
}

// WithAllowedSerializers sets allowed serializers.
func (b *NegotiationPolicyBuilder) WithAllowedSerializers(serializers ...string) *NegotiationPolicyBuilder {
	b.policy.AllowedSerializers = serializers
	return b
}

// WithPreferredSerializer sets preferred serializer.
func (b *NegotiationPolicyBuilder) WithPreferredSerializer(name string) *NegotiationPolicyBuilder {
	b.policy.PreferredSerializer = name
	return b
}

// WithPreferredCodecChainSecurity sets preferred codec chain security.
func (b *NegotiationPolicyBuilder) WithPreferredCodecChainSecurity(level codec.SecurityLevel) *NegotiationPolicyBuilder {
	b.policy.PreferredCodecChainSecurity = level
	return b
}

// WithMaxCodecChainLength sets max chain length.
func (b *NegotiationPolicyBuilder) WithMaxCodecChainLength(max int) *NegotiationPolicyBuilder {
	b.policy.MaxCodecChainLength = max
	return b
}

// WithChallengeTimeout sets challenge timeout.
func (b *NegotiationPolicyBuilder) WithChallengeTimeout(timeout time.Duration) *NegotiationPolicyBuilder {
	b.policy.ChallengeTimeout = timeout
	return b
}

// WithHandshakeTimeout sets handshake timeout.
func (b *NegotiationPolicyBuilder) WithHandshakeTimeout(timeout time.Duration) *NegotiationPolicyBuilder {
	b.policy.HandshakeTimeout = timeout
	return b
}

// WithRejectOnMismatch sets reject on mismatch behavior.
func (b *NegotiationPolicyBuilder) WithRejectOnMismatch(reject bool) *NegotiationPolicyBuilder {
	b.policy.RejectOnMismatch = reject
	return b
}

// WithSessionTimeout sets session timeout.
func (b *NegotiationPolicyBuilder) WithSessionTimeout(timeout time.Duration) *NegotiationPolicyBuilder {
	b.policy.SessionTimeout = timeout
	return b
}

// WithMaxSessions sets max sessions.
func (b *NegotiationPolicyBuilder) WithMaxSessions(max int) *NegotiationPolicyBuilder {
	b.policy.MaxSessions = max
	return b
}

// Build builds the policy.
func (b *NegotiationPolicyBuilder) Build() NegotiationPolicy {
	return b.policy
}
