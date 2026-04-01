// Package main provides an interactive VoidBus client example using new Dial API.
//
// This example demonstrates:
// - Creating a VoidBus client
// - Registering codecs
// - Using Dial to establish connection (with auto-negotiation)
// - Standard net.Conn Read/Write interface
// - Interactive message sending/receiving
package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
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
)

const (
	serverAddr = "localhost:8080"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║           VoidBus Interactive Client (net.Conn API)       ║")
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Println("║ Commands:                                                  ║")
	fmt.Println("║   <message>  - Send message to server                      ║")
	fmt.Println("║   quit       - Exit client                                 ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// === Step 1: Create Bus ===
	fmt.Println("[1/4] Creating VoidBus...")
	bus, err := voidbus.New()
	if err != nil {
		log.Fatalf("Failed to create bus: %v", err)
	}
	fmt.Println("      ✓ Bus created successfully")

	// === Step 2: Register Codec ===
	fmt.Println("[2/4] Registering codecs...")
	codec := base64.New()
	if err := bus.RegisterCodec(codec); err != nil {
		log.Fatalf("Failed to register codec: %v", err)
	}
	fmt.Printf("      ✓ Registered codec: %s (SecurityLevel: %d)\n",
		codec.Code(), codec.SecurityLevel())

	// === Step 3: Create and Register Channel ===
	fmt.Println("[3/4] Creating TCP channel...")
	chConfig := channel.ChannelConfig{
		Address: serverAddr,
		Timeout: 5 * time.Second,
	}
	clientCh, err := tcp.NewClientChannel(chConfig)
	if err != nil {
		log.Fatalf("Failed to create TCP channel: %v", err)
	}
	fmt.Printf("      ✓ TCP channel created (Type: %s)\n", clientCh.Type())

	// Add channel to bus
	if err := bus.AddChannel(clientCh); err != nil {
		log.Fatalf("Failed to add channel: %v", err)
	}

	// === Step 4: Dial (Auto-negotiation) ===
	fmt.Println("[4/4] Dialing server (auto-negotiation)...")

	// Dial returns net.Conn (standard Go interface)
	conn, err := bus.Dial(clientCh)
	if err != nil {
		log.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	fmt.Printf("      ✓ Connected and negotiated successfully\n")
	fmt.Printf("      ✓ RemoteAddr: %s\n", conn.RemoteAddr().String())
	fmt.Println()

	// === Interactive Session ===
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Println("Ready for interactive input (net.Conn Read/Write)")
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Println()

	// Setup signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Receive goroutine (using standard net.Conn Read)
	var receivedCount int
	var mu sync.Mutex
	receiveDone := make(chan struct{})

	go func() {
		defer close(receiveDone)
		buf := make([]byte, 64*1024) // 64KB buffer for complete message
		for {
			// Set read deadline
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

			n, err := conn.Read(buf)
			if err != nil {
				if netErr, ok := err.(*net.OpError); ok && netErr.Timeout() {
					// Timeout is expected for polling
					continue
				}
				if err == io.EOF {
					fmt.Println("\n🔴 Server closed connection")
					return
				}
				continue
			}

			mu.Lock()
			receivedCount++
			count := receivedCount
			mu.Unlock()

			data := make([]byte, n)
			copy(data, buf[:n])
			fmt.Printf("\n📨 [MSG #%d] Received: %s (%d bytes)\n", count, string(data), n)
			fmt.Print("> ")
		}
	}()

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

			// Send message using standard net.Conn Write
			sendCount++
			fmt.Printf("📤 [MSG #%d] Sending: %s (%d bytes)...\n", sendCount, input, len(input))

			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			n, err := conn.Write([]byte(input))
			if err != nil {
				fmt.Printf("   ❌ Send failed: %v\n", err)
			} else {
				fmt.Printf("   ✓ Sent %d bytes successfully\n", n)
			}
		}
	}()

	// Wait for exit signal
	<-sigChan
	fmt.Println()
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Println("Shutting down...")

	// Close connection
	conn.Close()

	// Stop bus
	bus.Stop()

	fmt.Println("✓ Client stopped")
	fmt.Printf("Stats: Sent %d messages, Received %d messages\n", sendCount, receivedCount)
}
