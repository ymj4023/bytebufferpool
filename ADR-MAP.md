# ADR map

This is the root entry point for the repository's cascading architectural decision records.

## Repository-wide decisions

- [ADR-0001: Build a clean-room API](./docs/adr/0001-clean-room-api.md) — accepted; repository scope.
- [ADR-0002: Use cascading domain maps](./docs/adr/0002-cascading-domain-maps.md) — accepted; repository scope.

## Context decision maps

- [bytebufferpool decisions](./docs/contexts/bytebufferpool/ADR-MAP.md) — public pooling and buffer semantics.

## Map rules

- Repository-wide ADRs live under `docs/adr/`.
- Context-specific ADRs live under their owning context.
- An entry may point to an ADR or a child `ADR-MAP.md`.
- Record each ADR's scope and status.
- Record supersession links without copying the decision body.
- Every ADR must remain reachable from this root.
