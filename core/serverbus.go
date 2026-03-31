// Package core provides ServerBus implementation for server-side Hall mode.
//
// ServerBus implements the "Hall" pattern: listens for incoming connections,
// performs handshake negotiation, and creates individual Bus instances for
// each accepted client.
//
// Design Constraints (see docs/ARCHITECTURE.md §3.2):
// - ServerBus manages multiple client Bus instances
// - Each client gets independent full-duplex Bus
// - Handshake negotiation for security alignment
// - SessionID used as client identifier (NOT exposed config)
// - Challenge mechanism prevents degradation attacks
package core

import (
	"errors"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/internal"
	"github.com/Script-OS/VoidBus/keyprovider"
	"github.com/Script-OS/VoidBus/protocol"
	"github.com/Script-OS/VoidBus/serializer"
)

// ServerBus errors
var (
	ErrServerBusNotRunning     = errors.New("serverbus: not running")
	ErrServerBusAlreadyRunning = errors.New("serverbus: already running")
	ErrServerChannelRequired   = errors.New("serverbus: server channel required")
	ErrKeyProviderRequired     = errors.New("serverbus: key provider required for encryption")
	ErrClientNotFound          = errors.New("serverbus: client not found")
	ErrClientDisconnected      = errors.New("serverbus: client disconnected")
)

// ServerBus is the server-side Hall mode implementation.
type ServerBus struct {
	mu sync.RWMutex

	// Core modules
	serverChannel channel.ServerChannel
	serializer    serializer.Serializer
	codecChain    codec.CodecChain
	keyProvider   keyprovider.KeyProvider

	// Configuration
	config ServerBusConfig

	// Protocol
	handshake *protocol.HandshakeManager
	busConfig BusConfig

	// State
	running bool
	stopCh  chan struct{}

	// Client registry
	clients map[string]*ClientInfo

	// Handlers
	onClientConnect    func(*ClientInfo)
	onClientDisconnect func(*ClientInfo)
	onClientMessage    func(*ClientInfo, []byte)
	onError            func(error)
}

// NewServerBus creates a new ServerBus with default configuration.
func NewServerBus() *ServerBus {
	return NewServerBusWithConfig(DefaultServerBusConfig())
}

// NewServerBusWithConfig creates a new ServerBus with specified configuration.
func NewServerBusWithConfig(config ServerBusConfig) *ServerBus {
	return &ServerBus{
		config:    config,
		stopCh:    make(chan struct{}),
		clients:   make(map[string]*ClientInfo),
		handshake: protocol.NewHandshakeManager(protocol.DefaultNegotiationPolicy()),
		busConfig: DefaultBusConfig(),
	}
}

// SetServerChannel sets the server channel.
func (s *ServerBus) SetServerChannel(ch channel.ServerChannel) *ServerBus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serverChannel = ch
	return s
}

// SetSerializer sets the serializer for client buses.
func (s *ServerBus) SetSerializer(ser serializer.Serializer) *ServerBus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serializer = ser
	return s
}

// SetCodecChain sets the codec chain for client buses.
func (s *ServerBus) SetCodecChain(chain codec.CodecChain) *ServerBus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codecChain = chain
	return s
}

// SetKeyProvider sets the key provider for client buses.
func (s *ServerBus) SetKeyProvider(kp keyprovider.KeyProvider) *ServerBus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keyProvider = kp
	return s
}

// SetBusConfig sets the bus configuration for client buses.
func (s *ServerBus) SetBusConfig(config BusConfig) *ServerBus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busConfig = config
	return s
}

// OnClientConnect registers client connect handler.
func (s *ServerBus) OnClientConnect(handler func(*ClientInfo)) *ServerBus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onClientConnect = handler
	return s
}

// OnClientDisconnect registers client disconnect handler.
func (s *ServerBus) OnClientDisconnect(handler func(*ClientInfo)) *ServerBus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onClientDisconnect = handler
	return s
}

// OnClientMessage registers client message handler.
func (s *ServerBus) OnClientMessage(handler func(*ClientInfo, []byte)) *ServerBus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onClientMessage = handler
	return s
}

// OnError registers error handler.
func (s *ServerBus) OnError(handler func(error)) *ServerBus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onError = handler
	return s
}

