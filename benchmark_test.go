// Package voidbus provides performance benchmarks for VoidBus v2.0.
package voidbus

import (
	"testing"
	"time"

	"github.com/Script-OS/VoidBus/codec"
	"github.com/Script-OS/VoidBus/codec/aes"
	"github.com/Script-OS/VoidBus/codec/base64"
	"github.com/Script-OS/VoidBus/codec/plain"
	"github.com/Script-OS/VoidBus/fragment"
	"github.com/Script-OS/VoidBus/internal"
	"github.com/Script-OS/VoidBus/keyprovider/embedded"
	"github.com/Script-OS/VoidBus/protocol"
)

// ============================================================================
// Protocol Header Benchmarks
// ============================================================================

// BenchmarkHeaderEncode benchmarks Header.Encode operation
func BenchmarkHeaderEncode(b *testing.B) {
	header := &protocol.Header{
		SessionID:     "test-session-12345",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    2,
		CodecHash:     [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		DataChecksum:  12345,
		DataHash:      [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		Timestamp:     time.Now().Unix(),
		Flags:         protocol.FlagIsLast,
	}
	data := []byte("benchmark test data payload")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = header.Encode(data)
	}
}

// BenchmarkHeaderEncode_LargeSessionID benchmarks with longer session ID
func BenchmarkHeaderEncode_LargeSessionID(b *testing.B) {
	header := &protocol.Header{
		SessionID:     "session-id-with-max-length-64-characters-xxxxxxxxxxxx",
		FragmentIndex: 0,
		FragmentTotal: 100,
		CodecDepth:    3,
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix(),
		Flags:         0,
	}
	data := make([]byte, 512)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = header.Encode(data)
	}
}

// BenchmarkDecodeHeader benchmarks DecodeHeader operation with validation
func BenchmarkDecodeHeader(b *testing.B) {
	header := &protocol.Header{
		SessionID:     "test-session-12345",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    2,
		CodecHash:     [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		DataChecksum:  12345,
		DataHash:      [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		Timestamp:     time.Now().Unix(),
		Flags:         protocol.FlagIsLast,
	}
	data := []byte("benchmark test data payload")
	packet := header.Encode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = protocol.DecodeHeader(packet)
	}
}

// BenchmarkDecodeHeader_LargePacket benchmarks with larger packet
func BenchmarkDecodeHeader_LargePacket(b *testing.B) {
	header := &protocol.Header{
		SessionID:     "session-id-with-max-length-64-characters-xxxxxxxxxxxx",
		FragmentIndex: 50,
		FragmentTotal: 100,
		CodecDepth:    3,
		CodecHash:     [32]byte{},
		DataChecksum:  0,
		DataHash:      [32]byte{},
		Timestamp:     time.Now().Unix(),
		Flags:         0,
	}
	data := make([]byte, 512)
	packet := header.Encode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = protocol.DecodeHeader(packet)
	}
}

// BenchmarkHeaderEncodeDecode benchmarks full encode-decode cycle
func BenchmarkHeaderEncodeDecode(b *testing.B) {
	header := &protocol.Header{
		SessionID:     "test-session-12345",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    2,
		CodecHash:     [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		DataChecksum:  12345,
		DataHash:      [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		Timestamp:     time.Now().Unix(),
		Flags:         protocol.FlagIsLast,
	}
	data := []byte("benchmark test data payload")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packet := header.Encode(data)
		_, _, _ = protocol.DecodeHeader(packet)
	}
}

// ============================================================================
// Fragment Benchmarks
// ============================================================================

// BenchmarkAdaptiveSplit_Small benchmarks splitting small data (1KB)
func BenchmarkAdaptiveSplit_Small(b *testing.B) {
	config := fragment.DefaultFragmentConfig()
	mgr := fragment.NewFragmentManager(config)
	defer mgr.Stop()

	data := make([]byte, 1024) // 1KB
	mtu := 1024

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = mgr.AdaptiveSplit(data, mtu)
	}
}

// BenchmarkAdaptiveSplit_Medium benchmarks splitting medium data (64KB)
func BenchmarkAdaptiveSplit_Medium(b *testing.B) {
	config := fragment.DefaultFragmentConfig()
	mgr := fragment.NewFragmentManager(config)
	defer mgr.Stop()

	data := make([]byte, 64*1024) // 64KB
	mtu := 1024

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = mgr.AdaptiveSplit(data, mtu)
	}
}

