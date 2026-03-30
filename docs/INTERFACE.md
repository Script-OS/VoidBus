# VoidBus 接口规范文档

## 1. 接口设计原则

### 1.1 核心原则
- **最小接口原则**：每个接口仅包含必要的方法
- **职责单一原则**：每个接口只有一个明确的职责
- **版本兼容原则**：接口扩展通过组合而非修改
- **错误明确原则**：每个方法返回明确的错误类型

### 1.2 接口分类
| 分类 | 说明 | 示例 |
|------|------|------|
| **核心接口** | 系统运行必需的接口 | Channel, Codec, Serializer |
| **扩展接口** | 增强功能的可选接口 | KeyAwareCodec, ServerChannel |
| **管理接口** | 生命周期管理接口 | Bus, ServerBus, MultiBus |
| **辅助接口** | 内部使用的接口 | SessionRegistry, FragmentManager |

---

## 2. Serializer（序列化器）接口规范

### 2.1 核心接口

```go
package serializer

// Serializer 序列化器核心接口
// 负责数据结构的序列化与反序列化
// 可暴露在元数据协议中，用于双方协商
type Serializer interface {
    // Serialize 将数据序列化为字节流
    //
    // 参数约束:
    //   - data: 必须为非nil的byte切片，长度>=0
    //
    // 返回值保证:
    //   - 成功时返回序列化后的字节流，长度>=0
    //   - 失败时返回nil和明确的错误
    //
    // 错误类型:
    //   - ErrInvalidData: 输入数据无效
    //   - ErrSerializationFailed: 序列化过程失败
    Serialize(data []byte) ([]byte, error)
    
    // Deserialize 将字节流反序列化为原始数据
    //
    // 参数约束:
    //   - data: 必须为有效的序列化格式字节流
    //
    // 返回值保证:
    //   - 成功时返回原始数据
    //   - 失败时返回nil和明确的错误
    //
    // 错误类型:
    //   - ErrInvalidData: 输入数据格式无效
    //   - ErrDeserializationFailed: 反序列化过程失败
    Deserialize(data []byte) ([]byte, error)
    
    // Name 返回序列化器名称
    //
    // 返回值保证:
    //   - 返回唯一的、可暴露的名称标识
    //   - 名称格式: 小写字母+数字+下划线，如 "json", "protobuf_v2"
    Name() string
    
    // Priority 返回优先级
    //
    // 返回值保证:
    //   - 返回0-100的优先级值
    //   - 值越高优先级越高
    //   - 用于协商时的排序选择
    Priority() int
}
```

### 2.2 Serializer注册接口

```go
// SerializerModule 序列化器模块接口（用于注册）
type SerializerModule interface {
    // Create 创建序列化器实例
    //
    // 参数约束:
    //   - args: 可选配置参数，类型由具体实现定义
    //
    // 返回值保证:
    //   - 成功时返回Serializer实例
    //   - 失败时返回nil和错误
    Create(args interface{}) (Serializer, error)
    
    // Name 返回模块名称（与Create返回的Serializer.Name()一致）
    Name() string
}

// SerializerRegistry 序列化器注册表
type SerializerRegistry interface {
    // Register 注册序列化器模块
    Register(module SerializerModule) error
    
    // Get 获取序列化器实例
    Get(name string) (Serializer, error)
    
    // List 列出所有已注册的序列化器名称
    List() []string
    
    // GetByPriority 按优先级获取可用序列化器列表（降序）
    GetByPriority() []Serializer
}
```

### 2.3 预定义Serializer类型

| 名称 | 优先级 | 说明 | 实现位置 |
|------|--------|------|----------|
| `plain` | 0 | 无序列化，直接传递原始字节 | `serializer/plain/` |
| `json` | 50 | JSON序列化 | `serializer/json/` |
| `protobuf` | 80 | Protocol Buffers序列化 | `serializer/protobuf/` |

---

## 3. Codec（编码/加密）接口规范

### 3.1 核心接口

```go
package codec

// SecurityLevel 安全等级定义
type SecurityLevel int

const (
    // SecurityLevelNone 无安全措施（仅调试模式）
    // Release模式下禁止使用
    SecurityLevelNone SecurityLevel = 0
    
    // SecurityLevelLow 低安全等级
    // 仅编码类操作，如Base64
    SecurityLevelLow SecurityLevel = 1
    
    // SecurityLevelMedium 中等安全等级
    // 对称加密，如AES-128
    SecurityLevelMedium SecurityLevel = 2
    
    // SecurityLevelHigh 高安全等级
    // 强加密，如AES-256, RSA-2048+
    SecurityLevelHigh SecurityLevel = 3
)

// Codec 编码/加密核心接口
// 负责数据的编码/加密和解码/解密
// 不可暴露在元数据协议中
type Codec interface {
    // Encode 编码/加密数据
    //
    // 参数约束:
    //   - data: 必须为非nil的byte切片
    //
    // 返回值保证:
    //   - 成功时返回编码后的数据
    //   - 输出长度: encode后长度可能增大
    //
    // 错误类型:
    //   - ErrKeyRequired: 需要密钥但未设置
    //   - ErrInvalidKey: 密钥无效
    //   - ErrInvalidData: 数据无效
    //   - ErrEncodingFailed: 编码过程失败
    Encode(data []byte) ([]byte, error)
    
    // Decode 解码/解密数据
    //
    // 参数约束:
    //   - data: 必须为有效的编码格式数据
    //
    // 返回值保证:
    //   - 成功时返回原始数据
    //
    // 错误类型:
    //   - ErrKeyRequired: 需要密钥但未设置
    //   - ErrInvalidKey: 密钥无效
    //   - ErrInvalidData: 数据格式无效或损坏
    //   - ErrDecodingFailed: 解码过程失败
    Decode(data []byte) ([]byte, error)
    
    // InternalID 返回内部标识符
    //
    // 返回值保证:
    //   - 返回唯一标识符，仅用于内部管理
    //   - 此ID不可通过网络传输
    //   - 格式: 内部编码，如 "codec_aes_256_gcm"
    InternalID() string
    
    // SecurityLevel 返回安全等级
    //
    // 返回值保证:
    //   - 返回SecurityLevel常量值
    //   - 用于协商时的安全等级匹配
    SecurityLevel() SecurityLevel
}
```

