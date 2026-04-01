// Package main provides an interactive VoidBus client example.
//
// This example demonstrates:
// - Creating a VoidBus client
// - Registering codecs
// - Establishing TCP connection
// - Negotiation handshake
// - Interactive message sending/receiving
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Script-OS/VoidBus"
	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/channel/tcp"
	"github.com/Script-OS/VoidBus/codec/base64"
	"github.com/Script-OS/VoidBus/negotiate"
)

const (
	serverAddr = "localhost:8080"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║           VoidBus Interactive Client Example              ║")
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Println("║ Commands:                                                  ║")
	fmt.Println("║   <message>  - Send message to server                      ║")
	fmt.Println("║   quit       - Exit client                                 ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// === Step 1: Create Bus ===
	fmt.Println("[1/6] Creating VoidBus...")
	bus, err := voidbus.New()
	if err != nil {
		log.Fatalf("Failed to create bus: %v", err)
	}
	fmt.Println("      ✓ Bus created successfully")

	// === Step 2: Register Codec ===
	fmt.Println("[2/6] Registering codecs...")
	codec := base64.New()
	if err := bus.RegisterCodec(codec); err != nil {
		log.Fatalf("Failed to register codec: %v", err)
	}
	fmt.Printf("      ✓ Registered codec: %s (SecurityLevel: %d)\n",
		codec.Code(), codec.SecurityLevel())

	// === Step 3: Create TCP Channel ===
	fmt.Println("[3/6] Connecting to server...")
	chConfig := channel.ChannelConfig{
		Address: serverAddr,
		Timeout: 5, // 5 seconds connect timeout
	}
	clientCh, err := tcp.NewClientChannel(chConfig)
	if err != nil {
		log.Fatalf("Failed to create TCP channel: %v", err)
	}
	fmt.Printf("      ✓ TCP channel created (ID: %s)\n", clientCh.Type())

	// Add channel to bus
	if err := bus.AddChannel(clientCh); err != nil {
		log.Fatalf("Failed to add channel: %v", err)
	}

	// Connect
	if err := bus.Connect(serverAddr); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	fmt.Printf("      ✓ Connected to %s\n", serverAddr)

	// === Step 4: Negotiation ===
	fmt.Println("[4/6] Performing negotiation handshake...")

	// Create negotiate request (auto-generated from registered codecs)
	request, err := bus.CreateNegotiateRequest()
	if err != nil {
		log.Fatalf("Failed to create negotiate request: %v", err)
	}
	fmt.Printf("      → Client Codec Bitmap: %08b\n", request.CodecBitmap)
	fmt.Printf("      → Client Channel Bitmap: %08b\n", request.ChannelBitmap)
	fmt.Printf("      → SessionNonce: %x\n", request.SessionNonce)

	// Encode and send request
	requestData, err := request.Encode()
	if err != nil {
		log.Fatalf("Failed to encode request: %v", err)
	}
	fmt.Printf("      → Sending negotiate request (%d bytes)\n", len(requestData))

	if err := clientCh.Send(requestData); err != nil {
		log.Fatalf("Failed to send negotiate request: %v", err)
	}

	// Receive response
	responseData, err := clientCh.Receive()
	if err != nil {
		log.Fatalf("Failed to receive negotiate response: %v", err)
	}
	fmt.Printf("      ← Received negotiate response (%d bytes)\n", len(responseData))

	// Decode response
	response, err := negotiate.DecodeNegotiateResponse(responseData)
	if err != nil {
		log.Fatalf("Failed to decode negotiate response: %v", err)
	}
	fmt.Printf("      ← Server Codec Bitmap: %08b\n", response.CodecBitmap)
	fmt.Printf("      ← Server Channel Bitmap: %08b\n", response.ChannelBitmap)
	fmt.Printf("      ← SessionID: %x\n", response.SessionID)
	fmt.Printf("      ← Status: %d (Success=0, Reject=1)\n", response.Status)

	if response.Status != negotiate.NegotiateStatusSuccess {
		log.Fatalf("Negotiation rejected by server")
	}

	// Apply negotiation result
	if err := bus.ApplyNegotiateResponse(response); err != nil {
		log.Fatalf("Failed to apply negotiate response: %v", err)
	}
	fmt.Println("      ✓ Negotiation completed successfully")

	// === Step 5: Start Receive Loop ===
	fmt.Println("[5/6] Starting receive loop...")

	var receivedCount int
	var mu sync.Mutex

	bus.OnMessage(func(data []byte) {
		mu.Lock()
		receivedCount++
		count := receivedCount
		mu.Unlock()

		fmt.Printf("\n📨 [MSG #%d] Received: %s (%d bytes)\n", count, string(data), len(data))
		fmt.Print("> ")
	})

	bus.OnError(func(err error) {
		fmt.Printf("\n❌ Error: %v\n", err)
		fmt.Print("> ")
	})

	if err := bus.StartReceive(); err != nil {
		log.Fatalf("Failed to start receive: %v", err)
	}
	fmt.Println("      ✓ Receive loop started")
	fmt.Println()

	// === Step 6: Interactive Input ===
	fmt.Println("[6/6] Ready for interactive input")
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Println()

	// Setup signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Input reader
	reader := bufio.NewReader(os.Stdin)
	var sendCount int

	// Input loop
	go func() {
		for {
			fmt.Print("> ")
			input, err := reader.ReadString('\n')
			if err != nil {
				if err.Error() == "EOF" {
					fmt.Println("\nExiting...")
					sigChan <- syscall.SIGTERM
					return
				}
				continue
			}

			input = strings.TrimSpace(input)
			if input == "" {
				continue
			}

			if input == "quit" || input == "exit" {
				fmt.Println("Exiting...")
				sigChan <- syscall.SIGTERM
				return
			}

			// Send message
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			sendCount++
			fmt.Printf("📤 [MSG #%d] Sending: %s (%d bytes)...\n", sendCount, input, len(input))

			if err := bus.SendWithContext(ctx, []byte(input)); err != nil {
				fmt.Printf("   ❌ Send failed: %v\n", err)
			} else {
				fmt.Printf("   ✓ Sent successfully\n")
			}
			cancel()
		}
	}()

	// Wait for exit signal
	<-sigChan
	fmt.Println()
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Println("Shutting down...")

	// Stop bus
	if err := bus.Stop(); err != nil {
		log.Printf("Error stopping bus: %v", err)
	}

	// Close channel
	if err := clientCh.Close(); err != nil {
		log.Printf("Error closing channel: %v", err)
	}

	fmt.Println("✓ Client stopped")
	fmt.Printf("Stats: Sent %d messages, Received %d messages\n", sendCount, receivedCount)
}
