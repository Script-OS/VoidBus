# VoidBus 架构设计文档

## 1. 项目概述

VoidBus 是一个高度模块化、可组合的通信总线库，实现隐蔽通信与多信道分发能力。

### 核心特性

- **统一简洁的 API**：单一 Bus 入口，简单的 `New() → AddCodec → AddChannel → Send → Receive` 使用方式
- **随机 Codec 链选择**：从能力池中随机选择 Codec 组合，通过 Hash 匹配解码
- **自适应切片**：根据 Channel 承载能力自动调整切片大小
- **多信道随机分发**：同一数据的不同切片可通过不同信道（TCP/UDP/DNS等）发送
- **可靠传输**：Session 管理 + 分片重传机制
- **能力协商**：初始连接时协商 Codec 代号集合

### 安全边界

| 模块 | 可暴露性 | 说明 |
|------|----------|------|
| Codec 代号 | ✅ 协商时暴露 | 初始连接时交换支持的代号集合 |
| Codec Hash | ✅ 可暴露 | SHA256(代号组合)，不暴露具体组合 |
| Channel 类型 | ❌ 不可暴露 | 信道类型不暴露 |
| 密钥 | ❌ 不可暴露 | 密钥相关信息不可暴露 |

---

## 2. 架构全景图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              VoidBus（统一入口）                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                              Core Bus                                    ││
│  │  ┌───────────────┐ ┌───────────────┐ ┌───────────────┐ ┌─────────────┐  ││
│  │  │ CodecManager  │ │  ChannelPool  │ │FragmentManager│ │ SessionMgr  │  ││
│  │  │ (随机+Hash)   │ │ (MTU+健康度)  │ │ (自适应切片)  │ │ (生命周期)  │  ││
│  │  └───────────────┘ └───────────────┘ └───────────────┘ └─────────────┘  ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                        │                                    │
│  ┌────────────────────────────────────┼────────────────────────────────────┐│
│  │                            Protocol Layer                                ││
│  │  ┌────────────┐ ┌──────────────┐ ┌───────────────┐ ┌──────────────────┐││
│  │  │ Metadata   │ │ Negotiation  │ │  NAK Protocol │ │   END Protocol   │││
│  │  │ (Header)   │ │ (能力协商)   │ │  (重传请求)   │ │   (结束确认)     │││
│  │  └────────────┘ └──────────────┘ └───────────────┘ └──────────────────┘││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                        │                                    │
│  ┌────────────────────────────────────┼────────────────────────────────────┐│
│  │                            Plugin Layer                                  ││
│  │  ┌─────────────────────────────────┐ ┌─────────────────────────────────┐││
│  │  │          Codec Pool             │ │         Channel Pool            │││
│  │  │ ┌─────┐┌─────┐┌─────┐┌─────┐   │ │ ┌─────┐┌─────┐┌─────┐┌─────┐   │││
│  │  │ │AES  ││B64  ││XOR  ││...  │   │ │ │TCP  ││UDP  ││DNS  ││ICMP │   │││
│  │  │ │(A)  ││(B)  ││(X)  ││     │   │ │ │     ││     ││     ││     │   │││
│  │  │ └─────┘└─────┘└─────┘└─────┘   │ │ └─────┘└─────┘└─────┘└─────┘   │││
│  │  └─────────────────────────────────┘ └─────────────────────────────────┘││
│  └─────────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. 核心设计决策

### 3.1 架构简化

| 特性 | 旧架构 | 新架构 |
|------|--------|--------|
| Bus 类型 | Bus + MultiBus 分离 | 统一单一 Bus |
| Serializer | 必选模块 | **已取消** |
| Codec 链 | 固定配置 | **随机选择** |
| 切片大小 | 固定 MaxFragmentSize | **自适应** |
| Metadata | 不暴露 Codec 信息 | **暴露链深+Hash** |
| 多信道发送 | MultiBus 支持 | **内置随机分发** |
| 重传机制 | 无 | **Session号+分片号重传** |
| Buffer 管理 | 仅接收端 | **双端保留** |

### 3.2 设计原则

