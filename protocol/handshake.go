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
	"crypto/sha256"
	"crypto/subtle"
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
	// Current implementation uses SHA-256 hash for verification.
	// Production systems should use CodecChain.Encode() with the selected chain.
	// See simpleChallengeResponse documentation for more details.
	expectedResponse := simpleChallengeResponse(state.Challenge)

	// Use constant-time comparison to prevent timing attacks
	match := subtle.ConstantTimeCompare(confirm.ChallengeResponse, expectedResponse) == 1

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

// simpleChallengeResponse creates a challenge response using SHA-256.
//
// IMPORTANT: This is a simplified implementation. Production systems should:
// 1. Use the selected CodecChain.Encode() to process the challenge
// 2. Store the selected CodecChain instance in HandshakeState
// 3. Verify the response using CodecChain.Decode()
//
// The current implementation uses SHA-256 for basic challenge verification.
// This provides integrity verification but NOT the full security proof that
// CodecChain encoding would provide.
//
// TODO: Refactor to store CodecChain instance in HandshakeState and use
// CodecChain.Encode() for challenge processing. This requires:
// - HandshakeManager to have access to codec.ChainRegistry
// - HandshakeState to store selected CodecChain instance
// - ProcessConfirm to use stored CodecChain for verification
func simpleChallengeResponse(challenge []byte) []byte {
	if len(challenge) == 0 {
		return nil
	}

	// Use SHA-256 to create a deterministic response
	// This is better than simple copy but not as secure as CodecChain encoding
	hash := sha256.Sum256(challenge)
	return hash[:]
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

// SerializeRequest serializes HandshakeRequest to bytes.
func SerializeRequest(req *HandshakeRequest) ([]byte, error) {
	if req == nil {
		return nil, ErrInvalidRequest
	}
	// Simple serialization format:
	// [version:1][timestamp:8][clientID_len:2][clientID][serializer_count:2][serializers...][codec_count:2][codecs...]
	result := make([]byte, 0)
	result = append(result, byte(req.Version))
	result = append(result, encodeInt64(req.Timestamp)...)
	result = append(result, encodeUint16(uint16(len(req.ClientID)))...)
	result = append(result, []byte(req.ClientID)...)
	result = append(result, encodeUint16(uint16(len(req.SupportedSerializers)))...)
	for _, s := range req.SupportedSerializers {
		result = append(result, encodeUint16(uint16(len(s.Name)))...)
		result = append(result, []byte(s.Name)...)
		result = append(result, byte(s.Priority))
	}
	result = append(result, encodeUint16(uint16(len(req.SupportedCodecChains)))...)
	for _, c := range req.SupportedCodecChains {
		result = append(result, byte(c.SecurityLevel))
		result = append(result, byte(c.ChainLength))
		result = append(result, encodeUint16(uint16(len(c.Hash)))...)
		result = append(result, []byte(c.Hash)...)
	}
	result = append(result, byte(req.MinSecurityLevel))
	return result, nil
}

// DeserializeRequest deserializes bytes to HandshakeRequest.
func DeserializeRequest(data []byte) (*HandshakeRequest, error) {
	if len(data) < 11 {
		return nil, ErrInvalidRequest
	}
	req := NewHandshakeRequest("")
	offset := 0
	req.Version = data[offset]
	offset++
	req.Timestamp = decodeInt64(data[offset : offset+8])
	offset += 8
	clientIDLen := decodeUint16(data[offset : offset+2])
	offset += 2
	req.ClientID = string(data[offset : offset+int(clientIDLen)])
	offset += int(clientIDLen)
	serializerCount := decodeUint16(data[offset : offset+2])
	offset += 2
	for i := 0; i < int(serializerCount); i++ {
		nameLen := decodeUint16(data[offset : offset+2])
		offset += 2
		name := string(data[offset : offset+int(nameLen)])
		offset += int(nameLen)
		priority := int(data[offset])
		offset++
		req.AddSerializer(name, priority)
	}
	codecCount := decodeUint16(data[offset : offset+2])
	offset += 2
	for i := 0; i < int(codecCount); i++ {
		level := codec.SecurityLevel(data[offset])
		offset++
		chainLen := int(data[offset])
		offset++
		hashLen := decodeUint16(data[offset : offset+2])
		offset += 2
		hash := string(data[offset : offset+int(hashLen)])
		offset += int(hashLen)
		req.AddCodecChain(level, chainLen, hash)
	}
	if offset < len(data) {
		req.MinSecurityLevel = codec.SecurityLevel(data[offset])
	}
	return req, nil
}

// SerializeResponse serializes HandshakeResponse to bytes.
func SerializeResponse(resp *HandshakeResponse) ([]byte, error) {
	if resp == nil {
		return nil, ErrInvalidRequest
	}
	result := make([]byte, 0)
	// [accepted:1][timestamp:8][sessionID_len:2][sessionID][reject_len:2][reject][serializer_len:2][serializer][codec_info][challenge_len:4][challenge]
	result = append(result, byte(0))
	if resp.Accepted {
		result[0] = 1
	}
	result = append(result, encodeInt64(resp.Timestamp)...)
	result = append(result, encodeUint16(uint16(len(resp.SessionID)))...)
	result = append(result, []byte(resp.SessionID)...)
	result = append(result, encodeUint16(uint16(len(resp.RejectReason)))...)
	result = append(result, []byte(resp.RejectReason)...)
	result = append(result, encodeUint16(uint16(len(resp.SelectedSerializer)))...)
	result = append(result, []byte(resp.SelectedSerializer)...)
	result = append(result, byte(resp.SelectedCodecChainInfo.SecurityLevel))
	result = append(result, byte(resp.SelectedCodecChainInfo.ChainLength))
	result = append(result, encodeUint16(uint16(len(resp.SelectedCodecChainInfo.Hash)))...)
	result = append(result, []byte(resp.SelectedCodecChainInfo.Hash)...)
	result = append(result, encodeUint32(uint32(len(resp.ServerChallenge)))...)
	result = append(result, resp.ServerChallenge...)
	return result, nil
}

// DeserializeResponse deserializes bytes to HandshakeResponse.
func DeserializeResponse(data []byte) (*HandshakeResponse, error) {
	if len(data) < 11 {
		return nil, ErrInvalidRequest
	}
	resp := NewHandshakeResponse(data[0] == 1)
	offset := 1
	resp.Timestamp = decodeInt64(data[offset : offset+8])
	offset += 8
	sessionIDLen := decodeUint16(data[offset : offset+2])
	offset += 2
	resp.SessionID = string(data[offset : offset+int(sessionIDLen)])
	offset += int(sessionIDLen)
	rejectLen := decodeUint16(data[offset : offset+2])
	offset += 2
	resp.RejectReason = string(data[offset : offset+int(rejectLen)])
	offset += int(rejectLen)
	serializerLen := decodeUint16(data[offset : offset+2])
	offset += 2
	resp.SelectedSerializer = string(data[offset : offset+int(serializerLen)])
	offset += int(serializerLen)
	resp.SelectedCodecChainInfo.SecurityLevel = codec.SecurityLevel(data[offset])
	offset++
	resp.SelectedCodecChainInfo.ChainLength = int(data[offset])
	offset++
	hashLen := decodeUint16(data[offset : offset+2])
	offset += 2
	resp.SelectedCodecChainInfo.Hash = string(data[offset : offset+int(hashLen)])
	offset += int(hashLen)
	challengeLen := decodeUint32(data[offset : offset+4])
	offset += 4
	if challengeLen > 0 && offset+int(challengeLen) <= len(data) {
		resp.ServerChallenge = data[offset : offset+int(challengeLen)]
	}
	return resp, nil
}

// SerializeConfirm serializes HandshakeConfirm to bytes.
func SerializeConfirm(confirm *HandshakeConfirm) ([]byte, error) {
	if confirm == nil {
		return nil, ErrInvalidSessionID
	}
	result := make([]byte, 0)
	result = append(result, encodeInt64(confirm.Timestamp)...)
	result = append(result, encodeUint16(uint16(len(confirm.SessionID)))...)
	result = append(result, []byte(confirm.SessionID)...)
	result = append(result, encodeUint32(uint32(len(confirm.ChallengeResponse)))...)
	result = append(result, confirm.ChallengeResponse...)
	return result, nil
}

// DeserializeConfirm deserializes bytes to HandshakeConfirm.
func DeserializeConfirm(data []byte) (*HandshakeConfirm, error) {
	if len(data) < 14 {
		return nil, ErrInvalidSessionID
	}
	offset := 0
	_ = decodeInt64(data[offset : offset+8]) // timestamp, not used in construction
	offset += 8
	sessionIDLen := decodeUint16(data[offset : offset+2])
	offset += 2
	sessionID := string(data[offset : offset+int(sessionIDLen)])
	offset += int(sessionIDLen)
	challengeLen := decodeUint32(data[offset : offset+4])
	offset += 4
	challengeResponse := data[offset : offset+int(challengeLen)]
	return NewHandshakeConfirm(sessionID, challengeResponse), nil
}

// Helper encoding functions
func encodeInt64(v int64) []byte {
	result := make([]byte, 8)
	for i := 0; i < 8; i++ {
		result[i] = byte(v >> (56 - i*8))
	}
	return result
}

func decodeInt64(data []byte) int64 {
	result := int64(0)
	for i := 0; i < 8; i++ {
		result |= int64(data[i]) << (56 - i*8)
	}
	return result
}

func encodeUint16(v uint16) []byte {
	return []byte{byte(v >> 8), byte(v)}
}

func decodeUint16(data []byte) uint16 {
	return uint16(data[0])<<8 | uint16(data[1])
}

func encodeUint32(v uint32) []byte {
	return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

func decodeUint32(data []byte) uint32 {
	return uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
}
