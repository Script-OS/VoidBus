// Package main provides an interactive VoidBus server example.
//
// This example demonstrates:
// - Creating a VoidBus server
// - Accepting client connections
// - Negotiation handshake handling
// - Multi-client management
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
	serverAddr = ":8080"
)

// ClientSession represents a connected client session.
type ClientSession struct {
	ID       string
	Bus      *voidbus.Bus
	Channel  channel.Channel
	Messages int
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║           VoidBus Interactive Server Example              ║")
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Println("║ Commands:                                                  ║")
	fmt.Println("║   <message>           - Broadcast to all clients           ║")
	fmt.Println("║   <client-id> <msg>   - Send to specific client            ║")
	fmt.Println("║   list                - List connected clients             ║")
	fmt.Println("║   quit                - Exit server                        ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// === Step 1: Create Server Channel ===
	fmt.Println("[1/4] Creating TCP server...")
	serverConfig := channel.ChannelConfig{
		Address: serverAddr,
	}
	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		log.Fatalf("Failed to create server channel: %v", err)
	}
	fmt.Printf("      ✓ Server listening on %s\n", serverAddr)

	// === Step 2: Create Server Negotiator ===
	fmt.Println("[2/4] Setting up negotiator...")

	serverNegotiator := negotiate.NewServerNegotiator(nil)

	// Set supported channels (TCP only for this example)
	channelBitmap := negotiate.NewChannelBitmap(0)
	channelBitmap.SetChannel(negotiate.ChannelBitTCP)
	serverNegotiator.SetChannelBitmap(channelBitmap)
	fmt.Printf("      → Server Channel Bitmap: %08b (TCP)\n", channelBitmap)

	// Set supported codecs (Base64 only for this example)
	codecBitmap := negotiate.NewCodecBitmap(0)
	codecBitmap.SetCodec(negotiate.CodecBitBase64)
	serverNegotiator.SetCodecBitmap(codecBitmap)
	fmt.Printf("      → Server Codec Bitmap: %08b (Base64)\n", codecBitmap)
	fmt.Println("      ✓ Negotiator configured")

	// === Step 3: Client Management ===
	fmt.Println("[3/4] Starting client handler...")
	fmt.Println()

	// Client sessions map
	clients := make(map[string]*ClientSession)
	var clientsMu sync.RWMutex
	var clientCount int

	// Accept clients in background
	go func() {
		for {
			clientCh, err := serverCh.Accept()
			if err != nil {
				// Server closed
				return
			}

			// Generate client ID
			clientID := fmt.Sprintf("client-%03d", clientCount+1)
			clientCount++

			fmt.Printf("🔗 New client connected: %s\n", clientID)

			// Handle client in separate goroutine
			go handleClient(clientID, clientCh, serverNegotiator, &clients, &clientsMu)
		}
	}()

	// === Step 4: Interactive Input ===
	fmt.Println("[4/4] Ready for interactive input")
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Println()

	// Setup signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Input reader
	reader := bufio.NewReader(os.Stdin)

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

			// Parse command
			parts := strings.SplitN(input, " ", 2)
			cmd := parts[0]

			switch cmd {
			case "quit", "exit":
				fmt.Println("Exiting...")
				sigChan <- syscall.SIGTERM
				return

			case "list":
				clientsMu.RLock()
				fmt.Printf("\n📋 Connected clients (%d):\n", len(clients))
				for id, sess := range clients {
					fmt.Printf("   - %s (messages: %d)\n", id, sess.Messages)
				}
				if len(clients) == 0 {
					fmt.Println("   (no clients connected)")
				}
				clientsMu.RUnlock()
				fmt.Println()

			default:
				// Send message
				if len(parts) == 1 {
					// Broadcast to all clients
					broadcastMessage(&clients, &clientsMu, input)
				} else {
					// Send to specific client
					targetClient := parts[0]
					message := parts[1]

					clientsMu.RLock()
					sess, ok := clients[targetClient]
					clientsMu.RUnlock()

					if !ok {
						fmt.Printf("❌ Client '%s' not found\n", targetClient)
						continue
					}

					sendToClient(sess, message)
				}
			}
		}
	}()

	// Wait for exit signal
	<-sigChan
	fmt.Println()
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Println("Shutting down...")

	// Stop all client buses
	clientsMu.Lock()
	for _, sess := range clients {
		sess.Bus.Stop()
		sess.Channel.Close()
	}
	clientsMu.Unlock()

	// Close server
	serverCh.Close()

	fmt.Println("✓ Server stopped")
}

