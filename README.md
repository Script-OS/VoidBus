# VoidBus

VoidBus is a highly modular and composable covert communication bus library that achieves complete separation of channels and encoding/decoding, supporting arbitrary combinations and replacements.

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────┐
│                   User Application Layer                 │
│            (User serialization/deserialization)          │
└─────────────────────────────┬────────────────────────────┘
                              │
                              │ Raw Data ([]byte)
                              │
┌─────────────────────────────▼───────────────────────────┐
│                   Bus (Unified Entry Point)             │
│                                                         │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────┐     │
│  │CodecManager │  │ ChannelPool  │  │FragmentMgr  │     │
│  │- Register   │  │- Manage Ch   │  │- Split Data │     │
│  │- Chain      │  │- Health Check│  │- Reassemble │     │
│  │- Match      │  │- Load Balance│  │- ACK/NAK    │     │
│  └─────────────┘  └──────────────┘  └─────────────┘     │
│                                                         │
│  ┌─────────────┐                                        │
│  │ SessionMgr  │                                        │
│  │- Manage     │                                        │
│  │- Track      │                                        │
│  │- Cleanup    │                                        │
│  └─────────────┘                                        │
└─────────────┬──────────────────────────┬────────────────┘
              │                          │
              │ Codec Encode/Decode      │ Fragment Transfer
              │                          │
┌─────────────▼──────────────────────────▼─────────────────┐
│                   Protocol Layer                         │
│         Header + Security + Bitmap Negotiation           │
└─────────────┬──────────────────────────┬─────────────────┘
              │                          │
              │ Negotiate Channel        │ Data Channel
              │ (WebSocket)              │ (TCP/UDP/WS)
              │                          │
┌─────────────▼──────────────────────────▼────────────────┐
│                   Channel Layer                         │
│                                                         │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐                     │
│  │TCP │ │ WS │ │UDP │ │ICMP│ │DNS │                     │
│  │Rel.│ │Rel.│ │Unre│ │Unre│ │Unre│                     │
│  └────┘ └────┘ └────┘ └────┘ └────┘                     │
│                                                         │
└───────────────────────────────┬─────────────────────────┘
                                │
                                │ Network Transport
                                │
┌───────────────────────────────▼─────────────────────────┐
│                      Remote Peer                        │
└─────────────────────────────────────────────────────────┘

Send Flow:    Raw Data → Codec Encode → Fragment → Multi-Channel → Network
Receive Flow: Network → Reassemble → Codec Decode → Raw Data
Negotiation:  WebSocket → Bitmap Exchange → Channel/Codec Match
```

## Quick Start

### Client Example

```go
package main

import (
    "fmt"
    "time"
    
    voidbus "github.com/Script-OS/VoidBus"
    "github.com/Script-OS/VoidBus/channel"
    "github.com/Script-OS/VoidBus/channel/ws"
    "github.com/Script-OS/VoidBus/codec/aes"
    "github.com/Script-OS/VoidBus/codec/base64"
)

func main() {
    // 1. Create Bus instance
    bus, err := voidbus.New(nil)
    if err != nil {
        panic(err)
    }
    defer bus.Stop()

    // 2. Set encryption key (AES-256 requires 32-byte key)
    key := []byte("32-byte-secret-key-for-aes-256!!")
    if err := bus.SetKey(key); err != nil {
        panic(err)
    }

    // 3. Register Codec (user-defined code, must match on both ends)
    bus.RegisterCodec(aes.NewAES256Codec())    // code: "aes"
    bus.RegisterCodec(base64.New())            // code: "base64"

    // 4. Add channel
    bus.AddChannel(ws.NewClientChannel(channel.ChannelConfig{
        Address:        "ws://localhost:8080/ws",
        ConnectTimeout: 10 * time.Second,
    }))

    // 5. Establish connection (auto negotiation)
    conn, err := bus.Dial()
    if err != nil {
        panic(err)
    }
    defer conn.Close()

    // 6. Send message
    message := []byte("Hello, VoidBus!")
    if _, err := conn.Write(message); err != nil {
        panic(err)
    }
    fmt.Printf("Sent: %s\n", message)

    // 7. Receive message
    buf := make([]byte, 4096)
    n, err := conn.Read(buf)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Received: %s\n", buf[:n])
}
```

### Server Example

```go
package main

