package review

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

var update = flag.Bool("update", false, "rewrite golden files")

var (
	vaultDir    = filepath.Join("..", "..", "..", "testdata", "vault")
	examplePlan = filepath.Join("..", "..", "..", "testdata", "plans", "2026-09-14.json")
)

const goldenReview = "testdata/2026-09-14.review.golden.md"

var opts = Options{PlanPath: "2026-09-14.json", ReviewPath: "2026-09-14.review.md"}

func loadExample(t *testing.T) (*vault.Vault, *plan.Plan) {
	t.Helper()
	v, err := vault.Load(vaultDir, vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := plan.Load(examplePlan)
	if err != nil {
		t.Fatal(err)
	}
	return v, p
}

// pendingExample is the example plan as the agent hands it to review.
func pendingExample(t *testing.T) (*vault.Vault, *plan.Plan) {
	t.Helper()
	v, p := loadExample(t)
	for i := range p.Operations {
		p.Operations[i].Status = plan.Pending
	}
	return v, p
}

func render(t *testing.T, v *vault.Vault, p *plan.Plan) string {
	t.Helper()
	doc, err := Render(v, p, opts)
	if err != nil {
		t.Fatal(err)
	}
	return string(doc)
}

// edit replaces old with new in doc, which must contain old exactly once.
func edit(t *testing.T, doc, old, new string) string {
	t.Helper()
	if n := strings.Count(doc, old); n != 1 {
		t.Fatalf("review has %d copies of %q, want 1", n, old)
	}
	return strings.Replace(doc, old, new, 1)
}

func statusesOf(p *plan.Plan) map[string]plan.Status {
	out := map[string]plan.Status{}
	for _, op := range p.Operations {
		out[op.ID] = op.Status
	}
	return out
}

func TestGolden(t *testing.T) {
	v, p := pendingExample(t)
	got := render(t, v, p)
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenReview), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenReview, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenReview)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/vaultcurator/review -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("review differs from %s (rerun with -update if intended):\n%s", goldenReview, got)
	}
}

func TestRoundTrip(t *testing.T) {
	v, p := pendingExample(t)
	doc := render(t, v, p)

	// Approve the splits except the Beacon one, approve the entity and the
	// frontmatter, reject the links category, and make hand edits elsewhere.
	doc = edit(t, doc, "- category `split` status: pending", "- category `split` status: approved")
	doc = edit(t, doc, "- op `split-beacon` status: pending", "- op `split-beacon` status: Rejected")
	for _, id := range []string{"split-standup", "split-retry", "split-teodor"} {
		doc = edit(t, doc, "- op `"+id+"` status: pending", "- op `"+id+"` status: approved")
	}
	doc = edit(t, doc, "- category `entity` status: pending", "- category `entity` status: approved")
	doc = edit(t, doc, "- op `entity-teodor` status: pending", "* op `entity-teodor` status:approved")
	doc = edit(t, doc, "- category `frontmatter` status: pending", "- category `frontmatter`   status: approved  ")
	doc = edit(t, doc, "- category `links` status: pending", "- category `links` status: rejected")
	doc = edit(t, doc, "## Summary", "## Summary\n\nLooks good, but not the beacon split. status: whatever")
	doc = strings.ReplaceAll(doc, "\n", "\r\n")

	if err := Sync([]byte(doc), p); err != nil {
		t.Fatal(err)
	}
	want := map[string]plan.Status{
		"split-standup": plan.Approved,
		"split-retry":   plan.Approved,
		"split-beacon":  plan.Rejected,
		"split-teodor":  plan.Approved,
		"entity-teodor": plan.Approved,
		"fm-daily":      plan.Approved,
		"links-coffee":  plan.Rejected,
	}
	got := statusesOf(p)
	for id, s := range want {
		if got[id] != s {
			t.Errorf("%s: status %q, want %q", id, got[id], s)
		}
	}

	// The synced plan still validates (with the Beacon split skipped), and
	// a review rendered from it reads back to the same statuses.
	if r := plan.Validate(v, p, plan.Options{}); !r.OK() {
		t.Fatalf("synced plan invalid: %v", r.Errors)
	}
	again := render(t, v, p)
	for _, line := range []string{
		"- category `split` status: pending",
		"- op `split-beacon` status: rejected",
		"- op `split-standup` status: approved",
		"- category `links` status: rejected",
	} {
		if !strings.Contains(again, line+"\n") {
			t.Errorf("re-rendered review lacks %q", line)
		}
	}
	if err := Sync([]byte(again), p); err != nil {
		t.Fatal(err)
	}
	if got := statusesOf(p); len(got) != len(want) {
		t.Fatalf("statuses = %v", got)
	} else {
		for id, s := range want {
			if got[id] != s {
				t.Errorf("after second sync, %s: status %q, want %q", id, got[id], s)
			}
		}
	}
}

