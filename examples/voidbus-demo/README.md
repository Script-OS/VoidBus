# VoidBus Demo

VoidBus 多通道分片传输演示程序。

## 功能特性

- ✅ 独立编译启动的 client 和 server
- ✅ 多 TCP channel 并行传输（默认 4 个通道）
- ✅ 自动分片传输（大数据自动分片）
- ✅ 随机分片分发（AllRandomDistributor）
- ✅ 多 Codec Chain 组合（Base64+AES-128, Base64+AES-256）
- ✅ 自动解密重组还原原始数据
- ✅ 双向全双工通信
- ✅ Echo 测试验证数据完整性

## 目录结构

```
voidbus-demo/
├── main.go                 # 统一入口（通过 -mode 区分 client/server）
├── go.mod
├── go.sum
├── README.md
└── internal/
    ├── config/
    │   └── config.go       # 配置定义
    ├── codec/
    │   └── setup.go        # Codec Chain 池构建
    ├── client/
    │   └── client.go       # Client 核心逻辑
    └── server/
        └── server.go       # Server 核心逻辑
```

## 编译

```bash
cd voidbus-demo
go build -o voidbus-demo .
```

## 使用方法

### 启动 Server

```bash
./voidbus-demo -mode=server -addr=:8080 -channels=4
```

参数说明：
- `-mode=server`: 服务器模式
- `-addr=:8080`: 监听地址（默认 :8080）
- `-channels=4`: TCP channel 数量（默认 4）
- `-timeout=30`: 读取超时时间（秒）

### 启动 Client

```bash
./voidbus-demo -mode=client -addr=localhost:8080 -channels=4 -size=51200
```

参数说明：
- `-mode=client`: 客户端模式
- `-addr=localhost:8080`: 服务器地址
- `-channels=4`: TCP channel 数量（默认 4）
- `-size=51200`: 测试数据大小（bytes，默认 50KB）
- `-timeout=30`: 读取超时时间（秒）

## 完整测试流程

### 终端 1：启动 Server

```bash
cd examples/voidbus-demo
./voidbus-demo -mode=server -addr=:8080 -channels=4
```

### 终端 2：启动 Client

```bash
cd examples/voidbus-demo
./voidbus-demo -mode=client -addr=localhost:8080 -channels=4 -size=51200
```

## 预期输出

### Server 输出

```
Server: 启动配置 - 监听=:8080, channels=4
Server: Built 2 codec chains
Server: Listening on :8080
Server: Selected codec chain with security level 3
Server: Started and ready
Server: Waiting for client connections...
Server: Client channel 0 connected
Server: Client channel 1 connected
Server: Client channel 2 connected
Server: Client channel 3 connected
Server: All 4 client channels connected
Server: Starting receiver goroutine for channel 0
Server: Starting receiver goroutine for channel 1
Server: Starting receiver goroutine for channel 2
Server: Starting receiver goroutine for channel 3
Server: 等待客户端发送数据...
Server: Received packet (fragment: true)
Server: Received fragment 0/51 (ID: ...)
...
Server: 收到数据 (51200 bytes)
Server: 回显数据给客户端...
Server: Sent packet 0 via channel 2
...
Server: 回显完成！
```

### Client 输出

```
Client: 启动配置 - 地址=localhost:8080, channels=4, 数据大小=51200 bytes
Client: Built 2 codec chains
Client: Connecting to localhost:8080 with 4 channels...
Client: Channel 0 connected
Client: Channel 1 connected
Client: Channel 2 connected
Client: Channel 3 connected
Client: Selected codec chain with security level 3
Client: Connected and ready
Client: 生成了 51200 bytes 测试数据
Client: Prepared 51 packets for 51200 bytes
Client: Sent packet 0 via channel 0
...
Client: 等待服务器回显数据...
Client: Received packet (fragment: true)
Client: Received fragment 0/51 (ID: ...)
...
Client: ✓ 成功接收并验证回显数据 (51200 bytes)
Client: 测试完成！
```

## 技术实现

### 数据流

**发送端**:
```
原始数据 → Serializer → CodecChain.Encode → Fragment.Split → AllRandomDistributor → Channel.Send
```

**接收端**:
```
Channel.Receive → Packet.Decode → FragmentManager → Reassemble → CodecChain.Decode → Serializer
```

### Codec Chain 池

- **Chain 1**: Base64 + AES-128-GCM (SecurityLevel: Medium)
- **Chain 2**: Base64 + AES-256-GCM (SecurityLevel: High)

### 分片策略

使用 `protocol.AllRandomDistributor`:
- 每个分片随机选择 (Channel, CodecChain) 组合
- 最大化传输多样性
- 对端自动重组

### 安全特性

- MinSecurityLevel: Medium
- 禁用 Plain codec
- 使用 AES-GCM  authenticated encryption
- Base64 编码确保传输安全

## 测试建议

| 测试场景 | 数据大小 | Channels | 预期分片数 |
|---------|---------|----------|-----------|
| 小数据测试 | 1KB | 2 | 1-2 |
| 中等数据 | 10KB | 4 | 10-15 |
| 大数据测试 | 100KB | 4 | 100-150 |
| 极限测试 | 1MB | 4 | 1000+ |

## 依赖

- Go 1.24+
- github.com/Script-OS/VoidBus

## 故障排除

### 连接失败

确保 Server 已启动并监听正确端口：
```bash
netstat -an | grep 8080
```

### 超时错误

增加超时时间：
```bash
./voidbus-demo -mode=client -timeout=60 ...
```

### 数据不匹配

检查：
1. Server 和 Client 使用相同的 channels 数量
2. 网络稳定，无丢包
3. Codec chains 配置一致

## License

MIT
