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
| **核心接口** | 系统运行必需的接口 | Channel, Codec |
| **扩展接口** | 增强功能的可选接口 | KeyAwareCodec |
| **管理接口** | 生命周期管理接口 | Bus, Module |
| **辅助接口** | 内部使用的接口 | SessionManager, FragmentManager |

---

## 2. Codec（编码/加密）接口规范

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

### 2.2 CodecChain接口

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

## 3. Channel（信道）接口规范

### 4.1 核心接口

```go
package channel

// ChannelType 信道类型标识
type ChannelType string

const (
    TypeTCP  ChannelType = "tcp"
    TypeUDP  ChannelType = "udp"
    TypeICMP ChannelType = "icmp"
    TypeWS   ChannelType = "ws"
    // TypeQUIC removed in v3.0 - simplification
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

## 4. KeyProvider（密钥提供者）接口规范

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

## 5. Fragment（分片）接口规范

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

## 6. Bus接口规范

### 6.1 核心 API（net.Conn/net.Listener 风格）

VoidBus v3.0 采用 Go 标准库风格的消息式通信接口。

```go
package voidbus

// Bus 统一总线接口
// 
// 提供两种模式:
//   - Client模式: 使用 Dial() 获取 net.Conn
//   - Server模式: 使用 Listen() 获取 net.Listener
type Bus struct {
    // 内部实现，不暴露接口
}

// New 创建 Bus 实例
//
// 参数:
//   - config: 配置结构，nil 时使用默认配置
//
// 返回值保证:
//   - 返回可用的 Bus 实例
func New(config *BusConfig) (*Bus, error)

// RegisterCodec 注册 Codec
//
// 参数约束:
//   - codec: 必须实现 Codec 接口
//   - codec.Code() 返回用户定义的代号
//
// 返回值保证:
//   - 注册成功后可用于编码
func (b *Bus) RegisterCodec(codec Codec) error

// AddChannel 添加 Channel
//
// 参数约束:
//   - channel: 必须实现 Channel 接口
//
// 返回值保证:
//   - 添加成功后可用于通信
func (b *Bus) AddChannel(channel Channel) error

// Dial 客户端连接
//
// 处理流程:
//   1. CreateNegotiateRequest (从注册 codecs 生成 Bitmap)
//   2. Send NegotiateRequest
//   3. Receive NegotiateResponse
//   4. ApplyNegotiateResponse
//   5. StartReceiveLoop
//   6. Return net.Conn
//
// 参数约束:
//   - ch: 已通过 AddChannel 添加的客户端 Channel
//
// 返回值保证:
//   - 返回已协商的 net.Conn
//   - Conn.Read: 每次返回一条完整消息
//   - Conn.Write: 每次发送一条完整消息
func (b *Bus) Dial(ch Channel) (net.Conn, error)

// Listen 服务端监听
//
// 参数约束:
//   - ch: 已通过 AddChannel 添加的 ServerChannel
//
// 返回值保证:
//   - 返回 net.Listener
//   - Listener.Accept: 返回已协商的客户端 net.Conn
func (b *Bus) Listen(ch Channel) (net.Listener, error)

// SetKey 设置密钥
//
// 用于需要密钥的 Codec (AES, ChaCha20 等)
func (b *Bus) SetKey(key []byte) error

// SetMaxCodecDepth 设置最大 Codec 链深度
func (b *Bus) SetMaxCodecDepth(depth int) error

// SetDebugMode 设置调试模式
//
// 启用后输出详细日志
func (b *Bus) SetDebugMode(enable bool)
```

### 6.2 VoidBusConn (net.Conn 实现)

```go
// VoidBusConn 实现 net.Conn 接口
// 提供消息式通信:
//   - Read: 返回一条完整消息（已重组、已解码）
//   - Write: 发送一条完整消息（自动编码、分片、多 Channel 分发）
type VoidBusConn struct {
    // 内部实现
}

// Read 读取一条完整消息
//
// 行为特性:
//   - 每次调用返回一条完整消息
//   - 自动处理: 分片重组 → Codec 解码 → 数据验证
//   - 阻塞直到收到完整消息或超时
//
// 参数约束:
//   - buf: 接收缓冲区，建议 4096+ 字节
//
// 返回值保证:
//   - n: 实际读取的字节数
//   - data: 完整、解码后的消息数据
func (c *VoidBusConn) Read(buf []byte) (n int, err error)

