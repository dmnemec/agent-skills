package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gittest"
)

// expectedSplit is testdata/expected/<date>.json.
type expectedSplit struct {
	Note     string `json:"note"`
	Date     string `json:"date"`
	Segments []struct {
		Title   string   `json:"title"`
		Primary []string `json:"primary"`
	} `json:"segments"`
}

// cli runs one command and fails the test on a non-zero exit.
func cli(t *testing.T, args ...string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := run(args, &out, &errOut); code != 0 {
		t.Fatalf("vault-curator %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, out.String(), errOut.String())
	}
	return out.String()
}

// scanCandidates maps each candidate path to its number of new blocks.
func scanCandidates(t *testing.T, vault string) map[string]int {
	t.Helper()
	var s struct {
		Candidates []struct {
			Path      string            `json:"path"`
			NewBlocks []json.RawMessage `json:"new_blocks"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(cli(t, "scan", "--vault", vault, "--json")), &s); err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, c := range s.Candidates {
		got[c.Path] = len(c.NewBlocks)
	}
	return got
}

func readVault(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestCurateEndToEnd walks the whole loop the curate skill drives: scan,
// plan, review, sync, apply, merge, watermark.
func TestCurateEndToEnd(t *testing.T) {
	r := gittest.Vault(t)
	const daily = "daily/2026-09-14.md"

	if n := scanCandidates(t, r.Dir)[daily]; n == 0 {
		t.Fatalf("scan lists no new blocks for %s", daily)
	}

	// The agent's plan, with every status pending, saved where the skill puts it.
	raw, err := os.ReadFile(testPlan)
	if err != nil {
		t.Fatal(err)
	}
	pending := regexp.MustCompile(`"status":\s*"[a-z]+"`).ReplaceAll(raw, []byte(`"status": "pending"`))
	planRel := ".vault-curator/plans/20261019-090000.json"
	reviewRel := ".vault-curator/plans/20261019-090000.review.md"
	gittest.WriteFile(t, r, planRel, string(pending))
	planPath := filepath.Join(r.Dir, filepath.FromSlash(planRel))
	reviewPath := filepath.Join(r.Dir, filepath.FromSlash(reviewRel))

	// Plan files are not notes: scan must not list them.
	if _, ok := scanCandidates(t, r.Dir)[planRel]; ok {
		t.Fatal("scan lists the plan file as a note")
	}

	cli(t, "validate", "--draft", "--vault", r.Dir, planPath)
	cli(t, "review", "--vault", r.Dir, planPath)
	review, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("review not written next to the plan: %v", err)
	}

	// The user approves every category.
	status := regexp.MustCompile("(?m)^(- category `[a-z-]+` status: )pending\r?$")
	approved := status.ReplaceAll(review, []byte("${1}approved"))
	// Approve-each operations are decided one by one.
	opStatus := regexp.MustCompile("(?m)^(- op `[^`]+` status: )pending\r?$")
	approved = opStatus.ReplaceAll(approved, []byte("${1}approved"))
	if bytes.Equal(approved, review) {
		t.Fatal("review has no pending category line")
	}
	if err := os.WriteFile(reviewPath, approved, 0o644); err != nil {
		t.Fatal(err)
	}
	cli(t, "review", "--sync", reviewPath, planPath)
	cli(t, "validate", "--vault", r.Dir, planPath)

	// Apply, merge, watermark.
	var ap struct {
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal([]byte(cli(t, "apply", "--vault", r.Dir, "--json", "--worktree-root", t.TempDir(), planPath)), &ap); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run("merge", "--no-ff", "--quiet", "-m", "merge curation", ap.Branch); err != nil {
		t.Fatal(err)
	}
	cli(t, "watermark", "--vault", r.Dir)

	// The plan and review stay in the vault as an audit trail.
	for _, p := range []string{planRel, reviewRel} {
		if _, err := os.Stat(filepath.Join(r.Dir, filepath.FromSlash(p))); err != nil {
			t.Errorf("audit trail file missing after merge: %v", err)
		}
	}

	// One note per expected segment, linked from the daily note.
	var exp expectedSplit
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "expected", "2026-09-14.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &exp); err != nil {
		t.Fatal(err)
	}
	dailyText := readVault(t, r.Dir, exp.Note)
	for _, seg := range exp.Segments {
		// Obsidian forbids ":" in file names, so "1:1 Teodor" is stored as "1-1 Teodor".
		stem := exp.Date + " " + strings.ReplaceAll(seg.Title, ":", "-")
		note := readVault(t, r.Dir, path.Join("meetings", stem+".md"))
		if !strings.Contains(note, "type: meeting") {
			t.Errorf("%s: not a meeting note:\n%s", stem, note)
		}
		for _, p := range seg.Primary {
			if !strings.Contains(note, `"[[`+p+`]]"`) {
				t.Errorf("%s: primary topic %s missing from frontmatter:\n%s", stem, p, note)
			}
		}
		if !strings.Contains(dailyText, "[["+stem+"]]") && !strings.Contains(dailyText, "[["+stem+"|") {
			t.Errorf("%s does not link to %s:\n%s", exp.Note, stem, dailyText)
		}
	}

	// Nothing is left to curate for the daily note.
	if n, ok := scanCandidates(t, r.Dir)[daily]; ok && n != 0 {
		t.Errorf("fresh scan still reports %d new blocks for %s", n, daily)
	}
}
