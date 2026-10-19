---
name: curate
description: Curate the user's markdown (Obsidian) vault with vault-curator. Use when the user asks to curate, tidy, clean up, organize, split or link notes in the vault, or to sweep new daily notes.
---

# Curate the vault

The user invokes this skill and answers questions in chat; that is the whole of their part. You run every command and write every file, the review's status lines included. You propose, the user approves, the CLI applies. Edit no vault file yourself; the plan is the only thing that reaches the vault (the review file is a working copy you fill in). Content and action-item state stay exactly as the user wrote them.

Run the steps in order. Each ends on a criterion; continue only when it holds.

## 1. Locate the vault (state: curate.locate)

Read the user's home-directory `AGENTS.md` for the vault path. Ask the user if it names none.

Done when the path is an existing folder inside a git work tree (`git -C <vault> rev-parse --show-toplevel` succeeds). Call it `<vault>` below.

## 2. Check the binary (state: curate.binary)

Run `vault-curator version`.

Done when it exits 0. When the command is missing or fails, run the setup skill (`vault-curator-setup`, [setup](../setup/SKILL.md)) yourself, then resume at step 3. If the setup skill is not installed, follow the repo's [INSTALL.md](../../../INSTALL.md). Run `vault-curator --help` for the command list; each command's `--help` has its flags.

### Earlier runs (state: curate.leftover)

Run `git -C <vault> branch --list 'curate/*'`. A listed branch is an earlier run that was applied but never finished: not merged, or merged without a watermark (`watermark` refuses for the same reason and names such branches). Ask the user whether to finish it (go to step 9 with that branch) or start a new run (continue).

## 3. Scan (state: curate.scan)

Run `vault-curator scan --vault <vault> --json`. Output shape: [docs/cli.md](../../../docs/cli.md) (if the repo is not at hand, `vault-curator scan --help`).

Done when you can name every candidate note, its blocks with their hashes, and the orphans and broken links. A scan with no candidates ends the run: tell the user nothing is new.

## 4. Write the plan (state: curate.plan)

Write `<vault>/.vault-curator/plans/<UTC YYYYMMDD-HHMMSS>.json` (create the folder if missing). The plan format and its rules are in [docs/plan.md](../../../docs/plan.md). Every `status` is `pending`. Before choosing boundaries, hubs or frontmatter, read [PLANNING.md](PLANNING.md).

Done when every new block of every candidate note is mapped by an operation, or its note is covered by a `keep` operation, and you can say which for each. A note you leave alone gets a `keep`; without one it returns in every later scan.

## 5. Validate the draft (state: curate.validate_draft)

Run `vault-curator validate --draft --vault <vault> <plan>`. Fix the plan, never the vault, until it exits 0.

Done when validate prints no errors.

## 6. Present the review (state: curate.review)

Run `vault-curator review --vault <vault> <plan>`; it writes `<plan minus .json>.review.md` next to the plan. Read that file, then present it in chat. The user reads the conversation, not the file.

1. **Summary first**: notes touched, operations per category, and the assumptions behind your boundaries and primary topics, so the user can correct them.
2. **Then each category** in review order. List its `auto` operations compactly (one line each: id and summary), and ask once: approve the category, or name the exceptions.
3. **Then each `approve-each` operation on its own**: id, summary, and its before/after (or split boundary) from the review. Ask approve or reject. A category answer never decides these; sync refuses while one is pending.

Done when every category and every `approve-each` operation has an answer from the user. Offer opening the review file in an editor as an alternative; do not make it the default.

## 7. Record the decisions (state: curate.record)

Write the answers into the review's `status:` lines yourself (`approved` or `rejected`; format in docs/cli.md under "Status lines"). Then run `vault-curator review --sync <review> <plan>`, which also stamps the plan as reviewed; `apply` refuses a plan whose statuses were not synced, so never write statuses into the plan yourself.

Done when sync exits 0 and the plan holds the statuses. If sync refuses because an operation is still pending, ask the user about it. If it refuses because the plan changed, render again with `--force` and return to step 6.

## 8. Apply (state: curate.apply)

Run `vault-curator validate --vault <vault> <plan>`, then `vault-curator apply --vault <vault> <plan>`. Apply snapshots the user's uncommitted vault changes first, then commits the approved operations on a `curate/*` branch in a temporary worktree. The vault's working tree stays untouched.

Done when apply exits 0 and prints the branch name.

### Validation fails after the decisions (state: curate.revise)

When `validate` or `apply` reports plan errors on the synced plan, nothing is applied. Explain the errors to the user, revise the plan, run `validate --draft` until it exits 0, render the review again with `--force`, and return to step 6, asking only about the operations that changed.

## 9. Confirm, merge, watermark (state: curate.confirm)

Show the user what changed: the branch name, each note changed (from apply's output) and, for rewrites and splits, the diff (`git -C <vault> diff <base>..<branch>`). Ask in chat whether to merge the branch into the vault.

- **Confirmed** (an explicit yes to this question; approvals from step 6 do not count): go to Merge below.
- **Declined**: change nothing. Tell the user the branch `<curate/branch>` stays and the run can be finished later: invoke this skill again, and "Earlier runs" finds the branch and offers to merge it.

Done when the merge and `watermark` both exit 0 and you have told the user the notes now merged, or when the user declined and you have said how to finish later.

### Merge (state: curate.merge)

Run `git -C <vault> merge --no-ff <curate/branch>`, then go to Watermark.

### Merge conflict (state: curate.abort_merge)

When the merge stops on conflicts, run `git -C <vault> merge --abort`, remove the branch's worktree and delete the branch, tell the user, and plan again from a fresh scan.

### Watermark (state: curate.watermark)

Run `vault-curator watermark --vault <vault> --branch <curate/branch>`.
