# Session Package - 会话管理模块

会话模块负责管理 VoidBus v2.0 的 Session 生命周期和状态。

## 文件结构

```
session/
├── manager.go        # SessionManager实现
├── session.go        # Session结构定义
└── README.md         # 本文档
```

## Session状态机

```
Created → Sending → WaitingACK → Completed
    │         │          │
    └─────→ Expired ←────┘
```

## 模块职责

### SessionManager

**职责**：
- Send Session创建和管理
- Receive Session创建和管理
- Session生命周期管理
- 过期Session清理

### Session

**职责**：
- Session状态管理
- Codec信息记录（Codes、Hash、Depth）
- Fragment统计（Total、Sent）
- 重传计数
Send(data)
  │
  ├─→ 创建 Session（UUID）
  ├─→ 创建 SendBuffer（保留原始数据）
  ├─→ 选择 Codec 链（随机，记录代号组合）
  ├─→ 切片分发（多 Channel 随机发送）
  │
  │  (接收端处理)
  │
  ├─→ 接收分片
  ├─→ 检测缺失 → 发送 NAK
  ├─→ 重组解码
  ├─→ 发送 END_ACK
  │
  └─→ 销毁 Session（发送端收到 END_ACK）
      清理 Buffer
```

## Session 状态机

```
Created → Sending → WaitingACK → Completed
    │         │          │
    └─────→ Expired ←────┘
```

## 文件结构

```
session/
├── session.go     # Session 结构定义
├── manager.go     # SessionManager 实现
└── README.md      # 本文档
```

## 接口定义

```go
// SessionState 定义Session状态
type SessionState int

const (
    StateCreated SessionState = iota
    StateSending
    StateWaitingACK
    StateCompleted
    StateExpired
)

// Session 表示一次数据传输的会话
type Session struct {
    ID           string
    State        SessionState
    CreatedAt    time.Time
    UpdatedAt    time.Time
    
    // Codec 信息
    CodecCodes   []string
    CodecHash    [32]byte
    
    // 统计信息
    TotalFragments uint16
    SentFragments  uint16
    Retransmits    int
}
```