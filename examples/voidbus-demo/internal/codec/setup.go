package democodec

import (
	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/codec/aes"
	"github.com/Script-OS/VoidBus/codec/base64"
	"github.com/Script-OS/VoidBus/keyprovider/embedded"
)

// Pre-defined shared keys for demo purposes
// In production, these should be negotiated via protocol.Negotiator
// or derived from a key exchange protocol (e.g., ECDH)
var (
	// Shared 128-bit key for AES-128-GCM (16 bytes)
	// This is a fixed key for demo purposes only!
	sharedKey128 = []byte{
		0x2b, 0x7e, 0x15, 0x16, 0x28, 0xae, 0xd2, 0xa6,
		0xab, 0xf7, 0x15, 0x88, 0x09, 0xcf, 0x4f, 0x3c,
	}
	// Shared 256-bit key for AES-256-GCM (32 bytes)
	// This is a fixed key for demo purposes only!
	sharedKey256 = []byte{
		0x60, 0x3d, 0xeb, 0x10, 0x15, 0xca, 0x71, 0xbe,
		0x2b, 0x73, 0xae, 0xf0, 0x85, 0x7d, 0x77, 0x81,
		0x1f, 0x35, 0x2c, 0x07, 0x3b, 0x61, 0x08, 0xd7,
		0x2d, 0x98, 0x10, 0xa3, 0x09, 0x14, 0xdf, 0xf4,
	}
)

// BuildCodecChainPool creates a pool of codec chains with different security levels
// Returns chains: Base64+AES-128-GCM (Medium) and Base64+AES-256-GCM (High)
func BuildCodecChainPool() ([]*codec.DefaultChain, error) {
	// Use pre-defined shared keys for demo
	// Both client and server will use the same keys

	// Create fixed key providers with shared keys
	keyProvider128, err := embedded.New(sharedKey128, "aes-128-key", "AES-128-GCM")
	if err != nil {
		return nil, err
	}

	keyProvider256, err := embedded.New(sharedKey256, "aes-256-key", "AES-256-GCM")
	if err != nil {
		return nil, err
	}

	// Create AES codecs
	aes128 := aes.NewAES128Codec()
	if err := aes128.SetKeyProvider(keyProvider128); err != nil {
		return nil, err
	}

	aes256 := aes.NewAES256Codec()
	if err := aes256.SetKeyProvider(keyProvider256); err != nil {
		return nil, err
	}

	// Create Base64 codec
	b64 := base64.New()

	// Build chain 1: Base64 + AES-128-GCM (Medium security)
	chain1 := codec.NewChain()
	chain1.AddCodec(b64)
	chain1.AddCodec(aes128)

	// Build chain 2: Base64 + AES-256-GCM (High security)
	chain2 := codec.NewChain()
	chain2.AddCodec(b64)
	chain2.AddCodec(aes256)

	return []*codec.DefaultChain{chain1, chain2}, nil
}

// GetAvailableCodecs returns a list of available codecs for negotiation
func GetAvailableCodecs() []codec.Codec {
	return []codec.Codec{
		base64.New(),
		aes.NewAES128Codec(),
		aes.NewAES256Codec(),
	}
}
