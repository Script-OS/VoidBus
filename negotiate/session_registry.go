// Package negotiate provides session registry for multi-channel association.
//
// SessionRegistry manages server-side session states for multi-channel connections.
// When a client connects through multiple channels, each channel sends a NegotiateRequest
// with the same SessionID. The registry associates these channels to the same session.
package negotiate

import (
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/internal"
)

// AllChannelBits contains all channel bits for iteration.
var AllChannelBits = []ChannelBit{
	ChannelBitWS, ChannelBitTCP, ChannelBitUDP, ChannelBitICMP,
	ChannelBitDNS, ChannelBitHTTP, ChannelBitReserved, ChannelBitReserved7,
	ChannelBitReserved8, ChannelBitReserved9, ChannelBitReserved10, ChannelBitReserved11,
	ChannelBitReserved12, ChannelBitReserved13, ChannelBitReserved14, ChannelBitReserved15,
}

// SessionState represents the state of a session.
type SessionState struct {
	mu sync.RWMutex

	// Session identification
	SessionID []byte // 8 bytes

	// Bus for this session (created on first connection)
	Bus any // Will be set to *Bus by listener

	// Channel tracking
	ConnectedChannels ChannelBitmap // Channels that have connected
	ExpectedChannels  ChannelBitmap // Channels negotiated (intersection)

	// Codec tracking
	NegotiatedCodecs CodecBitmap // Negotiated codec bitmap

	// State
	Ready      bool          // All expected channels connected
	ReadyChan  chan struct{} // Closed when session becomes ready
	CreateTime time.Time     // Session creation time

	// Channel connections (for associating channels to bus)
	ChannelConnections map[string]any // ChannelID -> Channel (will be set by listener)
}

// IsReady returns true if all expected channels are connected.
func (s *SessionState) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Ready
}

// WaitForReady blocks until the session is ready or timeout.
func (s *SessionState) WaitForReady(timeout time.Duration) bool {
	s.mu.RLock()
	ready := s.Ready
	readyChan := s.ReadyChan
	s.mu.RUnlock()

	if ready {
		return true
	}

	select {
	case <-readyChan:
		return true
	case <-time.After(timeout):
		return false
	}
}

// AddChannel adds a connected channel to the session.
// Returns true if this channel completes the session (makes it ready).
func (s *SessionState) AddChannel(channelType ChannelBit, channelID string, channel any) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Mark channel as connected
	s.ConnectedChannels.SetChannel(channelType)
	s.ChannelConnections[channelID] = channel

	// Check if all expected channels are now connected
	allConnected := true
	for _, chBit := range AllChannelBits {
		if s.ExpectedChannels.HasChannel(chBit) && !s.ConnectedChannels.HasChannel(chBit) {
			allConnected = false
			break
		}
	}

	if allConnected && !s.Ready {
		s.Ready = true
		close(s.ReadyChan)
		return true
	}

	return false
}

// GetChannelCount returns the number of connected channels.
func (s *SessionState) GetChannelCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.ChannelConnections)
}

// HasChannelType returns true if the session already has a channel of the given type.
// This is used to prevent duplicate UDP connections from the same client address.
func (s *SessionState) HasChannelType(channelType ChannelBit) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ConnectedChannels.HasChannel(channelType)
}

