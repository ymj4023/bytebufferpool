// Package bytebufferpool provides deterministic, ownership-aware reuse of byte storage.
//
// Pool offers Fast best-effort retention and Bounded exact Retained Capacity.
// Lease is the default ownership boundary; Raw Slice is an explicitly weaker
// alternative. Buffer provides append and standard I/O behavior on top of Lease.
package bytebufferpool
