# v1.1 integration validation measurements

Library revision: `0bcd6243a384d36fa6af46bb0152f068e5881c25` (all four feature
tickets integrated). Benchmark source Git blob: `9d2c3fda1c8be6a5545ba6577cead5ebca270e3b`,
file `benchmarks/validation_benchmark_test.go`, SHA256
`094B76BB92DD5AF6675BCB8941FD7C5C0DF97A5077DC9DCA8EBCA37C11E740B4`.
The library source is unchanged by the integration tests and this report.

Environment: Windows/amd64, Go 1.26.7, AMD Ryzen 9 8945HX with Radeon Graphics,
32 logical processors, benchmark `-cpu=1` (GOMAXPROCS=1), default GOGC=100 and
GOMEMLIMIT=off. Benchmark-module versions are pinned in `benchmarks/go.mod` and
`go.sum`; no competitor is timed here. benchstat is the pinned x/perf tool at
`v0.0.0-20260825160852-19be9d8e6c70`.

## Results

Each row has 10 samples at `-benchtime=1s`. Values below are medians; see
[raw output](./validation-raw.txt) and [benchstat](./validation-benchstat.txt)
for uncertainty and allocations. Do not combine the two workload units.
Only trailing spaces in the CPU header were trimmed for repository whitespace checks.

| Mode / history | 64-byte hot reuse, ns/op | 128 × 65-byte batch, µs/op |
| --- | ---: | ---: |
| Fast / validation disabled | 62.09 | 19.64 |
| Fast / limit 1 | 127.8 | 46.31 |
| Fast / limit 16,384 | 127.2 | 56.83 |
| Bounded / validation disabled | 51.58 | 19.73 |
| Bounded / limit 1 | 122.5 | 46.29 |
| Bounded / limit 16,384 | 122.7 | 56.74 |

Hot reuse is 0 B/op and 0 allocs/op for every row. A batch holds 128 distinct
live oversize addresses before Release and has 256 allocs/op, including payload
and wrapper allocations. Addresses may be reused between batches. The batch
workload is deliberately unpooled; neither mode retains those payloads.

`limit=0` in benchmark names means validation **disabled**, not a Config zero
limit with validation enabled. The latter defaults to 16,384, verified by tests.
ZeroOnRelease and optional counters are off. Validation includes diagnostic
filling, map bookkeeping and locking: this is total option overhead, not an
isolated claim about FIFO cost. GC is not explicitly invoked inside timed loops.
The larger diagnostic window costs more in this churn workload; no universal
fastest or memory-budget claim follows.

## Memory and reproduction

The [isolated memory report](../2026-09-06-windows-amd64-go1.26.7/validation/README.md)
contains 20 fresh-process samples, min/median/max summaries and source fingerprints
for the validation implementation. It shows a 16,384 tombstone logical bound,
active peaks above that bound, allocation high-water and post-Clear recovery.
The old prototype's 853 KiB estimate is not the v1.1 FIFO implementation's cost.

From the repository root, with the stated Go toolchain:

```text
go -C benchmarks test -run ^$ -bench ^BenchmarkValidationHistory$ -benchmem -benchtime=1s -count=10 -cpu=1
go -C benchmarks tool benchstat results/2026-09-08-windows-amd64-go1.26.7/validation-raw.txt
```

For memory reproduction and interpretation use the linked report. The final CI
also runs the isolated harness against the fully integrated source; retained
capacity, heap allocations and RSS remain different quantities.