### 3.2 密钥相关扩展接口

```go
// KeyAwareCodec 需要密钥的Codec扩展接口
type KeyAwareCodec interface {
    Codec
    
    // SetKeyProvider 设置密钥提供者
    //
    // 参数约束:
    //   - provider: 必须为有效的KeyProvider实例
    //
    // 返回值保证:
    //   - 设置成功后，后续Encode/Decode可正常使用密钥
    //
    // 错误类型:
    //   - ErrInvalidKeyProvider: provider无效
    //   - ErrKeyIncompatible: 密钥类型不兼容
    SetKeyProvider(provider KeyProvider) error
    
    // RequiresKey 返回是否需要密钥
    //
    // 返回值保证:
    //   - 加密类Codec返回true
    //   - 编码类Codec（如Base64）返回false
    RequiresKey() bool
    
    // KeyAlgorithm 返回需要的密钥算法类型
    //
    // 返回值保证:
    //   - 返回算法标识，如 "AES-256-GCM"
    //   - 非加密Codec返回空字符串
    KeyAlgorithm() string
}
```

### 3.3 CodecChain接口

```go
// CodecChain Codec链式组合接口
type CodecChain interface {
    // AddCodec 添加Codec到链末端
    //
    // 参数约束:
    //   - codec: 必须为有效的Codec实例
    //   - 链最大长度由NegotiationPolicy.MaxCodecChainLength限制
    //
    // 返回值保证:
    //   - 返回更新后的CodecChain（支持链式调用）
    //
    // 错误类型:
    //   - ErrChainTooLong: 链长度超过限制
    //   - ErrCodecConflict: Codec之间存在冲突
    AddCodec(codec Codec) CodecChain
    
    // AddCodecAt 添加Codec到指定位置
    //
    // 参数约束:
    //   - codec: 必须为有效的Codec实例
    //   - index: 0到当前链长度的有效索引
    //
    // 返回值保证:
    //   - Codec插入到指定位置
    AddCodecAt(codec Codec, index int) CodecChain
    
    // RemoveCodec 移除指定位置的Codec
    //
    // 参数约束:
    //   - index: 有效索引位置
    RemoveCodecAt(index int) CodecChain
    
    // Encode 按链顺序编码
    //
    // 处理顺序:
    //   data → Codec[0].Encode → Codec[1].Encode → ... → Codec[n].Encode → output
    //
    // 返回值保证:
    //   - 按顺序应用所有Codec的Encode方法
    Encode(data []byte) ([]byte, error)
    
    // Decode 按链逆序解码
    //
    // 处理顺序:
    //   data → Codec[n].Decode → ... → Codec[1].Decode → Codec[0].Decode → output
    //
    // 返回值保证:
    //   - 按逆序应用所有Codec的Decode方法
    Decode(data []byte) ([]byte, error)
    
    // SecurityLevel 返回链的整体安全等级
    //
    // 返回值保证:
    //   - 返回链中最低的SecurityLevel值（安全短板原则）
    SecurityLevel() SecurityLevel
    
    // Length 返回链中Codec数量
    Length() int
    
    // IsEmpty 返回链是否为空
    IsEmpty() bool
    
    // SetKeyProvider 为所有需要密钥的Codec设置KeyProvider
    //
    // 处理逻辑:
    //   - 遍历链中所有Codec
    //   - 对实现KeyAwareCodec的Codec调用SetKeyProvider
    SetKeyProvider(provider KeyProvider) error
    
    // Clone 克隆CodecChain
    //
    // 返回值保证:
    //   - 返回独立的副本，不影响原链
    Clone() CodecChain
}
```

### 3.4 预定义Codec类型

| InternalID | SecurityLevel | 需要密钥 | 说明 |
|------------|---------------|----------|------|
| `codec_plain` | None (0) | No | 无编码（仅调试） |
| `codec_base64` | Low (1) | No | Base64编码 |
| `codec_aes_128_gcm` | Medium (2) | Yes (128-bit) | AES-128-GCM加密 |
| `codec_aes_256_gcm` | High (3) | Yes (256-bit) | AES-256-GCM加密 |
| `codec_rsa_2048` | High (3) | Yes (RSA key) | RSA-2048加密 |

---

## 4. Channel（信道）接口规范

### 4.1 核心接口

