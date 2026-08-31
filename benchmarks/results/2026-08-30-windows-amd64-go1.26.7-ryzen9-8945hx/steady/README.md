# Steady-state Benchmark evidence

Generated on 2026-08-31 from source revision `9d0b0064d82eddfb7c0e0bc5df3ecc514564d7a3`.

The environment matches the parent directory: Windows/amd64, Go 1.26.7, AMD Ryzen 9 8945HX, 32 logical CPUs. Each group uses `-benchmem -benchtime=1s -count=10 -cpu=1,8`.

## Groups

| Raw output | Statistical summary | Samples | Workload |
| --- | --- | ---: | --- |
| `raw-1024.txt` | `raw-1024-benchstat.txt` | 240 | Raw requested-length API, fixed 1 KiB |
| `raw-mixed.txt` | `raw-mixed-benchstat.txt` | 240 | Pre-generated deterministic mixed-size trace |
| `buffer-16384.txt` | `buffer-16384-benchstat.txt` | 160 | Append-oriented Buffer, 16 KiB in 128-byte chunks |

## Commands

```text
go test -run '^$' -bench '^BenchmarkRawFixed$/^1024$' -benchmem -benchtime=1s -count=10 -cpu=1,8
go test -run '^$' -bench '^BenchmarkRawMixed$' -benchmem -benchtime=1s -count=10 -cpu=1,8
go test -run '^$' -bench '^BenchmarkBufferFixed$/^16384$' -benchmem -benchtime=1s -count=10 -cpu=1,8
go tool benchstat raw-1024.txt
go tool benchstat raw-mixed.txt
go tool benchstat buffer-16384.txt
```

## Interpretation boundary

- Raw Slice, Lease, and Buffer groups perform different work and are not merged into one ranking.
- gRPC is labelled `ZeroOnAcquire`; Project RawZero, RawValidation, and RawStats are named separately from dirty Raw.
- The mixed trace is generated before timing. Explicit GC is not called inside timed regions.
- These results describe this machine, Go version, source revision, and workload. They do not establish a universal fastest pool.
