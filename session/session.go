// Package session provides session management for VoidBus v2.0.
package session

import (
	"sync"
	"time"
)

// SessionState represents the state of a session.
type SessionState int

const (
	// StateCreated indicates session just created.
	StateCreated SessionState = iota

	// StateSending indicates session is sending fragments.
	StateSending

	// StateWaitingACK indicates session waiting for END_ACK.
	StateWaitingACK

	// StateCompleted indicates session completed successfully.
	StateCompleted

	// StateExpired indicates session expired/timeout.
	StateExpired
)

// String returns string representation of state.
func (s SessionState) String() string {
	switch s {
	case StateCreated:
		return "Created"
	case StateSending:
		return "Sending"
	case StateWaitingACK:
		return "WaitingACK"
	case StateCompleted:
		return "Completed"
	case StateExpired:
		return "Expired"
	default:
		return "Unknown"
	}
}

// Session represents a data transmission session.
type Session struct {
	mu sync.RWMutex

	// Identity
	ID        string    // Session UUID
	CreatedAt time.Time // Creation time
	UpdatedAt time.Time // Last update time

	// State
	State SessionState

	// Codec Info (for decoding)
	CodecCodes []string // Codec代号组合
	CodecHash  [32]byte // Codec链Hash
	CodecDepth int      // Codec链深度

	// Fragment Info
	TotalFragments uint16 // 总分片数
	SentFragments  uint16 // 已发送分片数

	// Retransmit
	Retransmits   int // 重传次数
	MaxRetransmit int // 最大重传次数

	// Timing
	Timeout time.Duration // Session超时时间

	// Data Hash (for verification)
	DataHash [32]byte // 原始数据Hash
}

// NewSession creates a new session.
func NewSession(id string, codecCodes []string, codecHash [32]byte, codecDepth int, dataHash [32]byte) *Session {
	return &Session{
		ID:            id,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		State:         StateCreated,
		CodecCodes:    codecCodes,
		CodecHash:     codecHash,
		CodecDepth:    codecDepth,
		DataHash:      dataHash,
		MaxRetransmit: 3,
		Timeout:       60 * time.Second,
	}
}

// SetTotalFragments sets total fragment count.
func (s *Session) SetTotalFragments(total uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalFragments = total
	s.UpdatedAt = time.Now()
}

// IncrementSent increments sent fragment count.
func (s *Session) IncrementSent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SentFragments++
	s.UpdatedAt = time.Now()

	// Update state if all sent
	if s.SentFragments == s.TotalFragments && s.State == StateSending {
		s.State = StateWaitingACK
	}
}

// IncrementRetransmit increments retransmit count.
func (s *Session) IncrementRetransmit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Retransmits++
	s.UpdatedAt = time.Now()

	// Check if exceeded max
	if s.Retransmits > s.MaxRetransmit {
		s.State = StateExpired
		return false
	}
	return true
}

// MarkSending marks session as sending.
func (s *Session) MarkSending() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = StateSending
	s.UpdatedAt = time.Now()
}

// MarkCompleted marks session as completed.
func (s *Session) MarkCompleted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = StateCompleted
	s.UpdatedAt = time.Now()
}

// MarkExpired marks session as expired.
func (s *Session) MarkExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = StateExpired
	s.UpdatedAt = time.Now()
}

// GetState returns current state.
func (s *Session) GetState() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// IsComplete returns whether session is completed.
func (s *Session) IsComplete() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State == StateCompleted
}

// IsExpired returns whether session is expired.
func (s *Session) IsExpired() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State == StateExpired || time.Since(s.UpdatedAt) > s.Timeout
}

// IsTimeout returns whether session is timeout.
func (s *Session) IsTimeout() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.UpdatedAt) > s.Timeout
}

// GetCodecInfo returns codec info.
func (s *Session) GetCodecInfo() (codes []string, hash [32]byte, depth int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CodecCodes, s.CodecHash, s.CodecDepth
}

// GetDataHash returns data hash.
func (s *Session) GetDataHash() [32]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.DataHash
}

// GetProgress returns sending progress.
func (s *Session) GetProgress() (sent uint16, total uint16) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SentFragments, s.TotalFragments
}

// GetRetransmitCount returns retransmit count.
func (s *Session) GetRetransmitCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Retransmits
}

// GetAge returns session age.
func (s *Session) GetAge() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.CreatedAt)
}

// SetTimeout sets session timeout.
func (s *Session) SetTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Timeout = timeout
}

// SetMaxRetransmit sets max retransmit count.
func (s *Session) SetMaxRetransmit(max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MaxRetransmit = max
}

// Stats returns session statistics.
func (s *Session) Stats() SessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SessionStats{
		ID:             s.ID,
		State:          s.State.String(),
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
		TotalFragments: s.TotalFragments,
		SentFragments:  s.SentFragments,
		Retransmits:    s.Retransmits,
		Age:            time.Since(s.CreatedAt),
	}
}

// SessionStats holds session statistics.
type SessionStats struct {
	ID             string
	State          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	TotalFragments uint16
	SentFragments  uint16
	Retransmits    int
	Age            time.Duration
}

// SessionType indicates whether this is a send or receive session.
type SessionType int

const (
	// SendSession indicates sender-side session.
	SendSession SessionType = iota

	// RecvSession indicates receiver-side session.
	RecvSession
)