```go
package channel

// ChannelType 信道类型标识
type ChannelType string

const (
    TypeTCP  ChannelType = "tcp"
    TypeUDP  ChannelType = "udp"
    TypeICMP ChannelType = "icmp"
    TypeQUIC ChannelType = "quic"
)

// Channel 信道核心接口
// 负责底层传输层的通信
// 不可暴露在元数据协议中
type Channel interface {
    // Send 发送原始字节数据
    //
    // 参数约束:
    //   - data: 必须为非nil的byte切片
    //   - 数据会被完整发送，不保证原子性
    //
    // 返回值保证:
    //   - 成功时数据已发送到对端
    //   - 失败时连接可能已断开
    //
    // 错误类型:
    //   - ErrChannelClosed: 信道已关闭
    //   - ErrChannelDisconnected: 连接已断开
    //   - ErrChannelSendFailed: 发送失败
    //   - ErrChannelTimeout: 发送超时
    Send(data []byte) error
    
    // Receive 接收原始字节数据
    //
    // 行为特性:
    //   - 阻塞操作，等待数据到达
    //   - 返回完整的一个数据单元
    //
    // 返回值保证:
    //   - 成功时返回接收到的完整数据
    //   - 失败时连接可能已断开
    //
    // 错误类型:
    //   - ErrChannelClosed: 信道已关闭
    //   - ErrChannelDisconnected: 连接已断开
    //   - ErrChannelRecvFailed: 接收失败
    //   - ErrChannelTimeout: 接收超时
    Receive() ([]byte, error)
    
    // Close 关闭信道
    //
    // 行为特性:
    //   - 关闭后信道不可再使用
    //   - 释放所有相关资源
    //
    // 返回值保证:
    //   - 成功时资源已释放
    //   - 多次调用Close返回ErrChannelClosed
    Close() error
    
    // IsConnected 返回连接状态
    //
    // 返回值保证:
    //   - true: 信道可用
    //   - false: 信道不可用（未连接或已断开）
    IsConnected() bool
    
    // Type 返回信道类型
    //
    // 返回值保证:
    //   - 返回ChannelType常量
    //   - 仅用于内部管理，不可传输
    Type() ChannelType
}
```

### 4.2 服务端扩展接口

```go
// ServerChannel 服务端信道接口
type ServerChannel interface {
    Channel
    
    // Accept 接受新连接
    //
    // 行为特性:
    //   - 阻塞操作，等待新连接
    //   - 每次返回一个新客户端Channel
    //
    // 返回值保证:
    //   - 成功时返回客户端Channel实例
    //   - 客户端Channel已建立连接
    //
    // 错误类型:
    //   - ErrChannelClosed: 服务端已关闭
    //   - ErrAcceptFailed: 接受连接失败
    Accept() (Channel, error)
    
    // ListenAddress 返回监听地址
    //
    // 返回值保证:
    //   - 返回格式: "host:port"
    ListenAddress() string
    
    // ClientCount 返回已接受的客户端数量
    ClientCount() int
}
```

### 4.3 Channel配置

```go
// ChannelConfig 信道配置
type ChannelConfig struct {
    // Address 目标地址
    // 格式: "host:port"
    Address string
    
    // Timeout 操作超时时间（秒）
    // 0表示无超时
    Timeout int
    
    // BufferSize 缓冲区大小（字节）
    BufferSize int
    
    // KeepAlive 心跳保活配置
    KeepAlive KeepAliveConfig
    
    // TLS TLS配置（可选）
    TLS *TLSConfig
}

// KeepAliveConfig 心跳保活配置
type KeepAliveConfig struct {
    // Enable 是否启用心跳
    Enable bool
    
    // Interval 心跳间隔（秒）
    Interval int
    
    // Timeout 心跳超时（秒）
    // 超时后认为连接断开
    Timeout int
    
    // MaxMissed 最大丢失心跳数
    // 超过此数量触发重连
    MaxMissed int
}
```

---

## 5. KeyProvider（密钥提供者）接口规范

### 5.1 核心接口

```go
package keyprovider

// KeyProviderType 密钥提供者类型
type KeyProviderType string

const (
    TypeURL      KeyProviderType = "url"      // URL加载
    TypeEmbedded KeyProviderType = "embedded" // 编译时嵌入
    TypeFile     KeyProviderType = "file"     // 文件加载
    TypeEnv      KeyProviderType = "env"      // 环境变量
)

// KeyProvider 密钥提供者核心接口
type KeyProvider interface {
    // GetKey 获取当前密钥
    //
    // 返回值保证:
    //   - 成功时返回有效的密钥字节
    //   - 密钥格式由Codec定义（如AES需要16/32字节）
    //
    // 错误类型:
    //   - ErrKeyNotFound: 密钥未找到
    //   - ErrKeyFetchFailed: 密钥获取失败
    //   - ErrKeyExpired: 密钥已过期
    GetKey() ([]byte, error)
    
    // RefreshKey 刷新密钥
    //
    // 当前实现:
    //   - 返回ErrNotImplemented（未来支持）
    //
    // 未来实现时:
    //   - 从源重新获取密钥
    //   - 支持密钥轮换
    RefreshKey() error
    
    // SupportsRefresh 返回是否支持刷新
    //
    // 返回值保证:
    //   - 当前实现返回false
    //   - 未来实现URL等动态源返回true
    SupportsRefresh() bool
    
    // Type 返回提供者类型
    //
    // 返回值保证:
    //   - 返回KeyProviderType常量
    Type() KeyProviderType
}
```

### 5.2 密钥元数据扩展接口

```go
// KeyMetadata 密钥元数据
type KeyMetadata struct {
    // ID 密钥唯一标识
    ID string
    
    // Algorithm 目标算法
    Algorithm string
    
    // CreatedAt 创建时间
    CreatedAt int64
    
    // ExpiresAt 过期时间（0表示永不过期）
    ExpiresAt int64
    
    // Source 密钥来源
    Source KeyProviderType
    
    // RotationCount 轮换次数（未来功能）
    RotationCount int
}

// KeyProviderWithMetadata 带元数据的密钥提供者
type KeyProviderWithMetadata interface {
    KeyProvider
    
    // GetKeyMetadata 获取密钥元数据
    GetKeyMetadata() (KeyMetadata, error)
}
```

### 5.3 Embedded KeyProvider设计

```go
// EmbeddedKeyProviderConfig 嵌入密钥配置
type EmbeddedKeyProviderConfig struct {
    // Key 密钥数据
    Key []byte
    
    // Algorithm 算法标识
    Algorithm string
    
    // KeyID 密钥标识
    KeyID string
}

// 使用embed机制嵌入密钥文件
// 
// 实现方式:
// import _ "embed"
// 
// //go:embed keys/aes_key.bin
// var embeddedAESKey []byte
//
// 或通过编译时变量注入:
// go build -ldflags "-X main.embeddedKey=xxx"
```

