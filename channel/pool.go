// Package channel provides ChannelPool for v2.1 architecture.
//
// ChannelPool manages multiple channels with:
// - Single selection algorithm: health-weighted random (roulette wheel)
// - Automatic channel switching on failure/timeout
// - Health tracking with recovery mechanism
//
// Selection Algorithm:
// - Filter: exclude unavailable channels
// - Weight: health score as weight
// - Random: roulette wheel selection
package channel

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

// ChannelState represents channel availability state.
type ChannelState int

const (
	StateAvailable   ChannelState = iota // Channel is available for use
	StateUnavailable                     // Channel is temporarily unavailable (error/timeout)
	StateRecovering                      // Channel is recovering from unavailable state
)

// ChannelInfo holds information about a channel in the pool.
type ChannelInfo struct {
	Channel       Channel
	ID            string        // Unique identifier
	Type          ChannelType   // Channel type identifier
	MTU           int           // Configured or default MTU
	HealthScore   float64       // 0.0 ~ 1.0 (weighted by error rate, latency, activity)
	State         ChannelState  // Availability state
	SendCount     int64         // Total successful sends
	ErrorCount    int64         // Total errors
	LastActivity  time.Time     // Last successful activity
	LastError     time.Time     // Last error time
	LastLatency   time.Duration // Last measured latency
	RecoveryCount int           // Successful sends since marked unavailable
}

// Constants for health evaluation
const (
	// Health score thresholds
	MinHealthScore    = 0.1 // Minimum health score before marking unavailable
	RecoveryThreshold = 5   // Successful sends needed to recover from unavailable

	// Timeout thresholds
	UnavailableTimeout = 3 * time.Second  // Timeout to mark channel unavailable
	RecoveryTimeout    = 10 * time.Second // Time before attempting recovery

	// Health evaluation weights
	ErrorWeight    = 0.5
	LatencyWeight  = 0.3
	ActivityWeight = 0.2
	MaxLatency     = 5 * time.Second
)

// ChannelPool manages multiple channels for covert communication.
type ChannelPool struct {
	mu sync.RWMutex

	channels    map[string]*ChannelInfo
	channelList []string // Ordered list for selection

	// Negotiation result (available channels after handshake)
	negotiatedChannels map[ChannelType]bool
	negotiatedCodecs   map[int]bool

	// Random source for selection
	rand *rand.Rand
}

