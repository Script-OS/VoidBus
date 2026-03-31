// Package voidbus provides configuration structures for VoidBus.
package voidbus

import "time"

// BusConfig is the main configuration for VoidBus v2.0.
type BusConfig struct {
	// === Codec 配置 ===
	MaxCodecDepth int // 最大Codec链深度，用户固定配置 (默认: 2)

	// === Channel 配置 ===
	DefaultMTU int // 默认MTU大小 (默认: 1024)
	MinMTU     int // 最小MTU限制 (默认: 64)
	MaxMTU     int // 最大MTU限制 (默认: 65535)

	// === Fragment 配置 ===
	FragmentTimeout     time.Duration // 分片重组超时 (默认: 30s)
	MaxPendingFragments int           // 最大待重组分片组数 (默认: 1000)

	// === Session 配置 ===
	SessionTimeout     time.Duration // Session超时时间 (默认: 60s)
	MaxRetransmit      int           // 最大重传次数 (默认: 3)
	RetransmitInterval time.Duration // 重传间隔 (默认: 5s)

	// === 接收配置 ===
	ReceiveMode    ReceiveMode // 接收模式: 阻塞/回调 (默认: 阻塞)
	RecvBufferSize int         // 接收缓冲区大小 (默认: 100)

	// === 连接配置 ===
	ConnectTimeout   time.Duration // 连接超时 (默认: 10s)
	NegotiateTimeout time.Duration // 协商超时 (默认: 30s)

	// === 调试配置 ===
	DebugMode bool // 调试模式，允许plaintext Codec
}

// ReceiveMode defines how data is received.
type ReceiveMode int

const (
	// ReceiveModeBlocking is the default mode - blocking Receive() call.
	ReceiveModeBlocking ReceiveMode = iota

	// ReceiveModeCallback uses callback OnMessage for async receiving.
	ReceiveModeCallback
)

// DefaultBusConfig returns the default configuration.
func DefaultBusConfig() *BusConfig {
	return &BusConfig{
		MaxCodecDepth:       2,
		DefaultMTU:          1024,
		MinMTU:              64,
		MaxMTU:              65535,
		FragmentTimeout:     30 * time.Second,
		MaxPendingFragments: 1000,
		SessionTimeout:      60 * time.Second,
		MaxRetransmit:       3,
		RetransmitInterval:  5 * time.Second,
		ReceiveMode:         ReceiveModeBlocking,
		RecvBufferSize:      100,
		ConnectTimeout:      10 * time.Second,
		NegotiateTimeout:    30 * time.Second,
		DebugMode:           false,
	}
}

// Validate validates the configuration.
func (c *BusConfig) Validate() error {
	if c.MaxCodecDepth < 1 {
		return ErrBusConfig
	}
	if c.MaxCodecDepth > 5 {
		return ErrBusConfig
	}
	if c.DefaultMTU < c.MinMTU || c.DefaultMTU > c.MaxMTU {
		return ErrBusConfig
	}
	if c.FragmentTimeout < 1*time.Second {
		return ErrBusConfig
	}
	if c.SessionTimeout < 1*time.Second {
		return ErrBusConfig
	}
	if c.MaxRetransmit < 0 {
		return ErrBusConfig
	}
	return nil
}

// ChannelMTUConfig allows user to override MTU for specific channels.
type ChannelMTUConfig struct {
	ChannelID string // Channel标识
	MTU       int    // 自定义MTU
}

// NegotiationConfig holds codec negotiation configuration.
type NegotiationConfig struct {
	// 双方协商后支持的Codec代号集合
	SupportedCodes []string

	// 协商后的最大链深度
	MaxDepth int

	// 协商时间
	NegotiatedAt time.Time
}

// Validate validates negotiation configuration.
func (c *NegotiationConfig) Validate() error {
	if len(c.SupportedCodes) == 0 {
		return ErrNoCommonCodec
	}
	if c.MaxDepth < 1 {
		return ErrBusConfig
	}
	return nil
}
