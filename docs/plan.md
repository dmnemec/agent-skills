# Curation plan

The plan is the only thing that crosses from the agent to the CLI (see "Architecture" in [design.md](design.md)). The agent reads `vault-curator scan --json` and writes a plan: a JSON file listing every change it proposes. The user approves it in review, and `vault-curator apply` carries out the approved operations. Nothing else edits the vault.

Lifecycle:

1. The agent writes the plan with every `status` set to `pending`.
2. The agent runs `vault-curator validate --draft --vault <vault> <plan.json>` and fixes the plan until it passes. `--draft` treats pending operations as approved.
3. `vault-curator review` shows the plan, and the user sets each status to `approved` or `rejected`.
4. `vault-curator validate --vault <vault> <plan.json>` (and `apply`, which runs the same checks) passes only when no operation is still pending.

## Top level

```json
{
  "version": 1,
  "operations": [ ... ]
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `version` | number | Schema version. Must be `1`. |
| `operations` | array | The proposed operations, in the order they apply. |

Unknown fields anywhere in the plan are an error, so a misspelled field fails instead of being ignored.

## Operations

Every operation has these fields:

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | string | Unique within the plan. Errors and review refer to it. |
| `op` | string | What the operation does: `rewrite`, `split`, `create`, `set_frontmatter`, `add_links` or `keep`. |
| `category` | string | Review group, approved together. Must fit `op` (table below). |
| `tier` | string | `auto` or `approve-each` (design.md, "Autonomy tiers"). It may be raised from `auto` to `approve-each`, never lowered. `never` is always rejected. |
| `status` | string | `pending`, `approved` or `rejected`. Only `approved` operations are applied; `rejected` ones are skipped. |
| `summary` | string | Optional. One line for review. |

The other fields depend on `op`. A field an op does not take is an error.

| `op` | Category | Lowest tier | Fields |
| --- | --- | --- | --- |
| `rewrite` | `restructure` | `auto` | `note`, `content`, `sources` |
| `split` | `split` | `approve-each` | `note`, `path`, `content`, `sources`, optional `link` |
| `create` | `entity` (people, projects, components) or `note` | `approve-each` | `path`, `content` |
| `set_frontmatter` | `frontmatter` | `auto` | `note`, `frontmatter` |
| `add_links` | `links` | `auto` | `note`, `links` |
| `keep` | `keep` | `auto` | `note` |

| Field | Type | Meaning |
| --- | --- | --- |
| `note` | string | Vault-relative path of the existing note the operation acts on, as in scan output (`daily/2026-09-14.md`). Matched case-insensitively. `set_frontmatter` and `add_links` may also name a note created earlier in the plan. |
| `path` | string | Vault-relative path of the note a `split` or `create` makes. It must end in `.md`, use `/`, not exist yet, and have a title (file name) no other note or alias uses. Obsidian forbids `* " \ < > : \| ? # ^ [ ]` in file names, so they are rejected. |
| `content` | string | The complete resulting note, frontmatter included. A missing final newline is added. |
| `sources` | array | The source mapping: which original bullets this operation moves into `content` (below). |
| `link` | string | The line a `split` leaves in `note` where the moved bullets were. It must hold only links, one of them to the new note. Default: `- [[<new note title>]]`. |
| `frontmatter` | object | Fields to set. A `null` value removes the field. Existing fields keep their place; new ones follow in the order `type`, `date`, `project`, `topics`, `aliases`, `attendees`, `repos`, `links`, `supersedes`, then alphabetically. Wikilinks are written double-quoted (`"[[Harbor]]"`) and dates plain (`2026-09-14`). |
| `links` | array of strings | Note names (`"Unsorted"`, not `"[[Unsorted]]"`). Each is appended to the end of the note as a `- [[name]]` bullet. |

### What each op does

