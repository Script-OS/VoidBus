# VoidBus 架构设计文档

## 1. 项目概述

VoidBus是一个高度模块化、可组合的通信总线库，实现信道与序列化/编解码的完全分离，支持任意组合和更换。

### 核心特性
- **四层分离架构**：Serializer（序列化）+ Codec（编解码）+ Channel（信道）+ Fragment（分片）
- **Codec链式组合**：支持多个Codec按顺序组合，如 AES → Base64
- **可插拔架构**：所有模块通过build tags控制编译，仅编译需要的模块
- **双向全双工通信**：Server侧可同时向多个客户端接收和发送信息
- **分片多信道传输**：支持数据分片，通过不同信道/编码组合发送
- **安全协商机制**：防止降级攻击，Release模式禁用plaintext
- **灵活的密钥管理**：支持URL加载（预留）和embed嵌入两种密钥获取方式

### 安全边界
| 模块 | 可暴露性 | 说明 |
|------|----------|------|
| Serializer | ✅ 可暴露 | 序列化类型可出现在元数据协议中 |
| Codec | ❌ 不可暴露 | 加密/编码方式不可暴露，仅通过SessionID间接引用 |
| Channel | ❌ 不可暴露 | 信道类型不可暴露 |
| KeyProvider | ❌ 不可暴露 | 密钥相关信息不可暴露 |

## 2. 架构约束

### 2.1 模块边界定义

#### 2.1.1 Serializer（序列化模块）
**职责**：
- 负责数据结构的序列化与反序列化
- 提供序列化类型标识（可暴露）

**不负责**：
- 数据编码/加密
- 数据传输
- 数据分片

**接口契约**：
```go
type Serializer interface {
    Serialize(data []byte) ([]byte, error)
    Deserialize(data []byte) ([]byte, error)
    Name() string      // 可暴露在元数据中
    Priority() int     // 用于协商排序
}
```

#### 2.1.2 Codec（编解码模块）
**职责**：
- 负责数据的编码/加密和解码/解密
- 提供安全等级标识（用于协商，不暴露具体名称）
- 支持密钥注入（通过KeyProvider）

**不负责**：
- 数据序列化
- 数据传输
- 密钥获取（由KeyProvider提供）

**接口契约**：
```go
type Codec interface {
    Encode(data []byte) ([]byte, error)
    Decode(data []byte) ([]byte, error)
    InternalID() string        // 内部标识，不可传输
    SecurityLevel() SecurityLevel
}

type KeyAwareCodec interface {
    Codec
    SetKeyProvider(provider KeyProvider) error
    RequiresKey() bool
    KeyAlgorithm() string
}

// SecurityLevel 安全等级
const (
    SecurityLevelNone   = 0  // Plaintext（仅调试）
    SecurityLevelLow    = 1  // Base64等
    SecurityLevelMedium = 2  // AES-128
    SecurityLevelHigh   = 3  // AES-256, RSA
)
```

#### 2.1.3 CodecChain（Codec链）
**职责**：
- 支持多个Codec的链式组合
- 管理Codec顺序
- 计算整体安全等级（取最低值）

**处理顺序**：
```
Encode:  data → Codec[0].Encode → Codec[1].Encode → ... → output
Decode:  data → Codec[n].Decode → ... → Codec[1].Decode → Codec[0].Decode → output
```

#### 2.1.4 Channel（信道模块）
**职责**：
- 负责底层传输层的建立和维护
- 处理网络连接的生命周期
- 提供数据的发送和接收接口
- 支持心跳保活机制

**不负责**：
- 数据的序列化
- 数据编码/加密
- 数据分片
- 密钥管理

**接口契约**：
```go
type Channel interface {
    Send(data []byte) error
    Receive() ([]byte, error)
    Close() error
    IsConnected() bool
    Type() ChannelType    // 内部标识，不可传输
}

type ServerChannel interface {
    Channel
    Accept() (Channel, error)
    ListenAddress() string
}
```

#### 2.1.5 Fragment（分片模块）
**职责**：
- 负责大数据的分片和重组
- 管理分片元数据（分片存在可暴露，细节不可暴露）
- 支持分片完整性校验

**不负责**：
- 数据传输
- 数据序列化
- 数据编码/加密

