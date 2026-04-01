// Package voidbus provides net.Addr implementation for VoidBus connections.
package voidbus

import "net"

// voidBusAddr implements net.Addr for VoidBus connections.
type voidBusAddr struct {
	network string // Network type (e.g., "voidbus", "voidbus-tcp")
	address string // Address string (e.g., channel ID or remote address)
}

// Network returns the network type.
func (a *voidBusAddr) Network() string {
	return a.network
}

// String returns the address string.
func (a *voidBusAddr) String() string {
	return a.address
}

// NewVoidBusAddr creates a new VoidBus address.
func NewVoidBusAddr(network, address string) net.Addr {
	return &voidBusAddr{
		network: network,
		address: address,
	}
}
