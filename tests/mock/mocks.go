// Package mock provides mock implementations for VoidBus testing.
//
// This package is designed for testing purposes only.
// Mock implementations simulate VoidBus module behaviors without actual functionality.
package mock

import (
	"errors"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/fragment"
	"github.com/Script-OS/VoidBus/internal"
	"github.com/Script-OS/VoidBus/session"
)

// Common mock errors
var (
	ErrMockNotImplemented = errors.New("mock: not implemented")
	ErrMockFailure        = errors.New("mock: simulated failure")
)

// ============================================================================
// MockCodecManager
// ============================================================================

// MockCodecManager simulates CodecManager behavior for testing.
type MockCodecManager struct {
	mu sync.RWMutex

	// Registered codecs
	codecs map[string]codec.Codec

	// Configuration
	maxDepth int

	// Negotiation state
	negotiated bool

	// Behavior control
	FailAddCodec      bool
	FailRandomSelect  bool
	FailMatchByHash   bool
	FailNegotiate     bool
	RandomSelectError error
	MatchError        error
}

// NewMockCodecManager creates a new MockCodecManager.
func NewMockCodecManager() *MockCodecManager {
	return &MockCodecManager{
		codecs:   make(map[string]codec.Codec),
		maxDepth: 2,
	}
}

// Name returns the module name.
func (m *MockCodecManager) Name() string {
	return "MockCodecManager"
}

// Stop stops the mock manager.
func (m *MockCodecManager) Stop() error {
	return nil
}

// ModuleStats returns module statistics.
func (m *MockCodecManager) ModuleStats() interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]interface{}{
		"codec_count": len(m.codecs),
		"max_depth":   m.maxDepth,
		"negotiated":  m.negotiated,
	}
}

// AddCodec adds a codec with the given code.
func (m *MockCodecManager) AddCodec(c codec.Codec, code string) error {
	if m.FailAddCodec {
		return ErrMockFailure
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.codecs[code] = c
	return nil
}

// SetMaxDepth sets maximum codec chain depth.
func (m *MockCodecManager) SetMaxDepth(depth int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxDepth = depth
	return nil
}

// SetSalt sets salt for hash computation (mock: no effect).
func (m *MockCodecManager) SetSalt(salt []byte) {}

// RandomSelect randomly selects codec chain (mock: returns first registered codec).
func (m *MockCodecManager) RandomSelect() ([]string, codec.CodecChain, error) {
	if m.FailRandomSelect {
		if m.RandomSelectError != nil {
			return nil, nil, m.RandomSelectError
		}
		return nil, nil, ErrMockFailure
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.codecs) == 0 {
		return nil, nil, ErrMockFailure
	}

	// Return first registered codec
	for code := range m.codecs {
		return []string{code}, codec.NewChain(), nil
	}

	return nil, nil, ErrMockFailure
}

// MatchByHash matches codec chain by hash (mock: always fails unless configured).
func (m *MockCodecManager) MatchByHash(hash [32]byte) ([]string, codec.CodecChain, error) {
	if m.FailMatchByHash {
		if m.MatchError != nil {
			return nil, nil, m.MatchError
		}
		return nil, nil, ErrMockFailure
	}

	// Mock: return empty chain
	return []string{}, codec.NewChain(), nil
}

// ComputeHash computes hash for codec codes (mock: returns zero hash).
func (m *MockCodecManager) ComputeHash(codes []string) [32]byte {
	return [32]byte{}
}

// Negotiate performs capability negotiation (mock: returns all codes).
func (m *MockCodecManager) Negotiate(remoteCodes []string, remoteMaxDepth int, salt []byte) ([]string, error) {
	if m.FailNegotiate {
		return nil, ErrMockFailure
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.negotiated = true

	// Return intersection (mock: return all remote codes)
	return remoteCodes, nil
}

// GetSupportedCodes returns supported codec codes.
func (m *MockCodecManager) GetSupportedCodes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	codes := make([]string, 0, len(m.codecs))
	for code := range m.codecs {
		codes = append(codes, code)
	}
	return codes
}

// GetMaxDepth returns maximum depth.
func (m *MockCodecManager) GetMaxDepth() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maxDepth
}

