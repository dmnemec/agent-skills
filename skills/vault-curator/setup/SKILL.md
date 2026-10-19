---
name: vault-curator-setup
description: Set up vault-curator from nothing. Use when the user asks to install or set up vault-curator, when `vault-curator` is missing, or when no vault path is recorded in their home-directory AGENTS.md.
---

# Set up vault-curator

Take the user from nothing to a scanned vault. The user only chats with you; you run every command. Run the steps in order. Each ends on a criterion; continue only when it holds.

Rules for every step:

- Check first. If the criterion already holds, report "already set up" for that step and change nothing.
- Announce each change to the machine before you make it.
- Ask the user to confirm before `git init` and before editing any file outside the vault-curator install (the home-directory `AGENTS.md`).

## 1. Binary (state: setup.binary)

Run `vault-curator version`. Latest is the newest tag from `git ls-remote --tags https://github.com/dmnemec/agent-skills`, or the version `go install` resolves if there are no tags.

- Missing, or older than latest: run `go install github.com/dmnemec/agent-skills/cmd/vault-curator@latest`.
- `go` missing: tell the user Go 1.24 or newer is needed (https://go.dev/dl/), then stop. A later run of this skill continues once Go is installed.
- Installed but not on `PATH`: add `$(go env GOPATH)/bin` to `PATH` for this session, then ask the user once whether to add it permanently. With a yes, detect the shell and append the entry yourself: the bash or zsh profile (`~/.zshrc`, `~/.bashrc`) on macOS and Linux; the PowerShell `$PROFILE` or the user `PATH` environment variable on Windows. Skip the append when the profile already contains the entry. Announce which file or variable changed.

Done when `vault-curator version` exits 0 and prints a version at least as new as latest; then go to step 2. A `dev` build counts as current.

## 2. Vault (state: setup.vault)

Read the `## Notes vault` section of `~/AGENTS.md` first. If it names a folder that exists, that is `<vault>`.

Otherwise ask the user for the path. Offer to search common locations (home, `~/Documents`, `~/Dropbox`, `~/iCloud*`, `~/OneDrive*`) for folders containing `.obsidian/`, and let them pick.

Then run `git -C <vault> rev-parse --show-toplevel`. If `<vault>` is not in a git work tree, offer `git init` in `<vault>`, and run it only after the user agrees. Without agreement, tell the user vault-curator needs git, then stop.

Done when `<vault>` exists, holds markdown, and `git -C <vault> rev-parse --show-toplevel` exits 0; then go to step 3. When the user names no vault, or declines `git init`, stop.

## 3. Record the vault (state: setup.record)

Two files. `~/AGENTS.md` (`%USERPROFILE%\AGENTS.md` on Windows) is the single source of the vault path; the running product's global instructions file must load it.

First, `~/AGENTS.md` holds exactly one section, shaped like this:

```markdown
## Notes vault

The Obsidian vault is at <vault> (a git repository).
```

- No file: say you will create it, wait for a yes, create it.
- File without the section: say you will append it, wait for a yes, append.
- Section present with the same path: change nothing.
- Section present with another path: show the user the old and new path, wait for a yes, replace the path in place.

Second, the running product's global instructions file. Take its path from the section for this product in [adapters/README.md](../../../adapters/README.md). If that section says the file imports other files with `@path` lines (Claude Code does):

- Ensure the file contains the line `@~/AGENTS.md`. Missing: say you will append it, wait for a yes, append. Do not also copy the section.

Otherwise (copy-only product):

- Ensure the file contains the `## Notes vault` section copied from `~/AGENTS.md`. Missing: say you will append it, wait for a yes, append. Present with different text: say you will update it in place, wait for a yes, update. Never add a second copy.

If the file itself does not exist, say you will create it, wait for a yes, then write it. Tell the user each file you wrote or left alone.

Done when the running product's global file contains `@~/AGENTS.md` exactly once (import case) or exactly one `## Notes vault` section matching `~/AGENTS.md` (copy case), and `~/AGENTS.md` holds exactly one `## Notes vault` section naming `<vault>`; then go to step 4. On a declined write, stop.

## 4. Skills (state: setup.skills)

`<repo>` is the clone of https://github.com/dmnemec/agent-skills that contains this skill (follow the symlink or junction to find it). If this skill was copied out of a clone, clone the repo to `~/agent-skills` and use that.

The skills are the folders `curate`, `backfill`, `entities` and `setup` under `<repo>/skills/vault-curator/`. Each installs as `vault-curator-<folder>`, the `name:` in its `SKILL.md`: `vault-curator-curate`, `vault-curator-backfill`, `vault-curator-entities`, `vault-curator-setup`. Find where the user's agent reads skills from the matching section of [adapters/README.md](../../../adapters/README.md): `#claude-code`, `#openai-codex`, `#github-copilot` or `#code-puppy`. If the section is missing or marked unverified, tell the user and ask them for the location.

For each of the four install names not already present there, link it to `<repo>/skills/vault-curator/<folder>`. Announce the list first. A link keeps the `docs/` links the skills use reachable; a copy breaks them.

- macOS and Linux: a symlink (`ln -s`).
- Windows: a directory junction, `mklink /J <skills location>\vault-curator-<folder> <repo>\skills\vault-curator\<folder>` (run in `cmd`; no administrator rights or developer mode needed).
- Where a link fails, tell the user and copy the folder, and say that the copy cannot reach `docs/`.

Done when all four install names are present in the skills location, each containing a `SKILL.md`. If you installed any, tell the user to start a new agent session so it loads them. Then go to step 5.

## 5. First scan (state: setup.scan)

Run `vault-curator scan --vault <vault>` (read-only). Report the candidate count and whether the watermark exists (`--json` gives `watermark.exists`).

Done when you have reported the numbers. Then offer the next step and act on the answer:

- No watermark (first-time vault): offer the [backfill](../backfill/SKILL.md) skill.
- Watermark exists: offer the [curate](../curate/SKILL.md) skill.
- On the user choosing curate, start the curate skill at its step 1; on backfill, start the backfill skill at its step 1.
- On no, you are done. If every step reported "already set up", say "already set up".