// BenchmarkAdaptiveSplit_Large benchmarks splitting large data (1MB)
func BenchmarkAdaptiveSplit_Large(b *testing.B) {
	config := fragment.DefaultFragmentConfig()
	mgr := fragment.NewFragmentManager(config)
	defer mgr.Stop()

	data := make([]byte, 1024*1024) // 1MB
	mtu := 1024

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = mgr.AdaptiveSplit(data, mtu)
	}
}

// BenchmarkAdaptiveSplit_VeryLarge benchmarks splitting very large data (10MB)
func BenchmarkAdaptiveSplit_VeryLarge(b *testing.B) {
	config := fragment.DefaultFragmentConfig()
	mgr := fragment.NewFragmentManager(config)
	defer mgr.Stop()

	data := make([]byte, 10*1024*1024) // 10MB
	mtu := 1024

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = mgr.AdaptiveSplit(data, mtu)
	}
}

// BenchmarkReassemble benchmarks reassembling fragments
func BenchmarkReassemble(b *testing.B) {
	config := fragment.DefaultFragmentConfig()
	mgr := fragment.NewFragmentManager(config)
	defer mgr.Stop()

	data := make([]byte, 10*1024) // 10KB
	mtu := 1024
	fragments, checksums, _ := mgr.AdaptiveSplit(data, mtu)

	// Prepare receive buffer
	sessionID := "bench-session"
	total := uint16(len(fragments))
	dataHash := internal.ComputeDataHash(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create receive buffer
		buf := mgr.CreateRecvBuffer(sessionID, total, 1, [32]byte{}, dataHash)

		// Add all fragments
		for idx, frag := range fragments {
			buf.AddFragment(uint16(idx), frag, checksums[idx])
		}

		// Reassemble
		_, _ = buf.Reassemble()

		// Clean up
		mgr.RemoveRecvBuffer(sessionID)
	}
}

// ============================================================================
// Codec Chain Benchmarks
// ============================================================================

// BenchmarkCodecChain_Plain benchmarks plain codec (no transformation)
func BenchmarkCodecChain_Plain(b *testing.B) {
	chain := codec.NewChain()
	chain.AddCodec(plain.New())

	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoded, _ := chain.Encode(data)
		_, _ = chain.Decode(encoded)
	}
}

// BenchmarkCodecChain_Base64 benchmarks base64 codec
func BenchmarkCodecChain_Base64(b *testing.B) {
	chain := codec.NewChain()
	chain.AddCodec(base64.New())

	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoded, _ := chain.Encode(data)
		_, _ = chain.Decode(encoded)
	}
}

// BenchmarkCodecChain_AES benchmarks AES-256-GCM codec
func BenchmarkCodecChain_AES(b *testing.B) {
	chain := codec.NewChain()
	aesCodec := aes.NewAES256Codec()
	// Set key via KeyProvider
	key := []byte("32-byte-secret-key-for-aes-256!!")
	provider, _ := embedded.New(key, "", "AES-256-GCM")
	aesCodec.SetKeyProvider(provider)
	chain.AddCodec(aesCodec)

	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoded, _ := chain.Encode(data)
		_, _ = chain.Decode(encoded)
	}
}

// BenchmarkCodecChain_Mixed benchmarks mixed codec chain (Plain + Base64)
func BenchmarkCodecChain_Mixed(b *testing.B) {
	chain := codec.NewChain()
	chain.AddCodec(plain.New())
	chain.AddCodec(base64.New())

	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoded, _ := chain.Encode(data)
		_, _ = chain.Decode(encoded)
	}
}

// BenchmarkCodecChain_LargeData benchmarks codec chain with large data (64KB)
func BenchmarkCodecChain_LargeData(b *testing.B) {
	chain := codec.NewChain()
	chain.AddCodec(base64.New())

	data := make([]byte, 64*1024) // 64KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoded, _ := chain.Encode(data)
		_, _ = chain.Decode(encoded)
	}
}

// BenchmarkCodecChain_EncodeOnly benchmarks encode only
func BenchmarkCodecChain_EncodeOnly(b *testing.B) {
	chain := codec.NewChain()
	chain.AddCodec(base64.New())

	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chain.Encode(data)
	}
}

