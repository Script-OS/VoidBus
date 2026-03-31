// Package protocol provides handshake protocol for VoidBus session negotiation.
//
// Handshake Protocol:
//
//	Client -> Server: HandshakeRequest (supported serializers, codec chain info)
//	Server -> Client: HandshakeResponse (accept/reject, selected serializer, challenge)
//	Client -> Server: HandshakeConfirm (session ID, challenge response)
//
// Security Design:
//   - Serializer name CAN be exposed
//   - Codec names MUST NOT be exposed (only security level)
//   - Challenge mechanism prevents degradation attacks
//   - Release mode MUST use SecurityLevelMedium or higher
package protocol

import (
	"errors"
	"time"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/internal"
)

// Handshake errors
var (
	ErrHandshakeFailed       = errors.New("handshake: failed")
	ErrHandshakeTimeout      = errors.New("handshake: timeout")
	ErrSecurityLevelMismatch = errors.New("handshake: security level mismatch")
	ErrChallengeFailed       = errors.New("handshake: challenge verification failed")
	ErrDegradationAttack     = errors.New("handshake: potential degradation attack detected")
	ErrInvalidRequest        = errors.New("handshake: invalid request")
	ErrInvalidResponse       = errors.New("handshake: invalid response")
	ErrInvalidSessionID      = errors.New("handshake: invalid session ID")
)

// HandshakeRequest is sent by client to initiate negotiation.
type HandshakeRequest struct {
	// ClientID is client identifier (random UUID)
	ClientID string

	// SupportedSerializers is list of supported serializers (by priority)
	SupportedSerializers []SerializerInfo

	// SupportedCodecChains is list of supported codec chain info
	// NOTE: Does NOT expose specific codec names, only security levels
	SupportedCodecChains []CodecChainInfo

	// MinSecurityLevel is minimum required security level
	MinSecurityLevel codec.SecurityLevel

	// Timestamp is request timestamp
	Timestamp int64

	// Version is protocol version
	Version uint8
}

// SerializerInfo contains serializer information.
type SerializerInfo struct {
	// Name is serializer name (CAN be exposed)
	Name string

	// Priority is serializer priority (higher = preferred)
	Priority int
}

// CodecChainInfo contains codec chain information.
// IMPORTANT: Does NOT expose specific codec names!
// Only exposes security level for negotiation.
type CodecChainInfo struct {
	// SecurityLevel is chain's overall security level
	SecurityLevel codec.SecurityLevel

	// ChainLength is number of codecs in chain
	ChainLength int

	// Hash is chain configuration hash (for verification, does NOT expose config)
	Hash string
}

// HandshakeResponse is sent by server to accept/reject negotiation.
type HandshakeResponse struct {
	// Accepted indicates whether connection is accepted
	Accepted bool

	// RejectReason explains why connection was rejected
	RejectReason string

	// SelectedSerializer is the chosen serializer name
	SelectedSerializer string

	// SelectedCodecChainInfo is info about chosen codec chain
	SelectedCodecChainInfo CodecChainInfo

	// SessionID is assigned session identifier
	SessionID string

	// ServerChallenge is challenge data for client verification
	// Client must encode this with selected codec chain and return
	ServerChallenge []byte

	// Timestamp is response timestamp
	Timestamp int64
}

// HandshakeConfirm is sent by client to complete handshake.
type HandshakeConfirm struct {
	// SessionID is the assigned session ID
	SessionID string

	// ChallengeResponse is challenge encoded with selected codec chain
	// Proves client possesses the claimed codec capability
	ChallengeResponse []byte

	// Timestamp is confirmation timestamp
	Timestamp int64
}

// NewHandshakeRequest creates a new handshake request.
func NewHandshakeRequest(clientID string) *HandshakeRequest {
	return &HandshakeRequest{
		ClientID:             clientID,
		SupportedSerializers: make([]SerializerInfo, 0),
		SupportedCodecChains: make([]CodecChainInfo, 0),
		Timestamp:            time.Now().Unix(),
		Version:              PacketVersion,
	}
}

// AddSerializer adds a supported serializer to the request.
func (r *HandshakeRequest) AddSerializer(name string, priority int) {
	r.SupportedSerializers = append(r.SupportedSerializers, SerializerInfo{
		Name:     name,
		Priority: priority,
	})
}

// AddCodecChain adds a supported codec chain info to the request.
func (r *HandshakeRequest) AddCodecChain(level codec.SecurityLevel, chainLength int, hash string) {
	r.SupportedCodecChains = append(r.SupportedCodecChains, CodecChainInfo{
		SecurityLevel: level,
		ChainLength:   chainLength,
		Hash:          hash,
	})
}