// SessionRegistry manages all session states.
type SessionRegistry struct {
	mu sync.RWMutex

	// Sessions indexed by SessionID (hex string for map key)
	sessions map[string]*SessionState

	// Timeout for session readiness
	sessionTimeout time.Duration

	// Cleanup interval
	cleanupInterval time.Duration

	// Stop channel
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// SessionRegistryConfig provides configuration for SessionRegistry.
type SessionRegistryConfig struct {
	SessionTimeout  time.Duration // How long to wait for all channels
	CleanupInterval time.Duration // How often to cleanup stale sessions
	MaxSessionAge   time.Duration // Maximum age of a session before cleanup
}

// DefaultSessionRegistryConfig returns default configuration.
func DefaultSessionRegistryConfig() *SessionRegistryConfig {
	return &SessionRegistryConfig{
		SessionTimeout:  30 * time.Second,
		CleanupInterval: 60 * time.Second,
		MaxSessionAge:   5 * time.Minute,
	}
}

// NewSessionRegistry creates a new session registry.
func NewSessionRegistry(config *SessionRegistryConfig) *SessionRegistry {
	if config == nil {
		config = DefaultSessionRegistryConfig()
	}

	reg := &SessionRegistry{
		sessions:        make(map[string]*SessionState),
		sessionTimeout:  config.SessionTimeout,
		cleanupInterval: config.CleanupInterval,
		stopChan:        make(chan struct{}),
	}

	// Start cleanup goroutine
	reg.wg.Add(1)
	go reg.cleanupLoop(config.MaxSessionAge)

	return reg
}

// sessionIDToString converts SessionID bytes to map key string.
func sessionIDToString(sessionID []byte) string {
	if len(sessionID) == 0 {
		return ""
	}
	return internal.EncodeHex(sessionID)
}

// CreateSession creates a new session for a first-time connection.
// Returns the newly created session state.
func (r *SessionRegistry) CreateSession(sessionID []byte, expectedChannels ChannelBitmap, negotiatedCodecs CodecBitmap) *SessionState {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := sessionIDToString(sessionID)

	// Check if session already exists (shouldn't happen for first connection)
	if existing, ok := r.sessions[key]; ok {
		return existing
	}

	state := &SessionState{
		SessionID:          sessionID,
		ExpectedChannels:   expectedChannels,
		NegotiatedCodecs:   negotiatedCodecs,
		ConnectedChannels:  NewChannelBitmap(0),
		ReadyChan:          make(chan struct{}),
		CreateTime:         time.Now(),
		ChannelConnections: make(map[string]any),
	}

	r.sessions[key] = state
	return state
}

// AssociateSession associates a channel connection to an existing session.
// Returns the session state if found, nil if session doesn't exist.
func (r *SessionRegistry) AssociateSession(sessionID []byte, channelType ChannelBit, channelID string, channel any) *SessionState {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := sessionIDToString(sessionID)

	state, ok := r.sessions[key]
	if !ok {
		return nil
	}

	// Add channel to session
	state.AddChannel(channelType, channelID, channel)

	return state
}

// GetSession returns the session state for a given SessionID.
func (r *SessionRegistry) GetSession(sessionID []byte) *SessionState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := sessionIDToString(sessionID)
	return r.sessions[key]
}

// HasSession returns true if a session exists for the given SessionID.
func (r *SessionRegistry) HasSession(sessionID []byte) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := sessionIDToString(sessionID)
	return r.sessions[key] != nil
}

// RemoveSession removes a session from the registry.
func (r *SessionRegistry) RemoveSession(sessionID []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := sessionIDToString(sessionID)
	if state, ok := r.sessions[key]; ok {
		// Close readyChan if not already closed
		state.mu.Lock()
		if !state.Ready {
			close(state.ReadyChan)
		}
		state.mu.Unlock()

		delete(r.sessions, key)
	}
}

// GetReadySessions returns all sessions that are ready.
func (r *SessionRegistry) GetReadySessions() []*SessionState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ready := make([]*SessionState, 0)
	for _, state := range r.sessions {
		if state.IsReady() {
			ready = append(ready, state)
		}
	}
	return ready
}

// GetPendingSessions returns all sessions that are not yet ready.
func (r *SessionRegistry) GetPendingSessions() []*SessionState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pending := make([]*SessionState, 0)
	for _, state := range r.sessions {
		if !state.IsReady() {
			pending = append(pending, state)
		}
	}
	return pending
}

// WaitForSessionReady waits for a specific session to become ready.
func (r *SessionRegistry) WaitForSessionReady(sessionID []byte, timeout time.Duration) *SessionState {
	state := r.GetSession(sessionID)
	if state == nil {
		return nil
	}

	if state.WaitForReady(timeout) {
		return state
	}
	return nil
}

// cleanupLoop periodically removes stale sessions.
func (r *SessionRegistry) cleanupLoop(maxAge time.Duration) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
			return
		case <-ticker.C:
			r.cleanupStaleSessions(maxAge)
		}
	}
}

// cleanupStaleSessions removes sessions older than maxAge.
func (r *SessionRegistry) cleanupStaleSessions(maxAge time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for key, state := range r.sessions {
		if now.Sub(state.CreateTime) > maxAge {
			// Session is stale, remove it
			state.mu.Lock()
			if !state.Ready {
				close(state.ReadyChan)
			}
			state.mu.Unlock()

			delete(r.sessions, key)
		}
	}
}

// Stop stops the registry and cleanup goroutine.
func (r *SessionRegistry) Stop() {
	close(r.stopChan)
	r.wg.Wait()
}

// Count returns the number of sessions in the registry.
func (r *SessionRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}
