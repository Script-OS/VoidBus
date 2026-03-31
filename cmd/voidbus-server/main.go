// Package main provides a standalone VoidBus server.
//
// Features:
// - Listens for multiple clients
// - Supports codec negotiation
// - Full-duplex communication
// - Automatic data reassembly
//
// Build: go build -o voidbus-server ./cmd/voidbus-server
// Run: ./voidbus-server -addr :9000
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"VoidBus/channel"
	"VoidBus/channel/tcp"
	"VoidBus/codec"
	"VoidBus/codec/base64"
	"VoidBus/core"
	"VoidBus/fragment"
	"VoidBus/fragment/simple"
	"VoidBus/keyprovider/embedded"
	"VoidBus/protocol"
	"VoidBus/serializer/plain"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	Address        string
	MaxClients     int
	DefaultKey     string
	EnableFragment bool
	FragmentSize   int
}

// NegotiationMessage for handshake
type NegotiationMessage struct {
	Type       string `json:"type"`
	ClientID   string `json:"client_id,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Serializer string `json:"serializer,omitempty"`
	CodecLevel int    `json:"codec_level,omitempty"`
	Challenge  []byte `json:"challenge,omitempty"`
	Response   []byte `json:"response,omitempty"`
	Accepted   bool   `json:"accepted,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// ClientSession represents a connected client
type ClientSession struct {
	SessionID   string
	ClientID    string
	Bus         *core.Bus
	CodecLevel  codec.SecurityLevel
	ConnectedAt time.Time
	RecvCount   int64
	SendCount   int64
}

// Server manages all client connections
type Server struct {
	config    ServerConfig
	serverBus *core.ServerBus
	sessions  map[string]*ClientSession
	mu        sync.RWMutex
	key       []byte
	fragment  fragment.Fragment
}

func main() {
	addr := flag.String("addr", ":9000", "Server address")
	maxClients := flag.Int("max", 100, "Maximum clients")
	key := flag.String("key", "voidbus-server-key-32bytes!!", "Encryption key (32 bytes)")
	flag.Parse()

	config := ServerConfig{
		Address:        *addr,
		MaxClients:     *maxClients,
		DefaultKey:     *key,
		EnableFragment: true,
		FragmentSize:   4096,
	}

	server := &Server{
		config:   config,
		sessions: make(map[string]*ClientSession),
		key:      []byte(config.DefaultKey),
		fragment: simple.New(fragment.DefaultFragmentConfig()),
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Server start failed: %v", err)
	}

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down server...")
	server.Stop()
}

// Start starts the server
func (s *Server) Start() error {
	// Create server channel
	serverConfig := channel.ChannelConfig{Address: s.config.Address}
	serverCh, err := tcp.NewServerChannel(serverConfig)
	if err != nil {
		return fmt.Errorf("create server channel: %w", err)
	}

	// Create key provider
	kp, err := embedded.New(s.key, "server-key", "AES-256-GCM")
	if err != nil {
		return fmt.Errorf("create key provider: %w", err)
	}

	// Create server bus
	s.serverBus = core.NewServerBusBuilder().
		UseServerChannel(serverCh).
		UseSerializer(plain.New()).
		UseCodec(base64.New()). // Default codec
		UseKeyProvider(kp).
		WithBusConfig(core.BusConfig{
			EnableFragment:  s.config.EnableFragment,
			MaxFragmentSize: s.config.FragmentSize,
			AsyncReceive:    false,
		}).
		OnClientConnect(s.onClientConnect).
		OnClientDisconnect(s.onClientDisconnect).
		OnClientMessage(s.onClientMessage).
		OnError(func(err error) { log.Printf("[ERROR] %v", err) }).
		Build()

	if err := s.serverBus.Start(); err != nil {
		return fmt.Errorf("start server bus: %w", err)
	}

	log.Printf("Server started on %s", s.config.Address)
	log.Printf("Fragment enabled: %v, size: %d", s.config.EnableFragment, s.config.FragmentSize)

	return nil
}

// Stop stops the server
func (s *Server) Stop() {
	if s.serverBus != nil {
		s.serverBus.Stop()
	}
}

func (s *Server) onClientConnect(client *core.ClientInfo) {
	log.Printf("[CONNECT] Client: %s, Status: %s", client.SessionID, client.Status)
}

func (s *Server) onClientDisconnect(client *core.ClientInfo) {
	log.Printf("[DISCONNECT] Client: %s", client.SessionID)
	s.mu.Lock()
	delete(s.sessions, client.SessionID)
	s.mu.Unlock()
}

func (s *Server) onClientMessage(client *core.ClientInfo, data []byte) {
	s.mu.Lock()
	session, exists := s.sessions[client.SessionID]
	if !exists {
		session = &ClientSession{
			SessionID:   client.SessionID,
			ConnectedAt: time.Now(),
		}
		s.sessions[client.SessionID] = session
	}
	session.RecvCount++
	s.mu.Unlock()

	// Parse message
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err == nil {
		msgType, _ := msg["type"].(string)
		switch msgType {
		case "negotiate":
			s.handleNegotiation(client.SessionID, msg)
			return
		case "data":
			// Handle data message
			if payload, ok := msg["payload"].([]byte); ok {
				log.Printf("[DATA] From %s: %d bytes", client.SessionID, len(payload))
				// Echo back
				response := map[string]interface{}{
					"type":    "ack",
					"seq":     msg["seq"],
					"status":  "ok",
					"message": fmt.Sprintf("Received %d bytes", len(payload)),
				}
				respData, _ := json.Marshal(response)
				s.serverBus.SendTo(client.SessionID, respData)
			}
		default:
			log.Printf("[MSG] From %s: %s", client.SessionID, string(data))
		}
	} else {
		log.Printf("[RAW] From %s: %d bytes", client.SessionID, len(data))
		// Echo back
		response := map[string]interface{}{
			"type":    "ack",
			"status":  "ok",
			"message": fmt.Sprintf("Received %d bytes", len(data)),
		}
		respData, _ := json.Marshal(response)
		s.serverBus.SendTo(client.SessionID, respData)
	}
}

func (s *Server) handleNegotiation(sessionID string, msg map[string]interface{}) {
	// Parse client's codec capabilities
	codecLevel, _ := msg["codec_level"].(float64)

	// Log the negotiated codec level
	log.Printf("[NEGOTIATE] Session %s: codec_level=%d", sessionID, int(codecLevel))

	// Send negotiation response
	response := map[string]interface{}{
		"type":        "negotiate_ack",
		"session_id":  sessionID,
		"codec_level": codecLevel,
		"accepted":    true,
	}
	respData, _ := json.Marshal(response)
	s.serverBus.SendTo(sessionID, respData)
}

func (s *Server) broadcast(message []byte) error {
	return s.serverBus.Broadcast(message)
}

func (s *Server) sendTo(sessionID string, data []byte) error {
	return s.serverBus.SendTo(sessionID, data)
}

func (s *Server) listClients() []*core.ClientInfo {
	return s.serverBus.ListClients()
}

// Handshake helper - simplified version using existing protocol
func createHandshakeRequest() *protocol.HandshakeRequest {
	req := protocol.NewHandshakeRequest("client")
	req.AddSerializer("plain", 10)
	req.AddCodecChain(codec.SecurityLevelHigh, 1, "aes256")
	req.AddCodecChain(codec.SecurityLevelMedium, 1, "aes128")
	req.AddCodecChain(codec.SecurityLevelLow, 1, "base64")
	return req
}
