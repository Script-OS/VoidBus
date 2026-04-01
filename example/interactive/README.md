# VoidBus Interactive Example

This example demonstrates a complete VoidBus client/server interaction with:

- TCP channel transport
- Base64 codec encoding
- Negotiation handshake with bitmap protocol
- Interactive message sending/receiving
- Multi-client server support

## Quick Start

### 1. Start Server

```bash
cd example/interactive/server
go run main.go
```

Server output:
```
╔════════════════════════════════════════════════════════════╗
║           VoidBus Interactive Server Example              ║
╠════════════════════════════════════════════════════════════╣
║ Commands:                                                  ║
║   <message>           - Broadcast to all clients           ║
║   <client-id> <msg>   - Send to specific client            ║
║   list                - List connected clients             ║
║   quit                - Exit server                        ║
╚════════════════════════════════════════════════════════════╝

[1/4] Creating TCP server...
      ✓ Server listening on :8080
[2/4] Setting up negotiator...
      → Server Channel Bitmap: 00000010 (TCP)
      → Server Codec Bitmap: 00000010 (Base64)
      ✓ Negotiator configured
[3/4] Starting client handler...
[4/4] Ready for interactive input
═════════════════════════════════════════════════════════════
>
```

### 2. Start Client (in another terminal)

```bash
cd example/interactive/client
go run main.go
```

Client output:
```
╔════════════════════════════════════════════════════════════╗
║           VoidBus Interactive Client Example              ║
╠════════════════════════════════════════════════════════════╣
║ Commands:                                                  ║
║   <message>  - Send message to server                      ║
║   quit       - Exit client                                 ║
╚════════════════════════════════════════════════════════════╝

[1/6] Creating VoidBus...
      ✓ Bus created successfully
[2/6] Registering codecs...
      ✓ Registered codec: base64 (SecurityLevel: 1)
[3/6] Connecting to server...
      ✓ TCP channel created (ID: tcp)
      ✓ Connected to localhost:8080
[4/6] Performing negotiation handshake...
      → Client Codec Bitmap: 00000010
      → Client Channel Bitmap: 00000010
      → SessionNonce: a1b2c3d4e5f6g7h8
      → Sending negotiate request (28 bytes)
      ← Received negotiate response (24 bytes)
      ← Server Codec Bitmap: 00000010
      ← Server Channel Bitmap: 00000010
      ← SessionID: 1234567890abcdef
      ← Status: 0 (Success=0, Reject=1)
      ✓ Negotiation completed successfully
[5/6] Starting receive loop...
      ✓ Receive loop started
[6/6] Ready for interactive input
═════════════════════════════════════════════════════════════

>
```

## Negotiation Handshake Details

The negotiation handshake is visible in the output:

| Stage | Client | Server |
|-------|--------|--------|
| Request | Creates bitmap from registered codecs/channels | - |
| Send | Encodes request → Send via raw channel | Receives request |
| Process | - | Decodes → Compute intersection → Creates response |
| Response | Receives → Decodes response | Encodes response → Send |
| Apply | Applies negotiated bitmap | - |

### Bitmap Format

- **Codec Bitmap**: Each bit represents a supported codec (Plain=0, Base64=1, AES=2, XOR=3, ChaCha20=4, RSA=5)
- **Channel Bitmap**: Each bit represents a supported channel (WS=0, TCP=1, QUIC=2, UDP=3)

Example: Base64 codec only → `00000010` (bit 1 set)

## Interactive Commands

### Client Commands

| Command | Description |
|---------|-------------|
| `<message>` | Send message to server |
| `quit` | Exit client |

### Server Commands

| Command | Description |
|---------|-------------|
| `<message>` | Broadcast to all connected clients |
| `<client-id> <msg>` | Send to specific client (e.g., `client-001 Hello`) |
| `list` | List all connected clients |
| `quit` | Exit server |

## Example Interaction

```
# Client terminal
> Hello Server!
📤 [MSG #1] Sending: Hello Server! (13 bytes)...
   ✓ Sent successfully

# Server terminal
> 
📨 [client-001 MSG #1] Received: Hello Server! (13 bytes)
> Hello Client!
📤 Broadcasting to 1 clients: Hello Client!
   ✓ [client-001] Sent: Hello Client! (13 bytes)

# Client terminal
> 
📨 [MSG #1] Received: Hello Client! (13 bytes)
>
```

## Architecture Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                         VoidBus Architecture                         │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Client                                                Server       │
│  ┌─────┐                                               ┌─────┐      │
│  │ Bus │                                               │ Bus │      │
│  └──┬──┘                                               └──┬──┘      │
│     │                                                      │        │
│  ┌──┴──────────────────────────────────────────────────────┴──┐   │
│  │                    TCP Channel (Raw)                        │   │
│  │  ┌──────────────────────────────────────────────────────┐  │   │
│  │  │ Negotiation (Bitmap Protocol, NOT VoidBus Header)    │  │   │
│  │  │  Request: [ChannelBitmap][CodecBitmap][Nonce]        │  │   │
│  │  │  Response: [ChannelBitmap][CodecBitmap][SessionID]   │  │   │
│  │  └──────────────────────────────────────────────────────┘  │   │
│  │                                                             │   │
│  │  ┌──────────────────────────────────────────────────────┐  │   │
│  │  │ Data Transfer (VoidBus Protocol)                     │  │   │
│  │  │  Header: [SessionID][CodecHash][FragmentInfo]...     │  │   │
│  │  │  Data: [Encoded][Fragmented]                         │  │   │
│  │  └──────────────────────────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  Components:                                                        │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐                │
│  │ CodecManager │ │ ChannelPool  │ │ FragmentMgr  │                │
│  │ (Base64)     │ │ (TCP)        │ │ (Adaptive)   │                │
│  └──────────────┘ └──────────────┘ └──────────────┘                │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Key API Usage

| API | Usage |
|-----|-------|
| `voidbus.New()` | Create Bus instance |
| `bus.RegisterCodec(codec)` | Register codec (uses `codec.Code()` as identifier) |
| `bus.AddChannel(ch)` | Add transport channel |
| `bus.Connect(addr)` | Mark as connected |
| `bus.CreateNegotiateRequest()` | Auto-generate negotiation request from registered codecs |
| `bus.ApplyNegotiateResponse(response)` | Apply negotiation result |
| `bus.StartReceive()` | Start background receive loop |
| `bus.OnMessage(handler)` | Set message callback |
| `bus.OnError(handler)` | Set error callback |
| `bus.SendWithContext(ctx, data)` | Send data with timeout |
| `bus.Stop()` | Stop and cleanup |

## Limitations

Based on current VoidBus API, the following information is **not displayed**:

- Fragment count (FragmentManager internal)
- Selected Codec chain (CodecManager internal)
- Selected Channel (ChannelPool internal)
- Send/Receive Buffer details

## Build

```bash
# Build server
cd example/interactive/server
go build -o voidbus-server

# Build client
cd example/interactive/client
go build -o voidbus-client
```

## Run

```bash
# Terminal 1: Server
./voidbus-server

# Terminal 2: Client
./voidbus-client

# Terminal 3: Another Client (optional)
./voidbus-client
```