package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gittest"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/gitx"
)

func TestApplyAndWatermarkJSON(t *testing.T) {
	r := gittest.Vault(t)
	var out, errOut bytes.Buffer
	code := run([]string{"apply", "--vault", r.Dir, "--json", testPlan,
		"--branch", "curate/cli", "--worktree-root", t.TempDir()}, &out, &errOut)
	if code != 0 {
		t.Fatalf("apply exit code = %d, stderr = %s", code, errOut.String())
	}
	var got struct {
		OK       bool   `json:"ok"`
		Branch   string `json:"branch"`
		Worktree string `json:"worktree"`
		Vault    string `json:"vault"`
		Base     string `json:"base"`
		Commit   string `json:"commit"`
		Changes  []struct {
			Path    string   `json:"path"`
			Created bool     `json:"created"`
			Ops     []string `json:"ops"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if !got.OK || got.Branch != "curate/cli" || got.Worktree == "" || got.Vault != got.Worktree ||
		len(got.Base) != 40 || len(got.Commit) != 40 || len(got.Changes) != 7 {
		t.Errorf("apply output = %s", out.String())
	}
	if c := got.Changes[0]; c.Path != "Coffee machine options.md" || c.Created || len(c.Ops) != 1 || c.Ops[0] != "links-coffee" {
		t.Errorf("first change = %+v", c)
	}

	// Before the merge, watermark refuses.
	out.Reset()
	errOut.Reset()
	if code := run([]string{"watermark", "--vault", r.Dir}, &out, &errOut); code != 1 ||
		!strings.Contains(errOut.String(), "not merged into HEAD") {
		t.Errorf("watermark before merge: code = %d, stderr = %s", code, errOut.String())
	}

	if _, err := r.Run("merge", "--no-ff", "--quiet", "-m", "merge curation", "curate/cli"); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"watermark", "--vault", r.Dir, "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("watermark exit code = %d, stderr = %s", code, errOut.String())
	}
	var wm struct {
		Tag, Commit, Branch, Worktree string
	}
	if err := json.Unmarshal(out.Bytes(), &wm); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if wm.Tag != gitx.WatermarkTag || wm.Commit != got.Commit || wm.Branch != "curate/cli" || wm.Worktree != got.Worktree {
		t.Errorf("watermark output = %s", out.String())
	}

	// A fresh scan has nothing left to curate.
	out.Reset()
	if code := run([]string{"scan", "--vault", r.Dir, "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("scan exit code = %d, stderr = %s", code, errOut.String())
	}
	var s struct {
		Candidates []any `json:"candidates"`
	}
	if err := json.Unmarshal(out.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Candidates) != 0 {
		t.Errorf("candidates after watermark: %v", s.Candidates)
	}
}

func TestApplyText(t *testing.T) {
	r := gittest.Vault(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"apply", "--vault", r.Dir, "--branch", "curate/cli",
		"--worktree-root", t.TempDir(), testPlan}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	s := out.String()
	for _, want := range []string{
		"applied 7 notes on branch curate/cli\nworktree: ",
		"  create meetings/2026-09-14 Beacon sync.md (split-beacon)\n",
		"merge curate/cli into your branch, then run: vault-curator watermark --vault ",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("stdout = %s\nwant %q", s, want)
		}
	}
	out.Reset()
	if code := run([]string{"watermark", "--vault", r.Dir}, &out, &errOut); code != 1 {
		t.Errorf("watermark before merge: exit code = %d", code)
	}
}

func TestApplyRefusesInvalidPlan(t *testing.T) {
	r := gittest.Vault(t)
	p := writePlan(t, `cap at 5, jitter`, `cap at five, jitter`)
	var out, errOut bytes.Buffer
	if code := run([]string{"apply", "--vault", r.Dir, "--json", "--worktree-root", t.TempDir(), p}, &out, &errOut); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "op split-retry: ") || !strings.Contains(errOut.String(), "plan invalid: 2 errors\n") {
		t.Errorf("stderr = %s", errOut.String())
	}
	var got struct {
		OK     bool  `json:"ok"`
		Errors []any `json:"errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.OK || len(got.Errors) != 2 {
		t.Errorf("stdout = %s (%v)", out.String(), err)
	}
	if branches, _ := r.Run("branch", "--list", "curate/*"); branches != "" {
		t.Errorf("branches left: %s", branches)
	}
}

func TestApplyRefusesDirtyVault(t *testing.T) {
	r := gittest.Vault(t)
	gittest.AppendFile(t, r, "Unsorted.md", "- uncommitted\n")
	var out, errOut bytes.Buffer
	if code := run([]string{"apply", "--vault", r.Dir, "--worktree-root", t.TempDir(), testPlan}, &out, &errOut); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "commit them before applying") || !strings.Contains(errOut.String(), "Unsorted.md") {
		t.Errorf("stderr = %s", errOut.String())
	}
}

func TestApplyUsage(t *testing.T) {
	for _, args := range [][]string{{"apply"}, {"apply", "--vault", "."}, {"apply", testPlan}, {"watermark", "extra"}} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 2 {
			t.Errorf("%q: exit code = %d, want 2", args, code)
		}
	}
}
