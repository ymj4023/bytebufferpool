# `bytebufferpool` 全新实现设计

- 日期：2026-08-29
- module：`github.com/ymj4023/bytebufferpool`
- 状态：已完成对话评审，等待书面规格确认
- 最低 Go 版本：1.22
- 许可证：MIT

## 1. 背景与目标

本项目不是 `github.com/valyala/bytebufferpool` 的 fork，也不追求其 API 兼容性。它从公开资料中吸收容量分级、有界缓存等经过验证的设计思想，使用全新 API 和独立代码解决以下问题：

1. 大容量 buffer 在流量尖峰后长期滞留。
2. 运行时自适应校准不透明，结果依赖历史流量和热身顺序。
3. 无法限制空闲池保留的 buffer capacity 总量。
4. 重复归还、跨池归还等误用难以及时发现。
5. 缺少按长度申请 `[]byte` 的 API。
6. `ReadFrom`、短写和异常 Reader/Writer 等边界语义不完整。
7. Benchmark 陈旧、场景单一且结果不可复现。

市场调研、源码分析与一手资料链接见 [`docs/research/byte-buffer-pools.md`](../../research/byte-buffer-pools.md)。

## 2. 非目标

第一版不包含：

- 与 `valyala/bytebufferpool` 兼容的类型、包级全局池或迁移适配层。
- 根据运行时流量自动改变容量等级的校准器。
- 后台清理 goroutine、finalizer 自动归还或定时收缩。
- 每容量等级的动态配额或自动再平衡。
- 阻止所有 `[]byte` use-after-release 的内存安全承诺；Go 的可变切片别名无法在归还后撤销。
- 将第三方实现源码复制、改名后纳入本仓库。

## 3. 成功标准

实现完成必须同时满足：

- 根模块没有第三方运行时依赖。
- 默认容量等级确定且可解释，自定义等级在构造时完整校验。
- 超过最大池化容量的对象不会进入任何后端。
- Bounded 模式中，空闲 buffer 的 `sum(cap(buffer))` 不超过 `MaxRetainedBytes`；该字段不冒充进程 RSS 或 Go heap 的硬上限。
- Fast 与 Bounded 使用同一套公共 API，并分别测试、分别 Benchmark。
- 验证模式能拒绝可观测到的连续双重归还、跨池归还、外来切片和非法容量。
- `Buffer` 的释放、复制、扩容和 I/O 边界行为有确定测试。
- `go test ./...`、`go vet ./...` 和 `go test -race ./...` 均执行；环境无法运行的检查必须明确记录，不能称为已通过。
- Benchmark 保存源码、固定依赖、原始输出、统计汇总、机器信息和复现命令。
- README 不使用无统计支持的“全市场最快”等笼统结论。
- 所有受具体第三方实现启发的非显然算法均有相邻的 `Design reference:` 注释，并说明本实现差异。

## 4. 公共 API

以下是第一版的目标 API 形状；实现计划可调整未导出名称，不能改变本节公开语义。

### 4.1 配置与构造

```go
type Mode uint8

const (
	Fast Mode = iota
	Bounded
)

type Config struct {
	Mode                 Mode
	Classes              []int
	MaxPooledCapacity    int
	MaxRetainedBytes     int64
	MaxAcquireSize       int
	ZeroOnRelease        bool
	ValidationEnabled    bool
	StatsEnabled         bool
}

func DefaultConfig(mode Mode) Config
func PowerOfTwo(minimum, maximum int) ([]int, error)
func New(config Config) (*Pool, error)
```

默认容量等级为 64 B 起、按 2 的幂增长至 1 MiB。`Classes` 为空时使用默认等级；非空时必须满足正数、严格递增、无重复，且每一项不超过 `MaxPooledCapacity`。

`DefaultConfig(Fast)` 使用 1 MiB 的 `MaxPooledCapacity`、无限制的 `MaxAcquireSize`、关闭清零/验证/统计。`DefaultConfig(Bounded)` 在此基础上设置 32 MiB 的 `MaxRetainedBytes`。这些默认值属于第一版稳定 API；README 同时展示如何按工作集显式覆盖。

`PowerOfTwo` 要求 minimum 和 maximum 均为正的 2 的幂且 minimum 不大于 maximum，否则返回错误。返回列表包含两个端点。

`MaxPooledCapacity` 为 0 时使用 1 MiB 默认值；负数无效。它控制可进入池的单个对象上限；不存在可容纳请求的容量等级时，该次申请不池化。自定义等级的最后一项可以小于 `MaxPooledCapacity`，两者之间的请求明确视为 unpooled。`MaxAcquireSize` 为 0 表示不额外限制单次申请，负值无效；非零时不得小于 `MaxPooledCapacity`，负数请求或超过该值的申请失败。

