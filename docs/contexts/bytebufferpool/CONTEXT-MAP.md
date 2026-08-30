# bytebufferpool context map

This scope owns the public language of the byte buffer pool library.

## Contexts

- [Pool](./pool/CONTEXT.md) — owns reusable storage, capacity, retention, and ownership terms.
- [Buffer](./buffer/CONTEXT.md) — owns the append-oriented value built on Pool leases.

## Relationships

- **Buffer → Pool**: a Buffer exclusively owns one Pool Lease and replaces that Lease when it grows.
- **Pool → Buffer**: Pool defines the storage and release contracts that Buffer follows.
