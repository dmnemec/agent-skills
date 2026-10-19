---
name: curate
description: Curate the user's markdown (Obsidian) vault with vault-curator. Use when the user asks to curate, tidy, clean up, organize, split or link notes in the vault, or to sweep new daily notes.
---

# Curate the vault

You propose, the user approves, the CLI applies. Edit no vault file yourself; the only file you write is the plan, and the plan is the only thing that reaches the vault. Content and action-item state stay exactly as the user wrote them.

Run the steps in order. Each ends on a criterion; continue only when it holds.

## 1. Locate the vault

Read the user's home-directory `AGENTS.md` for the vault path. Ask the user if it names none.

Done when the path is an existing folder inside a git work tree (`git -C <vault> rev-parse --show-toplevel` succeeds). Call it `<vault>` below.

## 2. Check the binary

Run `vault-curator version`.

Done when it exits 0. When the command is missing, stop and tell the user to install it, then rerun this skill:

```
go install github.com/dmnemec/agent-skills/cmd/vault-curator@latest
```

(or `go build ./cmd/vault-curator` in a clone of github.com/dmnemec/agent-skills). Run `vault-curator --help` for the command list; each command's `--help` has its flags.

## 3. Scan

Run `vault-curator scan --vault <vault> --json`. Output shape: [docs/cli.md](../../../docs/cli.md) (if the repo is not at hand, `vault-curator scan --help`).

Done when you can name every candidate note, its blocks with their hashes, and the orphans and broken links. A scan with no candidates ends the run: tell the user nothing is new.

## 4. Write the plan

Write `<vault>/.vault-curator/plans/<UTC YYYYMMDD-HHMMSS>.json` (create the folder if missing). The plan format and its rules are in [docs/plan.md](../../../docs/plan.md). Every `status` is `pending`. Before choosing boundaries, hubs or frontmatter, read [PLANNING.md](PLANNING.md).

Done when every new block of every candidate note is either mapped by an operation or deliberately left alone, and you can say which for each.

## 5. Validate the draft

Run `vault-curator validate --draft --vault <vault> <plan>`. Fix the plan, never the vault, until it exits 0.

Done when validate prints no errors.

## 6. Render the review

Run `vault-curator review --vault <vault> <plan>`. It writes `<plan minus .json>.review.md` next to the plan.

Done when the review file exists. Tell the user its full path to open in any editor, and offer the alternative of showing it here in the conversation and recording their decisions into it. Name the assumptions behind your boundaries and primary topics, so the user can correct them.

## 7. Wait for decisions

Stop and wait. Decisions are the `status:` lines in the review (`approved` or `rejected`; format in docs/cli.md under "Status lines"). When the user answers in conversation, write those statuses into the review file yourself.

Done when the user has said they are finished deciding. Then run `vault-curator review --sync <review> <plan>`; it exits 0 and the plan now holds the statuses. If sync refuses because the plan changed, render again with `--force` and return to step 6.

## 8. Apply

Run `vault-curator validate --vault <vault> <plan>`, then `vault-curator apply --vault <vault> <plan>`. Apply snapshots the user's uncommitted vault changes first, then commits the approved operations on a `curate/*` branch in a temporary worktree. The vault's working tree stays untouched.

Done when apply exits 0 and prints the branch name.

## 9. Hand off the merge

Tell the user to merge the branch when satisfied, in the vault's repo, then record it:

```
git -C <vault> merge --no-ff <curate/branch>
vault-curator watermark --vault <vault>
```

Done when you have stated the branch name, the notes changed (from apply's output) and both commands. The run is complete; the watermark moves only after the user merges.
