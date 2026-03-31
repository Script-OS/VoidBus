// Package protocol provides channel selection strategy interfaces for VoidBus.
//
// ChannelSelector defines how channels are selected for data transmission
// in MultiBus scenarios. Different strategies optimize for different goals:
// - Random: Simple, fair distribution
// - RoundRobin: Ordered, predictable distribution
// - Weighted: Priority-based distribution
// - HealthAware: Reliability-based distribution
//
// Design Constraints:
// - Selector operates on ChannelSelectInfo, NOT raw Channel
// - Selector does NOT perform actual transmission
// - Selector SHOULD consider channel health status
package protocol

import (
	"errors"

	"github.com/Script-OS/VoidBus/fragment"
)

// Selector errors
var (
	// ErrNoAvailableChannel indicates no active channel available
	ErrNoAvailableChannel = errors.New("selector: no available channel")
	// ErrChannelSelectionFailed indicates selection failed
	ErrChannelSelectionFailed = errors.New("selector: selection failed")
	// ErrInvalidChannelIndex indicates invalid channel index
	ErrInvalidChannelIndex = errors.New("selector: invalid channel index")
)

// ChannelSelectInfo contains information for channel selection.
// This is a lightweight structure passed to selectors.
type ChannelSelectInfo struct {
	// Index is the channel index in the pool
	Index int

	// Weight is the weight for weighted selection (0-100)
	Weight int

	// Status is the channel status (0=active, 1=inactive, 2=error)
	Status int

	// IsConnected indicates whether channel is connected
	IsConnected bool

	// LastActivity is last activity timestamp
	LastActivity int64

	// SendCount is number of packets sent
	SendCount int64

	// ReceiveCount is number of packets received
	ReceiveCount int64

	// ErrorCount is number of errors
	ErrorCount int64
}

// ChannelSelectStatus constants
const (
	ChannelSelectStatusActive   = 0
	ChannelSelectStatusInactive = 1
	ChannelSelectStatusError    = 2
)

// ChannelSelector defines the interface for channel selection strategies.
type ChannelSelector interface {
	// Select selects a channel from the pool for sending.
	//
	// Parameter Constraints:
	//   - channels: MUST be non-nil slice of ChannelSelectInfo
	//
	// Return Guarantees:
	//   - Returns index of selected channel (0-based)
	//   - Returns error if no channel available
	//
	// Selection Criteria:
	//   - Only selects channels with Status == ChannelSelectStatusActive
	//   - Only selects channels where IsConnected == true
	Select(channels []ChannelSelectInfo) (int, error)

	// SelectForFragment selects channel for specific fragment.
	// May use fragment info for intelligent routing decisions.
	//
	// Parameter Constraints:
	//   - channels: MUST be non-nil slice of ChannelSelectInfo
	//   - fragmentInfo: fragment metadata (optional routing hint)
	SelectForFragment(channels []ChannelSelectInfo, fragmentInfo fragment.FragmentInfo) (int, error)

	// Name returns the strategy name for logging/debugging.
	Name() string

	// Reset resets the selector state (e.g., round-robin counter).
	Reset()
}

// FragmentDistributor defines how fragments are distributed across channels.
type FragmentDistributor interface {
	// Distribute distributes fragments to channels.
	//
	// Parameter Constraints:
	//   - fragmentCount: total number of fragments
	//   - channels: available channel pool
	//
	// Return Guarantees:
	//   - Returns mapping: channelIndex -> [fragmentIndices]
	//   - Returns nil if no channel available
	//
	// Distribution Strategies:
	//   - AllRandom: Each fragment to random channel (max diversity)
	//   - Grouped: All fragments of one message to same channel (ordering)
	//   - RoundRobin: Fragments distributed in order (balanced)
	//   - Weighted: Weight-based distribution (priority)
	Distribute(fragmentCount int, channels []ChannelSelectInfo) map[int][]int

	// Name returns the distribution strategy name.
	Name() string
}

// DistributionStrategy defines how fragments are distributed.
type DistributionStrategy int

