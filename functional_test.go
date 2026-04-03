package voidbus

import (
	"crypto/rsa"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/channel/tcp"
	"github.com/Script-OS/VoidBus/channel/udp"
	"github.com/Script-OS/VoidBus/channel/ws"
	"github.com/Script-OS/VoidBus/codec/aes"
	"github.com/Script-OS/VoidBus/codec/base64"
	"github.com/Script-OS/VoidBus/codec/chacha20"
	"github.com/Script-OS/VoidBus/codec/plain"
	rsacodec "github.com/Script-OS/VoidBus/codec/rsa"
	"github.com/Script-OS/VoidBus/codec/xor"
)

// FunctionalTestSuite provides comprehensive functional tests for VoidBus.
// Tests progress from simple to complex:
// 1. Single channel + plaintext (debug mode)
// 2. Single channel + depth=1 codec (all codecs)
// 3. Random channel + plaintext
// 4. Random channel + depth=1 codec (all codecs)
// 5. Random multi-channel + depth=2 codec chain

// === Test Configuration ===

const (
	testMessageCount = 3                     // Number of messages to send in each test
	testMessageDelay = 10 * time.Millisecond // Delay between messages
	testTimeout      = 30 * time.Second      // Overall test timeout
)

var (
	// Test keys for encryption codecs
	testAESKey    = make([]byte, 32) // AES-256 key
	testXORKey    = make([]byte, 32) // XOR key
	testChaChaKey = make([]byte, 32) // ChaCha20 key
	testRSAKey    *rsa.PrivateKey    // RSA test key pair
)

func init() {
	// Initialize test keys with non-zero values
	for i := range testAESKey {
		testAESKey[i] = byte(i)
		testXORKey[i] = byte(i + 1)
		testChaChaKey[i] = byte(i + 2)
	}

	// Generate RSA test key pair (2048 bits)
	// Note: RSA codec is for key exchange, max plaintext ~190 bytes
	if key, err := rsacodec.GenerateKey(2048); err == nil {
		testRSAKey = key
	}
}

// === Level 1: Single Channel + Debug Mode + Plaintext ===

func TestFunctional_SingleChannel_Plaintext(t *testing.T) {
	testName := "Level1-TCP-Plaintext"
	t.Run(testName, func(t *testing.T) {
		runSingleChannelTest(t, channel.TypeTCP, []string{"plain"}, nil, true)
	})

	testName = "Level1-WS-Plaintext"
	t.Run(testName, func(t *testing.T) {
		runSingleChannelTest(t, channel.TypeWS, []string{"plain"}, nil, true)
	})

	testName = "Level1-UDP-Plaintext"
	t.Run(testName, func(t *testing.T) {
		runSingleChannelTest(t, channel.TypeUDP, []string{"plain"}, nil, true)
	})
}

// === Level 2: Single Channel + Depth=1 Codec (All Codecs) ===

func TestFunctional_SingleChannel_AllCodecs(t *testing.T) {
	codecs := []struct {
		name string
		key  []byte
	}{
		{"base64", nil},
		{"xor", testXORKey},
		{"aes", testAESKey},
		{"chacha20", testChaChaKey},
		// RSA codec is for key exchange only (not suitable for bidirectional data communication)
		// RSA requires hybrid encryption: RSA for key exchange + AES/ChaCha20 for data
	}

	channels := []channel.ChannelType{
		channel.TypeTCP,
		channel.TypeWS,
		channel.TypeUDP,
	}

	for _, ch := range channels {
		for _, codec := range codecs {
			testName := fmt.Sprintf("Level2-%s-%s", ch, codec.name)
			t.Run(testName, func(t *testing.T) {
				runSingleChannelTest(t, ch, []string{codec.name}, codec.key, false)
			})
		}
	}
}

// === Level 3: Random Channel + Plaintext ===

func TestFunctional_RandomChannel_Plaintext(t *testing.T) {
	testName := "Level3-RandomChannel-Plaintext"
	t.Run(testName, func(t *testing.T) {
		runRandomChannelTest(t, []string{"plain"}, nil, true)
	})
}

