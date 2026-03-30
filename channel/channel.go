// Package channel provides the channel registry for VoidBus.
//
// For interface definitions, see interface.go.
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.4):
// - Channel.Type() MUST NOT be transmitted over network
package channel

import (
	"errors"
	"io"
	"time"
)

// ChannelRegistry manages registered channels.
type ChannelRegistry struct {
	modules map[ChannelType]ChannelModule
}

// NewChannelRegistry creates a new ChannelRegistry.
func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{
		modules: make(map[ChannelType]ChannelModule),
	}
}

// Register registers a channel module.
func (r *ChannelRegistry) Register(module ChannelModule) error {
	if module == nil {
		return errors.New("channel: cannot register nil module")
	}
	r.modules[module.Type()] = module
	return nil
}

// GetClient retrieves a client channel instance.
func (r *ChannelRegistry) GetClient(typ ChannelType, config ChannelConfig) (Channel, error) {
	module, exists := r.modules[typ]
	if !exists {
		return nil, errors.New("channel: type not registered: " + string(typ))
	}
	return module.CreateClient(config)
}

// GetServer retrieves a server channel instance.
func (r *ChannelRegistry) GetServer(typ ChannelType, config ChannelConfig) (ServerChannel, error) {
	module, exists := r.modules[typ]
	if !exists {
		return nil, errors.New("channel: type not registered: " + string(typ))
	}
	return module.CreateServer(config)
}

// List returns all registered channel types.
func (r *ChannelRegistry) List() []ChannelType {
	result := make([]ChannelType, 0, len(r.modules))
	for typ := range r.modules {
		result = append(result, typ)
	}
	return result
}

// Exists checks if a channel type is registered.
func (r *ChannelRegistry) Exists(typ ChannelType) bool {
	_, exists := r.modules[typ]
	return exists
}

// Global registry
var globalRegistry = NewChannelRegistry()

// Register registers a module to the global registry.
func Register(module ChannelModule) error {
	return globalRegistry.Register(module)
}

// GetClient retrieves a client channel from the global registry.
func GetClient(typ ChannelType, config ChannelConfig) (Channel, error) {
	return globalRegistry.GetClient(typ, config)
}

// GetServer retrieves a server channel from the global registry.
func GetServer(typ ChannelType, config ChannelConfig) (ServerChannel, error) {
	return globalRegistry.GetServer(typ, config)
}

// List returns all channel types from the global registry.
func List() []ChannelType {
	return globalRegistry.List()
}

// GlobalRegistry returns the global registry instance.
func GlobalRegistry() *ChannelRegistry {
	return globalRegistry
}

// Frame protocol constants for TCP channel
const (
	FrameHeaderSize = 4
	MaxFrameSize    = 16 * 1024 * 1024 // 16MB
)

// WriteFrame writes a length-prefixed frame to the writer.
func WriteFrame(w io.Writer, data []byte) error {
	if len(data) > MaxFrameSize {
		return errors.New("channel: frame too large")
	}

	header := make([]byte, FrameHeaderSize)
	header[0] = byte(len(data) >> 24)
	header[1] = byte(len(data) >> 16)
	header[2] = byte(len(data) >> 8)
	header[3] = byte(len(data))

	if _, err := w.Write(header); err != nil {
		return err
	}

	_, err := w.Write(data)
	return err
}

// ReadFrame reads a length-prefixed frame from the reader.
func ReadFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, FrameHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	length := int(header[0])<<24 | int(header[1])<<16 | int(header[2])<<8 | int(header[3])
	if length > MaxFrameSize {
		return nil, errors.New("channel: frame too large")
	}
	if length < 0 {
		return nil, errors.New("channel: invalid frame length")
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return data, nil
}

// DefaultChannelConfig returns default channel configuration.
func DefaultChannelConfig() ChannelConfig {
	return ChannelConfig{
		Timeout:         30 * time.Second,
		ConnectTimeout:  10 * time.Second,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		MaxMessageSize:  MaxFrameSize,
		BufferSize:      4096,
		KeepAlive:       true,
		KeepAlivePeriod: 30 * time.Second,
		ReuseAddr:       true,
	}
}
