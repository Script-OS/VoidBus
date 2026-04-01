// Package voidbus provides module abstraction for VoidBus v2.0.
package voidbus

// Module is the base interface for all VoidBus modules.
type Module interface {
	// Name returns the module name.
	Name() string

	// Stop stops the module and releases resources.
	Stop() error

	// ModuleStats returns module statistics.
	ModuleStats() interface{}
}

// CodecModule is the abstraction for codec management module.
type CodecModule interface {
	Module

	// === Codec Registration ===
	// AddCodec registers a codec with user-defined code.
	AddCodec(codec interface{}, code string) error

	// SetMaxDepth sets maximum codec chain depth.
	SetMaxDepth(depth int) error

	// SetSalt sets salt for hash computation.
	SetSalt(salt []byte)

	// === Codec Selection ===
	// RandomSelect randomly selects codec chain.
	RandomSelect() (codes []string, chain interface{}, err error)

	// MatchByHash matches codec chain by hash.
	MatchByHash(hash [32]byte) (codes []string, chain interface{}, err error)

	// ComputeHash computes hash for codec codes.
	ComputeHash(codes []string) [32]byte

	// === Negotiation ===
	// Negotiate performs capability negotiation.
	Negotiate(remoteCodes []string, remoteMaxDepth int, salt []byte) ([]string, error)

	// GetSupportedCodes returns supported codec codes.
	GetSupportedCodes() []string

	// GetMaxDepth returns maximum depth.
	GetMaxDepth() int

	// IsNegotiated returns negotiation status.
	IsNegotiated() bool
}

// ChannelModule is the abstraction for channel management module.
type ChannelModule interface {
	Module

	// === Channel Registration ===
	// AddChannel adds a channel to the pool.
	AddChannel(channel interface{}, id string) error

	// RemoveChannel removes a channel from the pool.
	RemoveChannel(id string) error

	// SetMTUOverride sets custom MTU for a channel.
	SetMTUOverride(id string, mtu int) error

	// SetDefaultMTU sets default MTU for all channels.
	SetDefaultMTU(mtu int)

	// === Channel Selection ===
	// RandomSelect randomly selects a channel.
	RandomSelect() (interface{}, error)

	// SelectHealthy selects the healthiest channel.
	SelectHealthy() (interface{}, error)

	// SelectForMTU selects channel suitable for given data size.
	SelectForMTU(dataSize int) (interface{}, error)

	// GetAdaptiveMTU returns adaptive MTU based on all channels.
	GetAdaptiveMTU() int

	// === Health Tracking ===
	// RecordSend records successful send operation.
	RecordSend(id string, latency interface{})

	// RecordError records send error.
	RecordError(id string)

	// GetHealthScore returns health score for a channel.
	GetHealthScore(id string) (float64, error)

	// === Management ===
	// Count returns number of channels.
	Count() int

	// GetChannelIDs returns all channel IDs.
	GetChannelIDs() []string

	// CloseAll closes all channels.
	CloseAll() error
}

// FragmentModule is the abstraction for fragment management module.
type FragmentModule interface {
	Module

	// === Send Buffer ===
	// CreateSendBuffer creates send buffer for a session.
	CreateSendBuffer(sessionID string, data []byte) interface{}

	// GetSendBuffer retrieves send buffer.
	GetSendBuffer(sessionID string) (interface{}, error)

	// RemoveSendBuffer removes send buffer.
	RemoveSendBuffer(sessionID string) error

	// === Receive Buffer ===
	// CreateRecvBuffer creates receive buffer for a session.
	CreateRecvBuffer(sessionID string, total uint16, codecDepth uint8, codecHash [32]byte, dataHash [32]byte) interface{}

	// GetRecvBuffer retrieves receive buffer.
	GetRecvBuffer(sessionID string) (interface{}, error)

	// RemoveRecvBuffer removes receive buffer.
	RemoveRecvBuffer(sessionID string) error

	// === Fragmentation ===
	// AdaptiveSplit splits data adaptively based on MTU.
	AdaptiveSplit(data []byte, mtu int) ([][]byte, []uint32, error)

	// Reassemble reassembles fragments.
	Reassemble(sessionID string) ([]byte, error)

	// === NAK Handling ===
	// GetMissingFragments returns missing fragment indices.
	GetMissingFragments(sessionID string) ([]uint16, error)

	// === Cleanup ===
	// CleanupExpired removes expired buffers.
	CleanupExpired() int

	// ClearAll clears all buffers.
	ClearAll() error
}

// SessionModule is the abstraction for session management module.
type SessionModule interface {
	Module

	// === Send Session ===
	// CreateSendSession creates send session.
	CreateSendSession(codecCodes []string, codecHash [32]byte, codecDepth int, dataHash [32]byte) interface{}

	// GetSendSession retrieves send session.
	GetSendSession(sessionID string) (interface{}, error)

	// CompleteSendSession marks send session as completed.
	CompleteSendSession(sessionID string) error

	// RemoveSendSession removes send session.
	RemoveSendSession(sessionID string) error

	// === Receive Session ===
	// CreateRecvSession creates receive session.
	CreateRecvSession(sessionID string, codecCodes []string, codecHash [32]byte, codecDepth int, dataHash [32]byte) interface{}

	// GetRecvSession retrieves receive session.
	GetRecvSession(sessionID string) (interface{}, error)

	// CompleteRecvSession marks receive session as completed.
	CompleteRecvSession(sessionID string) error

	// === Lookup ===
	// Exists checks if session exists.
	Exists(sessionID string) bool

	// === Cleanup ===
	// CleanupExpired removes expired sessions.
	CleanupExpired() int

	// ClearAll clears all sessions.
	ClearAll() error
}

// ModuleConfig holds configuration for module initialization.
type ModuleConfig struct {
	// Module name
	Name string

	// Module-specific configuration
	Config interface{}
}

// ModuleRegistry manages module registration and lifecycle.
type ModuleRegistry struct {
	modules map[string]Module
}

// NewModuleRegistry creates a new module registry.
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		modules: make(map[string]Module),
	}
}

// Register registers a module.
func (r *ModuleRegistry) Register(module Module) error {
	if module == nil {
		return ErrModuleNotSet
	}

	name := module.Name()
	if name == "" {
		return ErrBusConfig
	}

	if _, exists := r.modules[name]; exists {
		return ErrModuleNotSet
	}

	r.modules[name] = module
	return nil
}

// Get retrieves a module by name.
func (r *ModuleRegistry) Get(name string) (Module, error) {
	module, exists := r.modules[name]
	if !exists {
		return nil, ErrModuleNotSet
	}
	return module, nil
}

// StopAll stops all registered modules.
func (r *ModuleRegistry) StopAll() error {
	var lastErr error
	for _, module := range r.modules {
		if err := module.Stop(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// List returns all registered module names.
func (r *ModuleRegistry) List() []string {
	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	return names
}