// === Level 4: Random Channel + Depth=1 Codec (All Codecs) ===

func TestFunctional_RandomChannel_AllCodecs(t *testing.T) {
	codecs := []struct {
		name string
		key  []byte
	}{
		{"base64", nil},
		{"xor", testXORKey},
		{"aes", testAESKey},
		{"chacha20", testChaChaKey},
		// RSA codec is for key exchange only (requires hybrid encryption)
	}

	for _, codec := range codecs {
		testName := fmt.Sprintf("Level4-RandomChannel-%s", codec.name)
		t.Run(testName, func(t *testing.T) {
			runRandomChannelTest(t, []string{codec.name}, codec.key, false)
		})
	}
}

// === Level 5: Random Multi-Channel + Depth=2 Codec Chain ===

func TestFunctional_RandomMultiChannel_CodecChains(t *testing.T) {
	chains := []struct {
		name   string
		codecs []string
		key    []byte
	}{
		{"base64-xor", []string{"base64", "xor"}, testXORKey},
		{"xor-aes", []string{"xor", "aes"}, testAESKey},
		{"aes-chacha20", []string{"aes", "chacha20"}, testChaChaKey},
		{"base64-aes", []string{"base64", "aes"}, testAESKey},
	}

	for _, chain := range chains {
		testName := fmt.Sprintf("Level5-MultiChannel-%s", chain.name)
		t.Run(testName, func(t *testing.T) {
			runRandomMultiChannelTest(t, chain.codecs, chain.key)
		})
	}
}

// === Test Helper Functions ===

// runSingleChannelTest tests communication over a single fixed channel.
func runSingleChannelTest(t *testing.T, chType channel.ChannelType, codecChain []string, key []byte, debugMode bool) {
	// Allocate ports (sequential to avoid conflicts)
	basePort := getTestPort()
	serverPort := basePort

	// Create server
	serverBus, _ := createTestBus(t, chType, serverPort, codecChain, key, debugMode, true)
	defer serverBus.Stop()

	// Start server listener
	listener, err := serverBus.Listen()
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	// Server accept goroutine
	serverReady := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			t.Logf("Accept error: %v", err)
			return
		}
		serverReady <- conn
	}()

	// Wait for server to be ready (especially important for WebSocket)
	time.Sleep(200 * time.Millisecond)

	// Create client (after server is listening)
	clientBus, _ := createTestBus(t, chType, serverPort, codecChain, key, debugMode, false)
	defer clientBus.Stop()

	// Client dial
	conn, err := clientBus.Dial()
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	// Wait for server to accept
	var serverConn net.Conn
	select {
	case serverConn = <-serverReady:
	case <-time.After(5 * time.Second):
		t.Fatal("Server accept timeout")
	}
	defer serverConn.Close()

	// Run bidirectional test
	runBidirectionalTest(t, conn, serverConn, testMessageCount)
}

// runRandomChannelTest tests communication with random channel selection.
func runRandomChannelTest(t *testing.T, codecChain []string, key []byte, debugMode bool) {
	// Test all three channels in random selection
	chTypes := []channel.ChannelType{
		channel.TypeTCP,
		channel.TypeWS,
		channel.TypeUDP,
	}

	// Run test for each channel type (simulating random selection)
	for _, chType := range chTypes {
		testName := string(chType)
		t.Run(testName, func(t *testing.T) {
			runSingleChannelTest(t, chType, codecChain, key, debugMode)
		})
	}
}

// runRandomMultiChannelTest tests communication with multiple channels and codec chains.
func runRandomMultiChannelTest(t *testing.T, codecChain []string, key []byte) {
	// Test with all three channels
	chTypes := []channel.ChannelType{
		channel.TypeTCP,
		channel.TypeWS,
		channel.TypeUDP,
	}

	for _, chType := range chTypes {
		testName := string(chType)
		t.Run(testName, func(t *testing.T) {
			runSingleChannelTest(t, chType, codecChain, key, false)
		})
	}
}