// Write 写入一条完整消息
//
// 行为特性:
//   - 每次调用发送一条完整消息
//   - 自动处理: Codec 编码 → 自适应分片 → 多 Channel 并行分发
//
// 参数约束:
//   - data: 要发送的消息数据
//
// 返回值保证:
//   - 数据已发送到对端（可靠传输）
func (c *VoidBusConn) Write(data []byte) (n int, err error)

// Close 关闭连接
func (c *VoidBusConn) Close() error

// LocalAddr 返回本地地址
func (c *VoidBusConn) LocalAddr() net.Addr

// RemoteAddr 返回对端地址
func (c *VoidBusConn) RemoteAddr() net.Addr

// SetDeadline 设置读写超时
//
// 超时适用于整条消息的重组/编码/发送
func (c *VoidBusConn) SetDeadline(t time.Time) error

// SetReadDeadline 设置读取超时
func (c *VoidBusConn) SetReadDeadline(t time.Time) error

// SetWriteDeadline 设置写入超时
func (c *VoidBusConn) SetWriteDeadline(t time.Time) error
```

### 6.3 VoidBusListener (net.Listener 实现)

```go
// VoidBusListener 实现 net.Listener 接口
type VoidBusListener struct {
    // 内部实现
}

// Accept 接受新客户端连接
//
// 行为特性:
//   - 阻塞等待新连接
//   - 自动处理协商流程
//   - 返回已协商的 net.Conn
//
// 返回值保证:
//   - 返回的 net.Conn 已完成协商
//   - 可直接用于 Read/Write
func (l *VoidBusListener) Accept() (net.Conn, error)

// Close 关闭监听器
func (l *VoidBusListener) Close() error

// Addr 返回监听地址
func (l *VoidBusListener) Addr() net.Addr
```

---

## 7. BusConfig 结构

```go
// BusConfig 总线配置
type BusConfig struct {
    // MaxCodecDepth 最大Codec链深度 (默认: 2)
    MaxCodecDepth int
    
    // DefaultMTU 默认MTU大小 (默认: 1024)
    DefaultMTU int
    
    // RecvBufferSize 接收缓冲区大小 (默认: 100)
    RecvBufferSize int
    
    // DebugMode 调试模式，输出详细日志
    DebugMode bool
}

// DefaultBusConfig 返回默认配置
func DefaultBusConfig() *BusConfig {
    return &BusConfig{
        MaxCodecDepth:  2,
        DefaultMTU:     1024,
        RecvBufferSize: 100,
        DebugMode:      false,
    }
}
```
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

---

## 8. Negotiate（能力协商）接口规范

### 9.1 NegotiateRequest结构

```go
package negotiate

// NegotiateRequest 客户端协商请求
type NegotiateRequest struct {
    // ChannelBitmap 信道类型位图
    // Bit 0=WS, 1=TCP, 2=UDP, 3=ICMP, 4=DNS, 5=HTTP, 6=Reserved, 7=Reserved
    // (QUIC removed in v3.0 - compact mapping)
    ChannelBitmap []byte
    
    // CodecBitmap Codec类型位图
    // Bit 0=Plain, 1=Base64, 2=AES256, 3=XOR, 4=ChaCha20, 5=RSA, 6=GZIP, 7=ZSTD
    CodecBitmap []byte
    
    // Timestamp 请求时间戳
    Timestamp int64
}