// handleClient handles a connected client session.
func handleClient(
	clientID string,
	clientCh channel.Channel,
	serverNegotiator *negotiate.ServerNegotiatorImpl,
	clients *map[string]*ClientSession,
	clientsMu *sync.RWMutex,
) {
	// === Step 1: Create Bus for this client ===
	bus, err := voidbus.New()
	if err != nil {
		log.Printf("[%s] Failed to create bus: %v", clientID, err)
		clientCh.Close()
		return
	}

	// Register codec
	codec := base64.New()
	if err := bus.RegisterCodec(codec); err != nil {
		log.Printf("[%s] Failed to register codec: %v", clientID, err)
		bus.Stop()
		clientCh.Close()
		return
	}

	// Add channel
	if err := bus.AddChannel(clientCh); err != nil {
		log.Printf("[%s] Failed to add channel: %v", clientID, err)
		bus.Stop()
		clientCh.Close()
		return
	}

	// === Step 2: Negotiation ===
	fmt.Printf("[%s] Waiting for negotiate request...\n", clientID)

	// Receive negotiate request
	requestData, err := clientCh.Receive()
	if err != nil {
		log.Printf("[%s] Failed to receive negotiate request: %v", clientID, err)
		bus.Stop()
		clientCh.Close()
		return
	}
	fmt.Printf("[%s] ← Received negotiate request (%d bytes)\n", clientID, len(requestData))

	// Handle request
	response, err := serverNegotiator.HandleRawRequest(requestData)
	if err != nil {
		log.Printf("[%s] Failed to handle negotiate request: %v", clientID, err)
		bus.Stop()
		clientCh.Close()
		return
	}

	// Decode request for display
	request, err := negotiate.DecodeNegotiateRequest(requestData)
	if err == nil {
		fmt.Printf("[%s] ← Client Codec Bitmap: %08b\n", clientID, request.CodecBitmap)
		fmt.Printf("[%s] ← Client Channel Bitmap: %08b\n", clientID, request.ChannelBitmap)
	}

	fmt.Printf("[%s] → Server Codec Bitmap: %08b\n", clientID, response.CodecBitmap)
	fmt.Printf("[%s] → Server Channel Bitmap: %08b\n", clientID, response.ChannelBitmap)
	fmt.Printf("[%s] → Status: %d\n", clientID, response.Status)

	// Send response
	responseData, err := response.Encode()
	if err != nil {
		log.Printf("[%s] Failed to encode response: %v", clientID, err)
		bus.Stop()
		clientCh.Close()
		return
	}
	fmt.Printf("[%s] → Sending negotiate response (%d bytes)\n", clientID, len(responseData))

	if err := clientCh.Send(responseData); err != nil {
		log.Printf("[%s] Failed to send negotiate response: %v", clientID, err)
		bus.Stop()
		clientCh.Close()
		return
	}

	// Apply negotiation result
	if response.Status == negotiate.NegotiateStatusSuccess {
		if err := bus.SetNegotiatedBitmap(response.CodecBitmap); err != nil {
			log.Printf("[%s] Failed to apply negotiated bitmap: %v", clientID, err)
			bus.Stop()
			clientCh.Close()
			return
		}
		fmt.Printf("[%s] ✓ Negotiation completed\n", clientID)
	} else {
		fmt.Printf("[%s] ✗ Negotiation rejected\n", clientID)
		bus.Stop()
		clientCh.Close()
		return
	}

	// === Step 3: Start Receive ===
	var receivedCount int
	var sessMu sync.Mutex

	bus.OnMessage(func(data []byte) {
		sessMu.Lock()
		receivedCount++
		count := receivedCount
		sessMu.Unlock()

		// Update client session message count
		clientsMu.RLock()
		sess, ok := (*clients)[clientID]
		clientsMu.RUnlock()
		if ok {
			sess.Messages++
		}

		fmt.Printf("\n📨 [%s MSG #%d] Received: %s (%d bytes)\n", clientID, count, string(data), len(data))
		fmt.Print("> ")
	})

	bus.OnError(func(err error) {
		fmt.Printf("\n❌ [%s] Error: %v\n", clientID, err)
		fmt.Print("> ")

		// Remove client on error
		clientsMu.Lock()
		delete(*clients, clientID)
		clientsMu.Unlock()

		bus.Stop()
		clientCh.Close()
		fmt.Printf("🔌 [%s] Disconnected\n", clientID)
	})

	if err := bus.StartReceive(); err != nil {
		log.Printf("[%s] Failed to start receive: %v", clientID, err)
		bus.Stop()
		clientCh.Close()
		return
	}

	// === Step 4: Register client ===
	sess := &ClientSession{
		ID:      clientID,
		Bus:     bus,
		Channel: clientCh,
	}

	clientsMu.Lock()
	(*clients)[clientID] = sess
	clientsMu.Unlock()

	fmt.Printf("[%s] ✓ Client session ready\n", clientID)
	fmt.Println()

	// Keep client alive
	for {
		if !bus.IsRunning() {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Cleanup
	clientsMu.Lock()
	delete(*clients, clientID)
	clientsMu.Unlock()

	fmt.Printf("🔌 [%s] Disconnected\n", clientID)
}

// broadcastMessage sends a message to all connected clients.
func broadcastMessage(clients *map[string]*ClientSession, clientsMu *sync.RWMutex, message string) {
	clientsMu.RLock()
	count := len(*clients)
	clientsMu.RUnlock()

	if count == 0 {
		fmt.Println("❌ No clients connected")
		return
	}

	fmt.Printf("📤 Broadcasting to %d clients: %s\n", count, message)

	clientsMu.RLock()
	for _, sess := range *clients {
		sendToClient(sess, message)
	}
	clientsMu.RUnlock()
}

// sendToClient sends a message to a specific client.
func sendToClient(sess *ClientSession, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sess.Bus.SendWithContext(ctx, []byte(message)); err != nil {
		fmt.Printf("   ❌ [%s] Send failed: %v\n", sess.ID, err)
	} else {
		fmt.Printf("   ✓ [%s] Sent: %s (%d bytes)\n", sess.ID, message, len(message))
	}
}