1. **简洁优先**：作为可被 import 的库，API 简单易懂
2. **隐蔽性**：Codec Hash 而非明文传输，随机选择增加分析难度
3. **可靠性**：分片重传、超时处理、健康度评估
4. **灵活性**：用户自定义 Codec 代号、最大链深度、MTU 配置

---

## 4. 模块边界定义

### 4.1 CodecManager（编解码管理器）

**职责**：
- 管理可用 Codec 实例池（代号 → Codec 映射）
- 随机选择 Codec 链组合
- 计算 Codec 链 Hash（SHA256(代号组合)）
- 接收端通过 Hash 匹配解码链

**不负责**：
- 数据序列化（已取消 Serializer）
- 数据传输
- 数据分片

**核心算法**：

```go
// 发送端：随机选择 Codec 链
func (m *CodecManager) RandomSelect(depth int) ([]string, CodecChain)

// Hash 计算
func ComputeHash(codeChain []string) [32]byte {
    concatenated := strings.Join(codeChain, "")
    return sha256.Sum256([]byte(concatenated))
}

// 接收端：排列组合匹配
func (m *CodecManager) MatchByHash(hash [32]byte, depth int, supportedCodes []string) ([]string, CodecChain, error) {
    // 生成所有可能的代号排列组合
    permutations := GeneratePermutations(supportedCodes, depth)
    
    // 对每个组合计算 Hash 并匹配
    for _, combo := range permutations {
        if ComputeHash(combo) == hash {
            return combo, m.CreateChain(combo), nil
        }
    }
    
    return nil, nil, ErrCodecChainMismatch
}
```

### 4.2 ChannelPool（信道池）

**职责**：
- 管理多个 Channel 实例
- 提供每个 Channel 的 MTU 信息
- 评估信道健康度（用于 NAK 选择）
- 随机分发切片

**接口定义**：

```go
type ChannelInfo struct {
    Channel      Channel
    MTU          int           // 默认或用户配置
    HealthScore  float64       // 0.0 ~ 1.0
    SendCount    int64
    ErrorCount   int64
    LastActivity time.Time
}

// 核心方法
func (p *ChannelPool) GetHealthyChannel() *ChannelInfo  // 用于 NAK
func (p *ChannelPool) RandomSelect() *ChannelInfo       // 用于切片分发
func (p *ChannelPool) GetAdaptiveMTU() int              // 建议切片大小
```

**MTU 配置优先级**：
```
用户配置 > Channel.DefaultMTU() > 全局默认值(1024)
```

**健康度评估算法**：

```go
func (e *HealthEvaluator) Evaluate(info *ChannelInfo) float64 {
    // 1. 计算错误率
    errorRate := float64(info.ErrorCount) / float64(info.SendCount + 1)
    errorScore := 1.0 - errorRate
    
    // 2. 计算延迟得分
    latency := time.Since(info.LastActivity)
    latencyScore := 1.0 - min(latency/timeout, 1.0)
    
    // 3. 综合得分
    return errorScore*e.ErrorWeight + latencyScore*e.LatencyWeight
}
```

### 4.3 FragmentManager（切片管理器）

**职责**：
- 自适应切片（根据 ChannelPool 的 MTU）
- 切片元数据附加
- 接收端重组管理
- 缺失分片检测

**关键数据结构**：

```go
// 发送端缓存
type SendBuffer struct {
    SessionID    string
    OriginalData []byte
    Fragments    [][]byte
    CodecChain   []string    // 记录选定的代号组合
    CodecHash    [32]byte
    SentTime     time.Time
    Retransmit   int
    Complete     bool
}

// 接收端缓存
type RecvBuffer struct {
    SessionID    string
    Total        uint16
    Received     map[uint16][]byte
    Missing      []uint16
    CodecDepth   uint8
    CodecHash    [32]byte
    StartTime    time.Time
    Complete     bool
}
```

### 4.4 SessionManager（会话管理器）

**职责**：
- Session 创建与销毁
- Session 生命周期管理
- 与 FragmentManager 协同管理 Buffer

**生命周期**：

