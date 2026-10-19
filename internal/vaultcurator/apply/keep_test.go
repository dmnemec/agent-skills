package apply

import (
	"fmt"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gittest"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/scan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/state"
)

func keepOps(notes ...string) []plan.Operation {
	var ops []plan.Operation
	for i, n := range notes {
		ops = append(ops, plan.Operation{
			ID: fmt.Sprintf("keep-%d", i), Op: plan.Keep, Category: plan.CategoryKeep,
			Tier: plan.TierAuto, Status: plan.Approved, Note: n,
		})
	}
	return ops
}

func candidatePaths(t *testing.T, dir string) map[string]bool {
	t.Helper()
	s, err := scan.Run(dir, scan.Options{All: true})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, c := range s.Candidates {
		out[c.Path] = true
	}
	return out
}

func TestApplyKeepOnlyMarksNotes(t *testing.T) {
	r := gittest.Vault(t)
	before := snapshot(t, r.Dir)
	kept := []string{"daily/2026-09-14.md", "Coffee machine options.md"}
	for _, k := range kept {
		if !candidatePaths(t, r.Dir)[k] {
			t.Fatalf("%s is not a candidate before apply", k)
		}
	}

	p := &plan.Plan{Version: plan.Version, Operations: keepOps(kept...)}
	res, err := Apply(r.Dir, p, Options{SkipReviewCheckForTests: true, Branch: testBranch, WorktreeRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changes) != 0 || len(res.Kept) != 2 {
		t.Errorf("changes = %d, kept = %v", len(res.Changes), res.Kept)
	}

	// Only the state file differs from the vault before.
	after := snapshot(t, res.VaultDir)
	for path, content := range before {
		if after[path] != content {
			t.Errorf("%s changed", path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok && path != state.Path && path != ".git" {
			t.Errorf("unexpected new file %s", path)
		}
	}
	if files := run(t, r, "show", "--name-only", "--format=", res.Commit); files != state.Path {
		t.Errorf("commit changes %q, want only %s", files, state.Path)
	}

	// After the merge, a scan no longer lists the kept notes.
	run(t, r, "merge", "--no-ff", "--quiet", "-m", "merge curation", testBranch)
	got := candidatePaths(t, r.Dir)
	for _, k := range kept {
		if got[k] {
			t.Errorf("%s is still a candidate after keep", k)
		}
	}
	if !got["daily/2026-09-15.md"] {
		t.Errorf("unrelated notes should stay candidates: %v", got)
	}
}

func TestApplyKeepWithWritesMarksBoth(t *testing.T) {
	r := gittest.Vault(t)
	p := loadPlan(t)
	p.Operations = append(p.Operations, keepOps("daily/2026-09-15.md")...)
	res, err := Apply(r.Dir, p, Options{SkipReviewCheckForTests: true, Branch: testBranch, WorktreeRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.Load(res.VaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Tracked("daily/2026-09-15.md") || !st.Tracked("daily/2026-09-14.md") {
		t.Errorf("tracked: %v", st.Paths())
	}
}

func TestApplyRefusesKeepWithChangeOnSameNote(t *testing.T) {
	r := gittest.Vault(t)
	p := loadPlan(t)
	p.Operations = append(p.Operations, keepOps("daily/2026-09-14.md")...)
	_, err := Apply(r.Dir, p, Options{SkipReviewCheckForTests: true, Branch: testBranch, WorktreeRoot: t.TempDir()})
	if _, ok := err.(*InvalidPlanError); !ok {
		t.Fatalf("err = %v, want InvalidPlanError", err)
	}
}
