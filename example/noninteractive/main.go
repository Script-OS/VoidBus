// Package main provides non-interactive test suite for VoidBus.
//
// This test suite covers:
// - All Channels: TCP, WebSocket, UDP (QUIC removed in v3.0)
// - All Codecs: base64, xor, aes, chacha20, rsa (plain excluded per security policy)
// - Codec chains: depth 1-3
// - Multi-channel negotiation
//
// Test Matrix:
// - Phase 1 (depth=1): 4 channels × 5 codecs = 20 tests
// - Phase 2 (depth=2): 4 channels × 4 chain combos = 16 tests
// - Phase 3 (depth=3): 4 channels × 3 chain combos = 12 tests
// - Total: 48 tests
//
// Test Flow:
// 1. Setup Server (指定channel + codecs)
// 2. Setup Client (匹配channel + codecs)
// 3. Server Listen -> Client Dial -> Auto Negotiate
// 4. Run Message Rounds (3 rounds, bidirectional)
// 5. Verify Message Integrity
// 6. Cleanup
// 7. Report Result
//
// On Bug Found: Immediately stop and provide fix proposal.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	voidbus "github.com/Script-OS/VoidBus"
	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/channel/tcp"
	"github.com/Script-OS/VoidBus/channel/udp"
	"github.com/Script-OS/VoidBus/channel/ws"
	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/codec/aes"
	"github.com/Script-OS/VoidBus/codec/base64"
	"github.com/Script-OS/VoidBus/codec/chacha20"
	rsacodec "github.com/Script-OS/VoidBus/codec/rsa"
	"github.com/Script-OS/VoidBus/codec/xor"
	"github.com/Script-OS/VoidBus/keyprovider/embedded"
)

// TestConfig holds test configuration
type TestConfig struct {
	Name        string
	Description string

	// Channel configuration
	ServerChannel channel.ChannelType
	ClientChannel channel.ChannelType

	// Codec chain configuration (depth 1-3)
	CodecChain []string

	// Message configuration
	MessageRounds int
	Timeout       time.Duration

	// Key configuration (for KeyAwareCodec)
	Key []byte
}

// TestResult holds test execution result
type TestResult struct {
	TestName        string
	Success         bool
	Error           error
	Duration        time.Duration
	RoundsCompleted int
	Details         string
}

// TestRunner manages test execution
type TestRunner struct {
	results []TestResult
	mu      sync.Mutex
	port    int // Starting port

	// RSA key pool (shared across RSA tests)
	rsaPrivateKey *rsa.PrivateKey
	rsaMu         sync.RWMutex
}

// NewTestRunner creates a new test runner
func NewTestRunner() *TestRunner {
	return &TestRunner{
		port: 9000, // Start from port 9000 to avoid conflict
	}
}

// initRSAKey initializes RSA key pair (lazy initialization)
func (tr *TestRunner) initRSAKey() error {
	tr.rsaMu.RLock()
	if tr.rsaPrivateKey != nil {
		tr.rsaMu.RUnlock()
		return nil
	}
	tr.rsaMu.RUnlock()

	tr.rsaMu.Lock()
	defer tr.rsaMu.Unlock()

	// Double check after acquiring write lock
	if tr.rsaPrivateKey != nil {
		return nil
	}

	key, err := rsa.GenerateKey(rand.Reader, rsacodec.DefaultKeySize)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %v", err)
	}
	tr.rsaPrivateKey = key
	return nil
}

// RunTest executes a single test case
func (tr *TestRunner) RunTest(tc TestConfig) TestResult {
	log.Printf("========== Starting Test: %s ========== ", tc.Name)
	log.Printf("Description: %s", tc.Description)
	log.Printf("Channel: Server=%s, Client=%s", tc.ServerChannel, tc.ClientChannel)
	log.Printf("Codec Chain: %v (depth=%d)", tc.CodecChain, len(tc.CodecChain))

	startTime := time.Now()
	result := TestResult{
		TestName: tc.Name,
	}

	// Allocate port
	tr.mu.Lock()
	port := tr.port
	tr.port++
	tr.mu.Unlock()

	// Run test
	ctx, cancel := context.WithTimeout(context.Background(), tc.Timeout)
	defer cancel()

	err := tr.executeTest(ctx, tc, port)
	if err != nil {
		result.Success = false
		result.Error = err
		result.Details = fmt.Sprintf("Test failed at error: %v", err)
		log.Printf("[FAIL] %s: %v", tc.Name, err)
	} else {
		result.Success = true
		result.RoundsCompleted = tc.MessageRounds
		result.Details = "All rounds completed successfully"
		log.Printf("[PASS] %s", tc.Name)
	}

	result.Duration = time.Since(startTime)
	tr.results = append(tr.results, result)

	return result
}

