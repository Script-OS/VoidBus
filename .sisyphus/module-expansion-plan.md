# VoidBus 模块扩展实施计划

## 目标
扩展 VoidBus 的 Channel、Codec、Serializer 模块，增加更多传输协议、加密算法和序列化格式。

---

## Phase 1: 新增 Channel 模块

### 1.1 UDP Channel (`channel/udp/udp.go`)
**职责**: 无连接、低延迟传输
**特点**:
- 基于 UDP 协议
- 无连接建立开销
- 不保证送达和顺序
- 适用场景：实时性要求高、可容忍丢包

**实现内容**:
- `ClientChannel`: UDP 客户端
- `ServerChannel`: UDP 服务端
- 帧协议：4 字节长度前缀 + 数据
- MaxFrameSize: 65KB

**API**:
```go
config := channel.ChannelConfig{
    Address:    "localhost:9090",
    BufferSize: 65535,
}
udpChannel := udp.NewClientChannel(config)
```

---

### 1.2 WebSocket Channel (`channel/ws/ws.go`)
**职责**: 基于 WebSocket 全双工通信
**特点**:
- TCP 长连接
- 支持 HTTP 升级握手
- 适合 Web 浏览器通信
- 可穿越防火墙

**实现内容**:
- `ClientChannel`: WebSocket 客户端
- `ServerChannel`: WebSocket 服务端
- 依赖：`golang.org/x/net/websocket`
- MessageFrame 协议

**依赖添加**:
```bash
go get golang.org/x/net/websocket
```

---

### 1.3 QUIC Channel (`channel/quic/quic.go`) (可选高阶功能)
**职责**: 基于 UDP 的低延迟、多路复用传输
**特点**:
- 0-RTT 连接建立
- 内置 TLS 加密
- 多路复用无队头阻塞
- 适用场景：高延迟网络、实时通信

**依赖**: `github.com/quic-go/quic-go`

---

## Phase 2: 新增 Codec 模块

### 2.1 ChaCha20-Poly1305 Codec (`codec/chacha20/chacha20.go`)
**职责**: 现代流式加密算法
**特点**:
- 安全等级：High
- 256 位密钥
- 性能优于 AES（无硬件加速环境）
- 适用于移动设备

**实现内容**:
- `ChaCha20Codec`: 实现 Codec 和 KeyAwareCodec
- KeySize: 32 字节
- NonceSize: 12 字节
- 输出格式：nonce + ciphertext + tag

**API**:
```go
codec := chacha20.NewChaCha20Poly1305()
codec.SetKeyProvider(embedded.New(key))
```

---

### 2.2 RSA Codec (`codec/rsa/rsa.go`) (可选高阶功能)
**职责**: 非对称加密
**特点**:
- 安全等级：High
- 用于密钥交换、数字签名
- 性能较低，不适合大量数据
- 通常与对称加密组合使用

**实现内容**:
- `RSACodec`: 实现 Codec 和 KeyAwareCodec
- 支持 RSA-2048、RSA-4096
- 需要 KeyProvider 提供私钥

---

### 2.3 XOR Codec (`codec/xor/xor.go`)
**职责**: 简单混淆（低安全等级）
**特点**:
- 安全等级：None (调试用) 或 Low (简单混淆)
- 极高性能
- 适用于非安全场景的简单干扰
- 不推荐用于生产环境

**实现内容**:
- `XORCodec`: 实现 Codec 接口
- Key: 单字节或短密钥循环使用
- 输出格式：原始数据长度不变

---

## Phase 3: 新增 Serializer 模块

### 3.1 Protobuf Serializer (`serializer/protobuf/protobuf.go`)
**职责**: Protocol Buffers 序列化
**特点**:
- 优先级：80 (最高)
- 二进制格式，体积小
- 速度快
- 需要预定义 schema

**依赖**: `google.golang.org/protobuf`

**实现内容**:
- `ProtobufSerializer`: 实现 Serializer 接口
- 支持动态消息或预定义 proto
- Name: "protobuf"
- Priority: 80

---

### 3.2 MessagePack Serializer (`serializer/msgpack/msgpack.go`)
**职责**: MessagePack 二进制序列化
**特点**:
- 优先级：60
- 比 JSON 更紧凑
- 支持更多数据类型
- 无需预定义 schema

**依赖**: `github.com/vmihailenco/msgpack/v5`

**实现内容**:
- `MessagePackSerializer`: 实现 Serializer 接口
- Name: "msgpack"
- Priority: 60

---

## 实施优先级

### Priority 1 (核心功能)
1. ✅ UDP Channel - 基础传输协议补充
2. ✅ ChaCha20-Poly1305 Codec - 现代对称加密
3. ✅ Protobuf Serializer - 高性能序列化

### Priority 2 (增强功能)
4. ✅ WebSocket Channel - Web 支持
5. ✅ MessagePack Serializer - 紧凑序列化
6. ✅ XOR Codec - 简单混淆

### Priority 3 (可选高阶)
7. ⏸️ QUIC Channel - 高阶传输（复杂度高）
8. ⏸️ RSA Codec - 非对称加密（复杂度中等）

---

## 技术约束与规范

### 代码结构
每个模块遵循统一模式：
```
module/
├── <module>.go       # 主要实现
├── module_test.go    # 单元测试
└── README.md         # 模块文档
```

### 命名规范
- Package: 小写（`udp`, `chacha20`, `protobuf`）
- 类型: 驼峰（`ClientChannel`, `ChaCha20Codec`）
- InternalID: 下划线分隔（`codec_chacha20_poly1305`）

### 注册机制
每个模块在 `init()` 中自动注册到全局 Registry。

### 安全性要求
- Codec.SecurityLevel 必须准确：
  - ChaCha20Poly1305: High
  - RSA-2048+: High
  - XOR: None (仅调试)
- Release 模式 `MinSecurityLevel >= Medium`

---

## 交付成果

1. **新增 8 个模块**（3 个 Channel + 3 个 Codec + 2 个 Serializer）
2. **单元测试覆盖**每个模块核心功能
3. **文档更新**:
   - 各模块 README.md
   - 项目根 README.md 更新
   - docs/ARCHITECTURE.md 更新

---

## 风险与挑战

### 中等风险
- **WebSocket 依赖**: 需要添加外部依赖 `golang.org/x/net/websocket`
- **Protobuf 使用**: 需要定义消息格式

### 低风险
- 所有模块遵循现有接口规范
- 通过 Registry 解耦，不影响现有代码

---

## 实施步骤

1. **创建模块目录结构**
2. **实现核心功能**（按 Priority 顺序）
3. **编写单元测试**
4. **更新文档**
5. **验证集成测试**

---

## 时间预估

- Priority 1 (UDP + ChaCha20 + Protobuf): 2-3 天
- Priority 2 (WebSocket + MessagePack + XOR): 1-2 天
- Priority 3 (QUIC + RSA): 3-4 天（可选）

总计：**6-9 天**（完成全部）

---

## 下一步行动

1. [ ] 确认优先级
2. [ ] 实施 Priority 1 模块
3. [ ] 编写测试验证
4. [ ] 更新文档
5. [ ] 实施 Priority 2 模块
6. [ ] 评估是否需要 Priority 3

---

**备注**: 所有实现将遵循 VoidBus 四层分离架构原则，确保模块边界清晰、接口契约严格。
