# Go byte buffer pool 一手资料调研

调研日期：2026-08-29

## 结论先行

本项目既然选择「全新 API，不兼容 `valyala/bytebufferpool`」，就不应复制它的 `Get() *ByteBuffer` / `Put(*ByteBuffer)` 和运行时自校准策略。更适合的新起点是：

1. 以**显式请求容量**为核心，而不是先取一个零长、容量由历史流量决定的 buffer。
2. 使用**确定性的容量等级**和明确的 `MaxPooledCapacity`；超大 buffer 可以分配，但归还时直接丢弃，避免一次峰值污染常态内存。
3. 默认把「未清零、速度优先」和「完整清零、安全优先」视为两种不同语义，分别 Benchmark，不能拿 dirty pool 与 zeroing pool 直接比。
4. 如果承诺硬内存上限、精确统计或主动清空，就不能只把 `sync.Pool` 包一层；`sync.Pool` 的条目会被运行时无通知移除，也不提供可枚举、可计数的保留集合。
5. 普通 Go API 无法阻止调用者保存一份 `[]byte` 并在归还后继续读写。全新 API 能做的是避免外来 buffer、跨池归还和双重归还破坏池结构，并提供严格/调试模式；不能宣称彻底消除 use-after-release。
6. Benchmark 必须拆成 raw `[]byte`、append/writer、安全清零、稳态吞吐和峰值后内存五类。`valyala` README 引用的旧结果只能作为历史资料，不能作为新库的性能证据。

## `valyala/bytebufferpool` 到底做了什么

### API 与 buffer 行为

