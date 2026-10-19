---
name: entities
description: Seed, create, merge and alias entities (people, projects, components, systems) in the user's markdown (Obsidian) vault with vault-curator. Use when the user asks to seed or backfill entities, add a person, project, component or system note, merge duplicate entities, add an alias, or says a name should be a note.
---

# Manage entities

An entity is a note for a person, project, component or system; other notes link to it. You propose, the user approves, the CLI applies, exactly as in [curate](../curate/SKILL.md). Edit no vault file yourself; the only file you write is the plan.

Steps 1 and 2 of curate (locate the vault, check the binary) come first; call the vault `<vault>`. Entity changes go in one batch: every seed, alias and merge in a single plan, so the user decides once.

## 1. List

Run `vault-curator entities --vault <vault> --json`. Output shape and the candidate heuristic: [docs/cli.md](../../../docs/cli.md) under "entities".

Done when you can name every existing entity with its aliases, and every candidate with its count.

## 2. Decide each candidate

Read the occurrences (`path` and `line`) in the notes, then give each candidate exactly one verdict:

- **seed**: a new note. Pick its kind: `person`, `project` or `component` (a system is a `component`).
- **alias**: another name for an existing entity, such as `SMP` for the SMP project. A variant spelling of a name that has a note belongs here.
- **merge**: two notes name one thing (step 4).
- **skip**: a capitalized term that is not an entity.

When the kind or the identity is unclear, ask the user rather than guess. Skip a name seen in one note only.

Done when every candidate has a verdict and a one-line reason.

## 3. Write the plan

Write `<vault>/.vault-curator/plans/<UTC YYYYMMDD-HHMMSS>.json` with every `status` `pending`. Format: [docs/plan.md](../../../docs/plan.md).

- **Seed**: a `create` operation, category `entity`, tier `approve-each`, at `people/<Name>.md`, `projects/<Name>.md` or `components/<Name>.md`. Content is frontmatter (`type`, plus `aliases` for short forms such as `Teodor`), the heading, and nothing invented: no bullets, no role, no facts. Take the name from the longest form the notes use. When the occurrences show which project or component the name belongs to, add a link-only bullet to it.
- **Alias**: a `set_frontmatter` on the existing entity, category `frontmatter`, tier `approve-each`. `aliases` replaces the list, so write the existing aliases plus the new one. A new entity's aliases go in its `create` content.
- **Merge**: step 4.

Mentions stay plain text; curate links them when it next handles those notes. Seeding adds the targets.

Done when the plan holds one operation set per seed, alias and merge verdict, and every tier is `approve-each`.

## 4. Merge two entities

Merge B into A when B's notes and links should land on A. A plan deletes nothing, so a merge is an alias plus redirected links ([docs/plan.md](../../../docs/plan.md), "Merging entities"):

1. A `set_frontmatter` on A adding B's name to `aliases`.
2. Run `vault-curator entities --vault <vault> --links-to B --json`. For each note in `notes`, a `rewrite` (category `restructure`, tier `approve-each`) that changes each `[[B]]` to `[[A|B]]` and every other byte the same, including links in frontmatter, with one source per entry of `blocks`.
3. Leave B's own note in place and tell the user to move its content into A and delete it by hand after the merge; the alias then takes over B's name.

When B has no note, only plain-text mentions, step 1 alone is the merge.

Done when every note that `--links-to B` lists has a `rewrite` in the plan, and each `summary` names A.

## 5. Validate, review, apply

Follow curate steps 5 to 9 on this plan: [validate the draft](../curate/SKILL.md), render the review, wait for decisions, apply, hand off the merge. The review groups seeds under "new entities"; tell the user which verdicts you were unsure of.

Done when you have named the branch, the entity notes created, the aliases added and the merges applied.
