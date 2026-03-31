// Package protocol provides fragment distribution strategies for VoidBus.
//
// FragmentDistributor implementations define how fragments are distributed
// across multiple channels in MultiBus scenarios.
//
// Distribution Strategies:
// - AllRandom: Each fragment to random channel (max diversity)
// - Grouped: All fragments to same channel (ordering guarantee)
// - RoundRobin: Fragments distributed in order (balanced load)
// - Weighted: Distribution based on channel weights (priority)
// - HealthAware: Distribution based on channel health (reliability)
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.4):
// - Distributor operates on ChannelSelectInfo, NOT raw Channel
// - Distributor does NOT perform actual transmission
// - Distributor SHOULD consider channel health status
package protocol

import (
	"errors"
	"math/rand"
	"sync"
)

// Distributor errors
var (
	// ErrInvalidFragmentCount indicates invalid fragment count
	ErrInvalidFragmentCount = errors.New("distributor: invalid fragment count")
	// ErrNoActiveChannel indicates no active channel available
	ErrNoActiveChannel = errors.New("distributor: no active channel")
	// ErrInvalidWeight indicates invalid weight configuration
	ErrInvalidWeight = errors.New("distributor: invalid weight")
)

// AllRandomDistributor distributes each fragment to a random channel.
// This provides maximum diversity and fault tolerance.
type AllRandomDistributor struct {
	mu sync.RWMutex
}

// NewAllRandomDistributor creates a new AllRandomDistributor.
func NewAllRandomDistributor() *AllRandomDistributor {
	return &AllRandomDistributor{}
}

// Distribute distributes fragments randomly.
func (d *AllRandomDistributor) Distribute(fragmentCount int, channels []ChannelSelectInfo) map[int][]int {
	d.mu.Lock()
	defer d.mu.Unlock()

	if fragmentCount <= 0 || len(channels) == 0 {
		return nil
	}

	// Filter active channels
	active := filterActiveChannels(channels)
	if len(active) == 0 {
		return nil
	}

	result := make(map[int][]int)
	for i := 0; i < fragmentCount; i++ {
		// Select random active channel
		selectedIdx := rand.Intn(len(active))
		channelIdx := active[selectedIdx].Index
		result[channelIdx] = append(result[channelIdx], i)
	}

	return result
}

// Name returns the distributor name.
func (d *AllRandomDistributor) Name() string {
	return "all_random"
}

// GroupedDistributor sends all fragments to one selected channel.
// This guarantees ordering but provides no diversity.
type GroupedDistributor struct {
	mu          sync.RWMutex
	selector    ChannelSelector
	lastChannel int
}

// NewGroupedDistributor creates a new GroupedDistributor.
func NewGroupedDistributor(selector ChannelSelector) *GroupedDistributor {
	return &GroupedDistributor{
		selector: selector,
	}
}

// Distribute sends all fragments to one channel.
func (d *GroupedDistributor) Distribute(fragmentCount int, channels []ChannelSelectInfo) map[int][]int {
	d.mu.Lock()
	defer d.mu.Unlock()

	if fragmentCount <= 0 || len(channels) == 0 {
		return nil
	}

	// Filter active channels
	active := filterActiveChannels(channels)
	if len(active) == 0 {
		return nil
	}

	// Select channel
	var selectedIdx int
	var err error

	if d.selector != nil {
		selectedIdx, err = d.selector.Select(channels)
		if err != nil {
			// Fallback to random
			selectedIdx = active[rand.Intn(len(active))].Index
		}
	} else {
		// Random selection
		selectedIdx = active[rand.Intn(len(active))].Index
	}

	d.lastChannel = selectedIdx

	// All fragments go to selected channel
	result := make(map[int][]int)
	fragments := make([]int, fragmentCount)
	for i := 0; i < fragmentCount; i++ {
		fragments[i] = i
	}
	result[selectedIdx] = fragments

	return result
}

// Name returns the distributor name.
func (d *GroupedDistributor) Name() string {
	return "grouped"
}

// LastChannel returns the last selected channel index.
func (d *GroupedDistributor) LastChannel() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastChannel
}

