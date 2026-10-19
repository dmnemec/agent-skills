package review

import (
	"errors"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
)

// syncedExample renders the pending example, approves everything and syncs.
func syncedExample(t *testing.T) *plan.Plan {
	t.Helper()
	v, p := pendingExample(t)
	doc := strings.ReplaceAll(render(t, v, p), "status: pending", "status: approved")
	if err := Sync([]byte(doc), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSyncStampsPlan(t *testing.T) {
	_, p := pendingExample(t)
	if err := Verify(p); !errors.Is(err, ErrNotReviewed) || !strings.Contains(err.Error(), "no review stamp") {
		t.Errorf("unsynced plan: Verify = %v", err)
	}
	p = syncedExample(t)
	if p.Review == nil || len(p.Review.PlanSHA256) != 64 || len(p.Review.StatusesSHA256) != 64 {
		t.Fatalf("stamp = %+v", p.Review)
	}
	if hash, _ := Hash(p); p.Review.PlanSHA256 != hash {
		t.Error("stamp plan hash is not the review hash")
	}
	if err := Verify(p); err != nil {
		t.Errorf("synced plan: Verify = %v", err)
	}

	// The stamp survives the plan file round trip.
	data, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	q, err := plan.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(q); err != nil {
		t.Errorf("after round trip: Verify = %v", err)
	}
}

func TestVerifyDetectsEdits(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(p *plan.Plan)
		want string
	}{
		{"status", func(p *plan.Plan) { p.Op("split-beacon").Status = plan.Rejected }, "status changed after the review was synced"},
		{"content", func(p *plan.Plan) { p.Op("split-beacon").Summary = "other" }, "plan changed after its review"},
		{"removed stamp", func(p *plan.Plan) { p.Review = nil }, "no review stamp"},
		{"forged stamp", func(p *plan.Plan) { p.Review.StatusesSHA256 = strings.Repeat("0", 64) }, "status changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := syncedExample(t)
			tc.edit(p)
			err := Verify(p)
			if !errors.Is(err, ErrNotReviewed) || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "review --sync") {
				t.Errorf("Verify = %v, want %q", err, tc.want)
			}
		})
	}
}
