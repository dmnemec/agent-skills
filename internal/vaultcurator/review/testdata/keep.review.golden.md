<!-- vault-curator review plan-sha256=484c0d36ed57bd99698e6ac95cb38c7fb780bd206f0f325638730cb276a425cc -->
# Curation review: 2026-09-14.json

Decide each change by setting its `status:` to `approved` or `rejected`. A category's status decides its auto operations (listed as decided by the category status), so approve a category and then reject the exceptions. Every other operation has its own `op` line and is decided one by one: sync fails until each is `approved` or `rejected`, whatever its category says. Only the `status:` lines are read back; other edits are ignored. Then write the decisions into the plan:

```sh
vault-curator review --sync 2026-09-14.review.md 2026-09-14.json
```

## Summary

| Category | Auto | Approve each | Status |
| --- | ---: | ---: | --- |
| Kept notes | 2 | 0 | 2 pending |
| **Total** | 2 | 0 | 2 pending |

## Kept notes

- category `keep` status: pending

Decided by the category status:

- **keep-0915**: Nothing to do. Keeps `daily/2026-09-15.md` as is: no change, and scan stops listing its current blocks.
- **keep-0916**: Keeps `daily/2026-09-16.md` as is: no change, and scan stops listing its current blocks.