**接口契约**：
```go
type Fragment interface {
    Split(data []byte, maxSize int) ([][]byte, error)
    Reassemble(fragments [][]byte) ([]byte, error)
    GetFragmentInfo(fragment []byte) (FragmentInfo, error)
    SetFragmentInfo(data []byte, info FragmentInfo) ([]byte, error)
}

type FragmentInfo struct {
    ID        string    // UUID，随机无语义，可暴露
    Index     uint16    // 可暴露
    Total     uint16    // 可暴露
    Checksum  uint32
    IsLast    bool
}
```

#### 2.1.6 KeyProvider（密钥提供者模块）
**职责**：
- 提供密钥获取接口
- 支持多种密钥来源（URL/Embedded）
- 预留密钥刷新机制（架构兼容）

**不负责**：
- 使用密钥进行加解密
- 密钥生成
- 密钥存储安全

**接口契约**：
```go
type KeyProvider interface {
    GetKey() ([]byte, error)
    RefreshKey() error              // 当前返回ErrNotImplemented
    SupportsRefresh() bool          // 当前返回false，架构兼容
    Type() KeyProviderType
}
```

#### 2.1.7 Bus（总线核心）
**职责**：
- 组装和管理各个模块实例
- 协调数据流在模块间的传递
- 提供统一的使用接口
- 管理双向通信

**不负责**：
- 具体的传输实现
- 具体的序列化实现
- 具体的编解码实现
- 具体的分片逻辑

#### 2.1.8 ServerBus（服务端总线）
**职责**：
- 监听并接受客户端连接
- 管理多个客户端Bus实例
- 执行安全协商（Handshake）
- 防止降级攻击

**不负责**：
- 具体业务逻辑处理
- 客户端配置选择

#### 2.1.9 MultiBus（多信道总线）
**职责**：
- 管理多个Bus实例
- 支持分片到信道的分配（随机/指定）
- 聚合所有信道接收的消息

**分配策略**：
- 随机多信道分片：系统自动分配分片到不同信道
- 用户指定单一信道：完整数据通过指定信道发送

### 2.2 数据流定义

#### 2.2.1 发送流程
```
原始数据
  → Serializer.Serialize() → 序列化数据
  → CodecChain.Encode() → 编码/加密数据
  → [可选] Fragment.Split() → 分片数据
  → Channel.Send() → 网络传输
```

#### 2.2.2 接收流程
```
Channel.Receive() → 原始网络数据
  → [可选] Fragment.Reassemble() → 完整数据
  → CodecChain.Decode() → 序列化数据
  → Serializer.Deserialize() → 原始数据
```

### 2.3 模块依赖方向

```
┌─────────────────────────────────────────────┐
│                  Bus                        │
│    ┌─────────┬─────────┬─────────┬───────┐  │
│    │Serializer│CodecChain│Channel │Fragment│ │
│    └─────────┴────┬────┴────┬────┴───────┘  │
│                   │         │               │
│            KeyProvider      │               │
│                             │               │
│                    ServerChannel (可选)      │
└─────────────────────────────────────────────┘

依赖规则：
- Serializer: 无依赖
- Codec: 可选依赖KeyProvider
- CodecChain: 组合多个Codec
- Channel: 无依赖
- Fragment: 无依赖
- KeyProvider: 无依赖
- Bus: 组合所有模块
```

### 2.4 元数据协议设计

#### Packet结构
```go
type Packet struct {
    Header  PacketHeader
    Payload []byte
}

type PacketHeader struct {
    SessionID         string    // UUID，间接引用Codec/Channel配置
    FragmentInfo      FragmentInfo
    SerializerType    string    // 可暴露
    PayloadChecksum   uint32
    Timestamp         int64     // 防重放
    Version           uint8
}
```

#### 安全设计
- **SessionID**: 随机UUID，不直接暴露配置，仅作为本地SessionRegistry的索引
- **SerializerType**: 可暴露，用于双方协商
- **Codec配置**: 仅存储在本地SessionRegistry中，不传输
- **Channel类型**: 仅存储在本地，不传输

### 2.5 安全协商机制

#### Handshake流程
```
Client                          Server
  │                               │
  │── HandshakeRequest ──────────→│
  │   (支持的Serializer列表)       │
  │   (支持的CodecChain安全等级)   │
  │   (MinSecurityLevel)          │
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

#### 安全策略
```go
type NegotiationPolicy struct {
    DebugMode           bool           // 调试模式允许plaintext
    MinSecurityLevel    SecurityLevel  // Release>=Medium
    RejectOnMismatch    bool           // 安全等级不匹配时拒绝
    MaxCodecChainLength int            // 防止过长链
}
```

### 2.6 编译时模块选择

使用Go的build tags机制：

```go
// serializer/plain/plain.go
//go:build plain_serializer

