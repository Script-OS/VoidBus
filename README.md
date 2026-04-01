# VoidBus

VoidBus 是一个高度模块化、可组合的通信总线库，实现信道与编解码的完全分离，支持任意组合和更换。

## 核心特性

- **三层分离架构**：Codec（编解码）+ Channel（信道）+ Fragment（分片）- 用户自行序列化
- **Codec链式组合**：支持多个Codec按顺序组合，如 AES → Base64
- **可插拔架构**：所有模块通过接口定义，支持自定义实现
- **双向全双工通信**：Server侧可同时向多个客户端接收和发送信息
- **分片多信道传输**：支持数据分片，通过不同信道/编码组合发送
- **安全协商机制**：防止降级攻击，Release模式禁用plaintext
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
├── protocol/           # 协议层
│   ├── header.go       # Header结构 + 安全验证
│   └── header_test.go  # Header安全验证测试
│
├── codec/              # 编解码模块 [不可暴露]
│   ├── interface.go    # Codec接口定义
│   ├── manager.go      # CodecManager
│   ├── chain.go        # CodecChain实现
│   ├── chain_test.go   # CodecChain测试
│   ├── plain/          # Pass-through（仅调试）
│   ├── base64/         # Base64编码
│   ├── aes/            # AES-GCM加密
│   ├── xor/            # XOR编码
│   ├── chacha20/       # ChaCha20加密
│   └── rsa/            # RSA加密
│
├── channel/            # 信道模块 [不可暴露]
│   ├── interface.go    # Channel接口定义
│   ├── pool.go         # ChannelPool
│   └── tcp/            # TCP传输实现
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
│   ├── id.go           # ID生成
│   ├── checksum.go     # CRC32校验
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

### 发送流程
```
原始数据（用户自行序列化）
  → CodecChain.Encode() → 编码/加密数据
  → FragmentManager.AdaptiveSplit() → 分片数据（自适应MTU）
  → ChannelPool.RandomChannel() → 随机信道选择
  → Channel.Send() → 网络传输
```

### 接收流程
```
Channel.Receive() → 原始网络数据
  → DecodeHeader() → 安全验证 + 解析Header
  → FragmentManager.AddFragment() → 分片缓存
  → FragmentManager.Reassemble() → 完整数据
  → CodecChain.Decode() → 解码数据
  → 用户自行反序列化 → 原始数据
```

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
    "github.com/Script-OS/VoidBus/codec/plain"
)

func main() {
    // 创建Bus（返回error）
    bus, err := VoidBus.New()
    if err != nil {
        panic(err)
    }
    defer bus.Stop()

    // 设置密钥（返回error）
    key := []byte("32-byte-secret-key-for-aes-256!!")
    err = bus.SetKey(key)
    if err != nil {
        panic(err)
    }

    // 添加Codec（用户自定义代号）
    bus.AddCodec(aes.NewAES256Codec(), "A")  // AES-256-GCM
    bus.AddCodec(base64.New(), "B")          // Base64

    // 设置最大链深度
    bus.SetMaxCodecDepth(2)

    // 添加信道
    bus.AddChannel(tcp.NewClientChannel("server:8080"))

    // 发送数据（用户自行序列化）
    data := []byte("Hello, VoidBus!")  // 或 JSON/Protobuf 序列化
    bus.Send(data)
}
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
| errors.go | 高 | 错误处理测试 |
| codec/aes | 81.7% | AES编解码测试 |
| codec/base64 | 95.2% | Base64编解码测试 |
| codec/plain | 94.7% | Plain编解码测试 |

## 模块文档

- [codec/](codec/README.md) - 编解码模块
- [channel/](channel/README.md) - 信道模块
- [fragment/](fragment/README.md) - 分片模块
- [session/](session/README.md) - Session模块
- [protocol/](protocol/README.md) - 协议层
- [keyprovider/](keyprovider/embedded/README.md) - 密钥提供者

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