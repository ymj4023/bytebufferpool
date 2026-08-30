---
status: accepted
---

# Use deterministic capacity classes

Pool sizing will use immutable, validated Capacity Classes selected explicitly by configuration, and Backing Storage outside those classes will not be retained. This rejects runtime self-calibration because history-dependent sizing makes memory behavior, warmup, tests, and Benchmark results difficult to explain or reproduce.
