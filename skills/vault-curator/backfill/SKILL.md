---
name: vault-curator-backfill
description: Backfill an existing markdown (Obsidian) vault with vault-curator. Use when the user asks to curate their vault for the first time, backfill or process old notes, or resume a partial backfill.
---

# Backfill the vault

A one-time pass that curates the existing vault in small batches, each one reviewed, applied and merged before the next. The first batch seeds entities, because every other note links to them. Each batch is an ordinary [curate](../curate/SKILL.md) run over a few notes, so the rules there hold: you propose, the user approves, the CLI applies. You run every command and edit no vault file yourself.

Batch size is `<N>`: 5 notes unless the user names another. Use fewer when the notes are long or the user wants lighter reviews.

Run the steps in order. Each ends on a criterion; continue only when it holds.

## 1. Prepare (state: backfill.prepare)

Do curate steps 1 and 2: locate the vault (`<vault>`) and check the binary.

Done when `<vault>` is a folder inside a git work tree and `vault-curator version` exits 0; then go to step 2. On no vault or a missing binary, hand off to the setup skill at its step 1, exactly as curate steps 1 and 2 say.

## 2. Find where backfill stands (state: backfill.start)

Run `vault-curator scan --vault <vault> --all --limit <N> --json`. `--all` ignores the watermark and lists notes with unprocessed blocks, in path order. `remaining` counts the notes beyond this batch. Flags: [docs/cli.md](../../../docs/cli.md) under "scan".

Done when you can state the batch's notes, `remaining`, and `watermark.exists`.

- On no candidates, backfill is finished: tell the user and stop (done).
- On `watermark.exists` false, no curation has run, so this is the first batch: go to step 3.
- On candidates and `watermark.exists` true, go to step 4. This is also how you resume after an interruption; the state file records what is done, so rerunning this step yields the next batch.

## 3. Batch 1: seed entities (state: backfill.seed)

Follow the [entities](../entities/SKILL.md) skill from its step 2 through its step 5 on the whole vault. Its step 5 hands off to curate step 5, and the review-to-merge steps run once there; do not repeat curate steps 6 to 9 here, and skip entities step 6.

Done when the seed run exits: merged and watermarked, applied nothing, or discarded for a re-plan (a declined merge or an abandoned review stops the run instead). Then go to step 5, whatever the exit; never back to step 2, so seeding runs once per backfill. The seeded stubs, if any, now exist as link targets.

## 4. Batch: curate the notes (state: backfill.plan)

Take the notes from the latest `scan --all --limit` (step 2, or step 5 for later batches). Do curate step 4 on exactly these notes, with the order and size fixed by the scan: write one plan. Then go to curate step 5, which carries the plan through the review-to-merge steps (validate the draft, present the review in chat, record the decisions, apply, confirm and merge). Read [PLANNING.md](../curate/PLANNING.md) before writing the plan. Tell the user how many notes remain after this batch.

Done when curate steps 5 to 9 have exited: merged and watermarked, applied nothing, or discarded for a re-plan. Every exit goes to step 5, which finds out what the batch left. A declined merge or an abandoned review stops the run; start the skill again at step 2 to resume.

Every note in the batch ends processed: a note that needs no change gets a `keep` operation in the plan. A note whose operations the user rejected stays unprocessed and returns in the next batch; for a note the user wants left alone, propose `keep` for it in that batch.

## 5. Repeat or finish (state: backfill.next)

Run `vault-curator scan --vault <vault> --all --limit <N> --json` again. A note left unprocessed (rejected operations, a discarded batch) returns in it.

- On candidates, go to step 4 with that batch.
- On an empty `candidates` list, backfill is finished (done).

Done when the scan is in hand and you have taken one of those branches, or the user stops. Tell the user the number of batches run and the notes that stayed unprocessed. To resume later, start this skill again at step 2.
