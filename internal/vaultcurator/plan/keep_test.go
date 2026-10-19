package plan

import (
	"strings"
	"testing"
)

func keepOp(id, notePath string) Operation {
	return Operation{ID: id, Op: Keep, Category: CategoryKeep, Tier: TierAuto, Status: Approved, Note: notePath}
}

func TestKeep(t *testing.T) {
	v := smallVault()
	run := func(ops ...Operation) *Result {
		return Validate(v, &Plan{Version: 1, Operations: ops}, Options{})
	}

	t.Run("keep-only plan lists kept notes and changes nothing", func(t *testing.T) {
		r := run(keepOp("k2", "other.md"), keepOp("k1", "Sync.md"))
		requireOK(t, r)
		if len(r.Changes) != 0 {
			t.Errorf("changes = %v, want none", r.Changes)
		}
		if got := strings.Join(r.Kept, ","); got != "Other.md,Sync.md" {
			t.Errorf("kept = %q, want the notes' own paths sorted", got)
		}
	})
	t.Run("missing note", func(t *testing.T) {
		requireErr(t, run(keepOp("k", "Nope.md")).Errors, "k", "no note Nope.md")
	})
	t.Run("rejected keep keeps nothing", func(t *testing.T) {
		k := keepOp("k", "Sync.md")
		k.Status = Rejected
		r := run(k)
		requireOK(t, r)
		if len(r.Kept) != 0 {
			t.Errorf("kept = %v", r.Kept)
		}
	})
	t.Run("duplicate keep", func(t *testing.T) {
		r := run(keepOp("k1", "Sync.md"), keepOp("k2", "sync.md"))
		requireErr(t, r.Errors, "k2", "also kept by k1")
		if r.Kept != nil {
			t.Errorf("kept = %v on a failed plan", r.Kept)
		}
	})
	t.Run("keep and another op on one note", func(t *testing.T) {
		fm := Operation{ID: "fm", Op: SetFrontmatter, Category: CategoryFrontmatter, Tier: TierAuto, Status: Approved,
			Note: "Other.md", Frontmatter: map[string]any{"type": "note"}}
		requireErr(t, run(keepOp("k", "Other.md"), fm).Errors, "k", "also changed by fm")
		requireErr(t, run(fm, keepOp("k", "Other.md")).Errors, "k", "also changed by fm")
		rw := rewriteOp(v, restructured)
		requireErr(t, run(rw, keepOp("k", "Sync.md")).Errors, "k", "also changed by rw")
	})
	t.Run("rejected op does not conflict", func(t *testing.T) {
		fm := Operation{ID: "fm", Op: SetFrontmatter, Category: CategoryFrontmatter, Tier: TierAuto, Status: Rejected,
			Note: "Other.md", Frontmatter: map[string]any{"type": "note"}}
		requireOK(t, run(fm, keepOp("k", "Other.md")))
	})
	t.Run("shape", func(t *testing.T) {
		k := keepOp("k", "Other.md")
		k.Category = CategoryLinks
		requireErr(t, run(k).Errors, "k", "category")
		k = keepOp("k", "Other.md")
		k.Content = "x"
		requireErr(t, run(k).Errors, "k", `does not take "content"`)
		k = keepOp("k", "")
		requireErr(t, run(k).Errors, "k", `needs "note"`)
	})
}
