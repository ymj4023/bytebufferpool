# bytebufferpool v1.1 resource-governance specification

- Date: 2026-09-05
- Module: `github.com/ymj4023/bytebufferpool`
- Status: published as [GitHub Issue #16](https://github.com/ymj4023/bytebufferpool/issues/16)
- Target release: `v1.1.0`
- Minimum Go version: 1.22
- Sources: GitHub Issues #14 and #15, maintainer design rounds Q1–Q23, and the isolated Validation Tombstone memory probe

## Problem Statement

As a long-running Go service operator, I can enable enhanced Raw Slice validation to diagnose ownership mistakes, but inactive ownership records currently remain forever and `Clear` does not release their map storage. This makes a safety-oriented option capable of retaining metadata in proportion to every unique backing address it has observed.

As a Bounded Backend user, I can observe only global retained inventory. I cannot inspect exact idle inventory per Capacity Class or the Pool Generation, and several public contracts remain easy to misread: optional operation counters are not one transactional snapshot, Pool values must not be copied after use, and a Fast release accepted immediately before `Clear` may become unreachable at once.

As an operator interpreting ReleaseStatus, I also cannot distinguish a legitimate Unpooled Backing Storage release within the pooling cutoff from malformed storage because both currently use `DroppedInvalid`.

## Solution

Version 1.1 will make enhanced Raw Slice validation suitable for long-running Pools by bounding inactive Validation Tombstones while never evicting active ownership records. The limit is configurable, uses a measured default of 16,384 tombstones, evicts the oldest inactive record first, and is observable through exact Validation Inventory. `Clear` preserves active ownership while rebuilding validation state so inactive map storage can become garbage.

Stats will separately expose exact Bounded Class Inventory and the current Generation without making optional operation counters mandatory. ReleaseStatus will add a numerically compatible `DroppedUnpooled` outcome for valid class-gap storage, with a matching optional counter. Public documentation will state the copy, snapshot, and concurrent-Clear boundaries directly.

The Bounded Backend remains a Retained Capacity governor, not a total-memory or retained-object-count governor. Object-count limits and high-water metrics remain out of scope until supported by an actionable production workload.

## User Stories

1. As a long-running service operator, I want enhanced validation metadata to have a finite steady inactive history, so that diagnostic mode does not grow with every address ever observed.
2. As a Raw Slice user, I want active ownership records never evicted, so that the Pool does not forget storage I still own.
3. As an operator, I want the tombstone limit to apply only to inactive records, so that concurrent active ownership remains correct even above the configured history size.
4. As a library adopter, I want a measured default tombstone limit, so that enabling validation is safe without first inventing a value.
5. As a service with a known misuse rate, I want to configure the tombstone limit, so that I can trade diagnostic history for metadata cost.
6. As a maintainer, I want negative tombstone limits rejected, so that invalid policy does not reach runtime behavior.
7. As a maintainer, I want tombstone configuration rejected when validation is disabled, so that a silently ignored safety option cannot hide misconfiguration.
8. As a debugger, I want inactive records evicted oldest-first, so that the diagnostic window is deterministic and explainable.
9. As a debugger, I want a repeated Release whose tombstone remains present classified as duplicate, so that recent misuse remains precise.
10. As a debugger, I want a Release whose tombstone was evicted classified as foreign, so that the Pool does not claim knowledge it no longer has.
11. As a caller, I want both evicted duplicates and genuinely foreign slices rejected without mutation, so that weaker classification does not weaken ownership safety.
12. As an operator, I want `Clear` to remove inactive validation history, so that I can deliberately discard diagnostic cache state.
13. As a Raw Slice owner, I want `Clear` to preserve my active ownership record, so that my later Release still follows current Raw Slice semantics.
14. As an operator, I want `Clear` to replace the old validation map, so that tombstone bucket storage can become eligible for garbage collection.
15. As an operator, I want exact active Raw Slice counts, so that current callers can be distinguished from retained diagnostic history.
16. As an operator, I want exact Validation Tombstone counts, so that I can see the bounded history approach its limit.
17. As an operator, I want the effective tombstone limit reported, so that a defaulted configuration is observable.
18. As a Fast user, I want validation inventory available independently of operation counters, so that safety state does not require unrelated instrumentation.
19. As a Bounded user, I want exact idle object count per Capacity Class, so that skewed retention is visible.
20. As a Bounded user, I want exact Retained Capacity per Capacity Class, so that per-class bytes reconcile with the global budget.
21. As a Bounded user with counters disabled, I still want Class Inventory, so that mandatory budget state remains observable.
22. As a Fast user, I want Class Inventory marked unavailable rather than estimated, so that runtime-managed retention is not presented as fact.
23. As any Pool user, I want the current Generation in Stats, so that Clear-driven lifecycle changes are observable.
24. As an API user, I do not want a redundant ClearCount field, so that Generation remains the one lifecycle measure.
25. As a metrics consumer, I want optional operation counters documented as independently loaded values, so that I do not treat them as a transactional snapshot.
26. As a metrics consumer, I want Bounded and Validation inventories distinguished from optional operation counters, so that their stronger snapshot guarantees remain clear.
27. As a Go developer, I want Pool documented as non-copyable after first use, so that copying mutexes, atomics, and runtime pools is not mistaken for supported behavior.
28. As a Fast user, I want `Retained` documented as best-effort acceptance, so that a concurrent `Clear` immediately making the release unreachable is not surprising.
29. As a caller releasing valid class-gap storage, I want `DroppedUnpooled`, so that normal unpooled disposal is not reported as invalid input.
30. As a caller releasing storage above the pooling cutoff, I want `DroppedOversize` unchanged, so that the two disposal reasons remain distinct.
31. As an existing caller, I want all v1 ReleaseStatus numeric values preserved, so that upgrading does not silently reinterpret persisted or exported values.
32. As an operator with counters enabled, I want a DroppedUnpooled counter, so that legitimate unpooled traffic is measurable.
33. As a Lease and Raw Slice user, I want the same class-gap classification, so that acquisition style does not change capacity policy.
34. As a Raw Slice user with validation disabled, I want documentation to retain the existing provenance limitation, so that a foreign class-gap slice is not presented as distinguishable.
35. As a project maintainer, I want exact memory-probe evidence for the chosen tombstone default, so that the default is grounded in observed cost.
36. As a project maintainer, I want active-owner peaks explicitly excluded from the tombstone guarantee, so that a record-count limit is not overstated as total memory governance.
37. As a project maintainer, I want no background cleanup worker, so that Pool lifecycle and scheduling remain caller-controlled.
38. As a library consumer, I want no new runtime dependencies, so that the root module remains lightweight.
39. As a test author, I want all correctness behavior verified through exported APIs, so that validation and Stats internals remain replaceable.
40. As a maintainer, I want race tests to cover concurrent validation, Stats, Release, and Clear, so that the new inventory guarantees hold under load.
41. As a performance evaluator, I want validation limit overhead and memory recovery measured separately from ordinary Fast and Bounded throughput, so that safety cost is visible.
42. As an open-source user, I want the accepted behavior published as one coherent v1.1 specification, so that linked tickets do not drift apart.
43. As a module user, I want the completed compatible feature set released as signed `v1.1.0`, so that provenance and SemVer intent are explicit.

## Implementation Decisions

- `Config` gains `MaxValidationTombstones` as an integer count. Zero selects an effective default of 16,384 when enhanced validation is enabled; positive values set the exact inactive history limit; negative values are invalid.
- A non-zero tombstone limit with enhanced validation disabled is an invalid configuration. Configuration never silently enables validation or ignores the field.
- The tombstone limit applies only to inactive Validation Tombstones. Active Raw Slice ownership records are never rejected or evicted because of the limit.
- A valid Raw Slice Release converts its active ownership record into an inactive tombstone. When the limit would be exceeded, the oldest inactive tombstone is evicted first.
- FIFO bookkeeping must tolerate backing-address reuse. An obsolete queue entry must never evict a newer active record or a newer tombstone for the same address.
- A repeated Release with a live tombstone returns `RejectedDuplicate`. After eviction or Clear removes that tombstone, the same old Release returns `RejectedForeign`. Both paths reject the slice without clearing, filling, or retaining it.
- `Clear` replaces validation storage with a newly allocated map containing only active ownership records. Inactive tombstones and their FIFO history are discarded so the old map and queue can become garbage.
- An active Raw Slice that spans Clear remains tracked and retains the existing best-effort Raw Slice generation behavior when later released.
- Stats gains exact Validation Inventory fields for availability, active Raw Slice count, inactive Validation Tombstone count, and the effective maximum tombstone count. Validation Inventory is available whenever enhanced validation is enabled, independently of `StatsEnabled` and Pool mode.
- Stats gains `Generation` as an always-available unsigned lifecycle value. It starts at zero and advances once per successful Clear; no separate ClearCount is added.
- A new `ClassInventory` value contains Capacity, IdleStorageCount, and RetainedCapacity. Stats exposes a slice of Class Inventory for the Bounded Backend even when optional counters are disabled.
- Bounded Class Inventory is computed under the same per-class synchronization as global retained inventory and reconciles exactly with global RetainedStorageCount and RetainedCapacity. Fast mode continues to mark retained inventory unavailable and does not estimate per-class values.
- Existing `ClassStats` remains the optional per-operation counter surface. Its independently loaded counters are not described as one transactional snapshot.
- `DroppedUnpooled` is appended after all existing ReleaseStatus values so every existing numeric value remains stable. Its string form and optional aggregate counter are added.
- `DroppedUnpooled` applies when valid Backing Storage is at or below `MaxPooledCapacity` but no exact Capacity Class can retain it. Storage above the cutoff remains `DroppedOversize`; malformed or changed-capacity storage remains `DroppedInvalid`.
- Lease and Raw Slice paths share the new unpooled classification when provenance is known. Raw Slice with validation disabled retains its documented inability to distinguish some foreign storage from valid unpooled storage.
- Pool documentation states that Pool must not be copied after first use. Documentation also states that optional counters are non-transactional and that Fast `Retained` does not promise survival across concurrent Clear.
- The Bounded Backend remains governed only by Retained Capacity. No retained-object-count limit or total Go heap/RSS limit is introduced.
- High-water metrics are not added. Current exact inventories and Generation are the accepted actionable observability surface.
- The root module gains no third-party runtime dependency and no background goroutine or finalizer.
- Public API additions are compatible but intentionally ship in the minor release `v1.1.0`.

## Testing Decisions

- Correctness tests continue to use the exported package API from an external test package. No internal map, queue, mutex, or test hook is asserted.
- Configuration tests cover zero defaulting, positive limits, negative limits, validation-disabled incompatibility, immutable copied configuration, and both Pool modes.
- Validation tests use small explicit limits to prove FIFO eviction: recent tombstones remain duplicate while the oldest evicted tombstone becomes foreign.
- Tests hold more active Raw Slices than the tombstone limit and prove none are rejected or forgotten; releasing them reduces inactive history to the configured bound.
- Address-reuse tests prove stale FIFO entries cannot evict a current active owner or a newer tombstone.
- Clear tests create both active records and tombstones, then prove tombstones disappear, active owners remain valid, Generation advances, and later active Release enters the new bounded history.
- Rejected foreign and duplicate releases remain unmodified under both dirty and zeroing configurations.
- Validation Inventory tests assert exact active, tombstone, and effective-limit values through Stats with operation counters both disabled and enabled.
- Class Inventory contract tests run across all default Capacity Classes and representative custom classes, reconciling per-class counts and capacity with global inventory before and after Acquire, Release, budget exhaustion, and Clear.
- Fast tests assert retained and Class Inventory unavailability rather than runtime survival or exact counts.
- Generation tests cover construction, repeated Clear, both modes, counters disabled, and concurrent Stats reads.
- ReleaseStatus tests lock every pre-v1.1 numeric value, verify `DroppedUnpooled` is appended, and cover string conversion and unknown values.
- Classification tests distinguish class-gap, oversize, malformed, changed-capacity, stale, foreign, and duplicate releases through Lease and Raw Slice seams where each distinction is observable.
- Optional counter tests verify DroppedUnpooled without making Validation or Class Inventory depend on `StatsEnabled`.
- Concurrency and race tests exercise Acquire, Raw Slice Release, Stats, and Clear without relying on implementation hooks.
- The isolated validation memory harness repeats disabled/enabled runs, records effective limits and logical inventory, and separates logical bounds from HeapAlloc, HeapInuse, and RSS claims.
- Published evidence records raw samples, statistical summaries, exact revisions, Go/OS/CPU/runtime settings, and reproduction commands.
- Tests, vet, fuzz smoke, root race detection, Benchmark contracts, all-Benchmark smoke, validation-focused measurements, and isolated memory smoke are required before release.

## Out of Scope

- A hard limit on active Raw Slice ownership records or rejection of valid acquisition because validation has many active owners.
- A retained Backing Storage object-count limit.
- High-water metrics or resettable historical maxima.
- A hard limit on allocator metadata, Go heap, or process RSS.
- Exact retained inventory for the Fast Backend.
- Complete prevention of Raw Slice ABA or use-after-release through saved mutable aliases.
- Transactional consistency across every optional operation counter and inventory field in one Stats call.
- Background cleanup workers, periodic tombstone expiration, wall-clock TTLs, finalizers, or runtime-adaptive limits.
- Direct Prometheus or other monitoring-client integration.
- Changing existing ReleaseStatus numeric values or reclassifying Oversize Backing Storage as unpooled.

## Further Notes

- The isolated Windows/amd64 Go 1.25.7 probe measured approximately 53.33 bytes of retained HeapAlloc per inactive validation record. At 16,384 records, the observed median increment was about 853 KiB HeapAlloc and 880 KiB HeapInuse; this informs the default but is not a cross-version memory guarantee.
- Deleting entries from a Go map does not promise that peak bucket storage shrinks. The Clear contract therefore rebuilds active validation state rather than only deleting inactive entries in place.
- Validation Tombstone bounds and Class Inventory are separate from Retained Capacity. Their coexistence must not be marketed as total-memory governance.
- Issues #14 and #15 remain the source triage records. The published v1.1 specification becomes the contract for agent-ready implementation tickets.
- Rejected object-count and high-water proposals are preserved under `.out-of-scope/` with explicit reconsideration triggers.
- The final release will use the repository's configured SSH signing key and will not move any existing tag.