```
Send(data)
  │
  ├─→ 创建 Session（UUID）
  ├─→ 创建 SendBuffer（保留原始数据）
  ├─→ 选择 Codec 链（随机，记录代号组合）
  ├─→ 切片分发（多 Channel 随机发送）
  │
  │  (接收端处理)
  │
  ├─→ 接收分片
  ├─→ 检测缺失 → 发送 NAK
  ├─→ 重组解码
  ├─→ 发送 END_ACK
  │
  └─→ 销毁 Session（发送端收到 END_ACK）
      清理 Buffer
```

---

## 5. 数据流详细设计

### 5.1 发送流程

```
原始数据 ([]byte)
    │
    ▼
┌──────────────────────────────────────────────────────────────┐
│ Step 1: CodecManager.RandomSelect(depth)                     │
│   - 从协商后的代号池随机选择 codec 组合                        │
│   - 例如: ["A", "B"] 表示 AES → Base64                       │
│   - 计算 hash = SHA256("AB")                                 │
│   - 记录到 sessionChain[sessionID]                           │
└──────────────────────────────────────────────────────────────┘
    │
    ▼
┌──────────────────────────────────────────────────────────────┐
│ Step 2: CodecChain.Encode(data)                              │
│   - data → Codec[A].Encode → Codec[B].Encode → encodedData  │
└──────────────────────────────────────────────────────────────┘
    │
    ▼ encodedData
    │
┌──────────────────────────────────────────────────────────────┐
│ Step 3: FragmentManager.AdaptiveSplit(encodedData)           │
│   - 获取 ChannelPool 的建议 MTU                              │
│   - 根据 MTU 切片: [frag0, frag1, frag2, ...]               │
│   - 每个切片附加 Metadata Header                             │
└──────────────────────────────────────────────────────────────┘
    │
    ▼ [fragments with headers]
    │
┌──────────────────────────────────────────────────────────────┐
│ Step 4: ChannelPool.DistributeFragments(fragments)           │
│   - 随机选择 Channel 发送每个切片                            │
│   - frag0 → Channel[UDP]                                    │
│   - frag1 → Channel[TCP]                                    │
│   - frag2 → Channel[DNS]                                    │
└──────────────────────────────────────────────────────────────┘
    │
    ▼ 各信道传输
```

### 5.2 接收流程

```
各信道接收数据
    │
    ▼
┌──────────────────────────────────────────────────────────────┐
│ Step 1: 提取 Metadata                                        │
│   - 解析 Header: SessionID, FragmentIndex, CodecHash, etc.  │
│   - 根据 SessionID 创建/更新 RecvBuffer                      │
└──────────────────────────────────────────────────────────────┘
    │
    ▼
┌──────────────────────────────────────────────────────────────┐
│ Step 2: 检测缺失分片                                          │
│   - 检查 RecvBuffer[sessionID].Missing                      │
│   - 如果有缺失 → 通过健康信道发送 NAK                         │
│   - NAK 格式: {SessionID, MissingIndices: [3, 7]}           │
└──────────────────────────────────────────────────────────────┘
    │
    ▼ (所有分片到达后)
    │
┌──────────────────────────────────────────────────────────────┐
│ Step 3: CodecManager.MatchByHash(hash, depth)               │
│   - 生成所有可能的代号排列组合                                │
│   - 计算每个组合的 hash                                       │
│   - 匹配接收到的 hash                                         │
│   - 得到解码链代号组合: ["A", "B"]                           │
└──────────────────────────────────────────────────────────────┘
    │
    ▼
┌──────────────────────────────────────────────────────────────┐
│ Step 4: 重组 + 解码                                          │
│   - 按 Index 顺序重组分片 → encodedData                      │
│   - CodecChain.Decode(encodedData)                          │
│   - encodedData → Codec[B].Decode → Codec[A].Decode → data  │
└──────────────────────────────────────────────────────────────┘
    │
    ▼ 原始数据
    │
┌──────────────────────────────────────────────────────────────┐
│ Step 5: 发送 END_ACK                                         │
│   - 通过健康信道发送: {SessionID, Status: "COMPLETE"}        │
└──────────────────────────────────────────────────────────────┘
    │
    ▼ 返回给用户 Receive()
```