// Start starts the server bus.
func (s *ServerBus) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return ErrServerBusAlreadyRunning
	}

	// Validate configuration
	if err := s.config.Validate(); err != nil {
		return err
	}

	// Validate required modules
	if s.serverChannel == nil {
		return ErrServerChannelRequired
	}
	if s.serializer == nil {
		return errors.New("serverbus: serializer required")
	}
	if s.codecChain == nil {
		return errors.New("serverbus: codec chain required")
	}

	// Check if key provider is required
	if s.codecChain.SecurityLevel() >= codec.SecurityLevelMedium {
		if s.keyProvider == nil {
			return ErrKeyProviderRequired
		}
	}

	// Reset stop channel
	s.stopCh = make(chan struct{})
	s.running = true

	// Start accept loop
	go s.acceptLoop()

	return nil
}

// Stop stops the server bus.
func (s *ServerBus) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return ErrServerBusNotRunning
	}

	s.running = false
	close(s.stopCh)

	// Stop all client buses
	for _, client := range s.clients {
		if client.Bus != nil && client.Bus.IsRunning() {
			client.Bus.Stop()
		}
	}

	// Clear clients
	s.clients = make(map[string]*ClientInfo)

	// Close server channel
	if s.serverChannel != nil {
		return s.serverChannel.Close()
	}

	return nil
}

// acceptLoop handles incoming connections.
func (s *ServerBus) acceptLoop() {
	for {
		select {
		case <-s.stopCh:
			return
		default:
			// Accept new connection
			clientChannel, err := s.serverChannel.Accept()
			if err != nil {
				s.handleError("accept", err)
				continue
			}

			// Check max clients
			s.mu.RLock()
			if s.config.MaxClients > 0 && len(s.clients) >= s.config.MaxClients {
				s.mu.RUnlock()
				clientChannel.Close()
				s.handleError("accept", errors.New("max clients reached"))
				continue
			}
			s.mu.RUnlock()

			// Handle new client in background
			go s.handleNewClient(clientChannel)
		}
	}
}

// handleNewClient handles a new client connection.
func (s *ServerBus) handleNewClient(clientChannel channel.Channel) {
	// Create temporary client info
	tempSessionID := internal.GenerateSessionID()
	tempClient := &ClientInfo{
		SessionID:     tempSessionID,
		ConnectedAt:   time.Now().Unix(),
		LastActivity:  time.Now().Unix(),
		RemoteAddress: string(clientChannel.Type()),
		Status:        ClientStatusHandshaking,
	}

	// Notify connect
	if s.onClientConnect != nil {
		s.onClientConnect(tempClient)
	}

	// Perform handshake
	bus, finalSessionID, err := s.performHandshake(clientChannel)
	if err != nil {
		clientChannel.Close()
		s.handleError("handshake", err)
		if s.onClientDisconnect != nil {
			tempClient.Status = ClientStatusDisconnected
			s.onClientDisconnect(tempClient)
		}
		return
	}

	// Update client info
	tempClient.SessionID = finalSessionID
	tempClient.Bus = bus
	tempClient.Status = ClientStatusActive
	tempClient.Serializer = s.serializer.Name()
	tempClient.SecurityLevel = s.codecChain.SecurityLevel()

	// Register client
	s.mu.Lock()
	s.clients[finalSessionID] = tempClient
	s.mu.Unlock()

	// Start client message handler
	go s.handleClientMessages(tempClient)
}

