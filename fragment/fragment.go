// Package fragment defines the Fragment interface for data fragmentation and reassembly.
//
// Fragment is responsible for splitting large data into smaller fragments
// and reassembling them back into original data.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.5):
// - Fragment MUST NOT handle data serialization
// - Fragment MUST NOT handle data encoding/encryption
// - Fragment MUST NOT handle data transmission
// - FragmentInfo.ID CAN be exposed (random UUID, no semantic info)
// - FragmentInfo.Index/Total CAN be exposed
// - Fragment details MUST NOT be exposed
package fragment

import (
	"errors"
	"sync"
	"time"
)

// Common fragment errors
var (
	// ErrInvalidFragmentSize indicates invalid fragment size
	ErrInvalidFragmentSize = errors.New("fragment: invalid fragment size")
	// ErrInvalidFragmentInfo indicates invalid fragment info
	ErrInvalidFragmentInfo = errors.New("fragment: invalid fragment info")
	// ErrFragmentIncomplete indicates fragment group is incomplete
	ErrFragmentIncomplete = errors.New("fragment: incomplete")
	// ErrFragmentMismatch indicates fragment ID mismatch
	ErrFragmentMismatch = errors.New("fragment: mismatch")
	// ErrFragmentTimeout indicates reassembly timeout
	ErrFragmentTimeout = errors.New("fragment: timeout")
	// ErrFragmentCorrupted indicates fragment data is corrupted
	ErrFragmentCorrupted = errors.New("fragment: corrupted")
	// ErrFragmentMissing indicates some fragments are missing
	ErrFragmentMissing = errors.New("fragment: missing")
	// ErrFragmentFailed indicates fragmentation failed
	ErrFragmentFailed = errors.New("fragment: fragmentation failed")
	// ErrReassemblyFailed indicates reassembly failed
	ErrReassemblyFailed = errors.New("fragment: reassembly failed")
	// ErrStateNotFound indicates fragment state not found
	ErrStateNotFound = errors.New("fragment: state not found")
)

// Fragment is the core interface for data fragmentation and reassembly.
//
// Responsibilities:
// - Split large data into smaller fragments (Split)
// - Reassemble fragments back to original data (Reassemble)
// - Manage fragment metadata (GetFragmentInfo, SetFragmentInfo)
//
// NOT Responsible for:
// - Data serialization (handled by Serializer)
// - Data encoding/encryption (handled by Codec)
// - Data transmission (handled by Channel)
type Fragment interface {
	// Split splits data into fragments.
	//
	// Parameter Constraints:
	//   - data: MUST be non-nil byte slice
	//   - maxSize: MUST be >= 64 bytes
	//
	// Return Guarantees:
	//   - Returns fragment list, each fragment <= maxSize
	//   - Fragments maintain original data order
	//   - Each fragment contains FragmentInfo header
	//
	// Error Types:
	//   - ErrInvalidFragmentSize: maxSize is invalid
	//   - ErrFragmentFailed: split failed
	Split(data []byte, maxSize int) ([][]byte, error)

	// Reassemble reassembles fragments into original data.
	//
	// Parameter Constraints:
	//   - fragments: MUST be all fragments of same ID
	//   - Fragments must be complete and in correct order
	//
	// Return Guarantees:
	//   - Returns reconstructed original data
	//
	// Error Types:
	//   - ErrFragmentIncomplete: fragments incomplete
	//   - ErrFragmentMissing: fragments missing
	//   - ErrFragmentCorrupted: fragment corrupted
	//   - ErrFragmentMismatch: fragment ID mismatch
	Reassemble(fragments [][]byte) ([]byte, error)

	// GetFragmentInfo extracts fragment metadata from fragment data.
	//
	// Parameter Constraints:
	//   - fragment: MUST be fragment data with FragmentInfo header
	//
	// Return Guarantees:
	//   - Returns fragment's metadata
	GetFragmentInfo(fragment []byte) (FragmentInfo, error)

	// SetFragmentInfo adds fragment metadata header to data.
	//
	// Return Guarantees:
	//   - Returns fragment data with header
	SetFragmentInfo(data []byte, info FragmentInfo) ([]byte, error)
}

