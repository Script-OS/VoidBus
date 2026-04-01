// Package main provides an interactive VoidBus server example using new Listen API.
//
// This example demonstrates:
// - Creating a VoidBus server
// - Using Listen to accept connections (with auto-negotiation)
// - Standard net.Listener Accept interface
// - Standard net.Conn Read/Write interface
// - Multi-client management
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
	serverAddr = ":8080"
)

// ClientSession represents a connected client session.
type ClientSession struct {
	ID       string
	Conn     net.Conn
	Messages int
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║         VoidBus Interactive Server (net.Listener API)     ║")
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Println("║ Commands:                                                  ║")
	fmt.Println("║   <message>           - Broadcast to all clients           ║")
	fmt.Println("║   <client-id> <msg>   - Send to specific client            ║")
	fmt.Println("║   list                - List connected clients             ║")
	fmt.Println("║   quit                - Exit server                        ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// === Step 1: Create Bus ===
	fmt.Println("[1/3] Creating VoidBus...")
	bus, err := voidbus.New()
	if err != nil {
		log.Fatalf("Failed to create bus: %v", err)
	}
	fmt.Println("      ✓ Bus created successfully")

	// === Step 2: Register Codec ===
	fmt.Println("[2/3] Registering codecs...")
	codec := base64.New()
	if err := bus.RegisterCodec(codec); err != nil {
		log.Fatalf("Failed to register codec: %v", err)
	}
	fmt.Printf("      ✓ Registered codec: %s (SecurityLevel: %d)\n",
		codec.Code(), codec.SecurityLevel())

	// === Step 3: Create and Register Server Channel ===
	fmt.Println("[3/3] Creating TCP server...")
	serverConfig := channel.ChannelConfig{
		Address: serverAddr,
	}
	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		log.Fatalf("Failed to create server channel: %v", err)
	}
	fmt.Printf("      ✓ TCP server channel created\n")

	// Add channel to bus
	if err := bus.AddChannel(serverCh); err != nil {
		log.Fatalf("Failed to add channel: %v", err)
	}

	// === Listen (Auto-accept with auto-negotiation) ===
	fmt.Printf("Listening on %s (auto-negotiation enabled)...\n", serverAddr)
	fmt.Println()

	// Listen returns net.Listener (standard Go interface)
	listener, err := bus.Listen(serverCh)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	fmt.Printf("      ✓ Server listening on %s\n", listener.Addr().String())
	fmt.Println()

	// === Client Management ===
	clients := make(map[string]*ClientSession)
	var clientsMu sync.RWMutex
	var clientCount int

	// Accept clients in background (using standard net.Listener Accept)
	go func() {
		for {
			// Accept returns net.Conn (standard Go interface)
			conn, err := listener.Accept()
			if err != nil {
				// Listener closed
				return
			}

			// Generate client ID
			clientCount++
			clientID := fmt.Sprintf("client-%03d", clientCount)

			fmt.Printf("🔗 New client connected: %s (RemoteAddr: %s)\n", clientID, conn.RemoteAddr().String())

			// Register client
			sess := &ClientSession{
				ID:   clientID,
				Conn: conn,
			}

			clientsMu.Lock()
			clients[clientID] = sess
			clientsMu.Unlock()

			// Handle client in separate goroutine
			go handleClient(clientID, conn, &clients, &clientsMu)
		}
	}()

	// === Interactive Input ===
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Println("Ready for interactive input (net.Conn Read/Write)")
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
					fmt.Printf("   - %s (messages: %d, addr: %s)\n", id, sess.Messages, sess.Conn.RemoteAddr().String())
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

	// Close all client connections
	clientsMu.Lock()
	for _, sess := range clients {
		sess.Conn.Close()
	}
	clientsMu.Unlock()

	// Close listener
	listener.Close()

	// Stop bus
	bus.Stop()

	fmt.Println("✓ Server stopped")
}

// handleClient handles a connected client session using standard net.Conn.
func handleClient(
	clientID string,
	conn net.Conn,
	clients *map[string]*ClientSession,
	clientsMu *sync.RWMutex,
) {
	defer func() {
		conn.Close()
		clientsMu.Lock()
		delete(*clients, clientID)
		clientsMu.Unlock()
		fmt.Printf("🔌 [%s] Disconnected\n", clientID)
	}()

	buf := make([]byte, 64*1024) // 64KB buffer for complete message

	for {
		// Set read deadline for polling
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, err := conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(*net.OpError); ok && netErr.Timeout() {
				// Timeout is expected for polling
				continue
			}
			if err == io.EOF {
				fmt.Printf("\n🔴 [%s] Client closed connection\n", clientID)
				return
			}
			fmt.Printf("\n❌ [%s] Read error: %v\n", clientID, err)
			return
		}

		// Update message count
		clientsMu.RLock()
		sess, ok := (*clients)[clientID]
		clientsMu.RUnlock()
		if ok {
			sess.Messages++
		}

		data := make([]byte, n)
		copy(data, buf[:n])

		fmt.Printf("\n📨 [%s] Received: %s (%d bytes)\n", clientID, string(data), n)
		fmt.Print("> ")

		// Echo back to client
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		conn.Write([]byte("Echo: " + string(data)))
	}
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

// sendToClient sends a message to a specific client using standard net.Conn Write.
func sendToClient(sess *ClientSession, message string) {
	sess.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	n, err := sess.Conn.Write([]byte(message))
	if err != nil {
		fmt.Printf("   ❌ [%s] Send failed: %v\n", sess.ID, err)
	} else {
		fmt.Printf("   ✓ [%s] Sent: %s (%d bytes)\n", sess.ID, message, n)
	}
}
