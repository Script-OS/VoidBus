// Package internal provides adaptive timeout utilities for VoidBus v2.0.
// Implements RFC 6298-style RTT-based timeout calculation.
package internal

import (
	"sync"
	"time"
)

// AdaptiveTimeout implements adaptive timeout calculation.
// Based on RFC 6298 TCP retransmission timeout algorithm.
type AdaptiveTimeout struct {
	mu sync.RWMutex

	// Estimated RTT
	srtt time.Duration // Smoothed RTT

	// RTT variation
	rttvar time.Duration

	// Minimum timeout
	minTimeout time.Duration

	// Maximum timeout
	maxTimeout time.Duration

	// Current timeout value
	rto time.Duration // Retransmission Timeout

	// First measurement flag
	firstMeasurement bool

	// History of RTT samples
	samples    []time.Duration
	maxSamples int
}

// NewAdaptiveTimeout creates a new adaptive timeout calculator.
func NewAdaptiveTimeout(minTimeout, maxTimeout time.Duration) *AdaptiveTimeout {
	return &AdaptiveTimeout{
		srtt:             0,
		rttvar:           0,
		rto:              minTimeout,
		minTimeout:       minTimeout,
		maxTimeout:       maxTimeout,
		firstMeasurement: true,
		samples:          make([]time.Duration, 0, 10),
		maxSamples:       10,
	}
}

// AddMeasurement adds a new RTT measurement and updates timeout.
// Implements RFC 6298 algorithm:
//
//	On first measurement:
//	  SRTT = R
//	  RTTVAR = R/2
//	  RTO = SRTT + max(G, 4*RTTVAR)
//
//	On subsequent measurements:
//	  RTTVAR = (1 - beta) * RTTVAR + beta * |SRTT - R|
//	  SRTT = (1 - alpha) * SRTT + alpha * R
//	  RTO = SRTT + max(G, 4*RTTVAR)
//
//	Where alpha = 1/8, beta = 1/4, G = clock granularity
func (t *AdaptiveTimeout) AddMeasurement(rtt time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Store sample for statistics
	t.samples = append(t.samples, rtt)
	if len(t.samples) > t.maxSamples {
		t.samples = t.samples[1:]
	}

	// RFC 6298 constants
	alpha := 1.0 / 8.0                   // 0.125
	beta := 1.0 / 4.0                    // 0.25
	clockGranularity := time.Millisecond // G

	if t.firstMeasurement {
		t.srtt = rtt
		t.rttvar = rtt / 2
		t.firstMeasurement = false
	} else {
		// Update RTTVAR
		delta := t.srtt - rtt
		if delta < 0 {
			delta = -delta
		}
		t.rttvar = time.Duration((1-beta)*float64(t.rttvar) + beta*float64(delta))

		// Update SRTT
		t.srtt = time.Duration((1-alpha)*float64(t.srtt) + alpha*float64(rtt))
	}

	// Calculate RTO
	t.rto = t.srtt + maxDuration(clockGranularity, 4*t.rttvar)

	// Clamp to min/max
	if t.rto < t.minTimeout {
		t.rto = t.minTimeout
	}
	if t.rto > t.maxTimeout {
		t.rto = t.maxTimeout
	}
}

// GetTimeout returns current timeout value.
func (t *AdaptiveTimeout) GetTimeout() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.rto
}

// GetSRTT returns smoothed RTT.
func (t *AdaptiveTimeout) GetSRTT() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.srtt
}

// GetRTTVAR returns RTT variation.
func (t *AdaptiveTimeout) GetRTTVAR() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.rttvar
}

// GetAverage returns average of all RTT samples.
func (t *AdaptiveTimeout) GetAverage() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.samples) == 0 {
		return 0
	}

	var total time.Duration
	for _, s := range t.samples {
		total += s
	}
	return total / time.Duration(len(t.samples))
}

// Reset resets the timeout calculator to initial state.
func (t *AdaptiveTimeout) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.srtt = 0
	t.rttvar = 0
	t.rto = t.minTimeout
	t.firstMeasurement = true
	t.samples = make([]time.Duration, 0, t.maxSamples)
}

// Backoff applies exponential backoff for retransmission.
// RTO = min(maxRTO, RTO * 2)
func (t *AdaptiveTimeout) Backoff() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.rto = t.rto * 2
	if t.rto > t.maxTimeout {
		t.rto = t.maxTimeout
	}
}

// SampleCount returns number of samples collected.
func (t *AdaptiveTimeout) SampleCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.samples)
}

// maxDuration returns the larger of two durations.
func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// DefaultTimeoutConfig returns default timeout configuration.
func DefaultTimeoutConfig() *AdaptiveTimeout {
	return NewAdaptiveTimeout(
		1*time.Second,  // Min timeout
		30*time.Second, // Max timeout
	)
}
