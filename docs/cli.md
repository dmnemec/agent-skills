# vault-curator CLI

The deterministic half of vault-curator. Every query command takes `--json`; its output shape is documented here and pinned by golden-file tests.

## version

```
vault-curator version
```

Prints the version on one line. The version is the first of these that is set:

1. The value given at build time with `-ldflags "-X main.version=..."`.
2. The module version recorded in the binary, such as `v0.1.0` or a pseudo-version, as set by `go install github.com/dmnemec/agent-skills/cmd/vault-curator@<version>`.
3. `dev`, for a local build with no version (`(devel)`).

## scan

```
vault-curator scan [--vault <path>] [--json] [--fallback-hub <title>] [--all] [--limit <n>]
```

Lists what curation should look at, plus the vault-wide context the agent plans against.

- `--vault` is the vault root (default `.`). It must be inside a git work tree; it may be the repo root or a folder in it.
- `--fallback-hub` is the title of the fallback hub note (default `Unsorted`).
- `--all` ignores the watermark: the candidates are every note with at least one unprocessed block per the state file (a note absent from state has all its blocks unprocessed). Notes whose blocks are all processed are not candidates. The backfill skill uses it: after a first batch is merged and watermarked, the remaining old notes are unchanged since the tag, so a plain scan never lists them.
- `--limit N` keeps at most N candidates, taken in path order so successive batches progress deterministically; `remaining` in the JSON counts the candidates left out. It works with or without `--all`. Zero (the default) means no cap; a negative value is bad usage.
- Without `--json`, prints a short summary.

Exit codes: 0 on success, 1 when the scan fails (for example, the vault is not in a git repo), 2 on bad usage.

### How candidates are chosen

1. **Watermark.** The git tag `vault-curator/watermark` marks the last curation commit. Only curation moves it; commits made outside curation (by you, an editor plugin or a scheduled sync) never do.
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
  "remaining": 0,
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
| `remaining` | Candidates left out by `--limit`; 0 when nothing was cut. With `--all --limit`, 0 means this batch is the last. |
| `candidates[].path`, `.title` | The note's path, and its file name without `.md` (what wikilinks resolve against). |
| `candidates[].type` | Frontmatter `type`, lowercased. Optional. |
| `candidates[].tracked` | True when curation has processed this note before (it has a state entry). |
| `candidates[].processed_blocks` | How many of the note's blocks were already processed and so left out of `new_blocks`. |
| `candidates[].new_blocks[]` | Unprocessed top-level blocks, in file order. Empty means nothing new. Same fields as `blocks[]`. |
| `candidates[].blocks[]` | Every top-level block of the note, processed or not, in file order. A `rewrite` maps them all. |
| `blocks[].start_line`, `.end_line` | 1-based, inclusive line range in the note. |
| `blocks[].hash` | The block's content hash: 16 hex characters, stable across reindenting, marker and case changes, and across ticking or unticking a checkbox (`[ ]`, `[x]`, `[X]`), so closing an action item does not make its block new. Plans refer to blocks by it. |
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

## entities

```
vault-curator entities [--vault <path>] [--json] [--min-count <n>] [--fallback-hub <title>]
vault-curator entities [--vault <path>] --links-to <note> [--json]
```

Lists the entities the vault has and the **candidate** entities it only mentions. It reads the files and needs no git repo; `--vault` defaults to `.`. Without `--json`, prints a short listing. The `entities` skill drives it.

Exit codes: 0 on success, 1 on failure (an unreadable vault, an unknown `--links-to` note), 2 on bad usage.

### Candidate heuristic

A candidate is a name or term the notes mention at least `--min-count` times (default 2) in plain text without a link, that no note title or alias names.

1. Prose only. Fenced code, inline code, wikilinks, markdown links, bare URLs, headings and generated blocks are ignored. Frontmatter values count (an `attendees:` list of plain names).
2. A name is a run of capitalized words joined by single spaces: `Teodor Quill`, `Sana`. Weekdays, months, articles, pronouns, conjunctions and `AI` end a run.
3. A single word seen only at the start of a sentence or bullet (`Add retries`) is not evidence. It needs one capitalized mention that does not open a sentence or bullet.
4. A single word that belongs to exactly one multi-word name folds into it: `Teodor` joins `Teodor Quill` and is listed under `variants`.
5. A name that resolves to a note title or alias (`Mira`, the alias of `Mira Vantongeren`) is not a candidate.
6. Mentions are then recounted case-insensitively on whole words, so `oskar` counts for `Oskar Fenwick`.

The heuristic finds names, not their kind. Whether a candidate is a person, project, component or system is the agent's call, and the user's at review. It misses names that only ever open a sentence, and lists capitalized terms that are not entities; review catches both.

### JSON output

