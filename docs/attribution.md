# Attribution and clean-room boundary

`bytebufferpool` is independently implemented. Public projects were read to understand API contracts, known failure modes, and established design ideas. Their source files were not copied, translated line-for-line, or renamed into this repository.

Non-obvious inspired library code carries an adjacent `Design reference:` URL and states how this implementation differs.

| Reference | Locked version/source | License | What informed the design | Independent difference |
| --- | --- | --- | --- | --- |
| Go `sync.Pool`, `strings.Builder`, and `bytes.Buffer` | Go 1.26.7 standard library | BSD-3-Clause | Runtime pool contract; self-pointer copy detection; geometric growth to amortize repeated appends | Capacity Classes, explicit retention semantics, shared storage tokens, and Lease lifecycle are project-specific; geometric reservation applies only when no Capacity Class fits and is clamped by `MaxAcquireSize` |
| valyala/bytebufferpool | v1.0.0 | MIT | Historical API, adaptive calibration, open failure modes, comparison target | New Lease-first API; deterministic classes; no copied implementation or compatibility layer |
| libp2p/go-buffer-pool | v0.1.0 | MIT | Per-size runtime pools and wrapper reuse | Explicit cutoff, custom classes, Generation, Lease ownership, Bounded mode, clearing, validation, and statistics |
| oxtoacart/bpool | commit `03653db5a59c` | Apache-2.0 | Non-blocking drop when a bounded container is full | Global byte-capacity CAS and per-class LIFO instead of channel capacity; no replacement allocation on Release |
| gRPC-Go `mem.BufferPool` | v1.83.2 | Apache-2.0 | Tiered public API and zeroing semantics | Oversize storage is dropped instead of entering an unbounded fallback; zeroing is opt-in and separately measured |
| Prometheus `util/pool` | v0.314.0 | Apache-2.0 | Geometric classes and public comparison behavior | Explicit validated class list, exact-capacity Release, no reflection in the library, and deterministic constructor errors |

## Benchmark-only dependencies

The separate Benchmark module pins and imports the public APIs above plus `golang.org/x/perf/cmd/benchstat`. These dependencies do not enter the library module. Adapter contract tests verify equivalent requested length, sufficient capacity, bytes touched, and cleanup semantics before timing.

## Evidence policy

- Dirty, zeroing, validation, statistics, Lease, Raw Slice, and Buffer results are labelled separately.
- Raw Slice and Buffer results are not merged into one ranking.
- Raw outputs, exact dependency versions, environment metadata, statistical summaries, reproduction commands, and heap profiles are committed.
- The README reports observed trade-offs and does not claim universal superiority.