// performHandshake performs the handshake negotiation.
func (s *ServerBus) performHandshake(clientChannel channel.Channel) (*Bus, string, error) {
	// Step 1: Receive handshake request
	reqData, err := clientChannel.Receive()
	if err != nil {
		return nil, "", err
	}

	// Deserialize request (using plain serializer for handshake)
	req, err := s.deserializeHandshakeRequest(reqData)
	if err != nil {
		return nil, "", err
	}

	// Step 2: Process request and create response
	response, err := s.handshake.ProcessRequest(req)
	if err != nil {
		return nil, "", err
	}

	// Serialize response
	respData, err := s.serializeHandshakeResponse(response)
	if err != nil {
		return nil, "", err
	}

	// Send response
	if err := clientChannel.Send(respData); err != nil {
		return nil, "", err
	}

	// If rejected, return error
	if !response.Accepted {
		return nil, "", protocol.ErrHandshakeFailed
	}

	// Step 3: Receive confirmation
	confirmData, err := clientChannel.Receive()
	if err != nil {
		return nil, "", err
	}

	// Deserialize confirmation
	confirm, err := s.deserializeHandshakeConfirm(confirmData)
	if err != nil {
		return nil, "", err
	}

	// Process confirmation
	result, err := s.handshake.ProcessConfirm(confirm)
	if err != nil {
		return nil, "", err
	}

	// Step 4: Create client Bus
	bus := NewWithConfig(s.busConfig)
	bus.SetSerializer(s.serializer)
	bus.SetCodecChain(s.codecChain.Clone())
	bus.SetChannel(clientChannel)

	if s.keyProvider != nil {
		bus.SetKeyProvider(s.keyProvider)
	}

	// Start bus
	if err := bus.Start(); err != nil {
		return nil, "", err
	}

	return bus, result.SessionID, nil
}

// handleClientMessages handles messages from a client.
func (s *ServerBus) handleClientMessages(client *ClientInfo) {
	for {
		select {
		case <-s.stopCh:
			return
		default:
			if client.Bus == nil || !client.Bus.IsRunning() {
				return
			}

			// Receive message
			data, err := client.Bus.Receive()
			if err != nil {
				s.handleClientDisconnect(client)
				return
			}

			// Update last activity
			client.LastActivity = time.Now().Unix()

			// Call message handler
			if s.onClientMessage != nil && data != nil {
				s.onClientMessage(client, data)
			}
		}
	}
}

// handleClientDisconnect handles client disconnect.
func (s *ServerBus) handleClientDisconnect(client *ClientInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	client.Status = ClientStatusDisconnected

	if client.Bus != nil {
		client.Bus.Stop()
	}

	delete(s.clients, client.SessionID)

	if s.onClientDisconnect != nil {
		s.onClientDisconnect(client)
	}
}

// SendTo sends data to a specific client.
func (s *ServerBus) SendTo(sessionID string, data []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.running {
		return ErrServerBusNotRunning
	}

	client, exists := s.clients[sessionID]
	if !exists {
		return ErrClientNotFound
	}

	if client.Status != ClientStatusActive || client.Bus == nil {
		return ErrClientDisconnected
	}

	client.LastActivity = time.Now().Unix()
	return client.Bus.Send(data)
}

// Broadcast sends data to all active clients.
func (s *ServerBus) Broadcast(data []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.running {
		return ErrServerBusNotRunning
	}

	var lastErr error
	for _, client := range s.clients {
		if client.Status == ClientStatusActive && client.Bus != nil {
			if err := client.Bus.Send(data); err != nil {
				lastErr = err
				s.handleError("broadcast", err)
			}
			client.LastActivity = time.Now().Unix()
		}
	}

	return lastErr
}

// GetClient returns client info by session ID.
func (s *ServerBus) GetClient(sessionID string) (*ClientInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	client, exists := s.clients[sessionID]
	if !exists {
		return nil, ErrClientNotFound
	}

	return client, nil
}

// ListClients returns all active clients.
func (s *ServerBus) ListClients() []*ClientInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*ClientInfo, 0, len(s.clients))
	for _, client := range s.clients {
		if client.Status == ClientStatusActive {
			result = append(result, client)
		}
	}
	return result
}

// ClientCount returns the number of active clients.
func (s *ServerBus) ClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, client := range s.clients {
		if client.Status == ClientStatusActive {
			count++
		}
	}
	return count
}

// IsRunning returns running status.
func (s *ServerBus) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetConfig returns current configuration.
func (s *ServerBus) GetConfig() ServerBusConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// handleError handles errors.
func (s *ServerBus) handleError(op string, err error) {
	if s.onError != nil {
		s.onError(errors.New(op + ": " + err.Error()))
	}
}

// Handshake serialization helpers

func (s *ServerBus) deserializeHandshakeRequest(data []byte) (*protocol.HandshakeRequest, error) {
	return protocol.DeserializeRequest(data)
}

func (s *ServerBus) serializeHandshakeResponse(resp *protocol.HandshakeResponse) ([]byte, error) {
	return protocol.SerializeResponse(resp)
}