// executeTest executes the actual test logic
func (tr *TestRunner) executeTest(ctx context.Context, tc TestConfig, port int) error {
	// Phase 1: Setup Server
	serverReady := make(chan error, 1)
	serverDone := make(chan struct{})

	var serverBus *voidbus.Bus
	var serverListener net.Listener

	go func() {
		defer close(serverDone)

		// Create server bus
		bus, err := voidbus.New(nil)
		if err != nil {
			serverReady <- fmt.Errorf("Server: failed to create bus: %v", err)
			return
		}
		serverBus = bus

		// Register codecs
		for _, code := range tc.CodecChain {
			c, err := tr.createCodec(code, tc.Key)
			if err != nil {
				serverReady <- fmt.Errorf("Server: failed to create codec %s: %v", code, err)
				return
			}
			if err := bus.RegisterCodec(c); err != nil {
				serverReady <- fmt.Errorf("Server: failed to register codec %s: %v", code, err)
				return
			}
		}

		// Create server channel
		serverCh, err := tr.createServerChannel(tc.ServerChannel, port)
		if err != nil {
			serverReady <- fmt.Errorf("Server: failed to create channel: %v", err)
			return
		}

		if err := bus.AddChannel(serverCh); err != nil {
			serverReady <- fmt.Errorf("Server: failed to add channel: %v", err)
			return
		}

		// Listen
		listener, err := bus.Listen(serverCh)
		if err != nil {
			serverReady <- fmt.Errorf("Server: failed to listen: %v", err)
			return
		}
		serverListener = listener
		serverReady <- nil

		log.Printf("[Server] Listening on port %d", port)

		// Accept client
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[Server] Accept error: %v", err)
			return
		}
		log.Printf("[Server] Client connected")

		// Handle message rounds
		for round := 1; round <= tc.MessageRounds; round++ {
			select {
			case <-ctx.Done():
				conn.Close()
				return
			default:
			}

			// Receive from client
			buf := make([]byte, 1024)
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				log.Printf("[Server] Round %d: Receive error: %v", round, err)
				conn.Close()
				return
			}
			receivedMsg := string(buf[:n])
			expectedMsg := fmt.Sprintf("Client-Round%d", round)
			if receivedMsg != expectedMsg {
				log.Printf("[Server] Round %d: Message mismatch: expected '%s', got '%s'", round, expectedMsg, receivedMsg)
				conn.Close()
				return
			}
			log.Printf("[Server] Round %d: Received '%s'", round, receivedMsg)

			// Send reply to client
			replyMsg := fmt.Sprintf("Server-Reply-Round%d", round)
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := conn.Write([]byte(replyMsg)); err != nil {
				log.Printf("[Server] Round %d: Send reply error: %v", round, err)
				conn.Close()
				return
			}
			log.Printf("[Server] Round %d: Sent reply '%s'", round, replyMsg)
		}

		log.Printf("[Server] All rounds completed")
		conn.Close()
	}()

	// Wait for server ready
	select {
	case err := <-serverReady:
		if err != nil {
			return fmt.Errorf("Server setup failed: %v", err)
		}
	case <-ctx.Done():
		return fmt.Errorf("Server setup timeout")
	}

	// Phase 2: Setup Client
	clientBus, err := voidbus.New(nil)
	if err != nil {
		return fmt.Errorf("Client: failed to create bus: %v", err)
	}

	// Register codecs (same as server)
	for _, code := range tc.CodecChain {
		c, err := tr.createCodec(code, tc.Key)
		if err != nil {
			return fmt.Errorf("Client: failed to create codec %s: %v", code, err)
		}
		if err := clientBus.RegisterCodec(c); err != nil {
			return fmt.Errorf("Client: failed to register codec %s: %v", code, err)
		}
	}

	// Create client channel
	clientCh, err := tr.createClientChannel(tc.ClientChannel, port)
	if err != nil {
		return fmt.Errorf("Client: failed to create channel: %v", err)
	}

	if err := clientBus.AddChannel(clientCh); err != nil {
		return fmt.Errorf("Client: failed to add channel: %v", err)
	}

	// Dial server
	log.Printf("[Client] Dialing server on port %d", port)
	conn, err := clientBus.Dial(clientCh)
	if err != nil {
		return fmt.Errorf("Client: failed to dial: %v", err)
	}
	log.Printf("[Client] Connected to server")

	// Phase 3: Message Rounds
	for round := 1; round <= tc.MessageRounds; round++ {
		select {
		case <-ctx.Done():
			conn.Close()
			return fmt.Errorf("Test timeout during round %d", round)
		default:
		}

		// Send message to server
		msg := fmt.Sprintf("Client-Round%d", round)
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write([]byte(msg)); err != nil {
			conn.Close()
			return fmt.Errorf("Round %d: Client send failed: %v", round, err)
		}
		log.Printf("[Client] Round %d: Sent '%s'", round, msg)

		// Receive reply from server
		buf := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			conn.Close()
			return fmt.Errorf("Round %d: Client receive reply failed: %v", round, err)
		}
		receivedReply := string(buf[:n])
		expectedReply := fmt.Sprintf("Server-Reply-Round%d", round)
		if receivedReply != expectedReply {
			conn.Close()
			return fmt.Errorf("Round %d: Reply mismatch: expected '%s', got '%s'", round, expectedReply, receivedReply)
		}
		log.Printf("[Client] Round %d: Received reply '%s'", round, receivedReply)
	}

	log.Printf("[Client] All rounds completed")

	// Phase 4: Cleanup (close connections, then buses)
	log.Printf("[Cleanup] Starting cleanup...")

	// Close client connection (triggers bus.Stop() → channelPool.CloseAll() → unblocks receiveLoop)
	conn.Close()

	// Brief pause for server to detect connection close
	time.Sleep(100 * time.Millisecond)

	// Close server listener (closes serverCh → unblocks acceptLoop)
	if serverListener != nil {
		serverListener.Close()
	}

	// Stop buses — should return quickly now that channels are closed
	if serverBus != nil {
		serverBus.Stop()
	}
	if clientBus != nil {
		clientBus.Stop()
	}

	// Wait for server goroutine to exit
	select {
	case <-serverDone:
		log.Printf("[Cleanup] Server stopped cleanly")
	case <-time.After(2 * time.Second):
		log.Printf("[Cleanup] Server goroutine timeout (acceptable)")
	}

	log.Printf("[Cleanup] Test completed")

	return nil
}