---

## 6. Fragment（分片）接口规范

### 6.1 核心接口

```go
package fragment

// Fragment 分片器核心接口
type Fragment interface {
    // Split 分片数据
    //
    // 参数约束:
    //   - data: 必须为非nil的byte切片
    //   - maxSize: 每个分片的最大大小（>=64字节）
    //
    // 返回值保证:
    //   - 返回分片列表，每个分片<=maxSize
    //   - 分片顺序保持原始数据顺序
    //   - 每个分片包含FragmentInfo头
    //
    // 错误类型:
    //   - ErrInvalidMaxSize: maxSize无效
    //   - ErrFragmentFailed: 分片失败
    Split(data []byte, maxSize int) ([][]byte, error)
    
    // Reassemble 重组数据
    //
    // 参数约束:
    //   - fragments: 必须为同一ID的所有分片
    //   - 分片必须完整且顺序正确
    //
    // 返回值保证:
    //   - 返回重组后的原始数据
    //
    // 错误类型:
    //   - ErrFragmentIncomplete: 分片不完整
    //   - ErrFragmentMissing: 分片丢失
    //   - ErrFragmentCorrupted: 分片损坏
    //   - ErrFragmentMismatch: 分片ID不匹配
    Reassemble(fragments [][]byte) ([]byte, error)
    
    // GetFragmentInfo 从分片数据中提取元数据
    //
    // 参数约束:
    //   - fragment: 带FragmentInfo头的分片数据
    //
    // 返回值保证:
    //   - 返回分片的元数据信息
    GetFragmentInfo(fragment []byte) (FragmentInfo, error)
    
    // SetFragmentInfo 为分片数据添加元数据头
    //
    // 返回值保证:
    //   - 返回带头的分片数据
    SetFragmentInfo(data []byte, info FragmentInfo) ([]byte, error)
}
```

### 6.2 FragmentInfo结构

```go
// FragmentInfo 分片元数据
type FragmentInfo struct {
    // ID 分片组唯一标识
    // 格式: UUID v4，随机生成，无语义信息
    ID string
    
    // Index 分片序号（0-based）
    Index uint16
    
    // Total 总分片数
    Total uint16
    
    // Size 分片数据大小（不含头部）
    Size uint32
    
    // Checksum 分片数据校验和（CRC32）
    Checksum uint32
    
    // IsLast 是否最后一片
    IsLast bool
}
```

### 6.3 FragmentManager接口

```go
// FragmentManager 分片管理器
// 负责接收端分片的缓存和重组
type FragmentManager interface {
    // CreateState 创建分片重组状态
    //
    // 参数约束:
    //   - id: 分片组ID
    //   - totalCount: 预期总分片数
    CreateState(id string, totalCount int) error
    
    // AddFragment 添加接收到的分片
    //
    // 参数约束:
    //   - id: 分片组ID
    //   - index: 分片序号
    //   - data: 分片数据
    //
    // 行为特性:
    //   - 自动验证Checksum
    //   - 检查Index是否在有效范围
    AddFragment(id string, index int, data []byte) error
    
    // IsComplete 检查分片组是否完整
    IsComplete(id string) (bool, error)
    
    // GetMissingIndices 获取丢失的分片序号
    GetMissingIndices(id string) ([]int, error)
    
    // Reassemble 重组完整分片组
    //
    // 前置条件:
    //   - IsComplete(id) == true
    Reassemble(id string) ([]byte, error)
    
    // ClearState 清除分片状态
    ClearState(id string) error
    
    // GetTimeoutIds 获取超时的分片组ID列表
    //
    // 用于清理长时间未完成的分片组
    GetTimeoutIds(timeout time.Duration) []string
    
    // SetTimeout 设置分片超时时间
    SetTimeout(id string, timeout time.Time) error
}
```

---

## 7. Bus接口规范

### 7.1 核心Bus接口

```go
package voidbus

// Bus 单信道总线接口
type Bus interface {
    // Send 发送数据
    //
    // 处理流程:
    //   data → Serializer.Serialize → CodecChain.Encode → [Fragment.Split] → Channel.Send
    //
    // 参数约束:
    //   - data: 非nil的byte切片
    //
    // 返回值保证:
    //   - 数据已发送到对端
    Send(data []byte) error
    
    // Receive 接收数据（阻塞）
    //
    // 处理流程:
    //   Channel.Receive → [Fragment.Reassemble] → CodecChain.Decode → Serializer.Deserialize → data
    //
    // 返回值保证:
    //   - 返回完整、解码后的数据
    Receive() ([]byte, error)
    
    // SetSerializer 设置序列化器
    SetSerializer(serializer Serializer) Bus
    
    // SetCodecChain 设置Codec链
    SetCodecChain(chain CodecChain) Bus
    
    // SetChannel 设置信道
    SetChannel(channel Channel) Bus
    
    // SetKeyProvider 设置密钥提供者
    SetKeyProvider(provider KeyProvider) Bus
    
    // SetFragment 设置分片器（可选）
    SetFragment(fragment Fragment) Bus
    
    // OnMessage 注册消息处理回调
    //
    // 当启用异步接收时，接收到的消息通过回调处理
    OnMessage(handler func(data []byte)) Bus
    
    // OnError 注册错误处理回调
    OnError(handler func(err error)) Bus
    
    // Start 启动总线
    //
    // 前置条件:
    //   - Serializer已设置
    //   - CodecChain已设置且非空
    //   - Channel已设置且已连接
    //   - 如Codec需要密钥，KeyProvider已设置
    //
    // 行为特性:
    //   - 启动后台接收循环（如果设置了OnMessage）
    Start() error
    
    // Stop 停止总线
    //
    // 行为特性:
    //   - 停止后台接收循环
    //   - 关闭Channel
    Stop() error
    
    // IsRunning 返回运行状态
    IsRunning() bool
    
    // GetSessionID 返回会话ID
    //
    // 用于MultiBus中的标识
    GetSessionID() string
}
```

