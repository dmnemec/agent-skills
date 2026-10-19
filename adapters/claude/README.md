# vault-curator for Claude Code

Install the `curate` skill so Claude Code can tidy, split and link the notes in your vault, with you approving every change.

## 1. Install the binary

```
go install github.com/dmnemec/agent-skills/cmd/vault-curator@latest
vault-curator version
```

Or clone this repo and run `go build -o vault-curator ./cmd/vault-curator`, then put the binary on your `PATH`. It needs Go 1.24 or newer.

## 2. Install the skill

Claude Code reads skills from `~/.claude/skills/`. Copy the folder, or symlink it so it follows repo updates:

```
git clone https://github.com/dmnemec/agent-skills
mkdir -p ~/.claude/skills
ln -s "$PWD/agent-skills/skills/vault-curator/curate" ~/.claude/skills/curate
```

On Windows, copy `skills/vault-curator/curate` into `%USERPROFILE%\.claude\skills\curate`.

The skill links to `docs/` and `testdata/` of this repo. A copy outside the clone still works from `SKILL.md` and `PLANNING.md` alone, but a symlink keeps those references reachable.

## 3. Tell Claude where the vault is

Add the vault path to `~/AGENTS.md` (or the `CLAUDE.md` your setup loads from home):

```markdown
## Notes vault

The Obsidian vault is at /Users/me/Documents/vault (a git repository).
```

The vault must be a git repo, or a folder inside one.

## 4. Run it

Ask Claude to "curate my vault". It scans, writes a plan to `<vault>/.vault-curator/plans/`, and gives you the path of a review file to open in any editor. Mark each section `approved` or `rejected`, tell Claude you are done, and it applies the plan on a `curate/*` branch. Merge that branch in the vault's repo, then run `vault-curator watermark --vault <vault>`.

The `.vault-curator/plans/` folder is hidden from Obsidian and kept in git as an audit trail.