// IsNegotiated returns negotiation status.
func (m *MockCodecManager) IsNegotiated() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.negotiated
}

// Stats returns codec manager statistics.
func (m *MockCodecManager) Stats() map[string]interface{} {
	return m.ModuleStats().(map[string]interface{})
}

// ============================================================================
// MockFragmentManager
// ============================================================================

// MockFragmentManager simulates FragmentManager behavior for testing.
type MockFragmentManager struct {
	mu sync.RWMutex

	// Buffers
	sendBuffers map[string]*fragment.SendBuffer
	recvBuffers map[string]*fragment.RecvBuffer

	// Behavior control
	FailAdaptiveSplit bool
	FailReassemble    bool
	FailAddFragment   bool
	SplitResult       [][]byte
	SplitChecksums    []uint32
}

// NewMockFragmentManager creates a new MockFragmentManager.
func NewMockFragmentManager() *MockFragmentManager {
	return &MockFragmentManager{
		sendBuffers: make(map[string]*fragment.SendBuffer),
		recvBuffers: make(map[string]*fragment.RecvBuffer),
	}
}

// Name returns the module name.
func (m *MockFragmentManager) Name() string {
	return "MockFragmentManager"
}

// Stop stops the mock manager.
func (m *MockFragmentManager) Stop() error {
	return nil
}

// ModuleStats returns module statistics.
func (m *MockFragmentManager) ModuleStats() interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]interface{}{
		"send_buffers": len(m.sendBuffers),
		"recv_buffers": len(m.recvBuffers),
	}
}

// CreateSendBuffer creates a send buffer (mock: stores reference).
func (m *MockFragmentManager) CreateSendBuffer(sessionID string, data []byte) *fragment.SendBuffer {
	m.mu.Lock()
	defer m.mu.Unlock()

	buf := fragment.NewSendBuffer(sessionID, data)
	m.sendBuffers[sessionID] = buf
	return buf
}

// GetSendBuffer retrieves a send buffer.
func (m *MockFragmentManager) GetSendBuffer(sessionID string) (*fragment.SendBuffer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	buf, exists := m.sendBuffers[sessionID]
	if !exists {
		return nil, ErrMockFailure
	}
	return buf, nil
}

// RemoveSendBuffer removes a send buffer.
func (m *MockFragmentManager) RemoveSendBuffer(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sendBuffers, sessionID)
	return nil
}

// CreateRecvBuffer creates a receive buffer (mock: stores reference).
func (m *MockFragmentManager) CreateRecvBuffer(sessionID string, total uint16, codecDepth uint8, codecHash [32]byte, dataHash [32]byte) *fragment.RecvBuffer {
	m.mu.Lock()
	defer m.mu.Unlock()

	buf := fragment.NewRecvBuffer(sessionID, total, codecDepth, codecHash, dataHash)
	m.recvBuffers[sessionID] = buf
	return buf
}

// GetRecvBuffer retrieves a receive buffer.
func (m *MockFragmentManager) GetRecvBuffer(sessionID string) (*fragment.RecvBuffer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	buf, exists := m.recvBuffers[sessionID]
	if !exists {
		return nil, ErrMockFailure
	}
	return buf, nil
}

// RemoveRecvBuffer removes a receive buffer.
func (m *MockFragmentManager) RemoveRecvBuffer(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.recvBuffers, sessionID)
	return nil
}

// AdaptiveSplit splits data adaptively (mock: returns configured result or single fragment).
func (m *MockFragmentManager) AdaptiveSplit(data []byte, mtu int) ([][]byte, []uint32, error) {
	if m.FailAdaptiveSplit {
		return nil, nil, ErrMockFailure
	}

	// Use configured result if set
	if m.SplitResult != nil {
		return m.SplitResult, m.SplitChecksums, nil
	}

	// Mock: return data as single fragment
	return [][]byte{data}, []uint32{0}, nil
}

// Reassemble reassembles fragments (mock: returns configured data).
func (m *MockFragmentManager) Reassemble(sessionID string) ([]byte, error) {
	if m.FailReassemble {
		return nil, ErrMockFailure
	}

	m.mu.RLock()
	buf, exists := m.recvBuffers[sessionID]
	m.mu.RUnlock()

	if !exists {
		return nil, ErrMockFailure
	}

	return buf.Reassemble()
}