import (
    "fmt"
    "io"
    "net"
    
    voidbus "github.com/Script-OS/VoidBus"
    "github.com/Script-OS/VoidBus/channel"
    "github.com/Script-OS/VoidBus/channel/ws"
    "github.com/Script-OS/VoidBus/codec/aes"
    "github.com/Script-OS/VoidBus/codec/base64"
)

func main() {
    // 1. Create Bus instance
    bus, err := voidbus.New(nil)
    if err != nil {
        panic(err)
    }
    defer bus.Stop()

    // 2. Set encryption key (must match client)
    key := []byte("32-byte-secret-key-for-aes-256!!")
    if err := bus.SetKey(key); err != nil {
        panic(err)
    }

    // 3. Register Codec (must match client)
    bus.RegisterCodec(aes.NewAES256Codec())
    bus.RegisterCodec(base64.New())

    // 4. Add server channel
    bus.AddChannel(ws.NewServerChannel(channel.ChannelConfig{
        Address: ":8080",
    }))

    // 5. Start listening (aggregates all channels)
    listener, err := bus.Listen()
    if err != nil {
        panic(err)
    }
    defer listener.Close()

    fmt.Println("Server listening on :8080")

    // 6. Accept connections
    for {
        conn, err := listener.Accept()
        if err != nil {
            fmt.Printf("Accept error: %v\n", err)
            continue
        }

        go handleConnection(conn)
    }
}

func handleConnection(conn net.Conn) {
    defer conn.Close()

    buf := make([]byte, 4096)
    for {
        // Receive message
        n, err := conn.Read(buf)
        if err != nil {
            if err == io.EOF {
                fmt.Println("Client disconnected")
            } else {
                fmt.Printf("Read error: %v\n", err)
            }
            return
        }

        fmt.Printf("Received: %s\n", buf[:n])

        // Echo message back
        if _, err := conn.Write(buf[:n]); err != nil {
            fmt.Printf("Write error: %v\n", err)
            return
        }
    }
}
```

### Run Examples

```bash
# Start server
go run server.go

