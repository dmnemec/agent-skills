package review

import (
	"os"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
)

const goldenKeep = "testdata/keep.review.golden.md"

// keepExample is the example plan plus two keeps of notes it leaves alone.
func keepExample(t *testing.T) *plan.Plan {
	t.Helper()
	_, p := pendingExample(t)
	for i, n := range []string{"daily/2026-09-15.md", "daily/2026-09-16.md"} {
		p.Operations = append(p.Operations, plan.Operation{
			ID: []string{"keep-0915", "keep-0916"}[i], Op: plan.Keep, Category: plan.CategoryKeep,
			Tier: plan.TierAuto, Status: plan.Pending, Summary: "Already well-formed", Note: n,
		})
	}
	return p
}

func TestKeepGolden(t *testing.T) {
	v, _ := pendingExample(t)
	p := &plan.Plan{Version: plan.Version}
	p.Operations = []plan.Operation{
		{ID: "keep-0915", Op: plan.Keep, Category: plan.CategoryKeep, Tier: plan.TierAuto, Status: plan.Pending,
			Summary: "Nothing to do", Note: "daily/2026-09-15.md"},
		{ID: "keep-0916", Op: plan.Keep, Category: plan.CategoryKeep, Tier: plan.TierAuto, Status: plan.Pending,
			Note: "daily/2026-09-16.md"},
	}
	got := render(t, v, p)
	if *update {
		if err := os.WriteFile(goldenKeep, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenKeep)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/vaultcurator/review -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("review differs from %s (rerun with -update if intended):\n%s", goldenKeep, got)
	}
}

func TestKeepIsLastAndSyncs(t *testing.T) {
	v, _ := pendingExample(t)
	p := keepExample(t)
	doc := render(t, v, p)
	kept := strings.Index(doc, "## Kept notes")
	if kept < 0 {
		t.Fatalf("no Kept notes section:\n%s", doc)
	}
	for _, other := range []string{"## Splits", "## New entities", "## Frontmatter", "## Links"} {
		if i := strings.Index(doc, other); i < 0 || i > kept {
			t.Errorf("%s (at %d) should come before Kept notes (at %d)", other, i, kept)
		}
	}
	if !strings.Contains(doc, "`daily/2026-09-15.md`") || !strings.Contains(doc, "`daily/2026-09-16.md`") {
		t.Errorf("kept notes not listed:\n%s", doc)
	}

	// Approve everything, then reject one keep by its own line.
	doc = edit(t, doc, "- category `keep` status: pending", "- category `keep` status: approved")
	doc = edit(t, doc, "- category `split` status: pending", "- category `split` status: approved")
	doc = edit(t, doc, "- category `entity` status: pending", "- category `entity` status: approved")
	doc = edit(t, doc, "- category `frontmatter` status: pending", "- category `frontmatter` status: approved")
	doc = edit(t, doc, "- category `links` status: pending", "- category `links` status: approved")
	for _, id := range []string{"split-standup", "split-retry", "split-beacon", "split-teodor", "entity-teodor"} {
		doc = edit(t, doc, "- op `"+id+"` status: pending", "- op `"+id+"` status: approved")
	}
	if err := Sync([]byte(doc), p); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"keep-0915", "keep-0916"} {
		if p.Op(id).Status != plan.Approved {
			t.Errorf("%s = %s, want approved", id, p.Op(id).Status)
		}
	}
}