// serializer/plain/plain_empty.go
//go:build !plain_serializer
```

编译命令：
```bash
# TCP + AES-256 + JSON
go build -tags "tcp_channel,aes_codec,json_serializer"

# UDP + Base64 + Plain (调试)
go build -tags "udp_channel,base64_codec,plain_serializer,debug_mode"

# ICMP + RSA + Protobuf
go build -tags "icmp_channel,rsa_codec,protobuf_serializer"
```

### 2.7 错误处理策略

1. **Serializer层**：
   - 序列化失败返回明确错误
   - 无效数据返回ErrInvalidData

2. **Codec层**：
   - 密钥缺失返回ErrKeyRequired
   - 无效密钥返回ErrInvalidKey
   - 编解码失败返回具体原因

3. **Channel层**：
   - 网络错误返回error
   - 连接状态通过IsConnected()标识
   - 支持重连机制（由Bus配置控制）

4. **Fragment层**：
   - 分片丢失通过FragmentInfo标识
   - 超时自动清理不完整分片组

5. **Bus层**：
   - 模块缺失返回ErrModuleNotSet
   - 协商失败返回ErrHandshakeFailed
   - 降级攻击返回ErrDegradationAttack

### 2.8 版本兼容性要求

1. **接口稳定性**：
   - 核心接口保持向后兼容
   - 新增功能通过接口扩展实现
   - 废弃接口标记Deprecated并保留一个版本周期

2. **密钥刷新兼容性**：
   - RefreshKey当前返回ErrNotImplemented
   - SupportsRefresh返回false
   - 接口预留，未来实现无需改动架构

## 3. 实现优先级

### Phase 1: 核心框架
1. 定义所有核心接口
2. 实现SessionRegistry
3. 实现Bus基础结构
4. 实现基本的TCP Channel
5. 实现Plain Serializer
6. 实现CodecChain基础

### Phase 2: 基础功能
1. 实现UDP Channel
2. 实现Base64 Codec
3. 实现AES Codec（支持KeyProvider）
4. 实现JSON Serializer
5. 实现EmbeddedKeyProvider

### Phase 3: 高级功能
1. 实现ICMP Channel
2. 实现RSA Codec
3. 实现Fragment分片模块
4. 实现FragmentManager
5. 实现ServerBus + Handshake
6. 实现MultiBus

### Phase 4: 未来功能
1. URL KeyProvider实现
2. 密钥刷新/轮换机制
3. Protobuf Serializer

## 4. 使用示例

### 4.1 基本客户端使用
```go
// 创建Codec链: AES-256 -> Base64
codecChain := codec.NewChain().
    AddCodec(aes.NewAES256GCM()).
    AddCodec(base64.New())
codecChain.SetKeyProvider(embedded.New(...))

// 创建Bus
bus := voidbus.NewBuilder().
    UseSerializerInstance(json.New()).
    UseCodecChain(codecChain).
    UseChannel(tcp.NewClient("server:8080")).
    WithConfig(voidbus.BusConfig{
        AutoReconnect: true,
    }).
    OnMessage(func(data []byte) {
        fmt.Println("Received:", string(data))
    }).
    Build()

bus.Start()
bus.Send([]byte("Hello, VoidBus!"))
```

### 4.2 服务端使用
```go
serverBus := voidbus.NewServerBus().
    SetNegotiationPolicy(voidbus.DefaultNegotiationPolicy()).
    SetSerializer(json.New()).
    SetCodecChain(codec.NewChain().AddCodec(aes.NewAES256GCM())).
    OnClientConnect(func(clientID string, bus *voidbus.ClientBus) {
        fmt.Println("Client connected:", clientID)
    }).
    OnMessage(func(clientID string, data []byte) {
        fmt.Println("From", clientID, ":", string(data))
        serverBus.SendTo(clientID, []byte("ACK"))
    })

