# VoidBus 锁使用最佳实践

本文档记录VoidBus项目中锁使用的设计原则和最佳实践，确保代码的可维护性和并发安全。

---

## 核心原则

### 1. 锁顺序原则
**所有嵌套锁必须遵循统一的获取顺序**，避免循环依赖导致死锁。

VoidBus中的锁层级顺序：
```
Manager锁 → Component锁 → Channel锁
```

例如：
- `Bus.mu` → `ChannelPool.mu` → `Channel.mu`
- `ServerChannel.mu` → `AcceptedChannel.mu`

**错误示例**：
```go
// 错误：在持有channel.mu时调用server方法
func (a *AcceptedChannel) Close() error {
    a.mu.Lock()              // 持有channel锁
    a.server.removeClient()  // server方法需要server锁 → 反向顺序 → 死锁！
    a.mu.Unlock()
}
```

**正确示例**：
```go
// 正确：先释放锁，再调用server方法
func (a *AcceptedChannel) Close() error {
    a.mu.Lock()
    server := a.server
    id := a.id
    a.mu.Unlock()           // 先释放锁
    
    if server != nil {
        server.removeClient(id) // server锁 → 安全
    }
}
```

---

### 2. 最小持锁时间原则
**锁应快速获取、立即释放**，避免在持锁状态下执行：
- 阻塞I/O操作
- 外部方法调用
- 长时间计算
- 等待goroutine完成

**正确示例**（Receive方法）：
```go
func (c *ClientChannel) Receive() ([]byte, error) {
    c.mu.RLock()
    if c.closed {
        c.mu.RUnlock()
        return nil, ErrClosed
    }
    stream := c.stream       // 复制引用
    c.mu.RUnlock()           // 立即释放锁
    
    // 阻塞I/O在锁外进行
    data, err := io.ReadFull(stream, buf)
    ...
}
```

---

### 3. 复制-释放-操作模式
当需要调用外部方法时，采用**三步模式**：

1. **复制**：在锁内复制必要的数据引用
2. **释放**：释放锁
3. **操作**：在锁外调用外部方法

**应用场景**：
- Channel.Close() 调用 Server.removeClient()
- ServerChannel.Close() 调用 Client.Close()
- Manager操作调用Component方法

**示例代码**：
```go
// ServerChannel.Close() - 复制client列表后锁外关闭
func (s *ServerChannel) Close() error {
    s.mu.Lock()
    if s.closed {
        s.mu.Unlock()
        return ErrClosed
    }
    s.closed = true
    
    // 复制client列表
    clients := make([]*AcceptedChannel, 0, len(s.clients))
    for _, client := range s.clients {
        clients = append(clients, client)
    }
    s.clients = make(map[string]*AcceptedChannel)
    
    s.mu.Unlock()           // 先释放锁
    
    // 在锁外关闭clients
    for _, client := range clients {
        client.Close()
    }
    
    return nil
}
```

---

### 4. 两阶段清理模式
**GC清理操作应缩小锁范围**，避免长时间阻塞其他操作：

**错误示例**：
```go
// 错误：长时间持写锁
func (m *FragmentManager) CleanupExpired() int {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    for id, buf := range m.sendBuffers {  // 遍历时间：O(n)
        if buf.IsExpired() {
            delete(m.sendBuffers, id)     // 写锁阻塞所有操作
        }
    }
}
```

**正确示例**：
```go
// 正确：两阶段清理
func (m *FragmentManager) CleanupExpired() int {
    // 阶段1：读锁快速识别（快）
    m.mu.RLock()
    expiredIDs := make([]string, 0)
    for id, buf := range m.sendBuffers {
        if buf.IsExpired() {
            expiredIDs = append(expiredIDs, id)
        }
    }
    m.mu.RUnlock()
    
    // 阶段2：写锁批量删除（短）
    m.mu.Lock()
    for _, id := range expiredIDs {
        delete(m.sendBuffers, id)
    }
    m.mu.Unlock()
    
    return len(expiredIDs)
}
```

**性能对比**：
- 错误方式：写锁持有时间 = O(n)遍历 + O(k)删除
- 正确方式：读锁时间 = O(n)遍历（允许并发读），写锁时间 = O(k)删除

---

## 已正确实现的设计

### 1. Channel.Receive() - 锁外阻塞I/O
所有Channel实现（TCP/WS/QUIC/UDP）的Receive方法都遵循：
- 读锁内快速检查状态
- 复制连接引用
- 立即释放锁
- 锁外进行阻塞I/O

这是**教科书式正确做法**。

### 2. TCP ServerChannel.Close() - 复制-释放-操作
已正确实现复制client列表后锁外关闭。

