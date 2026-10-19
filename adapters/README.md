# Adapters

vault-curator is a set of skill folders plus a CLI. The folders are the same for every product; what differs is where each product loads skills from and where it reads your global instructions (the file that holds the `## Notes vault` section).

Common to all four:

- Clone the repo once to `~/agent-skills`. The skill folders are `curate`, `backfill`, `entities` and `setup` under `~/agent-skills/skills/vault-curator/`.
- Install by symlinking each folder into the product's skills location, so `git pull` updates it and the skills' links into `docs/` and `testdata/` stay reachable. If symlinks fail (Windows without developer mode), copy the folder instead; a copy still works from its `SKILL.md` and `PLANNING.md` alone.
- Install the binary first: `go install github.com/dmnemec/agent-skills/cmd/vault-curator@latest` (Go 1.24 or newer).
- The setup skill writes the vault path to `~/AGENTS.md`:

  ```markdown
  ## Notes vault

  The Obsidian vault is at /Users/me/Documents/vault (a git repository).
  ```

  Each product below says whether it reads that file or needs the section placed elsewhere.
- Start a new session after installing; skills are discovered at startup.
- Run it: invoke the skill ("curate my vault") and answer in chat. The agent runs every command and edits every file; the only questions are the review decisions and a final confirmation before it merges.

On Windows, replace `~` with `%USERPROFILE%` and copy instead of symlinking unless developer mode is on.

## Claude Code

Skills location: `~/.claude/skills/<skill-name>/SKILL.md` (personal, all projects). Project level is `.claude/skills/`.

Install:

```
mkdir -p ~/.claude/skills
for s in curate backfill entities setup; do
  ln -s ~/agent-skills/skills/vault-curator/$s ~/.claude/skills/vault-curator-$s
done
```

Any folder name works as long as `SKILL.md` is inside. Fallback: copy the folders.

Symlinks: supported. The docs say symlinked skill folders work for the enterprise, personal and project locations.

Global instructions: `~/.claude/CLAUDE.md` is the documented user-level file. Claude Code reads `AGENTS.md` as project instructions only where no `CLAUDE.md` exists, and the docs do not say it reads a home-directory `~/AGENTS.md` from sessions started elsewhere. Unverified: that `~/AGENTS.md` is picked up on its own. Assumed: import it from `~/.claude/CLAUDE.md` with the documented `@path` syntax (absolute and home paths are allowed):

```markdown
@~/AGENTS.md
```

Differences: none; the skills were written for Claude first. Skills load on demand from their `description`, and the agent runs the CLI directly.

Source: https://code.claude.com/docs/en/skills and https://code.claude.com/docs/en/memory

## OpenAI Codex

Skills location (Codex CLI): user `$HOME/.agents/skills`; repo `.agents/skills` from the working directory up to the repo root; admin `/etc/codex/skills`.

Install:

```
mkdir -p ~/.agents/skills
for s in curate backfill entities setup; do
  ln -s ~/agent-skills/skills/vault-curator/$s ~/.agents/skills/vault-curator-$s
done
```

Fallback: copy the folders.

Symlinks: supported. The docs say Codex "supports symlinked skill folders and follows the symlink target when scanning these locations."

Global instructions: `~/.codex/AGENTS.md` (or under `$CODEX_HOME`). Codex reads `AGENTS.override.md` there first if it exists, otherwise `AGENTS.md`, and uses only the first non-empty file. Unverified: that Codex reads a plain `~/AGENTS.md`; the docs describe only the Codex home directory and project directories. Assumed: put the `## Notes vault` section in `~/.codex/AGENTS.md` (or symlink that file to `~/AGENTS.md`).

Differences: Unverified. Assumed: whether Codex's sandbox lets `vault-curator` and `git` write in the vault outside the workspace is not confirmed; it may prompt for approval or need the vault added as a writable directory.

Source: https://developers.openai.com/codex/skills (redirects to https://learn.chatgpt.com/docs/build-skills) and https://developers.openai.com/codex/guides/agents-md (redirects to https://learn.chatgpt.com/docs/agent-configuration/agents-md)

## GitHub Copilot

Skills location: personal `~/.copilot/skills/` or `~/.agents/skills/`; project `.github/skills/`, `.claude/skills/` or `.agents/skills/`. Per the docs, agent skills work in the Copilot CLI, the cloud agent, and agent mode in VS Code and JetBrains. This section covers the CLI, which runs local commands.

Install:

```
mkdir -p ~/.copilot/skills
for s in curate backfill entities setup; do
  ln -s ~/agent-skills/skills/vault-curator/$s ~/.copilot/skills/vault-curator-$s
done
```

If you already linked the skills into `~/.agents/skills` for Codex, Copilot reads that location too and needs nothing more. Fallback: copy the folders.

Symlinks: Unverified. The docs do not mention them. Assumed to work as in the other products; copy the folders if the skills do not appear.

Global instructions: `$HOME/.copilot/copilot-instructions.md` ("user-level instructions that apply across repositories"; `COPILOT_HOME` relocates the directory). The CLI also reads `AGENTS.md` in the repository root, working directory and directories between them, plus extra directories listed in `COPILOT_CUSTOM_INSTRUCTIONS_DIRS`. Unverified: that a home-directory `~/AGENTS.md` is read. Assumed: put the `## Notes vault` section in `~/.copilot/copilot-instructions.md`, or add `$HOME` to `COPILOT_CUSTOM_INSTRUCTIONS_DIRS`.

Differences: the cloud agent and IDE agent mode work on a repository checkout or the open workspace, not an arbitrary vault folder, and cannot install the Go binary. Unverified: assumed that only the CLI is suitable for vault-curator.

Source: https://docs.github.com/en/copilot/concepts/agents/about-agent-skills, https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills and https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-custom-instructions

## Code Puppy

Skills location: `~/.code_puppy/skills/` (user) and `./skills/` (project). Add other directories with `/skills add <path>` or `skill_directories` in the config. Skills are on by default (`skills_enabled`) and scanned at startup. Each skill needs a `SKILL.md` with `name` and `description` frontmatter.

Install:

```
mkdir -p ~/.code_puppy/skills
for s in curate backfill entities setup; do
  ln -s ~/agent-skills/skills/vault-curator/$s ~/.code_puppy/skills/vault-curator-$s
done
```

Fallback: copy the folders.

Symlinks: Unverified. The skills doc does not say whether symlinked skill directories are followed. If a linked skill is not listed after a restart, copy the folder.

Global instructions: `~/.code_puppy/AGENTS.md` ("global rules, applied to all projects"; combined with the project `.code_puppy/AGENTS.md` or `./AGENTS.md`, global first). Unverified: that `~/AGENTS.md` is read. Assumed: put the `## Notes vault` section in `~/.code_puppy/AGENTS.md`.

Differences: Unverified. This is the open item "Code Puppy specifics" in [docs/design.md](../docs/design.md). The skills doc says nothing about tool names, MCP, or whether relative links from `SKILL.md` into `docs/` resolve. Confirm on the work machine and update this section.

Source: https://github.com/mpfaffenberger/code_puppy/blob/main/docs/AGENT_SKILLS.md and https://github.com/mpfaffenberger/code_puppy (README, global `~/.code_puppy/AGENTS.md`)