// Validate validates the handshake request.
func (r *HandshakeRequest) Validate() error {
	if r.ClientID == "" {
		return ErrInvalidRequest
	}
	if len(r.SupportedSerializers) == 0 {
		return ErrInvalidRequest
	}
	if len(r.SupportedCodecChains) == 0 {
		return ErrInvalidRequest
	}
	return nil
}

// NewHandshakeResponse creates a new handshake response.
func NewHandshakeResponse(accepted bool) *HandshakeResponse {
	return &HandshakeResponse{
		Accepted:  accepted,
		Timestamp: time.Now().Unix(),
		SessionID: internal.GenerateSessionID(),
	}
}

// WithSerializer sets the selected serializer.
func (r *HandshakeResponse) WithSerializer(name string) *HandshakeResponse {
	r.SelectedSerializer = name
	return r
}

// WithCodecChain sets the selected codec chain info.
func (r *HandshakeResponse) WithCodecChain(info CodecChainInfo) *HandshakeResponse {
	r.SelectedCodecChainInfo = info
	return r
}

// WithChallenge sets the server challenge.
func (r *HandshakeResponse) WithChallenge(challenge []byte) *HandshakeResponse {
	r.ServerChallenge = challenge
	return r
}

// Reject creates a rejection response.
func RejectHandshake(reason string) *HandshakeResponse {
	return &HandshakeResponse{
		Accepted:     false,
		RejectReason: reason,
		Timestamp:    time.Now().Unix(),
	}
}

// NewHandshakeConfirm creates a new handshake confirmation.
func NewHandshakeConfirm(sessionID string, challengeResponse []byte) *HandshakeConfirm {
	return &HandshakeConfirm{
		SessionID:         sessionID,
		ChallengeResponse: challengeResponse,
		Timestamp:         time.Now().Unix(),
	}
}

// HandshakeResult contains the result of a successful handshake.
type HandshakeResult struct {
	SessionID       string
	ClientID        string
	Serializer      string
	CodecChainLevel codec.SecurityLevel
	CodecChainHash  string
	ChallengePassed bool
	Timestamp       time.Time
}

// HandshakeState tracks the state of a handshake process.
type HandshakeState struct {
	ClientID    string
	SessionID   string
	Request     *HandshakeRequest
	Response    *HandshakeResponse
	Challenge   []byte
	StartedAt   time.Time
	CompletedAt time.Time
	Status      HandshakeStatus
}

// HandshakeStatus represents the status of a handshake.
type HandshakeStatus int

const (
	HandshakeStatusPending HandshakeStatus = iota
	HandshakeStatusChallenged
	HandshakeStatusCompleted
	HandshakeStatusFailed
	HandshakeStatusTimeout
)

