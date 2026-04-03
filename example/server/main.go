package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	voidbus "github.com/Script-OS/VoidBus"
	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/channel/tcp"
	"github.com/Script-OS/VoidBus/channel/udp"
	"github.com/Script-OS/VoidBus/channel/ws"
	"github.com/Script-OS/VoidBus/codec/aes"
	"github.com/Script-OS/VoidBus/codec/base64"
	"github.com/Script-OS/VoidBus/codec/chacha20"
	"github.com/Script-OS/VoidBus/codec/xor"
)

const (
	tcpPort = 19000
	wsPort  = 19001
	udpPort = 19002
)

var (
	clients   []net.Conn
	clientsMu sync.Mutex
)

func main() {
	key := []byte("voidbus-example-secret-key-32b!!")

	bus, err := voidbus.New(nil)
	if err != nil {
		panic(err)
	}
	bus.SetKey(key)

	// Register multiple codecs - Bus randomly selects codec chains per message
	bus.RegisterCodec(base64.New())
	bus.RegisterCodec(xor.New())
	bus.RegisterCodec(aes.NewAES256Codec())
	bus.RegisterCodec(chacha20.New())

	// Register all server channels - Listener aggregates them for multi-channel sessions
	serverChannels := []struct {
		ch  channel.Channel
		typ string
	}{
		{must(tcp.NewServerChannel(channel.ChannelConfig{Address: fmt.Sprintf(":%d", tcpPort)})), "TCP"},
		{must(ws.NewServerChannel(channel.ChannelConfig{Address: fmt.Sprintf(":%d", wsPort)})), "WS"},
		{must(udp.NewServerChannel(channel.ChannelConfig{Address: fmt.Sprintf(":%d", udpPort)})), "UDP"},
	}

	for _, sc := range serverChannels {
		bus.AddChannel(sc.ch)
		fmt.Printf("Registered %s channel on :%d\n", sc.typ, portFor(sc.ch.Type()))
	}

	// Listen aggregates all registered server channels
	listener, err := bus.Listen()
	if err != nil {
		panic(fmt.Sprintf("listen: %v", err))
	}

	for _, sc := range serverChannels {
		fmt.Printf("Listening on %s :%d\n", sc.typ, portFor(sc.ch.Type()))
	}

	// Graceful shutdown setup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Accept loop - each Accept returns a session with all channels connected
	connChan := make(chan net.Conn, 16)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connChan <- conn
		}
	}()

	// Handle clients
	go func() {
		for conn := range connChan {
			go handleClient(conn)
		}
	}()

	// Interactive stdin loop (optional - server runs without stdin input)
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("\nType messages to broadcast to all clients (Ctrl+C to exit):")

	done := make(chan struct{}, 1)
	go func() {
		for {
			fmt.Print("server> ")
			if !scanner.Scan() {
				// stdin closed (EOF) - but don't exit, just stop reading
				// Server should keep running for testing
				return
			}
			msg := scanner.Text()
			if msg == "" {
				continue
			}
			broadcast(msg)
		}
	}()

	// Wait for signal (stdin EOF no longer triggers shutdown)
	sig := <-sigChan
	fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
	close(done)

	listener.Close()
	bus.Stop()
	fmt.Println("Server stopped.")
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func portFor(chType channel.ChannelType) int {
	switch chType {
	case channel.TypeTCP:
		return tcpPort
	case channel.TypeWS:
		return wsPort
	case channel.TypeUDP:
		return udpPort
	default:
		return 0
	}
}

func handleClient(conn net.Conn) {
	defer func() {
		conn.Close()
		removeClient(conn)
	}()

	addClient(conn)
	fmt.Printf("Client connected from %s\n", conn.RemoteAddr())

	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Printf("Client %s disconnected: %v\n", conn.RemoteAddr(), err)
			return
		}
		msg := string(buf[:n])
		fmt.Printf("[%s] %s\n", conn.RemoteAddr(), msg)

		// Echo back
		conn.Write([]byte(fmt.Sprintf("s2c:%s", msg)))
	}
}

func addClient(conn net.Conn) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	clients = append(clients, conn)
	fmt.Printf("Connected clients: %d\n", len(clients))
}

func removeClient(conn net.Conn) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for i, c := range clients {
		if c == conn {
			clients = append(clients[:i], clients[i+1:]...)
			break
		}
	}
	fmt.Printf("Connected clients: %d\n", len(clients))
}

func broadcast(msg string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	if len(clients) == 0 {
		fmt.Println("No connected clients.")
		return
	}

	for _, conn := range clients {
		go func(c net.Conn) {
			if _, err := c.Write([]byte(msg)); err != nil {
				fmt.Printf("Failed to send to %s: %v\n", c.RemoteAddr(), err)
			}
		}(conn)
	}
	fmt.Printf("Broadcast to %d clients\n", len(clients))
}
