package client

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/channel"
	tcpchannel "github.com/Script-OS/VoidBus/channel/tcp"
	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/fragment"
	"github.com/Script-OS/VoidBus/protocol"
	"github.com/Script-OS/VoidBus/serializer/json"
	democodec "voidbus-demo/internal/codec"
	"voidbus-demo/internal/config"
)

// Client represents the demo client
type Client struct {
	config      *config.Config
	channels    []channel.Channel
	session     *protocol.Session
	codecChains []*codec.DefaultChain
	distributor *protocol.AllRandomDistributor
	fragmentMgr *fragment.DefaultFragmentManager
	transport   *protocol.TransportSender
	dataChan    chan []byte
	errChan     chan error

	// Lifecycle management
	mu         sync.Mutex
	wg         sync.WaitGroup
	shutdownCh chan struct{}
	shutdown   bool
}

// NewClient creates a new client instance
func NewClient(cfg *config.Config) *Client {
	return &Client{
		config:     cfg,
		channels:   make([]channel.Channel, 0, cfg.ChannelCount),
		dataChan:   make(chan []byte, 10),
		errChan:    make(chan error, 10),
		shutdownCh: make(chan struct{}),
	}
}

// Connect establishes connections and setup transport
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Printf("Client: Connecting to %s with %d channels...", c.config.ServerAddr, c.config.ChannelCount)

	// Build codec chains
	chains, err := democodec.BuildCodecChainPool()
	if err != nil {
		return fmt.Errorf("failed to build codec chains: %w", err)
	}
	c.codecChains = chains
	log.Printf("Client: Built %d codec chains", len(chains))

	// Create TCP channels
	for i := 0; i < c.config.ChannelCount; i++ {
		ch, err := tcpchannel.NewClientChannel(channel.ChannelConfig{
			Address:    c.config.ServerAddr,
			Timeout:    30,
			BufferSize: 4096,
		})
		if err != nil {
			// Clean up already created channels
			for _, createdCh := range c.channels {
				createdCh.Close()
			}
			return fmt.Errorf("failed to create channel %d: %w", i, err)
		}
		c.channels = append(c.channels, ch)
		log.Printf("Client: Channel %d connected", i)
	}

	// Create session
	c.session = protocol.NewSession("client-session-" + time.Now().Format("150405"))

	// Setup serializer
	c.session.Serializer = json.New()

	// Select best codec chain (use highest security level)
	bestChain := c.codecChains[0]
	for _, chain := range c.codecChains {
		if chain.SecurityLevel() > bestChain.SecurityLevel() {
			bestChain = chain
		}
	}
	c.session.CodecChain = bestChain
	log.Printf("Client: Selected codec chain with security level %v", bestChain.SecurityLevel())

	// Setup fragment manager
	fragConfig := fragment.DefaultFragmentConfig()
	fragConfig.MaxFragmentSize = 1024
	fragConfig.Timeout = 60
	c.fragmentMgr = fragment.NewFragmentManager(fragConfig)

	// Setup transport
	transportConfig := protocol.DefaultTransportConfig()
	transportConfig.MaxFragmentSize = 1024
	c.transport = protocol.NewTransportSender(transportConfig)

	// Setup distributor
	c.distributor = protocol.NewAllRandomDistributor()

	log.Printf("Client: Connected and ready")

	return nil
}

