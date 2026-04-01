// Package main provides a basic example of VoidBus v2.0 usage.
package main

import (
	"fmt"
	"log"

	"github.com/Script-OS/VoidBus"
	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/channel/tcp"
	"github.com/Script-OS/VoidBus/codec/aes"
	"github.com/Script-OS/VoidBus/codec/base64"
)

func main() {
	// Create a new VoidBus v2.0 instance
	bus, err := voidbus.New()
	if err != nil {
		log.Fatal("Failed to create VoidBus:", err)
	}

	// Configure key for encryption
	if err := bus.SetKey([]byte("32-byte-secret-key-for-aes-256-gcm")); err != nil {
		log.Fatal("Failed to set key:", err)
	}

	// Add codecs with user-defined codes
	// Code is obtained from codec.Code(), default values:
	// - aes.Code() = "aes"
	// - base64.Code() = "base64"
	// Users can customize via SetCode():
	//   aesCodec := aes.NewAES256Codec()
	//   aesCodec.SetCode("my-aes")  // Custom code
	//   bus.RegisterCodec(aesCodec)
	bus.RegisterCodec(aes.NewAES256Codec())
	bus.RegisterCodec(base64.New())

	// Set maximum codec chain depth
	bus.SetMaxCodecDepth(2)

	// Add channels
	// Create TCP client channel with config
	tcpChannel, err := tcp.NewClientChannel(channel.ChannelConfig{
		Address: "localhost:8080",
	})
	if err != nil {
		log.Fatal("Failed to create TCP channel:", err)
	}
	bus.AddChannel(tcpChannel)

	// Connect to remote
	if err := bus.Connect("localhost:8080"); err != nil {
		log.Fatal("Connect failed:", err)
	}

	// Perform capability negotiation
	// In real scenario, exchange supportedCodes with remote
	localCodes, localDepth := bus.GetNegotiationInfo()
	fmt.Printf("Local codes: %v, max depth: %d\n", localCodes, localDepth)

	// Simulate negotiation with remote (in real use, exchange via protocol)
	// For now, assume remote has same codes
	remoteCodes := []string{"aes", "base64"}
	remoteDepth := 2
	salt := []byte("negotiation-salt-32-bytes-long-key")

	_, err = bus.Negotiate(remoteCodes, remoteDepth, salt)
	if err != nil {
		log.Fatal("Negotiation failed:", err)
	}

	// Start receive loop (for callback mode)
	bus.OnMessage(func(data []byte) {
		fmt.Printf("Received: %s\n", string(data))
	})
	bus.OnError(func(err error) {
		log.Printf("Error: %v", err)
	})

	// Send data
	// - Creates new session
	// - Randomly selects codec chain (e.g., A→B or just A)
	// - Fragments data based on channel MTU
	// - Distributes across channels
	err = bus.Send([]byte("Hello, VoidBus v2.0!"))
	if err != nil {
		log.Fatal("Send failed:", err)
	}

	// For blocking receive mode:
	// data, err := bus.Receive()
	// if err != nil {
	//     log.Fatal("Receive failed:", err)
	// }
	// fmt.Printf("Received: %s\n", string(data))

	// Get statistics
	stats := bus.Stats()
	fmt.Printf("Stats: connected=%v, negotiated=%v, channels=%d, codecs=%d\n",
		stats.Connected, stats.Negotiated, stats.ChannelCount, stats.CodecCount)

	// Cleanup
	bus.Close()
}
