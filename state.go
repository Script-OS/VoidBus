// Package voidbus provides state management for Bus.
package voidbus

import (
	"errors"
)

// BusState defines the lifecycle states of a Bus.
// Uses int32 to support atomic operations.
type BusState int32

const (
	// StateIdle is the initial state, unused.
	StateIdle BusState = iota

	// StateConnected indicates connected but not negotiated.
	StateConnected

	// StateNegotiated indicates negotiated and ready for communication.
	StateNegotiated

	// StateRunning indicates running (receive loop started).
	StateRunning

	// StateClosed indicates closed.
	StateClosed
)

// State transition errors
var (
	ErrInvalidStateTransition = errors.New("invalid state transition")
	ErrBusClosed              = errors.New("bus is closed")
)

// setState sets the new state with validation.
// This method holds b.mu internally, external callers should NOT hold the lock.
// See docs/LOCKING.md §5.2 for locking principles.
func (b *Bus) setState(newState BusState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	currentState := BusState(b.state.Load())

	// State transition validation (prevent illegal transitions)
	// See docs/ARCHITECTURE.md §14.2 for transition rules
	switch currentState {
	case StateIdle:
		if newState != StateConnected && newState != StateRunning {
			return ErrInvalidStateTransition
		}
	case StateConnected:
		if newState != StateNegotiated && newState != StateClosed {
			return ErrInvalidStateTransition
		}
	case StateNegotiated:
		if newState != StateRunning && newState != StateClosed {
			return ErrInvalidStateTransition
		}
	case StateRunning:
		if newState != StateClosed {
			return ErrInvalidStateTransition
		}
	case StateClosed:
		return ErrBusClosed // Already closed, cannot transition
	}

	b.state.Store(int32(newState))
	return nil
}

// getState returns the current state (lock-free, atomic operation).
// See docs/LOCKING.md §5.3 for locking principles.
func (b *Bus) getState() BusState {
	return BusState(b.state.Load())
}

// State query methods (fast, lock-free)
// These methods use atomic operations for state checks without holding locks.

// isRunning returns true if the bus is in running state.
func (b *Bus) isRunning() bool {
	return b.getState() == StateRunning
}

// isNegotiated returns true if the bus is negotiated or in later states.
func (b *Bus) isNegotiated() bool {
	return b.getState() >= StateNegotiated
}

// isClosed returns true if the bus is closed.
func (b *Bus) isClosed() bool {
	return b.getState() == StateClosed
}

// isConnected returns true if the bus is connected or in later states.
func (b *Bus) isConnected() bool {
	return b.getState() >= StateConnected
}
