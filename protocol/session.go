// Package protocol provides session management for VoidBus.
//
// Session represents an established communication session after handshake.
// Session configuration is stored locally and MUST NOT be transmitted over network.
//
// Security Design:
//   - Session configuration NOT exposed in packets
//   - Only SessionID (random UUID) is transmitted as indirect reference
//   - SecurityLevel is tracked locally for policy enforcement
package protocol

import (
	"errors"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/fragment"
	"github.com/Script-OS/VoidBus/keyprovider"
	"github.com/Script-OS/VoidBus/serializer"
)

// Session errors
var (
	// ErrSessionClosed indicates session is closed
	ErrSessionClosed = errors.New("session: closed")
	// ErrSessionNotActive indicates session is not active
	ErrSessionNotActive = errors.New("session: not active")
	// ErrSessionNotReady indicates session is not ready for communication
	ErrSessionNotReady = errors.New("session: not ready")
)

// SessionStatus represents the status of a session.
type SessionStatus int

const (
	// SessionStatusHandshaking indicates session is in handshake phase
	SessionStatusHandshaking SessionStatus = iota
	// SessionStatusActive indicates session is active and ready for communication
	SessionStatusActive
	// SessionStatusIdle indicates session is idle (no recent activity)
	SessionStatusIdle
	// SessionStatusClosing indicates session is being closed
	SessionStatusClosing
	// SessionStatusClosed indicates session is closed
	SessionStatusClosed
)

// String returns string representation of SessionStatus.
func (s SessionStatus) String() string {
	switch s {
	case SessionStatusHandshaking:
		return "handshaking"
	case SessionStatusActive:
		return "active"
	case SessionStatusIdle:
		return "idle"
	case SessionStatusClosing:
		return "closing"
	case SessionStatusClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// Session represents an established communication session.
// IMPORTANT: This structure is stored locally only and MUST NOT be transmitted.
//
// Security Note:
// - CodecChain.InternalIDs() MUST NOT be transmitted
// - Channel.Type() MUST NOT be transmitted
// - KeyProvider info MUST NOT be transmitted
// - Only SessionID is used as indirect reference in packets
type Session struct {
	mu sync.RWMutex

	// ID is unique session identifier (random UUID, no semantic info)
	// CAN be exposed in packets as indirect reference
	ID string

	// ClientID is client identifier (from handshake)
	// Stored locally, NOT transmitted after handshake
	ClientID string

	// Serializer is the negotiated serializer
	// Serializer.Name() CAN be exposed for negotiation
	Serializer serializer.Serializer

	// CodecChain is the negotiated codec chain
	// CodecChain.InternalIDs() MUST NOT be transmitted
	// Only CodecChain.SecurityLevel() is used for negotiation
	CodecChain codec.CodecChain

	// Channel is the channel instance for this session
	// Channel.Type() MUST NOT be transmitted
	Channel channel.Channel

	// KeyProvider is the key provider (optional)
	// KeyProvider info MUST NOT be transmitted
	KeyProvider keyprovider.KeyProvider

	// Fragment is the fragment handler (optional)
	Fragment fragment.Fragment

	// SecurityLevel is the negotiated security level
	// CAN be used for negotiation, but NOT codec names
	SecurityLevel codec.SecurityLevel

	// SerializerType is the negotiated serializer name
	// CAN be exposed (serializer name only)
	SerializerType string

	// CodecChainHash is the hash of codec chain configuration
	// Used for configuration verification, does NOT expose codec names
	CodecChainHash string

	// RemoteAddress is the remote endpoint address
	// Stored locally for logging/monitoring
	RemoteAddress string

	// CreatedAt is session creation time
	CreatedAt time.Time

	// LastActivity is last activity timestamp
	LastActivity time.Time

	// Status is current session status
	Status SessionStatus

	// Metadata contains additional session metadata
	Metadata map[string]string

	// Statistics
	SendCount    int64
	ReceiveCount int64
	ErrorCount   int64
}

// NewSession creates a new session with specified configuration.
func NewSession(id string) *Session {
	return &Session{
		ID:           id,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		Status:       SessionStatusHandshaking,
		Metadata:     make(map[string]string),
	}
}

// SetSerializer sets the serializer for this session.
func (s *Session) SetSerializer(ser serializer.Serializer) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Serializer = ser
	if ser != nil {
		s.SerializerType = ser.Name()
	}
	return s
}

// SetCodecChain sets the codec chain for this session.
func (s *Session) SetCodecChain(chain codec.CodecChain) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CodecChain = chain
	if chain != nil {
		s.SecurityLevel = chain.SecurityLevel()
	}
	return s
}

// SetChannel sets the channel for this session.
func (s *Session) SetChannel(ch channel.Channel) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Channel = ch
	return s
}

// SetKeyProvider sets the key provider for this session.
func (s *Session) SetKeyProvider(kp keyprovider.KeyProvider) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.KeyProvider = kp
	return s
}

// SetFragment sets the fragment handler for this session.
func (s *Session) SetFragment(f fragment.Fragment) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Fragment = f
	return s
}

// SetClientID sets the client identifier.
func (s *Session) SetClientID(clientID string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ClientID = clientID
	return s
}

// SetRemoteAddress sets the remote address.
func (s *Session) SetRemoteAddress(addr string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RemoteAddress = addr
	return s
}

// SetStatus sets the session status.
func (s *Session) SetStatus(status SessionStatus) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
	return s
}