---

## 6. Metadata 协议设计

### 6.1 分片 Header 结构

```go
type FragmentMetadata struct {
    // === 核心字段（必须） ===
    SessionID     string    // UUID，标识本次发送
    FragmentIndex uint16    // 分片序号（0-based）
    FragmentTotal uint16    // 总分片数
    
    // === Codec 信息 ===
    CodecDepth    uint8     // Codec 链深度（层数）
    CodecHash     [32]byte  // SHA256(代号组合)
    
    // === 校验字段 ===
    DataChecksum  uint32    // CRC32(分片数据)
    
    // === 时间戳（可选） ===
    Timestamp     int64     // 发送时间，用于超时判断
    
    // === 标志位 ===
    IsLast        bool      // 是否最后一个分片
}
```

### 6.2 二进制编码格式

```
┌─────────────────────────────────────────────────────────────────┐
│                    Fragment Packet 格式                          │
├─────────────────────────────────────────────────────────────────┤
│ [HeaderLen:4][Header][Data]                                     │
│                                                                  │
│ Header:                                                          │
│ ┌─────────────────────────────────────────────────────────────┐│
│ │ [Version:1]                                                   ││
│ │ [CodecDepth:1]                                                ││
│ │ [Flags:1]         (IsLast等标志位)                            ││
│ │ [FragmentIndex:2]                                             ││
│ │ [FragmentTotal:2]                                             ││
│ │ [Timestamp:8]                                                 ││
│ │ [DataChecksum:4]                                              ││
│ │ [SessionIDLen:2][SessionID:变长]                              ││
│ │ [CodecHash:32]     (固定32字节SHA256)                         ││
│ └─────────────────────────────────────────────────────────────┘│
│                                                                  │
│ Data:                                                            │
│ ┌─────────────────────────────────────────────────────────────┐│
│ │ [分片数据，大小自适应于Channel MTU]                           ││
│ └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

### 6.3 NAK 消息格式

```go
type NAKMessage struct {
    SessionID      string
    MissingIndices []uint16    // 缺失的分片索引列表
    Timestamp      int64
}
```

### 6.4 END_ACK 消息格式

```go
type EndAckMessage struct {
    SessionID  string
    Status     string    // "COMPLETE"
    Timestamp  int64
}
```

---

## 7. 能力协商协议

### 7.1 协商流程

```
    Client                              Server
      │                                    │
      │─── NegotiationRequest ────────────→│
      │    {SupportedCodes: ["A","B","X"]} │
      │    {MaxDepth: 2}                   │
      │                                    │
      │←── NegotiationResponse ───────────│
      │    {Accepted: true}                │
      │    {SupportedCodes: ["A","B"]}     │  ← 确定共同支持的代号
      │    {MaxDepth: 2}                   │
      │                                    │
```

### 7.2 协商数据结构

```go
type NegotiationRequest struct {
    ClientID       string
    SupportedCodes []string    // 支持的 Codec 代号列表
    MaxDepth       int         // 支持的最大链深度
    Timestamp      int64
}

type NegotiationResponse struct {
    Accepted       bool
    RejectReason   string
    SupportedCodes []string    // 双方共同支持的代号
    MaxDepth       int         // 协商后的最大深度
    Timestamp      int64
}
```

---

## 8. 用户 API 设计

### 8.1 核心 API（net.Conn/net.Listener 风格）

VoidBus v3.0 采用 Go 标准库的 `net.Conn` 和 `net.Listener` 风格 API，提供消息式的通信接口。

```go
// === Client 模式 ===
bus := voidbus.New(nil)

// 注册 Codec
bus.RegisterCodec(base64.New())
bus.RegisterCodec(xor.New())

// 添加 Channel（配置包含目标地址）
ch := tcp.NewClientChannel(&tcp.ClientConfig{Address: "server:8080"})
bus.AddChannel(ch)