// FragmentInfo contains metadata about a fragment.
// ID, Index, Total CAN be exposed in metadata (random UUID, no semantic info).
type FragmentInfo struct {
	// ID is unique identifier for the fragment group.
	// Format: UUID v4, randomly generated, no semantic information.
	// CAN be exposed in metadata protocols.
	ID string

	// Index is the fragment sequence number (0-based).
	// CAN be exposed in metadata protocols.
	Index uint16

	// Total is the total number of fragments.
	// CAN be exposed in metadata protocols.
	Total uint16

	// Size is the fragment data size (excluding header).
	Size uint32

	// Checksum is CRC32 checksum of fragment data.
	Checksum uint32

	// IsLast indicates whether this is the last fragment.
	IsLast bool
}

// FragmentConfig provides configuration for fragmentation.
type FragmentConfig struct {
	// MaxFragmentSize is maximum fragment size in bytes (default 1024)
	MaxFragmentSize int

	// EnableChecksum enables checksum verification (default true)
	EnableChecksum bool

	// EnableCompression enables compression before fragmentation (default false)
	EnableCompression bool

	// Timeout is reassembly timeout in seconds (default 60)
	Timeout int

	// MaxBufferSize is maximum buffer size for reassembly (default 10MB)
	MaxBufferSize int
}

// DefaultFragmentConfig returns default configuration.
func DefaultFragmentConfig() FragmentConfig {
	return FragmentConfig{
		MaxFragmentSize:   1024,
		EnableChecksum:    true,
		EnableCompression: false,
		Timeout:           60,
		MaxBufferSize:     10 * 1024 * 1024, // 10MB
	}
}

// FragmentState represents the state of a reassembly operation.
type FragmentState struct {
	// ID is unique identifier for the reassembly
	ID string

	// ReceivedCount is number of fragments received
	ReceivedCount int

	// TotalCount is expected total fragments
	TotalCount int

	// Fragments is map of index to fragment data
	Fragments map[int][]byte

	// IsComplete indicates all fragments received
	IsComplete bool

	// CreatedAt is creation timestamp
	CreatedAt time.Time

	// Deadline is the deadline for completion
	Deadline time.Time
}

// FragmentManager manages the lifecycle of fragment operations.
// Responsible for caching and managing received fragments on receiver side.
type FragmentManager interface {
	// CreateState creates new fragment state for reassembly.
	//
	// Parameter Constraints:
	//   - id: fragment group ID
	//   - totalCount: expected total fragments
	CreateState(id string, totalCount int) error

	// GetState retrieves current fragment state.
	GetState(id string) (*FragmentState, error)

	// AddFragment adds received fragment to reassembly buffer.
	//
	// Parameter Constraints:
	//   - id: fragment group ID
	//   - index: fragment index
	//   - data: fragment data
	//
	// Behavior:
	//   - Automatically verifies Checksum
	//   - Checks index validity
	AddFragment(id string, index int, data []byte) error

	// IsComplete checks if fragment group is complete.
	IsComplete(id string) (bool, error)

	// GetMissingIndices returns missing fragment indices.
	GetMissingIndices(id string) ([]int, error)

	// Reassemble reassembles complete fragment group.
	//
	// Preconditions:
	//   - IsComplete(id) == true
	Reassemble(id string) ([]byte, error)

	// ClearState clears fragment state.
	ClearState(id string) error

	// GetTimeoutIDs returns IDs of timeout fragment groups.
	//
	// Used for cleaning incomplete groups.
	GetTimeoutIDs(timeout time.Duration) []string

	// SetTimeout sets deadline for fragment group.
	SetTimeout(id string, deadline time.Time) error

	// Count returns number of active fragment states.
	Count() int

	// ClearAll clears all fragment states.
	ClearAll() error
}

// DefaultFragmentManager is the default FragmentManager implementation.
type DefaultFragmentManager struct {
	mu     sync.RWMutex
	states map[string]*FragmentState
	config FragmentConfig
}

// NewFragmentManager creates a new FragmentManager.
func NewFragmentManager(config FragmentConfig) *DefaultFragmentManager {
	return &DefaultFragmentManager{
		states: make(map[string]*FragmentState),
		config: config,
	}
}

// CreateState creates new fragment state.
func (m *DefaultFragmentManager) CreateState(id string, totalCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.states[id]; exists {
		return errors.New("fragment: state already exists: " + id)
	}

	timeout := time.Duration(m.config.Timeout) * time.Second
	m.states[id] = &FragmentState{
		ID:         id,
		TotalCount: totalCount,
		Fragments:  make(map[int][]byte),
		CreatedAt:  time.Now(),
		Deadline:   time.Now().Add(timeout),
	}
	return nil
}

