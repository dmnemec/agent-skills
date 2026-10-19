package scan_test

import (
	"errors"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gittest"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/gitx"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/scan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/state"
)

const daily = "daily/2026-09-18.md"

// curated returns a fixture repo after a simulated curation: every block
// marked processed, committed, and the watermark tag on that commit.
func curated(t *testing.T) *gitx.Repo {
	t.Helper()
	r := gittest.Vault(t)
	before, err := scan.Run(r.Dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	st := state.New()
	for _, n := range before.Vault.Notes {
		st.MarkNote(n)
	}
	if err := st.Save(r.Dir); err != nil {
		t.Fatal(err)
	}
	c := gittest.Commit(t, r, "curate")
	if err := r.SetTag(gitx.WatermarkTag, c); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNoTagEveryNoteIsCandidate(t *testing.T) {
	r := gittest.Vault(t)
	res, err := scan.Run(r.Dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Watermark.Exists || res.Watermark.Commit != "" || res.Watermark.Tag != gitx.WatermarkTag {
		t.Errorf("Watermark = %+v", res.Watermark)
	}
	if len(res.Candidates) != len(res.Vault.Notes) || len(res.Candidates) != 15 {
		t.Fatalf("%d candidates for %d notes, want 15", len(res.Candidates), len(res.Vault.Notes))
	}
	for i, c := range res.Candidates {
		n := res.Vault.Notes[i]
		if c.Path != n.Path || c.Tracked || c.ProcessedBlocks != 0 || len(c.NewBlocks) != len(n.Blocks) || len(c.Blocks) != len(n.Blocks) {
			t.Errorf("candidate %+v for note %s with %d blocks", c, n.Path, len(n.Blocks))
		}
		for _, b := range c.Blocks {
			if b.Processed {
				t.Errorf("%s: block %+v processed without state", c.Path, b)
			}
		}
	}
}

func TestAppendAfterTagShowsOnlyNewBlock(t *testing.T) {
	r := curated(t)
	res, err := scan.Run(r.Dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Watermark.Exists || len(res.Candidates) != 0 {
		t.Fatalf("right after curation: watermark %+v, candidates %+v", res.Watermark, res.Candidates)
	}

	gittest.AppendFile(t, r, daily, "\n- coffee w/ Lio\n    - beacon webhook contract draft is up\n")
	gittest.Commit(t, r, "nightly") // the cron commit does not move the tag
	res, err = scan.Run(r.Dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("candidates = %+v, want only %s", res.Candidates, daily)
	}
	c := res.Candidates[0]
	if c.Path != daily || !c.Tracked || c.ProcessedBlocks != 1 || len(c.NewBlocks) != 1 {
		t.Fatalf("candidate = %+v", c)
	}
	b := c.NewBlocks[0]
	want := "- coffee w/ Lio\n    - beacon webhook contract draft is up\n"
	if b.Text != want || b.StartLine != 6 || b.EndLine != 7 || len(b.Hash) != 16 {
		t.Errorf("new block = %+v, want lines 6-7 text %q", b, want)
	}

	// Blocks lists the old block too, marked processed.
	if len(c.Blocks) != 2 {
		t.Fatalf("blocks = %+v, want 2", c.Blocks)
	}
	if old := c.Blocks[0]; !old.Processed || old.StartLine != 1 || old.EndLine != 4 || len(old.Hash) != 16 || old.Text == "" {
		t.Errorf("old block = %+v, want processed lines 1-4", old)
	}
	if nb := c.Blocks[1]; nb.Processed || nb != b {
		t.Errorf("new block in blocks = %+v, want %+v", nb, b)
	}
}

// TestRewriteFromScanValidates builds a rewrite of a tracked, appended-to
// daily note from nothing but its scan candidate, and checks the plan passes
// draft validation: every bullet of the note is mapped.
func TestRewriteFromScanValidates(t *testing.T) {
	r := curated(t)
	gittest.AppendFile(t, r, daily, "\n- coffee w/ Lio\n    - beacon webhook contract draft is up\n")
	gittest.Commit(t, r, "nightly")
	res, err := scan.Run(r.Dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("candidates = %+v", res.Candidates)
	}
	c := res.Candidates[0]

	content := "# " + c.Title + "\n"
	var sources []plan.Source
	for _, b := range c.Blocks {
		content += "\n" + b.Text
		sources = append(sources, plan.Source{Note: c.Path, Block: b.Hash, Lines: [2]int{b.StartLine, b.EndLine}})
	}
	p := &plan.Plan{Version: plan.Version, Operations: []plan.Operation{{
		ID: "restructure", Op: plan.Rewrite, Category: plan.CategoryRestructure,
		Tier: plan.TierAuto, Status: plan.Pending,
		Note: c.Path, Content: content, Sources: sources,
	}}}
	if v := plan.Validate(res.Vault, p, plan.Options{Draft: true}); !v.OK() {
		t.Errorf("rewrite from scan output fails validation: %+v", v.Errors)
	}

	// Mapping only the new blocks, as before, leaves old bullets unmapped.
	p.Operations[0].Sources = sources[1:]
	if v := plan.Validate(res.Vault, p, plan.Options{Draft: true}); v.OK() {
		t.Error("rewrite mapping only the new block validates, want unmapped bullets")
	}
}

func TestTagWithoutStateOffersWholeFile(t *testing.T) {
	r := gittest.Vault(t)
	if err := r.SetTag(gitx.WatermarkTag, "HEAD"); err != nil {
		t.Fatal(err)
	}
	gittest.AppendFile(t, r, daily, "\n- later\n")
	gittest.WriteFile(t, r, "assets/photo.png", "png")
	gittest.WriteFile(t, r, ".vault-curator/state.json", `{"version": 1, "files": {}}`)
	res, err := scan.Run(r.Dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].Path != daily {
		t.Fatalf("candidates = %+v, want only %s", res.Candidates, daily)
	}
	if c := res.Candidates[0]; c.Tracked || len(c.NewBlocks) != 2 {
		t.Errorf("candidate = %+v, want untracked with 2 new blocks", c)
	}
}

func TestGraphContext(t *testing.T) {
	res, err := scan.Run(gittest.Vault(t).Dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.BrokenLinks) != 1 || res.BrokenLinks[0].Target != "Tidegate v2 RFC" || res.BrokenLinks[0].From != "daily/2026-09-15.md" {
		t.Errorf("BrokenLinks = %+v", res.BrokenLinks)
	}
	if len(res.Orphans) == 0 || len(res.Entities) == 0 || len(res.Links) != len(res.Vault.Edges) || len(res.Notes) != 15 {
		t.Errorf("orphans %d, entities %d, links %d, notes %d", len(res.Orphans), len(res.Entities), len(res.Links), len(res.Notes))
	}
}

func TestNotARepo(t *testing.T) {
	if _, err := scan.Run(t.TempDir(), scan.Options{}); !errors.Is(err, gitx.ErrNotRepo) {
		t.Errorf("err = %v, want ErrNotRepo", err)
	}
}