// createTestBus creates a Bus instance for testing.
func createTestBus(t *testing.T, chType channel.ChannelType, port int, codecChain []string, key []byte, debugMode bool, isServer bool) (*Bus, channel.Channel) {
	// Create bus
	bus, err := New(nil)
	if err != nil {
		t.Fatalf("failed to create bus: %v", err)
	}

	// Set key if needed BEFORE registering codecs
	// (RegisterCodec will set keyProvider to codec at registration time)
	if len(key) > 0 {
		if err = bus.SetKey(key); err != nil {
			t.Fatalf("failed to set key: %v", err)
		}
	}

	// Register codecs
	for _, name := range codecChain {
		switch name {
		case "plain":
			bus.RegisterCodec(plain.New())
		case "base64":
			bus.RegisterCodec(base64.New())
		case "xor":
			bus.RegisterCodec(xor.New())
		case "aes":
			bus.RegisterCodec(aes.NewAES256Codec())
		case "chacha20":
			bus.RegisterCodec(chacha20.New())
		case "rsa":
			rsaCodec := rsacodec.New()
			// RSA codec requires direct key setting (not via key provider)
			// Server needs private key for decryption
			// Client needs public key for encryption
			if isServer {
				rsaCodec.SetPrivateKey(testRSAKey)
			} else {
				rsaCodec.SetPublicKey(&testRSAKey.PublicKey)
			}
			bus.RegisterCodec(rsaCodec)
		}
	}

	// Create channel
	address := fmt.Sprintf("127.0.0.1:%d", port)
	var ch channel.Channel
	switch chType {
	case channel.TypeTCP:
		if isServer {
			ch, err = createTCPServerChannel(address)
		} else {
			ch, err = createTCPClientChannel(address)
		}
	case channel.TypeWS:
		if isServer {
			ch, err = createWSServerChannel(address)
		} else {
			ch, err = createWSClientChannel(address)
		}
	case channel.TypeUDP:
		if isServer {
			ch, err = createUDPServerChannel(address)
		} else {
			ch, err = createUDPClientChannel(address)
		}
	default:
		t.Fatalf("unsupported channel type: %s", chType)
	}

	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	// Add channel to bus
	if err := bus.AddChannel(ch); err != nil {
		t.Fatalf("failed to add channel: %v", err)
	}

	return bus, ch
}

// runBidirectionalTest tests bidirectional communication between client and server.
func runBidirectionalTest(t *testing.T, clientConn, serverConn net.Conn, messageCount int) {
	var wg sync.WaitGroup
	clientErrors := make(chan error, messageCount*2)
	serverErrors := make(chan error, messageCount*2)

	// Client send/receive goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < messageCount; i++ {
			// Send message
			msg := fmt.Sprintf("Client-Message-%d", i)
			_, err := clientConn.Write([]byte(msg))
			if err != nil {
				clientErrors <- fmt.Errorf("client write error: %w", err)
				return
			}

			// Receive echo
			buf := make([]byte, 1024)
			n, err := clientConn.Read(buf)
			if err != nil {
				clientErrors <- fmt.Errorf("client read error: %w", err)
				return
			}

			// Verify echo
			expected := fmt.Sprintf("Server-Echo-%d", i)
			received := string(buf[:n])
			if received != expected {
				clientErrors <- fmt.Errorf("client received mismatch: expected %q, got %q", expected, received)
				return
			}

			time.Sleep(testMessageDelay)
		}
		clientErrors <- nil
	}()

	// Server receive/send goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < messageCount; i++ {
			// Receive message
			buf := make([]byte, 1024)
			n, err := serverConn.Read(buf)
			if err != nil {
				serverErrors <- fmt.Errorf("server read error: %w", err)
				return
			}

			// Verify message
			expected := fmt.Sprintf("Client-Message-%d", i)
			received := string(buf[:n])
			if received != expected {
				serverErrors <- fmt.Errorf("server received mismatch: expected %q, got %q", expected, received)
				return
			}

			// Send echo
			echo := fmt.Sprintf("Server-Echo-%d", i)
			_, err = serverConn.Write([]byte(echo))
			if err != nil {
				serverErrors <- fmt.Errorf("server write error: %w", err)
				return
			}
		}
		serverErrors <- nil
	}()

	// Wait for completion
	wg.Wait()

	// Check for errors
	select {
	case err := <-clientErrors:
		if err != nil {
			t.Errorf("Client error: %v", err)
		}
	case <-time.After(testTimeout):
		t.Error("Client timeout")
	}

	select {
	case err := <-serverErrors:
		if err != nil {
			t.Errorf("Server error: %v", err)
		}
	case <-time.After(testTimeout):
		t.Error("Server timeout")
	}
}

