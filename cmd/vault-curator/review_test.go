package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
)

// pendingPlan writes the example plan with every status pending.
func pendingPlan(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(testPlan)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "plan.json")
	data = bytes.ReplaceAll(data, []byte(`"status": "approved"`), []byte(`"status": "pending"`))
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReviewRenderAndSync(t *testing.T) {
	planPath := pendingPlan(t)
	reviewPath := strings.TrimSuffix(planPath, ".json") + ".review.md"

	var out, errOut bytes.Buffer
	if code := run([]string{"review", "--vault", testVault, planPath}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if want := "wrote " + reviewPath + ": 7 operations to review\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	doc, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatal(err)
	}

	// Rendering again would overwrite the user's edits.
	if code := run([]string{"review", "--vault", testVault, planPath}, &out, &errOut); code != 1 ||
		!strings.Contains(errOut.String(), "--force") {
		t.Errorf("second render: exit code %d, stderr %s", code, errOut.String())
	}

	edited := string(doc)
	for _, cat := range []string{"split", "entity", "frontmatter", "links"} {
		edited = strings.Replace(edited, "- category `"+cat+"` status: pending", "- category `"+cat+"` status: approved", 1)
	}
	edited = strings.Replace(edited, "- op `split-beacon` status: pending", "- op `split-beacon` status: rejected", 1)
	if err := os.WriteFile(reviewPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"review", "--sync", reviewPath, planPath}, &out, &errOut); code != 0 {
		t.Fatalf("sync exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), ": 6 approved, 1 rejected, 0 pending\n") {
		t.Errorf("stdout = %q", out.String())
	}
	p, err := plan.Load(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if s := p.Op("split-beacon").Status; s != plan.Rejected {
		t.Errorf("split-beacon status = %q", s)
	}
	if code := run([]string{"validate", "--vault", testVault, planPath}, &out, &errOut); code != 0 {
		t.Errorf("synced plan fails validate: %s", errOut.String())
	}
}

func TestReviewSyncRejectsUnknownOp(t *testing.T) {
	planPath := pendingPlan(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"review", "--vault", testVault, "--out", "-", planPath}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "<!-- vault-curator review plan-sha256=") {
		t.Fatalf("stdout = %s", out.String())
	}
	reviewPath := filepath.Join(t.TempDir(), "review.md")
	doc := out.String() + "\n- op `split-nope` status: approved\n"
	if err := os.WriteFile(reviewPath, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(planPath)
	errOut.Reset()
	if code := run([]string{"review", "--sync", reviewPath, planPath}, &out, &errOut); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "op split-nope is not in the plan") {
		t.Errorf("stderr = %s", errOut.String())
	}
	if after, _ := os.ReadFile(planPath); !bytes.Equal(before, after) {
		t.Error("failed sync changed the plan file")
	}
}

func TestReviewUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"review", testPlan}, &out, &errOut); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}
