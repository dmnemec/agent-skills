package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	testVault = filepath.Join("..", "..", "testdata", "vault")
	testPlan  = filepath.Join("..", "..", "testdata", "plans", "2026-09-14.json")
)

func TestValidateExamplePlan(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"validate", "--vault", testVault, testPlan}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "plan OK: 7 notes to change\n") {
		t.Errorf("stdout = %q", out.String())
	}
	if !strings.Contains(out.String(), "  create meetings/2026-09-14 Beacon sync.md (split-beacon)\n") ||
		!strings.Contains(out.String(), "  update daily/2026-09-14.md (split-standup, split-retry, split-beacon, split-teodor, fm-daily)\n") {
		t.Errorf("stdout = %q", out.String())
	}
}

// writePlan writes a copy of the example plan with old replaced by new.
func writePlan(t *testing.T, old, new string) string {
	t.Helper()
	data, err := os.ReadFile(testPlan)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(old)) {
		t.Fatalf("example plan has no %q", old)
	}
	p := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(p, bytes.Replace(data, []byte(old), []byte(new), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestValidateReportsErrors(t *testing.T) {
	p := writePlan(t, `cap at 5, jitter`, `cap at five, jitter`)
	var out, errOut bytes.Buffer
	if code := run([]string{"validate", p, "--vault", testVault}, &out, &errOut); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	want := `op split-retry: daily/2026-09-14.md:7: bullet not found in meetings/2026-09-14 Tidegate retry design.md (dropped or reworded): "- proposal: exp backoff, cap at 5, jitter"`
	if !strings.Contains(errOut.String(), want) {
		t.Errorf("stderr = %s\nwant line %s", errOut.String(), want)
	}
	if !strings.Contains(errOut.String(), "plan invalid: 2 errors\n") {
		t.Errorf("stderr = %s", errOut.String())
	}
}

func TestValidateJSON(t *testing.T) {
	p := writePlan(t, `"status": "approved",
      "summary": "Split the Beacon sync`, `"status": "pending",
      "summary": "Split the Beacon sync`)
	var out, errOut bytes.Buffer
	if code := run([]string{"validate", "--json", "--vault", testVault, p}, &out, &errOut); code != 1 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	var got struct {
		OK     bool `json:"ok"`
		Errors []struct {
			Op      string `json:"op"`
			Message string `json:"message"`
		} `json:"errors"`
		Changes []any `json:"changes"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if got.OK || len(got.Errors) != 1 || got.Errors[0].Op != "split-beacon" || len(got.Changes) != 0 {
		t.Errorf("got %+v", got)
	}

	out.Reset()
	if code := run([]string{"validate", "--json", "--draft", "--vault", testVault, p}, &out, &errOut); code != 0 {
		t.Fatalf("--draft exit code = %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"ok": true`) || !strings.Contains(out.String(), `"created": true`) {
		t.Errorf("stdout = %s", out.String())
	}
}

func TestValidateUsage(t *testing.T) {
	for _, args := range [][]string{
		{"validate"},
		{"validate", testPlan},
		{"validate", "--vault", testVault},
		{"validate", "--vault", testVault, testPlan, testPlan},
		{"validate", "--bogus"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 2 {
			t.Errorf("%v: exit code = %d, want 2", args, code)
		}
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"validate", "--vault", testVault, "missing.json"}, &out, &errOut); code != 1 {
		t.Errorf("missing plan: exit code = %d, want 1", code)
	}
}