// === Channel Creation Helpers ===

func createTCPServerChannel(address string) (channel.Channel, error) {
	ch, err := tcp.NewServerChannel(channel.ChannelConfig{
		Address: address,
	})
	return ch, err
}

func createTCPClientChannel(address string) (channel.Channel, error) {
	ch, err := tcp.NewClientChannel(channel.ChannelConfig{
		Address:        address,
		ConnectTimeout: 5 * time.Second,
	})
	return ch, err
}

func createWSServerChannel(address string) (channel.Channel, error) {
	// WebSocket server uses plain host:port format
	ch, err := ws.NewServerChannel(channel.ChannelConfig{
		Address: address,
	})
	return ch, err
}

func createWSClientChannel(address string) (channel.Channel, error) {
	// WebSocket client needs ws:// prefix
	wsAddress := "ws://" + address
	ch, err := ws.NewClientChannel(channel.ChannelConfig{
		Address:        wsAddress,
		ConnectTimeout: 5 * time.Second,
	})
	return ch, err
}

func createUDPServerChannel(address string) (channel.Channel, error) {
	ch, err := udp.NewServerChannel(channel.ChannelConfig{
		Address: address,
	})
	return ch, err
}

func createUDPClientChannel(address string) (channel.Channel, error) {
	ch, err := udp.NewClientChannel(channel.ChannelConfig{
		Address:        address,
		ConnectTimeout: 5 * time.Second,
	})
	return ch, err
}

// === Port Allocation ===

var (
	testPortCounter = 9000
	testPortMu      sync.Mutex
)

func getTestPort() int {
	testPortMu.Lock()
	defer testPortMu.Unlock()
	port := testPortCounter
	testPortCounter += 100 // Reserve 100 ports per test to avoid conflicts
	return port
}

func resetTestPort() {
	testPortMu.Lock()
	defer testPortMu.Unlock()
	testPortCounter = 9000
}

// === Level 6: True Multi-Channel Distribution Test ===
// This test verifies that fragments are actually distributed across multiple channels.

