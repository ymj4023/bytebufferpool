# bytebufferpool v1 specification seed

- Date: 2026-08-29
- Module: `github.com/ymj4023/bytebufferpool`
- Status: published as [GitHub Issue #1](https://github.com/ymj4023/bytebufferpool/issues/1)
- Minimum Go version: 1.22
- License: MIT

## Sources of truth

- Domain entry point: [`CONTEXT-MAP.md`](../../CONTEXT-MAP.md)
- Decision entry point: [`ADR-MAP.md`](../../ADR-MAP.md)
- Market research: [`byte-buffer-pools.md`](../research/byte-buffer-pools.md)

This specification defines what v1 must implement and how completion is verified. The ADRs own the reasons for the hard-to-reverse choices; this file does not duplicate those rationales.

## Goals

Version 1 must provide:

1. Deterministic, configurable Capacity Classes.
2. A default Lease API with explicit ownership and generation semantics.
3. An explicitly named Raw Slice API for callers that accept weaker lifecycle protection.
4. An append-oriented Buffer built on Lease.
5. Fast and Bounded retention backends behind one Pool type.
6. A per-object pooling cutoff and an optional single-acquisition limit.
7. A hard Retained Capacity budget in Bounded mode.
8. Optional full-capacity clearing, raw-slice validation, and operational statistics.
9. Correct Reader/Writer edge behavior.
10. Reproducible Benchmark results against representative alternatives.

## Non-goals

Version 1 will not provide:

- API compatibility with `github.com/valyala/bytebufferpool`.
- Package-level global pools.
- Runtime self-calibration or adaptive Capacity Classes.
- Background cleanup goroutines or finalizer-based release.
- Per-class dynamic quotas or automatic rebalancing.
- A claim that mutable `[]byte` aliases become memory-safe after release.
- Copied third-party implementation or test code.

## Configuration

```go
type Mode uint8

const (
	Fast Mode = iota
	Bounded
)

type Config struct {
	Mode              Mode
	Classes           []int
	MaxPooledCapacity int
	MaxRetainedCapacity  int64
	MaxAcquireSize    int
	ZeroOnRelease     bool
	ValidationEnabled bool
	StatsEnabled      bool
}

func DefaultConfig(mode Mode) Config
func PowerOfTwo(minimum, maximum int) ([]int, error)
func New(config Config) (*Pool, error)
```

Defaults:

- Capacity Classes: powers of two from 64 B through 1 MiB.
- `MaxPooledCapacity`: 1 MiB.
- `MaxAcquireSize`: 0, meaning no additional single-acquisition limit.
- `MaxRetainedCapacity`: 32 MiB in `DefaultConfig(Bounded)` and invalid when non-zero in Fast mode.
- Clearing, enhanced validation, and optional statistics: disabled.

Configuration validation:

- Classes must be positive, strictly increasing, unique, and no greater than `MaxPooledCapacity`.
- `PowerOfTwo` requires positive power-of-two endpoints and includes both endpoints.
- A zero `MaxPooledCapacity` selects the default; a negative value is invalid.
- A non-zero `MaxAcquireSize` must be at least `MaxPooledCapacity`; a negative value is invalid.
- Bounded mode requires `MaxRetainedCapacity > 0`.
- Fast mode rejects non-zero `MaxRetainedCapacity` because it cannot honor an exact inventory budget.
- When the last custom class is smaller than `MaxPooledCapacity`, requests in the gap are unpooled.

## Lease API

```go
func (p *Pool) Acquire(size int) Lease
func (p *Pool) TryAcquire(size int) (Lease, error)

func (l *Lease) Bytes() []byte
func (l *Lease) Len() int
func (l *Lease) Cap() int
func (l *Lease) Release() ReleaseStatus
```

`TryAcquire` returns a Lease whose bytes have `len == size` and `cap >= size`. `Acquire` uses the same behavior but panics on a size error that `TryAcquire` would return.

A Lease is a non-copyable value. It carries Pool provenance and the Generation in which its Backing Storage was acquired. Provenance, Generation, runtime copy checks, and duplicate-release state are always enabled; they are not controlled by `ValidationEnabled`.

The implementation uses a runtime copy check. A `go vet` no-copy lock marker is deliberately omitted because Lease is returned by value and the copylocks analyzer would flag every valid constructor return. Copying remains forbidden even though a copy made before the first runtime check cannot be perfectly distinguished by the receiver address; the shared storage token and release state must still prevent the same Backing Storage from entering the Pool twice.

The first `Release` transfers ownership and returns a ReleaseStatus. A repeated `Release` returns `RejectedDuplicate` and does not re-enter storage. Calls other than `Release` after release panic.

`Bytes` aliases the Backing Storage and is valid only until Release. Go cannot revoke a saved alias; retaining and using it after Release violates the ownership contract.

## Raw Slice API

```go
func (p *Pool) AcquireSlice(size int) []byte
func (p *Pool) TryAcquireSlice(size int) ([]byte, error)
func (p *Pool) ReleaseSlice(buffer []byte) ReleaseStatus
```

The Raw Slice API follows the same length, capacity, allocation-limit, clearing, retention, and statistics policies as Lease, but it does not carry a generation token.

`AcquireSlice(0)` returns nil and `ReleaseSlice(nil)` returns `IgnoredNil`. A returned slice may be ordinarily resliced, but release requires the original Backing Storage and its full capacity. If append replaces the Backing Storage, the replacement is not the borrowed object.

With enhanced validation enabled, Pool tracks raw Backing Storage ownership and rejects observable foreign, cross-Pool, duplicate, and invalid releases. It cannot reliably distinguish an old alias after the same address has been legitimately acquired again; this ABA limitation must appear in package documentation.

## ReleaseStatus

ReleaseStatus must distinguish at least:

- `Retained`
- `DroppedFull`
- `DroppedOversize`
- `DroppedInvalid`
- `DroppedStale`
- `RejectedForeign`
- `RejectedDuplicate`
- `IgnoredNil`

In Fast mode, `Retained` means the object was accepted by the best-effort backend; the Go runtime may discard it at any time.

## Buffer API

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

Buffer is a non-copyable value that owns one Lease. When growth is required, it acquires a sufficiently large new Lease, copies existing content, then releases the old Lease. Growth beyond `MaxPooledCapacity` but within `MaxAcquireSize` uses unpooled storage.

Growth failure must preserve existing content. Length arithmetic must detect integer overflow before allocation or mutation.

`Bytes` is valid only until the next mutation or Release. Repeated Release returns `RejectedDuplicate`; other calls after Release panic.

## Capacity routing

- Acquisition selects the first Capacity Class greater than or equal to the requested length.
- Default power-of-two classes may use `math/bits`; custom classes use a deterministic search.
- Release routes by capacity, not length.
- Only a capacity exactly equal to a configured class may be retained.
- Storage beyond `MaxPooledCapacity`, beyond the largest class, or with a non-class capacity is dropped.
- Routing never depends on request history, GC count, or warmup order.

## Retention backends

Pool has one immutable Mode.

Fast mode:

- Uses one runtime-managed pool per Capacity Class.
- Never mixes capacities in one class.
- May recycle slice wrappers to avoid avoidable interface allocations.
- Does not expose exact retained inventory.
- `Clear` advances Generation and replaces the reachable internal pools.

Bounded mode:

- Maintains a per-class LIFO of idle Backing Storage.
- Uses an atomic global Retained Capacity reservation.
- Drops a release without blocking when the capacity budget cannot be reserved.
- Reports the exact idle Backing Storage count and Retained Capacity according to `sum(cap)`.
- `Clear` releases current idle lists and resets retained inventory when no concurrent release refills them.

`MaxRetainedCapacity` limits idle byte-array capacity only. It is not a hard limit on allocator overhead, metadata, Go heap, or process RSS.

## Clear semantics

`Clear` removes currently idle storage and advances Pool Generation. A Lease acquired before Clear returns `DroppedStale` when released and cannot populate the new Generation.

Raw Slice has no generation token. A slice acquired before Clear may be accepted into the current generation when later released if it satisfies capacity and validation rules. This is an explicit best-effort limitation.

## Clearing and enhanced validation

`ZeroOnRelease` clears the full capacity of every valid release before the object is retained or dropped. Rejected foreign or duplicate storage must not be modified because another owner may be using it.

When `ValidationEnabled` is true, the Raw Slice path maintains its ownership table and may fill valid released storage with a diagnostic byte. If zeroing is also enabled, zeroing takes precedence and diagnostic fill is skipped.

Dirty and zeroing modes are different semantics and must have separate Benchmark results.

## I/O behavior

- `ReadFrom` appends instead of replacing existing content.
- Repeated `(0, nil)` reads terminate with `io.ErrNoProgress` after a finite empty-read limit.
- A read returning both data and an error preserves the data and returns the cumulative count with the error.
- `WriteTo` returns `io.ErrShortWrite` when a Writer reports a short write with no error.
- Successfully written bytes are removed from Buffer; a partial write removes only its confirmed prefix.
- Operations that would exceed `MaxAcquireSize` or overflow `int` return an error without partially mutating Buffer.

## Statistics

Optional counters include acquire, hit, miss, release, retained, each drop reason, validation rejection, and zeroed bytes, with per-class breakdowns where applicable.

When `StatsEnabled` is false, optional per-operation counters are not updated. Bounded mode still maintains the retained counters needed to enforce its budget.

Bounded snapshots lock the per-class idle lists to report an exact Backing Storage count and Retained Capacity. Fast snapshots mark retained inventory unavailable rather than estimating values that the runtime can invalidate silently. Statistics have no dependency on a monitoring client.

## Verification

Implementation proceeds in red-green-refactor slices and must cover:

1. Configuration and every Capacity Class boundary.
2. Fast hit/miss, class isolation, oversize drop, Generation, and Clear.
3. Bounded budget races, full drops, retained inventory, and Clear.
4. Lease provenance, copy misuse, duplicate Release, stale Generation, and post-release calls.
5. Raw nil behavior, foreign/cross-Pool/duplicate releases, append replacement, and ABA documentation.
6. Buffer writes, growth, failure atomicity, Reset, Release, and alias validity.
7. Reader/Writer zero progress, short writes, partial success, errors, and overflow.
8. Clearing, enhanced validation, statistics, and their combinations.
9. Concurrent acquisition never handing the same leased Backing Storage to two live owners.
10. Fuzz tests for configuration and Buffer operations.

Completion checks:

```text
go test ./...
go vet ./...
go test -race ./...
```

Any check that cannot run must be reported as not successfully executed, with its failure output preserved.

## Benchmark results

`benchmarks/` is a separate Go module so competitor dependencies do not enter the library module.

Raw Slice comparisons include `make`, a naive single `sync.Pool`, a cap-cutoff `sync.Pool`, libp2p, gRPC, Prometheus, and both project backends. Append/writer comparisons include `bytes.Buffer`, pooled `bytes.Buffer` with and without cutoff, valyala, bpool, and project Buffer.

Results are separated by semantic equivalence. Dirty and zeroing results, Raw Slice and Lease costs, statistics on/off, and enhanced validation on/off must not be merged into one ranking.

Workloads cover fixed sizes, class boundaries, a deterministic mixed trace, serial and parallel execution, cold/warm/post-GC states, exhausted Bounded budgets, oversize traffic, and recovery after large peaks.

Published results include benchmark source, adapter contract tests, exact dependency versions, raw output, `benchstat` summaries, Go/OS/CPU/runtime settings, heap profiles for peak tests, and reproduction commands. README reports throughput, allocations, peak heap, and steady retained capacity separately and does not claim universal superiority.

## Attribution

The root module has no third-party runtime dependency. Every non-obvious algorithm influenced by a public implementation receives an adjacent `Design reference:` comment containing the source URL and the differences in this implementation. `docs/attribution.md` records referenced projects, versions, licenses, and the clean-room boundary.

Benchmark adapters call public third-party APIs and do not copy their implementations.

## Delivery order

1. Module, license, configuration, and Capacity Class routing.
2. Lease and Fast backend.
3. Bounded backend and retained inventory.
4. Raw Slice enhanced validation and clearing.
5. Buffer and I/O behavior.
6. Documentation, attribution, and examples.
7. Benchmark module, adapters, raw results, and README summary.
8. Full tests, race, vet, benchmark reproduction, and spec review.

Any required public API change first updates the relevant Context, ADR when the reason changes, and this specification before implementation continues.
