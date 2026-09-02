# bytebufferpool

[中文说明](./README.ZH.md)

`bytebufferpool` is a deterministic, ownership-aware byte storage pool for Go.

It is a clean-room implementation with a new API. It is not a drop-in replacement for `github.com/valyala/bytebufferpool`.

## Why another pool?

- Explicit, immutable Capacity Classes instead of traffic-driven calibration.
- A Lease-first API that carries Pool provenance, Generation, and duplicate-release state.
- A clearly named Raw Slice escape hatch for callers that accept weaker lifecycle guarantees.
- Fast and Bounded retention modes behind one Pool contract.
- A hard Retained Capacity budget in Bounded mode.
- Optional full-capacity clearing, enhanced Raw Slice validation, and operation counters.
- Reproducible steady-state and peak-memory evidence without a universal “fastest” claim.

The library module has no third-party runtime dependencies and supports Go 1.22 or newer.

## Install

```text
go get github.com/ymj4023/bytebufferpool
```

## Lease-first use

```go
pool, err := bytebufferpool.New(bytebufferpool.DefaultConfig(bytebufferpool.Fast))
if err != nil {
	return err
}

lease := pool.Acquire(1500)
defer lease.Release()

payload := lease.Bytes() // len=1500, cap=2048 with default classes
copy(payload, source)
```

Lease is a non-copyable value. `Bytes` is valid only until Release. A repeated Release returns `RejectedDuplicate`; other methods panic after Release.

`TryAcquire` returns `ErrInvalidSize` for untrusted input. `Acquire` panics for the same programmer/configuration error.

## Raw Slice

```go
payload := pool.AcquireSlice(1500)
defer pool.ReleaseSlice(payload)
```

Raw Slice has the same length and Capacity Class contract as Lease, but carries no Generation token. It cannot reliably distinguish an old alias after the same backing address has been acquired again. Enable `ValidationEnabled` to reject observable foreign, cross-Pool, duplicate, and changed-capacity releases; this improves diagnosis but does not create memory safety.

If append replaces the Backing Storage, do not release the replacement as if it were the original borrowed slice.

## Buffer

```go
buffer := pool.Buffer(1024)
defer buffer.Release()

_, _ = buffer.WriteString("hello")
_ = buffer.WriteByte(' ')
_, _ = buffer.Write([]byte("world"))
_, _ = buffer.WriteTo(destination)
```

Buffer is non-copyable, owns a Lease, and implements `io.Writer`, `io.ByteWriter`, `io.StringWriter`, `io.ReaderFrom`, and `io.WriterTo`. Growth acquires a new Lease, copies live content, and immediately releases the old Lease. When no Capacity Class can satisfy the request, Buffer reserves capacity geometrically; the resulting unpooled Backing Storage is not retained, and `MaxAcquireSize` still caps each acquisition. Failed growth preserves existing content.

## Capacity and retention

Default Capacity Classes are powers of two from 64 B through 1 MiB. Acquisition selects the first class that fits. Release routes by capacity; oversize and non-class storage is dropped.

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

| Mode | Retention | Inventory |
| --- | --- | --- |
| Fast | One runtime-managed pool per Capacity Class | Best-effort; exact retained values unavailable |
| Bounded | Per-class LIFO under a global byte-capacity budget | Exact idle Backing Storage count and Retained Capacity |

Retained Capacity is `sum(cap)` of idle Backing Storage. It is not allocator overhead, Go heap, `HeapSys`, or process RSS.

`Clear` discards currently idle storage and advances Generation. A pre-Clear Lease returns `DroppedStale`; a pre-Clear Raw Slice has best-effort semantics because it carries no Generation.

## Clearing, validation, and statistics

```go
config := bytebufferpool.DefaultConfig(bytebufferpool.Bounded)
config.ZeroOnRelease = true
config.ValidationEnabled = true
config.StatsEnabled = true
```

- `ZeroOnRelease` clears the complete capacity of every valid release, even when that storage will be dropped.
- Foreign and duplicate validation failures are never modified.
- Zeroing takes precedence over diagnostic filling.
- Optional counters report acquire, hit, miss, Release outcomes, validation rejection, zeroed bytes, and per-class activity.
- Bounded inventory remains available when optional counters are disabled because it is required to enforce the budget.

## ReleaseStatus

Release reports one of:

- `Retained`
- `DroppedFull`
- `DroppedOversize`
- `DroppedInvalid`
- `DroppedStale`
- `RejectedForeign`
- `RejectedDuplicate`
- `IgnoredNil`

In Fast mode, `Retained` means accepted by a best-effort runtime pool; the runtime may discard the value at any time.

## Benchmark results

These are medians from Windows/amd64, Go 1.26.7, AMD Ryzen 9 8945HX. The two fixed-size tables use 10 samples with `-benchtime=1s -cpu=1,8`; the lifecycle table uses 10 samples with `-benchtime=20x -cpu=1,8`. The tables show the CPU=1 result and intentionally compare only the named workload.

### Raw requested-length API — 1 KiB

| Contender | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `make` | 174.0 | 1024 | 1 |
| `sync.Pool` with cutoff | 24.59 | 0 | 0 |
| Project Fast Lease | 71.77 | 0 | 0 |
| Project Fast Raw | 65.29 | 0 | 0 |
| Project Bounded Raw | 58.47 | 0 | 0 |
| libp2p v0.1.0 | 48.06 | 0 | 0 |
| gRPC v1.83.2, zero on acquire | 47.18 | 0 | 0 |
| Prometheus v0.314.0 | 135.8 | 48 | 2 |

### Append Buffer — 16 KiB in 128-byte chunks

| Contender | µs/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| new `bytes.Buffer` | 7.502 | 32.02 KiB | 10 |
| pooled `bytes.Buffer` with cutoff | 1.756 | 96 | 1 |
| valyala v1.0.0 | 1.568 | 96 | 1 |
| bpool SizedBufferPool | 6.618 | 28.14 KiB | 5 |
| Project Fast Buffer | 3.763 | 96 | 1 |
| Project Bounded Buffer | 3.882 | 96 | 1 |

The project is not universally fastest in these workloads. Its distinguishing behavior is deterministic sizing, explicit ownership, stale-generation rejection, and an exact optional Retained Capacity budget.

### Lifecycle and budget — Project Fast Raw, 1 KiB

| State | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Cold | 712.5 | 1,378 | 4 |
| Warm | 32.50 | 0 | 0 |
| PostGC | 502.5 | 353 | 5 |
| Bounded two-Release budget exhaustion | 110.0 | 8 | 1 |

### Concurrent 8 MiB × 8 peak

| Contender | Peak held HeapAlloc | Peak released HeapAlloc | GC2 HeapAlloc | Exact Retained Capacity |
| --- | ---: | ---: | ---: | ---: |
| `sync.Pool` with cutoff | 64.66 MiB | 64.66 MiB | 0.66 MiB | unavailable |
| Project Fast | 64.67 MiB | 64.68 MiB | 0.66 MiB | unavailable |
| Project Bounded | 64.67 MiB | 64.68 MiB | 0.67 MiB | 1 KiB |
| libp2p v0.1.0 | 64.68 MiB | 64.69 MiB | 0.67 MiB | unavailable |
| gRPC v1.83.2 | 64.66 MiB | 64.67 MiB | 0.66 MiB | unavailable |
| Prometheus v0.314.0 | 64.82 MiB | 64.82 MiB | 0.66 MiB | unavailable |

The peak-memory suite ran 11 contenders in fresh child processes, three repetitions each. It stores 33 raw results, 66 phase summaries, and 33 GC2 heap profiles, all bound to clean revision `a433aa511f19328771019507f3e9fd622a796bb4`. Bounded retained 1 KiB of steady idle storage and never exceeded its 32 MiB budget; runtime heap measurements remain separate from Pool inventory.

- [Steady-state raw output and benchstat summaries](./benchmarks/results/2026-08-30-windows-amd64-go1.26.7-ryzen9-8945hx/steady/README.md)
- [Peak-memory raw results and profiles](./benchmarks/results/2026-08-30-windows-amd64-go1.26.7-ryzen9-8945hx/README.md)
- [Benchmark methodology and commands](./benchmarks/README.md)
- [Market and source research](./docs/research/byte-buffer-pools.md)

## Attribution and clean-room boundary

Source inspirations, versions, licenses, and implementation differences are recorded in [`docs/attribution.md`](./docs/attribution.md). Non-obvious inspired algorithms also carry adjacent `Design reference:` comments. No third-party implementation or tests are copied into the library.

## License

MIT