// Dial 连接（自动执行协商）
conn, err := bus.Dial(ch)  // 返回 net.Conn

// 消息式通信
conn.Write([]byte("hello world"))      // 发送一条完整消息
buf := make([]byte, 4096)
n, err := conn.Read(buf)               // 接收一条完整消息

conn.Close()                           // 关闭连接

// === Server 模式 ===
bus := voidbus.New(nil)
bus.RegisterCodec(base64.New())

// 添加 Server Channel（配置包含监听地址）
serverCh := tcp.NewServerChannel(&tcp.ServerConfig{Address: ":8080"})
bus.AddChannel(serverCh)

// Listen 监听
listener, err := bus.Listen(serverCh)  // 返回 net.Listener

for {
    conn, err := listener.Accept()     // 接受新客户端连接（已协商）
    
    go func(c net.Conn) {
        buf := make([]byte, 4096)
        n, _ := c.Read(buf)            // 接收一条完整消息
        c.Write([]byte("echo"))        // 发送一条完整消息
        c.Close()
    }(conn)
}
```

### 8.2 消息语义

| 方法 | 语义 | 说明 |
|------|------|------|
| `Read(buf)` | 返回一条完整消息 | 自动重组、解码，每次调用返回一条消息 |
| `Write(data)` | 写入一条完整消息 | 自动编码、分片、多 Channel 分发 |
| `SetDeadline` | 整条消息的超时 | 重组/编码/发送的总超时 |
| `LocalAddr` | 返回本地地址 | 已注册 Channel 的地址 |
| `RemoteAddr` | 返回对端地址 | Dial 时 Channel 的目标地址 |

### 8.3 内部流程

**Dial 流程**:
```
bus.Dial(ch)
    │
    ├── 1. CreateNegotiateRequest (从注册的 codecs 生成 Bitmap)
    ├── 2. Send NegotiateRequest through Channel
    ├── 3. Receive NegotiateResponse
    ├── 4. ApplyNegotiateResponse (设置协商后的 codecs)
    ├── 5. StartReceiveLoop (后台接收循环)
    └── 6. Return net.Conn
```

**Listen 流程**:
```
bus.Listen(serverCh)
    │
    ├── 1. Mark as running
    └── 2. Return net.Listener
    
listener.Accept()
    │
    ├── 1. Accept new client connection
    ├── 2. Receive NegotiateRequest
    ├── 3. HandleNegotiateRequest (生成 NegotiateResponse)
    ├── 4. Send NegotiateResponse
    ├── 5. Create clientBus with negotiated codecs
    ├── 6. Start clientBus receive loop
    └── 7. Return net.Conn (for this client)