// RoundRobinDistributor distributes fragments in round-robin order.
// This provides balanced load distribution.
type RoundRobinDistributor struct {
	mu     sync.RWMutex
	offset int
}

// NewRoundRobinDistributor creates a new RoundRobinDistributor.
func NewRoundRobinDistributor() *RoundRobinDistributor {
	return &RoundRobinDistributor{}
}

// Distribute distributes fragments in round-robin order.
func (d *RoundRobinDistributor) Distribute(fragmentCount int, channels []ChannelSelectInfo) map[int][]int {
	d.mu.Lock()
	defer d.mu.Unlock()

	if fragmentCount <= 0 || len(channels) == 0 {
		return nil
	}

	// Filter active channels
	active := filterActiveChannels(channels)
	if len(active) == 0 {
		return nil
	}

	result := make(map[int][]int)
	channelCount := len(active)

	for i := 0; i < fragmentCount; i++ {
		// Round-robin selection
		activeIdx := (d.offset + i) % channelCount
		channelIdx := active[activeIdx].Index
		result[channelIdx] = append(result[channelIdx], i)
	}

	// Update offset for next round
	d.offset = (d.offset + fragmentCount) % channelCount

	return result
}

// Name returns the distributor name.
func (d *RoundRobinDistributor) Name() string {
	return "round_robin"
}

// Reset resets the round-robin offset.
func (d *RoundRobinDistributor) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.offset = 0
}

// WeightedDistributor distributes fragments based on channel weights.
// Higher weight channels receive more fragments.
type WeightedDistributor struct {
	mu sync.RWMutex
}

// NewWeightedDistributor creates a new WeightedDistributor.
func NewWeightedDistributor() *WeightedDistributor {
	return &WeightedDistributor{}
}

// Distribute distributes fragments based on weights.
func (d *WeightedDistributor) Distribute(fragmentCount int, channels []ChannelSelectInfo) map[int][]int {
	d.mu.Lock()
	defer d.mu.Unlock()

	if fragmentCount <= 0 || len(channels) == 0 {
		return nil
	}

	// Filter active channels
	active := filterActiveChannels(channels)
	if len(active) == 0 {
		return nil
	}

	// Calculate total weight
	totalWeight := 0
	for _, ch := range active {
		if ch.Weight <= 0 {
			continue // Skip zero/negative weights
		}
		totalWeight += ch.Weight
	}

	if totalWeight == 0 {
		// Fallback to equal distribution
		return distributeEqual(fragmentCount, active)
	}

	result := make(map[int][]int)
	remaining := fragmentCount

	// Distribute based on weight ratio
	for i, ch := range active {
		if ch.Weight <= 0 {
			continue
		}

		// Calculate share (proportional to weight)
		share := (fragmentCount * ch.Weight) / totalWeight
		if i == len(active)-1 {
			// Last channel gets remaining
			share = remaining
		}

		for j := 0; j < share && remaining > 0; j++ {
			result[ch.Index] = append(result[ch.Index], fragmentCount-remaining)
			remaining--
		}
	}

	// Distribute any remaining fragments randomly
	for remaining > 0 {
		for _, ch := range active {
			if remaining <= 0 {
				break
			}
			result[ch.Index] = append(result[ch.Index], fragmentCount-remaining)
			remaining--
		}
	}

	return result
}

// Name returns the distributor name.
func (d *WeightedDistributor) Name() string {
	return "weighted"
}

// HealthAwareDistributor distributes fragments based on channel health.
// Healthier channels receive more fragments.
type HealthAwareDistributor struct {
	mu            sync.RWMutex
	healthChecker ChannelHealthChecker
}

// NewHealthAwareDistributor creates a new HealthAwareDistributor.
func NewHealthAwareDistributor(checker ChannelHealthChecker) *HealthAwareDistributor {
	return &HealthAwareDistributor{
		healthChecker: checker,
	}
}

