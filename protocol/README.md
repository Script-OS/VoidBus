# Protocol Package - 协议层

协议层定义VoidBus通信过程中的数据结构和协商机制。

## 文件结构

```
protocol/
├── packet.go      # Packet/Header结构定义
├── handshake.go   # Handshake协议（Request/Response/Confirm）
├── session.go     # Session状态管理
├── control.go     # 控制消息（ACK/NACK/Heartbeat/Ping/Pong）
├── selector.go    # ChannelSelector接口定义
├── distributor.go # FragmentDistributor分发策略
├── transport.go   # TransportSender/TransportReceiver
├── negotiator.go  # Client/Server Negotiator
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

## 新增模块说明

### Session (session.go)

**职责**：
- 管理会话状态（Handshaking/Active/Idle/Closing/Closed）
- 存储Serializer/CodecChain/Channel配置（本地存储，不可传输）
- 提供统计数据（SendCount/ReceiveCount/ErrorCount）

**安全约束**：
- SessionID 可暴露（间接引用）
- 配置详情存储在本地 SessionRegistry

### Control (control.go)

**职责**：
- 定义控制消息类型（Ack/Nack/Heartbeat/Disconnect/FragmentAck/FragmentNack/Ping/Pong）
- 提供控制消息编解码

**设计**：控制消息不经过 CodecChain 加密，直接发送

### Selector (selector.go)

**职责**：
- 定义 ChannelSelector 接口（避免循环依赖，使用 ChannelSelectInfo）
- 定义 FragmentDistributor 接口

**ChannelSelectInfo**：包含 ID/Weight/Health/TypeAlias，不引用 raw Channel

### Distributor (distributor.go)

**职责**：
- 实现 5 种分发策略（AllRandom/Grouped/RoundRobin/Weighted/HealthAware）
- 将分片分配到不同信道

### Transport (transport.go)

**职责**：
- TransportSender: PrepareData() → Send()（Serialize→Encode→Fragment→Wrap）
- TransportReceiver: ReceiveAndProcess()（Receive→Reassemble→Decode→Deserialize）

**无状态设计**：状态由 Session 管理

### Negotiator (negotiator.go)

**职责**：
- ClientNegotiator: PrepareRequest() → ProcessResponse() → PrepareConfirm()
- ServerNegotiator: ProcessRequest() → VerifyConfirm()

**安全设计**：
- computeChainHash 使用 SHA-256 确定性哈希（不暴露 Codec 名称）
- Challenge 机制防止降级攻击

## 实现状态

| 文件 | 状态 | 关键特性 |
|------|------|----------|
| session.go | ✅ 完成 | 状态管理、统计计数 |
| control.go | ✅ 完成 | 8种控制消息 |
| selector.go | ✅ 完成 | ChannelSelectInfo 避免循环依赖 |
| distributor.go | ✅ 完成 | 5种分发策略 |
| transport.go | ✅ 完成 | Sender/Receiver 数据流 |
| negotiator.go | ✅ 完成 | Client/Server Negotiator |
| handshake.go | ✅ 完成 | 序列化方法 |
| packet.go | ✅ 完成 | Header/Payload |
| policy.go | ✅ 完成 | NegotiationPolicy |
| message.go | ✅ 完成 | Message/FragmentMetadata |
```