// NewChannelPool creates a new ChannelPool.
func NewChannelPool() *ChannelPool {
	return &ChannelPool{
		channels:           make(map[string]*ChannelInfo),
		channelList:        make([]string, 0),
		negotiatedChannels: make(map[ChannelType]bool),
		negotiatedCodecs:   make(map[int]bool),
		rand:               rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Name returns the module name.
func (p *ChannelPool) Name() string {
	return "ChannelPool"
}

// ModuleStats returns module statistics.
func (p *ChannelPool) ModuleStats() interface{} {
	return p.Stats()
}

// Stats returns pool statistics.
func (p *ChannelPool) Stats() ChannelPoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	available := 0
	unavailable := 0
	for _, info := range p.channels {
		if info.State == StateAvailable {
			available++
		} else {
			unavailable++
		}
	}

	return ChannelPoolStats{
		TotalChannels: len(p.channelList),
		Available:     available,
		Unavailable:   unavailable,
		AdaptiveMTU:   p.GetAdaptiveMTU(),
		TotalSends:    p.totalSends(),
		TotalErrors:   p.totalErrors(),
	}
}

// ChannelPoolStats holds pool statistics.
type ChannelPoolStats struct {
	TotalChannels int
	Available     int
	Unavailable   int
	AdaptiveMTU   int
	TotalSends    int64
	TotalErrors   int64
}

func (p *ChannelPool) totalSends() int64 {
	var total int64
	for _, info := range p.channels {
		total += info.SendCount
	}
	return total
}

func (p *ChannelPool) totalErrors() int64 {
	var total int64
	for _, info := range p.channels {
		total += info.ErrorCount
	}
	return total
}

// AddChannel adds a channel to the pool.
func (p *ChannelPool) AddChannel(ch Channel, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ch == nil {
		return errors.New("channel cannot be nil")
	}
	if id == "" {
		return errors.New("channel ID cannot be empty")
	}
	if _, exists := p.channels[id]; exists {
		return errors.New("channel already exists: " + id)
	}

	mtu := ch.DefaultMTU()
	if mtu <= 0 {
		mtu = 1024 // Default fallback
	}

	info := &ChannelInfo{
		Channel:      ch,
		ID:           id,
		Type:         ch.Type(),
		MTU:          mtu,
		HealthScore:  1.0,
		State:        StateAvailable,
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
		return errors.New("channel not found: " + id)
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

// SelectChannel selects a channel using health-weighted random selection.
// This is the ONLY selection algorithm.
//
// Algorithm: Roulette Wheel Selection
// 1. Filter: exclude channels with StateUnavailable
// 2. Weight: use HealthScore as weight
// 3. Random: select based on weighted probability
//
// Parameter: exclude - list of channel types to exclude (e.g., failed channels)
func (p *ChannelPool) SelectChannel(exclude []ChannelType) (*ChannelInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.channelList) == 0 {
		return nil, ErrChannelNotReady
	}

	// Build exclude set
	excludeSet := make(map[ChannelType]bool)
	for _, t := range exclude {
		excludeSet[t] = true
	}

	// Filter candidates
	candidates := make([]*ChannelInfo, 0)
	weights := make([]float64, 0)
	totalWeight := 0.0

	for _, id := range p.channelList {
		info := p.channels[id]
		// Skip unavailable or excluded
		if info.State == StateUnavailable {
			continue
		}
		if excludeSet[info.Type] {
			continue
		}
		if !info.Channel.IsConnected() {
			continue
		}

		candidates = append(candidates, info)
		weight := info.HealthScore
		if weight < MinHealthScore {
			weight = MinHealthScore // Minimum weight for selection
		}
		weights = append(weights, weight)
		totalWeight += weight
	}

	if len(candidates) == 0 {
		return nil, ErrChannelNotReady
	}

	// Roulette wheel selection
	if totalWeight <= 0 {
		// Fallback to uniform random if all weights are 0
		idx := p.rand.Intn(len(candidates))
		return candidates[idx], nil
	}

	// Select based on weighted probability
	r := p.rand.Float64() * totalWeight
	cumulative := 0.0
	for i, weight := range weights {
		cumulative += weight
		if r <= cumulative {
			return candidates[i], nil
		}
	}

	// Fallback to last candidate
	return candidates[len(candidates)-1], nil
}

// MarkUnavailable marks a channel as unavailable.
// Triggered by: error or timeout (3s no ACK for unreliable channels).
func (p *ChannelPool) MarkUnavailable(chType ChannelType) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, info := range p.channels {
		if info.Type == chType {
			info.State = StateUnavailable
			info.LastError = time.Now()
			info.HealthScore = evaluateHealth(info)
		}
	}
}

// MarkAvailable marks a channel as available again.
// Triggered by: successful sends reaching RecoveryThreshold.
func (p *ChannelPool) MarkAvailable(chType ChannelType) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, info := range p.channels {
		if info.Type == chType {
			info.State = StateAvailable
			info.RecoveryCount = 0
			info.HealthScore = evaluateHealth(info)
		}
	}
}

// RecordSend records a successful send.
func (p *ChannelPool) RecordSend(id string, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if info, exists := p.channels[id]; exists {
		info.SendCount++
		info.LastActivity = time.Now()
		info.LastLatency = latency

		// Recovery mechanism
		if info.State == StateUnavailable || info.State == StateRecovering {
			info.RecoveryCount++
			if info.RecoveryCount >= RecoveryThreshold {
				info.State = StateAvailable
				info.RecoveryCount = 0
			} else {
				info.State = StateRecovering
			}
		}

		info.HealthScore = evaluateHealth(info)
	}
}

// RecordError records a send error.
func (p *ChannelPool) RecordError(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if info, exists := p.channels[id]; exists {
		info.ErrorCount++
		info.LastError = time.Now()

		// Check if should mark unavailable
		if info.HealthScore < MinHealthScore || shouldMarkUnavailable(info) {
			info.State = StateUnavailable
		}

		info.HealthScore = evaluateHealth(info)
	}
}