func (s *ServerBus) deserializeHandshakeConfirm(data []byte) (*protocol.HandshakeConfirm, error) {
	return protocol.DeserializeConfirm(data)
}

// ServerBusBuilder provides fluent API for building ServerBus.
type ServerBusBuilder struct {
	serverBus *ServerBus
}

// NewServerBusBuilder creates a new ServerBusBuilder.
func NewServerBusBuilder() *ServerBusBuilder {
	return &ServerBusBuilder{
		serverBus: NewServerBus(),
	}
}

// UseServerChannel sets the server channel.
func (b *ServerBusBuilder) UseServerChannel(ch channel.ServerChannel) *ServerBusBuilder {
	b.serverBus.SetServerChannel(ch)
	return b
}

// UseSerializer sets the serializer.
func (b *ServerBusBuilder) UseSerializer(ser serializer.Serializer) *ServerBusBuilder {
	b.serverBus.SetSerializer(ser)
	return b
}

// UseSerializerFromRegistry sets the serializer from registry.
func (b *ServerBusBuilder) UseSerializerFromRegistry(name string) *ServerBusBuilder {
	ser, err := serializer.Get(name)
	if err == nil {
		b.serverBus.SetSerializer(ser)
	}
	return b
}

// UseCodecChain sets the codec chain.
func (b *ServerBusBuilder) UseCodecChain(chain codec.CodecChain) *ServerBusBuilder {
	b.serverBus.SetCodecChain(chain)
	return b
}

// UseCodec adds a codec to the chain.
func (b *ServerBusBuilder) UseCodec(c codec.Codec) *ServerBusBuilder {
	if b.serverBus.codecChain == nil {
		b.serverBus.codecChain = codec.NewChain()
	}
	b.serverBus.codecChain.AddCodec(c)
	return b
}

// UseKeyProvider sets the key provider.
func (b *ServerBusBuilder) UseKeyProvider(kp keyprovider.KeyProvider) *ServerBusBuilder {
	b.serverBus.SetKeyProvider(kp)
	return b
}

// WithConfig sets the server bus configuration.
func (b *ServerBusBuilder) WithConfig(config ServerBusConfig) *ServerBusBuilder {
	b.serverBus.config = config
	return b
}

// WithBusConfig sets the client bus configuration.
func (b *ServerBusBuilder) WithBusConfig(config BusConfig) *ServerBusBuilder {
	b.serverBus.SetBusConfig(config)
	return b
}

// WithNegotiationPolicy sets the negotiation policy.
func (b *ServerBusBuilder) WithNegotiationPolicy(policy protocol.NegotiationPolicy) *ServerBusBuilder {
	b.serverBus.handshake = protocol.NewHandshakeManager(policy)
	return b
}

// OnClientConnect sets client connect handler.
func (b *ServerBusBuilder) OnClientConnect(handler func(*ClientInfo)) *ServerBusBuilder {
	b.serverBus.OnClientConnect(handler)
	return b
}

// OnClientDisconnect sets client disconnect handler.
func (b *ServerBusBuilder) OnClientDisconnect(handler func(*ClientInfo)) *ServerBusBuilder {
	b.serverBus.OnClientDisconnect(handler)
	return b
}

// OnClientMessage sets client message handler.
func (b *ServerBusBuilder) OnClientMessage(handler func(*ClientInfo, []byte)) *ServerBusBuilder {
	b.serverBus.OnClientMessage(handler)
	return b
}

// OnError sets error handler.
func (b *ServerBusBuilder) OnError(handler func(error)) *ServerBusBuilder {
	b.serverBus.OnError(handler)
	return b
}

// Build builds and returns the ServerBus.
func (b *ServerBusBuilder) Build() *ServerBus {
	return b.serverBus
}

// BuildAndStart builds and starts the ServerBus.
func (b *ServerBusBuilder) BuildAndStart() (*ServerBus, error) {
	sb := b.Build()
	err := sb.Start()
	return sb, err
}

// Verify interface compliance
var (
	_ ServerBusInterface = (*ServerBus)(nil)
	// Note: ServerBus methods return *ServerBus for method chaining
	// ServerBusConfigurer is intentionally not implemented to maintain fluent API
)