// SendData sends data through multiple channels
func (c *Client) SendData(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.shutdown {
		return fmt.Errorf("client is shutdown")
	}

	// Prepare data for sending (serializes, encodes, fragments, wraps)
	packets, err := c.transport.PrepareData(c.session, data)
	if err != nil {
		return fmt.Errorf("failed to prepare data: %w", err)
	}

	log.Printf("Client: Prepared %d packets for %d bytes", len(packets), len(data))

	// Get channel infos for distributor
	log.Println("Client: Getting channel infos...")
	infos := c.getChannelInfos()
	log.Printf("Client: Got %d channel infos", len(infos))

	// Distribute fragments to channels
	log.Println("Client: Distributing packets...")
	distribution := c.distributor.Distribute(len(packets), infos)
	log.Printf("Client: Distributed to %d channels", len(distribution))

	// Send packets through assigned channels
	log.Println("Client: Sending packets...")
	for chIdx, pktIndices := range distribution {
		if chIdx >= len(c.channels) {
			log.Printf("Client: Skipping channel %d (out of range)", chIdx)
			continue
		}
		ch := c.channels[chIdx]

		for _, pktIdx := range pktIndices {
			if pktIdx >= len(packets) {
				continue
			}
			packet := packets[pktIdx]

			// Encode packet to bytes
			log.Printf("Client: Encoding packet %d...", pktIdx)
			packetData, err := packet.Encode()
			if err != nil {
				return fmt.Errorf("failed to encode packet %d: %w", pktIdx, err)
			}
			log.Printf("Client: Encoded packet %d (%d bytes)", pktIdx, len(packetData))

			// Send through channel
			log.Printf("Client: Sending packet %d via channel %d...", pktIdx, chIdx)
			if err := ch.Send(packetData); err != nil {
				return fmt.Errorf("failed to send packet %d via channel %d: %w", pktIdx, chIdx, err)
			}
			log.Printf("Client: Sent packet %d via channel %d", pktIdx, chIdx)
		}
	}

	log.Printf("Client: Data sent successfully")
	return nil
}

// StartReceiving starts receiving data from all channels
// Each channel receiver runs in a separate goroutine
func (c *Client) StartReceiving() {
	for i, ch := range c.channels {
		c.wg.Add(1)
		go func(chIdx int, ch channel.Channel) {
			defer c.wg.Done()
			log.Printf("Client: Starting receiver goroutine for channel %d", chIdx)

			for {
				// Check shutdown signal FIRST (non-blocking)
				select {
				case <-c.shutdownCh:
					log.Printf("Client: Channel %d receiver shutting down", chIdx)
					return
				default:
				}

				// Receive raw data (this is blocking)
				data, err := ch.Receive()
				if err != nil {
					// Check if this is due to shutdown
					select {
					case <-c.shutdownCh:
						log.Printf("Client: Channel %d receiver shutting down (connection closed)", chIdx)
						return
					default:
					}

					// Check if this is a terminal error (should not retry)
					if err == channel.ErrChannelDisconnected || err == channel.ErrChannelClosed {
						log.Printf("Client: Channel %d disconnected, stopping receiver", chIdx)
						return
					}

					// Check if ChannelError is not retryable
					var chErr *channel.ChannelError
					if errors.As(err, &chErr) && !chErr.Retryable {
						log.Printf("Client: Channel %d fatal error, stopping receiver: %v", chIdx, err)
						return
					}

					// Real error, may retry
					log.Printf("Client: Channel %d receive error: %v", chIdx, err)
					select {
					case c.errChan <- err:
					default:
					}
					time.Sleep(100 * time.Millisecond)
					continue
				}

				// Decode packet
				packet, err := protocol.DecodePacket(data)
				if err != nil {
					log.Printf("Client: Failed to decode packet: %v", err)
					continue
				}

				// Verify packet
				if err := packet.Verify(); err != nil {
					log.Printf("Client: Packet verification failed: %v", err)
					continue
				}

				log.Printf("Client: Received packet (fragment: %v)", packet.IsFragment())

				// Handle fragment or complete data
				if packet.IsFragment() {
					c.handleFragment(packet)
				} else {
					// Complete data (no fragmentation)
					result, err := c.processData(packet.Payload)
					if err != nil {
						log.Printf("Client: Failed to process data: %v", err)
						continue
					}

					log.Printf("Client: Received complete data (%d bytes)", len(result))
					c.sendDataSafe(result)
				}
			}
		}(i, ch)
	}
}

