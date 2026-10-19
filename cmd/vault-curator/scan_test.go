package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gittest"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/gitx"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/scan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/state"
)

var update = flag.Bool("update", false, "rewrite golden files")

const goldenScan = "testdata/scan.golden.json"

// scanFixture is the synthetic vault after one curation (every block
// processed, watermark tagged) and a later nightly commit that appended a
// block to a daily note.
func scanFixture(t *testing.T) *gitx.Repo {
	t.Helper()
	r := gittest.Vault(t)
	res, err := scan.Run(r.Dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	st := state.New()
	for _, n := range res.Vault.Notes {
		st.MarkNote(n)
	}
	if err := st.Save(r.Dir); err != nil {
		t.Fatal(err)
	}
	if err := r.SetTag(gitx.WatermarkTag, gittest.Commit(t, r, "curate")); err != nil {
		t.Fatal(err)
	}
	gittest.AppendFile(t, r, "daily/2026-09-18.md", "\n- coffee w/ Lio\n    - beacon webhook contract draft is up, see [[Beacon]]\n")
	gittest.Commit(t, r, "nightly")
	return r
}

func TestScanJSONGolden(t *testing.T) {
	r := scanFixture(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"scan", "--vault", r.Dir, "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	// Dates and identities are pinned, but keep the golden file independent
	// of commit hashes anyway.
	commit, _, _ := r.TagCommit(gitx.WatermarkTag)
	got := strings.ReplaceAll(out.String(), commit, "<commit>")

	var decoded scan.Result
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("output is not valid scan JSON: %v", err)
	}
	if *update {
		if err := os.WriteFile(goldenScan, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenScan)
	if err != nil {
		t.Fatalf("%v (run go test ./cmd/vault-curator -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("scan --json differs from %s (rerun with -update if intended):\n%s", goldenScan, got)
	}
}

func TestScanText(t *testing.T) {
	r := scanFixture(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"scan", "--vault", r.Dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	for _, want := range []string{"1 candidate notes, 1 new blocks", "daily/2026-09-18.md: 1 new, 1 processed", "lines 1-4", "processed\n", "lines 6-7", "new\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestScanAllLimitFlags(t *testing.T) {
	r := scanFixture(t)
	var out, errOut bytes.Buffer
	// Every block is processed in the fixture except the appended one.
	if code := run([]string{"scan", "--vault", r.Dir, "--json", "--all", "--limit", "2"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	var res scan.Result
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].Path != "daily/2026-09-18.md" || res.Remaining != 0 {
		t.Errorf("--all: candidates %+v, remaining %d", res.Candidates, res.Remaining)
	}
	if code := run([]string{"scan", "--vault", r.Dir, "--limit", "-1"}, &out, &errOut); code != 2 {
		t.Errorf("negative --limit: exit code = %d, want 2", code)
	}
}

func TestScanErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"scan", "--vault", t.TempDir()}, &out, &errOut); code != 1 {
		t.Errorf("not a repo: exit code = %d, want 1", code)
	}
	if code := run([]string{"scan", "--bogus"}, &out, &errOut); code != 2 {
		t.Errorf("bad flag: exit code = %d, want 2", code)
	}
	if code := run([]string{"scan", "extra"}, &out, &errOut); code != 2 {
		t.Errorf("extra arg: exit code = %d, want 2", code)
	}
}
