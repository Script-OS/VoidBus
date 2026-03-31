// Package session provides SessionManager for VoidBus v2.0.
package session

import (
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/internal"
)

// SessionManagerConfig holds configuration for SessionManager.
type SessionManagerConfig struct {
	MaxSessions    int           // 最大Session数 (默认: 10000)
	SessionTimeout time.Duration // Session超时 (默认: 60s)
	GCInterval     time.Duration // GC清理间隔 (默认: 10s)
	MaxRetransmit  int           // 最大重传次数 (默认: 3)
}

// DefaultSessionManagerConfig returns default configuration.
func DefaultSessionManagerConfig() SessionManagerConfig {
	return SessionManagerConfig{
		MaxSessions:    10000,
		SessionTimeout: 60 * time.Second,
		GCInterval:     10 * time.Second,
		MaxRetransmit:  3,
	}
}

// SessionManager manages all sessions.
type SessionManager struct {
	mu     sync.RWMutex
	config SessionManagerConfig

	// Send sessions (sender side)
	sendSessions map[string]*Session

	// Receive sessions (receiver side)
	recvSessions map[string]*Session

	// GC stop signal
	stopGC chan struct{}
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager(config SessionManagerConfig) *SessionManager {
	mgr := &SessionManager{
		config:       config,
		sendSessions: make(map[string]*Session),
		recvSessions: make(map[string]*Session),
		stopGC:       make(chan struct{}),
	}

	// Start GC goroutine
	go mgr.gcLoop()

	return mgr
}

// gcLoop periodically cleans up expired sessions.
func (m *SessionManager) gcLoop() {
	ticker := time.NewTicker(m.config.GCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.CleanupExpired()
		case <-m.stopGC:
			return
		}
	}
}

// Stop stops the manager and GC loop.
func (m *SessionManager) Stop() {
	close(m.stopGC)
}

// === Send Session Operations ===

// CreateSendSession creates a new send session.
func (m *SessionManager) CreateSendSession(codecCodes []string, codecHash [32]byte, codecDepth int, dataHash [32]byte) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate session ID
	sessionID := internal.GenerateID()

	session := NewSession(sessionID, codecCodes, codecHash, codecDepth, dataHash)
	session.SetTimeout(m.config.SessionTimeout)
	session.SetMaxRetransmit(m.config.MaxRetransmit)
	session.MarkSending()

	m.sendSessions[sessionID] = session
	return session
}

// GetSendSession retrieves a send session.
func (m *SessionManager) GetSendSession(sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sendSessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// CompleteSendSession marks send session as completed.
func (m *SessionManager) CompleteSendSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sendSessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	session.MarkCompleted()
	delete(m.sendSessions, sessionID)
	return nil
}

// RemoveSendSession removes a send session.
func (m *SessionManager) RemoveSendSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sendSessions, sessionID)
	return nil
}

// === Receive Session Operations ===

// CreateRecvSession creates a new receive session.
func (m *SessionManager) CreateRecvSession(sessionID string, codecCodes []string, codecHash [32]byte, codecDepth int, dataHash [32]byte) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := NewSession(sessionID, codecCodes, codecHash, codecDepth, dataHash)
	session.SetTimeout(m.config.SessionTimeout)

	m.recvSessions[sessionID] = session
	return session
}

// GetRecvSession retrieves a receive session.
func (m *SessionManager) GetRecvSession(sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.recvSessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// CompleteRecvSession marks receive session as completed.
func (m *SessionManager) CompleteRecvSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.recvSessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	session.MarkCompleted()
	delete(m.recvSessions, sessionID)
	return nil
}

// RemoveRecvSession removes a receive session.
func (m *SessionManager) RemoveRecvSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.recvSessions, sessionID)
	return nil
}

// === Lookup Operations ===

// GetSession retrieves any session by ID (send or recv).
func (m *SessionManager) GetSession(sessionID string) (*Session, SessionType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check send sessions first
	if session, exists := m.sendSessions[sessionID]; exists {
		return session, SendSession, nil
	}

	// Check receive sessions
	if session, exists := m.recvSessions[sessionID]; exists {
		return session, RecvSession, nil
	}

	return nil, SendSession, ErrSessionNotFound
}

// Exists checks if a session exists.
func (m *SessionManager) Exists(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, existsSend := m.sendSessions[sessionID]
	_, existsRecv := m.recvSessions[sessionID]
	return existsSend || existsRecv
}

// === Cleanup ===

// CleanupExpired removes expired sessions.
func (m *SessionManager) CleanupExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleaned := 0

	// Clean expired send sessions
	for id, session := range m.sendSessions {
		if session.IsExpired() || session.IsTimeout() {
			delete(m.sendSessions, id)
			cleaned++
		}
	}

	// Clean expired receive sessions
	for id, session := range m.recvSessions {
		if session.IsExpired() || session.IsTimeout() {
			delete(m.recvSessions, id)
			cleaned++
		}
	}

	return cleaned
}

// ClearAll clears all sessions.
func (m *SessionManager) ClearAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendSessions = make(map[string]*Session)
	m.recvSessions = make(map[string]*Session)
	return nil
}

// === Statistics ===

// Stats returns manager statistics.
func (m *SessionManager) Stats() SessionManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeSend := 0
	activeRecv := 0
	completed := 0
	expired := 0

	for _, s := range m.sendSessions {
		switch s.GetState() {
		case StateSending, StateWaitingACK:
			activeSend++
		case StateCompleted:
			completed++
		case StateExpired:
			expired++
		}
	}

	for _, s := range m.recvSessions {
		switch s.GetState() {
		case StateCreated, StateSending:
			activeRecv++
		case StateCompleted:
			completed++
		case StateExpired:
			expired++
		}
	}

	return SessionManagerStats{
		ActiveSendSessions: activeSend,
		ActiveRecvSessions: activeRecv,
		CompletedSessions:  completed,
		ExpiredSessions:    expired,
		TotalSendSessions:  len(m.sendSessions),
		TotalRecvSessions:  len(m.recvSessions),
	}
}

// SessionManagerStats holds manager statistics.
type SessionManagerStats struct {
	ActiveSendSessions int
	ActiveRecvSessions int
	CompletedSessions  int
	ExpiredSessions    int
	TotalSendSessions  int
	TotalRecvSessions  int
}

// === Count ===

// Count returns total number of sessions.
func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sendSessions) + len(m.recvSessions)
}

// SendSessionCount returns number of send sessions.
func (m *SessionManager) SendSessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sendSessions)
}

// RecvSessionCount returns number of receive sessions.
func (m *SessionManager) RecvSessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.recvSessions)
}

// ListSendSessions returns all send session IDs.
func (m *SessionManager) ListSendSessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.sendSessions))
	for id := range m.sendSessions {
		ids = append(ids, id)
	}
	return ids
}

// ListRecvSessions returns all receive session IDs.
func (m *SessionManager) ListRecvSessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.recvSessions))
	for id := range m.recvSessions {
		ids = append(ids, id)
	}
	return ids
}

// Error definitions
var (
	ErrSessionNotFound        = errorf("session not found")
	ErrSessionExpired         = errorf("session expired")
	ErrSessionAlreadyComplete = errorf("session already complete")
)

func errorf(msg string) error {
	return &SessionError{Msg: msg}
}

// SessionError represents a session error.
type SessionError struct {
	Msg string
}

func (e *SessionError) Error() string {
	return e.Msg
}