# In another terminal, start client
go run client.go
```

## Core Features

- **Three-Layer Separation Architecture**: Codec (encoding/decoding) + Channel (communication) + Fragment (splitting) - user handles serialization
- **Codec Chain Composition**: Supports multiple Codec combinations in sequence, with user-defined code identifiers
- **Pluggable Architecture**: All modules defined through interfaces, supporting custom implementations
- **Bidirectional Full-Duplex Communication**: Server can simultaneously receive and send information to multiple clients
- **Fragmented Multi-Channel Transmission**: Supports data fragmentation, sending through different channel/encoding combinations
- **Covert Channel Design**: Supports WebSocket (default), TCP, UDP, and other channels
- **Bitmap Negotiation Protocol**: Binary format negotiation for available channels and codecs (non-plaintext)
- **Channel Health Assessment**: Health-weighted random channel selection, automatic failover on failure
- **Reliable/Unreliable Channel Differentiation**: Reliable channels trust protocol reliability, unreliable channels implement ACK/NAK retransmission

## Directory Structure

```
VoidBus/
├── bus.go              # Bus core implementation (unified entry point)
├── module.go           # Module interface definition
├── config.go           # BusConfig configuration
├── errors.go           # Unified error definitions
│
├── negotiate/          # Negotiation module [core covert channel]
│   ├── interface.go    # Negotiator interface definition
│   ├── frame.go        # NegotiateRequest/Response frame encoding/decoding
│   ├── codec_bitmap.go # Codec Bitmap definition
│   ├── channel_bitmap.go # Channel Bitmap definition
│   ├── client_negotiator.go # Client negotiator
│   ├── server_negotiator.go # Server negotiator + SessionManager
│   └ negotiate_test.go # Negotiation module tests
│
├── protocol/           # Protocol layer
│   ├── header.go       # Header structure + security validation
│   └── header_test.go  # Header security validation tests
│
├── codec/              # Encoding/decoding module [not exposed]
│   ├── interface.go    # Codec interface definition + Code() method
│   ├── manager.go      # CodecManager (user-defined codes)
│   ├── chain.go        # CodecChain implementation
│   ├── chain_test.go   # CodecChain tests
│   ├── plain/          # Pass-through (debug only)
│   ├── base64/         # Base64 encoding
│   ├── aes/            # AES-GCM encryption
│   ├── xor/            # XOR encoding
│   ├── chacha20/       # ChaCha20-Poly1305 encryption
│   └── rsa/            # RSA-OAEP encryption
│
├── channel/            # Channel module [not exposed]
│   ├── interface.go    # Channel interface definition + IsReliable()
│   ├── pool.go         # ChannelPool (health-weighted random selection)
│   ├── tcp/            # TCP transmission (reliable)
│   ├── ws/             # WebSocket transmission (reliable, default negotiation channel)
│   └── udp/            # UDP transmission (unreliable, ACK/NAK retransmission)
│
├── fragment/           # Fragment module
│   ├── manager.go      # FragmentManager
│   └── buffer.go       # SendBuffer/RecvBuffer
│
├── session/            # Session module
│   ├── manager.go      # SessionManager
│   └── session.go      # Session definition
│
├── keyprovider/        # Key provider [not exposed]
│   └── embedded/       # Compile-time embedded key
│
├── internal/           # Internal utilities (not exposed externally)
│   ├── hash.go         # Hash computation + HashCache
│   ├── id.go           # ID generation + RandomIntRange
│   ├── checksum.go     # CRC16/CRC32 checksum
│   └ *_test.go         # Internal utility tests
│
├── tests/              # Test archive directory
│   ├── mock/           # Mock implementations (dependency injection testing)
│   │   └ mocks.go      # MockCodecManager/MockFragmentManager etc.
│   └ README.md         # Test documentation
│
├── docs/               # Documentation
│   ├── ARCHITECTURE.md # Architecture design document
│   └ INTERFACE.md      # Interface detailed specification
│
├── bus_test.go         # Bus core tests
├── errors_test.go      # Error handling tests
├── benchmark_test.go   # Performance benchmarks (19 benchmarks)
└── README.md           # Project documentation
```

## Security Boundaries

| Module | Exposure | Description |
|--------|----------|-------------|
| Codec | ❌ Not exposed | Encoding/decoding method not exposed, referenced indirectly via CodecHash |
| Channel | ❌ Not exposed | Channel type not exposed |
| KeyProvider | ❌ Not exposed | Key-related information not exposed |
| Codec Hash | ✅ Exposed | SHA256(code combination), doesn't expose specific combination |

## Data Flow

### Negotiation Flow
```
Client sends NegotiateRequest via default channel (WebSocket)
  → Server calculates intersection (Channel Bitmap & Codec Bitmap)
  → Server returns NegotiateResponse (available channels + Codec + SessionID)
  → Both sides dynamically compose Codec chain based on Bitmap
```

### Send Flow
```
Raw data (user serialization)
  → CodecManager.SelectChain() → Randomly select Codec combination (user-defined codes)
  → CodecChain.Encode() → Encode/encrypt data
  → FragmentManager.AdaptiveSplit() → Split data (adaptive MTU)
  → ChannelPool.SelectChannel() → Health-weighted random selection
  → Channel.Send() → Network transmission
    ├─ Reliable channel (TCP/WS): Trust protocol reliability
    └─ Unreliable channel (UDP): ACK/NAK retransmission mechanism
```

### Receive Flow
```
Channel.Receive() → Raw network data
  → DecodeHeader() → Security validation + Header parsing
  → CodecManager.MatchChain(Hash) → Match Codec combination
  → FragmentManager.AddFragment() → Fragment caching
  → FragmentManager.Reassemble() → Complete data
  → CodecChain.Decode() → Decode data
  → User deserialization → Raw data
```

### Failover Flow
```
Channel.Send() timeout 3s without ACK
  → ChannelPool.MarkUnavailable(chType)
  → FragmentManager.GetPendingFragments()
  → ChannelPool.SelectChannel(exclude=[unavailable])
  → New channel resend
