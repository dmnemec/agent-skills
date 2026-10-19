package apply

import (
	"errors"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gittest"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/review"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// reviewedPlan takes the example plan through review and sync, approving
// everything, as the curate skill does.
func reviewedPlan(t *testing.T, dir string) *plan.Plan {
	t.Helper()
	v, err := vault.Load(dir, vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := plan.Load(examplePlan)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := review.Render(v, p, review.Options{PlanPath: "plan.json", ReviewPath: "plan.review.md"})
	if err != nil {
		t.Fatal(err)
	}
	if err := review.Sync([]byte(strings.ReplaceAll(string(doc), "status: pending", "status: approved")), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestApplyRequiresReviewStamp(t *testing.T) {
	r := gittest.Vault(t)
	opts := Options{Branch: testBranch, WorktreeRoot: t.TempDir()}

	// Approved by hand: valid, but never reviewed.
	if _, err := Apply(r.Dir, loadPlan(t), opts); !errors.Is(err, review.ErrNotReviewed) ||
		!strings.Contains(err.Error(), "review --sync") {
		t.Fatalf("hand-approved plan: err = %v", err)
	}
	if branches, _ := r.Run("branch", "--list", "curate/*"); branches != "" {
		t.Errorf("branches left: %s", branches)
	}

	// A status edited after the sync breaks the stamp.
	p := reviewedPlan(t, r.Dir)
	p.Op("links-coffee").Status = plan.Rejected
	if _, err := Apply(r.Dir, p, opts); !errors.Is(err, review.ErrNotReviewed) {
		t.Fatalf("edited status: err = %v", err)
	}

	// Reviewed and synced: applies.
	if _, err := Apply(r.Dir, reviewedPlan(t, r.Dir), opts); err != nil {
		t.Fatalf("reviewed plan: %v", err)
	}
}
