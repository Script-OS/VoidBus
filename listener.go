// Package voidbus provides net.Listener implementation for VoidBus server connections.
//
// voidBusListener implements net.Listener for VoidBus server mode:
// - Accept: waits for and returns next client connection (net.Conn)
// - Close: stops listening and releases resources
// - Addr: returns the listening address
//
// Accept semantics:
// - Each Accept returns a net.Conn for a new client
// - The returned connection is already negotiated (codec/channels matched)
// - Client receives complete messages through conn.Read
package voidbus

import (
	"net"
	"sync"

	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/internal"
	"github.com/Script-OS/VoidBus/negotiate"
)

// voidBusListener implements net.Listener for VoidBus server.
type voidBusListener struct {
	bus       *Bus
	serverCh  channel.ServerChannel // The server channel for accepting connections
	channelID string                // Channel ID assigned by Bus

	// Accept state
	acceptChan chan net.Conn // Channel for accepted client connections
	errChan    chan error    // Channel for accept errors

	// Address
	addr net.Addr

	// State
	closed  bool
	closeMu sync.Mutex

	// WaitGroup for accept loop
	wg sync.WaitGroup
}

// Accept waits for and returns the next connection.
// Returns net.Conn (VoidBusConn) for each accepted client.
// The returned connection is already negotiated (codec/channels matched).
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

// Close stops the listener.
// It closes the server channel first to unblock the acceptLoop (which may be
// blocked on serverCh.Accept()), then waits for the loop to exit.
func (l *voidBusListener) Close() error {
	l.closeMu.Lock()
	if l.closed {
		l.closeMu.Unlock()
		return nil
	}
	l.closed = true

	// Close server channel first to unblock Accept() in acceptLoop.
	if l.serverCh != nil {
		l.serverCh.Close()
	}

	// Release closeMu BEFORE waiting — acceptLoop checks l.closed under this lock,
	// so holding it while waiting would deadlock.
	l.closeMu.Unlock()

	// Now wait for accept loop to exit (it will see l.closed == true)
	l.wg.Wait()

	return nil
}

// Addr returns the listener's network address.
func (l *voidBusListener) Addr() net.Addr {
	return l.addr
}

// acceptLoop runs in background, accepting new client connections.
// For each client:
// 1. Accept new channel connection
// 2. Handle negotiation automatically (wait for request → match codecs → send response)
// 3. Create VoidBusConn for client
// 4. Send conn to acceptChan
func (l *voidBusListener) acceptLoop() {
	defer l.wg.Done()

	for {
		l.closeMu.Lock()
		closed := l.closed
		l.closeMu.Unlock()
		if closed {
			return
		}

		// Accept new client connection
		clientCh, err := l.serverCh.Accept()
		if err != nil {
			// If listener was closed, Accept() returns error — exit cleanly.
			l.closeMu.Lock()
			closed = l.closed
			l.closeMu.Unlock()
			if closed {
				return
			}
			select {
			case l.errChan <- err:
			default:
				// Error channel full, skip
			}
			continue
		}

		// Handle client in background.
		// Check closed state before spawning to avoid wg.Add after wg.Wait race.
		l.closeMu.Lock()
		if l.closed {
			l.closeMu.Unlock()
			clientCh.Close()
			return
		}
		l.wg.Add(1)
		l.closeMu.Unlock()
		go l.handleClient(clientCh)
	}
}

// handleClient handles a single client connection.
// Automatically performs negotiation and creates VoidBusConn.
func (l *voidBusListener) handleClient(clientCh channel.Channel) {
	defer l.wg.Done()

	// Defensive check: Accept() may return nil channel on shutdown
	if clientCh == nil {
		return
	}

	// 1. Wait for client negotiation request
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

	// 3. Match codecs with server's supported codecs
	serverCodecBitmap := l.bus.codecManager.GenerateCodecBitmap()
	matchedCodecBitmap := negotiate.IntersectCodecBitmaps(request.CodecBitmap, serverCodecBitmap)

	serverChannelBitmap := l.bus.channelPool.GenerateChannelBitmap()
	matchedChannelBitmap := negotiate.IntersectChannelBitmaps(request.ChannelBitmap, serverChannelBitmap)

	// 4. Check if negotiation successful
	if negotiate.IsCodecBitmapEmpty(matchedCodecBitmap) {
		clientCh.Close()
		return
	}

	// 5. Create negotiation response
	response, err := negotiate.NewNegotiateResponse(matchedChannelBitmap, matchedCodecBitmap, request.SessionNonce, negotiate.NegotiateStatusSuccess)
	if err != nil {
		clientCh.Close()
		return
	}

	// 6. Send negotiation response
	responseData, err := response.Encode()
	if err != nil {
		clientCh.Close()
		return
	}

	if err := clientCh.Send(responseData); err != nil {
		clientCh.Close()
		return
	}

	// 7. Create new Bus for client (independent session)
	clientBus, err := New(nil)
	if err != nil {
		clientCh.Close()
		return
	}

	// 7.0 Enable debug mode for client bus if main bus has debug mode
	if l.bus.config.DebugMode {
		clientBus.SetDebugMode(true)
	}

	// 7.1 Copy codecs from main bus to client bus
	// Note: clientBus needs codecs to decode incoming messages
	for _, code := range l.bus.codecManager.GetAvailableCodes() {
		if c, err := l.bus.codecManager.GetCodec(code); err == nil {
			clientBus.RegisterCodec(c)
		}
	}

	// 8. Apply negotiated codec bitmap to client bus
	clientBus.codecManager.SetNegotiatedBitmap(matchedCodecBitmap)

	// 9. Generate channel ID for client
	channelID := string(clientCh.Type()) + "-" + internal.GenerateShortID()

	// 10. Add client channel to client bus
	if err := clientBus.AddChannelWithID(clientCh, channelID); err != nil {
		clientCh.Close()
		return
	}

	// 11. Mark client bus as connected and negotiated
	clientBus.connected.Store(true)
	clientBus.negotiated.Store(true)

	// 12. Create receive channel for client (buffer for complete messages)
	recvChan := make(chan []byte, 100)

	// 13. Start receive loop for client bus
	clientBus.running.Store(true)
	go clientBus.nakBatchLoop()
	chIDs := clientBus.channelPool.GetChannelIDs()
	for _, chID := range chIDs {
		info, err := clientBus.channelPool.GetChannel(chID)
		if err != nil {
			continue
		}
		clientBus.wg.Add(1)
		go clientBus.receiveLoop(info)
	}

	// 14. Create VoidBusConn for client
	conn := newVoidBusConn(clientBus, channelID, clientCh.Type(), recvChan)

	// 15. Send conn to acceptChan
	select {
	case l.acceptChan <- conn:
	default:
		// Accept channel full, close connection
		conn.Close()
	}

	// 16. Bridge bus receive to conn recvChan
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

// newVoidBusListener creates a new VoidBus listener.
func newVoidBusListener(bus *Bus, serverCh channel.ServerChannel, channelID string) *voidBusListener {
	listener := &voidBusListener{
		bus:        bus,
		serverCh:   serverCh,
		channelID:  channelID,
		acceptChan: make(chan net.Conn, 10),
		errChan:    make(chan error, 10),
	}

	// Set address
	if serverCh != nil {
		network := "voidbus-" + string(serverCh.Type())
		listener.addr = NewVoidBusAddr(network, serverCh.ListenAddress())
	} else {
		listener.addr = NewVoidBusAddr("voidbus", "")
	}

	// Start accept loop
	listener.wg.Add(1)
	go listener.acceptLoop()

	return listener
}