```

## Negotiation Protocol

VoidBus uses binary Bitmap format for negotiation (non-plaintext):

### NegotiateRequest Frame Format
```
[1 byte: Magic 0x56] [1 byte: Version]
[1 byte: ChCount] [N bytes: ChannelBitmap]
[1 byte: CodecCount] [N bytes: CodecBitmap]
[8 bytes: Nonce] [4 bytes: Timestamp]
[1 byte: PaddingLen] [M bytes: Padding]
[2 bytes: CRC16]
```

### NegotiateResponse Frame Format
```
[1 byte: Magic 0x42] [1 byte: Version]
[1 byte: ChCount] [N bytes: ChannelBitmap]
[1 byte: CodecCount] [N bytes: CodecBitmap]
[8 bytes: SessionID] [1 byte: Status]
[1 byte: PaddingLen] [M bytes: Padding]
[2 bytes: CRC16]
```

### Channel Reliability
| Channel | IsReliable | Description |
|---------|------------|-------------|
| WebSocket | ✅ | Default negotiation channel, firewall-friendly |
| TCP | ✅ | Reliable transmission |
| UDP | ❌ | Requires ACK/NAK retransmission (3s timeout) |
| ICMP | ❌ | Requires reliable retransmission |
| DNS | ❌ | Requires reliable retransmission |

## Quick Start

### Basic Usage

```go
import (
    voidbus "github.com/Script-OS/VoidBus"
    "github.com/Script-OS/VoidBus/channel"
    "github.com/Script-OS/VoidBus/channel/tcp"
    "github.com/Script-OS/VoidBus/channel/ws"
    "github.com/Script-OS/VoidBus/codec/aes"
    "github.com/Script-OS/VoidBus/codec/base64"
)

func main() {
    // 1. Create Bus
    bus, err := voidbus.New(nil)
    if err != nil {
        panic(err)
    }
    defer bus.Stop()

    // 2. Set key
    key := []byte("32-byte-secret-key-for-aes-256!!")
    if err := bus.SetKey(key); err != nil {
        panic(err)
    }

    // 3. Register Codec (user-defined code, must match on both ends)
    bus.RegisterCodec(aes.NewAES256Codec())   // Automatically uses codec.Code() = "aes"
    bus.RegisterCodec(base64.New())           // Automatically uses codec.Code() = "base64"

    // 4. Add Channel - supports multiple channels simultaneously
    bus.AddChannel(ws.NewClientChannel(channel.ChannelConfig{
        Address:        "ws://localhost:8080/ws",
        ConnectTimeout: 10 * time.Second,
    }))
    bus.AddChannel(tcp.NewClientChannel(channel.ChannelConfig{
        Address:        "localhost:8080",
        ConnectTimeout: 10 * time.Second,
    }))

    // 5. Dial - auto negotiation, uses all registered channels
    conn, err := bus.Dial()
    if err != nil {
        panic(err)
    }
    defer conn.Close()

    // 6. Send data (message semantics)
    data := []byte("Hello, VoidBus!")
    if _, err := conn.Write(data); err != nil {
        panic(err)
    }

    // 7. Receive data (returns complete message)
    buf := make([]byte, 4096)
    n, err := conn.Read(buf)
    if err != nil {
        panic(err)
    }
    fmt.Println("Received:", string(buf[:n]))
}
```

### Server Side

```go
import (
    voidbus "github.com/Script-OS/VoidBus"
    "github.com/Script-OS/VoidBus/channel"
    "github.com/Script-OS/VoidBus/channel/tcp"
    "github.com/Script-OS/VoidBus/channel/ws"
    "github.com/Script-OS/VoidBus/channel/udp"
)

