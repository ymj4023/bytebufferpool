# Issue #13 — post-cutoff Buffer growth evidence

This directory compares the same committed benchmark source against two fixed library revisions:

- Before: `2abad057b63b6d2f1f4e1d2a62dbd35aafba86ba` (`v1.0.1` source behavior)
- After: `6d7c4732b82328b9ba14109455e658e8050416e9`
- Benchmark source: after revision `6d7c4732b82328b9ba14109455e658e8050416e9`

## Environment

- Date: 2026-09-02
- Go: `go1.26.7`
- GOOS/GOARCH: `windows/amd64`
- CPU: AMD Ryzen 9 8945HX with Radeon Graphics
- Logical CPUs: 32
- Benchmark CPU setting: `-cpu=1`
- GOMAXPROCS during measurement: 1
- GOGC: default (`GOGC` unset)
- Go memory limit: default/unlimited (`GOMEMLIMIT` unset)
- Sampling: `-benchtime=1x -count=6 -benchmem`
- Source worktrees: clean before each build

## Artifacts

| File | Contents |
| --- | --- |
| [`before-2abad05.txt`](./before-2abad05.txt) | Raw samples with the library module replaced by a clean detached worktree at the before revision |
| [`after-6d7c473.txt`](./after-6d7c473.txt) | Raw samples from the clean after revision |
| [`benchstat.txt`](./benchstat.txt) | Before/after statistical comparison |

## Key results

| Project workload | Before sec/op | After sec/op | Before B/op | After B/op | Before allocs/op | After allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 MiB + 1 byte, 4 KiB writes | 449.1 µs | 393.6 µs | 3.008 MiB | 4.000 MiB | 63 | 63 |
| 2 MiB, 4 KiB writes | 47.7026 ms | 354.1 µs | 387.011 MiB | 4.000 MiB | 571 | 63 |
| 8 MiB, 4 KiB writes | 719.460 ms | 1.485 ms | 8073.08 MiB | 16.00 MiB | 3643 | 67 |
| 2 MiB `ReadFrom` | 330.481 ms | 708.1 µs | 3084.105 MiB | 8.003 MiB | 4164 | 70 |

The 2 MiB, 8 MiB, and `ReadFrom` improvements are significant at `p=0.002`, with six samples per benchmark name. The `cutoff+1` timing and allocation-count differences are not significant; its B/op increase of 32.99% is the expected cost of reserving geometric headroom. The `bytes.Buffer` control keeps the same allocation counts across revisions. Because `1x` timing has visible variance, allocation volume and count are the primary regression signal.

## Reproduction

Create a clean detached worktree for the before revision. Use the current `benchmarks` module as the shared harness, and create an alternate modfile identical to `benchmarks/go.mod` except for a replace directive pointing at the before worktree:

```text
replace github.com/ymj4023/bytebufferpool => <before-worktree>
```

Run the after and before samples with identical parameters:

```text
go test -run '^$' -bench '^(BenchmarkBufferPostCutoffGrowth|BenchmarkBufferReadFromPostCutoff)$' -benchmem -benchtime=1x -count=6 -cpu=1
go test -modfile=<before.mod> -mod=mod -run '^$' -bench '^(BenchmarkBufferPostCutoffGrowth|BenchmarkBufferReadFromPostCutoff)$' -benchmem -benchtime=1x -count=6 -cpu=1
go tool benchstat before-2abad05.txt after-6d7c473.txt
```

The alternate modfile and detached worktree are measurement scaffolding only. Neither changes the committed library or Benchmark module dependencies.