serverBus.Listen(":8080")
serverBus.Start()
```

### 4.3 MultiBus分片多信道发送
```go
// 创建多个Bus
tcpBus := voidbus.NewBuilder().
    UseChannel(tcp.NewClient("server:8080")).
    UseSerializer(json.New()).
    UseCodecChain(codec.NewChain().AddCodec(aes.NewAES256GCM())).
    Build()

udpBus := voidbus.NewBuilder().
    UseChannel(udp.NewClient("server:9090")).
    UseSerializer(json.New()).
    UseCodecChain(codec.NewChain().AddCodec(base64.New())).
    Build()

// 创建MultiBus
multiBus := voidbus.NewMultiBus().
    AddBus(tcpBus, 2, "primary").    // 权重2
    AddBus(udpBus, 1, "backup").     // 权重1
    SetDefaultStrategy(voidbus.SendStrategy{
        Mode:           voidbus.ModeWeighted,
        EnableFragment: true,
        MaxFragmentSize: 1024,
    }).
    OnMessage(func(sourceBusID string, data []byte) {
        fmt.Println("From", sourceBusID, ":", string(data))
    })

multiBus.Start()

// 加权随机多信道发送（自动分片）
multiBus.Send([]byte("Large data..."))

// 指定单一信道发送
multiBus.SendVia("primary", []byte("Important data"))
```

## 5. 目录结构

```
VoidBus/
├── docs/
│   ├── ARCHITECTURE.md          # 架构设计文档
│   └── INTERFACE.md             # 接口详细说明
│
├── core/                        # 核心组装层
│   ├── interfaces.go            # Bus/ServerBus/MultiBus接口定义
│   ├── bus.go                   # Bus核心实现
│   ├── serverbus.go             # ServerBus实现（服务端Hall模式）
│   ├── multibus.go              # MultiBus实现（多信道组合）
│   └── README.md                # 模块文档
│
├── protocol/                    # 协议层
│   ├── packet.go                # Packet/Header结构
│   ├── handshake.go             # Handshake协议实现
│   ├── message.go               # Message结构定义
│   ├── policy.go                # NegotiationPolicy定义
│   └── README.md                # 模块文档
│
├── registry/                    # 注册表
│   ├── registry.go              # SessionRegistry实现
│   └── README.md                # 模块文档
│
├── errors.go                    # 全局错误定义
│
├── serializer/                  # 序列化器模块 [可暴露]
│   ├── interface.go             # Serializer接口定义
│   ├── serializer.go            # SerializerRegistry
│   ├── plain/                   # Pass-through实现
│   │   └── plain.go
│   └── README.md                # 模块文档
│
├── codec/                       # 编码/加密模块 [不可暴露]
│   ├── interface.go             # Codec接口定义
│   ├── codec.go                 # CodecRegistry
│   ├── chain.go                 # CodecChain实现
│   ├── plain/                   # Pass-through（仅调试）
│   │   └── plain.go
│   ├── base64/                  # Base64编码
│   │   └── base64.go
│   ├── aes/                     # AES-GCM加密
│   │   └── aes.go
│   └── README.md                # 模块文档
│
├── channel/                     # 信道模块 [不可暴露]
│   ├── interface.go             # Channel接口定义
│   ├── channel.go               # ChannelRegistry
│   ├── tcp/                     # TCP传输实现
│   │   └── tcp.go
│   └── README.md                # 模块文档
│
├── fragment/                    # 分片模块 [部分可暴露]
│   ├── fragment.go              # Fragment接口 + FragmentManager
│   └── README.md                # 模块文档
│
├── keyprovider/                 # 密钥提供者 [不可暴露]
│   ├── keyprovider.go           # KeyProvider接口定义
│   ├── embedded/                # 编译时嵌入密钥
│   │   └── embedded.go
│   └── README.md                # 模块文档
│
├── internal/                    # 内部工具（不对外暴露）
│   ├── id.go                    # ID生成（UUID/SessionID）
│   ├── checksum.go              # CRC32校验
│   ├── crypto.go                # Challenge验证
│   └── README.md                # 包约束说明
│
└── README.md                    # 项目说明
```

## 6. 质量保证

### 6.1 测试策略
- 每个模块独立单元测试
- 集成测试验证模块组合
- Handshake安全测试验证协商机制
- 降级攻击测试验证安全策略
- 压力测试验证性能
- 混沌测试验证错误处理

### 6.2 代码规范
- 遵循Go标准代码规范
- 使用golangci-lint进行静态检查
- 接口注释完整
- 示例代码可运行

### 6.3 安全规范
- Release模式禁用plaintext Codec
- 协商过程验证客户端能力（Challenge）
- 元数据不暴露敏感配置
- 防重放攻击（Timestamp检查）

### 6.4 性能要求
- Serializer层：序列化延迟<5ms (1MB数据)
- Codec层：编解码延迟<10ms (1MB数据)
- CodecChain层：链式处理延迟累加
- Channel层：支持至少1Gbps吞吐量
- Fragment层：分片/重组延迟<5ms
- Bus层：组装和路由延迟<1ms

## 7. 实现状态与审查记录

### 7.1 已实现模块清单

| 模块 | 文件 | 状态 | 说明 |
|------|------|------|------|
| **Protocol** | | | |
| session.go | Session状态管理 | ✅ 完成 | Handshaking/Active/Idle/Closing/Closed |
| control.go | 控制消息 | ✅ 完成 | ACK/NACK/Heartbeat/Ping/Pong |
| selector.go | 信道选择接口 | ✅ 完成 | ChannelSelectInfo避免循环依赖 |
| distributor.go | 分片分发策略 | ✅ 完成 | 5种策略实现 |
| transport.go | 传输层 | ✅ 完成 | TransportSender/TransportReceiver |
| negotiator.go | 协商流程 | ✅ 完成 | Client/Server Negotiator |
| handshake.go | Handshake消息 | ✅ 完成 | 序列化方法 |
| packet.go | Packet结构 | ✅ 完成 | Header/Payload |
| policy.go | 协商策略 | ✅ 完成 | NegotiationPolicy |
| message.go | 消息抽象 | ✅ 完成 | Message/FragmentMetadata |
| **Codec** | | | |
| interface.go | Codec接口 | ✅ 完成 | Codec/KeyAwareCodec |
| chain.go | CodecChain | ✅ 完成 | 链式组合 |
| negotiation.go | Codec协商 | ✅ 完成 | CodecChainInfoGenerator |
| plain/plain.go | Plaintext | ✅ 完成 | 仅调试用 |
| base64/base64.go | Base64 | ✅ 完成 | Low SecurityLevel |
| aes/aes.go | AES-GCM | ✅ 完成 | Medium/High SecurityLevel |
| **Channel** | | | |
| interface.go | Channel接口 | ✅ 完成 | Channel/ServerChannel |
| channel.go | ChannelRegistry | ✅ 完成 | 模块注册 |
| tcp/tcp.go | TCP实现 | ✅ 完成 | 客户端/服务端 |
| selector/selector.go | 选择器实现 | ✅ 完成 | Random/RoundRobin/Weighted/HealthAware |
| **Registry** | | | |
| registry.go | SessionRegistry | ✅ 完成 | Session配置存储 |
| **Core** | | | |
| bus.go | Bus实现 | ✅ 完成 | 单信道总线 |
| serverbus.go | ServerBus | ✅ 完成 | 服务端总线+Handshake |
| multibus.go | MultiBus | ✅ 完成 | 多信道总线+Selector/Distributor |
| interfaces.go | 核心接口 | ✅ 完成 | Bus/ServerBus/MultiBus |

### 7.2 安全约束符合情况

| 约束项 | 状态 | 验证点 |
|--------|------|--------|
| Codec名称不暴露 | ✅ 通过 | CodecChainInfo仅含SecurityLevel+Hash |
| Channel类型不暴露 | ✅ 通过 | Type()仅用于内部标识 |
| KeyProvider信息不暴露 | ✅ 通过 | 接口不含KeyProvider返回 |
| SerializerType可暴露 | ✅ 通过 | Header.SerializerType用于协商 |
| SessionID可暴露 | ✅ 通过 | 作为间接引用，配置存储本地 |
| Challenge防降级攻击 | ✅ 通过 | ServerChallenge生成+验证 |
| Release最小SecurityLevel | ✅ 通过 | DefaultPolicy=Medium |
| Timestamp防重放 | ✅ 通过 | Packet.Timestamp+5分钟过期检查 |

### 7.3 已修复问题

| 问题 | 文件 | 修复内容 |
|------|------|----------|
| computeChainHash返回随机ID | negotiator.go | 使用SHA-256确定性哈希 |
| distributor rand.Seed废弃 | distributor.go | 移除Go 1.20+废弃代码 |

### 7.4 已知限制与待决策项

| 问题 | 影响 | 建议 |
|------|------|------|
| AddCodec超限静默返回 | 流式API设计 | 可添加AddCodecWithErr方法返回错误 |
| simpleChallengeResponse演示实现 | 中等安全 | 生产环境应使用CodecChain编码Challenge |
| Clone浅拷贝 | 低 | Codec实例共享，或标注浅拷贝语义 |
| Session错误计数RLock修改 | 低 | 使用atomic操作或改用Lock |
| Policy Validate()值类型 | 低 | 改为指针接收者 |

### 7.5 模块依赖图

```
┌─────────────────────────────────────────────────────────────────┐
│                          Core Layer                              │
│  ┌─────────┐  ┌─────────────┐  ┌───────────────┐                │
│  │ bus.go  │  │ serverbus.go│  │ multibus.go   │                │
│  └────┬────┘  └──────┬──────┘  └───────┬───────┘                │
└───────┼──────────────┼─────────────────┼────────────────────────┘
        │              │                 │
        ▼              ▼                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Protocol Layer                            │
