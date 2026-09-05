---
status: accepted
---

# Bound Raw Slice validation tombstones

Enhanced Raw Slice validation is supported for long-running Pools, so inactive Validation Tombstones will be retained in a configurable FIFO history with a default limit of 16,384 while active ownership records are never evicted. Evicting a tombstone intentionally degrades a later Release from duplicate to foreign, and `Clear` rebuilds validation state from active records so inactive map storage can become garbage; this bounds steady diagnostic history without claiming a hard limit on active owners, Go heap, or RSS.

## Consequences

Validation remains diagnostic rather than memory safety: mutable aliases still have ABA ambiguity, and a peak number of active Raw Slices may temporarily exceed the tombstone limit and raise the map's allocation high-water mark. The bounded history was chosen over permanent tombstones, which grow with every unique address, and over clearing active records, which would discard current ownership provenance.