### 7.2 BusBuilder接口

```go
// BusBuilder Bus构建器（Fluent API）
type BusBuilder interface {
    // UseSerializer 使用序列化器
    UseSerializer(name string) BusBuilder
    
    // UseSerializerInstance 使用序列化器实例
    UseSerializerInstance(serializer Serializer) BusBuilder
    
    // UseCodecChain 使用Codec链
    UseCodecChain(chain CodecChain) BusBuilder
    
    // UseCodec 添加单个Codec
    UseCodec(codec Codec) BusBuilder
    
    // UseChannel 使用信道
    UseChannel(channel Channel) BusBuilder
    
    // UseKeyProvider 使用密钥提供者
    UseKeyProvider(provider KeyProvider) BusBuilder
    
    // UseFragment 使用分片器
    UseFragment(fragment Fragment) BusBuilder
    
    // WithConfig 设置配置
    WithConfig(config BusConfig) BusBuilder
    
    // Build 构建Bus实例
    Build() (Bus, error)
}
```

### 7.3 BusConfig结构

```go
// BusConfig 总线配置
type BusConfig struct {
    // AsyncReceive 是否启用异步接收
    AsyncReceive bool
    
    // EnableFragment 是否启用分片
    EnableFragment bool
    
    // MaxFragmentSize 最大分片大小（字节）
    MaxFragmentSize int
    
    // SendQueueSize 发送队列大小
    SendQueueSize int
    
    // RecvQueueSize 接收队列大小
    RecvQueueSize int
    
    // FragmentTimeout 分片重组超时（秒）
    FragmentTimeout int
    
    // AutoReconnect 是否自动重连
    AutoReconnect bool
    
    // ReconnectDelay 重连延迟（秒）
    ReconnectDelay int
    
    // MaxReconnectAttempts 最大重连尝试次数（0=无限）
    MaxReconnectAttempts int
}
```

---

## 8. ServerBus接口规范

### 8.1 ServerBus接口

```go
// ServerBus 服务端总线接口
type ServerBus interface {
    // Listen 开始监听
    //
    // 参数约束:
    //   - address: 监听地址，格式 "host:port"
    Listen(address string) error
    
    // Start 启动服务端
    //
    // 前置条件:
    //   - 已调用Listen
    //   - NegotiationPolicy已设置
    Start() error
    
    // Stop 停止服务端
    //
    // 行为特性:
    //   - 关闭所有客户端连接
    //   - 停止监听
    Stop() error
    
    // SetNegotiationPolicy 设置协商策略
    SetNegotiationPolicy(policy NegotiationPolicy) ServerBus
    
    // SetSerializer 设置默认序列化器
    SetSerializer(serializer Serializer) ServerBus
    
    // SetCodecChain 设置默认Codec链
    SetCodecChain(chain CodecChain) ServerBus
    
    // SetKeyProvider 设置密钥提供者
    SetKeyProvider(provider KeyProvider) ServerBus
    
    // OnClientConnect 客户端连接回调
    //
    // 参数:
    //   - clientID: 客户端唯一标识
    //   - bus: 客户端Bus实例（可用于向客户端发送）
    OnClientConnect(handler func(clientID string, bus *ClientBus)) ServerBus
    
    // OnClientDisconnect 客户端断开回调
    OnClientDisconnect(handler func(clientID string, reason string)) ServerBus
    
    // OnMessage 消息接收回调
    //
    // 参数:
    //   - clientID: 来源客户端ID
    //   - data: 接收到的数据
    OnMessage(handler func(clientID string, data []byte)) ServerBus
    
    // Broadcast 向所有客户端广播
    Broadcast(data []byte) error
    
    // SendTo 向指定客户端发送
    SendTo(clientID string, data []byte) error
    
    // GetClient 获取客户端Bus实例
    GetClient(clientID string) (*ClientBus, error)
    
    // GetClients 获取所有客户端ID列表
    GetClients() []string
    
    // ClientCount 获取客户端数量
    ClientCount() int
    
    // IsRunning 返回运行状态
    IsRunning() bool
}
```

### 8.2 ClientBus结构

```go
// ClientBus 客户端Bus实例（服务端侧）
type ClientBus struct {
    // ClientID 客户端唯一标识
    ClientID string
    
    // SessionID 会话标识
    SessionID string
    
    // ConnectedAt 连接时间
    ConnectedAt time.Time
    
    // Serializer 选定的序列化器
    Serializer Serializer
    
    // CodecChain 选定的Codec链
    CodecChain CodecChain
    
    // Channel 客户端信道
    Channel Channel
}
```

---

## 9. MultiBus接口规范

### 9.1 MultiBus接口