// createCodec creates a codec instance by code
func (tr *TestRunner) createCodec(code string, key []byte) (codec.Codec, error) {
	switch code {
	case "base64":
		return base64.New(), nil
	case "xor":
		c := xor.New()
		if key != nil {
			provider, err := embedded.New(key, "", "XOR")
			if err != nil {
				return nil, err
			}
			if err := c.SetKeyProvider(provider); err != nil {
				return nil, err
			}
		}
		return c, nil
	case "aes":
		c := aes.NewAES256Codec()
		if key != nil {
			provider, err := embedded.New(key, "", "AES-256-GCM")
			if err != nil {
				return nil, err
			}
			if err := c.SetKeyProvider(provider); err != nil {
				return nil, err
			}
		}
		return c, nil
	case "chacha20":
		c := chacha20.New()
		if key != nil {
			provider, err := embedded.New(key, "", "ChaCha20-Poly1305")
			if err != nil {
				return nil, err
			}
			if err := c.SetKeyProvider(provider); err != nil {
				return nil, err
			}
		}
		return c, nil
	case "rsa":
		// RSA codec requires RSA key pair
		if err := tr.initRSAKey(); err != nil {
			return nil, err
		}
		tr.rsaMu.RLock()
		privKey := tr.rsaPrivateKey
		tr.rsaMu.RUnlock()

		// Create RSA codec with public key (for encryption) and private key (for decryption)
		return rsacodec.NewWithKeys(&privKey.PublicKey, privKey), nil
	default:
		return nil, fmt.Errorf("unknown codec: %s", code)
	}
}

