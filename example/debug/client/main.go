// Package main provides a debug VoidBus client for testing.
package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/Script-OS/VoidBus"
	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/channel/tcp"
	"github.com/Script-OS/VoidBus/codec/base64"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds | log.Lshortfile)
	fmt.Println("=== Debug VoidBus Client ===")

	// 1. Create Bus
	bus, err := voidbus.New(nil)
	if err != nil {
		log.Fatalf("Failed to create bus: %v", err)
	}
	bus.SetDebugMode(true)
	fmt.Println("[1] Bus created (debug mode enabled)")

	// 2. Register Codec
	codec := base64.New()
	if err := bus.RegisterCodec(codec); err != nil {
		log.Fatalf("Failed to register codec: %v", err)
	}
	fmt.Printf("[2] Codec registered: %s\n", codec.Code())

	// 3. Create Client Channel
	chConfig := channel.ChannelConfig{
		Address: "localhost:8081",
		Timeout: 5 * time.Second,
	}
	clientCh, err := tcp.NewClientChannel(chConfig)
	if err != nil {
		log.Fatalf("Failed to create client channel: %v", err)
	}
	fmt.Println("[3] Client channel created")

	if err := bus.AddChannel(clientCh); err != nil {
		log.Fatalf("Failed to add channel: %v", err)
	}

	// 4. Dial (auto negotiation)
	fmt.Println("[4] Dialing server...")
	conn, err := bus.Dial(clientCh)
	if err != nil {
		log.Fatalf("Failed to dial: %v", err)
	}
	fmt.Printf("[4] Connected: %s\n", conn.RemoteAddr().String())
	defer conn.Close()

	// 5. Send message to server
	msg1 := "Hello Server from Client!"
	fmt.Printf("[5] Sending message: %s (%d bytes)\n", msg1, len(msg1))
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Write([]byte(msg1))
	if err != nil {
		log.Fatalf("Failed to write: %v", err)
	}
	fmt.Printf("[5] Write completed: n=%d\n", n)

	// 6. Receive reply from server (with debug)
	buf := make([]byte, 64*1024)
	fmt.Println("[6] Waiting for reply from server...")

	for i := 0; i < 10; i++ {
		fmt.Printf("[6.%d] Attempting read...\n", i+1)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(*net.OpError); ok && netErr.Timeout() {
				fmt.Printf("[6.%d] Read timeout, retrying...\n", i+1)
				continue
			}
			log.Printf("[6.%d] Read error: %v\n", i+1, err)
			continue
		}
		fmt.Printf("[6.%d] Received reply: %s (%d bytes)\n", i+1, string(buf[:n]), n)
		break
	}

	// Cleanup
	fmt.Println("[7] Closing connection...")
	conn.Close()
	bus.Stop()
	fmt.Println("[7] Client stopped")
	fmt.Println("=== Test Complete: Client OK ===")
}