// GetMissingFragments returns missing fragment indices (mock: returns empty).
func (m *MockFragmentManager) GetMissingFragments(sessionID string) ([]uint16, error) {
	return []uint16{}, nil
}

// CleanupExpired removes expired buffers (mock: returns 0).
func (m *MockFragmentManager) CleanupExpired() int {
	return 0
}

// ClearAll clears all buffers.
func (m *MockFragmentManager) ClearAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendBuffers = make(map[string]*fragment.SendBuffer)
	m.recvBuffers = make(map[string]*fragment.RecvBuffer)
	return nil
}

// Stats returns fragment manager statistics.
func (m *MockFragmentManager) Stats() map[string]interface{} {
	return m.ModuleStats().(map[string]interface{})
}

// ============================================================================
// MockSessionManager
// ============================================================================

// MockSessionManager simulates SessionManager behavior for testing.
type MockSessionManager struct {
	mu sync.RWMutex

	// Sessions
	sendSessions map[string]*session.Session
	recvSessions map[string]*session.Session

	// Behavior control
	FailCreateSession bool
	FailGetSession    bool
}

// NewMockSessionManager creates a new MockSessionManager.
func NewMockSessionManager() *MockSessionManager {
	return &MockSessionManager{
		sendSessions: make(map[string]*session.Session),
		recvSessions: make(map[string]*session.Session),
	}
}

// Name returns the module name.
func (m *MockSessionManager) Name() string {
	return "MockSessionManager"
}

// Stop stops the mock manager.
func (m *MockSessionManager) Stop() error {
	return nil
}

// ModuleStats returns module statistics.
func (m *MockSessionManager) ModuleStats() interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]interface{}{
		"send_sessions": len(m.sendSessions),
		"recv_sessions": len(m.recvSessions),
	}
}

// CreateSendSession creates a send session (mock: returns new session).
func (m *MockSessionManager) CreateSendSession(codecCodes []string, codecHash [32]byte, codecDepth int, dataHash [32]byte) *session.Session {
	if m.FailCreateSession {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sess := session.NewSession(internal.GenerateSessionID(), codecCodes, codecHash, codecDepth, dataHash)
	m.sendSessions[sess.ID] = sess
	return sess
}

// GetSendSession retrieves a send session.
func (m *MockSessionManager) GetSendSession(sessionID string) (*session.Session, error) {
	if m.FailGetSession {
		return nil, ErrMockFailure
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, exists := m.sendSessions[sessionID]
	if !exists {
		return nil, ErrMockFailure
	}
	return sess, nil
}

// CompleteSendSession marks send session as completed.
func (m *MockSessionManager) CompleteSendSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, exists := m.sendSessions[sessionID]; exists {
		sess.MarkCompleted()
	}
	return nil
}

// RemoveSendSession removes a send session.
func (m *MockSessionManager) RemoveSendSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sendSessions, sessionID)
	return nil
}

// CreateRecvSession creates a receive session.
func (m *MockSessionManager) CreateRecvSession(sessionID string, codecCodes []string, codecHash [32]byte, codecDepth int, dataHash [32]byte) *session.Session {
	if m.FailCreateSession {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sess := session.NewSession(sessionID, codecCodes, codecHash, codecDepth, dataHash)
	m.recvSessions[sessionID] = sess
	return sess
}

// GetRecvSession retrieves a receive session.
func (m *MockSessionManager) GetRecvSession(sessionID string) (*session.Session, error) {
	if m.FailGetSession {
		return nil, ErrMockFailure
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, exists := m.recvSessions[sessionID]
	if !exists {
		return nil, ErrMockFailure
	}
	return sess, nil
}

// CompleteRecvSession marks receive session as completed.
func (m *MockSessionManager) CompleteRecvSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, exists := m.recvSessions[sessionID]; exists {
		sess.MarkCompleted()
	}
	return nil
}

// Exists checks if session exists.
func (m *MockSessionManager) Exists(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sendSessions[sessionID] != nil || m.recvSessions[sessionID] != nil
}

// CleanupExpired removes expired sessions (mock: returns 0).
func (m *MockSessionManager) CleanupExpired() int {
	return 0
}

// ClearAll clears all sessions.
func (m *MockSessionManager) ClearAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendSessions = make(map[string]*session.Session)
	m.recvSessions = make(map[string]*session.Session)
	return nil
}