// createServerChannel creates a server channel by type
func (tr *TestRunner) createServerChannel(chType channel.ChannelType, port int) (channel.Channel, error) {
	addr := fmt.Sprintf(":%d", port)

	switch chType {
	case channel.TypeTCP:
		config := channel.ChannelConfig{
			Address: addr,
		}
		return tcp.NewServerChannel(config)
	case channel.TypeWS:
		config := channel.ChannelConfig{
			Address: addr,
		}
		return ws.NewServerChannel(config)
	case channel.TypeUDP:
		config := channel.ChannelConfig{
			Address: addr,
		}
		return udp.NewServerChannel(config)
	default:
		return nil, fmt.Errorf("unknown channel type: %s", chType)
	}
}

// createClientChannel creates a client channel by type
func (tr *TestRunner) createClientChannel(chType channel.ChannelType, port int) (channel.Channel, error) {
	addr := fmt.Sprintf("localhost:%d", port)

	switch chType {
	case channel.TypeTCP:
		config := channel.ChannelConfig{
			Address: addr,
			Timeout: 10 * time.Second,
		}
		return tcp.NewClientChannel(config)
	case channel.TypeWS:
		config := channel.ChannelConfig{
			Address:        fmt.Sprintf("ws://%s", addr),
			ConnectTimeout: 10 * time.Second,
		}
		return ws.NewClientChannel(config)
	case channel.TypeUDP:
		config := channel.ChannelConfig{
			Address: addr,
			Timeout: udp.DefaultAckTimeout,
		}
		return udp.NewClientChannel(config)
	default:
		return nil, fmt.Errorf("unknown channel type: %s", chType)
	}
}