// handleFragment handles fragmented packet
func (c *Client) handleFragment(packet *protocol.Packet) {
	fragInfo := packet.Header.FragmentInfo
	log.Printf("Client: Received fragment %d/%d (ID: %s)", fragInfo.Index, fragInfo.Total, fragInfo.ID)

	// Create state if needed
	if err := c.fragmentMgr.CreateState(fragInfo.ID, int(fragInfo.Total)); err != nil {
		// May already exist, ignore
	}

	// Add fragment
	if err := c.fragmentMgr.AddFragment(fragInfo.ID, int(fragInfo.Index), packet.Payload); err != nil {
		log.Printf("Client: Failed to add fragment: %v", err)
		return
	}

	// Check if complete
	complete, err := c.fragmentMgr.IsComplete(fragInfo.ID)
	if err != nil {
		log.Printf("Client: Failed to check fragment complete: %v", err)
		return
	}

	if !complete {
		log.Printf("Client: Waiting for more fragments...")
		return
	}

	// Reassemble
	reassembled, err := c.fragmentMgr.Reassemble(fragInfo.ID)
	if err != nil {
		log.Printf("Client: Failed to reassemble: %v", err)
		return
	}

	// Process reassembled data
	result, err := c.processData(reassembled)
	if err != nil {
		log.Printf("Client: Failed to process data: %v", err)
		return
	}

	log.Printf("Client: Received complete data (%d bytes)", len(result))
	c.sendDataSafe(result)
}

// sendDataSafe sends data to channel without blocking
// Uses recover to handle panic when channel is closed during shutdown
func (c *Client) sendDataSafe(data []byte) {
	defer func() {
		if r := recover(); r != nil {
			// Channel closed during shutdown, ignore
		}
	}()
	select {
	case c.dataChan <- data:
	default:
		log.Printf("Client: Data channel full, dropping")
	}
}

// processData decodes and deserializes data
func (c *Client) processData(data []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.session.CodecChain != nil {
		var err error
		data, err = c.session.CodecChain.Decode(data)
		if err != nil {
			return nil, fmt.Errorf("decode failed: %w", err)
		}
	}

	if c.session.Serializer != nil {
		var err error
		data, err = c.session.Serializer.Deserialize(data)
		if err != nil {
			return nil, fmt.Errorf("deserialize failed: %w", err)
		}
	}

	return data, nil
}

// ReceiveData waits for received data
func (c *Client) ReceiveData() ([]byte, error) {
	select {
	case data, ok := <-c.dataChan:
		if !ok {
			return nil, errors.New("data channel closed")
		}
		return data, nil
	case err, ok := <-c.errChan:
		if !ok {
			return nil, errors.New("error channel closed")
		}
		return nil, err
	case <-time.After(c.config.ReadTimeout):
		return nil, fmt.Errorf("receive timeout")
	case <-c.shutdownCh:
		return nil, errors.New("client is shutting down")
	}
}

// getChannelInfos returns channel infos for distributor
// Note: Caller must hold c.mu to safely read c.channels
func (c *Client) getChannelInfos() []protocol.ChannelSelectInfo {
	infos := make([]protocol.ChannelSelectInfo, len(c.channels))
	for i, ch := range c.channels {
		infos[i] = protocol.ChannelSelectInfo{
			Index:        i,
			Status:       protocol.ChannelSelectStatusActive,
			IsConnected:  ch.IsConnected(),
			LastActivity: time.Now().Unix(),
		}
	}
	return infos
}

// Shutdown gracefully shuts down the client
// Phase 1: Signal all goroutines to stop
// Phase 2: Close all channels to unblock I/O
// Phase 3: Cleanup internal channels (immediately after closing connections)
// Phase 4: Wait for goroutines to finish
func (c *Client) Shutdown(timeout time.Duration) error {
	c.mu.Lock()
	if c.shutdown {
		c.mu.Unlock()
		return nil
	}
	c.shutdown = true
	c.mu.Unlock()

	log.Println("Client: Shutting down...")

	// Phase 1: Signal shutdown
	close(c.shutdownCh)

	// Phase 2: Close all TCP channels to unblock Receive()
	c.mu.Lock()
	for i, ch := range c.channels {
		if err := ch.Close(); err != nil {
			log.Printf("Client: Failed to close channel %d: %v", i, err)
		}
	}
	c.mu.Unlock()

	// Phase 3: Close internal channels immediately to unblock ReceiveData()
	// This ensures main goroutine waiting on ReceiveData() gets unblocked
	c.mu.Lock()
	close(c.dataChan)
	close(c.errChan)
	c.mu.Unlock()

	// Phase 4: Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("Client: All goroutines stopped")
	case <-time.After(timeout):
		log.Println("Client: Shutdown timeout, some goroutines may still be running")
	}

	log.Println("Client: Shutdown complete")
	return nil
}

// IsShutdown returns true if client is shutting down
func (c *Client) IsShutdown() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.shutdown
}