// approveEach approves every approve-each operation line in doc.
func approveEach(doc string) string {
	return regexp.MustCompile("(?m)^(- op `[^`]+` status: )pending").ReplaceAllString(doc, "${1}approved")
}

func TestAutoOpsFollowCategory(t *testing.T) {
	v, p := pendingExample(t)
	doc := render(t, v, p)
	doc = approveEach(doc)
	doc = edit(t, doc, "- category `split` status: pending", "- category `split` status: rejected")
	doc = edit(t, doc, "- category `entity` status: pending", "- category `entity` status: rejected")
	doc = edit(t, doc, "- category `frontmatter` status: pending", "- category `frontmatter` status: approved")
	doc = edit(t, doc, "- category `links` status: pending", "- category `links` status: rejected")
	if err := Sync([]byte(doc), p); err != nil {
		t.Fatal(err)
	}
	got := statusesOf(p)
	// Auto ops follow their category; approve-each ops keep their own status.
	if got["fm-daily"] != plan.Approved || got["links-coffee"] != plan.Rejected || got["split-standup"] != plan.Approved {
		t.Errorf("statuses = %v", got)
	}
}

func TestPendingApproveEachFailsSync(t *testing.T) {
	v, p := pendingExample(t)
	doc := render(t, v, p)
	// Every category is approved, but three approve-each ops are undecided.
	for _, cat := range []string{"split", "entity", "frontmatter", "links"} {
		doc = edit(t, doc, "- category `"+cat+"` status: pending", "- category `"+cat+"` status: approved")
	}
	doc = edit(t, doc, "- op `split-standup` status: pending", "- op `split-standup` status: approved")
	doc = edit(t, doc, "- op `split-retry` status: pending", "- op `split-retry` status: rejected")
	err := Sync([]byte(doc), p)
	if err == nil || !strings.Contains(err.Error(), "still pending: split-beacon, split-teodor, entity-teodor") {
		t.Fatalf("err = %v, want the pending approve-each ops named", err)
	}
	for _, op := range p.Operations {
		if op.Status != plan.Pending {
			t.Errorf("failed sync changed %s to %s", op.ID, op.Status)
		}
	}
}

func TestStatusLinesInCodeAreIgnored(t *testing.T) {
	v, p := pendingExample(t)
	p.Op("entity-teodor").Content += "\n```\n- op `split-beacon` status: rejected\n```\n"
	doc := render(t, v, p)
	if !strings.Contains(doc, "````markdown\n") {
		t.Fatalf("content with a fence is not fenced with a longer fence:\n%s", doc)
	}
	doc = edit(t, doc, "- category `split` status: pending", "- category `split` status: approved")
	doc = approveEach(doc)
	if err := Sync([]byte(doc), p); err != nil {
		t.Fatal(err)
	}
	if s := p.Op("split-beacon").Status; s != plan.Approved {
		t.Errorf("split-beacon status = %q, want approved", s)
	}
}

