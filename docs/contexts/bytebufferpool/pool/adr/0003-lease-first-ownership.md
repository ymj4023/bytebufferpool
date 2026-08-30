---
status: accepted
---

# Make Lease the default ownership boundary

The default acquisition API will return a non-copyable, generation-bound Lease whose provenance and duplicate-release checks are always active; explicitly named Raw Slice methods remain available for callers that accept weaker lifecycle guarantees. Buffer will own a Lease, old-generation Leases will be dropped after `Clear`, and dirty storage remains the performance default with optional full-capacity clearing because Go cannot revoke mutable slice aliases without adding a different ownership abstraction.
