// Package registry provides session registry for VoidBus.
//
// SessionRegistry manages session configurations locally.
// Sessions are referenced by SessionID in packets, but the actual
// configuration is NEVER transmitted over the network.
//
// Security Design:
//   - CodecChain configuration NOT exposed
//   - Channel configuration NOT exposed
//   - KeyProvider info NOT exposed
//   - Only SessionID is used as indirect reference in packets
package registry

import (
	"errors"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/fragment"
	"github.com/Script-OS/VoidBus/keyprovider"
	"github.com/Script-OS/VoidBus/protocol"
	"github.com/Script-OS/VoidBus/serializer"
)

// Session registry errors
var (
	ErrSessionNotFound  = errors.New("session: not found")
	ErrSessionExists    = errors.New("session: already exists")
	ErrSessionExpired   = errors.New("session: expired")
	ErrInvalidSessionID = errors.New("session: invalid id")
)

// RegistryStatistics contains registry statistics.
type RegistryStatistics struct {
	TotalSessions    int
	ActiveSessions   int
	IdleSessions     int
	ClosedSessions   int
	HandshakingCount int
	ExpiredCount     int
}

// SessionConfig holds configuration for a session.
// IMPORTANT: This structure is stored locally only and MUST NOT be transmitted over network.
//
// Security Note:
// - CodecChain configuration is NOT transmitted
// - Channel type is NOT transmitted
// - KeyProvider info is NOT transmitted
// - Only SessionID is used as indirect reference in packets
type SessionConfig struct {
	// SessionID is unique identifier for the session
	SessionID string

	// Status is session status
	Status protocol.SessionStatus

	// Serializer is the selected serializer
	Serializer serializer.Serializer

	// CodecChain is the selected codec chain
	CodecChain codec.CodecChain

	// Channel is the channel instance
	Channel channel.Channel

	// KeyProvider is the key provider (optional)
	KeyProvider keyprovider.KeyProvider

	// Fragment is the fragment handler (optional)
	Fragment fragment.Fragment

	// SecurityLevel is the negotiated security level
	SecurityLevel codec.SecurityLevel

	// ClientID is client identifier
	ClientID string

	// RemoteAddress is remote endpoint
	RemoteAddress string

	// CreatedAt is session creation time
	CreatedAt time.Time

	// LastActivity is last activity time
	LastActivity time.Time

	// Metadata contains additional session metadata
	Metadata map[string]string

	// Statistics
	SendCount    int64
	ReceiveCount int64
	ErrorCount   int64
}

// SessionRegistry manages session configurations.
type SessionRegistry interface {
	// Register registers a new session.
	Register(config SessionConfig) error

	// RegisterSession registers from protocol.Session.
	RegisterSession(session *protocol.Session) error

	// Get retrieves session configuration by ID.
	Get(sessionID string) (*SessionConfig, error)

	// GetSession retrieves as protocol.Session.
	GetSession(sessionID string) (*protocol.Session, error)

	// Update updates session configuration.
	Update(sessionID string, config SessionConfig) error

	// UpdateSession updates from protocol.Session.
	UpdateSession(session *protocol.Session) error

	// Remove removes a session.
	Remove(sessionID string) error

	// Exists checks if session exists.
	Exists(sessionID string) bool

	// List returns all session IDs.
	List() []string

	// ListByStatus returns session IDs by status.
	ListByStatus(status protocol.SessionStatus) []string

	// Count returns number of sessions.
	Count() int

	// CountByStatus returns number of sessions by status.
	CountByStatus(status protocol.SessionStatus) int

	// Clear removes all sessions.
	Clear() error

	// Touch updates LastActivity for a session.
	Touch(sessionID string) error

	// GetExpired returns expired session IDs.
	GetExpired(timeout time.Duration) []string

	// CleanupExpired removes expired sessions and returns count.
	CleanupExpired(timeout time.Duration) int

	// GetActive returns active session IDs.
	GetActive() []string

	// GetStatistics returns registry statistics.
	GetStatistics() RegistryStatistics
}

// DefaultSessionRegistry is the default SessionRegistry implementation.
type DefaultSessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*SessionConfig
}

// NewSessionRegistry creates a new SessionRegistry.
func NewSessionRegistry() *DefaultSessionRegistry {
	return &DefaultSessionRegistry{
		sessions: make(map[string]*SessionConfig),
	}
}

// Register registers a new session.
func (r *DefaultSessionRegistry) Register(config SessionConfig) error {
	if config.SessionID == "" {
		return ErrInvalidSessionID
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[config.SessionID]; exists {
		return ErrSessionExists
	}

	// Set timestamps
	if config.CreatedAt.IsZero() {
		config.CreatedAt = time.Now()
	}
	config.LastActivity = config.CreatedAt

	// Initialize metadata if nil
	if config.Metadata == nil {
		config.Metadata = make(map[string]string)
	}

	r.sessions[config.SessionID] = &config
	return nil
}

// Get retrieves session configuration.
func (r *DefaultSessionRegistry) Get(sessionID string) (*SessionConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	return config, nil
}

// Update updates session configuration.
func (r *DefaultSessionRegistry) Update(sessionID string, config SessionConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[sessionID]; !exists {
		return ErrSessionNotFound
	}

	config.SessionID = sessionID
	config.LastActivity = time.Now()
	r.sessions[sessionID] = &config
	return nil
}

// Remove removes a session.
func (r *DefaultSessionRegistry) Remove(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sessions, sessionID)
	return nil
}

