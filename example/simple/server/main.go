// Package main provides a simple VoidBus server for testing with debug output.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/Script-OS/VoidBus"
	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/channel/tcp"
	"github.com/Script-OS/VoidBus/codec/base64"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds | log.Lshortfile)
	fmt.Println("=== Simple VoidBus Server (Debug) ===")

	// 1. Create Bus
	bus, err := voidbus.New(nil)
	if err != nil {
		log.Fatalf("Failed to create bus: %v", err)
	}
	fmt.Println("[1] Bus created")

	// 2. Register Codec
	codec := base64.New()
	if err := bus.RegisterCodec(codec); err != nil {
		log.Fatalf("Failed to register codec: %v", err)
	}
	fmt.Printf("[2] Codec registered: %s\n", codec.Code())

	// 3. Create Server Channel
	serverConfig := channel.ChannelConfig{
		Address: ":8080",
	}
	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		log.Fatalf("Failed to create server channel: %v", err)
	}
	fmt.Println("[3] Server channel created")

	if err := bus.AddChannel(serverCh); err != nil {
		log.Fatalf("Failed to add channel: %v", err)
	}

	// 4. Listen
	listener, err := bus.Listen(serverCh)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	fmt.Printf("[4] Listening on %s\n", listener.Addr().String())

	// 5. Accept client
	fmt.Println("[5] Waiting for client...")
	conn, err := listener.Accept()
	if err != nil {
		log.Fatalf("Failed to accept: %v", err)
	}
	fmt.Printf("[5] Client connected: %s\n", conn.RemoteAddr().String())

	// 6. Receive message from client (with longer timeout)
	fmt.Println("[6] Waiting for message from client...")
	buf := make([]byte, 64*1024)

	// Try multiple reads with debug output
	for i := 0; i < 10; i++ {
		fmt.Printf("[6.%d] Attempting to read...\n", i+1)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Printf("[6.%d] Read error: %v\n", i+1, err)
			if i >= 5 {
				log.Fatalf("Failed to read after 6 attempts: %v", err)
			}
			continue
		}
		fmt.Printf("[6.%d] Received from client: %s (%d bytes)\n", i+1, string(buf[:n]), n)

		// 7. Send reply to client
		reply := "Server reply: Hello Client!"
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		wn, werr := conn.Write([]byte(reply))
		if werr != nil {
			log.Fatalf("Failed to write: %v", werr)
		}
		fmt.Printf("[7] Sent reply: %s (%d bytes)\n", reply, wn)
		break
	}

	// Cleanup
	fmt.Println("[8] Closing connection...")
	conn.Close()
	listener.Close()
	bus.Stop()
	fmt.Println("[8] Server stopped")
	fmt.Println("=== Test Complete: Server OK ===")
}
