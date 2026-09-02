# bytebufferpool

[English README](./README.md)

`bytebufferpool` 是一个面向 Go 的、行为确定且具备明确所有权语义的字节存储池。

这是一个采用全新 API 的 clean-room 实现，并不是 `github.com/valyala/bytebufferpool` 的直接替代品。

## 为什么还要实现一个 Pool？

- 使用显式且不可变的 Capacity Classes，而不是根据历史流量自动校准。
- 默认使用 Lease API，携带 Pool provenance、Generation 和重复 Release 状态。
- 为接受较弱生命周期保证的调用者提供名称明确的 Raw Slice 低层接口。
- 在同一个 Pool 契约下提供 Fast 和 Bounded 两种保留模式。
- Bounded 模式提供严格的 Retained Capacity 预算。
- 可选完整 capacity 清零、增强 Raw Slice 验证和运行统计。
- 提供可复现的稳态与峰值内存证据，不宣称“适用于所有场景的最快实现”。

库模块没有第三方运行时依赖，支持 Go 1.22 及以上版本。

## 安装

```text
go get github.com/ymj4023/bytebufferpool
```

## 优先使用 Lease

```go
pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
if err != nil {
	return err
}

lease := pool.Acquire(1500)
defer lease.Release()

payload := lease.Bytes() // 默认 classes 下 len=1500，cap=2048
copy(payload, source)
```

Lease 是不可复制的值。`Bytes` 只在 Release 之前有效。重复 Release 返回 `RejectedDuplicate`；Release 后调用其他方法会 panic。

处理不可信长度时使用 `TryAcquire`，它会返回 `ErrInvalidSize`。`Acquire` 遇到相同的程序或配置错误时会 panic。

## Raw Slice

```go
payload := pool.AcquireSlice(1500)
defer pool.ReleaseSlice(payload)
```

Raw Slice 与 Lease 具有相同的长度和 Capacity Class 契约，但不携带 Generation token。当同一个 backing address 被重新借出后，它无法可靠地区分旧 alias。启用 `ValidationEnabled` 可以拒绝可观察到的 foreign、cross-Pool、duplicate 和 changed-capacity Release；它有助于诊断，但不能提供内存安全。

如果 append 更换了 Backing Storage，不要把新 slice 当成原借用对象归还。

## Buffer

```go
buffer := pool.Buffer(1024)
defer buffer.Release()

_, _ = buffer.WriteString("hello")
_ = buffer.WriteByte(' ')
_, _ = buffer.Write([]byte("world"))
_, _ = buffer.WriteTo(destination)
```

Buffer 不可复制，内部拥有一个 Lease，并实现 `io.Writer`、`io.ByteWriter`、`io.StringWriter`、`io.ReaderFrom` 和 `io.WriterTo`。扩容时会获取新 Lease、复制有效内容并立即释放旧 Lease。当没有 Capacity Class 能满足请求时，Buffer 会按几何方式预留 capacity；由此产生的 unpooled Backing Storage 不会被保留，每次 acquisition 仍受 `MaxAcquireSize` 限制。扩容失败不会修改已有内容。

## Capacity 与保留策略

默认 Capacity Classes 是从 64 B 到 1 MiB 的 2 的幂。Acquire 选择第一个可以容纳请求的 class；Release 根据 capacity 路由，oversize 和 non-class storage 会被丢弃。

```go
classes, err := bytebufferpool.PowerOfTwo(256, 1<<20)
if err != nil {
	return err
}

config := bytebufferpool.DefaultConfig(bytebufferpool.Bounded)
config.Classes = classes
config.MaxPooledCapacity = 1 << 20
config.MaxRetainedCapacity = 32 << 20
config.MaxAcquireSize = 64 << 20

pool, err := bytebufferpool.New(config)
```

| 模式 | 保留方式 | 库存可观测性 |
| --- | --- | --- |
| Fast | 每个 Capacity Class 使用一个 runtime-managed pool | best-effort；无法提供精确保留量 |
| Bounded | 每级 LIFO 共享一个全局字节容量预算 | 精确的空闲 Backing Storage 数量和 Retained Capacity |

Retained Capacity 指空闲 Backing Storage 的 `sum(cap)`，不等于 allocator overhead、Go heap、`HeapSys` 或进程 RSS。

`Clear` 会丢弃当前空闲存储并推进 Generation。Clear 前获取的 Lease 会返回 `DroppedStale`；Clear 前获取的 Raw Slice 因为不携带 Generation，只能提供 best-effort 语义。

## 清零、验证与统计

```go
config := bytebufferpool.DefaultConfig(bytebufferpool.Bounded)
config.ZeroOnRelease = true
config.ValidationEnabled = true
config.StatsEnabled = true
```

