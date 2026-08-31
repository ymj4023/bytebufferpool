# v1 completion audit

This audit maps Spec Issue #1 to authoritative implementation, tests, documentation, and committed Benchmark evidence. It does not treat intent or the absence of an obvious failure as proof.

## User stories

| # | Requirement | Evidence |
| ---: | --- | --- |
| 1 | Explicit immutable Pool configuration | `Config`, `New`, and `TestPoolCopiesCapacityClassesAtConstruction` |
| 2 | Useful default Capacity Classes | `DefaultConfig` and `TestFastPoolAcquiresDeterministicLease` |
| 3 | Custom Capacity Classes | `Config.Classes`, `PowerOfTwo`, and configuration tests |
| 4 | Reject invalid or ambiguous classes | `ErrInvalidConfig` and `TestPoolRejectsAmbiguousCapacityClasses` |
| 5 | Non-panicking acquisition for untrusted lengths | `TryAcquire`, `TryAcquireSlice`, `TryBuffer`, and invalid-size tests |
| 6 | Concise trusted-length acquisition | `Acquire`, `AcquireSlice`, and `Buffer` |
| 7 | Requested length and sufficient capacity | Lease and Raw adapter contract tests |
| 8 | Lease as the default ownership boundary | `Lease`, Pool Context, and ADR-0003 |
| 9 | Detect duplicate Lease Release | shared block token/active state and duplicate-release tests |
| 10 | Fail immediately after Lease Release | `checkUsable` and post-release panic tests |
| 11 | Generation-bound Lease | `Clear`, `DroppedStale`, and Clear tests |
| 12 | Clearly named Raw Slice API | `AcquireSlice`, `TryAcquireSlice`, `ReleaseSlice` |
| 13 | Same Raw Slice capacity contract | cross-backend Raw Slice tests and Benchmark adapter contracts |
| 14 | Document Raw Slice ABA/alias limits | Raw Slice GoDoc and README Raw Slice section |
| 15 | Reject observable foreign and cross-Pool Raw releases | enhanced validation map and ownership tests |
| 16 | Reject observable duplicate Raw releases | validation state and duplicate-mutation test |
| 17 | Optional full-capacity clearing | `ZeroOnRelease` and capacity-wide zero tests |
| 18 | Clearing disabled by default | `DefaultConfig` and README configuration semantics |
| 19 | Rejected storage remains unmodified | foreign/duplicate zeroing tests |
| 20 | Throughput-oriented Fast retention | per-class runtime pools and Fast mode tests/Benchmarks |
| 21 | Fast inventory reported unavailable | `Stats.RetainedAvailable` and README mode table |
| 22 | Capacity-bounded retention | Bounded backend, ADR-0002, and concurrent budget tests |
| 23 | Non-blocking drop at full budget | atomic reservation and `DroppedFull` tests |
| 24 | Exact Bounded inventory | mandatory retained counters and Stats tests |
| 25 | Drop oversize Backing Storage | release classification and oversize tests |
| 26 | Drop non-class capacity | exact-capacity routing and class-gap tests |
| 27 | Clear idle storage | generation replacement and Clear tests |
| 28 | Distinguish Lease and Raw Clear semantics | stale Lease and current-generation Raw tests/documentation |
| 29 | Append-oriented Buffer | Buffer public API and examples |
| 30 | Buffer owns Lease | Buffer growth implementation, Context relationship, ADR-0003 |
| 31 | Growth promptly releases old Backing Storage | Bounded inventory assertion after growth |
| 32 | Failed growth preserves content | failure-atomic Buffer test |
| 33 | Standard Writer composition | compile-time interface assertions and examples |
| 34 | Terminate zero-progress Reader | `io.ErrNoProgress` test |
| 35 | Correctly report short writes | `io.ErrShortWrite` test |
| 36 | Preserve bytes returned with Reader error | data-plus-error test |
| 37 | Overflow and acquisition checks before mutation | Buffer growth checks and failure-atomic tests |
| 38 | Optional operational counters | `Stats`, `ClassStats`, and deterministic counter tests |
| 39 | Counters disabled by default | default config and disabled-counter inventory test |
| 40 | Correctness tests use public seam | root tests use external package `bytebufferpool_test` |
| 41 | Public concurrency/race verification | concurrent budget/Clear tests and CI race detector |
| 42 | Benchmark dependencies isolated | separate Benchmark module with root `replace`; root module has no requirements |
| 43 | Separate Raw, Lease, and Buffer results | Benchmark names, adapters, and results documentation |
| 44 | Separate dirty, clearing, stats, and validation costs | named project adapters and steady/memory evidence |
| 45 | Broad workload matrix | fixed, all class boundaries, mixed, parallel, budget, peak, and GC workloads |
| 46 | Raw results and reproducibility metadata | steady raw files/benchstat and memory JSON/profiles/READMEs |
| 47 | Attribute non-obvious design ideas | three adjacent `Design reference:` comments and attribution table |
| 48 | Report multiple result dimensions without universal claim | README throughput/allocation tables and memory evidence boundary |

