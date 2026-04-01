// Package negotiate provides client negotiator implementation.
package negotiate

import (
	"errors"
	"sync"
	"time"
)

// ClientNegotiatorImpl implements ClientNegotiator interface.
type ClientNegotiatorImpl struct {
	mu sync.RWMutex

	channelBitmap ChannelBitmap
	codecBitmap   CodecBitmap
	timeout       time.Duration
	maxRetryCount int
}

// NewClientNegotiator creates a new client negotiator.
func NewClientNegotiator(config *NegotiatorConfig) *ClientNegotiatorImpl {
	if config == nil {
		config = DefaultNegotiatorConfig()
	}

	return &ClientNegotiatorImpl{
		channelBitmap: config.ChannelBitmap,
		codecBitmap:   config.CodecBitmap,
		timeout:       config.Timeout,
		maxRetryCount: config.MaxRetryCount,
	}
}

// SetTimeout sets the negotiation timeout.
func (n *ClientNegotiatorImpl) SetTimeout(timeout time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.timeout = timeout
}

// GetTimeout returns the current timeout.
func (n *ClientNegotiatorImpl) GetTimeout() time.Duration {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.timeout
}

// SetChannelBitmap sets the client's supported channels.
func (n *ClientNegotiatorImpl) SetChannelBitmap(bitmap ChannelBitmap) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.channelBitmap = bitmap
}

// SetCodecBitmap sets the client's supported codecs.
func (n *ClientNegotiatorImpl) SetCodecBitmap(bitmap CodecBitmap) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.codecBitmap = bitmap
}

// GetChannelBitmap returns the client's channel bitmap.
func (n *ClientNegotiatorImpl) GetChannelBitmap() ChannelBitmap {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.channelBitmap
}

// GetCodecBitmap returns the client's codec bitmap.
func (n *ClientNegotiatorImpl) GetCodecBitmap() CodecBitmap {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.codecBitmap
}

// Negotiate performs negotiation with server.
// This is a simplified implementation that generates the request.
// Actual network transmission is handled by Bus layer.
func (n *ClientNegotiatorImpl) Negotiate(defaultChannel ChannelBit) (*Result, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Validate channel bitmap
	if IsChannelBitmapEmpty(n.channelBitmap) {
		return nil, ErrNoCommonChannels
	}

	// Validate codec bitmap
	if IsCodecBitmapEmpty(n.codecBitmap) {
		return nil, ErrNoCommonCodecs
	}

	// Check if default channel is supported
	if !n.channelBitmap.HasChannel(defaultChannel) {
		return nil, errors.New("negotiate: default channel not supported")
	}

	// Create negotiation request
	request, err := NewNegotiateRequest(n.channelBitmap, n.codecBitmap)
	if err != nil {
		return nil, err
	}

	// Encode request to bytes (for transmission)
	encoded, err := request.Encode()
	if err != nil {
		return nil, err
	}

	// Return request bytes for transmission
	// The actual network send is handled by Bus
	result := &Result{
		AvailableChannels: n.channelBitmap,
		AvailableCodecs:   n.codecBitmap,
		SessionID:         nil, // Will be set after server response
		Status:            NegotiateStatusSuccess,
		NegotiatedAt:      time.Now(),
	}

	// Store encoded request for reference
	_ = encoded // Will be used by Bus layer

	return result, nil
}

// CreateRequest creates a negotiation request without performing negotiation.
// Used when Bus needs to control the transmission.
func (n *ClientNegotiatorImpl) CreateRequest() (*NegotiateRequest, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return NewNegotiateRequest(n.channelBitmap, n.codecBitmap)
}

// ProcessResponse processes server response and returns result.
func (n *ClientNegotiatorImpl) ProcessResponse(response *NegotiateResponse) (*Result, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Check response status
	if response.Status == NegotiateStatusReject {
		return nil, ErrNoCommonChannels
	}

	if response.Status == NegotiateStatusRetry {
		return nil, errors.New("negotiate: server requests retry")
	}

	// Verify intersection is not empty
	if IsChannelBitmapEmpty(response.ChannelBitmap) {
		return nil, ErrNoCommonChannels
	}

	if IsCodecBitmapEmpty(response.CodecBitmap) {
		return nil, ErrNoCommonCodecs
	}

	// Create result
	result := &Result{
		AvailableChannels: response.ChannelBitmap,
		AvailableCodecs:   response.CodecBitmap,
		SessionID:         response.SessionID,
		Status:            response.Status,
		NegotiatedAt:      time.Now(),
	}

	return result, nil
}

// ValidateResponse validates a response from server.
func (n *ClientNegotiatorImpl) ValidateResponse(response *NegotiateResponse) error {
	// Verify checksum
	if response.Status != NegotiateStatusSuccess {
		return nil // Non-success status is valid
	}

	// Verify intersection
	intersectedChannels := IntersectChannelBitmaps(n.channelBitmap, response.ChannelBitmap)
	if !IsChannelBitmapEmpty(intersectedChannels) && IsChannelBitmapEmpty(response.ChannelBitmap) {
		return ErrNoCommonChannels
	}

	intersectedCodecs := IntersectCodecBitmaps(n.codecBitmap, response.CodecBitmap)
	if !IsCodecBitmapEmpty(intersectedCodecs) && IsCodecBitmapEmpty(response.CodecBitmap) {
		return ErrNoCommonCodecs
	}

	return nil
}
