---
status: accepted
---

# Use cascading domain maps

Domain terminology and architectural decisions will be discoverable through recursive `CONTEXT-MAP.md` and `ADR-MAP.md` indexes, because a flat root index becomes noisy as narrower scopes emerge. Every child document must remain reachable from the root, and maps contain navigation and scope rather than duplicated glossary or decision bodies.
