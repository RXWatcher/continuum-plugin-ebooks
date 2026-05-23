# silo.ebooks — docs

These are the deep docs for the ebooks portal. The top-level [README](../README.md)
covers what the plugin is and the public knobs; the documents here go further
into how it's wired, how to operate it, and how to debug it when it misbehaves.

The split is **operator** (admin/SRE) vs **end-user** (customer) where it
helps.

## For operators

- [architecture.md](architecture.md) — component map, request flow,
  portal↔backend boundary, where state lives.
- [operations.md](operations.md) — install, Postgres bootstrap, backend
  selection, day‑2 operations, secret rotation.
- [scheduled-tasks.md](scheduled-tasks.md) — each scheduled task: what it
  does, the row it touches, the failure mode that gets it stuck.
- [cache-and-streaming.md](cache-and-streaming.md) — proxy vs cache modes,
  the single‑flight cache filler, LRU eviction, disk‑pressure symptoms,
  kepubify temp files.
- [reader-integrations.md](reader-integrations.md) — OPDS, KOReader
  kosync, Kobo Sync, Kindle send. Auth flows and lifecycle for each.
- [debugging.md](debugging.md) — symptom → root‑cause runbook keyed off
  the most common production failures.

## For end users

- [user-guide.md](user-guide.md) — how a reader uses the portal: linking
  KOReader, OPDS in Marvin/Moon+, Send to Kindle, Send to Kobo.

## Conventions

- Routes shown with the access level set in the manifest: `[public]`,
  `[auth]`, `[admin]`.
- Tables and columns referenced are the post-migration shape; see
  [`internal/migrate/files/`](../internal/migrate/files/) for the SQL.
- "Backend" always means an `ebook_backend.v1` provider plugin —
  `silo.bookwarehouse-ebook`, `silo.ebook-requests`, or
  `silo.local-ebooks`. "Portal" is this plugin.
