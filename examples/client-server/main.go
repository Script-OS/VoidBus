// Package main provides a complete client-server example with ServerBus.
//
// This example demonstrates:
// - ServerBus listening for multiple clients
// - Client connecting and performing handshake
// - Bidirectional communication
// - Client management (list, count, broadcast)
//
// Run:
//
//	Server: go run examples/client-server/server.go
//	Client: go run examples/client-server/client.go
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"VoidBus/channel"
	"VoidBus/channel/tcp"
	"VoidBus/codec"
	"VoidBus/codec/base64"
	"VoidBus/core"
	plainserializer "VoidBus/serializer/plain"
)

const serverAddr = "127.0.0.1:9093"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go [server|client]")
		return
	}

	mode := os.Args[1]
	switch mode {
	case "server":
		runServer()
	case "client":
		runClient()
	default:
		fmt.Println("Unknown mode:", mode)
	}
}

// runServer runs the ServerBus example
func runServer() {
	fmt.Println("=== ServerBus Example (Server Mode) ===")

	// Create server channel
	serverConfig := channel.ChannelConfig{
		Address: serverAddr,
	}

	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		log.Fatalf("Failed to create server channel: %v", err)
	}
	fmt.Printf("Server listening on %s\n", serverAddr)

	// Create ServerBus with Base64 codec (SecurityLevelLow)
	// First create the basic ServerBus
	serverBus := core.NewServerBusBuilder().
		UseServerChannel(serverCh).
		UseSerializer(plainserializer.New()).
		UseCodec(base64.New()).
		Build()

	// Then set handlers that reference serverBus
	serverBus.OnClientConnect(func(client *core.ClientInfo) {
		fmt.Printf("[+] Client connected: %s (Status: %s)\n", client.SessionID, client.Status)
	})
	serverBus.OnClientDisconnect(func(client *core.ClientInfo) {
		fmt.Printf("[-] Client disconnected: %s\n", client.SessionID)
	})
	serverBus.OnClientMessage(func(client *core.ClientInfo, data []byte) {
		fmt.Printf("[MSG] From %s: %s\n", client.SessionID, string(data))
		// Echo back to client
		response := []byte("Echo: " + string(data))
		serverBus.SendTo(client.SessionID, response)
	})
	serverBus.OnError(func(err error) {
		log.Printf("[ERR] %v", err)
	})

	if err := serverBus.Start(); err != nil {
		log.Fatalf("Failed to start server bus: %v", err)
	}
	fmt.Println("ServerBus started, waiting for clients...")

	// Periodically broadcast to all clients
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-ticker.C:
				count := serverBus.ClientCount()
				if count > 0 {
					broadcast := []byte(fmt.Sprintf("Server broadcast: %d clients connected", count))
					serverBus.Broadcast(broadcast)
					fmt.Printf("[BCAST] Sent to %d clients\n", count)
				}
			}
		}
	}()

	// Keep running until interrupted
	select {}
}

// runClient runs the client Bus example
func runClient() {
	fmt.Println("=== Client Bus Example (Client Mode) ===")

	// Create client channel
	clientConfig := channel.ChannelConfig{
		Address: serverAddr,
		Timeout: 5,
	}

	clientCh, err := tcp.NewClientChannel(clientConfig)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	fmt.Printf("Connected to %s\n", serverAddr)

	// Create client Bus
	clientBus := core.NewBuilder().
		UseSerializerInstance(plainserializer.New()).
		UseCodecChain(codec.NewChain().AddCodec(base64.New())).
		UseChannel(clientCh).
		OnMessage(func(data []byte) {
			fmt.Printf("[RECV] %s\n", string(data))
		}).
		OnError(func(err error) {
			log.Printf("[ERR] %v", err)
		}).
		Build()

	// Start with sync receive (AsyncReceive=false)
	config := core.DefaultBusConfig()
	config.AsyncReceive = true
	clientBus.SetConfig(config)

	if err := clientBus.Start(); err != nil {
		log.Fatalf("Failed to start client bus: %v", err)
	}
	fmt.Printf("Client Bus started (SessionID: %s, SecurityLevel: %d)\n",
		clientBus.GetSessionID(), clientBus.SecurityLevel())

	// Send messages periodically
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		counter := 1
		for {
			select {
			case <-ticker.C:
				message := []byte(fmt.Sprintf("Hello from client #%d", counter))
				if err := clientBus.Send(message); err != nil {
					log.Printf("[ERR] Send failed: %v", err)
					return
				}
				fmt.Printf("[SEND] %s\n", string(message))
				counter++
			}
		}
	}()

	// Keep running until interrupted
	select {}
}