// Exists checks if session exists.
func (r *DefaultSessionRegistry) Exists(sessionID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.sessions[sessionID]
	return exists
}

// List returns all session IDs.
func (r *DefaultSessionRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.sessions))
	for id := range r.sessions {
		result = append(result, id)
	}
	return result
}

// Count returns number of sessions.
func (r *DefaultSessionRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}

// Clear removes all sessions.
func (r *DefaultSessionRegistry) Clear() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions = make(map[string]*SessionConfig)
	return nil
}

// Touch updates LastActivity for a session.
func (r *DefaultSessionRegistry) Touch(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	config, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	config.LastActivity = time.Now()
	return nil
}

// GetExpired returns expired session IDs.
func (r *DefaultSessionRegistry) GetExpired(timeout time.Duration) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	expired := make([]string, 0)

	for id, config := range r.sessions {
		if now.Sub(config.LastActivity) > timeout {
			expired = append(expired, id)
		}
	}
	return expired
}

// RegisterSession registers from protocol.Session.
func (r *DefaultSessionRegistry) RegisterSession(session *protocol.Session) error {
	if session == nil || session.ID == "" {
		return ErrInvalidSessionID
	}

	config := SessionConfig{
		SessionID:     session.ID,
		Status:        session.Status,
		Serializer:    session.Serializer,
		CodecChain:    session.CodecChain,
		Channel:       session.Channel,
		KeyProvider:   session.KeyProvider,
		Fragment:      session.Fragment,
		SecurityLevel: session.SecurityLevel,
		ClientID:      session.ClientID,
		RemoteAddress: session.RemoteAddress,
		CreatedAt:     session.CreatedAt,
		LastActivity:  session.LastActivity,
		Metadata:      session.Metadata,
		SendCount:     session.SendCount,
		ReceiveCount:  session.ReceiveCount,
		ErrorCount:    session.ErrorCount,
	}

	return r.Register(config)
}

// GetSession retrieves as protocol.Session.
func (r *DefaultSessionRegistry) GetSession(sessionID string) (*protocol.Session, error) {
	config, err := r.Get(sessionID)
	if err != nil {
		return nil, err
	}

	session := protocol.NewSession(config.SessionID)
	session.Status = config.Status
	session.Serializer = config.Serializer
	session.CodecChain = config.CodecChain
	session.Channel = config.Channel
	session.KeyProvider = config.KeyProvider
	session.Fragment = config.Fragment
	session.SecurityLevel = config.SecurityLevel
	session.ClientID = config.ClientID
	session.RemoteAddress = config.RemoteAddress
	session.CreatedAt = config.CreatedAt
	session.LastActivity = config.LastActivity
	session.Metadata = config.Metadata
	session.SendCount = config.SendCount
	session.ReceiveCount = config.ReceiveCount
	session.ErrorCount = config.ErrorCount

	return session, nil
}

// UpdateSession updates from protocol.Session.
func (r *DefaultSessionRegistry) UpdateSession(session *protocol.Session) error {
	if session == nil || session.ID == "" {
		return ErrInvalidSessionID
	}

	config := SessionConfig{
		SessionID:     session.ID,
		Status:        session.Status,
		Serializer:    session.Serializer,
		CodecChain:    session.CodecChain,
		Channel:       session.Channel,
		KeyProvider:   session.KeyProvider,
		Fragment:      session.Fragment,
		SecurityLevel: session.SecurityLevel,
		ClientID:      session.ClientID,
		RemoteAddress: session.RemoteAddress,
		CreatedAt:     session.CreatedAt,
		LastActivity:  session.LastActivity,
		Metadata:      session.Metadata,
		SendCount:     session.SendCount,
		ReceiveCount:  session.ReceiveCount,
		ErrorCount:    session.ErrorCount,
	}

	return r.Update(session.ID, config)
}

// ListByStatus returns session IDs by status.
func (r *DefaultSessionRegistry) ListByStatus(status protocol.SessionStatus) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0)
	for id, config := range r.sessions {
		if config.Status == status {
			result = append(result, id)
		}
	}
	return result
}

// CountByStatus returns number of sessions by status.
func (r *DefaultSessionRegistry) CountByStatus(status protocol.SessionStatus) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, config := range r.sessions {
		if config.Status == status {
			count++
		}
	}
	return count
}

// CleanupExpired removes expired sessions and returns count.
func (r *DefaultSessionRegistry) CleanupExpired(timeout time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	count := 0

	for id, config := range r.sessions {
		if now.Sub(config.LastActivity) > timeout {
			delete(r.sessions, id)
			count++
		}
	}
	return count
}

// GetActive returns active session IDs.
func (r *DefaultSessionRegistry) GetActive() []string {
	return r.ListByStatus(protocol.SessionStatusActive)
}

// GetStatistics returns registry statistics.
func (r *DefaultSessionRegistry) GetStatistics() RegistryStatistics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := RegistryStatistics{
		TotalSessions: len(r.sessions),
	}

	for _, config := range r.sessions {
		switch config.Status {
		case protocol.SessionStatusActive:
			stats.ActiveSessions++
		case protocol.SessionStatusIdle:
			stats.IdleSessions++
		case protocol.SessionStatusClosed:
			stats.ClosedSessions++
		case protocol.SessionStatusHandshaking:
			stats.HandshakingCount++
		}
	}

	return stats
}
