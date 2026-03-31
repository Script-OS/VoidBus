// Package main provides a standalone VoidBus client.
//
// Features:
// - Multiple channels with different codecs
// - Automatic data fragmentation
// - Random channel/codec selection for each fragment
// - Full-duplex communication
// - Codec negotiation
//
// Build: go build -o voidbus-client ./cmd/voidbus-client
// Run: ./voidbus-client -addr :9000 -channels 3 -size 102400
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"VoidBus/channel"
	"VoidBus/channel/tcp"
	"VoidBus/codec"
	"VoidBus/codec/aes"
	"VoidBus/codec/base64"
	"VoidBus/core"
	"VoidBus/fragment"
	"VoidBus/fragment/simple"
	"VoidBus/keyprovider/embedded"
	"VoidBus/serializer/plain"
)

// ClientConfig holds client configuration
type ClientConfig struct {
	ServerAddr   string
	ChannelCount int
	DataSize     int
	FragmentSize int
	Key          string
	SendInterval int
}

// ChannelWithCodec represents a channel with its codec
type ChannelWithCodec struct {
	Channel     channel.Channel
	CodecChain  codec.CodecChain
	Bus         *core.Bus
	SecurityLvl codec.SecurityLevel
	Index       int
	SendCount   int64
	RecvCount   int64
}

// Client manages multiple channels with different codecs
type Client struct {
	config    ClientConfig
	channels  []*ChannelWithCodec
	key       []byte
	fragment  fragment.Fragment
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
	recvData  [][]byte
	recvMutex sync.Mutex
}

// Message types
type Message struct {
	Type    string      `json:"type"`
	Seq     int         `json:"seq,omitempty"`
	Payload []byte      `json:"payload,omitempty"`
	Info    interface{} `json:"info,omitempty"`
}

