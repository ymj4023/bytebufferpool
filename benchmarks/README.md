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

Record the complete Go version, GOOS/GOARCH, CPU, logical core count, GOMAXPROCS, GOGC, memory limit, project commit, and dependency versions beside every published result.
