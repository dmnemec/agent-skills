---
type: project
aliases:
  - HBR
  - Harbor ingest
  - the Harbor project
repos:
  - harbor-ingest
  - https://git.example.com/larkspur/harbor-ledger.git
links:
  - https://tracker.example.com/browse/HBR
---
# Harbor

Harbor is the Larkspur team's new event ingest pipeline: tenants push events through [[Tidegate]], which stores them in [[Ledgerline]] and notifies downstream consumers through [[Beacon]].

Lead: [[Mira Vantongeren]]

## Components

- [[Tidegate]]: ingest gateway
- [[Ledgerline]]: event store
- [[Beacon]]: notification service (shared, not owned by Harbor)

## Meetings

- [[2026-09-08 Tidegate rate limits]]
- [[2026-09-10 Harbor kickoff]]