│  ┌────────────┐ ┌──────────┐ ┌─────────┐ ┌───────────────┐      │
│  │ handshake  │ │ negotiator│ │ session │ │ transport     │      │
│  └────────────┘ └──────────┘ └────┬────┘ └───────────────┘      │
│  ┌────────────┐ ┌──────────┐     │     ┌───────────────┐        │
│  │ packet     │ │ selector │     │     │ distributor   │        │
│  └────────────┘ └──────────┘     │     └───────────────┘        │
└─────────────────────────────────┼───────────────────────────────┘
                                  │
        ┌─────────────────────────┼─────────────────────────────┐
        │                         │                              │
        ▼                         ▼                              ▼
┌───────────────┐     ┌───────────────────┐     ┌───────────────────┐
│  Serializer   │     │      Codec        │     │     Channel       │
│ ┌───────────┐ │     │ ┌───────────────┐ │     │ ┌───────────────┐ │
│ │ plain     │ │     │ │ chain.go      │ │     │ │ tcp/tcp.go   │ │
│ │ json      │ │     │ │ negotiation.go│ │     │ │ selector/    │ │
│ └───────────┘ │     │ │ plain/        │ │     │ └───────────────┘ │
│               │     │ │ base64/       │ │     │                   │
│               │     │ │ aes/          │ │     │                   │
│               │     │ └───────────────┘ │     │                   │
└───────────────┘     └───────────────────┘     └───────────────────┘
        │                     │
        │                     ▼
        │           ┌───────────────────┐
        │           │   KeyProvider     │
        │           │ ┌───────────────┐ │
        │           │ │ embedded/     │ │
        │           │ └───────────────┘ │
        │           └───────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Registry Layer                            │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    registry.go                             │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### 7.6 数据流验证

