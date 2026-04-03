# Session Registry

## 覂述

SessionRegistry 管理 Server 端的多 channel Session 关联。当 Client 通过多个 channel 连接时，Server 使用 SessionRegistry 将所有 channel 关联到同一个 Session。

## 核心概念

### Session-based Multi-Channel Association

VoidBus 支持 Client 同时使用多个 channel（TCP、WebSocket、UDP）进行通信。为了关联这些 channel 到同一个连接，使用 SessionID 进行识别：

1. **首次连接**：Client 通过第一个 channel 发送 NegotiateRequest（SessionID = 空）
2. **获取 SessionID**：Server 返回 NegotiateResponse，包含生成的 SessionID
3. **后续连接**：Client 通过其他 channel 发送 NegotiateRequest（带上 SessionID）
4. **关联 Session**：Server 收到带 SessionID 的请求，关联到现有 Session

### SessionState 结构

```go
type SessionState struct {
    SessionID         string
    Bus               *Bus
    ConnectedChannels ChannelBitmap
    ExpectedChannels  ChannelBitmap
    Ready             bool
    ReadyChan         chan struct{}
    mu                sync.RWMutex
}
```

- `SessionID`: UUID，标识唯一 Session
- `Bus`: Client 独立的 Bus 实例
- `ConnectedChannels`: 已连接的 channel bitmap
- `ExpectedChannels`: 协商时确定的 channel bitmap
- `Ready`: Session 是否就绪（首个 channel 连接时立即就绪）

## API

### CreateSession

首次连接时创建新 Session：

```go
func (r *SessionRegistry) CreateSession(
    sessionID string,
    expectedChannels ChannelBitmap,
    negotiatedCodecs CodecBitmap,
) *SessionState
```

### AssociateSession

后续连接时关联到现有 Session：

```go
func (r *SessionRegistry) AssociateSession(
    sessionID string,
    channelType ChannelBit,
    channelID string,
    ch channel.Channel,
) error
```

### HasChannelType

检查 Session 是否已有特定类型的 channel：

```go
func (s *SessionState) HasChannelType(channelType ChannelBit) bool
```

用于 UDP 重复连接检测（每个 Session 只能有一个 UDP channel）。

## 时序图

```
Client                              Server
  │                                    │
  │─── TCP: NegotiateRequest ─────────→│ (SessionID = 空)
  │                                    │ CreateSession(sessionID="abc123")
  │←── TCP: NegotiateResponse ────────│ (SessionID = "abc123")
  │                                    │ Accept() 返回 conn
  │                                    │
  │─── WS: NegotiateRequest ──────────→│ (SessionID = "abc123")
  │                                    │ AssociateSession(sessionID="abc123", WS)
  │←── WS: NegotiateResponse ─────────│ (Status = Success)
  │                                    │
  │─── UDP: NegotiateRequest ─────────→│ (SessionID = "abc123")
  │                                    │ AssociateSession(sessionID="abc123", UDP)
  │←── UDP: NegotiateResponse ────────│ (Status = Success)
```

## UDP 重复连接处理

由于 UDP ServerChannel 使用 `clientsByAddr[remoteAddr.String()]` 路由，同一 client address 的第二个连接会覆盖第一个，导致数据丢失。

**解决方案**：
- Server 检测后续 UDP 连接（`!request.IsFirstConnection() && ch.Type() == UDP`）
- 检查 Session 是否已有 UDP channel
- 如果已有，发送 rejection response 并关闭 channel

```go
if !request.IsFirstConnection() && clientCh.Type() == channel.TypeUDP {
    session := l.sessionRegistry.GetSession(request.SessionID)
    if session != nil && session.HasChannelType(negotiate.ChannelBitUDP) {
        rejectResponse, _ := negotiate.NewNegotiateResponse(nil, nil, nil, negotiate.NegotiateStatusReject)
        clientCh.Send(rejectData)
        clientCh.Close()
        return
    }
}
```

## 注意事项

1. **首个 channel 立即就绪**：不需要等待所有 channel 连接，Accept 立即返回
2. **后续 channel 动态添加**：已就绪 Session 的后续 channel 直接添加到 channelPool
3. **UDP 单实例限制**：每个 Session 只能有一个 UDP channel
4. **Session 清理**：Session 超时后自动清理（default: 60s）