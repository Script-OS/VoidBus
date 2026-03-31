// Package selector provides channel selection strategy implementations.
//
// Selectors implement protocol.ChannelSelector interface for MultiBus
// channel selection strategies.
//
// Available Selectors:
// - RandomSelector: Simple random selection
// - RoundRobinSelector: Ordered round-robin selection
// - WeightedSelector: Weight-based priority selection
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.4):
// - Selector operates on protocol.ChannelSelectInfo
// - Selector does NOT perform actual transmission
// - Selector MUST be thread-safe
package selector

import (
	"math/rand"
	"sync"

	"github.com/Script-OS/VoidBus/fragment"
	"github.com/Script-OS/VoidBus/protocol"
)

// RandomSelector selects channels randomly.
// Simple and fair distribution strategy.
type RandomSelector struct {
	mu sync.RWMutex
}

// NewRandomSelector creates a new RandomSelector.
func NewRandomSelector() *RandomSelector {
	return &RandomSelector{}
}

// Select randomly selects an active channel.
func (s *RandomSelector) Select(channels []protocol.ChannelSelectInfo) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := protocol.ValidateChannels(channels); err != nil {
		return -1, err
	}

	// Filter active channels
	active := make([]protocol.ChannelSelectInfo, 0)
	for _, ch := range channels {
		if ch.Status == protocol.ChannelSelectStatusActive && ch.IsConnected {
			active = append(active, ch)
		}
	}

	if len(active) == 0 {
		return -1, protocol.ErrNoAvailableChannel
	}

	// Random selection
	selected := active[rand.Intn(len(active))]
	return selected.Index, nil
}

// SelectForFragment selects channel for specific fragment.
// For random selector, this is same as Select().
func (s *RandomSelector) SelectForFragment(channels []protocol.ChannelSelectInfo, info fragment.FragmentInfo) (int, error) {
	return s.Select(channels)
}

// Name returns the selector name.
func (s *RandomSelector) Name() string {
	return "random"
}

// Reset resets selector state (no state for random selector).
func (s *RandomSelector) Reset() {
	// No state to reset
}

// RoundRobinSelector selects channels in round-robin order.
// Provides ordered, predictable distribution.
type RoundRobinSelector struct {
	mu     sync.RWMutex
	offset int
}

// NewRoundRobinSelector creates a new RoundRobinSelector.
func NewRoundRobinSelector() *RoundRobinSelector {
	return &RoundRobinSelector{}
}

// Select selects channel in round-robin order.
func (s *RoundRobinSelector) Select(channels []protocol.ChannelSelectInfo) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := protocol.ValidateChannels(channels); err != nil {
		return -1, err
	}

	// Filter active channels
	active := make([]protocol.ChannelSelectInfo, 0)
	for _, ch := range channels {
		if ch.Status == protocol.ChannelSelectStatusActive && ch.IsConnected {
			active = append(active, ch)
		}
	}

	if len(active) == 0 {
		return -1, protocol.ErrNoAvailableChannel
	}

	// Round-robin selection
	selected := active[s.offset%len(active)]
	s.offset++

	return selected.Index, nil
}

// SelectForFragment selects channel for specific fragment.
func (s *RoundRobinSelector) SelectForFragment(channels []protocol.ChannelSelectInfo, info fragment.FragmentInfo) (int, error) {
	// For round-robin, use fragment index as hint
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := protocol.ValidateChannels(channels); err != nil {
		return -1, err
	}

	// Filter active channels
	active := make([]protocol.ChannelSelectInfo, 0)
	for _, ch := range channels {
		if ch.Status == protocol.ChannelSelectStatusActive && ch.IsConnected {
			active = append(active, ch)
		}
	}

	if len(active) == 0 {
		return -1, protocol.ErrNoAvailableChannel
	}

	// Use fragment index to influence selection
	idx := int(info.Index) % len(active)
	selected := active[idx]

	return selected.Index, nil
}

// Name returns the selector name.
func (s *RoundRobinSelector) Name() string {
	return "round_robin"
}

// Reset resets the round-robin offset.
func (s *RoundRobinSelector) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offset = 0
}

// Offset returns current offset.
func (s *RoundRobinSelector) Offset() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.offset
}

// WeightedSelector selects channels based on weight.
// Higher weight channels have higher probability of selection.
type WeightedSelector struct {
	mu sync.RWMutex
}

// NewWeightedSelector creates a new WeightedSelector.
func NewWeightedSelector() *WeightedSelector {
	return &WeightedSelector{}
}

