# Serializer Package - 序列化模块

序列化模块负责数据结构的序列化与反序列化，是VoidBus四层分离架构的第一层。

**安全边界**: ✅ 可暴露 - SerializerType可出现在元数据协议中。

## 文件结构

```
serializer/
├── interface.go      # Serializer接口定义
├── serializer.go     # SerializerModule, SerializerRegistry
└── plain/            # Pass-through序列化实现
    └── plain.go
```

## 模块职责

### Serializer接口

**职责**：
- 负责数据结构的序列化与反序列化
- 提供序列化类型标识（Name可暴露）
- 提供优先级（用于协商排序）

**不负责**：
- 数据编码/加密（由Codec负责）
- 数据传输（由Channel负责）
- 数据分片（由Fragment负责）

## 接口定义

```go
// Serializer - 序列化器接口
type Serializer interface {
    // Serialize 序列化数据
    Serialize(data []byte) ([]byte, error)
    
    // Deserialize 反序列化数据
    Deserialize(data []byte) ([]byte, error)
    
    // Name 序列化器名称（可暴露在元数据中）
    Name() string
    
    // Priority 优先级，用于协商排序（数值越大优先级越高）
    Priority() int
}

// SerializerModule - 可注册的序列化器模块
type SerializerModule interface {
    Serializer
    
    // Init 初始化模块
    Init(config SerializerConfig) error
    
    // ValidateConfig 验证配置
    ValidateConfig(config SerializerConfig) error
}

// SerializerRegistry - 序列化器注册表
type SerializerRegistry interface {
    Register(name string, serializer SerializerModule) error
    Get(name string) (Serializer, error)
    List() []string
    Default() Serializer
    SetDefault(name string) error
}
```

## 配置结构

```go
type SerializerConfig struct {
    Name     string
    Priority int
    Options  map[string]interface{}
}
```

## 已实现模块

### Plain Serializer

位置: `serializer/plain/plain.go`

**特点**：
- Pass-through序列化器，无转换
- 直接返回原始数据
- Priority = 0（最低优先级）
- 用于调试或不需要序列化的场景

```go
// 创建Plain Serializer
serializer := plain.New()

// 序列化（直接返回原数据）
data := []byte("Hello, VoidBus!")
serialized, err := serializer.Serialize(data) // serialized == data

// 反序列化（直接返回原数据）
original, err := serializer.Deserialize(serialized) // original == data

// 获取名称
name := serializer.Name() // "plain"

// 获取优先级
priority := serializer.Priority() // 0
```

## 待实现模块

- `serializer/json/` - JSON序列化
- `serializer/protobuf/` - Protobuf序列化
- `serializer/msgpack/` - MessagePack序列化

## 依赖关系

```
serializer/
├── 无外部模块依赖
└── 依赖 → errors.go    # 错误定义
```

## 使用示例

### 注册自定义Serializer

```go
registry := serializer.NewSerializerRegistry()

// 注册Plain Serializer
registry.Register("plain", plain.New())

// 设置默认
registry.SetDefault("plain")

// 获取Serializer
s, err := registry.Get("plain")

// 获取默认Serializer
defaultSerializer := registry.Default()
```

### 在Bus中使用

```go
bus := core.NewBuilder().
    UseSerializerInstance(plain.New()).  // 直接使用实例
    // 或
    UseSerializer("plain").              // 通过Registry获取
    UseCodecChain(codecChain).
    UseChannel(channel).
    Build()
```

### 协商过程

在ServerBus的Handshake协商中，Serializer.Name()会被暴露用于协商：

```go
// 客户端发送支持的Serializer列表
request := &protocol.HandshakeRequest{
    SupportedSerializers: []string{"json", "plain", "protobuf"},
    ...
}

// 服务端选择最高优先级的Serializer
selectedSerializer := selectHighestPriority(request.SupportedSerializers)
response := &protocol.HandshakeResponse{
    SelectedSerializer: selectedSerializer.Name(),
    ...
}
```