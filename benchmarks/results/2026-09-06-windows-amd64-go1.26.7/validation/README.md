# v1.1 Validation Tombstone memory evidence

This is an isolated-process observation, not a Go heap or RSS limit. Each of the
four configurations ran in five fresh processes. See [raw samples](./raw.jsonl),
[summary](./summary.json), and [source fingerprints](./source-sha256.json).

Environment: Windows/amd64, Go 1.26.7, AMD Ryzen 9 8945HX, 32 logical processors;
the harness sets GOMAXPROCS=1 and GOGC=off, leaves GOMEMLIMIT at the runtime
default, and runs two explicit GCs before every sample. The library is the #17
working tree based on `fec94e0b9fd99cd9350689e6fd685a0924e930c3`; fingerprints
identify every root Go source and the harness used. No competitor is measured.

## Workload and interpretation

The Fast Pool has one 64-byte Capacity Class. All requests are 65 bytes, so no
payload is retained. The harness records baseline, simultaneously active peak,
post-release peak, 1,024 subsequent batches of 128 acquisitions/releases, and
post-Clear. Each batch holds all allocations until acquisition finishes, ensuring
distinct live addresses within the batch; addresses may be reused between batches.

With validation enabled, active peaks of both 16,384 and 65,536 remain fully
tracked. After releasing either peak, inactive history is exactly 16,384; churn
does not exceed that limit. Clear leaves zero active records and zero tombstones.
The disabled control reports no Validation Inventory throughout.

Median post-release HeapAlloc is approximately 2.05 MiB at the 16,384 peak and
6.56 MiB at the 65,536 peak, versus about 0.55 MiB with validation disabled.
Post-Clear HeapAlloc is approximately 0.55 MiB in both enabled cases. These are
whole-process live-heap observations, not isolated struct sizes. In particular,
the larger active peak leaves map allocation high-water even after the logical
tombstone count falls to 16,384. The new FIFO metadata costs more than the older
unbounded-record prototype described in the spec; its earlier 853 KiB estimate
must not be treated as the v1.1 implementation's cost or a budget.

## Reproduction (PowerShell, from repository root)

```powershell
go -C benchmarks build -o ../.tmp/validationmem.exe ./cmd/validationmem
1..5 | ForEach-Object {
    foreach ($peakSize in 16384,65536) {
        foreach ($enabled in 'false','true') {
            & .tmp/validationmem.exe -peak $peakSize "-validation=$enabled"
            if ($LASTEXITCODE -ne 0) { throw 'validationmem failed' }
        }
    }
} > raw.jsonl
```

For `go -C`, the output path is relative to the benchmark directory. Summaries subtract
each process's baseline independently before computing min/median/max per phase.
HeapInuse and HeapAlloc are measured; RSS is not measured. Explicit GC eligibility
does not imply that the OS immediately reclaims resident pages.