// shouldMarkUnavailable determines if channel should be marked unavailable.
func shouldMarkUnavailable(info *ChannelInfo) bool {
	// High error rate
	totalOps := info.SendCount + info.ErrorCount
	if totalOps > 10 {
		errorRate := float64(info.ErrorCount) / float64(totalOps)
		if errorRate > 0.5 {
			return true
		}
	}

	// Recent consecutive errors
	if time.Since(info.LastError) < UnavailableTimeout && info.ErrorCount > 3 {
		return true
	}

	return false
}

// evaluateHealth calculates health score for a channel.
func evaluateHealth(info *ChannelInfo) float64 {
	if info == nil {
		return 0
	}

	// Error rate score
	totalOps := info.SendCount + info.ErrorCount
	if totalOps == 0 {
		totalOps = 1
	}
	errorRate := float64(info.ErrorCount) / float64(totalOps)
	errorScore := 1.0 - errorRate
	if errorScore < 0 {
		errorScore = 0
	}

	// Latency score
	latencyScore := 1.0
	if info.LastLatency > 0 {
		latencyScore = 1.0 - float64(info.LastLatency)/float64(MaxLatency)
		if latencyScore < 0 {
			latencyScore = 0
		}
	}

	// Activity score
	activityScore := 1.0
	if info.State == StateUnavailable {
		activityScore = 0.1
	} else if info.State == StateRecovering {
		activityScore = 0.5
	}

	// Weighted average
	return errorScore*ErrorWeight + latencyScore*LatencyWeight + activityScore*ActivityWeight
}

// GetAdaptiveMTU returns minimum MTU across available channels.
func (p *ChannelPool) GetAdaptiveMTU() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	minMTU := -1
	for _, info := range p.channels {
		if info.State == StateAvailable && info.Channel.IsConnected() {
			if minMTU < 0 || info.MTU < minMTU {
				minMTU = info.MTU
			}
		}
	}

	if minMTU < 0 {
		return 1024 // Default
	}
	return minMTU
}

// GetHealthScore returns health score for a channel.
func (p *ChannelPool) GetHealthScore(id string) (float64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	info, exists := p.channels[id]
	if !exists {
		return 0, errors.New("channel not found: " + id)
	}
	return info.HealthScore, nil
}

// GetChannel returns channel info by ID.
func (p *ChannelPool) GetChannel(id string) (*ChannelInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	info, exists := p.channels[id]
	if !exists {
		return nil, errors.New("channel not found: " + id)
	}
	return info, nil
}

// GetChannelIDs returns all channel IDs.
func (p *ChannelPool) GetChannelIDs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]string, len(p.channelList))
	copy(result, p.channelList)
	return result
}

// Count returns total channel count.
func (p *ChannelPool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.channelList)
}

// AvailableCount returns available channel count.
func (p *ChannelPool) AvailableCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, info := range p.channels {
		if info.State == StateAvailable && info.Channel.IsConnected() {
			count++
		}
	}
	return count
}

// CloseAll closes all channels.
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

// SetNegotiatedChannels sets available channels from negotiation result.
func (p *ChannelPool) SetNegotiatedChannels(channels map[ChannelType]bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.negotiatedChannels = channels
}

// SetNegotiatedCodecs sets available codecs from negotiation result.
func (p *ChannelPool) SetNegotiatedCodecs(codecs map[int]bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.negotiatedCodecs = codecs
}

// GetNegotiatedChannels returns negotiated channel types.
func (p *ChannelPool) GetNegotiatedChannels() map[ChannelType]bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make(map[ChannelType]bool)
	for k, v := range p.negotiatedChannels {
		result[k] = v
	}
	return result
}

// GetNegotiatedCodecs returns negotiated codec IDs.
func (p *ChannelPool) GetNegotiatedCodecs() map[int]bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make(map[int]bool)
	for k, v := range p.negotiatedCodecs {
		result[k] = v
	}
	return result
}