- **rewrite** replaces a note's whole content with `content`: repaired indentation, topic headings, consistent markers, inline links, frontmatter. Its `sources` must map every bullet of the note. A rewritten note cannot also be split; to restructure a note and split it, split it and put the restructured form in each split's `content`.
- **split** moves the bullets in `sources` out of `note` into a new note at `path`, whose content is `content`. In `note`, the moved lines are cut and `link` is written where the first of them was, so the daily note reads as the day's timeline. Bullets it does not select stay where they are, so a rejected split leaves the note as it was.
- **create** makes a new note that moves no content, such as an entity stub. Its content may hold headings, frontmatter and link-only bullets, but no other bullets, since that would be invented content.
- **set_frontmatter** sets fields in `note`'s frontmatter, adding a frontmatter block if there is none. The frontmatter is re-encoded; the body is untouched.
- **add_links** appends link bullets to `note`, such as filing an orphan under a hub.
- **keep** records "reviewed, keep as is" for `note`. It changes no content. On apply, every block of the note is marked processed in the state file (committed with the curation), so `scan` and `scan --all` stop listing the note until it gets new blocks. Without a `keep`, a note that needs no change, or whose operations were all rejected, is never marked and returns in every later scan. Review lists kept notes last, under "Kept notes". A plan whose only approved operations are `keep` applies. `keep` is an error when the note does not exist, when another operation in the plan touches the note (a rejected operation does not count), or when the note already has a `keep`.

```json
{
  "id": "keep-coffee",
  "op": "keep",
  "category": "keep",
  "tier": "auto",
  "status": "pending",
  "summary": "Already linked and structured; nothing to change",
  "note": "daily/2026-09-15.md"
}
```

### Merging entities