Bounded 模式要求 `MaxRetainedBytes > 0`。Fast 模式若收到非零 `MaxRetainedBytes`，`New` 返回配置错误，因为 `sync.Pool` 无法兑现精确保留预算。

### 4.2 定长切片

```go
func (p *Pool) Acquire(size int) []byte
func (p *Pool) TryAcquire(size int) ([]byte, error)
func (p *Pool) Release(buffer []byte) ReleaseStatus
```

`TryAcquire` 成功时保证 `len(buffer) == size` 且 `cap(buffer) >= size`。`Acquire` 是便利热路径：内部使用相同语义，但对 `TryAcquire` 会返回的参数错误执行 panic。

`Acquire(0)` 返回 `nil`；`Release(nil)` 返回 `IgnoredNil`。返回切片的旧内容未定义。调用者可以读写或普通 reslice，但归还时必须仍持有原 backing array 和完整 capacity。若 append 触发重新分配，调用者不能把新 backing array 冒充原借用对象归还：验证模式返回 `RejectedForeign`，非验证模式根据容量不变量丢弃或接纳。后一行为是 raw 快路径的已知弱点，安全敏感调用应使用 `Buffer`。

`ReleaseStatus` 至少区分：

- `Retained`
- `DroppedFull`
- `DroppedOversize`
- `DroppedInvalid`
- `RejectedForeign`
- `RejectedDuplicate`
- `IgnoredNil`

只有验证模式承诺区分 foreign 和 duplicate；关闭验证时，无法证明所有权的对象按容量不变量处理。

### 4.3 追加写入 Buffer

```go
func (p *Pool) Buffer(initialCapacity int) Buffer
func (p *Pool) TryBuffer(initialCapacity int) (Buffer, error)

func (b *Buffer) Bytes() []byte
func (b *Buffer) Len() int
func (b *Buffer) Cap() int
func (b *Buffer) Grow(n int) error
func (b *Buffer) Reset()
func (b *Buffer) Write(p []byte) (int, error)
func (b *Buffer) WriteByte(c byte) error
func (b *Buffer) WriteString(s string) (int, error)
func (b *Buffer) ReadFrom(r io.Reader) (int64, error)
func (b *Buffer) WriteTo(w io.Writer) (int64, error)
func (b *Buffer) Release() ReleaseStatus
```

`Buffer` 由值返回，避免规定每次构造都必须产生堆分配。它在第一次使用后不得复制；实现使用与 `strings.Builder` 同类的 self-pointer copy check，但代码独立编写，并在相邻注释标出标准库设计参考。

`Release` 清除 `Buffer` 持有的 pool 和 slice 引用。连续第二次释放返回 `RejectedDuplicate`，不会再次把 storage 放入池。释放后调用除 `Release` 外的方法会 panic，尽早暴露生命周期错误。

`Bytes` 返回的切片只在下一次修改或 `Release` 前有效。调用者保存该别名并在失效后访问属于所有权误用，验证模式和 race detector 只能提高发现概率，不能提供完全内存安全。

## 5. 容量与增长语义

### 5.1 路由

- 申请时选择第一个大于等于请求长度的容量等级。
- 默认 2 的幂等级使用 `math/bits` 计算索引；自定义等级使用 `sort.Search`。
- 归还时按 `cap(buffer)` 路由，只接纳与某一等级精确相等的容量。
- 超过 `MaxPooledCapacity`、超过最大等级或容量不精确匹配的对象直接丢弃。
- 路由只依赖当前配置和请求，不依赖历史流量、GC 次数或热身顺序。

### 5.2 Buffer 增长

`Buffer` 容量不足时从同一 `Pool` 申请下一个可容纳新长度的 slice，复制现有内容，再立即归还旧 slice。超过 `MaxPooledCapacity` 但未超过 `MaxAcquireSize` 时使用普通分配；释放时不入池。

若增长会超过 `MaxAcquireSize`，操作返回明确错误，并保持原有内容不变。`Write`、`WriteString`、`WriteByte` 和 `ReadFrom` 必须在修改前完成尺寸溢出检查。

## 6. 后端设计

### 6.1 统一 Pool

公共 API 只有一个 `Pool` 类型。`Mode` 在构造时确定且不可修改；容量表、限制、清零、验证与统计配置也全部不可变。

热路径根据固定 `Mode` 进入 Fast 或 Bounded 后端。首先保留简单分支实现，只有 Benchmark 证明分支成本显著时才改为内部函数指针；不能在没有数据前增加泛型公共类型或重复的 `FastPool`/`BoundedPool` API。

### 6.2 Fast 后端

