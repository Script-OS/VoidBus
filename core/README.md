# Core Package - 核心组装层

核心组装层是VoidBus的顶层模块，负责组装和管理各个功能模块实例，协调数据流在模块间的传递。

## 文件结构

```
core/
├── interfaces.go   # Bus/ServerBus/MultiBus接口定义
├── bus.go          # Bus核心实现
├── serverbus.go    # ServerBus实现（服务端Hall模式）
└── multibus.go     # MultiBus实现（多信道组合）
```

## 模块职责

### Bus

**职责**：
- 组装和管理各个模块实例（Serializer、CodecChain、Channel、Fragment）
- 协调数据流在模块间的传递
- 提供统一的使用接口
- 管理双向通信

**不负责**：
- 具体的传输实现（由Channel负责）
- 具体的序列化实现（由Serializer负责）
- 具体的编解码实现（由Codec负责）
- 具体的分片逻辑（由Fragment负责）

### ServerBus

**职责**：
- 监听并接受客户端连接
- 管理多个客户端Bus实例
- 执行安全协商（Handshake）
- 防止降级攻击

**不负责**：
- 具体业务逻辑处理
- 客户端配置选择

### MultiBus

**职责**：
- 管理多个Bus实例
- 支持分片到信道的分配（随机/轮询/加权/指定）
- 聚合所有信道接收的消息

**不负责**：
- 具体数据传输
- 分片策略细节（由Fragment负责）

## 接口定义

### Bus接口

```go
type Bus interface {
    // 生命周期
    Start() error
    Stop() error
    IsRunning() bool
    
    // 数据传输
    Send(data []byte) error
    Receive() ([]byte, error)
    
    // 消息处理
    OnMessage(handler MessageHandler) Bus
    
    // 状态查询
    GetSessionID() string
    GetSerializer() serializer.Serializer
    GetCodecChain() codec.CodecChain
    GetChannel() channel.Channel
}
```

### ServerBus接口

```go
type ServerBus interface {
    // 生命周期
    Listen(address string) error
    Start() error
    Stop() error
    IsRunning() bool
    
    // 客户端管理
    SendTo(clientID string, data []byte) error
    Broadcast(data []byte) error
    ListClients() []string
    ClientCount() int
    DisconnectClient(clientID string) error
    
    // 事件回调
    OnClientConnect(handler ClientConnectHandler) ServerBus
    OnClientDisconnect(handler ClientDisconnectHandler) ServerBus
    OnClientMessage(handler ClientMessageHandler) ServerBus
    OnError(handler ErrorHandler) ServerBus
}
```

### MultiBus接口

```go
type MultiBus interface {
    // 生命周期
    Start() error
    Stop() error
    IsRunning() bool
    
    // Bus管理
    AddBus(bus Bus, weight int, name string) MultiBus
    RemoveBus(name string) MultiBus
    GetBus(name string) Bus
    ListBuses() []string
    
    // 数据传输
    Send(data []byte) error
    SendVia(name string, data []byte) error
    Receive() ([]byte, error)
    
    // 策略配置
    SetStrategy(strategy SendStrategy) MultiBus
    
    // 事件回调
    OnMessage(handler MultiBusMessageHandler) MultiBus
    OnError(handler ErrorHandler) MultiBus
}
```

## 构建器模式

所有核心组件都使用Builder模式创建：

```go
// BusBuilder
bus := NewBuilder().
    UseSerializerInstance(serializer).
    UseCodecChain(codecChain).
    UseChannel(channel).
    WithConfig(BusConfig{...}).
    OnMessage(handler).
    Build()

// ServerBusBuilder
serverBus := NewServerBusBuilder().
    SetNegotiationPolicy(policy).
    SetSerializer(serializer).
    SetCodecChain(codecChain).
    SetKeyProvider(keyProvider).
    OnClientConnect(handler).
    OnClientDisconnect(handler).
    OnClientMessage(handler).
    OnError(handler).
    Build()

// MultiBusBuilder
multiBus := NewMultiBusBuilder().
    AddBus(bus1, 2, "primary").
    AddBus(bus2, 1, "backup").
    SetStrategy(strategy).
    OnMessage(handler).
    Build()
```

## 依赖关系

```
core/
├── 依赖 → protocol/     # Packet, Handshake, Policy
├── 依赖 → registry/     # SessionRegistry
├── 依赖 → serializer/   # Serializer接口
├── 依赖 → codec/        # Codec, CodecChain接口
├── 依赖 → channel/      # Channel接口
├── 依赖 → fragment/     # Fragment接口
├── 依赖 → keyprovider/  # KeyProvider接口
└── 依赖 → internal/     # ID生成, Challenge验证
├── 依赖 → errors.go     # 全局错误定义
```

## 使用示例

### 基本客户端

```go
bus := NewBuilder().
    UseSerializerInstance(plain.New()).
    UseCodecChain(codec.NewChain().AddCodec(aes.NewAES256GCM())).
    UseChannel(tcp.NewClientChannel("server:8080")).
    WithConfig(BusConfig{
        AutoReconnect: true,
        HeartbeatInterval: 30 * time.Second,
    }).
    OnMessage(func(data []byte) {
        log.Println("Received:", string(data))
    }).
    Build()

if err := bus.Start(); err != nil {
    log.Fatal(err)
}

bus.Send([]byte("Hello, VoidBus!"))
```

### 服务端

```go
serverBus := NewServerBusBuilder().
    SetNegotiationPolicy(protocol.DefaultNegotiationPolicy()).
    SetSerializer(plain.New()).
    SetCodecChain(codec.NewChain().AddCodec(aes.NewAES256GCM())).
    SetKeyProvider(embedded.New(key)).
    OnClientConnect(func(clientID string, bus Bus) {
        log.Println("Client connected:", clientID)
    }).
    OnClientMessage(func(clientID string, data []byte) {
        log.Println("Message from", clientID)
        serverBus.SendTo(clientID, []byte("ACK"))
    }).
    Build()

serverBus.Listen(":8080")
serverBus.Start()
```

### 多信道组合

```go
multiBus := NewMultiBusBuilder().
    AddBus(tcpBus, 2, "primary").
    AddBus(udpBus, 1, "backup").
    SetStrategy(SendStrategy{
        Mode: ModeWeighted,
        EnableFragment: true,
        MaxFragmentSize: 1024,
    }).
    OnMessage(func(sourceBusID string, data []byte) {
        log.Println("From", sourceBusID, ":", string(data))
    }).
    Build()

multiBus.Start()
multiBus.Send([]byte("Large data..."))       // 加权随机发送
multiBus.SendVia("primary", []byte("Important")) // 指定信道发送
```