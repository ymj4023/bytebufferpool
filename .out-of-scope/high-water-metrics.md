# Pool High-Water Metrics

The Pool will not add retained-capacity, object-count, or validation high-water metrics in the v1.1 observability work.

## Why this is out of scope

High-water values are history-dependent counters that require reset semantics, additional synchronization, and an operational interpretation before callers can act on them. The accepted v1.1 scope instead exposes current exact Class Inventory, current Validation Inventory, and Generation; these describe present Pool state without turning Stats into a monitoring subsystem.

This can be reconsidered when a production monitoring use case defines which high-water value is actionable, how it is reset, and whether tracking overhead is acceptable on the affected path.

## Prior requests

- [#15 — Audit resource-governance contracts and observability](https://github.com/ymj4023/bytebufferpool/issues/15)
