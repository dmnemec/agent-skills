# vault-curator CLI

The deterministic half of vault-curator. Every query command takes `--json`; its output shape is documented here and pinned by golden-file tests.

## scan

```
vault-curator scan [--vault <path>] [--json] [--fallback-hub <title>]
```

Lists what curation should look at, plus the vault-wide context the agent plans against.

- `--vault` is the vault root (default `.`). It must be inside a git work tree; it may be the repo root or a folder in it.
- `--fallback-hub` is the title of the fallback hub note (default `Unsorted`).
- Without `--json`, prints a short summary.

Exit codes: 0 on success, 1 when the scan fails (for example, the vault is not in a git repo), 2 on bad usage.

### How candidates are chosen

1. **Watermark.** The git tag `vault-curator/watermark` marks the last curation commit. Only curation moves it; the nightly cron commit never does.
2. **Changed notes.** With the tag, a note is a candidate when it differs from the tagged commit: committed since, modified in the work tree, staged, or new and untracked (but not ignored). Deleted notes are not listed. Without the tag, every note is a candidate.
3. **New blocks.** Inside a candidate, every top-level bullet block is listed in `blocks`, each marked `processed` or not, and the ones not yet processed are also listed in `new_blocks`. A `rewrite` must map every block of its note (see [plan.md](plan.md)); a split usually takes only new ones. Processed block hashes live in `.vault-curator/state.json`, committed in the vault. A block whose content changes gets a new hash and is listed again; an identical copy of a processed block is listed once per extra copy.

A candidate can have no new blocks, when the note changed only outside its bullet blocks (frontmatter, headings, prose) or has no blocks. It is still listed so the agent can decide.

Only markdown notes count. Attachments, `.vault-curator/` and `.git/` are never candidates.

### JSON output

```json
{
  "watermark": {
    "tag": "vault-curator/watermark",
    "exists": true,
    "commit": "<full commit hash>"
  },
  "candidates": [
    {
      "path": "daily/2026-09-18.md",
      "title": "2026-09-18",
      "tracked": true,
      "processed_blocks": 1,
      "new_blocks": [
        {
          "start_line": 6,
          "end_line": 7,
          "hash": "bbe13e85ccc8bf1e",
          "text": "- coffee w/ Lio\n    - beacon webhook contract draft is up, see [[Beacon]]\n",
          "processed": false
        }
      ],
      "blocks": [
        {
          "start_line": 1,
          "end_line": 4,
          "hash": "993ba18102dc89cb",
          "text": "- harbor demo prep w/ Sana\n    - demo flow: ingest -> ledgerline -> ops dashboard\n    - Sana presents, Mira drives\n    - AI Mira: seed demo tenant data\n",
          "processed": true
        },
        {
          "start_line": 6,
          "end_line": 7,
          "hash": "bbe13e85ccc8bf1e",
          "text": "- coffee w/ Lio\n    - beacon webhook contract draft is up, see [[Beacon]]\n",
          "processed": false
        }
      ]
    }
  ],
  "notes": [
    { "path": "components/Beacon.md", "title": "Beacon", "type": "component", "aliases": ["beacon service"] }
  ],
  "entities": [
    { "path": "components/Beacon.md", "title": "Beacon", "type": "component", "aliases": ["beacon service"], "hub": true }
  ],
  "links": [
    { "from": "components/Tidegate.md", "to": "components/Beacon.md", "target": "Beacon", "raw": "[[Beacon]]", "line": 10, "in_frontmatter": true }
  ],
  "broken_links": [
    { "from": "daily/2026-09-15.md", "target": "Tidegate v2 RFC", "raw": "[[Tidegate v2 RFC]]", "line": 6 }
  ],
  "orphans": ["Coffee machine options.md"]
}
```

All paths are vault-relative with forward slashes. Arrays are always present (empty rather than `null`); fields marked optional below are omitted when empty or false. The full output for the synthetic vault is in [`cmd/vault-curator/testdata/scan.golden.json`](../cmd/vault-curator/testdata/scan.golden.json).

| Field | Meaning |
| --- | --- |
| `watermark.tag` | The watermark tag name. |
| `watermark.exists` | False before the first curation: every note is then a candidate. |
| `watermark.commit` | The tagged commit. Optional: omitted when the tag does not exist. |
| `candidates[]` | Notes to curate, sorted by path. |
| `candidates[].path`, `.title` | The note's path, and its file name without `.md` (what wikilinks resolve against). |
| `candidates[].type` | Frontmatter `type`, lowercased. Optional. |
| `candidates[].tracked` | True when curation has processed this note before (it has a state entry). |
| `candidates[].processed_blocks` | How many of the note's blocks were already processed and so left out of `new_blocks`. |
| `candidates[].new_blocks[]` | Unprocessed top-level blocks, in file order. Empty means nothing new. Same fields as `blocks[]`. |
| `candidates[].blocks[]` | Every top-level block of the note, processed or not, in file order. A `rewrite` maps them all. |
| `blocks[].start_line`, `.end_line` | 1-based, inclusive line range in the note. |
| `blocks[].hash` | The block's content hash: 16 hex characters, stable across reindenting, marker and case changes. Plans refer to blocks by it. |
| `blocks[].heading` | Text of the nearest heading above the block. Optional. |
| `blocks[].text` | The block's raw source, including its final newline. |
| `blocks[].processed` | True when an earlier curation already processed the block. Always false in `new_blocks[]`. |
| `notes[]` | Every note: `path`, `title`, optional `type` and `aliases`. Sorted by path. |
| `entities[]` | Project, component and person notes and the fallback hub: `path`, `title`, `type`, optional `aliases`, and `hub` (projects, components and the fallback hub). Sorted by path. |
| `links[]` | The link graph: every link from one note to another, in note then line order. `to` is the resolved note's path, `target` the target as written (no heading or label), `raw` the link exactly as written. `in_frontmatter` (optional) marks links in frontmatter, such as typed edges. Links to attachments and external URLs are left out. |
| `broken_links[]` | Links that resolve to no note or attachment. Same fields as `links[]`, without `to`. |
| `orphans[]` | Paths of non-hub notes that reach no hub by following their links. |

### State file

`.vault-curator/state.json` records the processed blocks per note. `apply` updates it and commits it with the curation; scan only reads it.

```json
{
  "version": 1,
  "files": {
    "daily/2026-09-18.md": ["c5a1f0e2d4b6a8c0"]
  }
}
```

Hashes are listed in block order, once per processed copy. A missing file means nothing has been processed yet.
