# VoidBus Non-Interactive Test Suite

Automated test suite for VoidBus that verifies multi-channel and multi-codec functionality without user interaction.

## Overview

This test suite covers:

- **Channels**: TCP, WebSocket, UDP, QUIC
- **Codecs**: base64, xor, aes-256-gcm, chacha20-poly1305, rsa-oaep-sha256 (plain excluded per security policy)
- **Codec Chains**: depth 1 (single), depth 2 (chained), depth 3 (max depth)
- **Test Flow**: Server setup -> Client dial -> Auto negotiation -> 3 bidirectional message rounds -> Cleanup

## Test Matrix

Total: **48 tests** (20 + 16 + 12)

### Phase 1: Single Codec Tests (depth=1)

**4 channels × 5 codecs = 20 tests**

| Channel | Codecs Tested |
|---------|---------------|
| TCP     | base64, xor, aes, chacha20, rsa |
| WebSocket | base64, xor, aes, chacha20, rsa |
| UDP     | base64, xor, aes, chacha20, rsa |
| QUIC    | base64, xor, aes, chacha20, rsa |

**Key Requirements:**
- base64: No key required
- xor, aes, chacha20: 32-byte key required
- rsa: RSA key pair generated internally (2048-bit)

### Phase 2: Dual Codec Chain Tests (depth=2)

**4 channels × 4 chain combinations = 16 tests**

| Chain Combination | Key Used |
|-------------------|----------|
| base64 → xor      | xor key (32 bytes) |
| xor → aes         | aes key (32 bytes) |
| aes → chacha20    | chacha20 key (32 bytes) |
| base64 → aes      | aes key (32 bytes) |

**Note:** RSA codec is excluded from codec chains due to message size limitations (~190 bytes max).

### Phase 3: Triple Codec Chain Tests (depth=3)

**4 channels × 3 chain combinations = 12 tests**

| Chain Combination | Key Used |
|-------------------|----------|
| base64 → xor → aes | aes key (32 bytes) |
| xor → aes → chacha20 | chacha20 key (32 bytes) |
| base64 → aes → chacha20 | chacha20 key (32 bytes) |

## Quick Start

```bash
cd example/noninteractive
go run main.go
```

## Test Flow (per test case)

```
1. Server: Create Bus + Register Codecs + Create ServerChannel + Listen
2. Client: Create Bus + Register Codecs + Create ClientChannel + Dial
3. Auto Negotiation: Bitmap exchange (codec + channel matching)
4. Message Rounds (x3):
   Client -> "Client-RoundN" -> Server (verify)
   Server -> "Server-Reply-RoundN" -> Client (verify)
5. Cleanup: Close connections -> Close listener -> Stop buses
```

## Key Design Decisions

### Test Keys
All encryption codecs use fixed 32-byte test keys created with `make([]byte, 32)` + `copy()` to ensure exact length compliance.

### RSA Codec Handling
- RSA key pair generated once and shared across all RSA tests (lazy initialization)
- RSA tests use small messages (< 50 bytes) to fit within RSA encryption limit (~190 bytes)
- RSA excluded from codec chains to avoid message size issues

### QUIC Channel Handling
- Server uses self-signed certificate generated internally
- Client uses InsecureSkipVerify TLS config (testing only)
- NextProtos set to ["voidbus"]

### UDP Channel Handling
- UDP channel has built-in ACK/NAK mechanism for reliability
- DefaultAckTimeout: 3 seconds
- MaxRetries: 3

### Codec Chain Constraint
No duplicate codec types in a single chain (CodecManager rejects duplicate code registrations).

### Cleanup Order
```
conn.Close()           -> triggers bus.Stop() -> channelPool.CloseAll()
serverListener.Close() -> closes serverCh -> unblocks acceptLoop
serverBus.Stop()       -> (already stopped via listener)
clientBus.Stop()       -> (already stopped via conn)
```

### Port Allocation
Tests use sequential ports starting from 9000, one per test, to avoid conflicts.

## Bugs Discovered

This test suite uncovered 6 bugs in VoidBus core:

| Bug | Severity | Component | Description | Status |
|-----|----------|-----------|-------------|--------|
| #1 | Critical | bus.go | Cleanup deadlock: `bus.Stop()` waits for `receiveLoop` which blocks on `Receive()` forever | Fixed |
| #2 | Medium | test config | AES key length error (30 bytes instead of 32) | Fixed |
| #3 | Critical | channel/ws | WebSocket `ServerChannel.Accept()` required HTTP handler integration | Fixed |
| #4 | Low | test config | Codec registration conflict with duplicate types in chain | Fixed |
| #5 | Critical | bus.go, listener.go | Root cause fix for cleanup deadlock: `Stop()` now closes all channels before `wg.Wait()`; `listener.Close()` releases lock before waiting | Fixed |
| #6 | Critical | channel/ws | `Accept()` returns `(nil, nil)` on closed channels causing nil pointer panic | Fixed |

## Build

```bash
cd example/noninteractive
go build -o voidbus-test
./voidbus-test
```

## Output Example

```
Generated 48 tests: 20 (phase1) + 16 (phase2) + 12 (phase3)

[PASS] P1-T01-tcp-base64
[PASS] P1-T02-tcp-xor
[PASS] P1-T03-tcp-aes
[PASS] P1-T04-tcp-chacha20
[PASS] P1-T05-tcp-rsa
...
[PASS] P2-T01-tcp-chain2-base64-xor
[PASS] P2-T02-tcp-chain2-xor-aes
...
[PASS] P3-T01-tcp-chain3-base64-xor-aes
[PASS] P3-T02-tcp-chain3-xor-aes-chacha20
...

========================================
VoidBus Non-Interactive Test Report
========================================
Total Tests: 48
Passed: 48
Failed: 0
========================================
```

## Test Naming Convention

- Phase 1: `P1-T{ID}-{channel}-{codec}`
- Phase 2: `P2-T{ID}-{channel}-chain2-{codec1}-{codec2}`
- Phase 3: `P3-T{ID}-{channel}-chain3-{codec1}-{codec2}-{codec3}`

Example:
- `P1-T01-tcp-base64` - Phase 1, Test 01, TCP channel, base64 codec
- `P2-T16-quic-chain2-aes-chacha20` - Phase 2, Test 16, QUIC channel, aes→chacha20 chain
- `P3-T12-quic-chain3-base64-aes-chacha20` - Phase 3, Test 12, QUIC channel, triple chain