# Retained Storage Count Limit

The Bounded Backend will continue to enforce Retained Capacity rather than adding a second hard limit on the number of idle Backing Storage objects.

## Why this is out of scope

Retained Capacity is the established, exact contract and already constrains the byte arrays that dominate ordinary pooled storage. A separate object-count limit would introduce another configuration dimension, drop policy, default, and interaction with per-class Capacity Classes without a real workload showing that idle object metadata dominates the capacity budget.

This can be reconsidered when production traces demonstrate a material metadata problem from very small custom Capacity Classes. That evidence must identify the relevant object counts and acceptable drop behavior rather than treating a count limit as a proxy for total Go heap or RSS.

## Prior requests

- [#15 — Audit resource-governance contracts and observability](https://github.com/ymj4023/bytebufferpool/issues/15)
