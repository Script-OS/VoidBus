package server

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

// Server represents the demo server
type Server struct {
	config      *config.Config
	serverCh    channel.ServerChannel
	clientChs   []channel.Channel
	codecChains []*codec.DefaultChain
	distributor *protocol.AllRandomDistributor
	fragmentMgr *fragment.DefaultFragmentManager
	transport   *protocol.TransportSender
	session     *protocol.Session
	dataChan    chan []byte
	errChan     chan error

	// Lifecycle management
	mu         sync.Mutex
	wg         sync.WaitGroup
	shutdownCh chan struct{}
	shutdown   bool
}

// NewServer creates a new server instance
func NewServer(cfg *config.Config) *Server {
	return &Server{
		config:     cfg,
		clientChs:  make([]channel.Channel, 0, cfg.ChannelCount),
		dataChan:   make(chan []byte, 10),
		errChan:    make(chan error, 10),
		shutdownCh: make(chan struct{}),
	}
}

// Start starts the server and listens for connections
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("Server: Starting on %s...", s.config.ServerAddr)

	// Build codec chains
	chains, err := democodec.BuildCodecChainPool()
	if err != nil {
		return fmt.Errorf("failed to build codec chains: %w", err)
	}
	s.codecChains = chains
	log.Printf("Server: Built %d codec chains", len(chains))

	// Create TCP server channel
	s.serverCh, err = tcpchannel.NewServerChannel(channel.ChannelConfig{
		Address:    s.config.ServerAddr,
		Timeout:    30,
		BufferSize: 4096,
	})
	if err != nil {
		return fmt.Errorf("failed to create server channel: %w", err)
	}
	log.Printf("Server: Listening on %s", s.config.ServerAddr)

	// Create session
	s.session = protocol.NewSession("server-session-" + time.Now().Format("150405"))

	// Setup serializer
	s.session.Serializer = json.New()

	// Select best codec chain
	bestChain := s.codecChains[0]
	for _, chain := range s.codecChains {
		if chain.SecurityLevel() > bestChain.SecurityLevel() {
			bestChain = chain
		}
	}
	s.session.CodecChain = bestChain
	log.Printf("Server: Selected codec chain with security level %v", bestChain.SecurityLevel())

	// Setup fragment manager
	fragConfig := fragment.DefaultFragmentConfig()
	fragConfig.MaxFragmentSize = 1024
	fragConfig.Timeout = 60
	s.fragmentMgr = fragment.NewFragmentManager(fragConfig)

	// Setup transport
	transportConfig := protocol.DefaultTransportConfig()
	transportConfig.MaxFragmentSize = 1024
	s.transport = protocol.NewTransportSender(transportConfig)

	// Setup distributor
	s.distributor = protocol.NewAllRandomDistributor()

	log.Printf("Server: Started and ready")

	return nil
}

// AcceptConnections accepts client connections
// This method blocks until all channels are accepted or shutdown is signaled
func (s *Server) AcceptConnections() error {
	log.Printf("Server: Waiting for client connections...")

	for i := 0; i < s.config.ChannelCount; i++ {
		// Check shutdown signal FIRST
		select {
		case <-s.shutdownCh:
			log.Printf("Server: AcceptConnections interrupted by shutdown")
			return nil
		default:
		}

		// Accept connection (blocking)
		ch, err := s.serverCh.Accept()
		if err != nil {
			// Check if this is due to shutdown
			select {
			case <-s.shutdownCh:
				log.Printf("Server: Accept interrupted by shutdown (listener closed)")
				return nil
			default:
				return fmt.Errorf("failed to accept connection %d: %w", i, err)
			}
		}

		s.mu.Lock()
		s.clientChs = append(s.clientChs, ch)
		s.mu.Unlock()
		log.Printf("Server: Client channel %d connected", i)
	}

	log.Printf("Server: All %d client channels connected", len(s.clientChs))
	return nil
}