// Stats returns session manager statistics.
func (m *MockSessionManager) Stats() map[string]interface{} {
	return m.ModuleStats().(map[string]interface{})
}

// ============================================================================
// MockChannelPool
// ============================================================================

// MockChannelPool simulates ChannelPool behavior for testing.
type MockChannelPool struct {
	mu sync.RWMutex

	// Channels (mock: just IDs)
	channelIDs map[string]bool

	// Configuration
	defaultMTU int

	// Behavior control
	FailAddChannel    bool
	FailRandomSelect  bool
	FailSelectHealthy bool
	RandomSelectID    string
}

// NewMockChannelPool creates a new MockChannelPool.
func NewMockChannelPool() *MockChannelPool {
	return &MockChannelPool{
		channelIDs: make(map[string]bool),
		defaultMTU: 1024,
	}
}

// Name returns the module name.
func (m *MockChannelPool) Name() string {
	return "MockChannelPool"
}

// Stop stops the mock pool.
func (m *MockChannelPool) Stop() error {
	return nil
}

// ModuleStats returns module statistics.
func (m *MockChannelPool) ModuleStats() interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]interface{}{
		"channel_count": len(m.channelIDs),
		"default_mtu":   m.defaultMTU,
	}
}

// AddChannel adds a channel (mock: just stores ID).
func (m *MockChannelPool) AddChannel(channel interface{}, id string) error {
	if m.FailAddChannel {
		return ErrMockFailure
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.channelIDs[id] = true
	return nil
}

// RemoveChannel removes a channel.
func (m *MockChannelPool) RemoveChannel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.channelIDs, id)
	return nil
}

// SetMTUOverride sets MTU override (mock: no effect).
func (m *MockChannelPool) SetMTUOverride(id string, mtu int) error {
	return nil
}

// SetDefaultMTU sets default MTU.
func (m *MockChannelPool) SetDefaultMTU(mtu int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultMTU = mtu
}

// RandomSelect randomly selects a channel (mock: returns configured ID or first).
func (m *MockChannelPool) RandomSelect() (string, error) {
	if m.FailRandomSelect {
		return "", ErrMockFailure
	}

	// Return configured ID if set
	if m.RandomSelectID != "" {
		return m.RandomSelectID, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for id := range m.channelIDs {
		return id, nil
	}

	return "", ErrMockFailure
}

// SelectHealthy selects healthy channel (mock: same as RandomSelect).
func (m *MockChannelPool) SelectHealthy() (string, error) {
	if m.FailSelectHealthy {
		return "", ErrMockFailure
	}
	return m.RandomSelect()
}

// SelectForMTU selects channel for MTU (mock: same as RandomSelect).
func (m *MockChannelPool) SelectForMTU(dataSize int) (string, error) {
	return m.RandomSelect()
}

// GetAdaptiveMTU returns adaptive MTU (mock: returns default).
func (m *MockChannelPool) GetAdaptiveMTU() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultMTU
}

// RecordSend records send (mock: no effect).
func (m *MockChannelPool) RecordSend(id string, latency time.Duration) {}

// RecordError records error (mock: no effect).
func (m *MockChannelPool) RecordError(id string) {}

// GetHealthScore returns health score (mock: returns 1.0).
func (m *MockChannelPool) GetHealthScore(id string) (float64, error) {
	return 1.0, nil
}

// Count returns number of channels.
func (m *MockChannelPool) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.channelIDs)
}

// GetChannelIDs returns all channel IDs.
func (m *MockChannelPool) GetChannelIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.channelIDs))
	for id := range m.channelIDs {
		ids = append(ids, id)
	}
	return ids
}

// CloseAll closes all channels (mock: clears IDs).
func (m *MockChannelPool) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channelIDs = make(map[string]bool)
	return nil
}

// Stats returns channel pool statistics.
func (m *MockChannelPool) Stats() map[string]interface{} {
	return m.ModuleStats().(map[string]interface{})
}

// ============================================================================
// MockAdaptiveTimer
// ============================================================================