const (
	// DistributeAllRandom distributes each fragment to a random channel
	DistributeAllRandom DistributionStrategy = iota
	// DistributeGrouped sends all fragments to one channel
	DistributeGrouped
	// DistributeRoundRobin distributes fragments in round-robin order
	DistributeRoundRobin
	// DistributeWeighted distributes based on channel weights
	DistributeWeighted
	// DistributeHealthAware considers channel health for distribution
	DistributeHealthAware
)

// String returns string representation.
func (s DistributionStrategy) String() string {
	switch s {
	case DistributeAllRandom:
		return "all_random"
	case DistributeGrouped:
		return "grouped"
	case DistributeRoundRobin:
		return "round_robin"
	case DistributeWeighted:
		return "weighted"
	case DistributeHealthAware:
		return "health_aware"
	default:
		return "unknown"
	}
}

// ChannelHealthChecker defines interface for channel health monitoring.
type ChannelHealthChecker interface {
	// Check performs health check on channel.
	Check(ch ChannelSelectInfo) ChannelHealthStatus

	// ShouldDisable returns whether channel should be disabled.
	ShouldDisable(ch ChannelSelectInfo) bool

	// ShouldEnable returns whether disabled channel should be re-enabled.
	ShouldEnable(ch ChannelSelectInfo) bool

	// Period returns health check interval.
	Period() int // seconds
}

// ChannelHealthStatus represents channel health status.
type ChannelHealthStatus struct {
	// IsHealthy indicates whether channel is healthy
	IsHealthy bool

	// Latency is recent average latency (milliseconds)
	Latency int64

	// ErrorRate is recent error rate (0.0 - 1.0)
	ErrorRate float64

	// SuccessRate is recent success rate (0.0 - 1.0)
	SuccessRate float64

	// LastCheckTime is last health check timestamp
	LastCheckTime int64

	// ConsecutiveErrors is number of consecutive errors
	ConsecutiveErrors int

	// ConsecutiveSuccesses is number of consecutive successes
	ConsecutiveSuccesses int
}

// IsUsable returns whether channel is usable for transmission.
func (s ChannelHealthStatus) IsUsable() bool {
	return s.IsHealthy && s.ErrorRate < 0.5
}

// HealthCheckConfig provides configuration for health checker.
type HealthCheckConfig struct {
	// CheckInterval is health check interval in seconds
	CheckInterval int

	// ErrorThreshold is error threshold for disabling channel
	ErrorThreshold float64

	// ConsecutiveErrorThreshold is consecutive error count for disabling
	ConsecutiveErrorThreshold int

	// RecoveryThreshold is success rate threshold for recovery
	RecoveryThreshold float64

	// ConsecutiveSuccessThreshold is consecutive success count for recovery
	ConsecutiveSuccessThreshold int

	// LatencyThreshold is latency threshold in milliseconds (optional)
	LatencyThreshold int64
}

// DefaultHealthCheckConfig returns default health check configuration.
func DefaultHealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		CheckInterval:               30,
		ErrorThreshold:              0.3,
		ConsecutiveErrorThreshold:   5,
		RecoveryThreshold:           0.8,
		ConsecutiveSuccessThreshold: 10,
		LatencyThreshold:            1000, // 1 second
	}
}

// filterActiveChannels filters only active and connected channels.
func filterActiveChannels(channels []ChannelSelectInfo) []ChannelSelectInfo {
	active := make([]ChannelSelectInfo, 0)
	for _, ch := range channels {
		if ch.Status == ChannelSelectStatusActive && ch.IsConnected {
			active = append(active, ch)
		}
	}
	return active
}

// ValidateChannels validates channel pool for selection.
func ValidateChannels(channels []ChannelSelectInfo) error {
	if channels == nil || len(channels) == 0 {
		return ErrNoAvailableChannel
	}

	active := filterActiveChannels(channels)
	if len(active) == 0 {
		return ErrNoAvailableChannel
	}

	return nil
}