公开 API 很小：包级及实例级 `Get`/`Put`，以及只含一个公开字段 `B []byte` 的 `ByteBuffer`。`ByteBuffer` 实现 `io.Writer`、`io.ReaderFrom`、`io.WriterTo` 和若干接近 `bytes.Buffer` 的方法，但没有实现 `io.Reader`。[公开文档](https://pkg.go.dev/github.com/valyala/bytebufferpool)；[ByteBuffer 源码](https://github.com/valyala/bytebufferpool/blob/master/bytebuffer.go)

`Get` 返回长度为 0 的 buffer；池 miss 时，容量取自动态 `defaultSize`。`Put` 先按 `len(b.B)` 记一次直方图，再读取动态 `maxSize`；当 `cap(b.B)` 不超过 `maxSize` 时才 `Reset` 并放回同一个 `sync.Pool`。首次校准之前 `maxSize == 0`，所以所有容量都会被接纳。[pool.go](https://github.com/valyala/bytebufferpool/blob/master/pool.go#L34-L72)

`Reset` 只把长度改成 0，不清除底层数组。因此旧内容仍在 capacity 范围内；拿到切片后手动扩展长度可以观察旧字节。这不是该库特有的问题，但全新 API 必须明确 dirty/zeroed 语义。[bytebuffer.go](https://github.com/valyala/bytebufferpool/blob/master/bytebuffer.go#L102-L105)

### 自校准策略

实现维护 20 个 2 的幂级别，起点 64B，最大名义级别 32MiB；更大的长度被压到最后一级。某一 bin 的累计 `Put` 次数超过 42,000 后触发校准。校准把最常见 bin 的大小设为 `defaultSize`，再按调用次数从高到低累计，覆盖约 95% 调用的 bin 中最大尺寸作为 `maxSize`。[pool.go 常量和状态](https://github.com/valyala/bytebufferpool/blob/master/pool.go#L8-L31)；[calibrate](https://github.com/valyala/bytebufferpool/blob/master/pool.go#L75-L110)

这不是「按请求大小找最接近的池」：所有被接纳的对象进入同一个 `sync.Pool`，`Get` 也没有 size 参数。动态 `defaultSize` 只影响 miss 后的新分配，动态 `maxSize` 只影响后续 `Put` 是否接纳。结果是性能和内存都依赖历史流量、GC 时机与热身顺序。

值得保留的思想是「超出常态的 buffer 不要长期保留」；不值得照搬的是隐藏在 42,000 次热身后的、不可配置且不可观测的自校准。仓库中关于 `defaultSize`/`maxSize` 偏小的报告从 2017 年一直处于 open；另有 2021 年 PR 尝试调整该逻辑，仍未合并。[issue #15](https://github.com/valyala/bytebufferpool/issues/15)；[PR #23](https://github.com/valyala/bytebufferpool/pull/23)

### 并发与所有权风险

池容器依靠 `sync.Pool` 和原子计数，可被多个 goroutine 调用；这不代表取出的 `ByteBuffer` 自身可以并发使用。源码只在注释中要求 `Put` 后不能再访问，否则会发生 data race。[Put 注释](https://github.com/valyala/bytebufferpool/blob/master/pool.go#L53-L62)

当前 API 还允许以下情况：

- 同一个指针被 `Put` 两次；两个后续 `Get` 可能取得同一对象，造成数据竞争或破坏数据。
- 把手工构造的 `ByteBuffer` 或从另一个 `Pool` 取得的对象归还；实现不验证 provenance。
- 保存 `B` 的别名，在 `Put` 后继续访问。公开字段让误用非常容易。
- `nil` 传给 `Put` 会解引用 panic；这不是有意设计的错误类型。

公开 issue 中确有 race 报告，但该报告只给出了调用方同时读写同一底层数组的 race trace，不能据此断言池内部有 race；它至少说明当前所有权契约不易被正确使用。[issue #18](https://github.com/valyala/bytebufferpool/issues/18)

### 内存保留与峰值污染

自校准后的 `maxSize` 能让超大 buffer 在下一次 `Put` 时被丢弃，但有几个边界：

- 校准前所有容量都可进入池。
- 已经在 `sync.Pool` 中的超大对象不会在校准时被主动清理，只能等它再次被取出、归还，或被 GC 周期清除。
- 如果大对象占到统计流量约 5% 以上，它会进入保留范围；单一 `sync.Pool` 仍可能把大容量对象交给很小的下一次工作。
- 没有 retained bytes、drop、hit/miss、当前阈值等统计，也没有硬内存预算。用户要求 hard limit 的 issue 至今没有实现。[issue #20](https://github.com/valyala/bytebufferpool/issues/20)

`ReadFrom` 每次容量不足时自行分配两倍数组并复制旧内容，旧 backing array 随后等待 GC；相关问题也是 open。[ReadFrom 源码](https://github.com/valyala/bytebufferpool/blob/master/bytebuffer.go#L21-L53)；[issue #25](https://github.com/valyala/bytebufferpool/issues/25)

### 维护状态

`v1.0.0` 发布于 2016-08-17。[pkg.go.dev 版本信息](https://pkg.go.dev/github.com/valyala/bytebufferpool?tab=versions)

默认分支最近一次 commit 是 2020-11-04，内容只是给 Travis 增加 ppc64le；`pool.go` 最近一次提交是 2016-07-01 的 typo 修复，核心策略实际停留在 2016 年。[默认分支 commits API](https://api.github.com/repos/valyala/bytebufferpool/commits?per_page=5)；[`pool.go` commits API](https://api.github.com/repos/valyala/bytebufferpool/commits?path=pool.go&per_page=10)

截至调研日，仓库页面显示 10 个 open issue、3 个 open PR；其中 open PR 分别始于 2017、2021 和 2024。[issues](https://github.com/valyala/bytebufferpool/issues)；[pull requests](https://github.com/valyala/bytebufferpool/pulls)

`fasthttp` 目前仍直接使用独立的 `bytebufferpool.Pool` 保存 request/response body；它是重要使用方，不是新的替代实现。[fasthttp/http.go](https://github.com/valyala/fasthttp/blob/master/http.go)

## 代表性实现对照

| 实现 | API / 容量策略 | 并发与保留 | 超大对象 | 可观测性 | 维护判断 |
| --- | --- | --- | --- | --- | --- |
| Go `sync.Pool` + `bytes.Buffer` | `Get/Put(any)`；buffer 自己增长，`Reset` 保留底层存储 | 多 goroutine 可安全操作池；条目可在任何时刻被 runtime 无通知移除 | 除非调用方加 cap cutoff，否则峰值 buffer 可回池 | 无法枚举或得到精确保留量 | Go 标准库，持续维护 |
| `libp2p/go-buffer-pool` | `Get(length) []byte` / `Put([]byte)`；32 个 2 的幂级别；返回 len=request、cap=next power of 2 | 每级一个 `sync.Pool`；另用一个 wrapper pool 避免 slice header 的 interface 分配 | 只丢弃 0 cap 和大于 `MaxInt32`；一般超大 buffer 仍可能保留 | 无 | [2025-08 有 Go 1.24 维护提交](https://github.com/libp2p/go-buffer-pool/commit/86b7b85b26ae7027614b2cc7e4cba3066de87e6e)；最新 tag `v0.1.0` 为 2022 |
| gRPC-Go `mem.BufferPool` | 公共 `Get(length) *[]byte` / `Put(*[]byte)`；可配置 tier；默认 256B、4K、16K、32K、1M；binary tier O(1) 查找 | 每 tier 使用 `sync.Pool`；公共默认实现取出时清零完整 capacity | 大于最大 tier 进入共享 fallback `SimpleBufferPool`，不是直接丢弃 | 无 | [`mem/buffer_pool.go` 在 2026-08 仍有行为更新](https://github.com/grpc/grpc-go/commit/7354d9c8debb4bcf2225bf429857078de310c176) |
| Prometheus `util/pool` | `New(min,max,factor,makeFunc)`；按几何级数建多个 `sync.Pool`；`Get(size)` 选首个可容纳 bucket | 多池并发；反射支持任意 slice 类型 | 大于最大 bucket 的 Get 现分配、Put 丢弃 | 无 | Prometheus 主仓库持续维护；[该文件 2026 仍有提交](https://github.com/prometheus/prometheus/commit/e14795bbf4fbd1837a9e3428aafb40b4b1dda99d) |
| `oxtoacart/bpool` | 有 `BufferPool`、固定宽度 `BytePool`、`SizedBufferPool`；底层是有界 channel | retained count 有明确上限，满时丢弃；channel 有竞争成本 | 普通 `BufferPool` 会保留增长后的大 buffer；`SizedBufferPool` 归还时把过大对象替换成初始容量的新 buffer | `NumPooled()`，没有 bytes/hit/drop | [最近 commit 2019-05](https://github.com/oxtoacart/bpool/commit/03653db5a59cd88b481403d312d7c324b56af377)，基本停更 |
| `valyala/bytebufferpool` | 零长 `ByteBuffer`；单池；42k 调用后按历史自校准默认容量与约 p95 上限 | `sync.Pool` + 原子直方图；行为依赖热身和 GC | 校准后 cap 超阈值丢弃；校准前全收 | 无 | 核心实现停在 2016，默认分支最近 commit 2020 |

### 标准库基线

`sync.Pool` 的正式契约明确说明：对象可在任何时候被自动移除，`Get` 甚至可以忽略已有条目；它适合减轻 GC 压力，不适合要求稳定库存或硬上限的 free list。[sync.Pool 文档](https://pkg.go.dev/sync#Pool)

标准库示例本身就是 `sync.Pool` + `*bytes.Buffer`。`bytes.Buffer.Reset` 会清空逻辑长度但保留底层存储，这正是简单 pool 会发生峰值污染的原因。[sync.Pool 示例](https://pkg.go.dev/sync#example-Pool)；[bytes.Buffer.Reset](https://pkg.go.dev/bytes#Buffer.Reset)

Go 官方 issue #23199 给出了这个问题的直接复现：10 个 256MiB 初始大请求后持续处理 1KiB 小请求，因大 buffer 被不断取出又放回，示例约需 35 次 GC 才释放最初约 2.5GiB。issue 的结论是 `sync.Pool` 假设各元素内存成本大致相同，不能把任意容量的对象当成等价条目。[golang/go#23199](https://github.com/golang/go/issues/23199)

Go 官方 slog handler 指南给出了一个更好的最小模式：pool 中保存 `*[]byte`，并在归还时拒绝超过 16KiB 的 buffer；文档明确用一次 1MiB 日志解释了为什么 cap cutoff 重要。[官方示例](https://github.com/golang/example/blob/master/slog-handler-guide/README.md#using-a-pool)

### `libp2p/go-buffer-pool`

这是最接近「公开、按 size 取 raw `[]byte`」的对照。其 README 明确列出代价：capacity 向上取 2 的幂、不清零可能泄露旧内容、显式归还可能造成 race 和内存破坏。[README](https://github.com/libp2p/go-buffer-pool)

源码用请求 length 的向上取整选择 Get bucket，用 buffer capacity 的向下取整选择 Put bucket；通过额外 `bufp` wrapper pool 避免直接把 slice header 放进 `any` 的分配成本。[pool.go](https://github.com/libp2p/go-buffer-pool/blob/v0.1.0/pool.go#L51-L117)

设计启示：按 size class 隔离比单一 `sync.Pool` 更可预测，但 2 的幂最多会带来接近 100% 的内部碎片；必须另加最大可保留容量，不能把 `MaxInt32` 当实用内存策略。

### gRPC-Go `mem.BufferPool`

公共接口返回 `*[]byte`，并要求 `Put` 时传入最初 buffer 的 prefix，以便重新使用完整 capacity。公共 API 提供普通 tier、binary tier 和 `NopBufferPool`，默认等级来自具体协议/运行时尺寸，而不是纯粹的全 2 次幂。[公共 API 源码](https://github.com/grpc/grpc-go/blob/master/mem/buffer_pool.go)

内部 binary tier 对 Get 使用「下一个可容纳等级」，对 Put 使用「不大于 capacity 的最大等级」，并明确用 `cap` 而不是 `len`，避免对象逐渐漂移到最小池。普通模式在取出时 `clear(b[:cap(b)])`。[内部实现](https://github.com/grpc/grpc-go/blob/master/internal/mem/buffer_pool.go#L1169-L1353)

设计启示：

- 等级可按真实协议工作集配置，不必强制每一个 2 的幂。
- Get/Put 路由应分别定义，且归还依据 capacity。
- 清零必须作为可比较的语义；gRPC 公共默认池为清零模式。
- gRPC 对超过最大 tier 的对象使用 fallback `sync.Pool`。对于通用库，这仍可能保留任意大的峰值 buffer；新库若以「抗峰值污染」为卖点，应选择 unpooled/drop，而不是照搬 fallback。

### Prometheus `util/pool`

Prometheus 主仓库的 `util/pool` 是一个几何分级的通用 slice pool：构造器接收最小、最大、factor 和 factory；超出最大值时直接 factory 分配且归还时丢弃。[pool.go](https://github.com/prometheus/prometheus/blob/main/util/pool/pool.go)

Prometheus scrape 路径用 `pool.New(1e3, 1e6, 3, ...)`，说明生产级 size classes 可以是贴合业务的 1K→3K→9K，而不必固定 2 倍增长。[scrape.go](https://github.com/prometheus/prometheus/blob/main/scrape/scrape.go)

需要注意：`Put` 把 slice 放入第一个 `size >= cap(slice)` 的 bucket，而 `Get` 直接返回 bucket 中对象、不再检查 capacity。由源码可推断，正确性依赖调用者只归还由该 pool 产生并保持容量等级不变的 slice；通用新 API 应在边界上验证这一不变量，或通过不暴露内部存储来保证它。

构造器还只检查 `factor >= 1`，随后用 `s = int(float64(s) * factor)` 生成下一档。`factor == 1`，或乘积经 `int` 截断后仍等于 `s`，会让循环不前进。全新 API 若支持 factor，应验证“下一档严格增大”；更简单的做法是只接受经过校验的显式 class 列表。[New 源码](https://github.com/prometheus/prometheus/blob/main/util/pool/pool.go#L29-L53)

### `oxtoacart/bpool`

`bpool` 用 bounded channel 实现确定的最大对象数，池满即丢弃，并提供 `NumPooled`。这比 `sync.Pool` 更容易解释和观察，但 channel 容量限制的是对象数，不是总字节数。[BufferPool 源码](https://github.com/oxtoacart/bpool/blob/master/bufferpool.go)

`SizedBufferPool` 在归还时发现 buffer capacity 超过预设值，会现场新建一个预设容量 buffer 放回池。这能消除峰值污染，却把一次分配成本放到了 `Put` 路径；更简单的通用策略是直接丢弃，让下一次 miss 再分配。[SizedBufferPool 源码](https://github.com/oxtoacart/bpool/blob/master/sizedbufferpool.go)

### `segmentio/encoding` 与 `prometheus/client_golang`：内部用法，不是替代库

`segmentio/encoding` 没有公开通用 buffer pool。JSON 编码器内部用单个 `sync.Pool` 保存初始 4KiB 的 `encoderBuffer`，复用时 `buf.data[:0]`，没有容量上限；这是一种局部、受控的优化，不应作为公共 API 对照。[segmentio/encoding/json/json.go](https://github.com/segmentio/encoding/blob/master/json/json.go#L274-L282)；[pool 定义](https://github.com/segmentio/encoding/blob/master/json/json.go#L590-L592)

`prometheus/client_golang` 同样没有公开通用 byte pool。实验性 remote API 每个实例持有一个初始 16KiB 的 `sync.Pool`，编码/压缩 buffer 可能增长后原样放回，也没有 cap cutoff。[remote_api.go](https://github.com/prometheus/client_golang/blob/main/exp/api/remote/remote_api.go#L146-L164)；[Get/Put 调用](https://github.com/prometheus/client_golang/blob/main/exp/api/remote/remote_api.go#L216-L264)

该项目 2025 年的 PR #1889 修复过一次更关键的生命周期错误：压缩后的 payload 仍被 HTTP request 使用时，buffer 已提前归池，可能造成 remote-write payload 损坏。这说明 `Get/Put` 的正确归还时点必须与最后一个消费者结束绑定；新 API 的 scoped lease 能改善表达，但仍需要 race/生命周期测试。[client_golang#1889](https://github.com/prometheus/client_golang/pull/1889)

两者值得参考的是「让 pool 靠近明确的调用场景」；它们不应进入通用库的公开 API 兼容矩阵。Prometheus 真正可复用的分级思路在主仓库 `util/pool`，不是 `client_golang`。

## 面向全新 API 的设计启示

### 1. 先明确所有权，而不是先追求 `Get` 的几个纳秒

建议让外部拿到的是带 pool provenance 和 generation 的 lease/value handle，底层 storage slot 才进入池。这样可以在不接受外来对象的前提下检测跨池归还、同一 generation 的双重归还，以及 stale handle 的再次调用。

一个研究阶段的 API 轮廓如下，名称仍需在正式设计阶段决定：

```go
lease := pool.Acquire(size)
defer lease.Release()

b := lease.Bytes() // len == size；归还后不再有效
```

关键约束：

- `Release` 的第二次调用必须是可检测的 no-op/error，严格模式可 panic，不能把同一 storage 入池两次。
- handle 必须带 generation；若直接复用 handle 指针，旧调用者可能在该指针被再次租出后“复活”。
- `Bytes()` 返回的 `[]byte` 别名仍无法被 Go 类型系统撤销。文档必须诚实说明：保存别名并在 Release 后使用仍属于未定义的所有权误用；`-race` 和 debug poison/ownership map 可帮助发现，但不能提供内存安全保证。
- 快速模式可以另外提供 raw `[]byte` API，但应把它命名为 unsafe/low-level 层，而不是让最危险的路径成为唯一入口。

### 2. 容量策略应确定、可配置、可解释

建议默认使用明确 size classes，并允许调用方覆盖：

- 通用默认可从 64B 起按 2 倍增长到 1MiB；协议型应用可传入 `[256, 4096, 16384, 32768, 1<<20]` 一类真实工作集。
- `Acquire(n)` 选择第一个 `class >= n`；`Release` 基于 capacity 路由。
- 对非标准 capacity，要么只在浪费比例可接受时放入较小等级，要么直接丢弃；不能把可能小于下一次请求的对象放入较大 bucket。
- `MaxPooledCapacity` 以上的请求允许正常分配，但 lease 标记为 unpooled，Release 直接丢弃。
- 若提供 hard `MaxRetainedCapacity`，实现必须自己管理有界 freelist；仅靠 `sync.Pool` 不能兑现这个契约。

不建议默认自适应。若未来增加 adaptive mode，应是显式 option，并暴露当前等级、样本窗口、阈值和重置方法；它还必须有 deterministic tests 和单独 Benchmark，不能改变默认性能的可复现性。

### 3. 明确三种不同的“上限”

API 和文档应区分：

1. `MaxPooledCapacity`：单个 buffer 超过它就不保留。
2. `MaxRetainedCapacity`：池中所有空闲 Backing Storage 的硬预算。
3. `MaxAcquireSize`：单次请求的业务保护；超过时返回 error，而不是尝试分配。

`valyala` 只有动态的第一类近似阈值，`bpool` 只有按对象数的第二类近似。把三者混成一个 `max` 会导致用户误解 OOM 行为。

### 4. 清零语义必须进入 API 和 Benchmark 名称

至少支持并清晰区分：

- `Dirty`：不清零，最快；可能泄露上一位租户的内容。
- `ZeroOnAcquire` 或 `ZeroOnRelease`：清整个 capacity，而不只是当前 len；二者的延迟位置不同。

只清 len 不足以阻止调用者通过 reslice 观察 capacity 内旧数据。清零成本与 buffer 大小线性相关，因此 README 结果必须明确模式，不能把 dirty 新库与 gRPC 默认 zeroing pool 的 ns/op 排在同一列后宣称胜出。

### 5. 可观测性与后端选择绑在一起

一个有界自管理后端可提供精确 `Stats()`：

- acquire、hit、miss、release、drop；
- 按 size class 的 hit/miss/drop；
- retained buffers/bytes、上限和 high-water mark；
- oversize allocations、zeroed bytes；
- debug 模式检测到的 double release/cross-pool release。

如果后端是 `sync.Pool`，因为 runtime 会静默移除条目，`retained` 只能是估算值；API 不应伪装成精确 gauge。每次操作上的 atomic metrics 也会改变 hot path，需要允许关闭，并分别 Benchmark metrics-on/off。

### 6. 不把 `bytes.Buffer` 兼容当成核心目标

`bytes.Buffer` 已经有成熟语义，`Reset` 明确保留底层存储，`Grow` 明确保证后续空间。[bytes.Buffer 文档](https://pkg.go.dev/bytes#Buffer)

新库可以后续提供 writer adapter，但核心应先把 raw storage 的容量、所有权、归还和上限做好。否则会像 `valyala` 一样把 buffer 方法、pool 策略和不安全的公开切片字段绑成一个浅接口，后续难以演进。

## 公平 Benchmark 方案

### 为什么不能复用旧 README 数字

`valyala` README 链接的结果为：`ByteBufferPool` 39ns/op、普通 `sync.Pool` 43.5ns/op、`bpool` 177ns/op；页面没有记录 Go 版本、依赖 commit、CPU 型号、样本次数或误差，只能看出是 `-8` 并行度。[历史结果页面](https://omgnull.github.io/go-benchmark/buffer/)

仓库自己还带有另一组 `bytebuffer_timing_test.go`：每个并行 worker 都只使用自己的局部 buffer，循环写入 100 次 9 字节内容后 Reset，只比较 `ByteBuffer.Write` 与 `bytes.Buffer.Write`，根本没有执行 pool 的 Get/Put。因此它只能比较两个 append wrapper，不能证明 pool 策略的吞吐或内存表现。[内置 benchmark 源码](https://github.com/valyala/bytebufferpool/blob/master/bytebuffer_timing_test.go)

对应源码把同一组固定字符串反复写入 buffer，并在 `testing.B.RunParallel` 中取出、写入、归还。它基本能比较「热池、单一约 1KiB append workload」的平均吞吐，但没有覆盖 size-aware API、混合尺寸、GC 后 miss、容量等级边界、超大峰值、清零、hard bound 或错误使用。[main.go](https://github.com/omgnull/go-benchmark/blob/master/buffer/main/main.go)；[main_test.go](https://github.com/omgnull/go-benchmark/blob/master/buffer/main/main_test.go)

因此新项目不应在 README 中复述这组数字，更不能把它写成当前 Go 上的结论。

### 对照集合：按语义分组

#### A. raw `[]byte` / exact length

所有实现都必须返回 `len == requested` 且 `cap >= requested`，并触碰相同的首尾字节防止编译器消除：

1. `make([]byte, n)`：无池基线。
2. 单个 `sync.Pool` + `*[]byte`：朴素基线。
3. `sync.Pool` + 明确 cap cutoff：官方 slog 指南模式。
4. 本项目新实现：dirty、zeroing、metrics on/off 分开命名。
5. `libp2p/go-buffer-pool`。
6. gRPC `mem.NewBinaryTieredBufferPool`：只放在 zeroing 语义组；公共实现会清零。
7. Prometheus `util/pool`：使用与本项目相同的 size classes；注明其 API 返回 `any`/反射。
8. `oxtoacart/bpool.BytePool` 只参加固定宽度场景，不能伪装成 variable-size 对照。

#### B. append / writer

所有实现写入相同 payload、验证相同 checksum：

1. 新建 `bytes.Buffer`。
2. `sync.Pool` + `*bytes.Buffer`，带/不带 cap cutoff 分开。
3. `valyala/bytebufferpool`，作为迁移参考而不是 API 目标。
4. `oxtoacart/bpool.BufferPool` 与 `SizedBufferPool`。
5. 本项目未来的 writer/append adapter。

不要把 raw slice 的 `Get(n)` 和从零开始 append 的 `bytes.Buffer` 放在同一个排名中；两边完成的工作不同。

#### C. 安全和生命周期成本

单列以下结果：

- dirty vs clear-full-capacity；
- raw API vs lease/generation checks；
- metrics off vs on；
- strict misuse detection off vs on。

这组结果回答安全措施成本，不能被隐藏在总榜中。

### 工作负载矩阵

1. **固定尺寸**：64B、1KiB、4KiB、16KiB、64KiB、1MiB。
2. **等级边界**：每个 class 的 `n-1`、`n`、`n+1`，直接暴露 rounding 和内部碎片。
3. **确定性混合分布**：例如 70% 512B、20% 4KiB、9% 64KiB、1% 1MiB；使用预生成 trace，计时区内不调用随机数。
4. **真实 trace**：若项目将用于 HTTP、日志或编码，从真实生产尺寸直方图生成匿名 trace，并把 trace 和生成规则提交到仓库。
5. **并发**：serial 与 `RunParallel` 分开；至少记录 `GOMAXPROCS=1` 和机器默认值。
6. **GC 敏感性**：稳态热池前后显式执行 GC 的场景分开；GC 放在计时区外，并标注目的，因为 `sync.Pool` 允许条目被回收。
7. **峰值后恢复**：先稳定运行小 buffer，再并发申请 8MiB/64MiB 峰值并归还，最后恢复小 buffer；记录峰值前、峰值后、1 次 GC、2 次 GC 后的进程内存和 pool 自有统计。
8. **过载/预算**：并发持有超过 retained budget 的 buffer，验证 drop 和上限，不只看 ns/op。

### 指标与运行规范

吞吐基准至少输出 `ns/op`、`B/op`、`allocs/op` 和处理字节数；使用 `b.ReportAllocs()`、`b.SetBytes()`，并保证每个实现执行相同 touch/checksum。

建议固定命令形态：

```text
go test -run '^$' -bench . -benchmem -benchtime=3s -count 10 -cpu 1,8
benchstat old.txt new.txt
```

发布结果时必须同时记录：

- Go 完整版本、GOOS/GOARCH、CPU 型号、逻辑核数；
- `GOMAXPROCS`、`GOGC`、是否设置 memory limit；
- 本项目 commit 和所有第三方依赖的精确版本；
- size classes、上限、zeroing、metrics、strict/debug 配置；
- 每组至少 10 次样本及 `benchstat` 统计，不只贴最好的一次。

内存峰值测试应让每个 contender 在独立进程运行，避免前一个 pool 和 GC 状态污染后一个。除 `runtime.ReadMemStats` 外，还应保存 heap profile；对于有界后端，同时报告精确 retained bytes。`HeapSys` 不等于 pool 实际持有内存，README 不应只挑一个 runtime 指标证明“无浪费”。

### 正确性与滥用不是 Benchmark

以下应写成测试并在 CI 中跑 `go test -race ./...`：

- `Acquire` 的长度/capacity 保证，所有 class 边界与负数/过大请求。
- Release 后再次 Release 不会把 storage 入池两次。
- 跨 pool Release、外来对象、nil/zero buffer 的明确行为。
- oversize Release 必须 drop，随后普通 Acquire 不得取得 oversize capacity。
- zeroing 模式清完整 capacity。
- stale lease generation 被拒绝。
- 并发 Acquire/Release 不共享同一已租出 storage。

把这些误用路径放入吞吐基准会掩盖真正风险；把它们只写进文档又会重演旧库的问题。

## 建议的第一版范围

在尚未有真实工作集数据前，第一版应保持小而可验证：

- 一个显式配置、固定 size classes 的 pool；
- `MaxPooledCapacity`，超大对象归还即 drop；
- dirty 与 zeroing 两种清晰模式；
- 防双重归还和跨池归还的 lease/provenance 机制，严格模式可配置；
- 如果选择有界后端，再提供 `MaxRetainedCapacity` 和精确 stats；如果选择 `sync.Pool` 后端，就不承诺硬预算；
- raw `[]byte` 和 writer benchmark 分组；
- 暂不做自校准、finalizer 自动归还、复杂 writer 兼容层或背景清理 goroutine。

这个范围直接解决 `valyala` 最难解释的历史依赖、超大 buffer 保留和所有权误用，同时仍允许以后依据 Benchmark/生产 trace 增加更深的策略。