- 每个容量等级拥有独立 `sync.Pool`。
- 不同容量不在同一个池中混放。
- 使用独立 wrapper recycler 避免把 slice header 直接装入 `any` 时产生不必要分配。
- runtime 可以无通知丢弃 `sync.Pool` 条目，因此 Fast 不报告精确 `RetainedBytes`，也不接受 `MaxRetainedBytes` 承诺。
- `Clear` 通过原子切换到新的内部 pool 世代，清除调用时已空闲且仍可达的缓存。raw `[]byte` 不携带借出世代，因此清理前已借出、清理后才归还的切片可以进入新世代；`Clear` 不承诺撤销在借对象。

“每容量等级一个 `sync.Pool`”和 wrapper recycler 的概念受 `libp2p/go-buffer-pool` 启发。实现必须独立编写，并在相邻注释中写明来源 URL、cap cutoff、配置、验证及 Clear 世代方面的差异。

### 6.3 Bounded 后端

- 每个容量等级维护互斥保护的 LIFO 空闲列表。
- 全局原子预算记录空闲 buffer 的 capacity 总和。
- 归还时先以 CAS 竞争容量预算；预算不足立即返回 `DroppedFull`，不阻塞。
- 成功取得缓存对象时，从对应等级弹出对象并扣减空闲预算。
- `Clear` 逐级清空列表并释放列表 backing storage。在没有并发归还时，调用返回后的保留计数为零；并发归还可以在清理过程中或之后重新填充缓存。它不撤销在借对象。

`MaxRetainedBytes` 的精确定义是“当前池内空闲 byte slice 的 `sum(cap)` 上限”，不包含 Go allocator 粒度、对象头、slice header、mutex、统计计数或运行时自身内存，因此不等于进程内存硬上限。

“容器满时非阻塞丢弃”的原则受 `oxtoacart/bpool` 启发；本实现不用其 channel 数据结构，不在 Release 路径分配替代 buffer，并增加全池字节预算。

## 7. 清零、安全与验证

### 7.1 清零

`ZeroOnRelease=false` 为默认值。开启时，合法 `Release` 对完整 capacity 执行 clear，而不只是当前 len；随后无论对象被保留还是丢弃，调用者释放的可访问存储都已经清零。验证失败的 foreign 或 duplicate 对象不得被修改，因为它可能属于其他调用者或已被重新借出。

Dirty 与 zeroing 是不同语义，Benchmark 必须分组，不能直接用 dirty 新实现与默认清零的实现比较后宣称胜出。

### 7.2 验证模式

`ValidationEnabled` 关闭时不维护所有权表。开启时，Pool 按 backing array 地址记录所属 pool、借出状态和内部世代：

- 申请时登记或激活记录。
- 归还时验证所属 pool、借出状态和容量等级。
- 检测到 foreign、连续 duplicate 或 invalid 时拒绝入池，并返回对应状态。
- 归还后可写入固定诊断字节，提高 stale alias 在测试中暴露的概率；若同时开启 `ZeroOnRelease`，清零语义优先，不写诊断字节。

验证表不能识别以下情况：切片已归还、同一 backing array 已被合法重新借出，旧别名随后再次调用原始 `Release`。地址相同导致 ABA，原始 `[]byte` 没有 generation token。文档必须保留此限制。

`Buffer` 自身不进入 storage pool，并保存私有生命周期状态，因此能检测其自身的重复释放和释放后方法调用。它仍不能撤销之前通过 `Bytes` 暴露的切片别名。

## 8. I/O 语义

- `ReadFrom` 追加数据，不覆盖已有内容。
- 连续 Reader 返回 `(0, nil)` 时采用有限空读计数，达到阈值后返回 `io.ErrNoProgress`，避免无限循环。
- Reader 返回 `n > 0` 与非 nil error 时，先保留已读字节，再返回累计字节数和该错误。
- `WriteTo` 对 `n < len(data)` 且 error 为 nil 的 Writer 返回 `io.ErrShortWrite`。
- `WriteTo` 成功后是否清空 Buffer 与 `bytes.Buffer.WriteTo` 保持一致：成功写出的前缀从逻辑内容移除；部分写入时只移除确认写出的部分。
- 所有长度计算先做整数溢出和 `MaxAcquireSize` 检查。

## 9. 统计与可观测性

`StatsEnabled=false` 时不执行每操作的通用原子计数。Bounded 为兑现容量预算而必须维护的 retained 数值不受该开关影响。

`Stats()` 返回快照，至少包含：

- acquire、hit、miss、release；
- retained、dropped full、dropped oversize、dropped invalid；
- validation 发现的 foreign 和 duplicate；
- zeroed bytes；
- 各容量等级的 hit、miss、retained 和 drop。

Bounded 的 `RetainedBuffers` 与 `RetainedBytes` 标记为 available 且数值准确到上述 `sum(cap)` 定义。Fast 明确标记为 unavailable，不返回估算值。

统计结构不依赖 Prometheus 或其他监控客户端。调用方自行把快照转换为自己的指标系统。

## 10. 测试策略