```

### 8.4 配置结构

```go
type BusConfig struct {
    MaxCodecDepth     int           // 最大链深度（默认 2）
    DefaultMTU        int           // 默认 MTU（默认 1024）
    RecvBufferSize    int           // 接收队列大小（默认 100）
    DebugMode         bool          // 调试模式
}
```

---

## 9. 目录结构

```
VoidBus/
├── bus.go                    # 核心 Bus 实现（Dial/Listen API）
├── addr.go                   # voidBusAddr 实现 net.Addr
├── conn.go                   # voidBusConn 实现 net.Conn（消息式 Read/Write）
├── listener.go               # voidBusListener 实现 net.Listener
├── config.go                 # BusConfig 配置结构
├── errors.go                 # 全局错误定义
├── module.go                 # Module 接口抽象
│
├── codec/                    # 编解码模块
│   ├── manager.go            # CodecManager（随机选择+Hash匹配）
│   ├── chain.go              # CodecChain（链式编解码）
│   ├── interface.go          # Codec 接口定义
│   ├── codec.go              # Codec 基础实现
│   ├── aes/                  # AES 实现
│   ├── base64/               # Base64 实现
│   ├── plain/                # Plain 实现（调试用）
│   ├── xor/                  # XOR 实现
│   ├── chacha20/             # ChaCha20 实现
│   └── rsa/                  # RSA 实现
│
├── channel/                  # 信道模块
│   ├── pool.go               # ChannelPool（MTU+健康度）+ HealthEvaluator
│   ├── interface.go          # Channel 接口（含 DefaultMTU, ServerChannel）
│   ├── tcp/                  # TCP Channel
│   ├── udp/                  # UDP Channel
│   ├── dns/                  # DNS Channel（低MTU示例）
│   └── ws/                   # WebSocket Channel
│
├── fragment/                 # 切片模块
│   ├── manager.go            # FragmentManager（自适应切片+重组）
│   ├── buffer.go             # SendBuffer/RecvBuffer 定义
│   └── errors.go             # Fragment 错误定义
│
├── session/                  # 会话模块
│   ├── manager.go            # SessionManager（生命周期）
│   └── session.go            # Session 结构定义
│
├── protocol/                 # 协议层
│   └── header.go             # Header 编解码 + NAK/END_ACK 消息
│
├── negotiate/                # 协商模块
│   ├── negotiate.go          # NegotiateRequest/Response 编解码
│   ├── server.go             # ServerNegotiator
│   └── bitmap.go             # Bitmap 操作
│
├── internal/                 # 内部工具
│   ├── hash.go               # Hash 计算（SHA256）+ HashCache
│   ├── id.go                 # UUID 生成
│   ├── checksum.go           # CRC32 校验
│   ├── timer.go              # 自适应超时（RFC 6298）
│   ├── permutation.go        # 排列组合生成器
│   └── crypto.go             # 加密工具
│
├── keyprovider/              # 密钥提供者
│   ├── keyprovider.go        # KeyProvider 接口
│   └── embedded/             # 嵌入式密钥提供者
│
└── example/                  # 使用示例
    └── interactive/          # 交互式 Client/Server 示例
        ├── client/main.go    # Client 示例
        ├── server/main.go    # Server 示例
        └── README.md         # 示例文档
```

---

## 10. 实现优先级

### Phase 1: 核心框架（必须最先）
1. `errors.go` - 全局错误定义
2. `config.go` - BusConfig 结构
3. `internal/hash.go` - Hash 计算
4. `internal/id.go` - UUID/SessionID 生成
5. `internal/checksum.go` - CRC32 校验
6. `internal/permutation.go` - 排列组合生成器

### Phase 2: 协议层
1. `protocol/header.go` - Header 编解码
2. `protocol/metadata.go` - FragmentMetadata 结构
3. `protocol/packet.go` - Packet 编解码
4. `protocol/nak.go` - NAK 消息
5. `protocol/end.go` - END_ACK 消息
6. `protocol/negotiation.go` - 能力协商协议

### Phase 3: Codec 模块
1. `codec/interface.go` - Codec 接口定义
2. `codec/chain.go` - CodecChain 实现
3. `codec/registry.go` - Codec 注册表
4. `codec/manager.go` - CodecManager（核心：随机选择+Hash匹配）
5. `codec/plain/plain.go` - Plain Codec（调试用）
6. `codec/aes/aes.go` - AES-GCM Codec
7. `codec/base64/base64.go` - Base64 Codec

### Phase 4: Channel 模块
1. `channel/interface.go` - Channel 接口（含 DefaultMTU）
2. `channel/pool.go` - ChannelPool 实现
3. `channel/health.go` - HealthEvaluator
4. `channel/tcp/tcp.go` - TCP Channel

### Phase 5: Fragment 模块
1. `fragment/metadata.go` - FragmentMetadata
2. `fragment/buffer.go` - SendBuffer/RecvBuffer
3. `fragment/splitter.go` - 切片算法
4. `fragment/manager.go` - FragmentManager

### Phase 6: Session 模块
1. `session/session.go` - Session 结构
2. `session/manager.go` - SessionManager

### Phase 7: 核心 Bus
1. `bus.go` - Bus 核心实现（整合所有模块）

### Phase 8: 示例与测试
1. `examples/basic.go` - 基础使用示例
2. 单元测试
3. 集成测试

---

## 11. 质量保证

### 11.1 测试策略
- 每个模块独立单元测试
- 排列组合匹配算法测试
- 多信道分发测试
- 重传机制测试
- 健康度评估测试
- 压力测试验证性能

### 11.2 代码规范
- 遵循 Go 标准代码规范
- 使用 golangci-lint 进行静态检查
- 接口注释完整
- 示例代码可运行

### 11.3 性能要求
- Codec 层：编解码延迟 <10ms (1MB 数据)
- Channel 层：支持至少 1Gbps 吞吐量
- Fragment 层：分片/重组延迟 <5ms
- Hash 匹配：depth=3 时排列组合数 ≤ n³，需优化

---

## 12. 安全验证设计

### 12.1 Protocol Header 安全验证

VoidBus v2.0 在 `DecodeHeader` 中实现了完整的安全验证，防止多种攻击：

**验证常量定义**：

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

**验证流程**：

| 验证项 | 防护目标 | 失败处理 |
|--------|----------|----------|
| PacketSize | 防止过大/过小包攻击 | 返回 V2ValidationError |
| SessionID长度 | 防止内存耗尽攻击 | 返回 V2ValidationError |
| FragmentTotal上限 | 防止资源耗尽 | 返回 V2ValidationError |
| FragmentIndex范围 | 防止越界访问 | 返回 V2ValidationError |
| CodecDepth范围 | 防止深度溢出 | 返回 V2ValidationError |
| Timestamp防重放 | 防止重放攻击 | 返回 V2ValidationError |
| Flags合法性 | 防止未知标志注入 | 返回 V2ValidationError |

**V2ValidationError 结构**：

```go
type V2ValidationError struct {
    Field    string      // 失败字段名
    Actual   interface{} // 实际值
    Expected interface{} // 期望值
    Msg      string      // 错误消息
}
```

### 12.2 Timestamp 验证逻辑

```go
now := time.Now().Unix()
age := now - header.Timestamp