```go
// MultiBus 多信道总线接口
type MultiBus interface {
    // AddBus 添加Bus实例
    //
    // 参数约束:
    //   - bus: 已配置好的Bus实例
    //   - weight: 权重（用于加权随机分配，>=1）
    //   - alias: 可选别名（用于SendVia指定）
    //
    // 返回值保证:
    //   - Bus被添加到可用列表
    //   - 返回分配的busID（可用于SendVia）
    AddBus(bus Bus, weight int, alias string) (busID string, error)
    
    // RemoveBus 移除Bus实例
    RemoveBus(busID string) error
    
    // Send 随机多信道发送
    //
    // 处理流程:
    //   - 如EnableFragment: Fragment.Split → 按策略分配分片到各Bus → 各Bus.Send
    //   - 如非分片: 按策略选择一个Bus → Bus.Send
    //
    // 分配策略:
    //   - 由SendStrategy控制
    Send(data []byte) error
    
    // SendVia 指定单一信道发送
    //
    // 参数约束:
    //   - busID: AddBus返回的标识或别名
    //
    // 行为特性:
    //   - 不分片，完整数据通过指定信道发送
    SendVia(busID string, data []byte) error
    
    // SendWithStrategy 按指定策略发送
    SendWithStrategy(data []byte, strategy SendStrategy) error
    
    // SetFragment 设置分片器
    SetFragment(fragment Fragment) MultiBus
    
    // SetDefaultStrategy 设置默认发送策略
    SetDefaultStrategy(strategy SendStrategy) MultiBus
    
    // OnMessage 注册消息回调
    //
    // 聚合所有Bus的消息
    //
    // 参数:
    //   - sourceBusID: 来源Bus标识
    //   - data: 接收到的数据
    OnMessage(handler func(sourceBusID string, data []byte)) MultiBus
    
    // OnError 注册错误回调
    OnError(handler func(err error)) MultiBus
    
    // Start 启动所有Bus
    Start() error
    
    // Stop 停止所有Bus
    Stop() error
    
    // IsRunning 返回运行状态
    IsRunning() bool
    
    // GetBus 获取指定Bus实例
    GetBus(busID string) (Bus, error)
    
    // GetAllBuses 获取所有Bus列表
    GetAllBuses() []Bus
    
    // BusCount 获取Bus数量
    BusCount() int
}
```

### 9.2 SendStrategy结构

```go
// SendMode 发送模式
type SendMode int

const (
    // ModeRandom 随机分配
    // 每个分片随机选择一个Bus
    ModeRandom SendMode = iota
    
    // ModeWeighted 加权随机
    // 根据AddBus时设置的weight进行加权随机
    ModeWeighted
    
    // ModeRoundRobin 轮询分配
    // 分片依次分配到各Bus
    ModeRoundRobin
    
    // ModeLeastLoad 最小负载
    // 选择当前负载最小的Bus
    ModeLeastLoad
    
    // ModeManual 手动指定
    // 使用SendVia手动指定
    ModeManual
)

// SendStrategy 发送策略
type SendStrategy struct {
    // Mode 发送模式
    Mode SendMode
    
    // EnableFragment 是否启用分片
    EnableFragment bool
    
    // MaxFragmentSize 最大分片大小
    MaxFragmentSize int
    
    // WeightOverrides 权重覆盖（可选）
    // key: busID, value: weight
    WeightOverrides map[string]int
    
    // BusOrder 指定Bus顺序（用于RoundRobin）
    BusOrder []string
}
```

---

## 10. Handshake协议接口

### 10.1 Handshake接口

```go
package handshake

// HandshakeRequest 客户端协商请求
type HandshakeRequest struct {
    // ClientID 客户端标识
    ClientID string
    
    // SupportedSerializers 支持的序列化器列表
    SupportedSerializers []SerializerInfo
    
    // SupportedCodecChains 支持的Codec链信息
    SupportedCodecChains []CodecChainInfo
    
    // MinSecurityLevel 要求的最低安全等级
    MinSecurityLevel SecurityLevel
    
    // Timestamp 请求时间戳
    Timestamp int64
    
    // Version 协议版本
    Version uint8
}

// SerializerInfo 序列化器信息
type SerializerInfo struct {
    Name     string
    Priority int
}

// CodecChainInfo Codec链信息
// 注意：不暴露具体Codec名称，仅暴露安全等级
type CodecChainInfo struct {
    // SecurityLevel 链的整体安全等级
    SecurityLevel SecurityLevel
    
    // ChainLength 链长度
    ChainLength int
    
    // Hash 链配置哈希（用于验证，不暴露配置）
    Hash string
}

// HandshakeResponse 服务端响应
type HandshakeResponse struct {
    // Accepted 是否接受
    Accepted bool
    
    // RejectReason 拒绝原因
    RejectReason string
    
    // SelectedSerializer 选定的序列化器
    SelectedSerializer string
    
    // SelectedCodecChainInfo 选定的Codec链信息
    SelectedCodecChainInfo CodecChainInfo
    
    // SessionID 分配的会话ID
    SessionID string
    
    // ServerChallenge 服务端挑战数据
    // 客户端需要用选定CodecChain处理后返回
    ServerChallenge []byte
    
    // Timestamp 响应时间戳
    Timestamp int64
}

// HandshakeConfirm 客户端确认
type HandshakeConfirm struct {
    // SessionID 会话ID
    SessionID string
    
    // ChallengeResponse 挑战响应
    // ServerChallenge经过CodecChain.Encode后的结果
    ChallengeResponse []byte
    
    // Timestamp 确认时间戳
    Timestamp int64
}
```

### 10.2 NegotiationPolicy结构

```go
// NegotiationPolicy 协商策略
type NegotiationPolicy struct {
    // DebugMode 是否为调试模式
    // Debug模式允许plaintext Codec
    DebugMode bool
    
    // MinSecurityLevel 最低安全等级
    // Release模式必须>=SecurityLevelMedium
    MinSecurityLevel SecurityLevel
    
    // AllowedSerializers 允许的序列化器白名单
    // 空列表表示允许所有已注册的
    AllowedSerializers []string
    
    // PreferredSerializer 优先选择的序列化器
    PreferredSerializer string
    
    // PreferredCodecChainSecurity 优先选择的Codec链安全等级
    PreferredCodecChainSecurity SecurityLevel
    
    // MaxCodecChainLength 最大Codec链长度
    MaxCodecChainLength int
    
    // ChallengeTimeout 挑战验证超时时间
    ChallengeTimeout time.Duration
    
    // HandshakeTimeout 整体握手超时时间
    HandshakeTimeout time.Duration
    
    // RejectOnMismatch 安全等级不匹配时是否拒绝
    // true: 拒绝连接
    // false: 选择匹配的最高安全等级
    RejectOnMismatch bool
}

// DefaultNegotiationPolicy 默认协商策略（Release）
func DefaultNegotiationPolicy() NegotiationPolicy {
    return NegotiationPolicy{
        DebugMode:                  false,
        MinSecurityLevel:           SecurityLevelMedium,
        AllowedSerializers:         []string{},
        PreferredSerializer:        "",
        PreferredCodecChainSecurity: SecurityLevelHigh,
        MaxCodecChainLength:        5,
        ChallengeTimeout:           30 * time.Second,
        HandshakeTimeout:           60 * time.Second,
        RejectOnMismatch:           true,
    }
}

// DebugNegotiationPolicy 调试模式协商策略
func DebugNegotiationPolicy() NegotiationPolicy {
    return NegotiationPolicy{
        DebugMode:                  true,
        MinSecurityLevel:           SecurityLevelNone,
        AllowedSerializers:         []string{"plain"},
        PreferredSerializer:        "plain",
        PreferredCodecChainSecurity: SecurityLevelNone,
        MaxCodecChainLength:        3,
        ChallengeTimeout:           60 * time.Second,
        HandshakeTimeout:           120 * time.Second,
        RejectOnMismatch:           false,
    }
}
```

