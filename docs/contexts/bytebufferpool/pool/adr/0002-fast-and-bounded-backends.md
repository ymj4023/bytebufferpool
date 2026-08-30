---
status: accepted
---

# Offer Fast and Bounded retention backends

One public Pool type will offer Fast and Bounded retention modes under the same acquisition contract: Fast prioritizes throughput without claiming exact retained inventory, while Bounded enforces Retained Capacity as `sum(cap)` of idle Backing Storage. Keeping these semantics distinct avoids pretending that a runtime-managed best-effort pool can also provide a hard byte budget.
