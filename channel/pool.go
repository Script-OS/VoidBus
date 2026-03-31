// Package channel provides ChannelPool for v2.0 architecture.
// ChannelPool manages multiple channels with MTU awareness and health evaluation.
package channel

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// ChannelInfo holds information about a channel in the pool.
type ChannelInfo struct {
	Channel      Channel
	ID           string  // Unique identifier
	MTU          int     // Configured or default MTU
	HealthScore  float64 // 0.0 ~ 1.0
	SendCount    int64
	ErrorCount   int64
	LastActivity time.Time
	LastLatency  time.Duration
}

// ChannelPool manages multiple channels for the v2.0 architecture.
// It provides:
// - MTU-aware channel selection for adaptive fragmentation
// - Health evaluation for reliable channel selection
// - Random distribution for covert channel usage
type ChannelPool struct {
	mu sync.RWMutex

	// channels maps channel ID to ChannelInfo
	channels map[string]*ChannelInfo

	// channelList maintains order for round-robin
	channelList []string

	// roundRobinIndex for round-robin selection
	roundRobinIndex int

	// defaultMTU is used when channel doesn't provide MTU
	defaultMTU int

	// healthEvaluator evaluates channel health
	healthEvaluator *HealthEvaluator

	// mtuOverrides allows user to set custom MTU per channel
	mtuOverrides map[string]int
}

// NewChannelPool creates a new ChannelPool.
func NewChannelPool() *ChannelPool {
	return &ChannelPool{
		channels:        make(map[string]*ChannelInfo),
		channelList:     make([]string, 0),
		defaultMTU:      1024,
		healthEvaluator: NewHealthEvaluator(),
		mtuOverrides:    make(map[string]int),
	}
}

// AddChannel adds a channel to the pool.
func (p *ChannelPool) AddChannel(ch Channel, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ch == nil {
		return fmt.Errorf("channel cannot be nil")
	}

	if id == "" {
		return fmt.Errorf("channel ID cannot be empty")
	}

	if _, exists := p.channels[id]; exists {
		return fmt.Errorf("channel with ID '%s' already exists", id)
	}

	// Determine MTU
	mtu := p.defaultMTU
	if override, exists := p.mtuOverrides[id]; exists {
		mtu = override
	} else if chMTU := ch.DefaultMTU(); chMTU > 0 {
		mtu = chMTU
	}

	info := &ChannelInfo{
		Channel:      ch,
		ID:           id,
		MTU:          mtu,
		HealthScore:  1.0, // Start with full health
		LastActivity: time.Now(),
	}

	p.channels[id] = info
	p.channelList = append(p.channelList, id)

	return nil
}

// RemoveChannel removes a channel from the pool.
func (p *ChannelPool) RemoveChannel(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.channels[id]; !exists {
		return fmt.Errorf("channel with ID '%s' not found", id)
	}

	delete(p.channels, id)

	// Remove from list
	for i, chID := range p.channelList {
		if chID == id {
			p.channelList = append(p.channelList[:i], p.channelList[i+1:]...)
			break
		}
	}

	return nil
}

// SetMTUOverride sets a custom MTU for a specific channel.
func (p *ChannelPool) SetMTUOverride(id string, mtu int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.mtuOverrides[id] = mtu

	if info, exists := p.channels[id]; exists {
		info.MTU = mtu
	}

	return nil
}

// SetDefaultMTU sets the default MTU for channels without specific MTU.
func (p *ChannelPool) SetDefaultMTU(mtu int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.defaultMTU = mtu
}

// GetChannel returns channel info by ID.
func (p *ChannelPool) GetChannel(id string) (*ChannelInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	info, exists := p.channels[id]
	if !exists {
		return nil, fmt.Errorf("channel with ID '%s' not found", id)
	}
	return info, nil
}

// RandomSelect randomly selects a channel for sending.
func (p *ChannelPool) RandomSelect() (*ChannelInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.channelList) == 0 {
		return nil, ErrChannelNotReady
	}

	// Use simple random selection
	idx := randomInt(len(p.channelList))
	id := p.channelList[idx]
	return p.channels[id], nil
}

// SelectHealthy selects the healthiest channel (for NAK, control messages).
func (p *ChannelPool) SelectHealthy() (*ChannelInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.channelList) == 0 {
		return nil, ErrChannelNotReady
	}

	var best *ChannelInfo
	bestScore := 0.0

	for _, id := range p.channelList {
		info := p.channels[id]
		score := p.healthEvaluator.Evaluate(info)
		if score > bestScore {
			bestScore = score
			best = info
		}
	}

	if best == nil {
		return nil, ErrChannelNotReady
	}

	return best, nil
}

