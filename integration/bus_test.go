// Package integration provides end-to-end integration tests for VoidBus.
//
// These tests verify:
// - Bus communication through TCP channel
// - ServerBus client management
// - Codec chain encryption
// - Full data flow (serialize -> encode -> send -> receive -> decode -> deserialize)
package integration_test

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"VoidBus/channel"
	"VoidBus/channel/tcp"
	"VoidBus/codec"
	"VoidBus/codec/aes"
	"VoidBus/codec/base64"
	"VoidBus/codec/plain"
	"VoidBus/core"
	"VoidBus/keyprovider/embedded"
	plainserializer "VoidBus/serializer/plain"
)

// getFreePort returns a free TCP port
func getFreePort() int {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 20000
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 20000
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// syncConfig returns a BusConfig with async receive disabled
func syncConfig() core.BusConfig {
	config := core.DefaultBusConfig()
	config.AsyncReceive = false
	return config
}

// TestTCPChannelDirect tests raw TCP channel communication first
func TestTCPChannelDirect(t *testing.T) {
	port := getFreePort()
	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)

	// Create server channel
	serverConfig := channel.ChannelConfig{Address: serverAddr}
	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server channel: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer serverCh.Close()

		clientCh, err := serverCh.Accept()
		if err != nil {
			t.Errorf("Accept error: %v", err)
			return
		}
		defer clientCh.Close()

		// Receive message
		data, err := clientCh.Receive()
		if err != nil {
			t.Errorf("Receive error: %v", err)
			return
		}

		// Send response
		response := []byte("echo_" + string(data))
		if err := clientCh.Send(response); err != nil {
			t.Errorf("Send error: %v", err)
			return
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// Create client channel
	clientConfig := channel.ChannelConfig{Address: serverAddr, Timeout: 5}
	clientCh, err := tcp.NewClientChannel(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create client channel: %v", err)
	}
	defer clientCh.Close()

	// Send message
	message := []byte("hello_tcp")
	if err := clientCh.Send(message); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Receive response
	response, err := clientCh.Receive()
	if err != nil {
		t.Fatalf("Failed to receive: %v", err)
	}

	wg.Wait()

	if string(response) != "echo_hello_tcp" {
		t.Errorf("Response mismatch: got %s, want %s", response, "echo_hello_tcp")
	}
}

// TestBusWithPlainCodec tests Bus with Plain serializer and Plain codec
func TestBusWithPlainCodec(t *testing.T) {
	port := getFreePort() + 1
	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)

	// Create server channel
	serverConfig := channel.ChannelConfig{Address: serverAddr}
	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server channel: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer serverCh.Close()

		clientCh, err := serverCh.Accept()
		if err != nil {
			t.Errorf("Accept error: %v", err)
			return
		}

		// Create server Bus (with sync receive mode)
		serverBus := core.NewBuilder().
			UseSerializerInstance(plainserializer.New()).
			UseCodecChain(codec.NewChain().AddCodec(plain.New())).
			UseChannel(clientCh).
			WithConfig(syncConfig()).
			Build()

		if err := serverBus.Start(); err != nil {
			t.Errorf("Start error: %v", err)
			clientCh.Close()
			return
		}

		// Receive message
		data, err := serverBus.Receive()
		if err != nil {
			t.Errorf("Receive error: %v", err)
			serverBus.Stop()
			return
		}

		// Send response
		response := []byte("echo_" + string(data))
		serverBus.Send(response)

		// Wait a bit for send to complete
		time.Sleep(100 * time.Millisecond)
		serverBus.Stop()
	}()

	time.Sleep(100 * time.Millisecond)

	// Create client channel
	clientConfig := channel.ChannelConfig{Address: serverAddr, Timeout: 5}
	clientCh, err := tcp.NewClientChannel(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create client channel: %v", err)
	}

	// Create client Bus (with sync receive mode)
	clientBus := core.NewBuilder().
		UseSerializerInstance(plainserializer.New()).
		UseCodecChain(codec.NewChain().AddCodec(plain.New())).
		UseChannel(clientCh).
		WithConfig(syncConfig()).
		Build()

	if err := clientBus.Start(); err != nil {
		t.Fatalf("Failed to start client bus: %v", err)
	}

	// Send message
	message := []byte("hello_bus")
	if err := clientBus.Send(message); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Receive response
	response, err := clientBus.Receive()
	if err != nil {
		t.Fatalf("Failed to receive: %v", err)
	}

	wg.Wait()
	clientBus.Stop()
	clientCh.Close()

	if string(response) != "echo_hello_bus" {
		t.Errorf("Response mismatch: got %s, want %s", response, "echo_hello_bus")
	}
}

