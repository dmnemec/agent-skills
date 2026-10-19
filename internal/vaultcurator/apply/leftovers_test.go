package apply

import (
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gittest"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/scan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/state"
)

// plainPaths returns the paths a plain scan lists.
func plainPaths(t *testing.T, dir string) map[string]bool {
	t.Helper()
	s, err := scan.Run(dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, c := range s.Candidates {
		out[c.Path] = true
	}
	return out
}

func TestLeftoversReturnInPlainScan(t *testing.T) {
	r := gittest.Vault(t)
	const rejected = "Coffee machine options.md"
	const kept = "daily/2026-09-14.md"

	ops := keepOps(kept, rejected)
	ops[1].Status = plan.Rejected
	p := &plan.Plan{Version: plan.Version, Operations: ops}
	if _, err := Apply(r.Dir, p, Options{Branch: testBranch, WorktreeRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	run(t, r, "merge", "--no-ff", "--quiet", "-m", "merge curation", testBranch)
	if _, err := Watermark(r.Dir, WatermarkOptions{}); err != nil {
		t.Fatal(err)
	}

	got := plainPaths(t, r.Dir)
	if !got[rejected] {
		t.Fatalf("rejected note is not in a plain scan: %v", got)
	}
	if got[kept] {
		t.Errorf("kept note is in a plain scan")
	}
	for path := range candidatePaths(t, r.Dir) {
		if !got[path] {
			t.Errorf("%s is in scan --all but not in a plain scan", path)
		}
	}

	// A keep clears the leftover.
	p2 := &plan.Plan{Version: plan.Version, Operations: keepOps(rejected)}
	if _, err := Apply(r.Dir, p2, Options{Branch: "curate/second", WorktreeRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	run(t, r, "merge", "--no-ff", "--quiet", "-m", "merge curation", "curate/second")
	if _, err := Watermark(r.Dir, WatermarkOptions{}); err != nil {
		t.Fatal(err)
	}
	if plainPaths(t, r.Dir)[rejected] {
		t.Error("kept note still in a plain scan")
	}
	st, err := state.Load(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range st.Leftovers {
		if l == rejected {
			t.Error("leftover not cleared in state")
		}
	}
}
