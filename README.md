# VoidBus

VoidBus 是一个高度模块化、可组合的隐蔽通信总线库，实现信道与编解码的完全分离，支持任意组合和更换。

## 核心特性

- **三层分离架构**：Codec（编解码）+ Channel（信道）+ Fragment（分片）- 用户自行序列化
- **Codec链式组合**：支持多个Codec按顺序组合，用户自定义代号标识
- **可插拔架构**：所有模块通过接口定义，支持自定义实现
- **双向全双工通信**：Server侧可同时向多个客户端接收和发送信息
- **分片多信道传输**：支持数据分片，通过不同信道/编码组合发送
- **隐蔽信道设计**：支持WebSocket（默认）、TCP、UDP等多种信道
- **Bitmap协商协议**：二进制格式协商可用信道和Codec（非明文）
- **信道健康度评估**：基于健康度加权随机选择信道，故障自动切换
- **可靠/不可靠信道区分**：可靠信道信任协议，不可靠信道实现ACK/NAK重传
- **协议安全验证**：Header完整安全验证，防止资源耗尽和重放攻击
- **统一错误处理**：增强错误类型，支持严重程度和上下文信息

## 目录结构

```
VoidBus/
├── bus.go              # Bus核心实现（统一入口）
├── module.go           # Module接口定义
├── config.go           # BusConfig配置
├── errors.go           # 统一错误定义（含EnhancedVoidBusError）
│
├── negotiate/          # 协商模块 [隐蔽信道核心]
│   ├── interface.go    # Negotiator接口定义
│   ├── frame.go        # NegotiateRequest/Response帧编解码
│   ├── codec_bitmap.go # Codec Bitmap定义
│   ├── channel_bitmap.go # Channel Bitmap定义
│   ├── client_negotiator.go # Client协商器
│   ├── server_negotiator.go # Server协商器 + SessionManager
│   └── negotiate_test.go # 协商模块测试
│
├── protocol/           # 协议层
│   ├── header.go       # Header结构 + 安全验证
│   └── header_test.go  # Header安全验证测试
│
├── codec/              # 编解码模块 [不可暴露]
│   ├── interface.go    # Codec接口定义 + Code()方法
│   ├── manager.go      # CodecManager（用户自定义代号）
│   ├── chain.go        # CodecChain实现
│   ├── chain_test.go   # CodecChain测试
│   ├── plain/          # Pass-through（仅调试）
│   ├── base64/         # Base64编码
│   ├── aes/            # AES-GCM加密
│   ├── xor/            # XOR编码
│   ├── chacha20/       # ChaCha20-Poly1305加密
│   └── rsa/            # RSA-OAEP加密
│
├── channel/            # 信道模块 [不可暴露]
│   ├── interface.go    # Channel接口定义 + IsReliable()
│   ├── pool.go         # ChannelPool（健康度加权随机选择）
│   ├── tcp/            # TCP传输（可靠）
│   ├── ws/             # WebSocket传输（可靠，默认协商信道）
│   └── udp/            # UDP传输（不可靠，ACK/NAK重传）
│
├── fragment/           # 分片模块
│   ├── manager.go      # FragmentManager
│   └── buffer.go       # SendBuffer/RecvBuffer
│
├── session/            # Session模块
│   ├── manager.go      # SessionManager
│   └── session.go      # Session定义
│
├── keyprovider/        # 密钥提供者 [不可暴露]
│   └── embedded/       # 编译时嵌入密钥
│
├── internal/           # 内部工具（不对外暴露）
│   ├── hash.go         # Hash计算 + HashCache
│   ├── id.go           # ID生成 + RandomIntRange
│   ├── checksum.go     # CRC16/CRC32校验
│   └── *_test.go       # 内部工具测试
│
├── tests/              # 测试归档目录
│   ├── mock/           # Mock实现（依赖注入测试）
│   │   └ mocks.go      # MockCodecManager/MockFragmentManager等
│   └ README.md         # 测试说明文档
│
├── docs/               # 文档
│   ├── ARCHITECTURE.md # 架构设计文档
│   └── INTERFACE.md    # 接口详细说明
│
├── bus_test.go         # Bus核心测试
├── errors_test.go      # 错误处理测试
├── benchmark_test.go   # 性能基准测试（19 benchmarks）
└── README.md           # 项目说明
```

## 安全边界

| 模块 | 可暴露性 | 说明 |
|------|----------|------|
| Codec | ❌ 不可暴露 | 编解码方式不可暴露，仅通过CodecHash间接引用 |
| Channel | ❌ 不可暴露 | 信道类型不可暴露 |
| KeyProvider | ❌ 不可暴露 | 密钥相关信息不可暴露 |
| Codec Hash | ✅ 可暴露 | SHA256(代号组合)，不暴露具体组合 |