- `ZeroOnRelease` 会清除每个有效 Release 的完整 capacity，即使该存储随后被丢弃。
- foreign 和 duplicate 验证失败不会修改传入存储。
- 同时启用时，清零优先于诊断填充。
- 可选 counters 记录 acquire、hit、miss、Release 结果、验证拒绝、清零字节数和每级活动。
- 即使关闭可选 counters，Bounded inventory 仍然可用，因为它是执行预算所必需的状态。

## ReleaseStatus

Release 会返回以下状态之一：

- `Retained`
- `DroppedFull`
- `DroppedOversize`
- `DroppedInvalid`
- `DroppedStale`
- `RejectedForeign`
- `RejectedDuplicate`
- `IgnoredNil`

在 Fast 模式下，`Retained` 只表示 best-effort runtime pool 接受了该值；runtime 随时可以丢弃它。

## Benchmark 结果

以下数据来自 Windows/amd64、Go 1.26.7、AMD Ryzen 9 8945HX。两个固定尺寸表使用 10 个样本和 `-benchtime=1s -cpu=1,8`；生命周期表使用 10 个样本和 `-benchtime=20x -cpu=1,8`。下表展示 CPU=1 的结果，并且只比较表中明确列出的 workload。

### Raw requested-length API — 1 KiB

| 对比项 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `make` | 174.0 | 1024 | 1 |
| 带 cutoff 的 `sync.Pool` | 24.59 | 0 | 0 |
| Project Fast Lease | 71.77 | 0 | 0 |
| Project Fast Raw | 65.29 | 0 | 0 |
| Project Bounded Raw | 58.47 | 0 | 0 |
| libp2p v0.1.0 | 48.06 | 0 | 0 |
| gRPC v1.83.2，acquire 时清零 | 47.18 | 0 | 0 |
| Prometheus v0.314.0 | 135.8 | 48 | 2 |

### Append Buffer — 以 128-byte chunks 写入 16 KiB

| 对比项 | µs/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| 新建 `bytes.Buffer` | 7.502 | 32.02 KiB | 10 |
| 带 cutoff 的 pooled `bytes.Buffer` | 1.756 | 96 | 1 |
| valyala v1.0.0 | 1.568 | 96 | 1 |
| bpool SizedBufferPool | 6.618 | 28.14 KiB | 5 |
| Project Fast Buffer | 3.763 | 96 | 1 |
| Project Bounded Buffer | 3.882 | 96 | 1 |

本项目在这些 workload 中并非全面最快。它的主要差异是确定性容量、显式所有权、旧 Generation 拒绝，以及可选的精确 Retained Capacity 预算。

### 生命周期与预算 — Project Fast Raw，1 KiB

| 状态 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Cold | 712.5 | 1,378 | 4 |
| Warm | 32.50 | 0 | 0 |
| PostGC | 502.5 | 353 | 5 |
| Bounded 两次 Release 预算耗尽 | 110.0 | 8 | 1 |

### 并发 8 MiB × 8 峰值

| 对比项 | Peak held HeapAlloc | Peak released HeapAlloc | GC2 HeapAlloc | 精确 Retained Capacity |
| --- | ---: | ---: | ---: | ---: |
| 带 cutoff 的 `sync.Pool` | 64.66 MiB | 64.66 MiB | 0.66 MiB | 不可用 |
| Project Fast | 64.67 MiB | 64.68 MiB | 0.66 MiB | 不可用 |
| Project Bounded | 64.67 MiB | 64.68 MiB | 0.67 MiB | 1 KiB |
| libp2p v0.1.0 | 64.68 MiB | 64.69 MiB | 0.67 MiB | 不可用 |
| gRPC v1.83.2 | 64.66 MiB | 64.67 MiB | 0.66 MiB | 不可用 |
| Prometheus v0.314.0 | 64.82 MiB | 64.82 MiB | 0.66 MiB | 不可用 |

峰值内存套件让 11 个 contenders 分别在全新 child process 中运行，每个重复 3 次。它保存了 33 个原始结果、66 个阶段汇总和 33 个 GC2 heap profiles，全部绑定到 clean revision `a433aa511f19328771019507f3e9fd622a796bb4`。Bounded 在稳态保留 1 KiB 空闲存储，并且从未超过 32 MiB 预算；runtime heap 指标与 Pool inventory 始终分开解释。

- [稳态原始输出与 benchstat 汇总](./benchmarks/results/2026-08-30-windows-amd64-go1.26.7-ryzen9-8945hx/steady/README.md)
- [峰值内存原始结果与 profiles](./benchmarks/results/2026-08-30-windows-amd64-go1.26.7-ryzen9-8945hx/README.md)
- [Benchmark 方法与命令](./benchmarks/README.md)
- [市场与源码调研](./docs/research/byte-buffer-pools.md)

## 归属与 clean-room 边界

源码参考、版本、许可证和实现差异记录在 [`docs/attribution.md`](./docs/attribution.md) 中。受到非显然设计启发的代码旁也有 `Design reference:` 注释。库中没有复制第三方实现或测试。

## License

MIT
