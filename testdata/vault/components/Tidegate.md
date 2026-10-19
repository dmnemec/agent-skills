---
type: component
aliases:
  - TG
  - tide gate
part-of:
  - "[[Harbor]]"
calls:
  - "[[Ledgerline]]"
  - "[[Beacon]]"
owned-by:
  - "[[Mira Vantongeren]]"
---
# Tidegate

HTTP ingest gateway for [[Harbor]]. Authenticates tenants, applies rate limits and retries, and writes events to [[Ledgerline]].

## Edges

- part-of [[Harbor]]
- calls [[Ledgerline]] (writes events) — from [[2026-09-10 Harbor kickoff]]
- calls [[Beacon]] (dead-letter notifications) — from [[2026-09-08 Tidegate rate limits]]
- owned-by [[Mira Vantongeren]]

## Rate limits

Per-tenant token bucket, 200 events/s burst. See [[2026-09-08 Tidegate rate limits#Decisions]].
