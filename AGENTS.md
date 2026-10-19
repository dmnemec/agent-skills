# AGENTS.md

- Design or scope work on vault-curator: read [docs/design.md](docs/design.md) first. It holds the decisions already made, the non-goals and the open items.

## Git workflow

- `main` is read-only for agents. Work on a branch named `<issue>-<slug>` (a worktree when running in parallel), and hand off as a PR with `Closes #<n>`. The maintainer merges.
- Current effort: PRs target the `v1` integration branch, not `main`. List every default you chose in the PR body under `Assumed:`; the maintainer reviews them all before `v1` merges into `main`.
- Commit messages and PR titles use Conventional Commits. Breaking changes still get `!`, but ship as a minor bump: the version stays below v2, because a v2+ Go module needs a `/v2` import path.

## Implementation rules

- Build in the order given under "Suggested build order" in the design doc. Each step should work on its own before the next starts.
- An open item in the design doc is an unmade decision. When work depends on one, stop and ask in the issue or PR rather than picking an answer.
- Test only against the synthetic vault in `testdata/`. Leave real vaults untouched, including the one named in the user's home-directory `AGENTS.md`.
- The vault stays plain markdown in git: no database, index, cache or required Obsidian plugin.
- Every CLI query command takes a `--json` flag.
- Before opening a PR, run `gofmt`, `go vet ./...` and `go test ./...`, and make sure all three pass.
- Built binaries go out as GitHub release assets. Leave them out of commits.
- The repo is AGPL-3.0. Add only dependencies, and copy only code, whose licenses are compatible with it (MIT, BSD, Apache-2.0, MPL-2.0, GPL-3.0 and AGPL-3.0 all are), and keep the original notices on anything copied.

## Agent skills

### Issue tracker

GitHub Issues in dmnemec/agent-skills, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five default labels (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
