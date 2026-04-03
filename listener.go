// Package voidbus provides net.Listener implementation for VoidBus server connections.
//
// voidBusListener implements net.Listener for VoidBus server mode:
// - Accept: waits for and returns next client connection (net.Conn)
// - Close: stops listening and releases resources
// - Addr: returns the listening address
//
// Multi-channel architecture (v2.0):
// - Listener aggregates all registered ServerChannels
// - Each channel runs its own acceptLoop
// - Sessions are managed by SessionRegistry
// - Accept returns conn when Session is ready (all channels connected)
package voidbus

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/internal"
	"github.com/Script-OS/VoidBus/negotiate"
)

// voidBusListener implements net.Listener for VoidBus server.
// Aggregates multiple ServerChannels for multi-channel connections.
type voidBusListener struct {
	bus *Bus

	// All server channels (aggregated)
	serverChannels map[string]channel.ServerChannel // ChannelID -> ServerChannel

	// Session registry
	sessionRegistry *negotiate.SessionRegistry

	// Accept state
	acceptChan chan net.Conn // Channel for ready session connections
	errChan    chan error    // Channel for accept errors

	// Address (primary channel's address)
	addr net.Addr

	// State
	closed  bool
	closeMu sync.Mutex

	// WaitGroup for accept loops
	wg sync.WaitGroup

	// Session ready timeout
	sessionTimeout time.Duration
}

// Accept waits for and returns the next ready session connection.
// Returns net.Conn (VoidBusConn) when all negotiated channels are connected.
func (l *voidBusListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.acceptChan:
		return conn, nil
	case err := <-l.errChan:
		return nil, err
	case <-l.bus.stopChan:
		return nil, net.ErrClosed
	}
}

// Close stops all listeners.
func (l *voidBusListener) Close() error {
	l.closeMu.Lock()
	if l.closed {
		l.closeMu.Unlock()
		return nil
	}
	l.closed = true

	// Close all server channels first to unblock accept loops
	for _, serverCh := range l.serverChannels {
		if serverCh != nil {
			serverCh.Close()
		}
	}

	// Stop session registry
	if l.sessionRegistry != nil {
		l.sessionRegistry.Stop()
	}

	l.closeMu.Unlock()

	// Wait for all accept loops to exit
	l.wg.Wait()

	return nil
}

// Addr returns the listener's network address.
func (l *voidBusListener) Addr() net.Addr {
	return l.addr
}

// startAcceptLoops starts accept loops for all server channels.
func (l *voidBusListener) startAcceptLoops() {
	for chID, serverCh := range l.serverChannels {
		l.wg.Add(1)
		go l.acceptLoop(chID, serverCh)
	}
}

// acceptLoop runs in background, accepting new client connections for a specific channel.
func (l *voidBusListener) acceptLoop(channelID string, serverCh channel.ServerChannel) {
	defer l.wg.Done()

	for {
		l.closeMu.Lock()
		closed := l.closed
		l.closeMu.Unlock()
		if closed {
			return
		}

		// Accept new client connection
		clientCh, err := serverCh.Accept()
		if err != nil {
			l.closeMu.Lock()
			closed := l.closed
			l.closeMu.Unlock()
			if closed {
				return
			}
			select {
			case l.errChan <- err:
			default:
			}
			continue
		}

		// Handle client in background
		l.closeMu.Lock()
		if l.closed {
			l.closeMu.Unlock()
			clientCh.Close()
			return
		}
		l.wg.Add(1)
		l.closeMu.Unlock()
		go l.handleClient(channelID, clientCh)
	}
}

