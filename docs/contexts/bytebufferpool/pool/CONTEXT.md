# Pool

The Pool context defines reusable byte storage, how callers own it, and what the pool may retain after use.

## Language

**Pool**:
An object that lends reusable byte storage under one immutable capacity, retention, and safety configuration.
_Avoid_: Cache, allocator, memory pool

**Backing Storage**:
The byte array temporarily owned by a Lease, Raw Slice, or Buffer and eligible for reuse by its Pool after release.
_Avoid_: Payload, data buffer

**Capacity Class**:
An exact backing-storage capacity that a Pool is allowed to retain and lend for requests up to that size.
_Avoid_: Bucket, tier, bin

**Lease**:
A non-copyable value granting exclusive, generation-bound ownership of one Backing Storage allocation.
_Avoid_: Handle, borrow, token

**Raw Slice**:
A low-level `[]byte` loan that bypasses Lease lifecycle protection while retaining the Pool's capacity contract.
_Avoid_: Unsafe Lease, untracked Lease

**Release**:
The transfer of Backing Storage ownership from a Lease, Raw Slice, or Buffer back to its Pool for retention or disposal.
_Avoid_: Put, free

**Fast Backend**:
The Pool retention mode that prioritizes throughput and treats retained storage as best-effort rather than measurable inventory.
_Avoid_: Unbounded Backend, sync mode

**Bounded Backend**:
The Pool retention mode that enforces a hard Retained Capacity budget for idle Backing Storage.
_Avoid_: Strict Backend, limited mode

**Retained Capacity**:
The sum of capacities of idle Backing Storage currently held by a Bounded Backend; it is not Go heap usage or process RSS.
_Avoid_: Retained memory, heap size

**Generation**:
The Pool epoch captured by a Lease so storage borrowed before `Clear` cannot enter the new epoch when released.
_Avoid_: Version, revision

**Validation Tombstone**:
An inactive Raw Slice ownership record retained within a bounded diagnostic history so a later Release may be classified as duplicate.
_Avoid_: Leaked record, stale owner

**Validation Inventory**:
The exact active Raw Slice and Validation Tombstone counts maintained while enhanced validation is enabled.
_Avoid_: Validation memory, raw map size

**Class Inventory**:
The exact count and Retained Capacity of idle Backing Storage in one Bounded Backend Capacity Class.
_Avoid_: ClassStats, bucket metrics

**Unpooled Backing Storage**:
Valid Backing Storage that no Capacity Class can retain; storage within the pooling cutoff is distinct from Oversize Backing Storage.
_Avoid_: Invalid storage, pooled fallback