// SendData sends data through multiple channels
func (s *Server) SendData(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.shutdown {
		return fmt.Errorf("server is shutdown")
	}

	if len(s.clientChs) == 0 {
		return fmt.Errorf("no client channels available")
	}

	// Prepare data for sending
	packets, err := s.transport.PrepareData(s.session, data)
	if err != nil {
		return fmt.Errorf("failed to prepare data: %w", err)
	}

	log.Printf("Server: Prepared %d packets for %d bytes", len(packets), len(data))

	// Get channel infos
	infos := s.getChannelInfos()

	// Distribute fragments to channels
	distribution := s.distributor.Distribute(len(packets), infos)

	// Send packets through assigned channels
	for chIdx, pktIndices := range distribution {
		if chIdx >= len(s.clientChs) {
			continue
		}
		ch := s.clientChs[chIdx]

		for _, pktIdx := range pktIndices {
			if pktIdx >= len(packets) {
				continue
			}
			packet := packets[pktIdx]

			// Encode packet
			packetData, err := packet.Encode()
			if err != nil {
				return fmt.Errorf("failed to encode packet %d: %w", pktIdx, err)
			}

			// Send through channel
			if err := ch.Send(packetData); err != nil {
				return fmt.Errorf("failed to send packet %d via channel %d: %w", pktIdx, chIdx, err)
			}
			log.Printf("Server: Sent packet %d via channel %d", pktIdx, chIdx)
		}
	}

	log.Printf("Server: Data sent successfully")
	return nil
}

// StartReceiving starts receiving data from all client channels
// Each channel receiver runs in a separate goroutine
func (s *Server) StartReceiving() {
	s.mu.Lock()
	channels := s.clientChs
	s.mu.Unlock()

	for i, ch := range channels {
		s.wg.Add(1)
		go func(chIdx int, ch channel.Channel) {
			defer s.wg.Done()
			log.Printf("Server: Starting receiver goroutine for channel %d", chIdx)

			for {
				// Check shutdown signal FIRST (non-blocking)
				select {
				case <-s.shutdownCh:
					log.Printf("Server: Channel %d receiver shutting down", chIdx)
					return
				default:
				}

				// Receive raw data (this is blocking)
				data, err := ch.Receive()
				if err != nil {
					// Check if this is due to shutdown
					select {
					case <-s.shutdownCh:
						log.Printf("Server: Channel %d receiver shutting down (connection closed)", chIdx)
						return
					default:
					}

					// Check if this is a terminal error (should not retry)
					if err == channel.ErrChannelDisconnected || err == channel.ErrChannelClosed {
						log.Printf("Server: Channel %d disconnected, stopping receiver", chIdx)
						return
					}

					// Check if ChannelError is not retryable
					var chErr *channel.ChannelError
					if errors.As(err, &chErr) && !chErr.Retryable {
						log.Printf("Server: Channel %d fatal error, stopping receiver: %v", chIdx, err)
						return
					}

					// Real error, may retry
					log.Printf("Server: Channel %d receive error: %v", chIdx, err)
					select {
					case s.errChan <- err:
					default:
					}
					time.Sleep(100 * time.Millisecond)
					continue
				}

				// Decode packet
				packet, err := protocol.DecodePacket(data)
				if err != nil {
					log.Printf("Server: Failed to decode packet: %v", err)
					continue
				}

				// Verify packet
				if err := packet.Verify(); err != nil {
					log.Printf("Server: Packet verification failed: %v", err)
					continue
				}

				log.Printf("Server: Received packet (fragment: %v)", packet.IsFragment())

				// Handle fragment or complete data
				if packet.IsFragment() {
					s.handleFragment(packet)
				} else {
					// Complete data (no fragmentation)
					result, err := s.processData(packet.Payload)
					if err != nil {
						log.Printf("Server: Failed to process data: %v", err)
						continue
					}

					log.Printf("Server: Received complete data (%d bytes)", len(result))
					s.sendDataSafe(result)
				}
			}
		}(i, ch)
	}
}

// handleFragment handles fragmented packet
func (s *Server) handleFragment(packet *protocol.Packet) {
	fragInfo := packet.Header.FragmentInfo
	log.Printf("Server: Received fragment %d/%d (ID: %s)", fragInfo.Index, fragInfo.Total, fragInfo.ID)

	// Create state if needed
	if err := s.fragmentMgr.CreateState(fragInfo.ID, int(fragInfo.Total)); err != nil {
		// May already exist, ignore
	}

	// Add fragment
	if err := s.fragmentMgr.AddFragment(fragInfo.ID, int(fragInfo.Index), packet.Payload); err != nil {
		log.Printf("Server: Failed to add fragment: %v", err)
		return
	}

	// Check if complete
	complete, err := s.fragmentMgr.IsComplete(fragInfo.ID)
	if err != nil {
		log.Printf("Server: Failed to check fragment complete: %v", err)
		return
	}

	if !complete {
		log.Printf("Server: Waiting for more fragments...")
		return
	}

	// Reassemble
	reassembled, err := s.fragmentMgr.Reassemble(fragInfo.ID)
	if err != nil {
		log.Printf("Server: Failed to reassemble: %v", err)
		return
	}

	// Process reassembled data
	result, err := s.processData(reassembled)
	if err != nil {
		log.Printf("Server: Failed to process data: %v", err)
		return
	}

	log.Printf("Server: Received complete data (%d bytes)", len(result))
	s.sendDataSafe(result)
}

