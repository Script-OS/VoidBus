# Tests 目录

此目录包含VoidBus的测试归档和Mock实现。

## 目录结构

```
tests/
├── mock/           # Mock实现
│   └ mocks.go      # 依赖注入测试用Mock实现
└── README.md       # 测试说明文档
```

## Mock实现

`tests/mock/mocks.go` 提供了以下Mock类型，用于依赖注入测试：

| Mock类型 | 说明 | 主要方法 |
|----------|------|----------|
| MockCodecManager | Codec管理器Mock | AddCodec, RandomSelect, MatchByHash, Negotiate |
| MockFragmentManager | Fragment管理器Mock | CreateSendBuffer, AdaptiveSplit, Reassemble |
| MockSessionManager | Session管理器Mock | CreateSendSession, GetSession, Exists |
| MockChannelPool | Channel池Mock | AddChannel, RandomSelect, GetAdaptiveMTU |
| MockAdaptiveTimer | 自适应定时器Mock | GetTimeout, RecordLatency, GetSRTT |

### 使用示例

```go
import (
    "github.com/Script-OS/VoidBus"
    "github.com/Script-OS/VoidBus/tests/mock"
)

func TestWithMock(t *testing.T) {
    // 创建Mock依赖集
    mocks := mock.MockDependencies()
    
    // 配置Mock行为
    mocks.CodecManager.FailRandomSelect = true
    
    // 使用依赖注入创建Bus
    deps := &voidbus.Dependencies{
        CodecManager:   mocks.CodecManager,
        ChannelPool:    mocks.ChannelPool,
        FragmentMgr:    mocks.FragmentManager,
        SessionMgr:     mocks.SessionManager,
        AdaptiveTimer:  mocks.AdaptiveTimer,
    }
    
    config := voidbus.DefaultBusConfig()
    bus, err := voidbus.NewWithDependencies(config, deps)
    // ...
}
```

### 行为控制

所有Mock实现支持以下行为控制：

```go
// 设置失败模式
mocks.CodecManager.FailAddCodec = true
mocks.ChannelPool.FailRandomSelect = true
mocks.FragmentManager.FailAdaptiveSplit = true

// 配置返回值
mocks.ChannelPool.RandomSelectID = "channel-1"
mocks.AdaptiveTimer.TimeoutResult = 5 * time.Second
mocks.FragmentManager.SplitResult = [][]byte{data}

// 重置为正常行为
mocks.ResetAll()

// 设置全部失败
mocks.SetFailAll()
```

## 测试文件分布

VoidBus遵循Go测试最佳实践，测试文件与源文件在同一目录：

| 测试文件 | 位置 | 说明 |
|----------|------|------|
| bus_test.go | 根目录 | Bus核心功能测试 |
| errors_test.go | 根目录 | 错误处理测试 |
| benchmark_test.go | 根目录 | 性能基准测试 |
| protocol/header_test.go | protocol/ | Header安全验证测试 |
| codec/*/test.go | codec/*/ | 各Codec实现测试 |
| internal/*_test.go | internal/ | 内部工具测试 |

## 运行测试

```bash
# 运行所有测试
go test ./...

# 运行测试并显示覆盖率
go test -cover ./...

# 运行性能基准测试
go test -bench=. -benchmem ./...

# 运行特定模块测试
go test -v ./protocol/...
go test -v ./codec/aes/...

# 运行tests/mock测试
go test -v ./tests/mock/...
```

## 测试覆盖率目标

| 模块 | 目标覆盖率 | 当前覆盖率 |
|------|-----------|-----------|
| bus.go | >85% | 32.5% |
| protocol/header.go | >90% | 89.3% |
| codec/manager.go | >80% | ~80% |
| errors.go | >80% | 高 |

## 性能基准测试结果

| Benchmark | 结果 |
|-----------|------|
| HeaderEncode | ~25 ns/op |
| HeaderDecode | ~85 ns/op |
| CodecChain_Plain | ~192 ns/op |
| CodecChain_Base64 | ~828 ns/op |
| CodecChain_AES | ~1264 ns/op |
| AdaptiveSplit_Small | ~114 ns/op |
| ComputeHash | ~69 ns/op |