// TestBusWithBase64Codec tests Bus with Base64 encoding
func TestBusWithBase64Codec(t *testing.T) {
	port := getFreePort() + 2
	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)

	// Create server channel
	serverConfig := channel.ChannelConfig{Address: serverAddr}
	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server channel: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer serverCh.Close()

		clientCh, err := serverCh.Accept()
		if err != nil {
			t.Errorf("Accept error: %v", err)
			return
		}

		serverBus := core.NewBuilder().
			UseSerializerInstance(plainserializer.New()).
			UseCodecChain(codec.NewChain().AddCodec(base64.New())).
			UseChannel(clientCh).
			WithConfig(syncConfig()).
			Build()

		if err := serverBus.Start(); err != nil {
			t.Errorf("Start error: %v", err)
			clientCh.Close()
			return
		}

		data, err := serverBus.Receive()
		if err != nil {
			t.Errorf("Receive error: %v", err)
			serverBus.Stop()
			return
		}

		// Verify security level
		if serverBus.SecurityLevel() != codec.SecurityLevelLow {
			t.Errorf("Wrong security level: got %d, want %d", serverBus.SecurityLevel(), codec.SecurityLevelLow)
		}

		// Echo back
		serverBus.Send(data)
		time.Sleep(100 * time.Millisecond)
		serverBus.Stop()
	}()

	time.Sleep(100 * time.Millisecond)

	clientConfig := channel.ChannelConfig{Address: serverAddr, Timeout: 5}
	clientCh, err := tcp.NewClientChannel(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create client channel: %v", err)
	}

	clientBus := core.NewBuilder().
		UseSerializerInstance(plainserializer.New()).
		UseCodecChain(codec.NewChain().AddCodec(base64.New())).
		UseChannel(clientCh).
		WithConfig(syncConfig()).
		Build()

	if err := clientBus.Start(); err != nil {
		t.Fatalf("Failed to start client bus: %v", err)
	}

	// Send message
	original := []byte("test_base64")
	if err := clientBus.Send(original); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Receive echo
	echo, err := clientBus.Receive()
	if err != nil {
		t.Fatalf("Failed to receive: %v", err)
	}

	wg.Wait()
	clientBus.Stop()
	clientCh.Close()

	if string(echo) != string(original) {
		t.Errorf("Echo mismatch: got %s, want %s", echo, original)
	}
}

// TestBusWithAESCodec tests Bus with AES-256 encryption
func TestBusWithAESCodec(t *testing.T) {
	port := getFreePort() + 3
	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)

	// Create key provider (32 bytes for AES-256)
	key := []byte("01234567890123456789012345678901") // exactly 32 bytes
	keyProvider, err := embedded.New(key, "test-key", "AES-256-GCM")
	if err != nil {
		t.Fatalf("Failed to create key provider: %v", err)
	}

	// Create server channel
	serverConfig := channel.ChannelConfig{Address: serverAddr}
	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server channel: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer serverCh.Close()

		clientCh, err := serverCh.Accept()
		if err != nil {
			t.Errorf("Accept error: %v", err)
			return
		}

		serverBus := core.NewBuilder().
			UseSerializerInstance(plainserializer.New()).
			UseCodecChain(codec.NewChain().AddCodec(aes.NewAES256Codec())).
			UseChannel(clientCh).
			UseKeyProvider(keyProvider).
			WithConfig(syncConfig()).
			Build()

		if err := serverBus.Start(); err != nil {
			t.Errorf("Start error: %v", err)
			clientCh.Close()
			return
		}

		// Verify security level
		if serverBus.SecurityLevel() != codec.SecurityLevelHigh {
			t.Errorf("Wrong security level: got %d, want %d", serverBus.SecurityLevel(), codec.SecurityLevelHigh)
		}

		data, err := serverBus.Receive()
		if err != nil {
			t.Errorf("Receive error: %v", err)
			serverBus.Stop()
			return
		}

		// Echo back
		serverBus.Send(data)
		time.Sleep(100 * time.Millisecond)
		serverBus.Stop()
	}()

	time.Sleep(100 * time.Millisecond)

	clientConfig := channel.ChannelConfig{Address: serverAddr, Timeout: 5}
	clientCh, err := tcp.NewClientChannel(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create client channel: %v", err)
	}

	clientBus := core.NewBuilder().
		UseSerializerInstance(plainserializer.New()).
		UseCodecChain(codec.NewChain().AddCodec(aes.NewAES256Codec())).
		UseChannel(clientCh).
		UseKeyProvider(keyProvider).
		WithConfig(syncConfig()).
		Build()

	if err := clientBus.Start(); err != nil {
		t.Fatalf("Failed to start client bus: %v", err)
	}

	// Send encrypted message
	original := []byte("secret_aes_message")
	if err := clientBus.Send(original); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Receive encrypted echo
	echo, err := clientBus.Receive()
	if err != nil {
		t.Fatalf("Failed to receive: %v", err)
	}

	wg.Wait()
	clientBus.Stop()
	clientCh.Close()

	if string(echo) != string(original) {
		t.Errorf("Echo mismatch: got %s, want %s", echo, original)
	}
}