### 3. Listener双重检查模式
快速检查状态后立即释放锁，避免不必要的锁竞争。

---

## 已修复的问题

### P0级修复（2026-04-02）

| 文件 | 方法 | 问题 | 修复方案 |
|------|------|------|----------|
| tcp/tcp.go | AcceptedChannel.Close() | 持锁调用server.removeClient() | 复制-释放-操作 |
| ws/ws.go | AcceptedChannel.Close() | 同上 | 同上 |
| quic/quic.go | AcceptedChannel.Close() | 同上 | 同上 |
| quic/quic.go | ServerChannel.Close() | 持锁调用client.Close() | 复制列表-释放-锁外关闭 |
| ws/ws.go | ServerChannel.Close() | 同上 | 同上 |

### P2级修复（2026-04-02）

| 文件 | 方法 | 问题 | 修复方案 |
|------|------|------|----------|
| fragment/manager.go | CleanupExpired() | 长时间持写锁 | 两阶段清理 |
| session/manager.go | CleanupExpired() | 同上 | 同上 |

---

## 代码审查检查清单

在修改涉及锁的代码时，必须检查：

1. ✅ 是否有嵌套锁？锁顺序是否一致？
2. ✅ 是否在锁内调用了外部方法？需要复制-释放-操作吗？
3. ✅ 是否在锁内进行了阻塞I/O？需要锁外操作吗？
4. ✅ 是否长时间持有写锁？可以用两阶段清理吗？
5. ✅ 是否有goroutine启动/等待？应在锁外进行吗？

---

## 5. 状态转换的锁使用原则（v3.0）

VoidBus v3.0 使用单一状态枚举管理 Bus 状态，状态转换遵循明确的锁使用原则。

### 5.1 状态枚举定义

```go
type BusState int32

const (
    StateIdle BusState = iota        // 初始状态
    StateConnected BusState = iota   // 已连接，未协商
    StateNegotiated BusState = iota  // 已协商，准备通信
    StateRunning BusState = iota     // 运行中
    StateClosed BusState = iota      // 已关闭
)
```

### 5.2 状态转换锁原则

**原则**: 状态转换方法 `setState()` 已持有 `b.mu` 锁，避免双重加锁。

**正确示例**:
```go
// 状态转换方法（已持锁）
func (b *Bus) setState(newState BusState) error {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    currentState := BusState(b.state.Load())
    
    // 状态转换验证
    switch currentState {
    case StateIdle:
        if newState != StateConnected && newState != StateRunning {
            return ErrInvalidStateTransition
        }
    ...
    }
    
    b.state.Store(int32(newState))
    return nil
}

// 外部调用（不持锁）
func (b *Bus) Dial(ch Channel) (net.Conn, error) {
    // 先进行其他操作（不持锁）
    ...
    
    // 状态转换时才持锁（setState 内部持锁）
    if err := b.setState(StateConnected); err != nil {
        return nil, err
    }
    
    ...
}
```

**错误示例**:
```go
// 错误：双重加锁
func (b *Bus) Dial(ch Channel) (net.Conn, error) {
    b.mu.Lock()
    defer b.mu.Unlock()  // 外部已持锁
    
    // setState 内部又会持锁 → 死锁！
    if err := b.setState(StateConnected); err != nil {
        return nil, err
    }
}
```

### 5.3 状态查询的锁原则

**原则**: 状态查询使用 atomic 操作，无需加锁。

**正确示例**:
```go
// 状态查询（无锁）
func (b *Bus) getState() BusState {
    return BusState(b.state.Load())
}

func (b *Bus) isRunning() bool {
    return b.getState() == StateRunning
}

// 外部快速检查（无锁）
func (b *Bus) Send(data []byte) error {
    if !b.isRunning() {
        return ErrBusNotRunning
    }
    
    // 继续发送逻辑...
}
```

### 5.4 状态转换验证

状态转换必须通过验证规则，防止非法转换：

| 当前状态 | 允许的目标状态 | 禁止的转换 |
|---------|---------------|-----------|
| StateIdle | StateConnected, StateRunning | StateNegotiated, StateClosed |
| StateConnected | StateNegotiated, StateClosed | StateIdle, StateRunning |
| StateNegotiated | StateRunning, StateClosed | StateIdle, StateConnected |
| StateRunning | StateClosed | 所有其他状态 |
| StateClosed | 无（禁止） | 所有状态 |

---

## 参考资料

- Go并发编程：https://go.dev/blog/pipelines
- 锁顺序原则：https://en.wikipedia.org/wiki/Lock_(computer_science)#Lock_ordering
- VoidBus架构文档：ARCHITECTURE.md

---

*文档维护：每次锁相关修复后更新此文档*
*最后更新：2026-04-03*