// SelectForMTU selects a channel suitable for the given data size.
// This is the smart distribution optimization (P1).
func (p *ChannelPool) SelectForMTU(dataSize int) (*ChannelInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.channelList) == 0 {
		return nil, ErrChannelNotReady
	}

	// Find channels with sufficient MTU
	candidates := make([]*ChannelInfo, 0)
	for _, id := range p.channelList {
		info := p.channels[id]
		if info.MTU >= dataSize && info.Channel.IsConnected() {
			candidates = append(candidates, info)
		}
	}

	// If no exact match, use smallest MTU that's still healthy
	if len(candidates) == 0 {
		// Fall back to healthy channel selection
		return p.SelectHealthy()
	}

	// Random among candidates
	idx := randomInt(len(candidates))
	return candidates[idx], nil
}

// RoundRobinSelect selects channel in round-robin order.
func (p *ChannelPool) RoundRobinSelect() (*ChannelInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.channelList) == 0 {
		return nil, ErrChannelNotReady
	}

	id := p.channelList[p.roundRobinIndex]
	p.roundRobinIndex = (p.roundRobinIndex + 1) % len(p.channelList)
	return p.channels[id], nil
}

// GetAdaptiveMTU calculates suggested MTU based on all channels.
// Returns the minimum MTU across all healthy channels.
func (p *ChannelPool) GetAdaptiveMTU() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.channelList) == 0 {
		return p.defaultMTU
	}

	minMTU := math.MaxInt32
	for _, id := range p.channelList {
		info := p.channels[id]
		if info.Channel.IsConnected() && info.MTU < minMTU {
			minMTU = info.MTU
		}
	}

	if minMTU == math.MaxInt32 {
		return p.defaultMTU
	}

	return minMTU
}

// RecordSend records a successful send operation.
func (p *ChannelPool) RecordSend(id string, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if info, exists := p.channels[id]; exists {
		info.SendCount++
		info.LastActivity = time.Now()
		info.LastLatency = latency
		info.HealthScore = p.healthEvaluator.Evaluate(info)
	}
}

// RecordError records a send error.
func (p *ChannelPool) RecordError(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if info, exists := p.channels[id]; exists {
		info.ErrorCount++
		info.HealthScore = p.healthEvaluator.Evaluate(info)
	}
}

// GetHealthScore returns the health score for a channel.
func (p *ChannelPool) GetHealthScore(id string) (float64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	info, exists := p.channels[id]
	if !exists {
		return 0, fmt.Errorf("channel with ID '%s' not found", id)
	}
	return info.HealthScore, nil
}

// Count returns the number of channels in the pool.
func (p *ChannelPool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.channelList)
}

// GetChannelIDs returns all channel IDs.
func (p *ChannelPool) GetChannelIDs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]string, len(p.channelList))
	copy(result, p.channelList)
	return result
}

// CloseAll closes all channels in the pool.
func (p *ChannelPool) CloseAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for _, info := range p.channels {
		if err := info.Channel.Close(); err != nil {
			lastErr = err
		}
	}

	p.channels = make(map[string]*ChannelInfo)
	p.channelList = make([]string, 0)

	return lastErr
}

// randomInt generates a random integer in [0, n).
func randomInt(n int) int {
	if n <= 0 {
		return 0
	}
	// Simple implementation using time
	return int(time.Now().UnixNano()) % n
}

// HealthEvaluator evaluates channel health.
type HealthEvaluator struct {
	// Weights for health score calculation
	ErrorWeight   float64
	LatencyWeight float64
	TimeWeight    float64

	// Thresholds
	MaxLatency time.Duration
	Timeout    time.Duration
}

// NewHealthEvaluator creates a new health evaluator.
func NewHealthEvaluator() *HealthEvaluator {
	return &HealthEvaluator{
		ErrorWeight:   0.5,
		LatencyWeight: 0.3,
		TimeWeight:    0.2,
		MaxLatency:    5 * time.Second,
		Timeout:       30 * time.Second,
	}
}

// Evaluate calculates health score for a channel.
func (e *HealthEvaluator) Evaluate(info *ChannelInfo) float64 {
	if info == nil {
		return 0
	}

	// 1. Error rate score (lower is better)
	totalOps := info.SendCount + info.ErrorCount
	if totalOps == 0 {
		totalOps = 1
	}
	errorRate := float64(info.ErrorCount) / float64(totalOps)
	errorScore := 1.0 - errorRate
	if errorScore < 0 {
		errorScore = 0
	}

	// 2. Latency score (lower is better)
	latencyScore := 1.0
	if info.LastLatency > 0 {
		latencyScore = 1.0 - float64(info.LastLatency)/float64(e.MaxLatency)
		if latencyScore < 0 {
			latencyScore = 0
		}
	}

	// 3. Time score (recent activity is better)
	timeSinceActivity := time.Since(info.LastActivity)
	timeScore := 1.0 - float64(timeSinceActivity)/float64(e.Timeout)
	if timeScore < 0 {
		timeScore = 0
	}

	// Weighted average
	totalScore := errorScore*e.ErrorWeight +
		latencyScore*e.LatencyWeight +
		timeScore*e.TimeWeight

	return totalScore
}