func main() {
    bus, _ := voidbus.New(nil)
    bus.SetKey([]byte("32-byte-secret-key-for-aes-256!!"))

    // Register Codec
    bus.RegisterCodec(aes.NewAES256Codec())
    bus.RegisterCodec(base64.New())

    // Add all Server Channels - Listener aggregates them
    bus.AddChannel(tcp.NewServerChannel(channel.ChannelConfig{Address: ":8080"}))
    bus.AddChannel(ws.NewServerChannel(channel.ChannelConfig{Address: ":8081"}))
    bus.AddChannel(udp.NewServerChannel(channel.ChannelConfig{Address: ":8082"}))

    // Listen - aggregates all channels, supports multi-channel Session
    listener, _ := bus.Listen()
    defer listener.Close()

    // Accept loop - each connection is associated with all channels
    for {
        conn, _ := listener.Accept()
        go handleClient(conn)
    }
}
```

### Automatic Bitmap Generation

During negotiation, Bitmap is **automatically** generated from registered Codec and Channel:

```go
// After registering Codec, CodecBitmap automatically includes corresponding bit
bus.RegisterCodec(aes.NewAES256Codec())  // Automatically sets CodecBitAES256
bus.RegisterCodec(base64.New())          // Automatically sets CodecBitBase64

// After adding Channel, ChannelBitmap automatically includes corresponding bit
bus.AddChannel(ws.NewClientChannel(...))  // Automatically sets ChannelBitWS
bus.AddChannel(tcp.NewClientChannel(...)) // Automatically sets ChannelBitTCP
bus.AddChannel(udp.NewClientChannel(...)) // Automatically sets ChannelBitUDP

// Auto negotiation during Dial/Listen, no manual request creation needed
conn, _ := bus.Dial()                     // Automatically sends NegotiateRequest
listener, _ := bus.Listen()               // Automatically receives and handles NegotiateRequest
```

### Multi-Channel Distribution Principle

VoidBus supports **using multiple channels simultaneously**, with random fragment distribution:

1. **Client Dial**: Negotiates via first channel, gets SessionID, subsequent channels asynchronously negotiate and associate
2. **Server Accept**: First channel connection returns immediately, subsequent channels dynamically added to Session
3. **Fragment Sending**: Each fragment independently calls ChannelPool.SelectChannel(), health-weighted random selection
4. **Fragment Receiving**: All channel receive loops aggregate to the same recvQueue

See [example/README.md](example/README.md) for details.

## Security Levels

| Level | Value | Examples |
|-------|-------|----------|
| None | 0 | Plain Codec (debug mode only) |
| Low | 1 | XOR, Base64 encoding |
| Medium | 2 | AES-128-GCM, ChaCha20 |
| High | 3 | AES-256-GCM, RSA |

**Release Mode**: Minimum security level is Medium, Plain Codec prohibited.

## Test Coverage

| Module | Coverage | Description |
|--------|----------|-------------|
| bus.go | 32.5% | Core entry tests |
| protocol/header.go | 89.3% | Security validation tests |
| negotiate | 79.5% | Negotiation protocol tests (64 test cases) |
| errors.go | High | Error handling tests |
| codec/aes | 81.7% | AES encoding/decoding tests |
| codec/base64 | 95.2% | Base64 encoding/decoding tests |
| codec/plain | 94.7% | Plain encoding/decoding tests |
| channel/ws | High | WebSocket channel tests |
| channel/udp | High | UDP reliable retransmission tests |

## Module Documentation

- [example/](example/README.md) - Interactive examples (multi-channel + multi-codec)
- [negotiate/](negotiate/README.md) - Negotiation module (Bitmap protocol)
- [codec/](codec/README.md) - Encoding/decoding module
- [channel/](channel/README.md) - Channel module
- [fragment/](fragment/README.md) - Fragment module
- [session/](session/README.md) - Session module
- [protocol/](protocol/README.md) - Protocol layer
- [keyprovider/](keyprovider/embedded/README.md) - Key provider
- [tests/](tests/README.md) - Test documentation

## Detailed Documentation

- [Architecture Design Document](docs/ARCHITECTURE.md)
- [Interface Detailed Specification](docs/INTERFACE.md)
- [Chinese Documentation](README_ZH.md)

## Build and Test

```bash
# Build all modules
go build ./...

# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run performance benchmarks
go test -bench=. -benchmem ./...

# Run specific module tests
go test -v ./protocol/...
```

## License

MIT License