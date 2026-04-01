// Package negotiate provides server negotiator implementation.
package negotiate

import (
	"sync"
	"time"
)

// ServerNegotiatorImpl implements ServerNegotiator interface.
type ServerNegotiatorImpl struct {
	mu sync.RWMutex

	channelBitmap ChannelBitmap
	codecBitmap   CodecBitmap
	timeout       time.Duration
	maxRetryCount int
}

// NewServerNegotiator creates a new server negotiator.
func NewServerNegotiator(config *NegotiatorConfig) *ServerNegotiatorImpl {
	if config == nil {
		config = DefaultNegotiatorConfig()
	}

	return &ServerNegotiatorImpl{
		channelBitmap: config.ChannelBitmap,
		codecBitmap:   config.CodecBitmap,
		timeout:       config.Timeout,
		maxRetryCount: config.MaxRetryCount,
	}
}

// SetTimeout sets the negotiation timeout.
func (n *ServerNegotiatorImpl) SetTimeout(timeout time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.timeout = timeout
}

// GetTimeout returns the current timeout.
func (n *ServerNegotiatorImpl) GetTimeout() time.Duration {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.timeout
}

// SetChannelBitmap sets the server's supported channels.
func (n *ServerNegotiatorImpl) SetChannelBitmap(bitmap ChannelBitmap) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.channelBitmap = bitmap
}

// SetCodecBitmap sets the server's supported codecs.
func (n *ServerNegotiatorImpl) SetCodecBitmap(bitmap CodecBitmap) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.codecBitmap = bitmap
}

// GetChannelBitmap returns the server's channel bitmap.
func (n *ServerNegotiatorImpl) GetChannelBitmap() ChannelBitmap {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.channelBitmap
}

// GetCodecBitmap returns the server's codec bitmap.
func (n *ServerNegotiatorImpl) GetCodecBitmap() CodecBitmap {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.codecBitmap
}

// HandleRequest processes a negotiation request from client.
func (n *ServerNegotiatorImpl) HandleRequest(request *NegotiateRequest) (*NegotiateResponse, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Compute intersection of channel bitmaps
	availableChannels := IntersectChannelBitmaps(n.channelBitmap, request.ChannelBitmap)

	// Compute intersection of codec bitmaps
	availableCodecs := IntersectCodecBitmaps(n.codecBitmap, request.CodecBitmap)

	// Check if intersection is empty
	if IsChannelBitmapEmpty(availableChannels) {
		// No common channels, reject
		return NewNegotiateResponse(
			[]byte{0},
			[]byte{0},
			request.SessionNonce,
			NegotiateStatusReject,
		)
	}

	if IsCodecBitmapEmpty(availableCodecs) {
		// No common codecs, reject
		return NewNegotiateResponse(
			[]byte{0},
			[]byte{0},
			request.SessionNonce,
			NegotiateStatusReject,
		)
	}

	// Create success response
	response, err := NewNegotiateResponse(
		availableChannels,
		availableCodecs,
		request.SessionNonce,
		NegotiateStatusSuccess,
	)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// HandleRawRequest processes raw encoded request bytes.
func (n *ServerNegotiatorImpl) HandleRawRequest(data []byte) (*NegotiateResponse, error) {
	// Decode request
	request, err := DecodeNegotiateRequest(data)
	if err != nil {
		return nil, err
	}

	return n.HandleRequest(request)
}

// CreateResult creates a Result from request and response.
func (n *ServerNegotiatorImpl) CreateResult(request *NegotiateRequest, response *NegotiateResponse) *Result {
	return &Result{
		AvailableChannels: response.ChannelBitmap,
		AvailableCodecs:   response.CodecBitmap,
		SessionID:         response.SessionID,
		Status:            response.Status,
		NegotiatedAt:      time.Now(),
	}
}

// ValidateRequest validates a negotiation request.
func (n *ServerNegotiatorImpl) ValidateRequest(request *NegotiateRequest) error {
	// Validate timestamp (already done in DecodeNegotiateRequest)
	// Validate nonce size (already done in DecodeNegotiateRequest)

	// Check for empty bitmaps
	if IsChannelBitmapEmpty(request.ChannelBitmap) {
		return ErrNoCommonChannels
	}

	if IsCodecBitmapEmpty(request.CodecBitmap) {
		return ErrNoCommonCodecs
	}

	return nil
}

// GetAvailableChannels returns server's available channels.
func (n *ServerNegotiatorImpl) GetAvailableChannels() []ChannelID {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.channelBitmap.GetChannelIDs()
}

// GetAvailableCodecs returns server's available codecs.
func (n *ServerNegotiatorImpl) GetAvailableCodecs() []CodecID {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.codecBitmap.GetCodecIDs()
}

// SessionManager manages negotiated sessions.
type SessionManager struct {
	mu sync.RWMutex

	sessions map[string]*Result // SessionID -> Result
}

// NewSessionManager creates a new session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Result),
	}
}

// AddSession adds a negotiated session.
func (s *SessionManager) AddSession(sessionID []byte, result *Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(sessionID)
	s.sessions[key] = result
}

// GetSession retrieves a negotiated session.
func (s *SessionManager) GetSession(sessionID []byte) (*Result, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := string(sessionID)
	result, ok := s.sessions[key]
	return result, ok
}

// RemoveSession removes a negotiated session.
func (s *SessionManager) RemoveSession(sessionID []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(sessionID)
	delete(s.sessions, key)
}

// HasSession checks if a session exists.
func (s *SessionManager) HasSession(sessionID []byte) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := string(sessionID)
	return s.sessions[key] != nil
}

// SessionCount returns the number of active sessions.
func (s *SessionManager) SessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}
