// Package main provides an encrypted communication example.
//
// This example demonstrates:
// - AES-256-GCM encryption (SecurityLevelHigh)
// - KeyProvider for key management
// - Secure client-server communication
//
// Security Note: This example uses a hardcoded key for demonstration.
// In production, keys should be loaded from secure sources.
//
// Run: go run examples/encrypted/main.go
package main

import (
	"fmt"
	"log"
	"time"

	"VoidBus/channel"
	"VoidBus/channel/tcp"
	"VoidBus/codec"
	"VoidBus/codec/aes"
	"VoidBus/core"
	"VoidBus/keyprovider/embedded"
	plainserializer "VoidBus/serializer/plain"
)

// Demo key (32 bytes for AES-256)
// WARNING: In production, never hardcode keys in source code!
var demoKey = []byte("this-is-a-demo-key-32bytes-long!")

func main() {
	fmt.Println("=== VoidBus Encrypted Communication Example ===")
	fmt.Println("Using AES-256-GCM encryption (SecurityLevelHigh)")
	fmt.Println()

	serverAddr := "127.0.0.1:9094"

	// Create embedded key provider
	keyProvider, err := embedded.New(demoKey, "demo-key-001", "AES-256-GCM")
	if err != nil {
		log.Fatalf("Failed to create key provider: %v", err)
	}
	fmt.Printf("KeyProvider created with embedded key\n")

	// Start server
	go runServer(serverAddr, keyProvider)

	// Wait for server to be ready
	time.Sleep(200 * time.Millisecond)

	// Run client
	runClient(serverAddr, keyProvider)

	fmt.Println()
	fmt.Println("=== Example Complete ===")
}

func runServer(addr string, kp *embedded.Provider) {
	// Create server channel
	serverConfig := channel.ChannelConfig{
		Address: addr,
	}

	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		log.Fatalf("Server: Failed to create channel: %v", err)
	}
	defer serverCh.Close()

	fmt.Printf("Server listening on %s\n", addr)

	// Accept one client
	clientCh, err := serverCh.Accept()
	if err != nil {
		log.Printf("Server: Failed to accept: %v", err)
		return
	}
	defer clientCh.Close()
	fmt.Printf("Server: Client connected\n")

	// Create AES-256 codec
	aesCodec := aes.NewAES256Codec()

	// Create server Bus with AES encryption
	serverBus := core.NewBuilder().
		UseSerializerInstance(plainserializer.New()).
		UseCodecChain(codec.NewChain().AddCodec(aesCodec)).
		UseChannel(clientCh).
		UseKeyProvider(kp).
		Build()

	if err := serverBus.Start(); err != nil {
		log.Printf("Server: Failed to start bus: %v", err)
		return
	}
	defer serverBus.Stop()

	fmt.Printf("Server Bus started (SecurityLevel: %d)\n", serverBus.SecurityLevel())

	// Receive encrypted message
	data, err := serverBus.Receive()
	if err != nil {
		log.Printf("Server: Failed to receive: %v", err)
		return
	}
	fmt.Printf("Server received (decrypted): %s\n", string(data))

	// Send encrypted response
	response := []byte("Secure response from server!")
	if err := serverBus.Send(response); err != nil {
		log.Printf("Server: Failed to send: %v", err)
		return
	}
	fmt.Printf("Server sent (encrypted): %s\n", string(response))
}

func runClient(addr string, kp *embedded.Provider) {
	// Create client channel
	clientConfig := channel.ChannelConfig{
		Address: addr,
		Timeout: 5,
	}

	clientCh, err := tcp.NewClientChannel(clientConfig)
	if err != nil {
		log.Fatalf("Client: Failed to connect: %v", err)
	}
	defer clientCh.Close()

	fmt.Printf("Client connected to %s\n", addr)

	// Create AES-256 codec
	aesCodec := aes.NewAES256Codec()

	// Create client Bus with AES encryption
	clientBus := core.NewBuilder().
		UseSerializerInstance(plainserializer.New()).
		UseCodecChain(codec.NewChain().AddCodec(aesCodec)).
		UseChannel(clientCh).
		UseKeyProvider(kp).
		Build()

	if err := clientBus.Start(); err != nil {
		log.Fatalf("Client: Failed to start bus: %v", err)
	}
	defer clientBus.Stop()

	fmt.Printf("Client Bus started (SecurityLevel: %d)\n", clientBus.SecurityLevel())

	// Send encrypted message
	message := []byte("Secret message from client!")
	if err := clientBus.Send(message); err != nil {
		log.Printf("Client: Failed to send: %v", err)
		return
	}
	fmt.Printf("Client sent (encrypted): %s\n", string(message))

	// Receive encrypted response
	response, err := clientBus.Receive()
	if err != nil {
		log.Printf("Client: Failed to receive: %v", err)
		return
	}
	fmt.Printf("Client received (decrypted): %s\n", string(response))
}
