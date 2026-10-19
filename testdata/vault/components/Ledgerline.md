---
type: component
aliases:
  - LL
  - the ledger
part-of:
  - "[[Harbor]]"
owned-by:
  - "[[Mira Vantongeren]]"
---
# Ledgerline

Append-only event store behind [[Harbor]]. [[Tidegate]] writes to it; a nightly compaction job rewrites cold segments.

## Schema

Events use envelope v2: a fixed header plus an opaque payload. Older notes: [[Ledgerline schema v2 notes]].
