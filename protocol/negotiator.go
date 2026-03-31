// Package protocol provides negotiation logic for VoidBus session establishment.
//
// Negotiator handles the complete handshake process:
//
//	Client: PrepareRequest -> ProcessResponse -> PrepareConfirm
//	Server: ProcessRequest -> VerifyConfirm
//
// Security Design:
//   - Serializer name CAN be exposed for negotiation
//   - Codec names MUST NOT be exposed (only SecurityLevel)
//   - Challenge mechanism prevents degradation attacks
//   - Server validates client has claimed codec capability
package protocol

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/internal"
	"github.com/Script-OS/VoidBus/serializer"
)

// Negotiator errors
var (
	ErrNegotiationFailed           = errors.New("negotiator: negotiation failed")
	ErrNegotiationTimeout          = errors.New("negotiator: timeout")
	ErrInvalidOffer                = errors.New("negotiator: invalid offer")
	ErrInvalidChallenge            = errors.New("negotiator: invalid challenge")
	ErrChallengeVerificationFailed = errors.New("negotiator: challenge verification failed")
	ErrSecurityPolicyViolation     = errors.New("negotiator: security policy violation")
	// Note: ErrDegradationAttack is defined in handshake.go
)

// NegotiationState represents the state of a negotiation process.
type NegotiationState int

const (
	// NegotiationStateInitial indicates negotiation not started
	NegotiationStateInitial NegotiationState = iota
	// NegotiationStateRequesting indicates client has sent request
	NegotiationStateRequesting
	// NegotiationStateChallenged indicates server has sent challenge
	NegotiationStateChallenged
	// NegotiationStateConfirming indicates client is confirming
	NegotiationStateConfirming
	// NegotiationStateCompleted indicates negotiation completed
	NegotiationStateCompleted
	// NegotiationStateFailed indicates negotiation failed
	NegotiationStateFailed
)

