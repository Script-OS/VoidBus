// Package main provides a simple example of VoidBus usage.
//
// This example demonstrates:
// - Creating a Bus with Plain serializer and Plain codec
// - Sending and receiving raw bytes
// - Using TCP channel for transport
//
// Run: go run examples/simple/main.go
package main

import (
	"fmt"
	"log"
	"time"

	"VoidBus/channel"
	"VoidBus/channel/tcp"
	"VoidBus/codec"
	"VoidBus/codec/plain"
	"VoidBus/core"
	plainserializer "VoidBus/serializer/plain"
)

func main() {
	fmt.Println("=== VoidBus Simple Example ===")
	fmt.Println()

	// Example 1: Direct TCP channel communication (no Bus)
	fmt.Println("--- Example 1: Direct TCP Channel ---")
	directTCPExample()

	// Example 2: Bus with Plain serializer and Plain codec
	fmt.Println()
	fmt.Println("--- Example 2: Bus with Plain Codec ---")
	busPlainExample()

	fmt.Println()
	fmt.Println("=== Examples Complete ===")
}

// directTCPExample demonstrates raw TCP channel communication
func directTCPExample() {
	// Start a simple TCP server
	serverAddr := "127.0.0.1:9091"

	// Create server channel
	serverConfig := channel.ChannelConfig{
		Address: serverAddr,
	}

	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		log.Printf("Failed to create server channel: %v", err)
		return
	}
	defer serverCh.Close()

	fmt.Printf("Server listening on %s\n", serverAddr)

	// Start accepting in background
	go func() {
		clientCh, err := serverCh.Accept()
		if err != nil {
			return
		}
		defer clientCh.Close()

		// Receive message
		data, err := clientCh.Receive()
		if err != nil {
			return
		}
		fmt.Printf("Server received: %s\n", string(data))

		// Send response
		response := []byte("Hello from server!")
		clientCh.Send(response)
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create client channel
	clientConfig := channel.ChannelConfig{
		Address: serverAddr,
		Timeout: 5,
	}

	clientCh, err := tcp.NewClientChannel(clientConfig)
	if err != nil {
		log.Printf("Failed to create client channel: %v", err)
		return
	}
	defer clientCh.Close()

	// Send message
	message := []byte("Hello from client!")
	if err := clientCh.Send(message); err != nil {
		log.Printf("Failed to send: %v", err)
		return
	}
	fmt.Printf("Client sent: %s\n", string(message))

	// Receive response
	response, err := clientCh.Receive()
	if err != nil {
		log.Printf("Failed to receive: %v", err)
		return
	}
	fmt.Printf("Client received: %s\n", string(response))
}

// busPlainExample demonstrates Bus with Plain serializer and codec
func busPlainExample() {
	serverAddr := "127.0.0.1:9092"

	// Create server channel
	serverConfig := channel.ChannelConfig{
		Address: serverAddr,
	}

	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		log.Printf("Failed to create server channel: %v", err)
		return
	}
	defer serverCh.Close()

	fmt.Printf("Server listening on %s\n", serverAddr)

	// Server-side: create Bus for each accepted client
	go func() {
		clientCh, err := serverCh.Accept()
		if err != nil {
			return
		}

		// Create server Bus
		serverBus := core.NewBuilder().UseSerializerInstance(plainserializer.New()).UseCodecChain(codec.NewChain().AddCodec(plain.New())).UseChannel(clientCh).Build()

		if err := serverBus.Start(); err != nil {
			clientCh.Close()
			return
		}
		defer serverBus.Stop()

		// Receive message
		data, err := serverBus.Receive()
		if err != nil {
			return
		}
		fmt.Printf("Server Bus received: %s\n", string(data))

		// Send response
		response := []byte("Response from Bus server!")
		serverBus.Send(response)
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Client-side: create Bus
	clientConfig := channel.ChannelConfig{
		Address: serverAddr,
		Timeout: 5,
	}

	clientCh, err := tcp.NewClientChannel(clientConfig)
	if err != nil {
		log.Printf("Failed to create client channel: %v", err)
		return
	}
	defer clientCh.Close()

	clientBus := core.NewBuilder().UseSerializerInstance(plainserializer.New()).UseCodecChain(codec.NewChain().AddCodec(plain.New())).UseChannel(clientCh).Build()

	if err := clientBus.Start(); err != nil {
		log.Printf("Failed to start client bus: %v", err)
		return
	}
	defer clientBus.Stop()

	// Send message
	message := []byte("Hello from Bus client!")
	if err := clientBus.Send(message); err != nil {
		log.Printf("Failed to send: %v", err)
		return
	}
	fmt.Printf("Client Bus sent: %s\n", string(message))

	// Receive response
	response, err := clientBus.Receive()
	if err != nil {
		log.Printf("Failed to receive: %v", err)
		return
	}
	fmt.Printf("Client Bus received: %s\n", string(response))

	fmt.Printf("Session ID: %s\n", clientBus.GetSessionID())
	fmt.Printf("Security Level: %d\n", clientBus.SecurityLevel())
}