## 数据流

### 协商流程
```
Client通过默认信道（WebSocket）发送NegotiateRequest
  → Server计算交集（Channel Bitmap & Codec Bitmap）
  → Server返回NegotiateResponse（可用信道 + Codec + SessionID）
  → 双方基于Bitmap动态组合Codec链
```

### 发送流程
```
原始数据（用户自行序列化）
  → CodecManager.SelectChain() → 随机选择Codec组合（用户自定义代号）
  → CodecChain.Encode() → 编码/加密数据
  → FragmentManager.AdaptiveSplit() → 分片数据（自适应MTU）
  → ChannelPool.SelectChannel() → 健康度加权随机选择
  → Channel.Send() → 网络传输
    ├─ 可靠信道（TCP/WS）: 信任协议可靠性
    └─ 不可靠信道（UDP）: ACK/NAK重传机制
```

### 接收流程
```
Channel.Receive() → 原始网络数据
  → DecodeHeader() → 安全验证 + 解析Header
  → CodecManager.MatchChain(Hash) → 匹配Codec组合
  → FragmentManager.AddFragment() → 分片缓存
  → FragmentManager.Reassemble() → 完整数据
  → CodecChain.Decode() → 解码数据
  → 用户自行反序列化 → 原始数据
```

### 故障切换
```
Channel.Send()超时3s无ACK
  → ChannelPool.MarkUnavailable(chType)
  → FragmentManager.GetPendingFragments()
  → ChannelPool.SelectChannel(exclude=[不可用])
  → 新信道重新发送
```

## 协商协议

VoidBus v2.1 使用二进制Bitmap格式协商（非明文）：

### NegotiateRequest帧格式
```
[1 byte: Magic 0x56] [1 byte: Version]
[1 byte: ChCount] [N bytes: ChannelBitmap]
[1 byte: CodecCount] [N bytes: CodecBitmap]
[8 bytes: Nonce] [4 bytes: Timestamp]
[1 byte: PaddingLen] [M bytes: Padding]
[2 bytes: CRC16]
```

### NegotiateResponse帧格式
```
[1 byte: Magic 0x42] [1 byte: Version]
[1 byte: ChCount] [N bytes: ChannelBitmap]
[1 byte: CodecCount] [N bytes: CodecBitmap]
[8 bytes: SessionID] [1 byte: Status]
[1 byte: PaddingLen] [M bytes: Padding]
[2 bytes: CRC16]
```

### Channel可靠性
| Channel | IsReliable | 说明 |
|---------|------------|------|
| WebSocket | ✅ | 默认协商信道，易穿透防火墙 |
| TCP | ✅ | 可靠传输 |
| UDP | ❌ | 需ACK/NAK重传（3s超时） |
| ICMP | ❌ | 需可靠重传 |
| DNS | ❌ | 需可靠重传 |

## 安全验证

VoidBus v2.0 在 Protocol Header 层面实现了完整的安全验证：

| 验证项 | 限制 | 说明 |
|--------|------|------|
| PacketSize | 84-65535字节 | 防止过大/过小包 |
| SessionID | 1-64字符 | 防止内存耗尽 |
| FragmentTotal | ≤10000 | 防止资源耗尽 |
| CodecDepth | 1-5 | 防止深度溢出 |
| Timestamp | ±1小时 | 防止重放攻击 |
| Flags | 仅允许已知位 | 防止未知标志 |

## 错误处理

VoidBus v2.0 实现了统一的错误处理策略：

```go
// 错误严重程度
type ErrorSeverity int
const (
    SeverityLow      // 可恢复
    SeverityMedium   // 需处理  
    SeverityHigh     // 严重影响
    SeverityCritical // 致命错误
)

// 增强错误类型
type EnhancedVoidBusError struct {
    *VoidBusError
    Severity    ErrorSeverity
    Recoverable bool
    Context     map[string]interface{}
}

// 统一错误包装函数
MustWrap(op, module, err)      // 关键路径
SoftWrap(op, module, err)      // 可选路径
RecoverableError(...)          // 可恢复错误
CriticalError(...)             // 致命错误
```

## 快速开始

### 基本使用

