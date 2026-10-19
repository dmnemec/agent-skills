---
name: backfill
description: Backfill an existing markdown (Obsidian) vault with vault-curator. Use when the user asks to curate their vault for the first time, backfill or process old notes, or resume a partial backfill.
---

# Backfill the vault

A one-time pass that curates the existing vault in small batches, each one reviewed, applied and merged before the next. The first batch seeds entities, because every other note links to them. Each batch is an ordinary [curate](../curate/SKILL.md) run over a few notes, so the rules there hold: you propose, the user approves, the CLI applies. You run every command and edit no vault file yourself.

Batch size is `<N>`: 5 notes unless the user names another. Use fewer when the notes are long or the user wants lighter reviews.

Run the steps in order. Each ends on a criterion; continue only when it holds.

## 1. Prepare

Do curate steps 1 and 2: locate the vault (`<vault>`) and check the binary.

Done when `<vault>` is a folder inside a git work tree and `vault-curator version` exits 0.

## 2. Find where backfill stands

Run `vault-curator scan --vault <vault> --all --limit <N> --json`. `--all` ignores the watermark and lists notes with unprocessed blocks, in path order. `remaining` counts the notes beyond this batch. Flags: [docs/cli.md](../../../docs/cli.md) under "scan".

Done when you can state the batch's notes, `remaining`, and `watermark.exists`.

- No candidates: backfill is finished; tell the user and stop.
- `watermark.exists` false: no curation has run, so this is the first batch; go to step 3.
- Otherwise: go to step 4. This is also how you resume after an interruption; the state file records what is done, so rerunning this step yields the next batch.

## 3. Batch 1: seed entities

Follow the [entities](../entities/SKILL.md) skill from its step 1 to its step 5 on the whole vault, then complete curate steps 6 to 9 (review in chat, decisions, apply, confirm, merge and watermark).

Done when the seed branch is merged and `vault-curator watermark` exits 0. The seeded stubs now exist as link targets. Then return to step 2.

## 4. Batch: curate the notes

Take the notes from the step 2 scan. Do curate steps 4 to 9 on exactly these notes, with the order and size fixed by the scan: write one plan, validate the draft, present the review in chat, record the decisions, apply, then confirm and merge. Read [PLANNING.md](../curate/PLANNING.md) before writing the plan. Tell the user how many notes remain after this batch.

Done when the merge is confirmed and run per curate step 9, `vault-curator watermark --vault <vault>` exits 0, and a fresh `scan --all --limit <N>` no longer lists this batch's notes.

Every note in the batch ends processed: a note that needs no change gets a `keep` operation in the plan. A note whose operations the user rejected stays unprocessed and returns in the next batch; for a note the user wants left alone, propose `keep` for it in that batch.

## 5. Repeat or finish

Return to step 2 for the next batch.

Done when a scan reports `remaining` 0 and an empty `candidates` list, or the user stops. Tell the user the number of batches run and the notes that stayed unprocessed. To resume later, start this skill again at step 2.
