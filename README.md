# agent-skills

Portable agent skills, plus the small CLIs that back them. The skills are built for Claude first and then adapted for other coding agents, such as Code Puppy and Codex.

## Projects

### vault-curator

> **Status:** v1 in progress. Implemented: `scan`, `validate`, `review`, `apply`, `watermark` and the `curate` skill. Not yet: hubs, entities, backfill, retrieval.

A set of skills plus a Go CLI that turns a flat, unlinked markdown vault (such as an Obsidian vault) into a linked, typed knowledge base. Every change is reviewed before it is committed.

- Restructures new notes, splits daily notes that hold several meetings, and links each note to its projects, components and people.
- Pulls decisions, constraints, open questions and action items out of meeting notes, and flags decisions that deserve an ADR.
- Maintains hub notes with meeting timelines, open actions and Mermaid diagrams.
- Builds a context pack before a design session, and exposes the component graph as JSON for other agents.

The agent proposes a plan, you approve it, and the CLI applies it on a separate git branch. The agent never edits your vault directly.

### Using vault-curator

Install the `vault-curator` binary and the `curate` skill, point your home-directory `AGENTS.md` at the vault, then ask your agent to curate it. Claude Code steps are in [adapters/claude/README.md](adapters/claude/README.md); the commands are documented in [docs/cli.md](docs/cli.md).

The full design is in [docs/design.md](docs/design.md).

## Planned layout

```text
skills/     SKILL.md folders (curate, backfill, entities, context, ask)
cmd/        Go source for the CLI
adapters/   thin per-agent notes for Claude, Code Puppy and Codex
testdata/   a synthetic vault for tests and demos
docs/       design documents
```

## License

Copyright (C) 2026 Devin Nemec.

Licensed under the [GNU Affero General Public License v3.0](LICENSE). You may use, study, modify and share this code. If you distribute it, or a modified version, or run a modified version as a network service, you must release your source under the same license and keep the copyright and license notices intact.

If you want to use it under other terms, open an issue.