type AckMessage struct {
	Type    string `json:"type"`
	Seq     int    `json:"seq"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9000", "Server address")
	channels := flag.Int("channels", 3, "Number of channels")
	size := flag.Int("size", 102400, "Data size in bytes")
	fragSize := flag.Int("frag", 4096, "Fragment size")
	key := flag.String("key", "voidbus-server-key-32bytes!!", "Encryption key")
	interval := flag.Int("interval", 2, "Send interval in seconds")
	flag.Parse()

	config := ClientConfig{
		ServerAddr:   *addr,
		ChannelCount: *channels,
		DataSize:     *size,
		FragmentSize: *fragSize,
		Key:          *key,
		SendInterval: *interval,
	}

	client := &Client{
		config:   config,
		channels: make([]*ChannelWithCodec, 0),
		key:      []byte(config.Key),
		fragment: simple.New(fragment.FragmentConfig{
			MaxFragmentSize: config.FragmentSize,
			EnableChecksum:  true,
		}),
		stopCh: make(chan struct{}),
	}

	if err := client.Start(); err != nil {
		log.Fatalf("Client start failed: %v", err)
	}

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down client...")
	client.Stop()
}

// Start starts the client
func (c *Client) Start() error {
	// Create key provider
	kp, err := embedded.New(c.key, "client-key", "AES-256-GCM")
	if err != nil {
		return fmt.Errorf("create key provider: %w", err)
	}

	// Create multiple channels with different codecs
	codecLevels := []codec.SecurityLevel{
		codec.SecurityLevelHigh,   // AES-256
		codec.SecurityLevelMedium, // AES-128
		codec.SecurityLevelLow,    // Base64
	}

	for i := 0; i < c.config.ChannelCount; i++ {
		// Create channel
		chConfig := channel.ChannelConfig{
			Address: c.config.ServerAddr,
			Timeout: 10,
		}
		ch, err := tcp.NewClientChannel(chConfig)
		if err != nil {
			log.Printf("Channel %d: %v", i, err)
			continue
		}

		// Select codec (cycle through available codecs)
		codecLevel := codecLevels[i%len(codecLevels)]
		var codecChain codec.CodecChain
		switch codecLevel {
		case codec.SecurityLevelHigh:
			codecChain = codec.NewChain().AddCodec(aes.NewAES256Codec())
		case codec.SecurityLevelMedium:
			codecChain = codec.NewChain().AddCodec(aes.NewAES128Codec())
		default:
			codecChain = codec.NewChain().AddCodec(base64.New())
		}

		// Create bus
		bus := core.NewBuilder().
			UseSerializerInstance(plain.New()).
			UseCodecChain(codecChain).
			UseChannel(ch).
			UseKeyProvider(kp).
			WithConfig(core.BusConfig{
				EnableFragment: false, // We handle fragmentation at app level
				AsyncReceive:   false,
				SendQueueSize:  100,
			}).
			Build()

		if err := bus.Start(); err != nil {
			log.Printf("Bus %d start: %v", i, err)
			ch.Close()
			continue
		}

		c.channels = append(c.channels, &ChannelWithCodec{
			Channel:     ch,
			CodecChain:  codecChain,
			Bus:         bus,
			SecurityLvl: codecLevel,
			Index:       i,
		})

		log.Printf("Channel %d: SecurityLevel=%d", i, codecLevel)
	}

	if len(c.channels) == 0 {
		return fmt.Errorf("no channels available")
	}

	c.running = true
	log.Printf("Client started with %d channels", len(c.channels))

	// Negotiate with server
	c.negotiate()

	// Start receive loops
	for _, ch := range c.channels {
		go c.receiveLoop(ch)
	}

	// Start send loop
	go c.sendLoop()

	return nil
}

// Stop stops the client
func (c *Client) Stop() {
	c.running = false
	close(c.stopCh)
	for _, ch := range c.channels {
		ch.Bus.Stop()
		ch.Channel.Close()
	}
}

// negotiate sends negotiation message
func (c *Client) negotiate() {
	// Select best codec level from available channels
	bestLevel := codec.SecurityLevelNone
	for _, ch := range c.channels {
		if ch.SecurityLvl > bestLevel {
			bestLevel = ch.SecurityLvl
		}
	}

	// Send negotiation
	msg := Message{
		Type: "negotiate",
		Info: map[string]interface{}{
			"codec_level": int(bestLevel),
			"channels":    len(c.channels),
		},
	}
	data, _ := json.Marshal(msg)

	// Send through first channel
	if len(c.channels) > 0 {
		c.channels[0].Bus.Send(data)
	}
}

// sendLoop sends data periodically
func (c *Client) sendLoop() {
	seq := 0
	ticker := time.NewTicker(time.Duration(c.config.SendInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			if !c.running {
				return
			}
			c.sendData(seq)
			seq++
		}
	}
}

// sendData sends fragmented data through random channels
func (c *Client) sendData(seq int) {
	// Generate test data
	data := make([]byte, c.config.DataSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Create message
	msg := Message{
		Type:    "data",
		Seq:     seq,
		Payload: data,
	}
	msgData, _ := json.Marshal(msg)

	// Fragment the data
	fragments, err := c.fragment.Split(msgData, c.config.FragmentSize)
	if err != nil {
		log.Printf("Fragment error: %v", err)
		return
	}

	log.Printf("[SEND] Seq=%d: %d bytes -> %d fragments", seq, len(msgData), len(fragments))

	// Send each fragment through random channel
	for i, frag := range fragments {
		ch := c.selectRandomChannel()
		if ch == nil {
			log.Printf("No available channel for fragment %d", i)
			continue
		}

		if err := ch.Bus.Send(frag); err != nil {
			log.Printf("Send error on channel %d: %v", ch.Index, err)
		} else {
			c.mu.Lock()
			ch.SendCount++
			c.mu.Unlock()
		}
	}

	c.printStats()
}

// selectRandomChannel selects a random available channel
func (c *Client) selectRandomChannel() *ChannelWithCodec {
	c.mu.RLock()
	defer c.mu.RUnlock()

	activeChannels := make([]*ChannelWithCodec, 0)
	for _, ch := range c.channels {
		if ch.Bus.IsRunning() {
			activeChannels = append(activeChannels, ch)
		}
	}

	if len(activeChannels) == 0 {
		return nil
	}

	return activeChannels[rand.Intn(len(activeChannels))]
}

// receiveLoop handles receiving from a channel
func (c *Client) receiveLoop(ch *ChannelWithCodec) {
	for {
		select {
		case <-c.stopCh:
			return
		default:
			if !c.running {
				return
			}

			data, err := ch.Bus.Receive()
			if err != nil {
				log.Printf("Channel %d receive error: %v", ch.Index, err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			c.mu.Lock()
			ch.RecvCount++
			c.mu.Unlock()

			// Try to parse as message
			var msg Message
			if err := json.Unmarshal(data, &msg); err == nil {
				switch msg.Type {
				case "ack":
					log.Printf("[RECV] Channel %d: ACK seq=%d, %s", ch.Index, msg.Seq, string(msg.Payload))
				case "negotiate_ack":
					log.Printf("[NEGOTIATE] ACK: accepted=%v", msg.Info)
				default:
					log.Printf("[RECV] Channel %d: %d bytes, type=%s", ch.Index, len(data), msg.Type)
				}
			} else {
				log.Printf("[RECV] Channel %d: %d bytes (raw)", ch.Index, len(data))
			}
		}
	}
}

// printStats prints channel statistics
func (c *Client) printStats() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalSend := int64(0)
	totalRecv := int64(0)
	for _, ch := range c.channels {
		totalSend += ch.SendCount
		totalRecv += ch.RecvCount
	}
	log.Printf("[STATS] Send: %d, Recv: %d", totalSend, totalRecv)
}
