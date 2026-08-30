# Domain docs

This repository uses cascading context and ADR maps.

## Entry points

Before exploring or changing the repository:

1. Read the root `CONTEXT-MAP.md`.
2. Follow every child context map relevant to the task until reaching its leaf `CONTEXT.md`.
3. Read the root `ADR-MAP.md`.
4. Follow every child ADR map relevant to the task and read the referenced ADRs.

If a referenced map or leaf does not exist, surface the broken pointer. If no entry exists for the current area, proceed with the known sources and note a possible domain-model gap.

## Map contract

Each map owns one scope.

A map entry may point to:

- A leaf document in the current scope.
- A child map representing a narrower scope.

Every entry states what the target owns, so an agent can decide whether to follow it without loading unrelated contexts.

Maps contain navigation and relationships, not duplicated glossary entries or decision bodies.

## Context documents

A leaf `CONTEXT.md` contains only canonical project terminology:

- One or two sentences per term.
- Preferred name and explicitly avoided synonyms.
- No implementation plan, task status, or architectural decision history.

When terminology spans multiple contexts, define it at their nearest common ancestor instead of duplicating it.

## ADR documents

- Root `docs/adr/` contains repository-wide decisions.
- A context's `adr/` directory contains decisions owned by that context.
- ADRs use sequential numbers within their owning scope.
- `ADR-MAP.md` records links, status, scope, and supersession relationships.
- The decision and rationale live in the ADR, not in the map.

Create an ADR only when the decision is hard to reverse, surprising without context, and the result of a real trade-off.

## Lazy growth

Create child maps, leaf contexts, and ADR directories only when there is real content to record. When a new scope is created, update its parent map in the same change so every document remains reachable from the root.

## Vocabulary and conflicts

Use terms exactly as defined by the relevant leaf context.

If proposed work conflicts with an ADR, name the ADR and surface the conflict explicitly. Do not silently override an accepted decision.
