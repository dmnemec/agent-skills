# Planning reference

Judgment calls for step 4 of [SKILL.md](SKILL.md). Design background: [docs/design.md](../../../docs/design.md), "Note model" and "Curation pipeline". A worked example is [testdata/plans/2026-09-14.json](../../../testdata/plans/2026-09-14.json), the plan for [testdata/vault/daily/2026-09-14.md](../../../testdata/vault/daily/2026-09-14.md).

## Split boundaries

A daily note holds several meetings. Read the raw text, not just the block list; a block can hold more than one meeting. Boundaries show as:

- a blank line
- a new top-level bullet, even with no blank line and with a changed marker (`-` to `*`)
- a level-2 or level-3 bullet run that is a separate meeting (a new subject and participants, an aside such as "joined late, turned into its own thing")

Indentation and marker inconsistencies (tabs, spaces, `-` `*` `+`) are noise. Choose the line range from whole bullets; a source covers whole bullets only.

A day with one meeting needs no split. A non-meeting block (a personal reminder) becomes its own segment on `Unsorted`.

Each split yields a new note titled `<date> <topic>` (a path such as `meetings/2026-09-14 Beacon sync.md`) and leaves a link line in the daily note, so the day reads as a timeline. Give the daily note `type: daily` through `set_frontmatter`.

## Restructure inside a note

Repair indentation, group points under `## Notes`, `## Decisions`, `## Constraints and requirements`, `## Open questions` and `## Action items`, and move each decision, constraint, open question or action item into its section out of the body. Reword nothing; add links around existing words (`[[Tidegate|tidegate]]`). Keep Mermaid blocks byte for byte. Leave `- [ ]` and `- [x]` as they are.

## Primary versus mention

A meeting is about one or two projects or components. Name those in frontmatter, as quoted wikilinks: `project: "[[Harbor]]"` for a whole-project meeting, `topics:` for components. Everything else mentioned stays an inline link. When two components are equally central, list both in `topics`. Primary topics are inference: state your reasoning in `summary` so review can confirm it.

## Links and entities

Link a mention only when `scan` shows a note for it (title or alias). A person or system with no note is proposed once, as a `create` with category `entity`, instead of a dangling link. Never link to a note that neither exists nor is created in the same plan.

## Orphans

For each orphan in the scan, propose `add_links` to the best-fit hub and say why in `summary`. When no hub fits, link to `Unsorted` (or the `--fallback-hub` you scanned with).

## Frontmatter

`type`, `date`, `project`/`topics`, `aliases` and the rest are defined in design.md "Note model"; the plan's `set_frontmatter` field rules are in docs/plan.md. Set only fields you can justify from the note.

## Keep

Propose `keep` (docs/plan.md) for a candidate note when it is already well-formed (clean structure, linked, right frontmatter) or nothing in it calls for a change. Give each a one-line `summary` saying which. A note takes either a `keep` or other operations, never both.

## Tiers

Use the lowest tier docs/plan.md allows. Raise a tier to `approve-each` when the change is a judgment call the user should see by itself, such as an uncertain primary topic.
