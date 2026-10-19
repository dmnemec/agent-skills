---
type: meeting
date: 2026-09-08
project: "[[Harbor]]"
topics:
  - "[[Tidegate]]"
attendees:
  - "[[Mira Vantongeren]]"
  - Oskar Fenwick
  - Lio Haverkamp
---
# 2026-09-08 Tidegate rate limits

## Notes

### Current limits

- [[Tidegate]] has a global limit only, so one noisy tenant starves the rest.
- Burst traffic from the nightly batch tenants hits [[Ledgerline|the ledger]] all at once.

### Per-tenant buckets

- Token bucket per tenant, refilled every second.
- Bucket config lives next to tenant auth config.
- [[Beacon]] gets a call when a tenant is throttled for more than 5 minutes.

## Decisions

- Per-tenant token bucket, 200 events/s burst by default.
- Throttling over 5 minutes notifies the tenant through [[Beacon]].

## Constraints and requirements

- Must not add a network hop on the hot path: bucket state stays in process.
- [[Beacon]] accepts at most 50 notifications/s per caller.

## Open questions

- Should bucket config be per tenant or per tenant tier?

## Action items

- [ ] Prototype in-process token bucket [owner:: [[Mira Vantongeren]]] [source:: [[2026-09-08 Tidegate rate limits]]]
- [ ] Share throttle notification payload with Beacon [owner:: Lio Haverkamp] [source:: [[2026-09-08 Tidegate rate limits]]]
