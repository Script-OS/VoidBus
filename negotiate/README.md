# VoidBus Negotiate Module

VoidBus协商模块提供隐蔽信道通信的初始握手和协商功能。

**重要**: Bitmap由Bus层**自动生成**，用户无需手动设置。

## 设计约束

- 协商数据**必须**使用二进制Bitmap格式（非明文）
- 协商协议**禁止**使用VoidBus Header格式
- Channel类型**禁止**在正常传输中暴露
- Codec ID仅用于协商，实际传输使用Hash匹配
- **Bitmap自动生成**：从注册的Codec和Channel自动生成

## 模块结构

```
negotiate/
├── interface.go           # Negotiator接口定义
├── frame.go               # NegotiateRequest/Response帧编解码
├── codec_bitmap.go        # Codec Bitmap定义
├── channel_bitmap.go      # Channel Bitmap定义
├── client_negotiator.go   # Client端协商器
├── server_negotiator.go   # Server端协商器 + SessionManager
├── bitmap_utils.go        # Bitmap工具函数
└── negotiate_test.go      # 协商模块测试（79.5%覆盖率）
```

## 协商流程

```
Client                                    Server
  │                                         │
  │  NegotiateRequest                       │
  │  (ChannelBitmap + CodecBitmap + Nonce)  │
  │  ──────────────────────────────────────▶│
  │                                         │
  │                        计算交集（AND运算） │
  │                                         │
  │  NegotiateResponse                      │
  │  (可用信道 + Codec + SessionID)          │
  │  ◀──────────────────────────────────────│
  │                                         │
  │  验证 + 保存SessionID                    │
  │                                         │
```

## 帧格式

### NegotiateRequest

```
┌─────────────────────────────────────────────────────────────┐
│ [1 byte:  Magic]          │ 固定值 0x56 ('V')               │
│ [1 byte:  Version]        │ 协议版本 0x01                    │
│ [1 byte:  ChannelCount]   │ Channel Bitmap字节数            │
│ [N bytes: ChannelBitmap]  │ 支持的信道Bitmap                 │
│ [1 byte:  CodecCount]     │ Codec Bitmap字节数              │
│ [N bytes: CodecBitmap]    │ 支持的Codec Bitmap              │
│ [8 bytes: SessionNonce]   │ 随机Nonce（用于SessionID生成）   │
│ [4 bytes: Timestamp]      │ Unix时间戳（防重放）             │
│ [1 byte:  PaddingLen]     │ 随机填充长度（0-255）            │
│ [M bytes: RandomPadding]  │ 随机填充（隐蔽性）               │
│ [2 bytes: Checksum]       │ CRC16校验                       │
└─────────────────────────────────────────────────────────────┘
```

### NegotiateResponse

```
┌─────────────────────────────────────────────────────────────┐
│ [1 byte:  Magic]          │ 固定值 0x42 ('B')               │
│ [1 byte:  Version]        │ 协议版本 0x01                    │
│ [1 byte:  ChannelCount]   │ Channel Bitmap字节数            │
│ [N bytes: ChannelBitmap]  │ 可用信道Bitmap（交集）           │
│ [1 byte:  CodecCount]     │ Codec Bitmap字节数              │
│ [N bytes: CodecBitmap]    │ 可用Codec Bitmap（交集）        │
│ [8 bytes: SessionID]      │ Server生成的SessionID           │
│ [1 byte:  Status]         │ 协商状态（0=成功, 1=拒绝）       │
│ [1 byte:  PaddingLen]     │ 随机填充长度                     │
│ [M bytes: RandomPadding]  │ 随机填充                        │
│ [2 bytes: Checksum]       │ CRC16校验                       │
└─────────────────────────────────────────────────────────────┘
```

## Bitmap定义

### Codec Bitmap

每个bit代表一种Codec类型：

| Bit | Codec | 说明 |
|-----|-------|------|
| 0 | Plain | 无转换（仅调试） |
| 1 | Base64 | Base64编码 |
| 2 | AES-256-GCM | AES加密 |
| 3 | XOR | XOR编码 |
| 4 | ChaCha20-Poly1305 | ChaCha20加密 |
| 5 | RSA-OAEP | RSA加密 |
| 6 | GZIP | GZIP压缩 |
| 7 | ZSTD | ZSTD压缩 |

**示例**：支持Plain + AES-256-GCM + ChaCha20
```
Bitmap = 0b00010101 = []byte{0x15}
```

### Channel Bitmap

每个bit代表一种Channel类型：

| Bit | Channel | IsReliable | 说明 |
|-----|---------|------------|------|
| 0 | WebSocket | ✅ | 默认协商信道 |
| 1 | TCP | ✅ | 可靠传输 |
| 2 | QUIC | ✅ | 0-RTT连接 |
| 3 | UDP | ❌ | 需ACK/NAK |
| 4 | ICMP | ❌ | ICMP隧道 |
| 5 | DNS | ❌ | DNS隧道 |
| 6 | HTTP | ✅ | HTTP/HTTPS |

**示例**：支持WebSocket + TCP + QUIC
```
Bitmap = 0b00000111 = []byte{0x07}
```

## 使用示例

### Client端