// handleClient handles a single client channel connection.
// Supports multi-channel session association:
// - First connection (no SessionID): creates new session
// - Subsequent connection (with SessionID): associates to existing session
//
// Special handling for UDP:
//   - UDP does not support multiple connections from the same client address
//   - If a subsequent UDP connection is detected, it is rejected to prevent
//     overwriting the existing AcceptedChannel in clientsByAddr
func (l *voidBusListener) handleClient(channelID string, clientCh channel.Channel) {
	defer l.wg.Done()

	if clientCh == nil {
		return
	}

	// 1. Receive negotiation request
	requestData, err := clientCh.Receive()
	if err != nil {
		select {
		case l.errChan <- err:
		default:
		}
		clientCh.Close()
		return
	}

	// 2. Decode negotiation request
	request, err := negotiate.DecodeNegotiateRequest(requestData)
	if err != nil {
		select {
		case l.errChan <- err:
		default:
		}
		clientCh.Close()
		return
	}

	// 3. Check for duplicate UDP connection rejection
	// UDP ServerChannel uses clientsByAddr[remoteAddr.String()] for routing.
	// A subsequent UDP connection from the same address would overwrite the
	// existing AcceptedChannel, causing data routing issues.
	// Only reject if the SESSION ALREADY has a UDP channel connected.
	if !request.IsFirstConnection() && clientCh.Type() == channel.TypeUDP {
		session := l.sessionRegistry.GetSession(request.SessionID)
		if session != nil && session.HasChannelType(negotiate.ChannelBitUDP) {
			if l.bus.config.DebugMode {
				println("[DEBUG] handleClient: rejecting duplicate UDP connection - session already has UDP channel")
			}
			// Send rejection response so client knows negotiation failed
			// Use a dummy SessionID (8 bytes) since Encode requires it
			dummySessionID := make([]byte, negotiate.NegotiateSessionIDSize)
			rejectResponse, err := negotiate.NewNegotiateResponse(nil, nil, dummySessionID, negotiate.NegotiateStatusReject)
			if err != nil {
				clientCh.Close()
				return
			}
			rejectData, err := rejectResponse.Encode()
			if err != nil {
				clientCh.Close()
				return
			}
			clientCh.Send(rejectData)
			clientCh.Close()
			return
		}
	}

	// 4. Compute bitmap intersection
	serverCodecBitmap := l.bus.codecManager.GenerateCodecBitmap()
	matchedCodecBitmap := negotiate.IntersectCodecBitmaps(request.CodecBitmap, serverCodecBitmap)

	serverChannelBitmap := l.bus.channelPool.GenerateChannelBitmap()
	matchedChannelBitmap := negotiate.IntersectChannelBitmaps(request.ChannelBitmap, serverChannelBitmap)

	// 5. Check if negotiation successful
	if negotiate.IsCodecBitmapEmpty(matchedCodecBitmap) || negotiate.IsChannelBitmapEmpty(matchedChannelBitmap) {
		clientCh.Close()
		return
	}

	// 6. Determine if this is first or subsequent connection
	isFirstConnection := request.IsFirstConnection()

	// 6. Get or create session
	var session *negotiate.SessionState
	var sessionID []byte
	var clientBus *Bus

	if isFirstConnection {
		// Generate new SessionID
		sessionID = negotiate.GenerateSessionID(request.SessionNonce)

		// Create new session
		session = l.sessionRegistry.CreateSession(sessionID, matchedChannelBitmap, matchedCodecBitmap)

		// Create new Bus for this session
		clientBus, err = New(nil)
		if err != nil {
			clientCh.Close()
			l.sessionRegistry.RemoveSession(sessionID)
			return
		}

		// Enable debug mode if main bus has it
		if l.bus.config.DebugMode {
			clientBus.SetDebugMode(true)
		}

		// Copy codecs from main bus
		for _, code := range l.bus.codecManager.GetAvailableCodes() {
			if c, err := l.bus.codecManager.GetCodec(code); err == nil {
				clientBus.RegisterCodec(c)
			}
		}

		// Apply negotiated codec bitmap
		clientBus.codecManager.SetNegotiatedBitmap(matchedCodecBitmap)

		// Store bus in session
		session.Bus = clientBus

		// Copy key provider
		if l.bus.keyProvider != nil {
			clientBus.keyProvider = l.bus.keyProvider
		}
	} else {
		// Use provided SessionID
		sessionID = request.SessionID

		// Find existing session
		session = l.sessionRegistry.GetSession(sessionID)
		if session == nil {
			// Session not found, reject connection
			clientCh.Close()
			return
		}

		// Get existing clientBus
		clientBus = session.Bus.(*Bus)
	}

	// 7. Create negotiation response
	response, err := negotiate.NewNegotiateResponse(matchedChannelBitmap, matchedCodecBitmap, sessionID, negotiate.NegotiateStatusSuccess)
	if err != nil {
		clientCh.Close()
		if isFirstConnection {
			l.sessionRegistry.RemoveSession(sessionID)
		}
		return
	}

	// 8. Send negotiation response
	responseData, err := response.Encode()
	if err != nil {
		clientCh.Close()
		if isFirstConnection {
			l.sessionRegistry.RemoveSession(sessionID)
		}
		return
	}

	if err := clientCh.Send(responseData); err != nil {
		clientCh.Close()
		if isFirstConnection {
			l.sessionRegistry.RemoveSession(sessionID)
		}
		return
	}

	// 9. Add channel to session
	newChannelID := fmt.Sprintf("%s-%s", clientCh.Type(), internal.GenerateShortID())
	if err := clientBus.AddChannelWithID(clientCh, newChannelID); err != nil {
		clientCh.Close()
		if isFirstConnection {
			l.sessionRegistry.RemoveSession(sessionID)
		}
		return
	}

	// 10. Determine channel bit for this connection
	channelBit := channelTypeToBit(clientCh.Type())

	// 11. Add channel to session
	session.AddChannel(channelBit, newChannelID, clientCh)

	// 12. Start receive loop for this channel immediately
	// Get ChannelInfo from pool
	if info, err := clientBus.channelPool.GetChannel(newChannelID); err == nil {
		clientBus.wg.Add(1)
		go clientBus.receiveLoop(info)
	}

	// 13. For first connection: start bus and return conn immediately
	// For subsequent connections: channel is already added, just need receive loop
	if isFirstConnection {
		l.startClientBusAndReturnConn(session, clientBus, newChannelID, clientCh.Type())
	}
}

