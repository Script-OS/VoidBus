# Protocol Package - 协议层

协议层定义VoidBus通信过程中的数据结构和协商机制。

## 文件结构

```
protocol/
├── packet.go      # Packet/Header结构定义
├── handshake.go   # Handshake协议（Request/Response/Confirm）
├── message.go     # Message结构定义
└── policy.go      # NegotiationPolicy定义
```

## 模块职责

### Packet

**职责**：
- 定义数据包的结构和编码/解码方法
- 提供数据完整性校验（CRC32）
- 支持分片元数据

**Packet结构**：
```go
type Packet struct {
    Header  PacketHeader
    Payload []byte
}

type PacketHeader struct {
    SessionID         string    // UUID，间接引用Codec/Channel配置
    FragmentInfo      FragmentInfo
    SerializerType    string    // 可暴露，用于协商
    PayloadChecksum   uint32    // CRC32校验
    Timestamp         int64     // 防重放攻击
    Version           uint8     // 协议版本
}
```

**编码格式**：
```
[HeaderLength:4bytes][Header:JSON][Payload:Nbytes]
```

### Handshake

**职责**：
- 定义三步握手协议
- 支持Serializer和CodecChain协商
- 包含Challenge验证机制

**握手流程**：
```
Client                          Server
  │                               │
  │── HandshakeRequest ──────────→│
  │   (支持的Serializer列表)       │
  │   (支持的CodecChain安全等级)   │
  │                               │
  │←── HandshakeResponse ─────────│
  │   (SelectedSerializer)        │
  │   (SelectedCodecChainInfo)    │
  │   (SessionID)                 │
  │   (ServerChallenge)           │
  │                               │
  │── HandshakeConfirm ──────────→│
  │   (ChallengeResponse)         │
  │                               │
  │←── 连接建立                   │
```

**结构定义**：
```go
type HandshakeRequest struct {
    ClientID           string
    SupportedSerializers []string
    SupportedCodecLevels []SecurityLevel
    MinSecurityLevel    SecurityLevel
    Timestamp           int64
}

type HandshakeResponse struct {
    SessionID           string
    SelectedSerializer  string
    SelectedCodecLevel  SecurityLevel
    ServerChallenge     []byte
    Timestamp           int64
}

type HandshakeConfirm struct {
    ClientID            string
    SessionID           string
    ChallengeResponse   []byte
    Timestamp           int64
}
```

### Message

**职责**：
- 定义用户层消息结构
- 支持消息类型标识

```go
type Message struct {
    Type    MessageType
    Data    []byte
    Meta    map[string]string
}

type MessageType uint8

const (
    MessageTypeData      MessageType = 0
    MessageTypeHeartbeat MessageType = 1
    MessageTypeAck       MessageType = 2
    MessageTypeError     MessageType = 3
)
```

### Policy (NegotiationPolicy)

**职责**：
- 定义安全协商策略
- 控制最小安全等级
- 防止降级攻击

```go
type NegotiationPolicy struct {
    DebugMode           bool           // 调试模式允许plaintext
    MinSecurityLevel    SecurityLevel  // Release>=Medium
    RejectOnMismatch    bool           // 安全等级不匹配时拒绝
    MaxCodecChainLength int            // 防止过长链
}
```

**预设策略**：
```go
// DefaultNegotiationPolicy - Release模式
func DefaultNegotiationPolicy() *NegotiationPolicy {
    return &NegotiationPolicy{
        DebugMode:           false,
        MinSecurityLevel:    SecurityLevelMedium, // 至少AES-128
        RejectOnMismatch:    true,
        MaxCodecChainLength: 5,
    }
}

// DebugNegotiationPolicy - 调试模式
func DebugNegotiationPolicy() *NegotiationPolicy {
    return &NegotiationPolicy{
        DebugMode:           true,
        MinSecurityLevel:    SecurityLevelNone, // 允许plaintext
        RejectOnMismatch:    false,
        MaxCodecChainLength: 10,
    }
}
```

## 安全设计

### 防重放攻击
- PacketHeader包含Timestamp字段
- 服务端验证时间戳在合理范围内

### 防降级攻击
- Release模式MinSecurityLevel >= Medium
- 禁止使用Plain Codec（SecurityLevel=0）
- Challenge验证确保客户端具备声称的安全能力

### 数据完整性
- PayloadChecksum使用CRC32校验
- 接收端验证校验和

## 依赖关系

```
protocol/
├── 依赖 → serializer/  # SerializerType
├── 依赖 → codec/       # SecurityLevel
├── 依赖 → fragment/    # FragmentInfo
├── 依赖 → internal/    # ID生成, Checksum
└── 依赖 → errors.go    # 错误定义
```

## 使用示例

### 创建Packet

```go
packet := &protocol.Packet{
    Header: protocol.PacketHeader{
        SessionID:      sessionID,
        SerializerType: "plain",
        Timestamp:      time.Now().Unix(),
        Version:        1,
    },
    Payload: data,
}

// 设置校验和
packet.Header.PayloadChecksum = internal.CalculateChecksum(data)

// 编码
encoded, err := packet.Encode()
```

### 解码Packet

```go
packet, err := protocol.DecodePacket(encoded)
if err != nil {
    return err
}

// 验证校验和
expectedChecksum := internal.CalculateChecksum(packet.Payload)
if packet.Header.PayloadChecksum != expectedChecksum {
    return errors.ErrChecksumMismatch
}
```

### 使用NegotiationPolicy

```go
// Release模式 - 严格安全
policy := protocol.DefaultNegotiationPolicy()

// Debug模式 - 允许plaintext
policy := protocol.DebugNegotiationPolicy()

// 自定义策略
policy := &protocol.NegotiationPolicy{
    DebugMode:           false,
    MinSecurityLevel:    codec.SecurityLevelHigh, // 要求AES-256
    RejectOnMismatch:    true,
    MaxCodecChainLength: 3,
}
```