There is no merge operation: alias entity B into A with a `set_frontmatter` on A (`"aliases"`: the full list, B's name added), then redirect every note that links to B with a `rewrite` that changes `[[B]]` to `[[A|B]]` (the lossless check reads a link as its shown text, so this passes). Links in frontmatter change inside the rewrite's `content`. B's own note stays, since a plan deletes nothing; once the user has moved its content into A and deleted it by hand, links to B resolve through the alias. [`vault-curator entities --links-to B`](cli.md#links-to-a-note) lists the linking notes and the blocks a rewrite must map. A test pins this in `internal/vaultcurator/plan`.

### Sources

A source names a run of original bullets by the block they are in:

```json
{"note": "daily/2026-09-14.md", "block": "94300d2390c6855f", "lines": [5, 11]}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `note` | string | The note the bullets come from. It must be the operation's `note`. |
| `block` | string | The block's `hash` from scan output. If the note changed since the scan and the block is gone, validation fails and asks for a new scan. |
| `lines` | `[first, last]` | 1-based inclusive line range inside the block. `first` must be a bullet's line, and the range must not end inside a bullet (a bullet spans its continuation lines). Every bullet whose line falls in the range is selected, at any depth. Use the block's own `start_line` and `end_line` to take the whole block. |

A block can be split between operations by giving each a different range, as `split-retry` and `split-beacon` do below.

## Post-plan content

`validate` computes the content every touched note will have, and `apply` writes exactly that. For each note:

1. The note's content: `content` of the `split` or `create` that makes it, `content` of its `rewrite`, or its current content with every approved split's lines cut out and replaced by that split's `link`.
2. Each `set_frontmatter` on the note, in plan order.
3. Each `add_links` on the note, in plan order.

Notes whose content ends up unchanged are not written. Kept notes are not written either: `validate` returns them separately (`Result.Kept`, `kept` in `--json`), and `apply` marks their blocks processed.

## Checks

`validate` reports every failure, each naming the operation and, where there is one, the note and line:

```
op split-retry: daily/2026-09-14.md:7: bullet not found in meetings/2026-09-14 Tidegate retry design.md (dropped or reworded): "- proposal: exp backoff, cap at 5, jitter"
```

- **Shape.** Known `op`, a `category` that fits it, a `tier` at or above its category's lowest tier, a valid `status`, the fields the op needs and no others, valid new paths, unique ids.
- **Approval.** A `pending` operation fails (unless `--draft`). A `never` tier fails.
- **Sources.** Each source's block exists in the note and its range covers whole bullets. No bullet is mapped by two operations.
- **Lossless check.** Every selected source bullet appears in its operation's `content`, matched on normalized text: indentation, list markers, whitespace (including line breaks) and case are ignored, and a link counts as the text it shows, so linking a word (`tidegate` to `[[Tidegate|tidegate]]`) keeps the match. Each content bullet matches at most one source bullet. A `rewrite` must also map every bullet of its note and keep every user-drawn Mermaid block byte for byte. Anything dropped or reworded fails.
- **Never tier.** A content bullet that matches no source bullet and is not only links is invented content. A source `- [ ]` item that comes out as `- [x]` closes an action item. Both fail.
- **Links.** No link in a touched note may resolve to nothing once the plan is applied, counting the notes the plan creates. Broken links the note already had before the plan are not counted against it.

Headings, paragraphs and other non-bullet text are not part of the lossless check, except Mermaid blocks in a rewritten note.

## Complete example

This plan splits the messy daily note `testdata/vault/daily/2026-09-14.md` into its four meetings (per `testdata/expected/2026-09-14.json`), creates a person note for Teodor, marks the daily note as curated, and files the orphan coffee note under `Unsorted`. It is also the test fixture `testdata/plans/2026-09-14.json`, kept identical by a test.

```json
{
  "version": 1,
  "operations": [
    {
      "id": "split-standup",
      "op": "split",
      "category": "split",
      "tier": "approve-each",
      "status": "approved",
      "summary": "Split the Harbor standup out of the 2026-09-14 daily note",
      "note": "daily/2026-09-14.md",
      "path": "meetings/2026-09-14 Harbor standup.md",
      "sources": [
        {"note": "daily/2026-09-14.md", "block": "59f9f532108a37a1", "lines": [1, 4]}
      ],
      "content": "---\ntype: meeting\ndate: 2026-09-14\nproject: \"[[Harbor]]\"\n---\n# 2026-09-14 Harbor standup\n\n- standup\n  - [[Teodor Quill|Teodor]]: [[Ledgerline|ledgerline]] compaction job still flaky, reran it twice over the weekend\n  - Sana wants the [[Harbor|harbor]] beta date firmed up by fri\n  - Oskar on call, paged 3x for [[Tidegate|tidegate]] 502s\n"
    },
    {
      "id": "split-retry",
      "op": "split",
      "category": "split",
      "tier": "approve-each",
      "status": "approved",
      "summary": "Split the Tidegate retry design meeting out of the daily note",
      "note": "daily/2026-09-14.md",
      "path": "meetings/2026-09-14 Tidegate retry design.md",
      "sources": [
        {"note": "daily/2026-09-14.md", "block": "94300d2390c6855f", "lines": [5, 11]}
      ],
      "content": "---\ntype: meeting\ndate: 2026-09-14\ntopics:\n  - \"[[Tidegate]]\"\n---\n# 2026-09-14 Tidegate retry design\n\n## Notes\n\n- [[Tidegate|tidegate]] retry design w/ [[Mira]], Oskar\n- current retries are unbounded, that's what hammered [[Ledgerline|ledgerline]] sat night\n- proposal: exp backoff, cap at 5, jitter\n  - Mira: cap should be configurable per tenant\n\n## Decisions\n\n- decided: cap 5 by default, per-tenant override later\n\n## Open questions\n\n- open q: dead-letter after the cap or just drop?\n\n## Action items\n\n- AI Oskar: add retry metrics to the dashboard\n"
    },
    {
      "id": "split-beacon",
      "op": "split",
      "category": "split",
      "tier": "approve-each",
      "status": "approved",
      "summary": "Split the Beacon sync, nested under the retry meeting, into its own note",
      "note": "daily/2026-09-14.md",
      "path": "meetings/2026-09-14 Beacon sync.md",
      "sources": [
        {"note": "daily/2026-09-14.md", "block": "94300d2390c6855f", "lines": [12, 16]}
      ],
      "content": "---\ntype: meeting\ndate: 2026-09-14\ntopics:\n  - \"[[Beacon]]\"\n  - \"[[Tidegate]]\"\n---\n# 2026-09-14 Beacon sync\n\n## Notes\n\n- [[Beacon|beacon]] sync (Lio joined late, turned into its own thing)\n- beacon needs a webhook when [[Tidegate|tidegate]] dead-letters\n- Lio: beacon rate limit is 50/s, tidegate bursts way past that\n\n## Decisions\n\n- agreed tidegate batches notifications before calling beacon\n\n## Action items\n\n- AI Lio: write up the beacon webhook contract\n"
    },
    {
      "id": "split-teodor",
      "op": "split",
      "category": "split",
      "tier": "approve-each",
      "status": "approved",
      "summary": "Split the 1:1 with Teodor out of the daily note",
      "note": "daily/2026-09-14.md",
      "path": "meetings/2026-09-14 1-1 Teodor.md",
      "link": "- [[2026-09-14 1-1 Teodor|1:1 Teodor]]",
      "sources": [
        {"note": "daily/2026-09-14.md", "block": "bfd33e0247b934ee", "lines": [18, 21]}
      ],
      "content": "---\ntype: meeting\ndate: 2026-09-14\ntopics:\n  - \"[[Ledgerline]]\"\nattendees:\n  - \"[[Teodor Quill]]\"\n---\n# 2026-09-14 1:1 Teodor\n\n- 1:1 [[Teodor Quill|teodor]]\n  - wants to own the [[Ledgerline|ledgerline]] on-call rotation\n  - compaction: suspects the index rebuild, not storage\n  - AI me: ask Sana about headcount for ledgerline\n"
    },
    {
      "id": "entity-teodor",
      "op": "create",
      "category": "entity",
      "tier": "approve-each",
      "status": "approved",
      "summary": "New person: Teodor Quill (in the standup and a 1:1)",
      "path": "people/Teodor Quill.md",
      "content": "---\ntype: person\n---\n# Teodor Quill\n"
    },
    {
      "id": "fm-daily",
      "op": "set_frontmatter",
      "category": "frontmatter",
      "tier": "auto",
      "status": "approved",
      "summary": "Mark the 2026-09-14 daily note as curated",
      "note": "daily/2026-09-14.md",
      "frontmatter": {"type": "daily", "date": "2026-09-14"}
    },
    {
      "id": "links-coffee",
      "op": "add_links",
      "category": "links",
      "tier": "auto",
      "status": "approved",
      "summary": "File the orphan coffee machine note under the fallback hub",
      "note": "Coffee machine options.md",
      "links": ["Unsorted"]
    }
  ]
}
```

Things to notice:

- The retry meeting's block (lines 5–16) also holds the Beacon sync, a level-2 run that is really its own meeting, so two splits take different ranges of the same block.
- The new notes regroup bullets under `## Decisions`, `## Open questions` and `## Action items` and add links, but keep every bullet's words, so the lossless check passes.
- `[[Teodor Quill|Teodor]]` resolves only because `entity-teodor` creates the note. Rejecting `entity-teodor` makes validation fail with a broken link.
- The 1:1 is titled `1-1 Teodor`, because `:` cannot appear in a file name; its `link` shows `1:1 Teodor`.

After the plan, `daily/2026-09-14.md` is:

```markdown
---
type: daily
date: 2026-09-14
---
- [[2026-09-14 Harbor standup]]
- [[2026-09-14 Tidegate retry design]]
- [[2026-09-14 Beacon sync]]

- [[2026-09-14 1-1 Teodor|1:1 Teodor]]
```

### Rewrite

The example has no `rewrite`. One has this shape (placeholders in angle brackets), with one source per block of the note so that every bullet is mapped:

```
{
  "id": "restructure-standup",
  "op": "rewrite",
  "category": "restructure",
  "tier": "auto",
  "status": "pending",
  "summary": "Repair indentation, group points under headings, link components",
  "note": "meetings/2026-09-20 Harbor standup.md",
  "sources": [
    {"note": "meetings/2026-09-20 Harbor standup.md", "block": "<hash>", "lines": [<start_line>, <end_line>]},
    {"note": "meetings/2026-09-20 Harbor standup.md", "block": "<hash>", "lines": [<start_line>, <end_line>]}
  ],
  "content": "<the whole restructured note>"
}
```
