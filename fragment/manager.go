// Package fragment provides v2.0 FragmentManager with adaptive splitting.
package fragment

import (
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/internal"
)

// FragmentConfig provides configuration for v2.0 fragmentation.
type FragmentConfig struct {
	MinMTU             int           // 最小MTU (默认: 64)
	MaxMTU             int           // 最大MTU (默认: 65535)
	DefaultMTU         int           // 默认MTU (默认: 1024)
	FragmentTimeout    time.Duration // 分片重组超时 (默认: 30s)
	MaxPendingSessions int           // 最大待重组Session数 (默认: 1000)
	GCInterval         time.Duration // GC清理间隔 (默认: 10s)
	HeaderOverhead     int           // Header开销字节 (默认: 64)
}

// DefaultFragmentConfig returns default v2.0 configuration.
func DefaultFragmentConfig() FragmentConfig {
	return FragmentConfig{
		MinMTU:             64,
		MaxMTU:             65535,
		DefaultMTU:         1024,
		FragmentTimeout:    30 * time.Second,
		MaxPendingSessions: 1000,
		GCInterval:         10 * time.Second,
		HeaderOverhead:     64, // V2Header大约64字节
	}
}

// FragmentManager manages v2.0 fragmentation with adaptive splitting.
type FragmentManager struct {
	mu     sync.RWMutex
	config FragmentConfig

	// Send buffers (sender side)
	sendBuffers map[string]*SendBuffer

	// Receive buffers (receiver side)
	recvBuffers map[string]*RecvBuffer

	// GC stop signal
	stopGC chan struct{}
}

// NewFragmentManager creates a new FragmentManager.
func NewFragmentManager(config FragmentConfig) *FragmentManager {
	mgr := &FragmentManager{
		config:      config,
		sendBuffers: make(map[string]*SendBuffer),
		recvBuffers: make(map[string]*RecvBuffer),
		stopGC:      make(chan struct{}),
	}

	// Start GC goroutine
	go mgr.gcLoop()

	return mgr
}

// Name returns the module name (implements Module interface).
func (m *FragmentManager) Name() string {
	return "FragmentManager"
}

// ModuleStats returns module statistics (implements Module interface).
func (m *FragmentManager) ModuleStats() interface{} {
	return m.Stats()
}

// gcLoop periodically cleans up expired buffers.
func (m *FragmentManager) gcLoop() {
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
func (m *FragmentManager) Stop() {
	close(m.stopGC)
}

// === Send Buffer Operations ===

// CreateSendBuffer creates a new send buffer for a session.
func (m *FragmentManager) CreateSendBuffer(sessionID string, data []byte) *SendBuffer {
	m.mu.Lock()
	defer m.mu.Unlock()

	buf := NewSendBuffer(sessionID, data)
	m.sendBuffers[sessionID] = buf
	return buf
}

// GetSendBuffer retrieves a send buffer.
func (m *FragmentManager) GetSendBuffer(sessionID string) (*SendBuffer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	buf, exists := m.sendBuffers[sessionID]
	if !exists {
		return nil, ErrStateNotFound
	}
	return buf, nil
}

// RemoveSendBuffer removes a send buffer.
func (m *FragmentManager) RemoveSendBuffer(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sendBuffers, sessionID)
	return nil
}

// === Receive Buffer Operations ===

// CreateRecvBuffer creates a new receive buffer for a session.
func (m *FragmentManager) CreateRecvBuffer(sessionID string, total uint16, codecDepth uint8, codecHash [32]byte, dataHash [32]byte) *RecvBuffer {
	m.mu.Lock()
	defer m.mu.Unlock()

	buf := NewRecvBuffer(sessionID, total, codecDepth, codecHash, dataHash)
	m.recvBuffers[sessionID] = buf
	return buf
}

// GetRecvBuffer retrieves a receive buffer.
func (m *FragmentManager) GetRecvBuffer(sessionID string) (*RecvBuffer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	buf, exists := m.recvBuffers[sessionID]
	if !exists {
		return nil, ErrStateNotFound
	}
	return buf, nil
}

// AddFragmentToRecv adds a fragment to receive buffer.
func (m *FragmentManager) AddFragmentToRecv(sessionID string, index uint16, data []byte, checksum uint32) (bool, error) {
	buf, err := m.GetRecvBuffer(sessionID)
	if err != nil {
		return false, err
	}
	return buf.AddFragment(index, data, checksum), nil
}

// RemoveRecvBuffer removes a receive buffer.
func (m *FragmentManager) RemoveRecvBuffer(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.recvBuffers, sessionID)
	return nil
}

// === Adaptive Splitting ===