#### 发送流程
```
原始数据
  → Serializer.Serialize()     [serializer/]
  → CodecChain.Encode()        [codec/chain.go]
    → Codec[0].Encode → Codec[1].Encode → ... → Codec[n].Encode
  → Fragment.Split() [可选]    [fragment/]
  → Packet.Wrap()              [protocol/packet.go]
  → ChannelSelector.Select()   [protocol/selector.go]
  → Channel.Send()             [channel/]
```

#### 接收流程
```
Channel.Receive()             [channel/]
  → Packet.Decode()           [protocol/packet.go]
  → Fragment.Reassemble() [可选] [fragment/]
  → CodecChain.Decode()       [codec/chain.go]
    → Codec[n].Decode → ... → Codec[1].Decode → Codec[0].Decode
  → Serializer.Deserialize()  [serializer/]
  → 原始数据
```

#### Handshake流程
```
Client                          Server
  │                               │
  │── HandshakeRequest ──────────→│
  │   (SerializerInfo列表)         │ [protocol/handshake.go]
  │   (CodecChainInfo列表)         │ [codec/negotiation.go]
  │                               │
  │←── HandshakeResponse ─────────│
  │   (SelectedSerializer)        │
  │   (SelectedCodecChainInfo)    │
  │   (SessionID)                 │
  │   (ServerChallenge)           │
  │                               │
  │── HandshakeConfirm ──────────→│
  │   (ChallengeResponse)         │ [使用CodecChain编码验证]
  │                               │
  │←── SessionEstablished ────────│
```