// SetMetadata sets a metadata key-value pair.
func (s *Session) SetMetadata(key, value string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Metadata[key] = value
	return s
}

// Activate activates the session (transition to Active status).
func (s *Session) Activate() *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = SessionStatusActive
	s.LastActivity = time.Now()
	return s
}

// Close closes the session (transition to Closed status).
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = SessionStatusClosed
	if s.Channel != nil {
		return s.Channel.Close()
	}
	return nil
}

// Touch updates the last activity timestamp.
func (s *Session) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActivity = time.Now()
}

// GetID returns the session ID.
func (s *Session) GetID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ID
}

// GetClientID returns the client ID.
func (s *Session) GetClientID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ClientID
}

// GetStatus returns the session status.
func (s *Session) GetStatus() SessionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status
}

// GetSecurityLevel returns the security level.
func (s *Session) GetSecurityLevel() codec.SecurityLevel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SecurityLevel
}

// GetSerializerType returns the serializer type.
func (s *Session) GetSerializerType() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SerializerType
}

// GetCreatedAt returns creation time.
func (s *Session) GetCreatedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CreatedAt
}

// GetLastActivity returns last activity time.
func (s *Session) GetLastActivity() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastActivity
}

// GetStatistics returns session statistics.
func (s *Session) GetStatistics() (sendCount, receiveCount, errorCount int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SendCount, s.ReceiveCount, s.ErrorCount
}

// IncrementSendCount increments send count.
func (s *Session) IncrementSendCount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SendCount++
}

// IncrementReceiveCount increments receive count.
func (s *Session) IncrementReceiveCount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ReceiveCount++
}

// IncrementErrorCount increments error count.
func (s *Session) IncrementErrorCount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ErrorCount++
}

// IsExpired checks if session is expired based on timeout.
func (s *Session) IsExpired(timeout time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.LastActivity) > timeout
}

// IsExpiredSince checks if session is expired since specific time.
func (s *Session) IsExpiredSince(since time.Time, timeout time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return since.Sub(s.LastActivity) > timeout
}

// IsActive returns whether session is active.
func (s *Session) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status == SessionStatusActive && s.Channel != nil && s.Channel.IsConnected()
}

// IsConnected returns whether session channel is connected.
func (s *Session) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Channel == nil {
		return false
	}
	return s.Channel.IsConnected()
}

// Encode encodes data using the session's codec chain.
func (s *Session) Encode(data []byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.CodecChain == nil {
		return data, nil
	}
	return s.CodecChain.Encode(data)
}

// Decode decodes data using the session's codec chain.
func (s *Session) Decode(data []byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.CodecChain == nil {
		return data, nil
	}
	return s.CodecChain.Decode(data)
}

// Serialize serializes data using the session's serializer.
func (s *Session) Serialize(data []byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Serializer == nil {
		return data, nil
	}
	return s.Serializer.Serialize(data)
}

// Deserialize deserializes data using the session's serializer.
func (s *Session) Deserialize(data []byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Serializer == nil {
		return data, nil
	}
	return s.Serializer.Deserialize(data)
}

// Send sends data through the session's channel.
func (s *Session) Send(data []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Channel == nil {
		return ErrSessionClosed
	}

	err := s.Channel.Send(data)
	if err != nil {
		s.ErrorCount++
		return err
	}

	s.SendCount++
	s.LastActivity = time.Now()
	return nil
}

// Receive receives data from the session's channel.
func (s *Session) Receive() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Channel == nil {
		return nil, ErrSessionClosed
	}

	data, err := s.Channel.Receive()
	if err != nil {
		s.ErrorCount++
		return nil, err
	}

	s.ReceiveCount++
	s.LastActivity = time.Now()
	return data, nil
}

// Clone creates a copy of session configuration (without channel state).
func (s *Session) Clone() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := NewSession(s.ID)
	clone.ClientID = s.ClientID
	clone.Serializer = s.Serializer
	clone.CodecChain = s.CodecChain
	clone.SecurityLevel = s.SecurityLevel
	clone.SerializerType = s.SerializerType
	clone.CodecChainHash = s.CodecChainHash
	clone.RemoteAddress = s.RemoteAddress
	clone.KeyProvider = s.KeyProvider
	clone.Fragment = s.Fragment
	clone.Metadata = make(map[string]string)
	for k, v := range s.Metadata {
		clone.Metadata[k] = v
	}

	return clone
}

// SessionInfo contains public session information (safe for logging/monitoring).
// Does NOT include sensitive configuration details.
type SessionInfo struct {
	ID             string
	ClientID       string
	SerializerType string
	SecurityLevel  codec.SecurityLevel
	RemoteAddress  string
	Status         SessionStatus
	CreatedAt      time.Time
	LastActivity   time.Time
	SendCount      int64
	ReceiveCount   int64
	ErrorCount     int64
	IsConnected    bool
}

// GetInfo returns safe session information for logging/monitoring.
func (s *Session) GetInfo() SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SessionInfo{
		ID:             s.ID,
		ClientID:       s.ClientID,
		SerializerType: s.SerializerType,
		SecurityLevel:  s.SecurityLevel,
		RemoteAddress:  s.RemoteAddress,
		Status:         s.Status,
		CreatedAt:      s.CreatedAt,
		LastActivity:   s.LastActivity,
		SendCount:      s.SendCount,
		ReceiveCount:   s.ReceiveCount,
		ErrorCount:     s.ErrorCount,
		IsConnected:    s.IsConnected(),
	}
}