// channelTypeToBit converts channel type to negotiate channel bit.
func channelTypeToBit(chType channel.ChannelType) negotiate.ChannelBit {
	switch chType {
	case channel.TypeWS:
		return negotiate.ChannelBitWS
	case channel.TypeTCP:
		return negotiate.ChannelBitTCP
	case channel.TypeUDP:
		return negotiate.ChannelBitUDP
	case channel.TypeICMP:
		return negotiate.ChannelBitICMP
	case channel.TypeDNS:
		return negotiate.ChannelBitDNS
	case channel.TypeHTTP:
		return negotiate.ChannelBitHTTP
	default:
		return negotiate.ChannelBitReserved
	}
}

// startClientBusAndReturnConn starts the client bus and returns the connection.
// Note: receiveLoop for the channel should already be started in handleClient.
func (l *voidBusListener) startClientBusAndReturnConn(session *negotiate.SessionState, clientBus *Bus, channelID string, chType channel.ChannelType) {
	// Mark client bus as connected and negotiated
	clientBus.connected.Store(true)
	clientBus.negotiated.Store(true)

	// Create receive channel for complete messages
	recvChan := make(chan []byte, 100)

	// Mark bus as running and start NAK batch loop
	clientBus.running.Store(true)
	go clientBus.nakBatchLoop()

	// Create VoidBusConn
	conn := newVoidBusConn(clientBus, channelID, chType, recvChan)

	// Send conn to acceptChan
	select {
	case l.acceptChan <- conn:
	default:
		// Accept channel full, close connection
		conn.Close()
		return
	}

	// Bridge bus receive to conn recvChan
	go l.bridgeReceive(clientBus, recvChan)
}

// bridgeReceive bridges clientBus receive queue to conn recvChan.
func (l *voidBusListener) bridgeReceive(clientBus *Bus, recvChan chan []byte) {
	for {
		select {
		case <-clientBus.stopChan:
			close(recvChan)
			return
		case data, ok := <-clientBus.recvQueue:
			if !ok {
				close(recvChan)
				return
			}
			select {
			case recvChan <- data:
			default:
				// Channel full, drop message
			}
		}
	}
}

// newVoidBusListener creates a new VoidBus listener with multi-channel support.
func newVoidBusListener(bus *Bus, sessionTimeout time.Duration) *voidBusListener {
	// Use server channels from bus (already populated during AddChannel)
	serverChannels := bus.serverChannels

	// Create session registry
	sessionRegistry := negotiate.NewSessionRegistry(&negotiate.SessionRegistryConfig{
		SessionTimeout:  sessionTimeout,
		CleanupInterval: 60 * time.Second,
		MaxSessionAge:   5 * time.Minute,
	})

	listener := &voidBusListener{
		bus:             bus,
		serverChannels:  serverChannels,
		sessionRegistry: sessionRegistry,
		acceptChan:      make(chan net.Conn, 10),
		errChan:         make(chan error, 10),
		sessionTimeout:  sessionTimeout,
	}

	// Set address (use first server channel's address)
	for _, serverCh := range serverChannels {
		if serverCh != nil {
			network := "voidbus-" + string(serverCh.Type())
			listener.addr = NewVoidBusAddr(network, serverCh.ListenAddress())
			break
		}
	}
	if listener.addr == nil {
		listener.addr = NewVoidBusAddr("voidbus", "")
	}

	// Start accept loops
	listener.startAcceptLoops()

	return listener
}