```json
{
  "entities": [
    { "path": "people/Mira Vantongeren.md", "title": "Mira Vantongeren", "type": "person", "aliases": ["Mira"], "hub": false, "backlinks": 9 }
  ],
  "candidates": [
    {
      "name": "Teodor Quill",
      "variants": ["Teodor"],
      "count": 8,
      "occurrences": [
        { "path": "daily/2026-09-14.md", "line": 2 }
      ]
    }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `entities[]` | The same entities as in `scan`, sorted by path, with `backlinks`: the number of links into the note. |
| `candidates[]` | Sorted by `count`, most first, then name. |
| `candidates[].name` | The longest form seen, as first written. |
| `candidates[].variants` | Shorter forms folded into `name`. Optional. |
| `candidates[].count` | The number of mentions; equals the length of `occurrences`. |
| `candidates[].occurrences[]` | Each mention: vault-relative `path` and 1-based `line`, in path then line order. |

### Links to a note

`--links-to <note>` (a title, alias or path) answers a different question, for merging entities: where do links point at this note? `--min-count` is ignored.

```json
{
  "target": { "path": "people/Mira Vantongeren.md", "title": "Mira Vantongeren" },
  "notes": [
    {
      "path": "components/Tidegate.md",
      "links": [ { "line": 12, "raw": "[[Mira Vantongeren]]", "in_frontmatter": true } ],
      "blocks": [ { "start_line": 17, "end_line": 20, "hash": "<hash>" } ]
    }
  ]
}
```

`blocks` lists every top-level block of the linking note, which a `rewrite` of that note must map as sources (see [plan.md](plan.md)). `scan` lists blocks only for notes changed since the watermark, so a linking note that has not changed is found here.

## review

```
vault-curator review --vault <path> [--out <review.md>] [--force] <plan.json>
vault-curator review --sync <review.md> <plan.json>
```

Turns a plan (see [plan.md](plan.md)) into a markdown review document the user edits in Obsidian or on GitHub, then reads the decisions back into the plan.

- **Render** (`--vault`) writes the review next to the plan (`plan.json` → `plan.review.md`), or to `--out` (`-` for stdout). The plan must pass `validate --draft`. An existing review file is not overwritten without `--force`, since it may hold edits.
- **Sync** (`--sync`) sets each operation's `status` in the plan from the review and rewrites the plan file. It needs no vault.

Exit codes: 0 on success, 1 on failure (a plan that fails `validate --draft`, a review that cannot be synced), 2 on bad usage.

### The review document

1. A summary: operations per category and tier, their statuses, and the notes the plan changes.
2. One section per category, in the order splits, new entities, new notes, restructures, frontmatter, links, kept notes. Each starts with the category's status line. Splits show every bullet of the split note, by line, under the note it goes to (the lossless mapping), then the note as it reads after the plan. Every split, create and rewrite has its own section with the resulting content; a rewrite shows the note before and after. Auto `set_frontmatter`, `add_links` and `keep` operations take one line each.

Note content appears only in fenced code blocks, so links in a review saved inside the vault add nothing to its graph. The review of the example plan (with every status pending) is pinned in [`internal/vaultcurator/review/testdata/2026-09-14.review.golden.md`](../internal/vaultcurator/review/testdata/2026-09-14.review.golden.md).

### Status lines

Sync reads only these lines, outside fenced code; every other edit is ignored:

```markdown
- category `split` status: approved
- op `split-beacon` status: rejected
```

- Each category has one line. Each `approve-each` operation has one, and so does an `auto` operation whose status differs from its category's.
- A status is `pending`, `approved` or `rejected` (any case).
- An `auto` operation takes its own status when that is `approved` or `rejected`, and its category's status otherwise. So approve a category, then reject the exceptions.
- An `approve-each` operation (a split, a new entity, a new note, a restructure) never takes its category's status. Each needs its own `approved` or `rejected`; the category line alone decides nothing for it.

Sync fails and leaves the plan unchanged when the review names an operation or category the plan does not have, repeats or garbles a status line, lacks a category's or an `approve-each` operation's line, holds a status it cannot read, or leaves an `approve-each` operation `pending`. The last error reads:

```
approve-each operations need their own approved or rejected, a category status does not decide them; still pending: split-beacon, entity-teodor
```

The first line of the review is a hidden comment holding the plan's hash:

```markdown
<!-- vault-curator review plan-sha256=<64 hex characters> -->
```

It is the SHA-256 of the plan as `plan.Marshal` writes it with every status cleared, so syncing, which changes only statuses, keeps it valid. Any other change to the plan since the review was rendered, or a missing comment, fails the sync: render the review again.

## apply

```
vault-curator apply --vault <path> [--branch <name>] [--worktree-root <folder>] [--json] <plan.json>
```

Carries out a reviewed plan without touching the vault's working tree, so nothing changes in Obsidian or on your branch until you merge.

1. **Validates** the plan against the working tree, exactly as `validate` does (without `--draft`); that is what scan read, uncommitted edits included. Any error stops apply before anything is committed or created, and the errors are printed as `validate` prints them. A plan with no approved operation that writes or keeps a note is refused too.
2. **Snapshots uncommitted changes.** The worktree starts from `HEAD`, so if anything under the vault folder is modified, staged or untracked, apply first commits it on the current branch as `curate: snapshot before apply`, with the vault's git hooks running. Only the vault folder is staged (`git add --all -- <vault>`); changes elsewhere in a larger repo are left as they are. If changes outside the vault folder are already staged, apply refuses rather than sweep them into the snapshot. A failed snapshot commit puts the index back as it was. A clean vault gets no snapshot.
3. **Creates a worktree** on a new branch starting at `HEAD` (the snapshot, if any). The branch is `--branch`, default `curate/<UTC time>` such as `curate/20261018-142501`; an existing branch is an error. The worktree is `<root>/vault-curator-<random>/<branch with / as ->`, where `<root>` is `--worktree-root` (default the OS temp folder). It must be outside the repo, so it is never inside the vault.
4. **Writes** each changed note's post-plan content, after checking every note still matches what was validated. Each written note's blocks are recorded as processed in `.vault-curator/state.json` (see [State file](#state-file)), and so is every block of each note an approved `keep` names, which changes no content. A plan of only `keep` operations writes just the state file.
5. **Commits** everything in the worktree as `curate: apply plan to N notes` (`curate: keep N notes as is` when nothing is written), with one line per note in the body. The vault's git hooks run; a failing hook fails apply.

Any failure after step 2 removes the worktree, its folder and the branch, so a failed apply leaves nothing but the snapshot commit, which holds only the user's own edits.

Without `--json`, prints (the first line only when a snapshot was made):

```
committed uncommitted vault changes first: snapshot 9b1c04e7a2d3
applied 7 notes on branch curate/20261018-142501
worktree: /tmp/vault-curator-1234/curate-20261018-142501
  update Coffee machine options.md (links-coffee)
  create meetings/2026-09-14 Beacon sync.md (split-beacon)
  keep daily/2026-09-15.md
