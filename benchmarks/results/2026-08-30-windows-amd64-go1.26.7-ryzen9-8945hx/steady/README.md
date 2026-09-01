# Steady-state Benchmark evidence

Generated on 2026-09-01 from source revision `498bb481121dc159d043f2dba23ec0e43b18f42a`.

The environment matches the parent directory: Windows/amd64, Go 1.26.7, AMD Ryzen 9 8945HX, 32 logical CPUs. Every group uses `-benchmem`; durations, sample counts, and CPU settings differ by workload and are recorded exactly in the commands below. Every benchmark name has at least six samples, so the committed `benchstat` summaries include finite 95% confidence intervals.

## Groups

| Raw output | Statistical summary | Samples | Workload |
| --- | --- | ---: | --- |
| `raw-1024.txt` | `raw-1024-benchstat.txt` | 240 | Raw requested-length API, fixed 1 KiB |
| `raw-mixed.txt` | `raw-mixed-benchstat.txt` | 240 | Pre-generated deterministic mixed-size trace |
| `buffer-16384.txt` | `buffer-16384-benchstat.txt` | 160 | Append-oriented Buffer, 16 KiB in 128-byte chunks |
| `raw-boundary.txt` | `raw-boundary-benchstat.txt` | 3240 | Every default Capacity Class at `n-1`, `n`, and `n+1` |
| `parallel.txt` | `parallel-benchstat.txt` | 120 | Raw and Buffer parallel workloads at CPU8 |
| `lifecycle-budget.txt` | `lifecycle-budget-benchstat.txt` | 80 | Cold, warm, post-GC, and Bounded budget exhaustion |

## Commands

```text
go test -run '^$' -bench '^BenchmarkRawFixed$/^1024$' -benchmem -benchtime=1s -count=10 -cpu=1,8
go test -run '^$' -bench '^BenchmarkRawMixed$' -benchmem -benchtime=1s -count=10 -cpu=1,8
go test -run '^$' -bench '^BenchmarkBufferFixed$/^16384$' -benchmem -benchtime=1s -count=10 -cpu=1,8
go test -run '^$' -bench '^BenchmarkRawBoundary$' -benchmem -benchtime=100ms -count=6 -cpu=1
go test -run '^$' -bench '^(BenchmarkRawParallel|BenchmarkBufferParallel)$' -benchmem -benchtime=500ms -count=6 -cpu=8
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