// PrintReport prints test execution report
func (tr *TestRunner) PrintReport() {
	fmt.Println("\n========================================")
	fmt.Println("VoidBus Non-Interactive Test Report")
	fmt.Println("========================================")

	passed := 0
	failed := 0

	for _, result := range tr.results {
		status := "PASS"
		if !result.Success {
			status = "FAIL"
			failed++
		} else {
			passed++
		}

		fmt.Printf("\n[%s] %s\n", status, result.TestName)
		fmt.Printf("  Duration: %v\n", result.Duration)
		if result.Error != nil {
			fmt.Printf("  Error: %v\n", result.Error)
		}
		if result.RoundsCompleted > 0 {
			fmt.Printf("  Rounds Completed: %d\n", result.RoundsCompleted)
		}
	}

	fmt.Println("\n----------------------------------------")
	fmt.Printf("Total Tests: %d\n", len(tr.results))
	fmt.Printf("Passed: %d\n", passed)
	fmt.Printf("Failed: %d\n", failed)
	fmt.Println("========================================")

	if failed > 0 {
		os.Exit(1)
	}
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds | log.Lshortfile)

	tr := NewTestRunner()

	// Define test keys (exactly 32 bytes each)
	xorKey := make([]byte, 32)
	copy(xorKey, "xor-test-key-32-bytes-xxxxxxxx")

	aesKey := make([]byte, 32)
	copy(aesKey, "aes256-test-key-32-bytes-xxxx")

	chachaKey := make([]byte, 32)
	copy(chachaKey, "chacha20-test-key-32-bytes-xx")

	// All channels
	channels := []channel.ChannelType{
		channel.TypeTCP,
		channel.TypeWS,
		channel.TypeUDP,
	}

	// All single codecs (no plain)
	singleCodecs := []struct {
		code string
		key  []byte
	}{
		{"base64", nil},
		{"xor", xorKey},
		{"aes", aesKey},
		{"chacha20", chachaKey},
		{"rsa", nil}, // RSA uses internal key pair
	}

	// Dual codec chains (depth=2)
	dualChains := []struct {
		codecs []string
		key    []byte
	}{
		{[]string{"base64", "xor"}, xorKey},
		{[]string{"xor", "aes"}, aesKey},
		{[]string{"aes", "chacha20"}, chachaKey},
		{[]string{"base64", "aes"}, aesKey},
	}

	// Triple codec chains (depth=3)
	tripleChains := []struct {
		codecs []string
		key    []byte
	}{
		{[]string{"base64", "xor", "aes"}, aesKey},
		{[]string{"xor", "aes", "chacha20"}, chachaKey},
		{[]string{"base64", "aes", "chacha20"}, chachaKey},
	}

	// Generate test matrix
	tests := []TestConfig{}

	// Phase 1: Single codec tests (4 channels × 5 codecs = 20 tests)
	testID := 1
	for _, ch := range channels {
		for _, codec := range singleCodecs {
			tests = append(tests, TestConfig{
				Name:          fmt.Sprintf("P1-T%02d-%s-%s", testID, ch, codec.code),
				Description:   fmt.Sprintf("%s + %s (depth=1)", ch, codec.code),
				ServerChannel: ch,
				ClientChannel: ch,
				CodecChain:    []string{codec.code},
				MessageRounds: 3,
				Timeout:       30 * time.Second,
				Key:           codec.key,
			})
			testID++
		}
	}

	// Phase 2: Dual codec chain tests (4 channels × 4 chains = 16 tests)
	testID = 1
	for _, ch := range channels {
		for _, chain := range dualChains {
			tests = append(tests, TestConfig{
				Name:          fmt.Sprintf("P2-T%02d-%s-chain2-%s", testID, ch, fmt.Sprintf("%s-%s", chain.codecs[0], chain.codecs[1])),
				Description:   fmt.Sprintf("%s + %s→%s (depth=2)", ch, chain.codecs[0], chain.codecs[1]),
				ServerChannel: ch,
				ClientChannel: ch,
				CodecChain:    chain.codecs,
				MessageRounds: 3,
				Timeout:       30 * time.Second,
				Key:           chain.key,
			})
			testID++
		}
	}

	// Phase 3: Triple codec chain tests (4 channels × 3 chains = 12 tests)
	testID = 1
	for _, ch := range channels {
		for _, chain := range tripleChains {
			tests = append(tests, TestConfig{
				Name:          fmt.Sprintf("P3-T%02d-%s-chain3-%s", testID, ch, fmt.Sprintf("%s-%s-%s", chain.codecs[0], chain.codecs[1], chain.codecs[2])),
				Description:   fmt.Sprintf("%s + %s→%s→%s (depth=3)", ch, chain.codecs[0], chain.codecs[1], chain.codecs[2]),
				ServerChannel: ch,
				ClientChannel: ch,
				CodecChain:    chain.codecs,
				MessageRounds: 3,
				Timeout:       30 * time.Second,
				Key:           chain.key,
			})
			testID++
		}
	}

	log.Printf("Generated %d tests: 20 (phase1) + 16 (phase2) + 12 (phase3)", len(tests))

	// Run all tests (continue on failure to collect all issues)
	var failedTests []string
	for _, tc := range tests {
		result := tr.RunTest(tc)
		if !result.Success {
			failedTests = append(failedTests, tc.Name)
		}
	}

	tr.PrintReport()

	// Summary of failed tests
	if len(failedTests) > 0 {
		log.Printf("\n========================================")
		log.Printf("Failed Tests Summary (%d/%d):", len(failedTests), len(tests))
		log.Printf("========================================")
		for _, name := range failedTests {
			for _, r := range tr.results {
				if r.TestName == name {
					log.Printf("\n[FAIL] %s", name)
					log.Printf("  Error: %v", r.Error)
					log.Printf("  Duration: %v", r.Duration)
					break
				}
			}
		}
		log.Printf("\n========================================")
		log.Printf("Please review the errors above for fix proposals.")
		log.Printf("========================================")
		os.Exit(1)
	} else {
		log.Printf("\n========================================")
		log.Printf("All %d tests passed!", len(tests))
		log.Printf("========================================")
	}
}