## Implementation decisions

| # | Decision | Evidence |
| ---: | --- | --- |
| 1 | Clean-room, no valyala compatibility layer | root ADR-0001, attribution, independent public API |
| 2 | No library runtime dependencies | root `go.mod` contains no `require` directive |
| 3 | Immutable configuration | constructor copies classes; no mutation API |
| 4 | Default 64 B–1 MiB powers of two | `DefaultConfig` and `PowerOfTwo` |
| 5 | First-fitting acquire and exact-capacity release | Pool routing functions and boundary tests |
| 6 | Default 1 MiB pooling cutoff | configuration constant and defaults |
| 7 | Optional maximum acquisition size | validation and Buffer failure tests |
| 8 | Non-copyable value Lease with provenance/token state | Lease implementation and copy tests |
| 9 | Old Generation Release is stale | atomic generation replacement and Clear tests |
| 10 | Explicitly named Raw Slice without Generation | Raw Slice API and documentation |
| 11 | Enhanced validation only adds expensive Raw tracking | optional record map; Lease checks remain unconditional |
| 12 | Stable ReleaseStatus taxonomy | constants, String method, examples, and outcome tests |
| 13 | Non-copyable Buffer owns/replaces Lease | Buffer implementation and copy/growth tests |
| 14 | Buffer aliases expire on mutation/Release | GoDoc and README |
| 15 | One Pool type with Fast and Bounded modes | Config Mode and shared public API tests |
| 16 | Per-class best-effort Fast runtime pools | generation Fast classes and unavailable inventory |
| 17 | Per-class Bounded LIFO under atomic capacity budget | Bounded implementation and concurrent tests |
| 18 | Retained Capacity is idle `sum(cap)` only | Context, Stats, README, memory evidence |
| 19 | Clear replaces current Generation | atomic current pointer and tests |
| 20 | Optional full-capacity clearing | release preparation and zero tests |
| 21 | Go-standard I/O edge behavior | Reader/Writer adversarial tests |
| 22 | Optional counters; mandatory Bounded inventory | counter allocation gate and Stats tests |
| 23 | No monitoring-client dependency | Stats is a library-owned snapshot type |
| 24 | Benchmark competitors only through public APIs | adapter code and contract tests |
| 25 | Adjacent source reference plus independent difference | three library comments and attribution |
| 26 | Go 1.22 and MIT | root `go.mod`, LICENSE, CI Go 1.22 job |

## Out-of-scope audit

| Excluded item | Evidence |
| --- | --- |
| valyala compatibility/global API | no compatibility package or global Pool functions |
| runtime adaptive sizing | Capacity Classes are immutable after construction |
| dynamic per-class quota tuning | Bounded uses one fixed global configured budget |
| background cleanup/finalizers | library starts no goroutine and registers no finalizer |
| total Go heap/RSS hard limit | public contract and docs limit the claim to Retained Capacity |
| complete mutable-alias safety | Raw Slice ABA limitation is explicit |
| exact Fast inventory | Stats marks it unavailable |
| monitoring-client integration | root module has no external requirements |
| universal performance ranking | README explicitly reports observed workload-specific trade-offs |
| production trace | mixed trace is documented as deterministic synthetic input |

## Verification evidence

- Local root module: full tests and `go vet` passed.
- Local Benchmark module: full tests, `go vet`, and all-Benchmark one-iteration smoke passed.
- Fuzz smoke: `FuzzPoolConfiguration` ran about 2.8 million executions; `FuzzBufferOperations` ran about 335 thousand executions without failure.
- CI repeatedly passed Go 1.22 tests/vet, stable tests/vet, and Linux race detection.
- CI Benchmark job passed adapter contracts, all-Benchmark one-iteration smoke, and all-contender isolated memory smoke.
- Steady evidence: 240 Raw 1 KiB samples, 240 Raw mixed samples, 160 Buffer 16 KiB samples, each with locked benchstat summaries.
- Peak evidence: 33 isolated raw results, 66 phase summaries, and 33 GC2 heap profiles bound to clean revision `8b95e892e8abcaf01abc52fc96a55f0a89df1266`.
- All Context and ADR leaves are reachable from the root cascading maps.
- Every repository Markdown local link resolves.
- `git diff --check` passes after generated-output normalization.