// BenchmarkCodecChain_DecodeOnly benchmarks decode only
func BenchmarkCodecChain_DecodeOnly(b *testing.B) {
	chain := codec.NewChain()
	chain.AddCodec(base64.New())

	data := make([]byte, 1024)
	encoded, _ := chain.Encode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chain.Decode(encoded)
	}
}

// ============================================================================
// Internal Hash Benchmarks
// ============================================================================

// BenchmarkComputeHash benchmarks hash computation for codec chain
func BenchmarkComputeHash(b *testing.B) {
	codeChain := []string{"A", "B", "C"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = internal.ComputeHash(codeChain)
	}
}

// BenchmarkComputeHashWithSalt benchmarks salted hash computation
func BenchmarkComputeHashWithSalt(b *testing.B) {
	codeChain := []string{"A", "B", "C"}
	salt := []byte("random-salt-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = internal.ComputeHashWithSalt(codeChain, salt)
	}
}

// BenchmarkComputeDataHash benchmarks data hash computation (SHA256)
func BenchmarkComputeDataHash_Small(b *testing.B) {
	data := make([]byte, 1024) // 1KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = internal.ComputeDataHash(data)
	}
}

// BenchmarkComputeDataHash_Medium benchmarks data hash (64KB)
func BenchmarkComputeDataHash_Medium(b *testing.B) {
	data := make([]byte, 64*1024) // 64KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = internal.ComputeDataHash(data)
	}
}

// BenchmarkComputeDataHash_Large benchmarks data hash (1MB)
func BenchmarkComputeDataHash_Large(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = internal.ComputeDataHash(data)
	}
}

// ============================================================================
// Checksum Benchmarks
// ============================================================================

// BenchmarkCalculateChecksum_Small benchmarks checksum for small data
func BenchmarkCalculateChecksum_Small(b *testing.B) {
	data := make([]byte, 1024) // 1KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = internal.CalculateChecksum(data)
	}
}

// BenchmarkCalculateChecksum_Medium benchmarks checksum for medium data
func BenchmarkCalculateChecksum_Medium(b *testing.B) {
	data := make([]byte, 64*1024) // 64KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = internal.CalculateChecksum(data)
	}
}

// BenchmarkCalculateChecksum_Large benchmarks checksum for large data
func BenchmarkCalculateChecksum_Large(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = internal.CalculateChecksum(data)
	}
}

// ============================================================================
// Parallel Benchmarks
// ============================================================================

// BenchmarkHeaderEncodeDecode_Parallel benchmarks parallel encode-decode
func BenchmarkHeaderEncodeDecode_Parallel(b *testing.B) {
	header := &protocol.Header{
		SessionID:     "test-session-12345",
		FragmentIndex: 0,
		FragmentTotal: 10,
		CodecDepth:    2,
		CodecHash:     [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		DataChecksum:  12345,
		DataHash:      [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		Timestamp:     time.Now().Unix(),
		Flags:         protocol.FlagIsLast,
	}
	data := []byte("benchmark test data payload")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			packet := header.Encode(data)
			_, _, _ = protocol.DecodeHeader(packet)
		}
	})
}

// BenchmarkAdaptiveSplit_Parallel benchmarks parallel splitting
func BenchmarkAdaptiveSplit_Parallel(b *testing.B) {
	config := fragment.DefaultFragmentConfig()
	mgr := fragment.NewFragmentManager(config)
	defer mgr.Stop()

	data := make([]byte, 1024)
	mtu := 1024

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = mgr.AdaptiveSplit(data, mtu)
		}
	})
}

// BenchmarkCodecChain_EncodeDecode_Parallel benchmarks parallel codec chain
func BenchmarkCodecChain_EncodeDecode_Parallel(b *testing.B) {
	chain := codec.NewChain()
	chain.AddCodec(base64.New())

	data := make([]byte, 1024)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			encoded, _ := chain.Encode(data)
			_, _ = chain.Decode(encoded)
		}
	})
}

// BenchmarkComputeHash_Parallel benchmarks parallel hash computation
func BenchmarkComputeHash_Parallel(b *testing.B) {
	codeChain := []string{"A", "B", "C"}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = internal.ComputeHash(codeChain)
		}
	})
}