// TestBusCodecChain tests CodecChain with multiple codecs (AES-128 + Base64)
func TestBusCodecChain(t *testing.T) {
	port := getFreePort() + 4
	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)

	key := []byte("0123456789012345") // exactly 16 bytes for AES-128
	keyProvider, err := embedded.New(key, "chain-key", "AES-128-GCM")
	if err != nil {
		t.Fatalf("Failed to create key provider: %v", err)
	}

	// Create server channel
	serverConfig := channel.ChannelConfig{Address: serverAddr}
	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server channel: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer serverCh.Close()

		clientCh, err := serverCh.Accept()
		if err != nil {
			t.Errorf("Accept error: %v", err)
			return
		}

		// Chain: AES-128 + Base64
		chain := codec.NewChain().AddCodec(aes.NewAES128Codec()).AddCodec(base64.New())

		serverBus := core.NewBuilder().
			UseSerializerInstance(plainserializer.New()).
			UseCodecChain(chain).
			UseChannel(clientCh).
			UseKeyProvider(keyProvider).
			WithConfig(syncConfig()).
			Build()

		if err := serverBus.Start(); err != nil {
			t.Errorf("Start error: %v", err)
			clientCh.Close()
			return
		}

		// Security level should be Low (Base64 is the weakest link)
		if serverBus.SecurityLevel() != codec.SecurityLevelLow {
			t.Errorf("Wrong security level: got %d, want %d (chain returns lowest)", serverBus.SecurityLevel(), codec.SecurityLevelLow)
		}

		data, err := serverBus.Receive()
		if err != nil {
			t.Errorf("Receive error: %v", err)
			serverBus.Stop()
			return
		}

		serverBus.Send(data)
		time.Sleep(100 * time.Millisecond)
		serverBus.Stop()
	}()

	time.Sleep(100 * time.Millisecond)

	clientConfig := channel.ChannelConfig{Address: serverAddr, Timeout: 5}
	clientCh, err := tcp.NewClientChannel(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create client channel: %v", err)
	}

	// Same chain: AES-128 + Base64
	chain := codec.NewChain().AddCodec(aes.NewAES128Codec()).AddCodec(base64.New())

	clientBus := core.NewBuilder().
		UseSerializerInstance(plainserializer.New()).
		UseCodecChain(chain).
		UseChannel(clientCh).
		UseKeyProvider(keyProvider).
		WithConfig(syncConfig()).
		Build()

	if err := clientBus.Start(); err != nil {
		t.Fatalf("Failed to start client bus: %v", err)
	}

	original := []byte("test_codec_chain")
	if err := clientBus.Send(original); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	echo, err := clientBus.Receive()
	if err != nil {
		t.Fatalf("Failed to receive: %v", err)
	}

	wg.Wait()
	clientBus.Stop()
	clientCh.Close()

	if string(echo) != string(original) {
		t.Errorf("Echo mismatch: got %s, want %s", echo, original)
	}
}

// TestBusMultipleMessages tests multiple sequential messages
func TestBusMultipleMessages(t *testing.T) {
	port := getFreePort() + 5
	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)

	// Create server channel
	serverConfig := channel.ChannelConfig{Address: serverAddr}
	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server channel: %v", err)
	}

	messageCount := 5
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer serverCh.Close()

		clientCh, err := serverCh.Accept()
		if err != nil {
			t.Errorf("Accept error: %v", err)
			return
		}

		serverBus := core.NewBuilder().
			UseSerializerInstance(plainserializer.New()).
			UseCodecChain(codec.NewChain().AddCodec(base64.New())).
			UseChannel(clientCh).
			WithConfig(syncConfig()).
			Build()

		if err := serverBus.Start(); err != nil {
			t.Errorf("Start error: %v", err)
			clientCh.Close()
			return
		}

		for i := 0; i < messageCount; i++ {
			data, err := serverBus.Receive()
			if err != nil {
				t.Errorf("Receive error at %d: %v", i, err)
				break
			}
			// Echo back
			response := []byte("echo_" + string(data))
			serverBus.Send(response)
		}

		time.Sleep(100 * time.Millisecond)
		serverBus.Stop()
	}()

	time.Sleep(100 * time.Millisecond)

	clientConfig := channel.ChannelConfig{Address: serverAddr, Timeout: 5}
	clientCh, err := tcp.NewClientChannel(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create client channel: %v", err)
	}

	clientBus := core.NewBuilder().
		UseSerializerInstance(plainserializer.New()).
		UseCodecChain(codec.NewChain().AddCodec(base64.New())).
		UseChannel(clientCh).
		WithConfig(syncConfig()).
		Build()

	if err := clientBus.Start(); err != nil {
		t.Fatalf("Failed to start client bus: %v", err)
	}

	// Send multiple messages
	for i := 0; i < messageCount; i++ {
		message := []byte(fmt.Sprintf("msg_%d", i))
		if err := clientBus.Send(message); err != nil {
			t.Fatalf("Failed to send message %d: %v", i, err)
		}

		response, err := clientBus.Receive()
		if err != nil {
			t.Fatalf("Failed to receive response %d: %v", i, err)
		}

		expected := "echo_" + string(message)
		if string(response) != expected {
			t.Errorf("Response mismatch at %d: got %s, want %s", i, response, expected)
		}
	}

	wg.Wait()
	clientBus.Stop()
	clientCh.Close()
}
