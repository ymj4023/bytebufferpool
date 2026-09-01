# Peak-memory evidence — Windows amd64

This directory contains isolated-process memory evidence regenerated on 2026-09-01.

## Environment

- Project revision: `a433aa511f19328771019507f3e9fd622a796bb4`
- Working tree at build time: clean (`vcs.modified=false`)
- Go: `go1.26.7`
- GOOS/GOARCH: `windows/amd64`
- CPU: AMD Ryzen 9 8945HX with Radeon Graphics
- Logical CPUs / GOMAXPROCS: 32 / 32
- GOGC: default
- Go memory limit: unlimited (`9223372036854775807`)

## Workload

- Contenders: 11
- Isolated repetitions: 3 per contender
- Steady request: 1 KiB, 10,000 acquire/release operations per steady phase
- Concurrent peak: 8 MiB × 8 held simultaneously
- Phases: steady-small, peak-held, peak-released, recovered-small, GC1, GC2
- Profiles: one heap profile after GC2 for every isolated run

## Reproduction

Build the harness from the recorded revision, then run:

```text
memorybench -contender all -small-size 1024 -small-iterations 10000 -peak-size 8388608 -peak-count 8 -repeat 3 -profile-dir benchmarks/results/2026-08-30-windows-amd64-go1.26.7-ryzen9-8945hx/profiles -output benchmarks/results/2026-08-30-windows-amd64-go1.26.7-ryzen9-8945hx/memory.json
```

`memory.json` contains all 33 raw child results and 66 min/mean/max summaries. The binary `.pprof` files are the corresponding GC2 heap profiles.

Repeated steady-state raw output and `benchstat` summaries are documented in [`steady/README.md`](./steady/README.md).

## Interpretation boundary

- `HeapAlloc`, `HeapInuse`, and `HeapSys` are Go runtime measurements, not Pool inventory.
- Only Bounded reports exact Retained Capacity; other contenders mark it unavailable.
- `peak-released` is sampled before explicit GC and is intentionally distinct from GC1 and GC2.
- This evidence describes one machine and workload. It is not a universal performance ranking.