---

## 11. SessionRegistry接口规范

### 11.1 SessionRegistry接口

```go
package registry

// SessionRegistry 会话注册表接口
type SessionRegistry interface {
    // Register 注册会话配置
    //
    // 参数约束:
    //   - config: 完整的会话配置
    Register(config SessionConfig) error
    
    // Get 获取会话配置
    //
    // 参数约束:
    //   - sessionID: 会话标识
    //
    // 返回值保证:
    //   - 返回对应的完整配置
    Get(sessionID string) (*SessionConfig, error)
    
    // Update 更新会话配置
    Update(sessionID string, config SessionConfig) error
    
    // Remove 移除会话
    Remove(sessionID string) error
    
    // Exists 检查会话是否存在
    Exists(sessionID string) bool
    
    // List 列出所有会话ID
    List() []string
    
    // Count 获取会话数量
    Count() int
    
    // Clear 清空所有会话
    Clear() error
}
```

### 11.2 SessionConfig结构

```go
// SessionConfig 会话配置
// 注意：此结构仅存储在本地，不通过网络传输
type SessionConfig struct {
    // SessionID 会话唯一标识
    SessionID string
    
    // Serializer 序列化器实例
    Serializer Serializer
    
    // CodecChain Codec链实例
    CodecChain CodecChain
    
    // Channel 信道实例
    Channel Channel
    
    // KeyProvider 密钥提供者（可选）
    KeyProvider KeyProvider
    
    // Fragment 分片器（可选）
    Fragment Fragment
    
    // CreatedAt 创建时间
    CreatedAt time.Time
    
    // LastActivity 最后活动时间
    LastActivity time.Time
    
    // Metadata 额外元数据
    Metadata map[string]string
}
```

---

## 12. 模块注册机制

### 12.1 Build Tags设计

```go
// 每个模块实现文件使用build tags控制编译
//
// 实现文件命名规范:
//   xxx.go         // 正常实现，build tag: xxx
//   xxx_empty.go   // 空实现，build tag: !xxx
//
// 示例: serializer/plain/plain.go
// //go:build plain_serializer
//
// 示例: serializer/plain/plain_empty.go
// //go:build !plain_serializer
```

### 12.2 编译命令示例

```bash
# 编译TCP信道 + AES加密 + JSON序列化
go build -tags "tcp_channel,aes_codec,json_serializer"

# 编译UDP信道 + Base64编码 + Plain序列化（调试模式）
go build -tags "udp_channel,base64_codec,plain_serializer,debug_mode"

# 编译ICMP信道 + RSA加密 + Protobuf序列化
go build -tags "icmp_channel,rsa_codec,protobuf_serializer"

# 编译完整版（所有模块）
go build -tags "full"
```

### 12.3 全局注册表设计

```go
// internal/registry/global.go
package registry

var (
    // Serializers 全局序列化器注册表
    Serializers = NewSerializerRegistry()
    
    // Codecs 全局Codec注册表
    Codecs = NewCodecRegistry()
    
    // Channels 全局Channel注册表
    Channels = NewChannelRegistry()
    
    // KeyProviders 全局KeyProvider注册表
    KeyProviders = NewKeyProviderRegistry()
    
    // Fragments 全局Fragment注册表
    Fragments = NewFragmentRegistry()
)

// 各模块init函数自动注册
// 示例: serializer/plain/plain.go
func init() {
    registry.Serializers.Register(&PlainModule{})
}
```

---

## 13. 错误类型定义

### 13.1 错误层次结构

```go
package voidbus

// VoidBusError 基础错误类型
type VoidBusError struct {
    Op      string    // 操作名称
    Module  string    // 模块名称
    Err     error     // 底层错误
    Msg     string    // 描述信息
}

// ChannelError 信道错误
type ChannelError struct {
    Op        string
    Err       error
    Msg       string
    Retryable bool    // 是否可重试
}

// CodecError Codec错误
type CodecError struct {
    Op          string
    Err         error
    Msg         string
    SecurityLevel SecurityLevel  // 相关安全等级
}

// FragmentError 分片错误
type FragmentError struct {
    Op         string
    Err        error
    Msg        string
    FragmentID string    // 相关分片ID
}

// HandshakeError 协商错误
type HandshakeError struct {
    Op             string
    Err            error
    Msg            string
    ClientID       string
    SecurityIssue  bool    // 是否涉及安全问题
}
```

### 13.2 预定义错误

