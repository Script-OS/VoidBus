# Codec Package - 编解码模块

编解码模块负责数据的编码/加密和解码/解密，是VoidBus四层分离架构的第二层。

**安全边界**: ❌ 不可暴露 - Codec配置不通过网络传输，仅通过SessionID间接引用。

## 文件结构

```
codec/
├── interface.go      # Codec接口定义
├── codec.go          # CodecRegistry
├── chain.go          # CodecChain接口及实现
├── plain/            # Pass-through Codec（仅调试）
│   └── plain.go
├── base64/           # Base64编码
│   └── base64.go
└── aes/              # AES-GCM加密
│   └── aes.go
```

## 模块职责

### Codec接口

**职责**：
- 负责数据的编码/加密和解码/解密
- 提供安全等级标识（用于协商，不暴露具体名称）
- 支持密钥注入（通过KeyProvider）

**不负责**：
- 数据序列化（由Serializer负责）
- 数据传输（由Channel负责）
- 密钥获取（由KeyProvider提供）

### CodecChain

**职责**：
- 支持多个Codec的链式组合
- 管理Codec处理顺序
- 计算整体安全等级（取最低值，安全短板原则）

## 接口定义

```go
// Codec - 编解码器接口
type Codec interface {
    // Encode 编码/加密数据
    Encode(data []byte) ([]byte, error)
    
    // Decode 解码/解密数据
    Decode(data []byte) ([]byte, error)
    
    // InternalID 内部标识（不可传输）
    InternalID() string
    
    // SecurityLevel 安全等级
    SecurityLevel() SecurityLevel
}

// KeyAwareCodec - 需要密钥的Codec
type KeyAwareCodec interface {
    Codec
    
    // SetKeyProvider 设置密钥提供者
    SetKeyProvider(provider keyprovider.KeyProvider) error
    
    // RequiresKey 是否需要密钥
    RequiresKey() bool
    
    // KeyAlgorithm 密钥算法标识
    KeyAlgorithm() string
}

// CodecChain - Codec链接口
type CodecChain interface {
    // AddCodec 添加Codec
    AddCodec(codec Codec) CodecChain
    
    // Encode 按顺序编码：data → Codec[0].Encode → Codec[1].Encode → ... → output
    Encode(data []byte) ([]byte, error)
    
    // Decode 按逆序解码：data → Codec[n].Decode → ... → Codec[1].Decode → Codec[0].Decode → output
    Decode(data []byte) ([]byte, error)
    
    // SecurityLevel 返回链中最低安全等级（安全短板原则）
    SecurityLevel() SecurityLevel
    
    // Clone 克隆CodecChain
    Clone() CodecChain
    
    // Length 返回Codec数量
    Length() int
}
```

## 安全等级定义

```go
type SecurityLevel int

const (
    SecurityLevelNone   SecurityLevel = 0  // Plaintext（仅调试）
    SecurityLevelLow    SecurityLevel = 1  // Base64等无加密
    SecurityLevelMedium SecurityLevel = 2  // AES-128
    SecurityLevelHigh   SecurityLevel = 3  // AES-256, RSA
)
```

**Release模式**: MinSecurityLevel >= SecurityLevelMedium，禁止使用Plain Codec。

## 已实现模块

### Plain Codec

位置: `codec/plain/plain.go`

**特点**：
- Pass-through编解码器
- SecurityLevel = None (0)
- 仅用于调试模式
- Release模式禁用

```go
codec := plain.New()
codec.InternalID()        // "plain"
codec.SecurityLevel()     // SecurityLevelNone
codec.RequiresKey()       // false
```

### Base64 Codec

位置: `codec/base64/base64.go`

**特点**：
- Base64编码/解码
- SecurityLevel = Low (1)
- 无加密，仅编码
- 不需要密钥

```go
codec := base64.New()
codec.InternalID()        // "base64"
codec.SecurityLevel()     // SecurityLevelLow
codec.RequiresKey()       // false

// Encode: []byte("Hello") → "SGVsbG8="
encoded, err := codec.Encode([]byte("Hello"))

// Decode: "SGVsbG8=" → []byte("Hello")
decoded, err := codec.Decode(encoded)
```

### AES Codec

位置: `codec/aes/aes.go`

**特点**：
- AES-GCM认证加密
- 支持AES-128和AES-256
- SecurityLevel = Medium (AES-128) 或 High (AES-256)
- 需要密钥（KeyAwareCodec）
- 12字节随机Nonce，输出格式: nonce + ciphertext + tag

```go
// AES-128-GCM (16字节密钥)
codec := aes.NewAES128GCM()
codec.InternalID()        // "aes-128-gcm"
codec.SecurityLevel()     // SecurityLevelMedium
codec.RequiresKey()       // true
codec.KeyAlgorithm()      // "AES-128-GCM"

// AES-256-GCM (32字节密钥)
codec := aes.NewAES256GCM()
codec.InternalID()        // "aes-256-gcm"
codec.SecurityLevel()     // SecurityLevelHigh
codec.RequiresKey()       // true
codec.KeyAlgorithm()      // "AES-256-GCM"

// 设置密钥提供者
codec.SetKeyProvider(embedded.New(key))
```

## CodecChain使用

### 创建链

```go
// 创建链: AES-256 → Base64
chain := codec.NewChain().
    AddCodec(aes.NewAES256GCM()).
    AddCodec(base64.New())

// 设置密钥提供者（AES需要密钥）
chain.SetKeyProvider(embedded.New(key))
```

### 处理顺序

```
Encode:  data → AES.Encode → Base64.Encode → output
Decode:  output → Base64.Decode → AES.Decode → data
```

### 安全等级

```go
// 链中最低等级决定整体安全等级
chain.SecurityLevel()  // SecurityLevelLow (Base64的等级)
// 注: AES-256是High，但Base64是Low，整体是Low
```

## 待实现模块

- `codec/rsa/` - RSA加密
- `codec/chacha20/` - ChaCha20-Poly1305

## 依赖关系

```
codec/
├── 依赖 → keyprovider/  # KeyProvider接口（KeyAwareCodec）
├── 依赖 → errors.go     # 错误定义
└── 无其他模块依赖
```

## 使用示例

### 在Bus中使用

```go
// 创建Codec链
chain := codec.NewChain().
    AddCodec(aes.NewAES256GCM()).
    AddCodec(base64.New())

// 设置密钥
chain.SetKeyProvider(embedded.New([]byte("32-byte-secret-key")))

// 创建Bus
bus := core.NewBuilder().
    UseSerializerInstance(plain.New()).
    UseCodecChain(chain).
    UseChannel(tcp.NewClientChannel("server:8080")).
    Build()
```

### 协商过程

在Handshake协商中，使用SecurityLevel而非InternalID：

```go
// 客户端发送支持的安全等级
request := &protocol.HandshakeRequest{
    SupportedCodecLevels: []codec.SecurityLevel{
        codec.SecurityLevelHigh,
        codec.SecurityLevelMedium,
    },
    MinSecurityLevel: codec.SecurityLevelMedium,
}

// 服务端验证
policy := protocol.DefaultNegotiationPolicy()
if request.MinSecurityLevel < policy.MinSecurityLevel {
    // 拒绝连接（防降级攻击）
    return errors.ErrDegradationAttack
}
```