merge curate/20261018-142501 into your branch, then run: vault-curator watermark --vault <path>
```

With `--json`:

```json
{
  "ok": true,
  "branch": "curate/20261018-142501",
  "worktree": "/tmp/vault-curator-1234/curate-20261018-142501",
  "vault": "/tmp/vault-curator-1234/curate-20261018-142501",
  "snapshot": "<snapshot commit>",
  "base": "<commit the branch starts from>",
  "commit": "<curation commit>",
  "changes": [
    { "path": "meetings/2026-09-14 Beacon sync.md", "created": true, "ops": ["split-beacon"] }
  ],
  "kept": ["daily/2026-09-15.md"]
}
```

| Field | Meaning |
| --- | --- |
| `branch` | The curation branch: one commit on top of `base`. |
| `worktree` | Absolute path of the worktree. |
| `vault` | The vault folder inside the worktree: `worktree` itself, or its subfolder when the vault is a folder in its repo. |
| `snapshot` | The commit of the vault's uncommitted changes on the current branch. Optional: omitted when the vault was clean. |
| `base`, `commit` | The vault's `HEAD` the branch starts from (after the snapshot), and the curation commit. |
| `changes[]` | The notes written, as in `validate --json`. |
| `kept[]` | The notes kept as is: their blocks are marked processed, their content is unchanged. |

When the plan is invalid, `--json` prints `{"ok": false, "errors": [...]}` (errors as in `validate --json`), and the errors also go to stderr.

Exit codes: 0 on success, 1 on any failure (invalid plan, changes staged outside the vault folder, a failing hook, git errors), 2 on bad usage.

## watermark

```
vault-curator watermark [--vault <path>] [--branch <name>] [--json]
```

Finishes a curation once the user has merged its branch into the vault's branch, by a merge commit or a fast-forward. Run it from the vault's main working tree; `--vault` defaults to `.`.

1. Picks the curation branch: `--branch`, or else the only `curate/*` branch that is an ancestor of `HEAD`. If none is merged it refuses and names the unmerged ones; if several are, it asks for `--branch`.
2. Refuses, changing nothing, when the branch is not an ancestor of `HEAD` (a squash or rebase merge is not detected), when it is the checked-out branch (watermark was run inside the worktree), or when its worktree has uncommitted changes.
3. Moves the `vault-curator/watermark` tag to the curation commit (the branch tip), removes the worktree and its temp folder, and deletes the branch. A worktree already deleted by hand is pruned.

The tag goes on the curation commit rather than the merge commit. The curated notes are identical in both, so a fresh scan does not list them; but notes committed outside curation on the main branch between apply and merge still differ from the curation commit, so they stay scan candidates instead of being skipped.

Without `--json`, prints:

```
moved vault-curator/watermark to 3f2a9c81d0e4 (curate/20261018-142501)
removed worktree /tmp/vault-curator-1234/curate-20261018-142501
deleted branch curate/20261018-142501
```

With `--json`:

```json
{
  "tag": "vault-curator/watermark",
  "commit": "<curation commit>",
  "branch": "curate/20261018-142501",
  "worktree": "/tmp/vault-curator-1234/curate-20261018-142501"
}
```

`worktree` is omitted when the worktree was already gone.

Exit codes: 0 on success, 1 on failure (including an unmerged branch), 2 on bad usage.