// GetState retrieves fragment state.
func (m *DefaultFragmentManager) GetState(id string) (*FragmentState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[id]
	if !exists {
		return nil, ErrStateNotFound
	}
	return state, nil
}

// AddFragment adds fragment to state.
func (m *DefaultFragmentManager) AddFragment(id string, index int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[id]
	if !exists {
		return ErrStateNotFound
	}

	if index < 0 || index >= state.TotalCount {
		return ErrInvalidFragmentInfo
	}

	// Check if already received
	if _, exists := state.Fragments[index]; exists {
		return nil // Already have this fragment, ignore
	}

	state.Fragments[index] = data
	state.ReceivedCount++

	// Check if complete
	if state.ReceivedCount == state.TotalCount {
		state.IsComplete = true
	}

	return nil
}

// IsComplete checks if fragment group is complete.
func (m *DefaultFragmentManager) IsComplete(id string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[id]
	if !exists {
		return false, ErrStateNotFound
	}
	return state.IsComplete, nil
}

// GetMissingIndices returns missing fragment indices.
func (m *DefaultFragmentManager) GetMissingIndices(id string) ([]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[id]
	if !exists {
		return nil, ErrStateNotFound
	}

	missing := make([]int, 0)
	for i := 0; i < state.TotalCount; i++ {
		if _, exists := state.Fragments[i]; !exists {
			missing = append(missing, i)
		}
	}
	return missing, nil
}

// Reassemble reassembles fragments.
func (m *DefaultFragmentManager) Reassemble(id string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[id]
	if !exists {
		return nil, ErrStateNotFound
	}

	if !state.IsComplete {
		return nil, ErrFragmentIncomplete
	}

	// Assemble in order
	result := make([]byte, 0)
	for i := 0; i < state.TotalCount; i++ {
		data, exists := state.Fragments[i]
		if !exists {
			return nil, ErrFragmentMissing
		}
		result = append(result, data...)
	}

	// Clean up state
	delete(m.states, id)

	return result, nil
}

// ClearState clears fragment state.
func (m *DefaultFragmentManager) ClearState(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.states, id)
	return nil
}

// GetTimeoutIDs returns timeout IDs.
func (m *DefaultFragmentManager) GetTimeoutIDs(timeout time.Duration) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	timeoutIDs := make([]string, 0)

	for id, state := range m.states {
		if now.Sub(state.CreatedAt) > timeout {
			timeoutIDs = append(timeoutIDs, id)
		}
	}
	return timeoutIDs
}

// SetTimeout sets deadline.
func (m *DefaultFragmentManager) SetTimeout(id string, deadline time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[id]
	if !exists {
		return ErrStateNotFound
	}
	state.Deadline = deadline
	return nil
}

// Count returns number of active states.
func (m *DefaultFragmentManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.states)
}

// ClearAll clears all states.
func (m *DefaultFragmentManager) ClearAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states = make(map[string]*FragmentState)
	return nil
}

// FragmentModule is the interface for fragment module registration.
type FragmentModule interface {
	// Create creates a Fragment instance.
	Create(config FragmentConfig) (Fragment, error)

	// Name returns the module name.
	Name() string
}

// FragmentRegistry manages registered fragments.
type FragmentRegistry struct {
	modules map[string]FragmentModule
}

// NewFragmentRegistry creates a new FragmentRegistry.
func NewFragmentRegistry() *FragmentRegistry {
	return &FragmentRegistry{
		modules: make(map[string]FragmentModule),
	}
}

// Register registers a fragment module.
func (r *FragmentRegistry) Register(module FragmentModule) error {
	if module == nil {
		return errors.New("fragment: cannot register nil module")
	}
	r.modules[module.Name()] = module
	return nil
}

// Get retrieves a Fragment instance.
func (r *FragmentRegistry) Get(name string, config FragmentConfig) (Fragment, error) {
	module, exists := r.modules[name]
	if !exists {
		return nil, errors.New("fragment: not found: " + name)
	}
	return module.Create(config)
}

// Global registry
var globalRegistry = NewFragmentRegistry()

// Register registers a module to the global registry.
func Register(module FragmentModule) error {
	return globalRegistry.Register(module)
}

// Get retrieves a Fragment from the global registry.
func Get(name string, config FragmentConfig) (Fragment, error) {
	return globalRegistry.Get(name, config)
}

// GlobalRegistry returns the global registry.
func GlobalRegistry() *FragmentRegistry {
	return globalRegistry
}
