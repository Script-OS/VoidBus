# KeyProvider Package - 密钥提供者模块

密钥提供者模块负责密钥的获取和管理，为需要密钥的Codec提供密钥。

**安全边界**: ❌ 不可暴露 - 密钥相关信息完全不暴露。

## 文件结构

```
keyprovider/
├── keyprovider.go    # KeyProvider接口定义
├── embedded/         # 编译时嵌入密钥实现
│   └── embedded.go
```

## 模块职责

### KeyProvider接口

**职责**：
- 提供密钥获取接口
- 支持多种密钥来源（URL/Embedded）
- 预留密钥刷新机制（架构兼容）

**不负责**：
- 使用密钥进行加解密（由Codec负责）
- 密钥生成
- 密钥存储安全

## 接口定义

```go
// KeyProvider - 密钥提供者接口
type KeyProvider interface {
    // GetKey 获取密钥
    GetKey() ([]byte, error)
    
    // RefreshKey 刷新密钥（当前返回ErrNotImplemented）
    RefreshKey() error
    
    // SupportsRefresh 是否支持刷新
    SupportsRefresh() bool
    
    // Type 密钥提供者类型
    Type() KeyProviderType
}

// KeyProviderType - 密钥提供者类型
type KeyProviderType string

const (
    TypeURL      KeyProviderType = "url"      // URL加载（预留）
    TypeEmbedded KeyProviderType = "embedded" // 编译时嵌入
    TypeFile     KeyProviderType = "file"     // 文件加载
    TypeEnv      KeyProviderType = "env"      // 环境变量
)

// KeyProviderWithMetadata - 带元数据的密钥提供者
type KeyProviderWithMetadata interface {
    KeyProvider
    
    // GetKeyMetadata 获取密钥元数据
    GetKeyMetadata() (*KeyMetadata, error)
}

// KeyMetadata - 密钥元数据
type KeyMetadata struct {
    Algorithm   string    // 加密算法
    KeySize     int       // 密钥大小（字节）
    CreatedAt   time.Time // 创建时间
    ExpiresAt   time.Time // 过期时间（可选）
    RotationID  string    // 轮换标识
}
```

## 已实现模块

### Embedded KeyProvider

位置: `keyprovider/embedded/embedded.go`

**特点**：
- 编译时嵌入密钥
- 使用Go的embed.FS机制
- 不支持刷新（SupportsRefresh = false）
- 密钥来源完全隐藏

```go
// 直接提供密钥
keyProvider := embedded.New([]byte("32-byte-secret-key-for-aes-256"))

// 从嵌入文件加载
//go:embed secret.key
var secretKey embed.FS
keyProvider := embedded.NewFromFS(secretKey, "secret.key")

// 获取密钥
key, err := keyProvider.GetKey()

// 不支持刷新
keyProvider.SupportsRefresh()  // false
keyProvider.RefreshKey()       // ErrNotImplemented

// 类型
keyProvider.Type()  // "embedded"
```

## 待实现模块

- `keyprovider/url/` - URL加载密钥（格式待确定）
- `keyprovider/file/` - 文件加载密钥
- `keyprovider/env/` - 环境变量密钥

## 密钥刷新预留

接口预留了密钥刷新机制，但当前实现不支持：

```go
// 架构兼容的刷新接口
type KeyProvider interface {
    RefreshKey() error       // 当前返回ErrNotImplemented
    SupportsRefresh() bool   // 当前返回false
}

// 未来实现时无需修改架构
type RefreshableKeyProvider struct {
    // ...
}

func (p *RefreshableKeyProvider) RefreshKey() error {
    // 实际刷新逻辑
    return nil
}

func (p *RefreshableKeyProvider) SupportsRefresh() bool {
    return true
}
```

## 依赖关系

```
keyprovider/
├── 无外部模块依赖
└── 依赖 → errors.go    # 错误定义
```

## 使用示例

### 与AES Codec配合

```go
// 创建密钥提供者
keyProvider := embedded.New([]byte("32-byte-secret-key-for-aes-256"))

// 创建AES Codec
aesCodec := aes.NewAES256GCM()

// 设置密钥提供者
aesCodec.SetKeyProvider(keyProvider)

// AES Codec会在Encode/Decode时自动获取密钥
encoded, err := aesCodec.Encode(data)
```

### 与CodecChain配合

```go
// 创建Codec链
chain := codec.NewChain().
    AddCodec(aes.NewAES256GCM()).
    AddCodec(base64.New())

// 设置密钥提供者（AES需要密钥）
keyProvider := embedded.New([]byte("32-byte-secret-key"))
chain.SetKeyProvider(keyProvider)

// 在Bus中使用
bus := core.NewBuilder().
    UseSerializerInstance(plain.New()).
    UseCodecChain(chain).
    UseChannel(tcp.NewClientChannel("server:8080")).
    Build()
```

### 在ServerBus中使用

```go
serverBus := core.NewServerBusBuilder().
    SetSerializer(plain.New()).
    SetCodecChain(codec.NewChain().AddCodec(aes.NewAES256GCM())).
    SetKeyProvider(embedded.New(serverKey)).  // 服务端密钥
    OnClientConnect(func(clientID string, bus core.Bus) {
        // 每个客户端Bus使用相同的KeyProvider
    }).
    Build()
```

### 编译时嵌入密钥

```go
// main.go
package main

import (
    "embed"
    "github.com/voidbus/keyprovider/embedded"
)

//go:embed keys/secret.key
var keyFiles embed.FS

func main() {
    // 从嵌入文件系统加载密钥
    keyProvider, err := embedded.NewFromFS(keyFiles, "keys/secret.key")
    if err != nil {
        panic(err)
    }
    
    // 密钥完全隐藏在编译后的二进制中
    // ...
}
```

## 安全建议

1. **密钥长度**: AES-128需要16字节，AES-256需要32字节
2. **密钥来源**: 优先使用embedded方式，避免密钥暴露在配置文件中
3. **密钥轮换**: 架构预留刷新接口，未来可支持动态密钥轮换
4. **密钥分离**: 不同服务使用不同密钥，避免密钥共享