// NegotiateResponse 服务端响应
type NegotiateResponse struct {
    // ChannelBitmap 协商后的信道类型位图
    ChannelBitmap []byte
    
    // CodecBitmap 协商后的Codec类型位图
    CodecBitmap []byte
    
    // Timestamp 响应时间戳
    Timestamp int64
}
```

### 9.2 NegotiationPolicy结构

```go
// NegotiationPolicy 协商策略
type NegotiationPolicy struct {
    // DebugMode 是否为调试模式
    // Debug模式允许plaintext Codec
    DebugMode bool
    
    // MinSecurityLevel 最低安全等级
    // Release模式必须>=SecurityLevelMedium
    MinSecurityLevel SecurityLevel
    
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
        PreferredCodecChainSecurity: SecurityLevelNone,
        MaxCodecChainLength:        3,
        ChallengeTimeout:           60 * time.Second,
        HandshakeTimeout:           120 * time.Second,
        RejectOnMismatch:           false,
    }
}
```

---

## 10. SessionRegistry接口规范

### 10.1 SessionRegistry接口

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

### 10.2 SessionConfig结构

```go
// SessionConfig 会话配置
// 注意：此结构仅存储在本地，不通过网络传输
type SessionConfig struct {
    // SessionID 会话唯一标识
    SessionID string
    
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

## 11. 模块注册机制

### 11.1 Build Tags设计

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

### 11.2 编译命令示例

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

### 11.3 全局注册表设计

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

## 12. 错误类型定义

### 12.1 错误层次结构

VoidBus v2.0 采用统一的错误处理策略，支持错误严重程度分级和上下文信息。

```go
package voidbus

// ErrorSeverity 错误严重程度
type ErrorSeverity int

const (
    SeverityLow      ErrorSeverity = iota // 可恢复，不影响主流程
    SeverityMedium                         // 需处理，可能影响部分功能
    SeverityHigh                           // 严重影响，主要功能受阻
    SeverityCritical                       // 致命错误，无法继续运行
)

func (s ErrorSeverity) String() string {
    switch s {
    case SeverityLow:      return "LOW"
    case SeverityMedium:   return "MEDIUM"
    case SeverityHigh:     return "HIGH"
    case SeverityCritical: return "CRITICAL"
    default:               return "UNKNOWN"
    }
}

// VoidBusError 基础错误类型
type VoidBusError struct {
    Op      string    // 操作名称
    Module  string    // 模块名称
    Err     error     // 底层错误
    Msg     string    // 描述信息
}

func (e *VoidBusError) Error() string {
    if e.Err != nil {
        return fmt.Sprintf("[%s/%s] %s: %v", e.Module, e.Op, e.Msg, e.Err)
    }
    return fmt.Sprintf("[%s/%s] %s", e.Module, e.Op, e.Msg)
}

func (e *VoidBusError) Unwrap() error {
    return e.Err
}

// EnhancedVoidBusError 增强错误类型
// 支持严重程度和上下文信息
type EnhancedVoidBusError struct {
    *VoidBusError
    Severity    ErrorSeverity
    Recoverable bool
    Context     map[string]interface{}  // 上下文信息（类型安全建议使用固定字段）
}

func (e *EnhancedVoidBusError) Error() string {
    return fmt.Sprintf("[%s/%s] %s (severity: %s, recoverable: %v)",
        e.Module, e.Op, e.Msg, e.Severity.String(), e.Recoverable)
}

// 错误辅助函数
func IsVoidBusError(err error) bool
func GetModule(err error) string
func GetOperation(err error) string
func IsEnhancedError(err error) bool
func GetSeverity(err error) ErrorSeverity
func IsRecoverable(err error) bool
func IsCritical(err error) bool
func GetContext(err error) map[string]interface{}
```

### 12.2 错误包装函数

| 函数 | 用途 | 严重程度 | 示例 |
|------|------|----------|------|
| `NewError(op, module, err)` | 创建基础错误 | SeverityMedium | `NewError("Encode", "codec", err)` |
| `WrapError(op, module, err, msg)` | 包装错误并添加消息 | SeverityMedium | `WrapError("Send", "bus", err, "send failed")` |
| `WrapModuleError(op, module, err)` | 模块级错误包装 | SeverityMedium | `WrapModuleError("SelectChain", "codec", err)` |
| `MustWrap(op, module, err)` | 关键路径强制包装 | SeverityHigh | `MustWrap("Connect", "channel", err)` |
| `SoftWrap(op, module, err)` | 可选路径软包装 | SeverityLow | `SoftWrap("Cleanup", "session", err)` |
| `RecoverableError(op, module, err, msg)` | 可恢复错误 | SeverityMedium + Recoverable=true | `RecoverableError("Retry", "channel", err, "temporary failure")` |
| `CriticalError(op, module, err, msg)` | 致命错误 | SeverityCritical | `CriticalError("Init", "bus", err, "initialization failed")` |
| `WrapWithContext(op, module, err, msg, ctx)` | 带上下文包装 | SeverityMedium | `WrapWithContext("Process", "fragment", err, "fragment lost", map[string]interface{}{"fragmentID": "xxx"})` |

### 12.3 预定义错误

```go
var (
    // 通用错误
    ErrNotImplemented     = errors.New("not implemented")
    ErrInvalidConfig      = errors.New("invalid configuration")
    ErrModuleNotSet       = errors.New("module not set")
    
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
    ErrNoHealthyChannel   = errors.New("no healthy channel available")
    
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
    
    // Handshake/Negotiate错误
    ErrHandshakeFailed      = errors.New("handshake failed")
    ErrHandshakeTimeout     = errors.New("handshake timeout")
    ErrSecurityLevelMismatch = errors.New("security level mismatch")
    ErrChallengeFailed      = errors.New("challenge verification failed")
    ErrDegradationAttack    = errors.New("potential degradation attack detected")
    ErrNegotiationFailed    = errors.New("negotiation failed")
    
    // Bus错误
    ErrBusNotRunning      = errors.New("bus not running")
    ErrBusAlreadyRunning  = errors.New("bus already running")
    
    // Session错误
    ErrSessionNotFound    = errors.New("session not found")
    ErrSessionExpired     = errors.New("session expired")
)
```

### 12.4 错误处理最佳实践

```go
// 创建 Bus 时检查错误
bus, err := voidbus.New()
if err != nil {
    if voidbus.IsCritical(err) {
        log.Fatal("Bus initialization failed: ", err)
    }
    // 可恢复错误处理
    log.Warn("Bus created with warnings: ", err)
}

// 发送时处理错误
err = bus.Send(data)
if err != nil {
    if voidbus.IsRecoverable(err) {
        // 可恢复：重试或降级
        retryCount++
        if retryCount < maxRetry {
            continue
        }
    }
    // 不可恢复：记录并退出
    log.Error("Send failed: ", err)
    return err
}

// 模块级错误包装
func (m *CodecManager) SelectChain() (CodecChain, string, error) {
    chain, hash, err := m.randomSelect(m.maxDepth)
    if err != nil {
        return nil, "", WrapModuleError("SelectChain", "codec", err)
    }
    return chain, hash, nil
}
```

---

## 13. 使用示例

### 13.1 基本使用（客户端）

```go
package main

import (
    "fmt"
    "time"
    
    voidbus "github.com/Script-OS/VoidBus"
    "github.com/Script-OS/VoidBus/channel/tcp"
    "github.com/Script-OS/VoidBus/codec/base64"
    "github.com/Script-OS/VoidBus/codec/xor"
)

func main() {
    // 1. 创建 Bus
    bus, err := voidbus.New(nil)
    if err != nil {
        panic(err)
    }
    
    // 2. 注册 Codec
    bus.RegisterCodec(base64.New())
    bus.RegisterCodec(xor.New())
    
    // 3. 添加 Channel（配置包含目标地址）
    ch := tcp.NewClientChannel(&tcp.ClientConfig{
        Address:        "localhost:8080",
        ConnectTimeout: 5 * time.Second,
    })
    bus.AddChannel(ch)
    
    // 4. Dial 连接（自动执行协商）
    conn, err := bus.Dial(ch)
    if err != nil {
        panic(err)
    }
    defer conn.Close()
    
    // 5. 发送消息
    _, err = conn.Write([]byte("Hello, VoidBus!"))
    if err != nil {
        fmt.Println("Send error:", err)
        return
    }
    
    // 6. 接收消息
    buf := make([]byte, 4096)
    n, err := conn.Read(buf)
    if err != nil {
        fmt.Println("Receive error:", err)
        return
    }
    fmt.Println("Received:", string(buf[:n]))
}
```

### 13.2 服务端使用

```go
package main

import (
    "fmt"
    "net"
    "time"
    
    voidbus "github.com/Script-OS/VoidBus"
    "github.com/Script-OS/VoidBus/channel/tcp"
    "github.com/Script-OS/VoidBus/codec/base64"
)

func handleClient(conn net.Conn) {
    defer conn.Close()
    
    buf := make([]byte, 4096)
    for {
        // 设置读取超时（用于轮询）
        conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
        n, err := conn.Read(buf)
        
        if err != nil {
            if netErr, ok := err.(*net.OpError); ok && netErr.Timeout() {
                continue // 超时，继续轮询
            }
            fmt.Println("Client disconnected:", err)
            return
        }
        
        fmt.Println("Received:", string(buf[:n]))
        
        // 回复
        conn.Write([]byte("ACK: " + string(buf[:n])))
    }
}

func main() {
    // 1. 创建 Bus
    bus, err := voidbus.New(nil)
    if err != nil {
        panic(err)
    }
    
    // 2. 注册 Codec
    bus.RegisterCodec(base64.New())
    
    // 3. 添加 Server Channel
    serverCh := tcp.NewServerChannel(&tcp.ServerConfig{
        Address: ":8080",
    })
    bus.AddChannel(serverCh)
    
    // 4. Listen 监听
    listener, err := bus.Listen(serverCh)
    if err != nil {
        panic(err)
    }
    defer listener.Close()
    
    fmt.Println("Server listening on :8080")
    
    // 5. 接受客户端连接
    for {
        conn, err := listener.Accept()
        if err != nil {
            fmt.Println("Accept error:", err)
            continue
        }
        
        fmt.Println("New client connected")
        go handleClient(conn)
    }
}
```

### 13.3 完整交互式示例

参见 `example/interactive/` 目录下的完整示例代码：
- `client/main.go`: 完整的客户端示例
- `server/main.go`: 完整的服务端示例
---

## 14. 实现状态与安全约束

### 14.1 接口实现状态

| 接口 | 实现文件 | 状态 | 关键方法 |
|------|----------|------|----------|
| Codec | codec/plain/plain.go, base64/base64.go, aes/aes.go | ✅ | Encode/Decode/InternalID/SecurityLevel |
| KeyAwareCodec | codec/aes/aes.go | ✅ | SetKeyProvider/RequiresKey/KeyAlgorithm |
| CodecChain | codec/chain.go | ✅ | AddCodec/Encode/Decode/SecurityLevel/Clone |
| Channel | channel/tcp/tcp.go | ✅ | Send/Receive/Close/IsConnected/Type |
| ServerChannel | channel/tcp/tcp.go | ✅ | Accept/ListenAddress |
| KeyProvider | keyprovider/embedded/embedded.go | ✅ | GetKey/RefreshKey/SupportsRefresh/Type |
| FragmentManager | fragment/manager.go | ✅ | CreateBuffer/AddFragment/IsComplete/Reassemble |
| SessionManager | session/manager.go | ✅ | CreateSession/GetSession/CompleteSession |
| Bus | bus.go | ✅ | Dial/Listen/RegisterCodec/AddChannel |
| VoidBusConn (net.Conn) | conn.go | ✅ | Read/Write/Close/SetDeadline |
| VoidBusListener (net.Listener) | listener.go | ✅ | Accept/Close/Addr |

### 14.2 安全约束实现说明

#### Header（暴露字段）

```go
// protocol/header.go
type Header struct {
    SessionID     string    // ✅ 可暴露（UUID）
    FragmentIndex uint16    // ✅ 可暴露
    FragmentTotal uint16    // ✅ 可暴露
    CodecDepth    uint8     // ✅ 可暴露（链深度）
    CodecHash     [32]byte  // ✅ 可暴露（SHA-256哈希，不暴露具体组合）
    DataChecksum  uint32    // ✅ 可暴露
    DataHash      [32]byte  // ✅ 可暴露
    Timestamp     int64     // ✅ 可暴露
    Flags         uint8     // ✅ 可暴露
}
```

**安全验证**:
- CodecHash 基于 code 序列计算，不暴露具体 codec 名称
- MaxSessionIDLength = 64 防止内存耗尽攻击
- MaxFragmentTotal = 10000 防止过度分片
- Timestamp 验证防止重放攻击

#### Session（配置不可传输）

```go
// session/session.go
type Session struct {
    ID           string      // ✅ 可暴露（间接引用）
    CodecChain   CodecChain  // ❌ 仅存储本地
    Channel      Channel     // ❌ 仅存储本地
    KeyProvider  KeyProvider // ❌ 仅存储本地
    Config       SessionConfig // ❌ 仅存储本地
}
```

**验证方法**:
- Packet.Header 仅包含 SessionID
- SessionConfig 通过 SessionID 在本地 SessionManager 查找
- Session.Stats() 返回仅包含统计信息的可暴露数据
- 生产环境应使用完整 CodecChain.Encode()

### 16.3 接口使用注意事项

#### AddCodec/AddCodecAt（流式API设计）

```go
// codec/chain.go
// 当前设计：静默返回原链（流式API）
chain := codec.NewChain().
    AddCodec(aes.NewAES256GCM()).
    AddCodec(base64.New())

// 超限时（>=5个Codec）：静默忽略新添加
// 这是设计决策，支持链式调用

// 如需错误处理，建议添加：
type CodecChain interface {
    // AddCodecWithErr 添加Codec并返回错误
    // 返回 ErrChainTooLong 当链超限
    AddCodecWithErr(codec Codec) (CodecChain, error)
}
```

#### Clone（浅拷贝语义）

```go
// codec/chain.go
// Clone 返回浅拷贝
// Codec 实例共享，适用于无状态 Codec
clone := chain.Clone()

// 如 Codec 有状态（如 keyProvider），修改 clone 会影响原链
// 使用场景：创建新 Session 时复用 Codec 配置
```

#### SecurityLevel（短板原则）

```go
// codec/chain.go
func (c *DefaultChain) SecurityLevel() SecurityLevel {
    // 返回链中最低安全等级
    // 示例：AES-256(High) + Base64(Low) = Low
    // 整体安全性由最薄弱环节决定
}
```

### 16.4 协议版本兼容

| 版本 | Packet.Version | 兼容性 |
|------|----------------|--------|
| v1.0 | 1 | 当前版本，所有接口稳定 |
| v1.1 | 1 | 将支持 Fragment 增强功能 |
| v2.0 | 2 | 可能重定义 Packet 格式 |

**向后兼容策略**:
- Version 字段用于识别协议版本
- v2.x 可通过 Version=1 降级兼容
- 不兼容变更需主版本号升级

---

## 15. Module 接口类型安全约束（v3.0）

VoidBus v3.0 优化 Module 接口定义，确保类型安全。

### 15.1 类型安全原则

Module 接口（CodecModule、ChannelModule、FragmentModule、SessionModule）遵循以下原则：

- **类型明确**: 所有参数和返回值使用具体类型，避免 `interface{}`
- **编译时检查**: 类型错误在编译时发现，而非运行时类型断言
- **向后兼容**: 保持接口语义不变，仅替换类型定义

### 15.2 CodecModule 接口改进

**改进前**（使用 interface{}）：
```go
type CodecModule interface {
    Module
    
    AddCodec(codec interface{}, code string) error
    RandomSelect() (codes []string, chain interface{}, err error)
    MatchByHash(hash [32]byte) (codes []string, chain interface{}, err error)
}
```

**改进后**（类型安全）：
```go
type CodecModule interface {
    Module
    
    // 明确类型参数：Codec 接口而非 interface{}
    AddCodec(codec codec.Codec, code string) error
    
    // 明确返回类型：CodecChain 接口而非 interface{}
    RandomSelect() (codes []string, chain codec.CodecChain, err error)
    MatchByHash(hash [32]byte) (codes []string, chain codec.CodecChain, err error)
}
```

### 15.3 ChannelModule 接口改进

**改进前**（使用 interface{}）：
```go
type ChannelModule interface {
    Module
    
    AddChannel(channel interface{}, id string) error
    RandomSelect() (interface{}, error)
    SelectHealthy() (interface{}, error)
}
```

**改进后**（类型安全）：
```go
type ChannelModule interface {
    Module
    
    // 明确类型参数：Channel 接口而非 interface{}
    AddChannel(channel channel.Channel, id string) error
    
    // 明确返回类型：Channel 接口而非 interface{}
    RandomSelect() (channel.Channel, error)
    SelectHealthy() (channel.Channel, error)
}
```

### 15.4 FragmentModule 和 SessionModule 改进

类似的改进应用于 FragmentModule 和 SessionModule：

- FragmentModule: 所有 `interface{}` 参数替换为 `fragment.Buffer` 等具体类型
- SessionModule: 所有 `interface{}` 参数替换为 `session.Session` 等具体类型

### 15.5 类型安全收益

| 方面 | 改进前 | 改进后 |
|------|--------|--------|
| 类型错误发现 | 运行时（类型断言） | 编译时 |
| IDE 支持 | 无类型推断 | 完整类型推断 |
| 代码维护 | 类型断言代码 | 无类型断言 |
| 性能 | 类型断言开销 | 无开销 |

### 15.6 Module 接口实现状态

| 接口 | 实现文件 | 状态 | 类型安全 |
|------|----------|------|----------|
| Module | module.go | ✅ 已实现 | 🔄 待改进（替换 interface{}） |
| CodecModule | module.go | ✅ 已定义 | 🔄 待改进 |
| ChannelModule | module.go | ✅ 已定义 | 🔄 待改进 |
| FragmentModule | module.go | ✅ 已定义 | 🔄 待改进 |
| SessionModule | module.go | ✅ 已定义 | 🔄 待改进 |

---

*最后更新：2026-04-03*