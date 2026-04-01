# Protocol Package - 协议层

协议层定义VoidBus通信过程中的数据结构和安全验证机制。

## 文件结构

```
protocol/
├── header.go         # Header结构 + 安全验证 + 编解码
└── header_test.go    # Header安全验证测试（18个测试用例，89.3%覆盖率）
```

## Header安全验证

VoidBus v2.0 在 Protocol Header 层面实现了完整的安全验证：

| 验证项 | 常量 | 限制 | 说明 |
|--------|------|------|------|
| PacketSize | MinPacketSize/MaxPacketSize | 84-65535字节 | 防止过大/过小包 |
| SessionID | MaxSessionIDLength/MinSessionIDLength | 1-64字符 | 防止内存耗尽 |
| FragmentTotal | MaxFragmentTotal | ≤10000 | 防止资源耗尽 |
| FragmentIndex | - | < FragmentTotal | 防止索引溢出 |
| CodecDepth | MaxCodecDepth/MinCodecDepth | 1-5 | 防止深度溢出 |
| Timestamp | MaxTimestampAge/MinTimestampAge | ±1小时/-5分钟 | 防止重放攻击 |
| Flags | - | 仅允许已知位 | 防止未知标志 |

### 验证常量定义

```go
const (
    MaxSessionIDLength = 64      // SessionID最大长度
    MinSessionIDLength = 1       // SessionID最小长度
    MaxFragmentTotal   = 10000   // 最大分片数
    MaxCodecDepth      = 5       // 最大Codec深度
    MinCodecDepth      = 1       // 最小Codec深度
    MaxTimestampAge    = 3600    // 最大时间戳偏差（秒）
    MinTimestampAge    = -300    // 最小时间戳偏差（允许未来5分钟）
    MaxPacketSize      = 65535   // 最大包大小
    MinPacketSize      = 84      // 最小包大小
)
```

### V2ValidationError

验证失败时返回详细的错误信息：

```go
type V2ValidationError struct {
    Field    string      // 失败字段名
    Actual   interface{} // 实际值
    Expected interface{} // 期望值
    Msg      string      // 错误消息
}
```

## Header结构

```go
type Header struct {
    SessionID     string    // Session标识
    FragmentIndex uint16    // 分片索引
    FragmentTotal uint16    // 总分片数
    CodecDepth    uint8     // Codec链深度
    CodecHash     [32]byte  // Codec链Hash（SHA256）
    DataChecksum  uint32    // 数据CRC32校验
    DataHash      [32]byte  // 数据Hash（SHA256）
    Timestamp     int64     // 时间戳（防重放）
    Flags         uint8     // 标志位
}

// Flags定义
const (
    FlagIsLast     uint8 = 0x01 // 最后一个分片
    FlagRetransmit uint8 = 0x02 // 重传分片
    FlagIsNAK      uint8 = 0x04 // NAK消息
    FlagIsENDACK   uint8 = 0x08 // ENDACK确认
)
```

## 编码格式

Header编码格式（二进制，固定长度前缀）：

```
[SessionIDLen:1byte][SessionID:Nbytes]
[FragmentIndex:2bytes][FragmentTotal:2bytes]
[CodecDepth:1byte][CodecHash:32bytes]
[DataChecksum:4bytes][DataHash:32bytes]
[Timestamp:8bytes][Flags:1byte]
[Payload:Nbytes]
```

## 使用示例

### 编码Header

```go
header := &protocol.Header{
    SessionID:     "session-123",
    FragmentIndex: 0,
    FragmentTotal: 10,
    CodecDepth:    2,
    CodecHash:     codecHash,
    DataChecksum:  checksum,
    DataHash:      dataHash,
    Timestamp:     time.Now().Unix(),
    Flags:         protocol.FlagIsLast,
}

packet := header.Encode(payload)
```

### 解码Header（含安全验证）

```go
header, payload, err := protocol.DecodeHeader(packet)
if err != nil {
    // 处理验证错误
    if validationErr, ok := err.(*protocol.V2ValidationError); ok {
        log.Printf("验证失败: 字段=%s, 实际值=%v, 期望值=%v",
            validationErr.Field, validationErr.Actual, validationErr.Expected)
    }
    return err
}
```

## 依赖关系

```
protocol/
├── 依赖 → internal/    # Checksum计算
└── 依赖 → errors.go    # 错误定义
```

## 测试覆盖

`header_test.go` 包含18个测试用例：

| 测试类型 | 测试用例 |
|----------|----------|
| 正常验证 | TestDecodeHeader_ValidPacket |
| SessionID验证 | TestDecodeHeader_SessionIDTooLong, TestDecodeHeader_SessionIDEmpty |
| FragmentTotal验证 | TestDecodeHeader_FragmentTotalExceedsLimit |
| FragmentIndex验证 | TestDecodeHeader_FragmentIndexExceedsTotal |
| CodecDepth验证 | TestDecodeHeader_CodecDepthExceedsMax |
| Timestamp验证 | TestDecodeHeader_TimestampTooOld, TestDecodeHeader_TimestampInFuture |
| Flags验证 | TestDecodeHeader_InvalidFlags |
| PacketSize验证 | TestDecodeHeader_PacketTooShort, TestDecodeHeader_PacketTooLarge |
| 边界测试 | TestDecodeHeader_BoundaryValues |

**覆盖率**: 89.3%
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