// sendDataSafe sends data to channel without blocking
// Uses recover to handle panic when channel is closed during shutdown
func (s *Server) sendDataSafe(data []byte) {
	defer func() {
		if r := recover(); r != nil {
			// Channel closed during shutdown, ignore
		}
	}()
	select {
	case s.dataChan <- data:
	default:
		log.Printf("Server: Data channel full, dropping")
	}
}

// processData decodes and deserializes data
func (s *Server) processData(data []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session.CodecChain != nil {
		var err error
		data, err = s.session.CodecChain.Decode(data)
		if err != nil {
			return nil, fmt.Errorf("decode failed: %w", err)
		}
	}

	if s.session.Serializer != nil {
		var err error
		data, err = s.session.Serializer.Deserialize(data)
		if err != nil {
			return nil, fmt.Errorf("deserialize failed: %w", err)
		}
	}

	return data, nil
}

// ReceiveData waits for received data
func (s *Server) ReceiveData() ([]byte, error) {
	select {
	case data, ok := <-s.dataChan:
		if !ok {
			return nil, errors.New("data channel closed")
		}
		return data, nil
	case err, ok := <-s.errChan:
		if !ok {
			return nil, errors.New("error channel closed")
		}
		return nil, err
	case <-time.After(s.config.ReadTimeout):
		return nil, fmt.Errorf("receive timeout")
	case <-s.shutdownCh:
		return nil, errors.New("server is shutting down")
	}
}

// getChannelInfos returns channel infos for distributor
func (s *Server) getChannelInfos() []protocol.ChannelSelectInfo {
	infos := make([]protocol.ChannelSelectInfo, len(s.clientChs))
	for i, ch := range s.clientChs {
		infos[i] = protocol.ChannelSelectInfo{
			Index:        i,
			Status:       protocol.ChannelSelectStatusActive,
			IsConnected:  ch.IsConnected(),
			LastActivity: time.Now().Unix(),
		}
	}
	return infos
}

// Shutdown gracefully shuts down the server
// Phase 1: Signal all goroutines to stop
// Phase 2: Close all channels to unblock I/O
// Phase 3: Cleanup internal channels (immediately after closing connections)
// Phase 4: Wait for goroutines to finish
func (s *Server) Shutdown(timeout time.Duration) error {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return nil
	}
	s.shutdown = true
	s.mu.Unlock()

	log.Println("Server: Shutting down...")

	// Phase 1: Signal shutdown
	log.Println("Server: Phase 1 - Signaling shutdown...")
	close(s.shutdownCh)
	log.Println("Server: Phase 1 complete")

	// Phase 2: Close all TCP channels to unblock Accept() and Receive()
	log.Println("Server: Phase 2 - Closing channels...")
	s.mu.Lock()
	// Close server listener to unblock Accept()
	if s.serverCh != nil {
		log.Println("Server: Closing server listener...")
		if err := s.serverCh.Close(); err != nil {
			log.Printf("Server: Failed to close server channel: %v", err)
		}
		log.Println("Server: Server listener closed")
	}
	// Close all client channels to unblock Receive()
	for i, ch := range s.clientChs {
		log.Printf("Server: Closing client channel %d...", i)
		if err := ch.Close(); err != nil {
			log.Printf("Server: Failed to close client channel %d: %v", i, err)
		}
		log.Printf("Server: Client channel %d closed", i)
	}
	s.mu.Unlock()
	log.Println("Server: Phase 2 complete")

	// Phase 3: Close internal channels immediately to unblock ReceiveData()
	log.Println("Server: Phase 3 - Closing internal channels...")
	s.mu.Lock()
	close(s.dataChan)
	close(s.errChan)
	s.mu.Unlock()
	log.Println("Server: Phase 3 complete")

	// Phase 4: Wait for goroutines with timeout
	log.Println("Server: Phase 4 - Waiting for goroutines...")
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("Server: All goroutines stopped")
	case <-time.After(timeout):
		log.Println("Server: Shutdown timeout, some goroutines may still be running")
	}

	log.Println("Server: Shutdown complete")
	return nil
}

// IsShutdown returns true if server is shutting down
func (s *Server) IsShutdown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shutdown
}
