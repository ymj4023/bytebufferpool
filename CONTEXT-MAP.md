# Context map

This is the root entry point for the repository's cascading domain context.

## Contexts

- [bytebufferpool](./docs/contexts/bytebufferpool/CONTEXT-MAP.md) — owns the language for pooled byte storage and append-oriented buffers.

## Relationships

- **bytebufferpool → standard library**: the package implements standard Go I/O interfaces where their contracts fit, but owns its pooling and lifecycle terminology.

## Map rules

- An entry may point to a leaf `CONTEXT.md` or a child `CONTEXT-MAP.md`.
- Every entry states the scope owned by its target.
- Child maps may recursively point to narrower maps.
- Every context document must remain reachable from this root.