```go
import (
    "github.com/Script-OS/VoidBus"
    "github.com/Script-OS/VoidBus/codec/aes"
    "github.com/Script-OS/VoidBus/codec/base64"
    "github.com/Script-OS/VoidBus/channel/ws"
    "github.com/Script-OS/VoidBus/channel/tcp"
    "github.com/Script-OS/VoidBus/negotiate"
)

func main() {
    // 1. 创建Bus
    bus, err := VoidBus.New()
    if err != nil {
        panic(err)
    }
    defer bus.Stop()

    // 2. 设置密钥
    key := []byte("32-byte-secret-key-for-aes-256!!")
    if err := bus.SetKey(key); err != nil {
        panic(err)
    }

    // 3. 注册Codec（用户自定义代号，需收发双端一致）
    aesCodec := aes.NewAES256Codec()
    aesCodec.SetCode("aes")  // 自定义代号
    bus.RegisterCodec(aesCodec)

    base64Codec := base64.New()
    base64Codec.SetCode("b64")  // 自定义代号
    bus.RegisterCodec(base64Codec)

    // 4. 注册Channel
    bus.RegisterChannel(ws.NewModule())   // WebSocket（默认协商信道）
    bus.RegisterChannel(tcp.NewModule())  // TCP

    // 5. 创建协商请求（Bitmap自动生成）
    request, err := bus.CreateNegotiateRequest()
    if err != nil {
        panic(err)
    }
    
    // 6. 编码并发送协商请求（通过WebSocket）
    encoded, err := request.Encode()
    if err != nil {
        panic(err)
    }
    // ... 通过WebSocket发送encoded到Server ...
    
    // 7. 接收并应用Server响应
    // response, _ := negotiate.DecodeNegotiateResponse(serverData)
    // bus.ApplyNegotiateResponse(response)

    // 8. 发送数据（用户自行序列化）
    data := []byte("Hello, VoidBus!")  // 或 JSON/Protobuf 序列化
    bus.Send(data)
}
```

### 自动Bitmap生成

协商时，Bitmap**自动**从注册的Codec和Channel生成，用户无需手动设置：

```go
// 注册Codec后，CodecBitmap自动包含对应的bit
bus.RegisterCodec(aesCodec)     // 自动设置CodecBitAES256
bus.RegisterCodec(base64Codec)  // 自动设置CodecBitBase64

// 注册Channel后，ChannelBitmap自动包含对应的bit  
bus.RegisterChannel(ws.NewModule())   // 自动设置ChannelBitWS
bus.RegisterChannel(tcp.NewModule())  // 自动设置ChannelBitTCP

// 创建协商请求时，Bitmap自动生成
request, _ := bus.CreateNegotiateRequest()
// request.ChannelBitmap 和 request.CodecBitmap 已自动填充
```

## 安全等级

| 等级 | 值 | 示例 |
|------|----|----|
| None | 0 | Plain Codec（仅调试模式） |
| Low | 1 | XOR, Base64编码 |
| Medium | 2 | AES-128-GCM, ChaCha20 |
| High | 3 | AES-256-GCM, RSA |

**Release模式**: 最小安全等级为 Medium，禁止使用 Plain Codec。

## 测试覆盖率

| 模块 | 覆盖率 | 说明 |
|------|--------|------|
| bus.go | 32.5% | 核心入口测试 |
| protocol/header.go | 89.3% | 安全验证测试 |
| negotiate | 79.5% | 协商协议测试（64个测试用例） |
| errors.go | 高 | 错误处理测试 |
| codec/aes | 81.7% | AES编解码测试 |
| codec/base64 | 95.2% | Base64编解码测试 |
| codec/plain | 94.7% | Plain编解码测试 |
| channel/ws | 高 | WebSocket信道测试 |
| channel/udp | 高 | UDP可靠重传测试 |

## 模块文档

- [negotiate/](negotiate/README.md) - 协商模块（Bitmap协议）
- [codec/](codec/README.md) - 编解码模块
- [channel/](channel/README.md) - 信道模块
- [fragment/](fragment/README.md) - 分片模块
- [session/](session/README.md) - Session模块
- [protocol/](protocol/README.md) - 协议层
- [keyprovider/](keyprovider/embedded/README.md) - 密钥提供者
- [tests/](tests/README.md) - 测试说明

## 详细文档

- [架构设计文档](docs/ARCHITECTURE.md)
- [接口详细说明](docs/INTERFACE.md)

## 编译与测试

```bash
# 编译所有模块
go build ./...

# 运行所有测试
go test ./...

# 运行测试并显示覆盖率
go test -cover ./...

# 运行性能基准测试
go test -bench=. -benchmem ./...

# 运行特定模块测试
go test -v ./protocol/...
```

## 许可证

MIT License