```go
var (
    // 通用错误
    ErrNotImplemented     = errors.New("not implemented")
    ErrInvalidConfig      = errors.New("invalid configuration")
    ErrModuleNotSet       = errors.New("module not set")
    
    // Serializer错误
    ErrSerializationFailed   = errors.New("serialization failed")
    ErrDeserializationFailed = errors.New("deserialization failed")
    ErrInvalidSerializer     = errors.New("invalid serializer")
    
    // Codec错误
    ErrCodecNotFound      = errors.New("codec not found")
    ErrCodecConflict      = errors.New("codec conflict")
    ErrChainTooLong       = errors.New("codec chain too long")
    ErrKeyRequired        = errors.New("key required")
    ErrInvalidKey         = errors.New("invalid key")
    ErrEncodingFailed     = errors.New("encoding failed")
    ErrDecodingFailed     = errors.New("decoding failed")
    
    // Channel错误
    ErrChannelClosed      = errors.New("channel closed")
    ErrChannelNotReady    = errors.New("channel not ready")
    ErrChannelTimeout     = errors.New("channel timeout")
    ErrChannelDisconnected = errors.New("channel disconnected")
    ErrChannelSendFailed  = errors.New("channel send failed")
    ErrChannelRecvFailed  = errors.New("channel receive failed")
    
    // Fragment错误
    ErrFragmentFailed     = errors.New("fragmentation failed")
    ErrFragmentIncomplete = errors.New("fragment incomplete")
    ErrFragmentMissing    = errors.New("fragment missing")
    ErrFragmentCorrupted  = errors.New("fragment corrupted")
    ErrFragmentMismatch   = errors.New("fragment mismatch")
    ErrFragmentTimeout    = errors.New("fragment timeout")
    
    // KeyProvider错误
    ErrKeyNotFound        = errors.New("key not found")
    ErrKeyExpired         = errors.New("key expired")
    ErrKeyFetchFailed     = errors.New("key fetch failed")
    
    // Handshake错误
    ErrHandshakeFailed      = errors.New("handshake failed")
    ErrHandshakeTimeout     = errors.New("handshake timeout")
    ErrSecurityLevelMismatch = errors.New("security level mismatch")
    ErrChallengeFailed      = errors.New("challenge verification failed")
    ErrDegradationAttack    = errors.New("potential degradation attack detected")
)
```

---

## 14. 使用示例

### 14.1 基本使用（客户端）

```go
package main

import (
    "VoidBus"
    "VoidBus/channel/tcp"
    "VoidBus/codec"
    "VoidBus/codec/aes"
    "VoidBus/codec/base64"
    "VoidBus/keyprovider/embedded"
    "VoidBus/serializer/json"
)

func main() {
    // 创建密钥提供者
    keyProvider := embedded.New(embedded.Config{
        Key:       []byte("32-byte-key-here..."),
        Algorithm: "AES-256-GCM",
    })
    
    // 创建Codec链: AES-256 -> Base64
    codecChain := codec.NewChain().
        AddCodec(aes.NewAES256GCM()).
        AddCodec(base64.New())
    codecChain.SetKeyProvider(keyProvider)
    
    // 创建Bus
    bus := voidbus.NewBuilder().
        UseSerializerInstance(json.New()).
        UseCodecChain(codecChain).
        UseChannel(tcp.NewClient("server:8080")).
        UseKeyProvider(keyProvider).
        WithConfig(voidbus.BusConfig{
            AutoReconnect: true,
        }).
        OnMessage(func(data []byte) {
            println("Received:", string(data))
        }).
        Build()
    
    bus.Start()
    bus.Send([]byte("Hello, VoidBus!"))
}
```

### 14.2 服务端使用

```go
func main() {
    // 服务端协商策略
    policy := voidbus.DefaultNegotiationPolicy()
    
    // 创建ServerBus
    serverBus := voidbus.NewServerBus().
        SetNegotiationPolicy(policy).
        SetSerializer(json.New()).
        SetCodecChain(codec.NewChain().AddCodec(aes.NewAES256GCM())).
        SetKeyProvider(embedded.New(embedded.Config{...})).
        OnClientConnect(func(clientID string, bus *voidbus.ClientBus) {
            println("Client connected:", clientID)
        }).
        OnMessage(func(clientID string, data []byte) {
            println("Message from", clientID, ":", string(data))
            // 回复
            serverBus.SendTo(clientID, []byte("ACK"))
        })
    
    serverBus.Listen(":8080")
    serverBus.Start()
}
```

### 14.3 MultiBus使用

```go
func main() {
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
        AddBus(tcpBus, 2, "primary").   // 权重2
        AddBus(udpBus, 1, "backup").    // 权重1
        SetDefaultStrategy(voidbus.SendStrategy{
            Mode:           voidbus.ModeWeighted,
            EnableFragment: true,
            MaxFragmentSize: 1024,
        }).
        OnMessage(func(sourceBusID string, data []byte) {
            println("From", sourceBusID, ":", string(data))
        })
    
    multiBus.Start()
    
    // 加权随机多信道发送（自动分片）
    multiBus.Send([]byte("Large data..."))
    
    // 指定单一信道发送
    multiBus.SendVia("primary", []byte("Important data"))
}
```

---

## 15. 版本兼容性说明

### 15.1 接口稳定性保证

| 接口类型 | 稳定性 | 变更规则 |
|----------|--------|----------|
| **核心接口** | 稳定 | 主版本号变更才可修改 |
| **扩展接口** | 较稳定 | 次版本号变更可扩展 |
| **内部接口** | 不保证 | 可随时变更 |

### 15.2 废弃规则

```go
// 废弃接口标记示例
//
// Deprecated: Use NewInterface instead. Will be removed in v2.0.
type OldInterface interface {
    OldMethod() error
}
```

### 15.3 版本号规则

- **主版本号(Major)**: 不兼容的接口变更
- **次版本号(Minor)**: 新增功能，向后兼容
- **修订号(Patch)**: Bug修复，向后兼容

示例: `v1.2.3`
- 1: 主版本
- 2: 次版本（新增功能）
- 3: 修订版本（Bug修复）