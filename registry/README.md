# Registry Package - 注册表

注册表模块管理VoidBus的会话配置和模块注册。

## 文件结构

```
registry/
└── registry.go    # SessionRegistry实现
```

## 模块职责

### SessionRegistry

**职责**：
- 存储Session配置（SessionID → SessionConfig映射）
- 管理客户端连接状态
- 提供配置查找接口

**不负责**：
- 网络连接管理
- 数据传输

## SessionConfig结构

SessionConfig存储本地配置信息，**不通过网络传输**：

```go
type SessionConfig struct {
    SessionID      string
    ClientID       string
    Serializer     serializer.Serializer
    CodecChain     codec.CodecChain
    Channel        channel.Channel
    KeyProvider    keyprovider.KeyProvider
    CreatedAt      time.Time
    LastActive     time.Time
    IsActive       bool
}
```

## 接口定义

```go
type SessionRegistry interface {
    // 注册管理
    Register(sessionID string, config *SessionConfig) error
    Unregister(sessionID string) error
    
    // 配置查询
    GetConfig(sessionID string) (*SessionConfig, error)
    GetSerializer(sessionID string) (serializer.Serializer, error)
    GetCodecChain(sessionID string) (codec.CodecChain, error)
    GetChannel(sessionID string) (channel.Channel, error)
    
    // 状态管理
    UpdateLastActive(sessionID string) error
    SetActive(sessionID string, active bool) error
    
    // 批量操作
    ListSessions() []string
    CountSessions() int
    ClearInactive(timeout time.Duration) int
}
```

## 安全设计

### 配置隔离
- SessionConfig仅存储在本地Registry
- 不通过网络传输Codec/Channel配置
- SessionID是随机UUID，无语义信息

### 间接引用
- Packet.Header.SessionID → Registry查找 → 本地SessionConfig
- 外部仅能看到SessionID，无法推断实际配置

## 依赖关系

```
registry/
├── 依赖 → serializer/  # Serializer接口
├── 依赖 → codec/       # CodecChain接口
├── 依赖 → channel/     # Channel接口
├── 依赖 → keyprovider/ # KeyProvider接口
├── 依赖 → internal/    # ID生成
└── 依赖 → errors.go    # 错误定义
```

## 使用示例

### 创建Registry

```go
registry := registry.NewSessionRegistry()
```

### 注册Session

```go
config := &registry.SessionConfig{
    SessionID:  internal.GenerateSessionID(),
    ClientID:   clientID,
    Serializer: serializer,
    CodecChain: codecChain,
    Channel:    channel,
    CreatedAt:  time.Now(),
    IsActive:   true,
}

err := registry.Register(config.SessionID, config)
```

### 查询配置

```go
// 通过SessionID查询完整配置
config, err := registry.GetConfig(sessionID)

// 查询特定组件
serializer, err := registry.GetSerializer(sessionID)
codecChain, err := registry.GetCodecChain(sessionID)
channel, err := registry.GetChannel(sessionID)
```

### 状态管理

```go
// 更新最后活跃时间
registry.UpdateLastActive(sessionID)

// 设置连接状态
registry.SetActive(sessionID, false)

// 清理超时Session
cleaned := registry.ClearInactive(5 * time.Minute)
log.Println("Cleaned", cleaned, "inactive sessions")
```

### 批量操作

```go
// 获取所有SessionID列表
sessions := registry.ListSessions()

// 获取Session数量
count := registry.CountSessions()
```