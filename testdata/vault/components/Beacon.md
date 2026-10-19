---
type: component
aliases:
  - beacon service
depends-on:
  - "[[Ledgerline]]"
related-to:
  - "[[Tidegate]]"
---
# Beacon

Shared notification service. Sends webhooks and emails when events need a human or another system. Reads event metadata from [[Ledgerline]].

## Edges

- depends-on [[Ledgerline]] (reads event metadata)
- related-to [[Tidegate]] (main caller)

## Interfaces

- `POST /notify`: one notification, rate limited to 50/s per caller
- Webhook contract for dead-letter events: draft pending