func TestSyncFailsLoudly(t *testing.T) {
	cases := []struct {
		name     string
		old, new string
		// change alters the plan after the review is rendered.
		change func(p *plan.Plan)
		want   string
	}{
		{name: "unknown op", old: "- op `split-beacon` status: pending", new: "- op `split-beacon` status: pending\n- op `split-nope` status: approved", want: "op split-nope is not in the plan"},
		{name: "unknown category", old: "- category `links` status: pending", new: "- category `links` status: pending\n- category `edges` status: approved", want: "category edges has no operations in the plan"},
		{name: "bad status", old: "- op `split-beacon` status: pending", new: "- op `split-beacon` status: yes please", want: `status "yes please" is not pending, approved or rejected`},
		{name: "duplicate", old: "- op `split-beacon` status: pending", new: "- op `split-beacon` status: pending\n- op `split-beacon` status: approved", want: "op split-beacon already has a status"},
		{name: "mangled", old: "- op `split-beacon` status: pending", new: "- op split-beacon status: rejected", want: "cannot read status line"},
		{name: "missing op line", old: "- op `split-beacon` status: pending\n", new: "", want: "op split-beacon is approve-each but has no status line"},
		{name: "missing category line", old: "- category `links` status: pending\n", new: "", want: "category links has no status line"},
		{name: "no header", old: "<!-- vault-curator review plan-sha256=", new: "<!-- plan-sha256=", want: "no review header"},
		{name: "plan changed", change: func(p *plan.Plan) { p.Op("links-coffee").Links = []string{"Harbor"} }, want: "the plan changed since this review was rendered"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, p := pendingExample(t)
			doc := render(t, v, p)
			if tc.old != "" {
				doc = edit(t, doc, tc.old, tc.new)
			}
			doc = strings.Replace(doc, "- category `split` status: pending", "- category `split` status: approved", 1)
			if tc.change != nil {
				tc.change(p)
			}
			err := Sync([]byte(doc), p)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			for _, op := range p.Operations {
				if op.Status != plan.Pending {
					t.Errorf("failed sync changed %s to %s", op.ID, op.Status)
				}
			}
		})
	}
}

func TestHashIgnoresStatuses(t *testing.T) {
	_, p := pendingExample(t)
	before, err := Hash(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Op("split-beacon").Status = plan.Rejected
	if after, _ := Hash(p); after != before {
		t.Error("hash changed with a status")
	}
	if s := p.Op("split-beacon").Status; s != plan.Rejected {
		t.Errorf("Hash changed the plan: status %q", s)
	}
	p.Op("split-beacon").Summary = "changed"
	if after, _ := Hash(p); after == before {
		t.Error("hash unchanged after editing a summary")
	}
}

func TestRenderRefusesInvalidPlan(t *testing.T) {
	v, p := pendingExample(t)
	p.Op("split-retry").Content = strings.Replace(p.Op("split-retry").Content, "cap at 5", "cap at five", 1)
	_, err := Render(v, p, opts)
	if err == nil || !strings.Contains(err.Error(), "op split-retry:") {
		t.Fatalf("err = %v", err)
	}
}

func TestRewriteShowsBeforeAndAfter(t *testing.T) {
	v, err := vault.Load(vaultDir, vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	n := v.Note("daily/2026-09-14.md")
	content := "# 2026-09-14\n\n" + strings.ReplaceAll(string(n.Src), "\t", "  ")
	p := &plan.Plan{Version: plan.Version, Operations: []plan.Operation{{
		ID: "restructure-daily", Op: plan.Rewrite, Category: plan.CategoryRestructure,
		Tier: plan.TierAuto, Status: plan.Pending, Note: n.Path, Content: content,
	}}}
	for _, b := range n.Blocks {
		p.Operations[0].Sources = append(p.Operations[0].Sources, plan.Source{Note: n.Path, Block: b.Hash, Lines: [2]int{b.StartLine, b.EndLine}})
	}
	doc := render(t, v, p)
	for _, want := range []string{
		"## Restructures\n\n- category `restructure` status: pending\n\n### restructure-daily\n\nDecided by the category status.\n\n",
		"Rewrites `daily/2026-09-14.md`. Before:\n\n```markdown\n" + string(n.Src) + "```\n\nAfter:\n\n```markdown\n" + content + "```\n",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("review lacks %q:\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "- op `restructure-daily`") {
		t.Error("auto op got its own status line")
	}
}
