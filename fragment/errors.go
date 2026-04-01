// Package fragment provides error definitions for VoidBus v2.0.
package fragment

import "errors"

// Fragment errors
var (
	// ErrInvalidFragmentSize indicates invalid fragment size
	ErrInvalidFragmentSize = errors.New("fragment: invalid fragment size")
	// ErrFragmentIncomplete indicates fragment group is incomplete
	ErrFragmentIncomplete = errors.New("fragment: incomplete")
	// ErrFragmentMissing indicates some fragments are missing
	ErrFragmentMissing = errors.New("fragment: missing")
	// ErrFragmentCorrupted indicates fragment data is corrupted
	ErrFragmentCorrupted = errors.New("fragment: corrupted")
	// ErrStateNotFound indicates fragment state not found
	ErrStateNotFound = errors.New("fragment: state not found")
	// ErrFragmentTimeout indicates reassembly timeout
	ErrFragmentTimeout = errors.New("fragment: timeout")
	// ErrFragmentMismatch indicates fragment ID mismatch
	ErrFragmentMismatch = errors.New("fragment: mismatch")
)