实现采用红—绿—重构的小步测试驱动：

1. 配置校验与容量映射。
2. Fast 的 miss/hit、容量隔离、oversize drop 和 Clear 世代。
3. Bounded 的预算竞争、满池 drop、精确保留统计和 Clear。
4. raw API 的长度/capacity、nil、append 后非法容量和限制错误。
5. 验证模式的 foreign、连续 duplicate、错误池和诊断填充。
6. Buffer 的写入、增长、Reset、copy check、Release 和失败原子性。
7. Reader/Writer 边界、短写、空读、部分成功和整数溢出。
8. zeroing、stats 开关和组合配置。
9. 并发 Acquire/Release 不会同时交付同一已借出 storage。
10. 配置与 Buffer 操作的 fuzz tests。

完成门槛：

```text
go test ./...
go vet ./...
go test -race ./...
```

如果本地 race detector 缺少 C 工具链或存在其他环境限制，必须保存失败输出并在交付说明中标为“未执行成功”，不能写成“测试通过”。

## 11. Benchmark 设计

### 11.1 独立 module

`benchmarks/` 使用独立 `go.mod`，固定所有竞品版本，使根模块保持零第三方依赖。竞品通过薄 adapter 调用公开 API，不复制源码。

### 11.2 分组

raw `[]byte` 组：

- `make([]byte, n)`
- 单一 `sync.Pool`
- 带 cap cutoff 的 `sync.Pool`
- `libp2p/go-buffer-pool`
- gRPC `mem.BufferPool`
- Prometheus `util/pool`
- 本项目 Fast
- 本项目 Bounded

append/writer 组：

- `bytes.Buffer`
- `sync.Pool` + `bytes.Buffer`，分别测无 cutoff 与有 cutoff
- `valyala/bytebufferpool`
- `oxtoacart/bpool.BufferPool` / `SizedBufferPool`
- 本项目 `Buffer`

第三方实现若无法满足完全相同的语义，只能出现在明确标注的子组中。例如 gRPC 默认清零，不能与 dirty 结果混排；固定宽度 `BytePool` 不能伪装成 variable-size pool。

### 11.3 工作负载

- 固定尺寸：64 B、1 KiB、4 KiB、16 KiB、64 KiB、1 MiB。
- 容量边界：每个等级的 `n-1`、`n`、`n+1`。
- 预生成的确定性混合尺寸 trace。
- 串行与 `RunParallel`。
- 冷池、热池和 GC 后 miss 分开。
- Bounded 预算耗尽与并发持有超过预算。
- 超大峰值后恢复小请求，并记录峰值前后、一次 GC、两次 GC 后的 heap 与池统计。
- zeroing、stats 和 validation 分别开关，单独报告成本。

计时区内不生成随机数，不调用 `runtime.GC()`。所有 adapter 返回相同逻辑长度、触碰相同字节并写入全局 sink，防止编译器消除工作。

### 11.4 结果发布

建议命令：

```text
go test -run '^$' -bench . -benchmem -benchtime=3s -count=10 -cpu=1,8
benchstat raw.txt
```

提交以下材料：

- Benchmark 源码和 adapter 语义测试。
- 第三方依赖的精确版本或 commit。
- 原始输出和 `benchstat` 汇总。
- Go 完整版本、GOOS/GOARCH、CPU、逻辑核、`GOMAXPROCS`、`GOGC`、memory limit 和命令。
- 独立进程运行的峰值污染内存结果及 heap profile。

README 只摘录代表性结果并链接原始材料。吞吐、分配、峰值内存和稳态保留分别报告，不压缩成单一“最快”排名。

## 12. 来源与许可证

项目代码使用 MIT License。只参考第三方的公开思想、行为与测试问题，不复制其实现代码。

具体规则：

1. 非显然算法旁添加 `Design reference:` 注释，包含项目 URL。
2. 注释同时说明本实现与参考实现的关键差异。
3. `docs/attribution.md` 集中列出参考项目、许可证、借鉴内容与未复制声明。
4. Benchmark adapter 只调用第三方公开 API；依赖许可证和版本写入 attribution。
5. code review 检查实现是否出现与第三方源码异常相似的结构、命名或注释。

## 13. 实施边界与顺序

第一版是一个可在单一实施计划中完成的库，不拆成多个独立产品。实施按以下依赖顺序推进：

1. module、许可证、配置与容量映射。
2. Fast 后端与 raw API。
3. Bounded 后端与预算统计。
4. 可选验证和清零。
5. Buffer 与 I/O 语义。
6. 文档、来源标注和示例。
7. 独立 Benchmark module、对照 adapter、原始结果和 README 摘要。
8. 全量测试、race、vet、Benchmark 复现和规格复核。

任何阶段发现 API 必须改变时，先更新本设计并取得确认，再继续实现。