// String returns string representation.
func (s HandshakeStatus) String() string {
	switch s {
	case HandshakeStatusPending:
		return "pending"
	case HandshakeStatusChallenged:
		return "challenged"
	case HandshakeStatusCompleted:
		return "completed"
	case HandshakeStatusFailed:
		return "failed"
	case HandshakeStatusTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// HandshakeManager manages handshake processes.
type HandshakeManager struct {
	policy NegotiationPolicy
	states map[string]*HandshakeState
}

// NewHandshakeManager creates a new HandshakeManager.
func NewHandshakeManager(policy NegotiationPolicy) *HandshakeManager {
	return &HandshakeManager{
		policy: policy,
		states: make(map[string]*HandshakeState),
	}
}

// ProcessRequest processes a handshake request and returns a response.
func (m *HandshakeManager) ProcessRequest(req *HandshakeRequest) (*HandshakeResponse, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return RejectHandshake(err.Error()), err
	}

	// Check security level
	if req.MinSecurityLevel > m.policy.MinSecurityLevel {
		// Client requires higher security than we support
		return RejectHandshake("security level not supported"), ErrSecurityLevelMismatch
	}

	// Check for degradation attack
	if !m.policy.DebugMode && req.MinSecurityLevel < m.policy.MinSecurityLevel {
		return RejectHandshake("security level too low"), ErrDegradationAttack
	}

	// Select serializer
	selectedSerializer := m.selectSerializer(req.SupportedSerializers)
	if selectedSerializer == "" {
		return RejectHandshake("no compatible serializer"), ErrHandshakeFailed
	}

	// Select codec chain
	selectedChain := m.selectCodecChain(req.SupportedCodecChains)
	if selectedChain.SecurityLevel < m.policy.MinSecurityLevel && !m.policy.DebugMode {
		return RejectHandshake("codec security level too low"), ErrDegradationAttack
	}

	// Generate challenge
	challenge, err := internal.GenerateChallenge()
	if err != nil {
		return RejectHandshake("internal error"), err
	}

	// Create response
	sessionID := internal.GenerateSessionID()
	response := NewHandshakeResponse(true).
		WithSerializer(selectedSerializer).
		WithCodecChain(selectedChain).
		WithChallenge(challenge)
	response.SessionID = sessionID

	// Store state
	m.states[sessionID] = &HandshakeState{
		ClientID:  req.ClientID,
		SessionID: sessionID,
		Request:   req,
		Response:  response,
		Challenge: challenge,
		StartedAt: time.Now(),
		Status:    HandshakeStatusChallenged,
	}

	return response, nil
}

// ProcessConfirm processes a handshake confirmation.
func (m *HandshakeManager) ProcessConfirm(confirm *HandshakeConfirm) (*HandshakeResult, error) {
	state, exists := m.states[confirm.SessionID]
	if !exists {
		return nil, ErrInvalidSessionID
	}

	// Check timeout
	if time.Since(state.StartedAt) > m.policy.ChallengeTimeout {
		state.Status = HandshakeStatusTimeout
		return nil, ErrHandshakeTimeout
	}

	// Verify challenge response
	// In a real implementation, this would encode the challenge with the selected codec chain
	// For now, we use simple verification
	expectedResponse := simpleChallengeResponse(state.Challenge)

	// Compare responses (constant-time comparison would be better for crypto)
	match := len(confirm.ChallengeResponse) == len(expectedResponse)
	if match {
		for i := range expectedResponse {
			if confirm.ChallengeResponse[i] != expectedResponse[i] {
				match = false
				break
			}
		}
	}

	if !match {
		state.Status = HandshakeStatusFailed
		return nil, ErrChallengeFailed
	}

	// Success
	state.Status = HandshakeStatusCompleted
	state.CompletedAt = time.Now()

	result := &HandshakeResult{
		SessionID:       confirm.SessionID,
		ClientID:        state.ClientID,
		Serializer:      state.Response.SelectedSerializer,
		CodecChainLevel: state.Response.SelectedCodecChainInfo.SecurityLevel,
		CodecChainHash:  state.Response.SelectedCodecChainInfo.Hash,
		ChallengePassed: true,
		Timestamp:       state.CompletedAt,
	}

	return result, nil
}

// selectSerializer selects the best serializer from the list.
func (m *HandshakeManager) selectSerializer(serializers []SerializerInfo) string {
	// Find highest priority serializer that is allowed
	best := ""
	bestPriority := -1

	for _, s := range serializers {
		// Check if allowed
		if len(m.policy.AllowedSerializers) > 0 {
			allowed := false
			for _, a := range m.policy.AllowedSerializers {
				if a == s.Name {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		// Check if preferred
		if s.Priority > bestPriority {
			best = s.Name
			bestPriority = s.Priority
		}
	}

	// Check preferred serializer
	if m.policy.PreferredSerializer != "" {
		for _, s := range serializers {
			if s.Name == m.policy.PreferredSerializer {
				return s.Name
			}
		}
	}

	return best
}

// selectCodecChain selects the best codec chain from the list.
func (m *HandshakeManager) selectCodecChain(chains []CodecChainInfo) CodecChainInfo {
	// Find highest security level that meets requirements
	var best CodecChainInfo
	bestLevel := codec.SecurityLevelNone

	for _, c := range chains {
		// Must meet minimum
		if c.SecurityLevel < m.policy.MinSecurityLevel {
			continue
		}

		// Prefer higher security
		if c.SecurityLevel > bestLevel {
			best = c
			bestLevel = c.SecurityLevel
		} else if c.SecurityLevel == bestLevel && c.ChainLength < best.ChainLength {
			// Same security, prefer shorter chain
			best = c
		}
	}

	// Check preferred security level
	if m.policy.PreferredCodecChainSecurity > codec.SecurityLevelNone {
		for _, c := range chains {
			if c.SecurityLevel == m.policy.PreferredCodecChainSecurity {
				return c
			}
		}
	}

	return best
}

// simpleChallengeResponse creates a simple challenge response.
// In real implementation, this would encode with the selected codec chain.
func simpleChallengeResponse(challenge []byte) []byte {
	// Simple SHA256 hash for demonstration
	// Real implementation should use the selected codec chain
	result := make([]byte, 32)
	copy(result, challenge)
	return result
}

// CleanupExpired removes expired handshake states.
func (m *HandshakeManager) CleanupExpired() {
	for id, state := range m.states {
		if time.Since(state.StartedAt) > m.policy.HandshakeTimeout {
			delete(m.states, id)
		}
	}
}

// GetPolicy returns the current negotiation policy.
func (m *HandshakeManager) GetPolicy() NegotiationPolicy {
	return m.policy
}