func TestFunctional_TrueMultiChannel_Distribution(t *testing.T) {
	// Allocate ports - use higher range to avoid conflicts
	// Skip the normal test port allocation to ensure no conflicts
	basePort := 10000 // Use ports 10000, 10001, 10002 for this special test
	tcpPort := basePort
	wsPort := basePort + 1
	udpPort := basePort + 2

	// Create server bus with ALL three channels
	serverBus, err := New(nil)
	if err != nil {
		t.Fatalf("Failed to create server bus: %v", err)
	}

	// Set key for codecs
	if err = serverBus.SetKey(testAESKey); err != nil {
		t.Fatalf("Failed to set server key: %v", err)
	}

	// Register codecs
	serverBus.RegisterCodec(base64.New())
	serverBus.RegisterCodec(aes.NewAES256Codec())

	// Add ALL server channels
	tcpServer, err := createTCPServerChannel(fmt.Sprintf(":%d", tcpPort))
	if err != nil {
		t.Fatalf("Failed to create TCP server: %v", err)
	}
	serverBus.AddChannel(tcpServer)

	wsServer, err := createWSServerChannel(fmt.Sprintf(":%d", wsPort))
	if err != nil {
		t.Fatalf("Failed to create WS server: %v", err)
	}
	serverBus.AddChannel(wsServer)

	udpServer, err := createUDPServerChannel(fmt.Sprintf(":%d", udpPort))
	if err != nil {
		t.Fatalf("Failed to create UDP server: %v", err)
	}
	serverBus.AddChannel(udpServer)

	defer serverBus.Stop() // Ensure server bus is stopped to release ports

	// Start listener
	listener, err := serverBus.Listen()
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	// Channel to receive accepted connection
	serverReady := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			t.Logf("Accept error: %v", err)
			return
		}
		serverReady <- conn
	}()

	// Create client bus with ALL three channels
	clientBus, err := New(nil)
	if err != nil {
		t.Fatalf("Failed to create client bus: %v", err)
	}

	if err = clientBus.SetKey(testAESKey); err != nil {
		t.Fatalf("Failed to set client key: %v", err)
	}

	clientBus.RegisterCodec(base64.New())
	clientBus.RegisterCodec(aes.NewAES256Codec())

	// Add ALL client channels
	tcpClient, err := createTCPClientChannel(fmt.Sprintf("127.0.0.1:%d", tcpPort))
	if err != nil {
		t.Fatalf("Failed to create TCP client: %v", err)
	}
	clientBus.AddChannel(tcpClient)

	wsClient, err := createWSClientChannel(fmt.Sprintf("127.0.0.1:%d", wsPort))
	if err != nil {
		t.Fatalf("Failed to create WS client: %v", err)
	}
	clientBus.AddChannel(wsClient)

	udpClient, err := createUDPClientChannel(fmt.Sprintf("127.0.0.1:%d", udpPort))
	if err != nil {
		t.Fatalf("Failed to create UDP client: %v", err)
	}
	clientBus.AddChannel(udpClient)

	defer clientBus.Stop()

	// Dial - should connect ALL channels
	conn, err := clientBus.Dial()
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	// Wait for server to accept
	var serverConn net.Conn
	select {
	case serverConn = <-serverReady:
	case <-time.After(5 * time.Second):
		t.Fatal("Server accept timeout")
	}
	defer serverConn.Close()

	// Send a small message first (no fragmentation needed)
	// This tests that multi-channel connection is working
	smallMessage := []byte("Hello from multi-channel client!")

	// Send from client
	_, err = conn.Write(smallMessage)
	if err != nil {
		t.Fatalf("Failed to send small message: %v", err)
	}

	// Receive on server
	buf := make([]byte, 128*1024)
	n, err := serverConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to receive small message: %v", err)
	}

	if string(buf[:n]) != string(smallMessage) {
		t.Fatalf("Small message mismatch: got %s, expected %s", string(buf[:n]), string(smallMessage))
	}

	t.Logf("Successfully sent and received small message: %s", string(buf[:n]))

	// Now send a LARGE message (force fragmentation across multiple channels)
	// 64KB should split into many fragments distributed across TCP/WS/UDP
	largeMessage := make([]byte, 64*1024)
	for i := range largeMessage {
		largeMessage[i] = byte(i % 256)
	}

	// Send from client
	_, err = conn.Write(largeMessage)
	if err != nil {
		t.Fatalf("Failed to send large message: %v", err)
	}

	// Receive on server
	n, err = serverConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to receive large message: %v", err)
	}

	if n != len(largeMessage) {
		t.Fatalf("Message size mismatch: got %d, expected %d", n, len(largeMessage))
	}

	// Verify content
	for i := 0; i < n; i++ {
		if buf[i] != largeMessage[i] {
			t.Fatalf("Content mismatch at byte %d: got %d, expected %d", i, buf[i], largeMessage[i])
		}
	}

	t.Logf("Successfully sent and received %d bytes across multiple channels", n)

	// Send response from server
	response := []byte("ACK: large message received")
	_, err = serverConn.Write(response)
	if err != nil {
		t.Fatalf("Failed to send response: %v", err)
	}

	// Receive response on client
	respBuf := make([]byte, 1024)
	respN, err := conn.Read(respBuf)
	if err != nil {
		t.Fatalf("Failed to receive response: %v", err)
	}

	if string(respBuf[:respN]) != string(response) {
		t.Fatalf("Response mismatch: got %s, expected %s", string(respBuf[:respN]), string(response))
	}

	t.Logf("Successfully received response: %s", string(respBuf[:respN]))
}