// AdaptiveSplit splits data adaptively based on MTU.
// Returns fragments with V2Header overhead accounted.
func (m *FragmentManager) AdaptiveSplit(data []byte, mtu int) ([][]byte, []uint32, error) {
	// Validate MTU
	if mtu < m.config.MinMTU {
		mtu = m.config.MinMTU
	}
	if mtu > m.config.MaxMTU {
		mtu = m.config.MaxMTU
	}

	// Calculate effective data size per fragment
	effectiveMTU := mtu - m.config.HeaderOverhead
	if effectiveMTU < 1 {
		return nil, nil, ErrInvalidFragmentSize
	}

	// Split data
	totalSize := len(data)
	fragmentCount := (totalSize + effectiveMTU - 1) / effectiveMTU

	fragments := make([][]byte, fragmentCount)
	checksums := make([]uint32, fragmentCount)

	for i := 0; i < fragmentCount; i++ {
		start := i * effectiveMTU
		end := start + effectiveMTU
		if end > totalSize {
			end = totalSize
		}

		fragments[i] = data[start:end]
		checksums[i] = internal.CalculateChecksum(fragments[i])
	}

	return fragments, checksums, nil
}

// Reassemble reassembles fragments from a complete receive buffer.
func (m *FragmentManager) Reassemble(sessionID string) ([]byte, error) {
	buf, err := m.GetRecvBuffer(sessionID)
	if err != nil {
		return nil, err
	}

	data, err := buf.Reassemble()
	if err != nil {
		return nil, err
	}

	// Verify data hash
	if !buf.VerifyDataHash(data) {
		return nil, ErrFragmentCorrupted
	}

	// Clean up buffer
	m.RemoveRecvBuffer(sessionID)

	return data, nil
}

// === NAK Handling ===

// GetMissingFragments returns missing fragments for a session.
func (m *FragmentManager) GetMissingFragments(sessionID string) ([]uint16, error) {
	buf, err := m.GetRecvBuffer(sessionID)
	if err != nil {
		return nil, err
	}
	return buf.GetMissing(), nil
}

// GetRetransmitFragments returns fragment data for retransmission.
func (m *FragmentManager) GetRetransmitFragments(sessionID string, indices []uint16) ([]*FragmentEntry, error) {
	buf, err := m.GetSendBuffer(sessionID)
	if err != nil {
		return nil, err
	}
	return buf.GetMissingFragments(indices), nil
}

// === Session Completion ===

// CompleteSendSession marks a send session as complete and removes buffer.
func (m *FragmentManager) CompleteSendSession(sessionID string) error {
	buf, err := m.GetSendBuffer(sessionID)
	if err != nil {
		return err
	}

	buf.MarkComplete()
	m.RemoveSendBuffer(sessionID)
	return nil
}

// CompleteRecvSession marks a receive session as complete.
func (m *FragmentManager) CompleteRecvSession(sessionID string) bool {
	buf, err := m.GetRecvBuffer(sessionID)
	if err != nil {
		return false
	}
	return buf.IsComplete()
}

// === Cleanup ===

// CleanupExpired removes expired buffers.
// IMPORTANT: Uses two-phase cleanup to minimize lock contention:
// 1. RLock: Quickly identify expired IDs (fast read)
// 2. Lock: Batch delete (brief write)
func (m *FragmentManager) CleanupExpired() int {
	now := time.Now()

	// Phase 1: Identify expired IDs under read lock (fast)
	m.mu.RLock()
	expiredSendIDs := make([]string, 0)
	for id, buf := range m.sendBuffers {
		if buf.IsExpired() || now.Sub(buf.SentTime) > m.config.FragmentTimeout*2 {
			expiredSendIDs = append(expiredSendIDs, id)
		}
	}

	expiredRecvIDs := make([]string, 0)
	for id, buf := range m.recvBuffers {
		if buf.IsExpired() || now.Sub(buf.GetLastActivity()) > m.config.FragmentTimeout {
			expiredRecvIDs = append(expiredRecvIDs, id)
		}
	}
	m.mu.RUnlock()

	// Phase 2: Delete under write lock (brief)
	m.mu.Lock()
	cleaned := 0
	for _, id := range expiredSendIDs {
		if _, exists := m.sendBuffers[id]; exists {
			delete(m.sendBuffers, id)
			cleaned++
		}
	}
	for _, id := range expiredRecvIDs {
		if _, exists := m.recvBuffers[id]; exists {
			delete(m.recvBuffers, id)
			cleaned++
		}
	}
	m.mu.Unlock()

	return cleaned
}

// === Statistics ===

// Stats returns manager statistics.
func (m *FragmentManager) Stats() FragmentStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeSend := 0
	activeRecv := 0

	for _, buf := range m.sendBuffers {
		if !buf.IsComplete() && !buf.IsExpired() {
			activeSend++
		}
	}

	for _, buf := range m.recvBuffers {
		if !buf.IsComplete() && !buf.IsExpired() {
			activeRecv++
		}
	}

	return FragmentStats{
		ActiveSendBuffers: activeSend,
		ActiveRecvBuffers: activeRecv,
		TotalSendBuffers:  len(m.sendBuffers),
		TotalRecvBuffers:  len(m.recvBuffers),
	}
}

// FragmentStats holds manager statistics.
type FragmentStats struct {
	ActiveSendBuffers int
	ActiveRecvBuffers int
	TotalSendBuffers  int
	TotalRecvBuffers  int
}

// === Count ===

// Count returns total number of pending buffers.
func (m *FragmentManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sendBuffers) + len(m.recvBuffers)
}

// ClearAll clears all buffers.
func (m *FragmentManager) ClearAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendBuffers = make(map[string]*SendBuffer)
	m.recvBuffers = make(map[string]*RecvBuffer)
	return nil
}