```go
import "github.com/Script-OS/VoidBus/negotiate"

// 创建协商器
config := negotiate.DefaultNegotiatorConfig()
client := negotiate.NewClientNegotiator(config)

// 设置支持的信道和Codec
chBitmap := negotiate.NewChannelBitmap(2)
chBitmap.SetChannel(negotiate.ChannelBitWS)
chBitmap.SetChannel(negotiate.ChannelBitTCP)
client.SetChannelBitmap(chBitmap)

codecBitmap := negotiate.NewCodecBitmap(2)
codecBitmap.SetCodec(negotiate.CodecBitAES256)
codecBitmap.SetCodec(negotiate.CodecBitChaCha20)
client.SetCodecBitmap(codecBitmap)

// 创建请求
request, err := client.CreateRequest()
if err != nil {
    panic(err)
}

// 编码为字节（通过WebSocket发送）
encoded, err := request.Encode()
if err != nil {
    panic(err)
}

// 发送encoded到Server...
// 接收Server响应...

// 处理响应
response, err := negotiate.DecodeNegotiateResponse(serverData)
result, err := client.ProcessResponse(response)
if result.IsSuccess() {
    // 协商成功，获取可用信道和Codec
    channels := result.GetAvailableChannelIDs()
    codecs := result.GetAvailableCodecIDs()
    sessionID := result.SessionID
}
```

### Server端

```go
import "github.com/Script-OS/VoidBus/negotiate"

// 创建协商器
config := negotiate.DefaultNegotiatorConfig()
server := negotiate.NewServerNegotiator(config)

// 设置Server支持的信道和Codec
chBitmap := negotiate.NewChannelBitmap(2)
chBitmap.SetChannel(negotiate.ChannelBitWS)
chBitmap.SetChannel(negotiate.ChannelBitQUIC)
server.SetChannelBitmap(chBitmap)

codecBitmap := negotiate.NewCodecBitmap(2)
codecBitmap.SetCodec(negotiate.CodecBitAES256)
codecBitmap.SetCodec(negotiate.CodecBitBase64)
server.SetCodecBitmap(codecBitmap)

// 处理Client请求
request, err := negotiate.DecodeNegotiateRequest(clientData)
response, err := server.HandleRequest(request)

// 编码响应
encoded, err := response.Encode()

// 发送encoded到Client...

// 保存Session
sessionManager := negotiate.NewSessionManager()
sessionManager.AddSession(response.SessionID, result)
```

## SessionManager

Server端使用SessionManager管理协商后的Session：

```go
// 创建SessionManager
sm := negotiate.NewSessionManager()

// 添加Session
sm.AddSession(sessionID, result)

// 获取Session
result, ok := sm.GetSession(sessionID)

// 移除Session
sm.RemoveSession(sessionID)

// Session计数
count := sm.SessionCount()
```

## 安全验证

协商协议包含以下安全验证：

| 验证项 | 说明 |
|--------|------|
| Magic检查 | Request: 0x56, Response: 0x42 |
| Version检查 | 当前版本: 0x01 |
| CRC16校验 | 帧完整性校验 |
| Timestamp验证 | 防重放攻击（±30秒） |
| Bitmap交集验证 | 确保双方有共同信道/Codec |

## 常量定义

```go
// Magic Numbers
NegotiateMagicRequest  = 0x56  // 'V'
NegotiateMagicResponse = 0x42  // 'B'
NegotiateVersion       = 0x01

// 状态码
NegotiateStatusSuccess = 0x00
NegotiateStatusReject  = 0x01
NegotiateStatusRetry   = 0x02

// 尺寸限制
NegotiateMinFrameSize    = 20
NegiateMaxFrameSize      = 300
NegotiateNonceSize       = 8
NegotiateSessionIDSize   = 8
NegotiateDefaultTimeout  = 10s
```

## 测试

```bash
# 运行协商模块测试
go test ./negotiate/... -v

# 查看覆盖率
go test ./negotiate/... -cover

# 运行特定测试
go test ./negotiate/... -run TestNegotiateRequest
```

## 与Bus集成

协商模块通过Bus层集成：

```go
bus, _ := VoidBus.New()

// 设置协商Bitmap
bus.SetNegotiatedChannelBitmap(channelBitmap)
bus.SetNegotiatedCodecBitmap(codecBitmap)

// 执行协商（默认WebSocket）
err := bus.Negotiate(negotiate.ChannelBitWS)

// 协商成功后自动使用Bitmap限制Codec/Channel选择
```

## 错误类型

```go
ErrInvalidMagic     // Magic不匹配
ErrInvalidVersion   // 版本不匹配
ErrInvalidChecksum  // 校验失败
ErrInvalidFrameSize // 帧大小异常
ErrTimestampExpired // 时间戳过期
ErrNoCommonChannels // 无共同信道
ErrNoCommonCodecs   // 无共同Codec
ErrNonceSize        // Nonce大小错误
ErrSessionIDSize    // SessionID大小错误
```

## 注意事项

1. **默认协商信道**：WebSocket（易穿透防火墙）
2. **随机填充**：增加隐蔽性，防止固定长度模式识别
3. **SessionID生成**：SHA256(Nonce)截断8字节
4. **交集计算**：Bitmap AND运算
5. **Codec动态组合**：协商后双方基于Bitmap自行动态选择组合