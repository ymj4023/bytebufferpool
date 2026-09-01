# Steady-state Benchmark evidence

Generated on 2026-09-01 from source revision `bda31718064b241fe8e9b954be8691fa6dfa8c66`.

The environment matches the parent directory: Windows/amd64, Go 1.26.7, AMD Ryzen 9 8945HX, 32 logical CPUs. Each group uses `-benchmem -benchtime=1s -count=10 -cpu=1,8`.

## Groups

| Raw output | Statistical summary | Samples | Workload |
| --- | --- | ---: | --- |
| `raw-1024.txt` | `raw-1024-benchstat.txt` | 240 | Raw requested-length API, fixed 1 KiB |
| `raw-mixed.txt` | `raw-mixed-benchstat.txt` | 240 | Pre-generated deterministic mixed-size trace |
| `buffer-16384.txt` | `buffer-16384-benchstat.txt` | 160 | Append-oriented Buffer, 16 KiB in 128-byte chunks |
| `raw-boundary.txt` | `raw-boundary-benchstat.txt` | 1620 | Every default Capacity Class at `n-1`, `n`, and `n+1` |
| `parallel.txt` | `parallel-benchstat.txt` | 100 | Raw and Buffer parallel workloads at CPU8 |
| `lifecycle-budget.txt` | `lifecycle-budget-benchstat.txt` | 80 | Cold, warm, post-GC, and Bounded budget exhaustion |

## Commands

```text
go test -run '^$' -bench '^BenchmarkRawFixed$/^1024$' -benchmem -benchtime=1s -count=10 -cpu=1,8
go test -run '^$' -bench '^BenchmarkRawMixed$' -benchmem -benchtime=1s -count=10 -cpu=1,8
go test -run '^$' -bench '^BenchmarkBufferFixed$/^16384$' -benchmem -benchtime=1s -count=10 -cpu=1,8
go test -run '^$' -bench '^BenchmarkRawBoundary$' -benchmem -benchtime=100ms -count=3 -cpu=1
go test -run '^$' -bench '^(BenchmarkRawParallel|BenchmarkBufferParallel)$' -benchmem -benchtime=500ms -count=5 -cpu=8
go test -run '^$' -bench '^(BenchmarkRawLifecycle|BenchmarkBoundedBudgetExhaustion)$' -benchmem -benchtime=20x -count=10 -cpu=1,8
go tool benchstat raw-1024.txt
go tool benchstat raw-mixed.txt
go tool benchstat buffer-16384.txt
go tool benchstat raw-boundary.txt
go tool benchstat parallel.txt
go tool benchstat lifecycle-budget.txt
```

## Interpretation boundary

- Raw Slice, Lease, and Buffer groups perform different work and are not merged into one ranking.
- gRPC is labelled `ZeroOnAcquire`; Project RawZero, RawValidation, and RawStats are named separately from dirty Raw.
- The mixed trace is generated before timing. Explicit GC is not called inside timed regions.
- These results describe this machine, Go version, source revision, and workload. They do not establish a universal fastest pool.
