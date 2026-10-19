# Ledgerline schema v2 notes

Notes from when [[Ledgerline]] moved to envelope v2. Current schema lives at [[Ledgerline#Schema]].

- header: version, event_type, received_at
- tenant_id still inside the payload in v2
- payload is opaque bytes, max 256 KB
