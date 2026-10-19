---
type: meeting
date: 2026-09-10
project: "[[Harbor]]"
topics:
  - "[[Harbor]]"
  - "[[Ledgerline]]"
attendees:
  - "[[Mira Vantongeren]]"
  - Sana Okafor-Lind
  - Teodor Quill
---
# 2026-09-10 Harbor kickoff

## Notes

### Shape of the pipeline

- Tenants push to [[Tidegate]], which writes to [[Ledgerline]].
- [[Beacon]] stays a shared service; [[Harbor]] is one caller among several.
- Mira sketched the flow on the whiteboard:

```mermaid
flowchart LR
    tenant([Tenant])  -->|HTTPS| tg[Tidegate]
    tg -->|append|  ll[(Ledgerline)]
    tg -. dead-letter .-> bc{{Beacon}}
    %% hand-drawn: keep as is
    ll --> compaction[[nightly compaction]]
```

### Timeline

- Beta with three tenants in October, general availability after the holidays.

## Decisions

- [[Ledgerline]] stays append-only; corrections are new events.
- Beta goes out with three tenants.

## Constraints and requirements

- Tenants must not see each other's events, even in logs.

## Open questions

- Who runs the [[Ledgerline]] on-call rotation once beta starts?

## Action items

- [ ] Write the beta tenant checklist [owner:: Sana Okafor-Lind] [source:: [[2026-09-10 Harbor kickoff]]]
- [x] Create the harbor-ingest repo [owner:: Teodor Quill] [source:: [[2026-09-10 Harbor kickoff]]]