// 防止重放攻击：拒绝超过1小时的包
if age > MaxTimestampAge {
    return nil, nil, NewValidationError("Timestamp", "packet too old", age, "<= 3600")
}

// 允许时钟偏差：接受未来5分钟的包
if age < MinTimestampAge {
    return nil, nil, NewValidationError("Timestamp", "timestamp in future", age, ">= -300")
}
```

---

## 13. 统一错误处理策略

### 13.1 错误严重程度

VoidBus v2.0 引入了错误严重程度分级：

```go
type ErrorSeverity int

const (
    SeverityLow      ErrorSeverity = iota // 可恢复，不影响主流程
    SeverityMedium                         // 需处理，可能影响部分功能
    SeverityHigh                           // 严重影响，主要功能受阻
    SeverityCritical                       // 致命错误，无法继续运行
)
```

### 13.2 EnhancedVoidBusError

增强错误类型支持严重程度和上下文信息：

```go
type EnhancedVoidBusError struct {
    *VoidBusError
    Severity    ErrorSeverity
    Recoverable bool
    Context     map[string]interface{}
}

func (e *EnhancedVoidBusError) Error() string {
    return fmt.Sprintf("[%s/%s] %s (severity: %s, recoverable: %v)",
        e.Module, e.Operation, e.Message, e.Severity.String(), e.Recoverable)
}
```

### 13.3 统一错误包装函数

| 函数 | 用途 | 严重程度 |
|------|------|----------|
| `MustWrap(op, module, err)` | 关键路径强制包装 | SeverityHigh |
| `SoftWrap(op, module, err)` | 可选路径软包装 | SeverityLow |
| `RecoverableError(...)` | 可恢复错误 | SeverityMedium + Recoverable=true |
| `CriticalError(...)` | 致命错误 | SeverityCritical |
| `WrapWithContext(...)` | 带上下文包装 | SeverityMedium |

### 13.4 错误辅助函数

```go
// 检查是否为增强错误
func IsEnhancedError(err error) bool

// 获取错误严重程度
func GetSeverity(err error) ErrorSeverity

// 检查是否可恢复
func IsRecoverable(err error) bool

// 检查是否为致命错误
func IsCritical(err error) bool

// 检查是否为高严重程度
func IsHighSeverity(err error) bool
```