// MockAdaptiveTimer simulates AdaptiveTimeout behavior for testing.
type MockAdaptiveTimer struct {
	mu sync.RWMutex

	// Configuration
	initialTimeout time.Duration
	maxTimeout     time.Duration

	// Behavior control
	FailGetTimeout bool
	TimeoutResult  time.Duration
}

// NewMockAdaptiveTimer creates a new MockAdaptiveTimer.
func NewMockAdaptiveTimer() *MockAdaptiveTimer {
	return &MockAdaptiveTimer{
		initialTimeout: 1 * time.Second,
		maxTimeout:     30 * time.Second,
		TimeoutResult:  5 * time.Second,
	}
}

// Name returns the module name.
func (m *MockAdaptiveTimer) Name() string {
	return "MockAdaptiveTimer"
}

// Stop stops the mock timer.
func (m *MockAdaptiveTimer) Stop() error {
	return nil
}

// ModuleStats returns module statistics.
func (m *MockAdaptiveTimer) ModuleStats() interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]interface{}{
		"initial_timeout": m.initialTimeout.String(),
		"max_timeout":     m.maxTimeout.String(),
	}
}

// GetTimeout returns timeout value (mock: returns configured result).
func (m *MockAdaptiveTimer) GetTimeout() time.Duration {
	if m.FailGetTimeout {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.TimeoutResult
}

// RecordLatency records latency (mock: no effect).
func (m *MockAdaptiveTimer) RecordLatency(latency time.Duration) {}

// Reset resets timer (mock: no effect).
func (m *MockAdaptiveTimer) Reset() {}

// GetSRTT returns smoothed RTT (mock: returns 1s).
func (m *MockAdaptiveTimer) GetSRTT() time.Duration {
	return 1 * time.Second
}

// Stats returns adaptive timer statistics.
func (m *MockAdaptiveTimer) Stats() map[string]interface{} {
	return m.ModuleStats().(map[string]interface{})
}

// ============================================================================
// Helper Functions
// ============================================================================

// MockDependencies creates a complete set of mock dependencies for testing.
func MockDependencies() *MockDependenciesSet {
	return &MockDependenciesSet{
		CodecManager:    NewMockCodecManager(),
		ChannelPool:     NewMockChannelPool(),
		FragmentManager: NewMockFragmentManager(),
		SessionManager:  NewMockSessionManager(),
		AdaptiveTimer:   NewMockAdaptiveTimer(),
	}
}

// MockDependenciesSet provides a convenient set of all mock dependencies.
type MockDependenciesSet struct {
	CodecManager    *MockCodecManager
	ChannelPool     *MockChannelPool
	FragmentManager *MockFragmentManager
	SessionManager  *MockSessionManager
	AdaptiveTimer   *MockAdaptiveTimer
}

// SetFailAll sets all mocks to fail.
func (d *MockDependenciesSet) SetFailAll() {
	d.CodecManager.FailAddCodec = true
	d.CodecManager.FailRandomSelect = true
	d.CodecManager.FailMatchByHash = true
	d.CodecManager.FailNegotiate = true
	d.ChannelPool.FailAddChannel = true
	d.ChannelPool.FailRandomSelect = true
	d.ChannelPool.FailSelectHealthy = true
	d.FragmentManager.FailAdaptiveSplit = true
	d.FragmentManager.FailReassemble = true
	d.SessionManager.FailCreateSession = true
	d.SessionManager.FailGetSession = true
	d.AdaptiveTimer.FailGetTimeout = true
}

// ResetAll resets all mocks to normal behavior.
func (d *MockDependenciesSet) ResetAll() {
	d.CodecManager.FailAddCodec = false
	d.CodecManager.FailRandomSelect = false
	d.CodecManager.FailMatchByHash = false
	d.CodecManager.FailNegotiate = false
	d.ChannelPool.FailAddChannel = false
	d.ChannelPool.FailRandomSelect = false
	d.ChannelPool.FailSelectHealthy = false
	d.FragmentManager.FailAdaptiveSplit = false
	d.FragmentManager.FailReassemble = false
	d.SessionManager.FailCreateSession = false
	d.SessionManager.FailGetSession = false
	d.AdaptiveTimer.FailGetTimeout = false
}
