# Benchmark suite

This module compares equivalent byte-storage work without adding competitor dependencies to the library module.

## Semantic groups

- `BenchmarkRaw*` measures requested-length byte slices. Lease, Raw Slice, dirty, zeroing, validation, and statistics modes are named separately.
- `BenchmarkBuffer*` measures append-oriented buffers receiving the same pre-generated chunks.
- gRPC is labelled `ZeroOnAcquire`; it is not ranked as if it performed the same work as a dirty pool.

All adapters have contract tests for requested length, sufficient capacity, touched bytes, and empty reuse. Timed regions do not generate random values or call `runtime.GC`.

## Commands

Run contract tests:

```text
go test ./...
```

Smoke-test every Benchmark with one iteration:

```text
go test -run '^$' -bench . -benchtime=1x
```

Collect repeated steady-state samples:

```text
go test -run '^$' -bench 'Benchmark(RawFixed|RawMixed|RawParallel|BufferFixed|BufferParallel)$' -benchmem -benchtime=3s -count=10 -cpu=1,8
```

Collect explicit lifecycle and budget samples with setup and GC outside timed regions:

```text
go test -run '^$' -bench '^(BenchmarkRawLifecycle|BenchmarkBoundedBudgetExhaustion)$' -benchmem -benchtime=20x -count=10 -cpu=1,8
```

Collect post-cutoff small-chunk and streaming Buffer growth samples:

```text
go test -run '^$' -bench '^(BenchmarkBufferPostCutoffGrowth|BenchmarkBufferReadFromPostCutoff)$' -benchmem -benchtime=1s -count=10 -cpu=1
```

The committed [Issue #13 before/after evidence](./results/2026-09-02-windows-amd64-go1.26.7-ryzen9-8945hx/issue-13/README.md) uses the same benchmark source against fixed library revisions and records the exact comparison procedure.

Record the complete Go version, GOOS/GOARCH, CPU, logical core count, GOMAXPROCS, GOGC, memory limit, project commit, and dependency versions beside every published result.

## Isolated-process memory suite

Build or run the memory harness with one contender per child process:

```text
go run ./cmd/memorybench -contender all -repeat 3 -profile-dir results/<environment>/profiles -output results/<environment>/memory.json
```

Each child records steady small traffic, a concurrently held peak, post-release recovery, restored small traffic, and samples after one and two explicit GC cycles. The output contains raw samples plus min/mean/max summaries across repeated isolated runs. Retained Capacity is reported only by contenders that can measure it exactly.
