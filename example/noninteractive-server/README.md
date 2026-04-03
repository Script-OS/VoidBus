# VoidBus Non-Interactive File Transfer Example

这个示例演示了如何使用 VoidBus 传输大文件（~10MB），并显示完整的日志信息。

## 特性

- **多 Channel 同时使用**：TCP/WS/UDP 同时连接，分片自动分布
- **多 Codec 随机组合**：最大深度限制为 3
- **完整日志**：显示 channel 分布、codec 链、传输速率
- **自动生成测试文件**：如果 test_file.bin 不存在，自动创建 10MB 文件

## 运行方式

### 1. 启动 Server

```bash
cd example/noninteractive-server
go run main.go
```

Server 将监听：
- TCP: :19000
- WebSocket: :19001
- UDP: :19002

### 2. 启动 Client（另一个终端）

```bash
cd example/noninteractive-client
go run main.go
```

或者指定服务器地址：

```bash
go run main.go 192.168.1.100
```

## 传输流程

采用双向确认机制，确保数据完整传输：

```
Client                              Server
  │                                   │
  │──── Connect (TCP + WS + UDP) ────→│
  │                                   │
  │──── Phase 1: Send file ──────────→│
  │      Send file size (8 bytes)     │
  │      Send file data (~10MB)       │
  │                                   │
  │←── ACK: TransferComplete Magic ───│ Server确认接收完成
  │                                   │
  │←─── Phase 2: Receive file ────────│
  │      Receive file size (8 bytes)  │
  │      Receive file data (~10MB)    │
  │                                   │
  │──── ACK: TransferComplete Magic ─→│ Client确认接收完成
  │                                   │
  │←──────── Transfer complete ──────→│
```

**确认机制说明**：
- 使用 Go 的 channel + goroutine 通知机制，避免简单的 `time.Sleep` 等待
- `TransferCompleteMagic` = `"DONE1234"` (8字节固定长度)
- 超时时间：5分钟（与 ReadDeadline 一致）

## 日志示例

### Server 日志

```
2024/01/15 10:30:00.123456 === VoidBus Non-Interactive Server ===
2024/01/15 10:30:00.123457 Starting server...
2024/01/15 10:30:00.123458 Encryption key set: 32 bytes
2024/01/15 10:30:00.123459 Max codec depth: 3
2024/01/15 10:30:00.123460 Debug mode: enabled
2024/01/15 10:30:00.123461 Registered codecs: base64, xor, aes, chacha20
2024/01/15 10:30:00.123462 Server channels: TCP:19000, WS:19001, UDP:19002
2024/01/15 10:30:00.123463 Waiting for client connection...
2024/01/15 10:30:05.123456 Client connected: tcp-abc123

=== Phase 1: Receiving file from client ===
2024/01/15 10:30:05.124000 Incoming file size: 10485760 bytes (10.00 MB)
2024/01/15 10:30:05.234567 File received: 10485760 bytes in 110.567ms
2024/01/15 10:30:05.234568 Receive rate: 90.45 MB/s
Client send info:
  Channels: [TCP:35, WS:33, UDP:32]
  Codec:    [aes->base64]
  Fragments: 100
  Data size: 10485760 bytes

=== Phase 2: Sending file to client ===
2024/01/15 10:30:05.234600 File to send: test_file.bin (10485760 bytes, 10.00 MB)
2024/01/15 10:30:05.345678 File sent: 10485760 bytes in 111.078ms
2024/01/15 10:30:05.345679 Send rate: 89.55 MB/s
Server send info:
  Channels: [TCP:34, WS:33, UDP:33]
  Codec:    [chacha20->xor]
  Fragments: 100
  Data size: 10485760 bytes

=== Transfer complete ===
Received: received_file.bin (10485760 bytes)
Sent: test_file.bin (10485760 bytes)
```

## 输出文件

- `test_file.bin` - 如果不存在，程序会自动创建 10MB 测试文件
- `received_file.bin` - 接收到的文件

## 验证文件完整性

```bash
# 计算原始文件 hash
shasum -a 256 test_file.bin

# 计算接收文件 hash（应该相同）
shasum -a 256 received_file.bin
```

## 配置参数

代码中的关键参数：

```go
const (
    tcpPort = 19000
    wsPort  = 19001
    udpPort = 19002
)

const key = "voidbus-file-transfer-test-key-32!"

// 最大 codec 深度
bus.SetMaxCodecDepth(3)

// 启用 debug 模式显示详细信息
bus.SetDebugMode(true)
```

## 注意事项

1. **密钥一致性**：Server 和 Client 必须使用相同的 32-byte 密钥
2. **端口可用**：确保 19000-19002 端口未被占用
3. **文件大小**：默认 10MB，可在 createTestFile() 中修改
4. **编解码链**：最大深度限制为 3，减少开销