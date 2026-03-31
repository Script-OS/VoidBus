# Fragment Package - 分片模块

分片模块负责大数据的分片和重组，是VoidBus四层分离架构的第四层。

**安全边界**: 部分可暴露 - FragmentInfo中的ID/Index/Total可暴露，Checksum不可暴露。

## 文件结构

```
fragment/
├── fragment.go       # Fragment接口 + FragmentManager实现
```

## 模块职责

### Fragment接口

**职责**：
- 负责大数据的分片和重组
- 管理分片元数据（ID/Index/Total）
- 支持分片完整性校验

**不负责**：
- 数据传输（由Channel负责）
- 数据序列化（由Serializer负责）
- 数据编码/加密（由Codec负责）

### FragmentManager

**职责**：
- 管理多个分片组的状态
- 处理分片的超时清理
- 支持并发重组

## 接口定义

```go
// Fragment - 分片接口
type Fragment interface {
    // Split 分片数据
    Split(data []byte, maxSize int) ([][]byte, error)
    
    // Reassemble 重组分片
    Reassemble(fragments [][]byte) ([]byte, error)
    
    // GetFragmentInfo 获取分片信息
    GetFragmentInfo(fragment []byte) (FragmentInfo, error)
    
    // SetFragmentInfo 设置分片信息
    SetFragmentInfo(data []byte, info FragmentInfo) ([]byte, error)
}

// FragmentInfo - 分片元数据（部分可暴露）
type FragmentInfo struct {
    ID        string    // UUID，随机无语义，可暴露
    Index     uint16    // 分片索引，可暴露
    Total     uint16    // 总分片数，可暴露
    Checksum  uint32    // 校验和，不可暴露
    IsLast    bool      // 是否最后一个分片
    Timestamp int64     // 时间戳
}

// FragmentConfig - 分片配置
type FragmentConfig struct {
    MaxFragmentSize    int           // 最大分片大小
    ReassemblyTimeout  time.Duration // 重组超时
    MaxPendingGroups   int           // 最大待重组组数
    CleanupInterval    time.Duration // 清理间隔
}

// DefaultFragmentConfig - 默认配置
var DefaultFragmentConfig = FragmentConfig{
    MaxFragmentSize:    64 * 1024,       // 64KB
    ReassemblyTimeout:  30 * time.Second,
    MaxPendingGroups:   1000,
    CleanupInterval:    10 * time.Second,
}
```

## 分片流程

### Split

```
原始数据
  → 计算Checksum
  → 生成FragmentID (UUID)
  → 按MaxFragmentSize切分
  → 为每个分片添加FragmentInfo头部
  → 返回分片数组
```

### Reassemble

```
分片数组
  → 按FragmentID分组
  → 按Index排序
  → 验证Total一致性
  → 验证完整性（所有分片都存在）
  → 验证Checksum
  → 拼接数据
  → 返回原始数据
```

## FragmentManager

```go
// FragmentManager - 分片管理器
type FragmentManager interface {
    // ProcessFragment 处理单个分片
    ProcessFragment(fragment []byte) (*ReassemblyResult, error)
    
    // GetPendingGroups 获取待重组组列表
    GetPendingGroups() []string
    
    // GetGroupStatus 获取组状态
    GetGroupStatus(groupID string) (*GroupStatus, error)
    
    // CancelGroup 取消重组
    CancelGroup(groupID string) error
    
    // Cleanup 清理超时组
    Cleanup() int
    
    // Stats 获取统计信息
    Stats() *FragmentStats
}

// ReassemblyResult - 重组结果
type ReassemblyResult struct {
    Completed bool       // 是否完成
    Data      []byte     // 完成后的数据
    GroupID   string     // 分片组ID
    Remaining int        // 剩余分片数
}

// GroupStatus - 组状态
type GroupStatus struct {
    GroupID     string
    Total       uint16
    Received    uint16
    Missing     []uint16
    FirstSeen   time.Time
    LastSeen    time.Time
    IsExpired   bool
}
```

## 依赖关系

```
fragment/
├── 依赖 → internal/    # ID生成, Checksum计算
└── 依赖 → errors.go    # 错误定义
```

## 使用示例

### 分片数据

```go
fragment := fragment.NewFragment(fragment.DefaultFragmentConfig)

// 分片大数据
largeData := make([]byte, 1024*1024) // 1MB
fragments, err := fragment.Split(largeData, 64*1024) // 64KB分片

// fragments是一个[][]byte，每个元素是一个分片
for i, f := range fragments {
    info, _ := fragment.GetFragmentInfo(f)
    log.Printf("Fragment %d: ID=%s, Index=%d, Total=%d", 
               i, info.ID, info.Index, info.Total)
}
```

### 重组数据

```go
// 重组分片
reassembled, err := fragment.Reassemble(fragments)

// 验证数据
if len(reassembled) == len(largeData) {
    log.Println("Reassembly successful")
}
```

### FragmentManager使用

```go
manager := fragment.NewFragmentManager(fragment.DefaultFragmentConfig)

// 处理接收到的分片
for {
    fragmentData, err := channel.Receive()
    if err != nil {
        break
    }
    
    result, err := manager.ProcessFragment(fragmentData)
    if err != nil {
        log.Println("Process error:", err)
        continue
    }
    
    if result.Completed {
        // 所有分片已接收，数据完整
        log.Println("Reassembly completed:", len(result.Data))
        handleCompleteData(result.Data)
    } else {
        // 还有分片待接收
        log.Printf("Received fragment, remaining: %d", result.Remaining)
    }
}

// 定期清理超时组
go func() {
    for {
        time.Sleep(manager.Config().CleanupInterval)
        cleaned := manager.Cleanup()
        log.Printf("Cleaned %d expired groups", cleaned)
    }
}()
```

### 在MultiBus中使用

MultiBus自动管理分片到信道的分配：

```go
multiBus := core.NewMultiBusBuilder().
    AddBus(tcpBus, 2, "primary").
    AddBus(udpBus, 1, "backup").
    SetStrategy(core.SendStrategy{
        Mode:           core.ModeWeighted,
        EnableFragment: true,
        MaxFragmentSize: 64 * 1024, // 64KB
    }).
    Build()

// 发送大数据，自动分片并分配到不同信道
multiBus.Send([]byte("Large data..."))

// 接收端自动重组
multiBus.OnMessage(func(sourceBusID string, data []byte) {
    log.Println("Received complete data:", len(data))
})
```