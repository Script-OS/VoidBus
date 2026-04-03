# VoidBus Interactive Example

这个示例展示了如何使用 VoidBus 通过多个 channel（TCP、WebSocket、UDP）和多种 codec（base64、xor、aes、chacha20）进行双向通信。

## 架构特性演示

- **多 Channel 同时使用**：TCP/WS/UDP 同时连接，分片随机分布到不同 channel
- **多 Codec 随机组合**：每条消息使用随机 codec 链（最多 depth=2）
- **Session-based 关联**：Client 通过 SessionID 将所有 channel 关联到同一连接
- **健康权重选择**：ChannelPool 根据健康度加权随机选择 channel
- **优雅退出**：Ctrl+C 后正确清理所有 goroutine 和资源

## 运行方式

### 启动 Server

```bash
cd example/server
go run main.go
```

Server 将监听：
- TCP: :19000
- WebSocket: :19001
- UDP: :19002

### 启动 Client

```bash
cd example/client
go run main.go
```

Client 将连接到：
- TCP: 127.0.0.1:19000
- WebSocket: ws://127.0.0.1:19001
- UDP: 127.0.0.1:19002

### 测试多 Channel 分布

1. Server 和 Client 启动后，在 Client 输入消息
2. Server 会显示收到的消息并回复
3. 观察日志可以看到消息通过不同的 channel 发送（TCP/WS/UDP）

### 优雅退出测试

在 Server 或 Client 进程中按 Ctrl+C：
- 立即停止 Accept/Read 循环
- 关闭所有 channel 连接
- 等待所有 goroutine 退出
- 打印 "Server/Client stopped"

## 代码结构

### Server (`server/main.go`)

```go
// 1. 创建 Bus 并设置密钥
bus := voidbus.New(nil)
bus.SetKey(key)

// 2. 注册多种 codec
bus.RegisterCodec(base64.New())
bus.RegisterCodec(xor.New())
bus.RegisterCodec(aes.NewAES256Codec())
bus.RegisterCodec(chacha20.New())

// 3. 注册所有 server channel
bus.AddChannel(tcp.NewServerChannel(...))
bus.AddChannel(ws.NewServerChannel(...))
bus.AddChannel(udp.NewServerChannel(...))

// 4. Listen - 聚合所有 channel
listener := bus.Listen()

// 5. Accept 循环
for {
    conn := listener.Accept()  // 每个 conn 已经关联了所有 channel
    go handleClient(conn)
}
```

### Client (`client/main.go`)

```go
// 1. 创建 Bus 并设置密钥
bus := voidbus.New(nil)
bus.SetKey(key)

// 2. 注册多种 codec
bus.RegisterCodec(base64.New())
bus.RegisterCodec(xor.New())
bus.RegisterCodec(aes.NewAES256Codec())
bus.RegisterCodec(chacha20.New())

// 3. 注册所有 client channel
bus.AddChannel(tcp.NewClientChannel(...))
bus.AddChannel(ws.NewClientChannel(...))
bus.AddChannel(udp.NewClientChannel(...))

// 4. Dial - 使用所有 channel
conn := bus.Dial()  // 自动协商，所有 channel 关联到同一 Session

// 5. 发送/接收
conn.Write([]byte("Hello"))
buf := make([]byte, 4096)
n := conn.Read(buf)
```

## 多 Channel 分布原理

### Client 端

1. **首次 Negotiation**：通过第一个 channel（TCP）发送 NegotiateRequest（SessionID=空）
2. **获取 SessionID**：收到 NegotiateResponse 后获取 SessionID
3. **后续 Negotiation**：异步通过 WS/UDP 发送 NegotiateRequest（带上 SessionID）
4. **Channel 关联**：所有 channel 的 receiveLoop 都把数据汇总到同一个 recvQueue

### Server 端

1. **聚合 Listener**：voidBusListener 同时监听 TCP/WS/UDP
2. **首个连接**：第一个 channel 连接时立即 Accept 返回
3. **后续关联**：收到带 SessionID 的 NegotiateRequest，动态添加到已有 Session
4. **Channel 池**：每个 accepted conn 都有独立的 ChannelPool，包含所有已关联 channel

### 分片分布

每条消息发送时：
1. **切片**：FragmentManager.AdaptiveSplit() 根据最小 MTU 切片
2. **选择 Channel**：每个分片独立调用 ChannelPool.SelectChannel()
3. **健康权重**：初始 HealthScore=1.0，理论均匀分布
4. **故障切换**：连续失败时 HealthScore 下降，自动切换到其他 channel

## 验证多 Channel 分布

启用 DebugMode 可以看到详细的 channel 选择日志：

```go
bus.SetDebugMode(true)
```

日志示例：
```
[DEBUG] sendInternal: sending fragment 0 on channel udp-6285cdda
[DEBUG] sendInternal: sending fragment 1 on channel ws-fc4c2bba
[DEBUG] sendInternal: sending fragment 2 on channel tcp-e6ec75c8
```

## 注意事项

1. **密钥一致性**：Server 和 Client 必须使用相同的 32-byte 密钥
2. **Codec 注册**：Server 和 Client 必须注册相同的 codec
3. **UDP 限制**：每个 Session 只能有一个 UDP channel（Server 会拒绝重复 UDP 连接）
4. **MTU 计算**：UDP 实际可用 MTU = 1395 bytes（扣除 VoidBus Header + UDP Header）

## 故障排查

### 连接失败

检查端口是否被占用：
```bash
lsof -i :19000 -i :19001 -i :19002
```

### 消息丢失

启用 DebugMode 查看 channel 健康度：
```go
bus.SetDebugMode(true)
```

### 优雅退出卡住

检查 goroutine 是否全部退出：
```bash
# 发送 SIGQUIT 查看 goroutine stack
kill -QUIT <pid>
```