// Select selects channel based on weight probability.
func (s *WeightedSelector) Select(channels []protocol.ChannelSelectInfo) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := protocol.ValidateChannels(channels); err != nil {
		return -1, err
	}

	// Filter active channels with valid weights
	active := make([]protocol.ChannelSelectInfo, 0)
	totalWeight := 0

	for _, ch := range channels {
		if ch.Status == protocol.ChannelSelectStatusActive && ch.IsConnected && ch.Weight > 0 {
			active = append(active, ch)
			totalWeight += ch.Weight
		}
	}

	if len(active) == 0 {
		return -1, protocol.ErrNoAvailableChannel
	}

	// Weighted random selection
	r := rand.Intn(totalWeight)
	cumulative := 0

	for _, ch := range active {
		cumulative += ch.Weight
		if r < cumulative {
			return ch.Index, nil
		}
	}

	// Fallback to last channel
	return active[len(active)-1].Index, nil
}

// SelectForFragment selects channel for specific fragment based on weight.
func (s *WeightedSelector) SelectForFragment(channels []protocol.ChannelSelectInfo, info fragment.FragmentInfo) (int, error) {
	// Weighted selector doesn't use fragment info for decision
	return s.Select(channels)
}

// Name returns the selector name.
func (s *WeightedSelector) Name() string {
	return "weighted"
}

// Reset resets selector state (no state for weighted selector).
func (s *WeightedSelector) Reset() {
	// No state to reset
}

// HealthAwareSelector selects channels based on health status.
// Prioritizes healthier channels for better reliability.
type HealthAwareSelector struct {
	mu            sync.RWMutex
	healthChecker protocol.ChannelHealthChecker
}

// NewHealthAwareSelector creates a new HealthAwareSelector.
func NewHealthAwareSelector(checker protocol.ChannelHealthChecker) *HealthAwareSelector {
	return &HealthAwareSelector{
		healthChecker: checker,
	}
}

// Select selects the healthiest channel.
func (s *HealthAwareSelector) Select(channels []protocol.ChannelSelectInfo) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := protocol.ValidateChannels(channels); err != nil {
		return -1, err
	}

	// Filter active channels
	active := make([]protocol.ChannelSelectInfo, 0)
	for _, ch := range channels {
		if ch.Status == protocol.ChannelSelectStatusActive && ch.IsConnected {
			active = append(active, ch)
		}
	}

	if len(active) == 0 {
		return -1, protocol.ErrNoAvailableChannel
	}

	// If no health checker, fallback to random
	if s.healthChecker == nil {
		selected := active[rand.Intn(len(active))]
		return selected.Index, nil
	}

	// Find healthiest channel
	var bestChannel *protocol.ChannelSelectInfo
	bestScore := -1.0

	for i := range active {
		health := s.healthChecker.Check(active[i])
		if !health.IsUsable() {
			continue
		}

		// Calculate score: success rate + latency factor
		score := health.SuccessRate
		if health.Latency > 0 && health.Latency < 1000 {
			score += (1.0 - float64(health.Latency)/1000.0) * 0.5
		}

		if score > bestScore {
			bestScore = score
			bestChannel = &active[i]
		}
	}

	if bestChannel == nil {
		// No usable channel, fallback to random active
		selected := active[rand.Intn(len(active))]
		return selected.Index, nil
	}

	return bestChannel.Index, nil
}

// SelectForFragment selects healthiest channel for fragment.
func (s *HealthAwareSelector) SelectForFragment(channels []protocol.ChannelSelectInfo, info fragment.FragmentInfo) (int, error) {
	return s.Select(channels)
}

// Name returns the selector name.
func (s *HealthAwareSelector) Name() string {
	return "health_aware"
}

// Reset resets selector state.
func (s *HealthAwareSelector) Reset() {
	// No state to reset
}

// SetHealthChecker sets the health checker.
func (s *HealthAwareSelector) SetHealthChecker(checker protocol.ChannelHealthChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthChecker = checker
}

// SelectorFactory creates selectors based on strategy.
type SelectorFactory struct{}

// NewSelectorFactory creates a new SelectorFactory.
func NewSelectorFactory() *SelectorFactory {
	return &SelectorFactory{}
}

// Create creates a selector based on distribution strategy.
func (f *SelectorFactory) Create(strategy protocol.DistributionStrategy) protocol.ChannelSelector {
	switch strategy {
	case protocol.DistributeAllRandom:
		return NewRandomSelector()
	case protocol.DistributeRoundRobin:
		return NewRoundRobinSelector()
	case protocol.DistributeWeighted:
		return NewWeightedSelector()
	case protocol.DistributeHealthAware:
		return NewHealthAwareSelector(nil)
	case protocol.DistributeGrouped:
		return NewRandomSelector() // Grouped uses any selector for initial selection
	default:
		return NewRandomSelector() // Default fallback
	}
}

// CreateWithHealthChecker creates a health-aware selector with checker.
func (f *SelectorFactory) CreateWithHealthChecker(checker protocol.ChannelHealthChecker) protocol.ChannelSelector {
	return NewHealthAwareSelector(checker)
}

// Global factory
var globalFactory = NewSelectorFactory()

// CreateSelector creates a selector using global factory.
func CreateSelector(strategy protocol.DistributionStrategy) protocol.ChannelSelector {
	return globalFactory.Create(strategy)
}
