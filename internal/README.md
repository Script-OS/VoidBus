# Internal Package

This package contains internal utilities for VoidBus. These utilities are NOT exposed to external packages.

## Files

```
internal/
├── hash.go           # Hash计算 + HashCache
├── id.go             # ID生成
├── checksum.go       # CRC32校验
├── crypto.go         # Challenge验证
├── adaptive.go       # AdaptiveTimeout（自适应超时）
├── hash_test.go      # Hash测试
├── id_test.go        # ID生成测试
├── checksum_test.go  # Checksum测试
└── crypto_test.go    # Crypto测试
```

### hash.go
Hash computation utilities:
- `ComputeHash(codes []string) [32]byte`: Compute SHA256 hash for codec codes
- `ComputeHashWithSalt(codes []string, salt []byte) [32]byte`: Compute hash with salt
- `HashCache`: Thread-safe hash cache for performance optimization

### id.go
ID generation utilities:
- `GenerateID()`: Generate random UUID v4
- `GenerateSessionID()`: Generate session UUID (format: "session-{timestamp}-{random}")
- `GenerateClientID()`: Generate client UUID
- `GenerateChallenge()`: Generate challenge bytes for handshake

### checksum.go
Checksum calculation:
- `CalculateChecksum(data []byte) uint32`: CRC32 checksum
- `ComputeDataHash(data []byte) [32]byte`: SHA256 hash for data integrity

### crypto.go
Cryptographic utilities for handshake:
- `ChallengeVerifier`: Interface for challenge verification
- `SimpleChallengeHandler`: Simple challenge handler implementation

### adaptive.go
Adaptive timeout management:
- `AdaptiveTimeout`: RTT-based adaptive timeout
- `GetTimeout()`: Get current timeout value
- `RecordLatency()`: Record latency for RTT calculation
- `Reset()`: Reset timeout state

## Usage Rules

1. Internal package is for VoidBus internal use only
2. All functions must have no external dependencies beyond Go standard library
3. Each utility should have a single, clear responsibility
4. New utilities must be documented in this README

## Performance Benchmarks

| Benchmark | Result |
|-----------|--------|
| BenchmarkComputeHash | ~69 ns/op |
| BenchmarkComputeHashWithSalt | ~84 ns/op |
| BenchmarkCalculateChecksum_Small | ~10 ns/op |
| BenchmarkCalculateChecksum_Medium | ~100 ns/op |
| BenchmarkCalculateChecksum_Large | ~1000 ns/op |