// Distribute distributes fragments based on health status.
func (d *HealthAwareDistributor) Distribute(fragmentCount int, channels []ChannelSelectInfo) map[int][]int {
	d.mu.Lock()
	defer d.mu.Unlock()

	if fragmentCount <= 0 || len(channels) == 0 {
		return nil
	}

	// Filter active channels
	active := filterActiveChannels(channels)
	if len(active) == 0 {
		return nil
	}

	// If no health checker, fallback to equal distribution
	if d.healthChecker == nil {
		return distributeEqual(fragmentCount, active)
	}

	// Calculate health scores
	type channelHealth struct {
		index  int
		health ChannelHealthStatus
	}

	healthScores := make([]channelHealth, 0, len(active))
	totalScore := 0

	for _, ch := range active {
		health := d.healthChecker.Check(ch)
		if health.IsUsable() {
			// Score based on success rate and latency
			score := int(health.SuccessRate * 100)
			if health.Latency > 0 {
				// Lower latency = higher score
				latencyScore := 100 - int(health.Latency/10)
				if latencyScore > 0 {
					score += latencyScore
				}
			}
			healthScores = append(healthScores, channelHealth{
				index:  ch.Index,
				health: health,
			})
			totalScore += score
		}
	}

	if len(healthScores) == 0 {
		return distributeEqual(fragmentCount, active)
	}

	result := make(map[int][]int)
	remaining := fragmentCount

	// Distribute based on health scores
	for i, ch := range healthScores {
		// Calculate share (convert to float for multiplication, then back to int)
		share := float64(fragmentCount) * ch.health.SuccessRate
		shareInt := int(share)
		if i == len(healthScores)-1 {
			shareInt = remaining
		}

		for j := 0; j < shareInt && remaining > 0; j++ {
			result[ch.index] = append(result[ch.index], fragmentCount-remaining)
			remaining--
		}
	}

	// Distribute remaining
	for remaining > 0 {
		for _, ch := range healthScores {
			if remaining <= 0 {
				break
			}
			result[ch.index] = append(result[ch.index], fragmentCount-remaining)
			remaining--
		}
	}

	return result
}

// Name returns the distributor name.
func (d *HealthAwareDistributor) Name() string {
	return "health_aware"
}

// SetHealthChecker sets the health checker.
func (d *HealthAwareDistributor) SetHealthChecker(checker ChannelHealthChecker) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.healthChecker = checker
}

// distributeEqual distributes fragments equally across channels.
func distributeEqual(fragmentCount int, activeChannels []ChannelSelectInfo) map[int][]int {
	result := make(map[int][]int)
	channelCount := len(activeChannels)

	for i := 0; i < fragmentCount; i++ {
		channelIdx := activeChannels[i%channelCount].Index
		result[channelIdx] = append(result[channelIdx], i)
	}

	return result
}

// DistributorFactory creates distributors based on strategy.
type DistributorFactory struct {
	mu sync.RWMutex
}

// NewDistributorFactory creates a new DistributorFactory.
func NewDistributorFactory() *DistributorFactory {
	return &DistributorFactory{}
}

// Create creates a distributor based on strategy.
func (f *DistributorFactory) Create(strategy DistributionStrategy) FragmentDistributor {
	switch strategy {
	case DistributeAllRandom:
		return NewAllRandomDistributor()
	case DistributeGrouped:
		return NewGroupedDistributor(nil)
	case DistributeRoundRobin:
		return NewRoundRobinDistributor()
	case DistributeWeighted:
		return NewWeightedDistributor()
	case DistributeHealthAware:
		return NewHealthAwareDistributor(nil)
	default:
		return NewRoundRobinDistributor() // Default fallback
	}
}

// CreateWithSelector creates a grouped distributor with selector.
func (f *DistributorFactory) CreateWithSelector(selector ChannelSelector) FragmentDistributor {
	return NewGroupedDistributor(selector)
}

// CreateWithHealthChecker creates a health-aware distributor with checker.
func (f *DistributorFactory) CreateWithHealthChecker(checker ChannelHealthChecker) FragmentDistributor {
	return NewHealthAwareDistributor(checker)
}

// Global factory
var globalFactory = NewDistributorFactory()

// CreateDistributor creates a distributor using global factory.
func CreateDistributor(strategy DistributionStrategy) FragmentDistributor {
	return globalFactory.Create(strategy)
}
