# VoidBus Non-Interactive Test Suite

Automated test suite for VoidBus that verifies multi-channel and multi-codec functionality without user interaction.

## Overview

This test suite covers:

- **Channels**: TCP, WebSocket (UDP, QUIC planned)
- **Codecs**: base64, xor, aes-256, chacha20 (RSA planned)
- **Codec Chains**: depth 1 (single), depth 2 (chained), depth 3 (max depth)
- **Test Flow**: Server setup -> Client dial -> Auto negotiation -> 3 bidirectional message rounds -> Cleanup

## Test Matrix

### Phase 1: Single Channel + Single Codec

| Test ID | Channel | Codec | Key Required |
|---------|---------|-------|:---:|
| T01 | TCP | base64 | No |
| T02 | TCP | xor | Yes |
| T03 | TCP | aes | Yes |
| T04 | TCP | chacha20 | Yes |
| T06 | WebSocket | base64 | No |
| T07 | WebSocket | xor | Yes |

### Phase 2: Codec Chain (depth=2)

| Test ID | Channel | Codec Chain | Key Required |
|---------|---------|-------------|:---:|
| T21 | TCP | base64 -> xor | Yes |
| T24 | WebSocket | base64 -> xor | Yes |

### Phase 3: Codec Chain (depth=3, max)

| Test ID | Channel | Codec Chain | Key Required |
|---------|---------|-------------|:---:|
| T31 | TCP | base64 -> xor -> aes | Yes |
| T32 | WebSocket | base64 -> xor -> chacha20 | Yes |

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
[PASS] T01-TCP-base64
[PASS] T02-TCP-xor
[PASS] T03-TCP-aes
[PASS] T04-TCP-chacha20
[PASS] T06-WS-base64
[PASS] T07-WS-xor
[PASS] T21-TCP-chain2-base64-xor
[PASS] T24-WS-chain2-base64-xor
[PASS] T31-TCP-chain3-base64-xor-aes
[PASS] T32-WS-chain3-base64-xor-chacha20

========================================
VoidBus Non-Interactive Test Report
========================================
Total Tests: 10
Passed: 10
Failed: 0
========================================
```

## Planned Tests (TODO)

- UDP channel tests (T11-T14, T26-T27, T35-T36)
- QUIC channel tests (T16-T19, T28-T29, T37-T38)
- RSA codec tests (T05, T10, T15, T20)
- Multi-channel negotiation tests (T41-T44)
