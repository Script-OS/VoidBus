# Channel Package - 信道模块

信道模块负责底层传输层的建立和维护，是VoidBus四层分离架构的第三层。

**安全边界**: ❌ 不可暴露 - Channel类型不通过网络传输，仅通过SessionID间接引用。

## 文件结构

```
channel/
├── interface.go      # Channel接口定义
├── channel.go        # ChannelRegistry, ServerChannelRegistry
└── tcp/              # TCP传输实现
    └── tcp.go
```

## 模块职责

### Channel接口

**职责**：
- 负责底层传输层的建立和维护
- 处理网络连接的生命周期
- 提供数据的发送和接收接口
- 支持心跳保活机制

**不负责**：
- 数据的序列化（由Serializer负责）
- 数据编码/加密（由Codec负责）
- 数据分片（由Fragment负责）
- 密钥管理（由KeyProvider负责）

### ServerChannel接口

**职责**：
- 服务端监听和接受连接
- 为每个客户端创建独立的Channel实例

## 接口定义

```go
// Channel - 信道接口
type Channel interface {
    // Send 发送数据
    Send(data []byte) error
    
    // Receive 接收数据
    Receive() ([]byte, error)
    
    // Close 关闭连接
    Close() error
    
    // IsConnected 检查连接状态
    IsConnected() bool
    
    // Type 信道类型（内部标识，不可传输）
    Type() ChannelType
}

// ServerChannel - 服务端信道接口
type ServerChannel interface {
    Channel
    
    // Accept 接受客户端连接，返回新的Channel
    Accept() (Channel, error)
    
    // ListenAddress 获取监听地址
    ListenAddress() string
}

// ChannelType - 信道类型
type ChannelType string

const (
    ChannelTypeTCP   ChannelType = "tcp"
    ChannelTypeUDP   ChannelType = "udp"
    ChannelTypeICMP  ChannelType = "icmp"
    ChannelTypeWS    ChannelType = "websocket"
    ChannelTypeQUIC  ChannelType = "quic"
)
```

## 已实现模块

### TCP Channel

位置: `channel/tcp/tcp.go`

**特点**：
- TCP传输实现
- 使用length-prefix帧协议（4字节长度 + N字节数据）
- MaxFrameSize = 16MB
- 支持心跳保活（可选）

**帧协议格式**：
```
[Length:4bytes(uint32)][Data:Nbytes]
```

**组件**：
- `ClientChannel`: 客户端连接
- `ServerChannel`: 服务端监听
- `AcceptedChannel`: 已接受的客户端连接

```go
// 客户端连接
clientChannel := tcp.NewClientChannel("server:8080")
err := clientChannel.Connect()
clientChannel.Type()  // "tcp"

// 发送数据（自动添加length-prefix）
err := clientChannel.Send([]byte("Hello"))

// 接收数据（自动解析length-prefix）
data, err := clientChannel.Receive()

// 关闭连接
clientChannel.Close()

// 服务端监听
serverChannel := tcp.NewServerChannel(":8080")
err := serverChannel.Listen()

// 接受客户端连接
acceptedChannel, err := serverChannel.Accept()
acceptedChannel.Type()  // "tcp"
```

## 待实现模块

- `channel/udp/` - UDP传输
- `channel/icmp/` - ICMP传输
- `channel/ws/` - WebSocket传输
- `channel/quic/` - QUIC传输

## ChannelRegistry

```go
// 客户端注册表
registry := channel.NewChannelRegistry()
registry.Register("tcp", tcp.NewClientChannel)
clientChannel, err := registry.Create("tcp", "server:8080")

// 服务端注册表
serverRegistry := channel.NewServerChannelRegistry()
serverRegistry.Register("tcp", tcp.NewServerChannel)
serverChannel, err := serverRegistry.Create("tcp", ":8080")
```

## 依赖关系

```
channel/
├── 无外部模块依赖
└── 依赖 → errors.go    # 错误定义
```

## 使用示例

### 在Bus中使用

```go
// 客户端
bus := core.NewBuilder().
    UseSerializerInstance(plain.New()).
    UseCodecChain(codecChain).
    UseChannel(tcp.NewClientChannel("server:8080")).  // 直接使用实例
    // 或
    UseChannelType("tcp", "server:8080").             // 通过Registry创建
    Build()

// 服务端
serverBus := core.NewServerBusBuilder().
    SetSerializer(plain.New()).
    SetCodecChain(codecChain).
    SetServerChannel(tcp.NewServerChannel(":8080")).  // 直接使用实例
    // 或
    SetChannelType("tcp", ":8080").                   // 通过Registry创建
    Build()
```

### 服务端接受连接

ServerBus会自动调用Accept循环，为每个客户端创建独立的Bus实例：

```go
serverBus := core.NewServerBusBuilder().
    SetServerChannel(tcp.NewServerChannel(":8080")).
    OnClientConnect(func(clientID string, bus core.Bus) {
        log.Println("Client connected:", clientID)
        // bus是该客户端的独立Bus实例
    }).
    OnClientMessage(func(clientID string, data []byte) {
        // 处理客户端消息
        serverBus.SendTo(clientID, []byte("ACK"))
    }).
    Build()

serverBus.Listen(":8080")
serverBus.Start()  // 启动Accept循环
```