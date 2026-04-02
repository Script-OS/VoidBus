// Package voidbus provides net.Conn implementation for VoidBus connections.
//
// voidBusConn implements message-oriented semantics:
// - Read: returns ONE complete message (reassembled, decoded) per call
// - Write: writes ONE complete message (encoded, fragmented, sent across multiple channels)
// - Deadline: timeout for complete message reassembly/encoding/sending
//
// Read semantics:
// - Each Read returns exactly one complete message
// - Returns n = len(complete_message)
// - If buffer is smaller than message, returns ErrBufferTooSmall with n = required_size
// - User should allocate buffer >= required_size and retry
package voidbus

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/channel"
)

// ErrBufferTooSmall indicates the user buffer is smaller than the complete message.
// n returned by Read indicates the required buffer size.
var ErrBufferTooSmall = errors.New("voidbus: buffer too small for complete message")

// voidBusConn implements net.Conn for VoidBus connections.
// Message-oriented semantics: each Read/Write operates on a complete message.
type voidBusConn struct {
	bus         *Bus
	channelID   string              // The channel ID used for negotiation (assigned by Bus)
	channelType channel.ChannelType // The channel type

	// Receive state (no partial read support - complete message per Read)
	recvMu   sync.Mutex
	recvChan chan []byte // Complete message channel (from bus receive loop)

	// Deadline
	readDeadline  time.Time
	writeDeadline time.Time

	// Address info
	localAddr  net.Addr
	remoteAddr net.Addr

	// State
	closed  bool
	closeMu sync.Mutex
}

// Read reads ONE complete message from the connection.
// Returns n = size of complete message (reassembled, decoded).
// If buffer is smaller than message, returns ErrBufferTooSmall with n = required_size.
// No partial read - each Read returns exactly one complete message.
func (c *voidBusConn) Read(b []byte) (n int, err error) {
	c.recvMu.Lock()
	defer c.recvMu.Unlock()

	// Check if closed
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return 0, net.ErrClosed
	}
	c.closeMu.Unlock()

	// Wait for complete message
	// If no deadline set (zero value), wait indefinitely
	if c.readDeadline.IsZero() {
		select {
		case data := <-c.recvChan:
			// Complete message arrived
			if len(data) > len(b) {
				// Buffer too small, return required size
				return len(data), ErrBufferTooSmall
			}
			copy(b, data)
			return len(data), nil
		case <-c.bus.stopChan:
			return 0, net.ErrClosed
		}
	}

	// Wait with deadline
	select {
	case data := <-c.recvChan:
		// Complete message arrived
		if len(data) > len(b) {
			// Buffer too small, return required size
			return len(data), ErrBufferTooSmall
		}
		copy(b, data)
		return len(data), nil

	case <-time.After(time.Until(c.readDeadline)):
		// Return net.Conn compliant timeout error
		return 0, &net.OpError{
			Op:     "read",
			Net:    "voidbus",
			Source: c.localAddr,
			Addr:   c.remoteAddr,
			Err:    os.ErrDeadlineExceeded,
		}

	case <-c.bus.stopChan:
		return 0, net.ErrClosed
	}
}

// Write writes ONE complete message to the connection.
// Returns n = size of the original message (before encoding/fragmentation).
// VoidBus internally handles: encoding, fragmentation, multi-channel distribution.
func (c *voidBusConn) Write(b []byte) (n int, err error) {
	// Check write deadline
	if !c.writeDeadline.IsZero() && time.Now().After(c.writeDeadline) {
		return 0, &net.OpError{
			Op:     "write",
			Net:    "voidbus",
			Source: c.localAddr,
			Addr:   c.remoteAddr,
			Err:    os.ErrDeadlineExceeded,
		}
	}

	// Check if closed
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return 0, net.ErrClosed
	}
	c.closeMu.Unlock()

	// Use bus SendWithContext (handles encoding, fragmentation, multi-channel)
	ctx := context.Background()
	if !c.writeDeadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, c.writeDeadline)
		defer cancel()
	}

	if err := c.bus.sendInternal(ctx, b); err != nil {
		return 0, err
	}

	// Return original message size
	return len(b), nil
}

// Close closes the connection.
// Calls bus.Stop() to stop the receive loop.
func (c *voidBusConn) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// Stop the bus (this will close stopChan and stop receiveLoop)
	// Note: For client mode, this stops the entire bus
	// For server mode, each client has its own clientBus
	c.bus.Stop()

	return nil
}

// LocalAddr returns the local network address.
func (c *voidBusConn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr returns the remote network address.
func (c *voidBusConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// SetDeadline sets the read and write deadlines.
// ReadDeadline: timeout for complete message reassembly.
// WriteDeadline: timeout for complete message encoding/sending.
func (c *voidBusConn) SetDeadline(t time.Time) error {
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

// SetReadDeadline sets the deadline for complete message reassembly.
func (c *voidBusConn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

// SetWriteDeadline sets the deadline for complete message encoding/sending.
func (c *voidBusConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}

// ChannelID returns the channel ID used for this connection.
func (c *voidBusConn) ChannelID() string {
	return c.channelID
}

// newVoidBusConn creates a new VoidBus connection.
func newVoidBusConn(bus *Bus, channelID string, chType channel.ChannelType, recvChan chan []byte) *voidBusConn {
	conn := &voidBusConn{
		bus:         bus,
		channelID:   channelID,
		channelType: chType,
		recvChan:    recvChan,
	}

	// Set addresses based on channel type
	if channelID != "" {
		network := "voidbus-" + string(chType)
		conn.localAddr = NewVoidBusAddr(network, channelID)
		conn.remoteAddr = NewVoidBusAddr(network, channelID)
	} else {
		conn.localAddr = NewVoidBusAddr("voidbus", "")
		conn.remoteAddr = NewVoidBusAddr("voidbus", "")
	}

	return conn
}
