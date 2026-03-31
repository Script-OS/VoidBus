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
	"github.com/Script-OS/VoidBus/serializer"
)

// Session registry errors
var (
	ErrSessionNotFound  = errors.New("session: not found")
	ErrSessionExists    = errors.New("session: already exists")
	ErrSessionExpired   = errors.New("session: expired")
	ErrInvalidSessionID = errors.New("session: invalid id")
)

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

	// CreatedAt is session creation time
	CreatedAt time.Time

	// LastActivity is last activity time
	LastActivity time.Time

	// Metadata contains additional session metadata
	Metadata map[string]string
}

// SessionRegistry manages session configurations.
type SessionRegistry interface {
	// Register registers a new session.
	Register(config SessionConfig) error

	// Get retrieves session configuration by ID.
	Get(sessionID string) (*SessionConfig, error)

	// Update updates session configuration.
	Update(sessionID string, config SessionConfig) error

	// Remove removes a session.
	Remove(sessionID string) error

	// Exists checks if session exists.
	Exists(sessionID string) bool

	// List returns all session IDs.
	List() []string

	// Count returns number of sessions.
	Count() int

	// Clear removes all sessions.
	Clear() error

	// Touch updates LastActivity for a session.
	Touch(sessionID string) error

	// GetExpired returns expired session IDs.
	GetExpired(timeout time.Duration) []string
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
