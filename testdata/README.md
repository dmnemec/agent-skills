# Synthetic test vault

`vault/` is the test fixture and demo for vault-curator. Everything in it is invented: the Larkspur team, its people, the Harbor project and its components. No real names, companies or data.

The fixture is plain files, not a git repo. Tests that need git copy `vault/` into a temp dir and `git init` there. `internal/vaultcurator/fixture` checks that the fixture and `expected/` stay consistent.

## The fictional team

- **Harbor** (`projects/Harbor.md`): an event ingest pipeline.
- **Tidegate** (ingest gateway), **Ledgerline** (event store), **Beacon** (shared notification service): the component hubs in `components/`.
- People: Mira Vantongeren (tech lead, the only person note), Teodor Quill, Sana Okafor-Lind, Oskar Fenwick, Lio Haverkamp. The last four have no notes, so a curate run should propose them as new entities rather than link them.

## What each file exercises

| Case | File |
| --- | --- |
| Messy daily: several meetings in one file, blank-line separators | `daily/2026-09-14.md`, `daily/2026-09-15.md`, `daily/2026-09-16.md` |
| Messy daily: boundary with no blank line, only a new top-level bullet and marker | `daily/2026-09-14.md` (lines 4–5) |
| Messy daily: inconsistent top-level indentation (whole meeting indented 3 spaces) | `daily/2026-09-15.md` (lines 11–17) |
| Messy daily: level-2 bullet run that is really a separate meeting | `daily/2026-09-14.md` (lines 12–16) |
| Messy daily: level-3 bullet run that is really a separate meeting | `daily/2026-09-16.md` (lines 6–9) |
| Messy daily: mixed tabs and spaces, including within one indent | all messy dailies; `daily/2026-09-15.md` line 14 is tab plus two spaces |
| Messy daily: mixed bullet markers `-`, `*`, `+` | `daily/2026-09-14.md`, `daily/2026-09-15.md`, `daily/2026-09-16.md` |
| Messy daily: non-meeting block that belongs on the fallback hub | `daily/2026-09-16.md` (lines 11–12) |
| Messy daily: single meeting, correct answer is no split | `daily/2026-09-18.md` |
| Already-curated daily (timeline of links) | `daily/2026-09-08.md` |
| Curated meeting notes: frontmatter (`type`, `date`, `project`, `topics`) and the extracted sections | `meetings/2026-09-08 Tidegate rate limits.md`, `meetings/2026-09-10 Harbor kickoff.md` |
| Open and done action items with inline `owner::` / `source::` fields | both meeting notes |
| Project hub with `aliases`, `repos` and `links` | `projects/Harbor.md` |
| Component hubs with typed edges: `part-of`, `calls`, `owned-by` | `components/Tidegate.md` |
| Typed edges: `part-of`, `owned-by` | `components/Ledgerline.md` |
| Typed edges: `depends-on`, `related-to` | `components/Beacon.md` |
| Person note | `people/Mira Vantongeren.md` |
| `Unsorted` fallback hub | `Unsorted.md` |
| Orphan note, links to no hub | `Coffee machine options.md` |
| Broken wikilink | `daily/2026-09-15.md` line 6, `[[Tidegate v2 RFC]]` |
| Wikilink with alias `[[note\|alias]]` | `daily/2026-09-16.md` line 4, `meetings/2026-09-08 Tidegate rate limits.md` |
| Wikilink with heading `[[note#heading]]` | `daily/2026-09-15.md` line 15, `components/Tidegate.md`, `archive/2025/q4/Ledgerline schema v2 notes.md` |
| Wikilink with heading and alias `[[note#heading\|alias]]` | `daily/2026-09-16.md` line 9 |
| User-drawn Mermaid block, preserved byte for byte | `meetings/2026-09-10 Harbor kickoff.md` (irregular spacing and a `%%` comment on purpose) |
| Note in a nested folder (folders carry no meaning) | `archive/2025/q4/Ledgerline schema v2 notes.md` |

## Expectations

`expected/<date>.json` holds the evaluation set for split accuracy and primary-topic inference, one file per messy daily note:

```json
{
  "note": "daily/2026-09-14.md",
  "date": "2026-09-14",
  "segments": [
    {
      "lines": [1, 4],
      "kind": "meeting",
      "title": "Harbor standup",
      "primary": ["Harbor"],
      "cue": "why the boundary is here"
    }
  ]
}
```

- `lines` is a 1-based, inclusive line range. Segments are in order and together cover every non-blank line of the note exactly once. Blank lines between segments belong to none.
- `kind` is `meeting` or `other` (not a meeting, such as personal reminders).
- `title` is the topic part of the split note's title; the split note is named `<date> <title>`.
- `primary` lists the one or two notes the segment is about, by note name. These drive hub timelines. Everything else mentioned is a link only.
- `cue` explains the boundary for a human reader. Tests ignore it.