// String returns string representation.
func (s NegotiationState) String() string {
	switch s {
	case NegotiationStateInitial:
		return "initial"
	case NegotiationStateRequesting:
		return "requesting"
	case NegotiationStateChallenged:
		return "challenged"
	case NegotiationStateConfirming:
		return "confirming"
	case NegotiationStateCompleted:
		return "completed"
	case NegotiationStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ClientOffer contains client's negotiation offer.
type ClientOffer struct {
	// ClientID is the client identifier
	ClientID string

	// SupportedSerializers is list of supported serializers
	SupportedSerializers []SerializerInfo

	// SupportedCodecChains is list of supported codec chain info
	SupportedCodecChains []CodecChainInfo

	// MinSecurityLevel is minimum required security level
	MinSecurityLevel codec.SecurityLevel
}

// ServerOffer contains server's negotiation offer.
type ServerOffer struct {
	// AvailableSerializers is list of available serializers
	AvailableSerializers []serializer.Serializer

	// AvailableCodecChains is list of available codec chains
	AvailableCodecChains []codec.CodecChain

	// Policy is the server's negotiation policy
	Policy NegotiationPolicy
}

// NegotiationResult contains the result of successful negotiation.
type NegotiationResult struct {
	// SessionID is the assigned session ID
	SessionID string

	// ClientID is the client identifier
	ClientID string

	// SelectedSerializer is the selected serializer
	SelectedSerializer serializer.Serializer

	// SelectedSerializerType is the serializer name
	SelectedSerializerType string

	// SelectedCodecChain is the selected codec chain
	SelectedCodecChain codec.CodecChain

	// SelectedSecurityLevel is the negotiated security level
	SelectedSecurityLevel codec.SecurityLevel

	// CodecChainHash is the hash of selected codec chain
	CodecChainHash string

	// CompletedAt is completion timestamp
	CompletedAt time.Time
}

// PendingNegotiation tracks an in-progress negotiation.
type PendingNegotiation struct {
	mu sync.RWMutex

	// ID is the negotiation tracking ID
	ID string

	// State is current negotiation state
	State NegotiationState

	// ClientID is the client identifier
	ClientID string

	// Request is the original request
	Request *HandshakeRequest

	// Response is the server response
	Response *HandshakeResponse

	// Challenge is the server challenge data
	Challenge []byte

	// SelectedSerializer is the selected serializer
	SelectedSerializer serializer.Serializer

	// SelectedCodecChain is the selected codec chain
	SelectedCodecChain codec.CodecChain

	// CreatedAt is creation time
	CreatedAt time.Time

	// UpdatedAt is last update time
	UpdatedAt time.Time
}

// ClientNegotiator handles client-side negotiation.
type ClientNegotiator struct {
	mu     sync.RWMutex
	offer  ClientOffer
	policy NegotiationPolicy
}

// NewClientNegotiator creates a new client negotiator.
func NewClientNegotiator(offer ClientOffer, policy NegotiationPolicy) *ClientNegotiator {
	return &ClientNegotiator{
		offer:  offer,
		policy: policy,
	}
}

// PrepareRequest prepares a handshake request from client offer.
func (n *ClientNegotiator) PrepareRequest() *HandshakeRequest {
	n.mu.RLock()
	defer n.mu.RUnlock()

	req := NewHandshakeRequest(n.offer.ClientID)
	req.MinSecurityLevel = n.offer.MinSecurityLevel
	req.SupportedSerializers = n.offer.SupportedSerializers
	req.SupportedCodecChains = n.offer.SupportedCodecChains

	return req
}

// ProcessResponse processes server's response and prepares confirmation.
// Returns the confirmation to send back, or error if response is rejected.
func (n *ClientNegotiator) ProcessResponse(
	response *HandshakeResponse,
	codecChain codec.CodecChain,
) (*HandshakeConfirm, error) {
	if !response.Accepted {
		return nil, errors.New(response.RejectReason)
	}

	// Verify security level
	if response.SelectedCodecChainInfo.SecurityLevel < n.offer.MinSecurityLevel {
		return nil, ErrSecurityPolicyViolation
	}

	// Generate challenge response using the codec chain
	challengeResponse, err := n.generateChallengeResponse(response.ServerChallenge, codecChain)
	if err != nil {
		return nil, err
	}

	return NewHandshakeConfirm(response.SessionID, challengeResponse), nil
}

// generateChallengeResponse encodes challenge with codec chain.
func (n *ClientNegotiator) generateChallengeResponse(
	challenge []byte,
	codecChain codec.CodecChain,
) ([]byte, error) {
	// Encode the challenge with the codec chain to prove we have the capability
	encoded, err := codecChain.Encode(challenge)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// ServerNegotiator handles server-side negotiation.
type ServerNegotiator struct {
	mu            sync.RWMutex
	offer         ServerOffer
	pending       map[string]*PendingNegotiation
	challengeSize int
}

// NewServerNegotiator creates a new server negotiator.
func NewServerNegotiator(offer ServerOffer) *ServerNegotiator {
	return &ServerNegotiator{
		offer:         offer,
		pending:       make(map[string]*PendingNegotiation),
		challengeSize: 32, // 32 bytes challenge
	}
}

// ProcessRequest processes a client's handshake request.
// Returns response to send back to client, or error if request is rejected.
func (n *ServerNegotiator) ProcessRequest(request *HandshakeRequest) (*HandshakeResponse, *PendingNegotiation, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Validate request
	if err := request.Validate(); err != nil {
		return RejectHandshake(err.Error()), nil, err
	}

	// Check security level
	if request.MinSecurityLevel > n.offer.Policy.MinSecurityLevel {
		return RejectHandshake("security level not supported"), nil, ErrSecurityPolicyViolation
	}

	// Check for degradation attack
	if !n.offer.Policy.DebugMode && request.MinSecurityLevel < n.offer.Policy.MinSecurityLevel {
		return RejectHandshake("security level too low"), nil, ErrDegradationAttack
	}

	// Select serializer
	selectedSerializer := n.selectSerializer(request.SupportedSerializers)
	if selectedSerializer == nil {
		return RejectHandshake("no compatible serializer"), nil, ErrNegotiationFailed
	}

	// Select codec chain
	selectedChain, chainInfo := n.selectCodecChain(request.SupportedCodecChains)
	if selectedChain == nil {
		return RejectHandshake("no compatible codec chain"), nil, ErrNegotiationFailed
	}

	// Verify security level
	if chainInfo.SecurityLevel < n.offer.Policy.MinSecurityLevel && !n.offer.Policy.DebugMode {
		return RejectHandshake("codec security level too low"), nil, ErrDegradationAttack
	}

	// Generate challenge
	challenge, err := n.generateChallenge()
	if err != nil {
		return RejectHandshake("internal error"), nil, err
	}

	// Create session ID
	sessionID := internal.GenerateSessionID()

	// Create response
	response := NewHandshakeResponse(true).
		WithSerializer(selectedSerializer.Name()).
		WithCodecChain(chainInfo).
		WithChallenge(challenge)
	response.SessionID = sessionID

	// Create pending negotiation
	pending := &PendingNegotiation{
		ID:                 internal.GenerateID(),
		State:              NegotiationStateChallenged,
		ClientID:           request.ClientID,
		Request:            request,
		Response:           response,
		Challenge:          challenge,
		SelectedSerializer: selectedSerializer,
		SelectedCodecChain: selectedChain,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	n.pending[pending.ID] = pending

	return response, pending, nil
}

// ProcessConfirm processes client's confirmation and completes negotiation.
func (n *ServerNegotiator) ProcessConfirm(
	confirm *HandshakeConfirm,
	pending *PendingNegotiation,
) (*NegotiationResult, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Check timeout
	if time.Since(pending.CreatedAt) > n.offer.Policy.ChallengeTimeout {
		pending.State = NegotiationStateFailed
		return nil, ErrNegotiationTimeout
	}

	// Verify challenge response
	if err := n.verifyChallengeResponse(
		pending.Challenge,
		confirm.ChallengeResponse,
		pending.SelectedCodecChain,
	); err != nil {
		pending.State = NegotiationStateFailed
		return nil, ErrChallengeVerificationFailed
	}

	// Success
	pending.State = NegotiationStateCompleted
	pending.UpdatedAt = time.Now()

	result := &NegotiationResult{
		SessionID:              confirm.SessionID,
		ClientID:               pending.ClientID,
		SelectedSerializer:     pending.SelectedSerializer,
		SelectedSerializerType: pending.SelectedSerializer.Name(),
		SelectedCodecChain:     pending.SelectedCodecChain,
		SelectedSecurityLevel:  pending.SelectedCodecChain.SecurityLevel(),
		CodecChainHash:         pending.Response.SelectedCodecChainInfo.Hash,
		CompletedAt:            time.Now(),
	}

	// Remove from pending
	delete(n.pending, pending.ID)

	return result, nil
}

// selectSerializer selects the best serializer from client's supported list.
func (n *ServerNegotiator) selectSerializer(clientSerializers []SerializerInfo) serializer.Serializer {
	// Build map of client serializers by name
	clientMap := make(map[string]int)
	for _, s := range clientSerializers {
		clientMap[s.Name] = s.Priority
	}

	// Find best match
	var best serializer.Serializer
	bestPriority := -1

	for _, ser := range n.offer.AvailableSerializers {
		if priority, ok := clientMap[ser.Name()]; ok {
			// Check if allowed by policy
			if len(n.offer.Policy.AllowedSerializers) > 0 {
				allowed := false
				for _, a := range n.offer.Policy.AllowedSerializers {
					if a == ser.Name() {
						allowed = true
						break
					}
				}
				if !allowed {
					continue
				}
			}

			if priority > bestPriority {
				best = ser
				bestPriority = priority
			}
		}
	}

	// Check preferred serializer
	if n.offer.Policy.PreferredSerializer != "" {
		for _, ser := range n.offer.AvailableSerializers {
			if ser.Name() == n.offer.Policy.PreferredSerializer {
				if _, ok := clientMap[ser.Name()]; ok {
					return ser
				}
			}
		}
	}

	return best
}

// selectCodecChain selects the best codec chain from client's supported list.
func (n *ServerNegotiator) selectCodecChain(clientChains []CodecChainInfo) (codec.CodecChain, CodecChainInfo) {
	// Build map of client chains by security level
	clientByLevel := make(map[codec.SecurityLevel][]CodecChainInfo)
	for _, c := range clientChains {
		clientByLevel[c.SecurityLevel] = append(clientByLevel[c.SecurityLevel], c)
	}

	// Find best match
	var bestChain codec.CodecChain
	var bestInfo CodecChainInfo
	bestLevel := codec.SecurityLevelNone

	for _, chain := range n.offer.AvailableCodecChains {
		chainLevel := chain.SecurityLevel()

		// Must meet minimum
		if chainLevel < n.offer.Policy.MinSecurityLevel {
			continue
		}

		// Must be supported by client
		clientChains, ok := clientByLevel[chainLevel]
		if !ok {
			continue
		}

		// Check for hash match
		chainInfo := generateChainInfo(chain)
		for _, cc := range clientChains {
			if cc.ChainLength == chainInfo.ChainLength {
				// Prefer higher security
				if chainLevel > bestLevel {
					bestChain = chain
					bestInfo = chainInfo
					bestLevel = chainLevel
				}
			}
		}
	}

	// Check preferred security level
	if n.offer.Policy.PreferredCodecChainSecurity > codec.SecurityLevelNone {
		// Find chain with preferred security level
		for _, chain := range n.offer.AvailableCodecChains {
			if chain.SecurityLevel() == n.offer.Policy.PreferredCodecChainSecurity {
				return chain, generateChainInfo(chain)
			}
		}
	}

	return bestChain, bestInfo
}

// generateChallenge generates a random challenge.
func (n *ServerNegotiator) generateChallenge() ([]byte, error) {
	challenge := make([]byte, n.challengeSize)
	_, err := rand.Read(challenge)
	if err != nil {
		return nil, err
	}
	return challenge, nil
}

// verifyChallengeResponse verifies client's challenge response.
func (n *ServerNegotiator) verifyChallengeResponse(
	challenge []byte,
	response []byte,
	chain codec.CodecChain,
) error {
	// Decode the response using the codec chain
	decoded, err := chain.Decode(response)
	if err != nil {
		return err
	}

	// Compare with original challenge
	if len(decoded) != len(challenge) {
		return ErrChallengeVerificationFailed
	}

	for i := range challenge {
		if decoded[i] != challenge[i] {
			return ErrChallengeVerificationFailed
		}
	}

	return nil
}

// GetPending returns pending negotiation by ID.
func (n *ServerNegotiator) GetPending(id string) (*PendingNegotiation, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	pending, ok := n.pending[id]
	return pending, ok
}

// RemovePending removes a pending negotiation.
func (n *ServerNegotiator) RemovePending(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	delete(n.pending, id)
}

// CleanupExpired removes expired pending negotiations.
func (n *ServerNegotiator) CleanupExpired() int {
	n.mu.Lock()
	defer n.mu.Unlock()

	count := 0
	for id, pending := range n.pending {
		if time.Since(pending.CreatedAt) > n.offer.Policy.HandshakeTimeout {
			delete(n.pending, id)
			count++
		}
	}
	return count
}

// PendingCount returns number of pending negotiations.
func (n *ServerNegotiator) PendingCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.pending)
}

// generateChainInfo generates CodecChainInfo from a codec chain.
func generateChainInfo(chain codec.CodecChain) CodecChainInfo {
	if chain == nil {
		return CodecChainInfo{}
	}
	return CodecChainInfo{
		SecurityLevel: chain.SecurityLevel(),
		ChainLength:   chain.Length(),
		Hash:          computeChainHash(chain),
	}
}

// computeChainHash computes a deterministic hash for codec chain configuration.
// Note: Does NOT expose codec names, only security levels.
// Uses SHA-256 truncated to 8 bytes for efficient transmission.
func computeChainHash(chain codec.CodecChain) string {
	// Use security levels to compute hash
	// This allows matching without exposing codec names
	levels := make([]byte, chain.Length())
	for i := 0; i < chain.Length(); i++ {
		c, err := chain.GetCodec(i)
		if err != nil {
			continue
		}
		levels[i] = byte(c.SecurityLevel())
	}

	// Empty chain returns empty hash
	if len(levels) == 0 {
		return ""
	}

	// Compute SHA-256 hash of security levels
	// This is deterministic and allows chain matching
	hash := sha256.Sum256(levels)

	// Return first 8 bytes as hex string (16 characters)
	// This is sufficient for matching while keeping size small
	return hex.EncodeToString(hash[:8])
}

// NegotiatorBuilder provides fluent API for building negotiators.
type NegotiatorBuilder struct {
	clientOffer  ClientOffer
	serverOffer  ServerOffer
	clientPolicy NegotiationPolicy
}

// NewNegotiatorBuilder creates a new negotiator builder.
func NewNegotiatorBuilder() *NegotiatorBuilder {
	return &NegotiatorBuilder{
		clientPolicy: DefaultNegotiationPolicy(),
	}
}

// WithClientID sets client ID.
func (b *NegotiatorBuilder) WithClientID(id string) *NegotiatorBuilder {
	b.clientOffer.ClientID = id
	return b
}

// WithSerializer adds a supported serializer.
func (b *NegotiatorBuilder) WithSerializer(name string, priority int) *NegotiatorBuilder {
	b.clientOffer.SupportedSerializers = append(
		b.clientOffer.SupportedSerializers,
		SerializerInfo{Name: name, Priority: priority},
	)
	return b
}

// WithCodecChain adds a supported codec chain info.
func (b *NegotiatorBuilder) WithCodecChain(level codec.SecurityLevel, length int, hash string) *NegotiatorBuilder {
	b.clientOffer.SupportedCodecChains = append(
		b.clientOffer.SupportedCodecChains,
		CodecChainInfo{SecurityLevel: level, ChainLength: length, Hash: hash},
	)
	return b
}

// WithMinSecurityLevel sets minimum security level.
func (b *NegotiatorBuilder) WithMinSecurityLevel(level codec.SecurityLevel) *NegotiatorBuilder {
	b.clientOffer.MinSecurityLevel = level
	return b
}

// WithServerSerializers sets server available serializers.
func (b *NegotiatorBuilder) WithServerSerializers(serializers ...serializer.Serializer) *NegotiatorBuilder {
	b.serverOffer.AvailableSerializers = serializers
	return b
}

// WithServerCodecChains sets server available codec chains.
func (b *NegotiatorBuilder) WithServerCodecChains(chains ...codec.CodecChain) *NegotiatorBuilder {
	b.serverOffer.AvailableCodecChains = chains
	return b
}

// WithServerPolicy sets server negotiation policy.
func (b *NegotiatorBuilder) WithServerPolicy(policy NegotiationPolicy) *NegotiatorBuilder {
	b.serverOffer.Policy = policy
	return b
}

// BuildClient builds a client negotiator.
func (b *NegotiatorBuilder) BuildClient() *ClientNegotiator {
	return NewClientNegotiator(b.clientOffer, b.clientPolicy)
}

// BuildServer builds a server negotiator.
func (b *NegotiatorBuilder) BuildServer() *ServerNegotiator {
	return NewServerNegotiator(b.serverOffer)
}
