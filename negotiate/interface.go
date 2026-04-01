// Package negotiate provides Negotiator interfaces for VoidBus.
//
// Negotiator handles initial handshake between client and server
// to exchange supported channels and codecs using bitmap protocol.
package negotiate

import (
	"time"
)

// Negotiator defines the negotiation interface.
// Both ClientNegotiator and ServerNegotiator implement this.
type Negotiator interface {
	// SetTimeout sets the negotiation timeout.
	SetTimeout(timeout time.Duration)

	// GetTimeout returns the current timeout.
	GetTimeout() time.Duration
}

// ClientNegotiator handles client-side negotiation.
type ClientNegotiator interface {
	Negotiator

	// Negotiate performs negotiation with server.
	// Returns negotiation result including available channels and codecs.
	// defaultChannel is the channel used for negotiation (WebSocket by default).
	Negotiate(defaultChannel ChannelBit) (*Result, error)

	// SetChannelBitmap sets the client's supported channels.
	SetChannelBitmap(bitmap ChannelBitmap)

	// SetCodecBitmap sets the client's supported codecs.
	SetCodecBitmap(bitmap CodecBitmap)

	// GetChannelBitmap returns the client's channel bitmap.
	GetChannelBitmap() ChannelBitmap

	// GetCodecBitmap returns the client's codec bitmap.
	GetCodecBitmap() CodecBitmap
}

// ServerNegotiator handles server-side negotiation.
type ServerNegotiator interface {
	Negotiator

	// HandleRequest processes a negotiation request from client.
	// Returns negotiation response with available channels and codecs.
	HandleRequest(request *NegotiateRequest) (*NegotiateResponse, error)

	// SetChannelBitmap sets the server's supported channels.
	SetChannelBitmap(bitmap ChannelBitmap)

	// SetCodecBitmap sets the server's supported codecs.
	SetCodecBitmap(bitmap CodecBitmap)

	// GetChannelBitmap returns the server's channel bitmap.
	GetChannelBitmap() ChannelBitmap

	// GetCodecBitmap returns the server's codec bitmap.
	GetCodecBitmap() CodecBitmap
}

// Result represents the negotiation result.
type Result struct {
	// AvailableChannels is the intersection of client and server channel bitmaps.
	AvailableChannels ChannelBitmap

	// AvailableCodecs is the intersection of client and server codec bitmaps.
	AvailableCodecs CodecBitmap

	// SessionID is the server-generated session identifier (8 bytes).
	SessionID []byte

	// Status indicates the negotiation result status.
	Status byte

	// NegotiatedAt is the timestamp when negotiation completed.
	NegotiatedAt time.Time
}

// IsSuccess checks if negotiation was successful.
func (r *Result) IsSuccess() bool {
	return r.Status == NegotiateStatusSuccess
}

// HasCommonChannels checks if there are common channels available.
func (r *Result) HasCommonChannels() bool {
	return !IsChannelBitmapEmpty(r.AvailableChannels)
}

// HasCommonCodecs checks if there are common codecs available.
func (r *Result) HasCommonCodecs() bool {
	return !IsCodecBitmapEmpty(r.AvailableCodecs)
}

// GetAvailableChannelIDs returns IDs of available channels.
func (r *Result) GetAvailableChannelIDs() []ChannelID {
	return r.AvailableChannels.GetChannelIDs()
}

// GetAvailableCodecIDs returns IDs of available codecs.
func (r *Result) GetAvailableCodecIDs() []CodecID {
	return r.AvailableCodecs.GetCodecIDs()
}

// NegotiatorConfig provides configuration for negotiator.
type NegotiatorConfig struct {
	// Timeout is the negotiation timeout duration.
	Timeout time.Duration

	// ChannelBitmap is the supported channels bitmap.
	ChannelBitmap ChannelBitmap

	// CodecBitmap is the supported codecs bitmap.
	CodecBitmap CodecBitmap

	// MaxRetryCount is the maximum retry count for negotiation.
	MaxRetryCount int
}

// DefaultNegotiatorConfig returns the default negotiator configuration.
func DefaultNegotiatorConfig() *NegotiatorConfig {
	return &NegotiatorConfig{
		Timeout:       NegotiateDefaultTimeout,
		ChannelBitmap: NewChannelBitmap(0), // Uses default size
		CodecBitmap:   NewCodecBitmap(0),   // Uses default size
		MaxRetryCount: 3,
	}
}
