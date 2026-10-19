package apply

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gittest"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/gitx"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/scan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/state"
)

var examplePlan = filepath.Join("..", "..", "..", "testdata", "plans", "2026-09-14.json")

const testBranch = "curate/test"

func loadPlan(t *testing.T) *plan.Plan {
	t.Helper()
	p, err := plan.Load(examplePlan)
	if err != nil {
		t.Fatal(err)
	}
	for i := range p.Operations {
		p.Operations[i].Status = plan.Approved
	}
	return p
}

// snapshot returns every file under dir except .git, keyed by slash path.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func head(t *testing.T, r *gitx.Repo) string {
	t.Helper()
	h, err := r.Head()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func run(t *testing.T, r *gitx.Repo, args ...string) string {
	t.Helper()
	out, err := r.Run(args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// requireNoTrace checks that a failed apply left no branch, worktree or
// temp folder, and the main tree as it was.
func requireNoTrace(t *testing.T, r *gitx.Repo, root string, before map[string]string, base string) {
	t.Helper()
	if _, ok, _ := r.BranchCommit(testBranch); ok {
		t.Errorf("branch %s left behind", testBranch)
	}
	if n := strings.Count(run(t, r, "worktree", "list", "--porcelain"), "worktree "); n != 1 {
		t.Errorf("%d worktrees, want only the main one", n)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("worktree root not empty: %v", entries)
	}
	if !reflect.DeepEqual(snapshot(t, r.Dir), before) {
		t.Errorf("main tree changed")
	}
	if h := head(t, r); h != base {
		t.Errorf("HEAD moved from %s to %s", base, h)
	}
}

func TestApplyExamplePlan(t *testing.T) {
	r := gittest.Vault(t)
	before := snapshot(t, r.Dir)
	base := head(t, r)
	p := loadPlan(t)
	root := t.TempDir()

	res, err := Apply(r.Dir, p, Options{Branch: testBranch, WorktreeRoot: root})
	if err != nil {
		t.Fatal(err)
	}

	// The main tree is untouched.
	if !reflect.DeepEqual(snapshot(t, r.Dir), before) {
		t.Error("main tree changed")
	}
	if h := head(t, r); h != base {
		t.Errorf("HEAD moved to %s", h)
	}
	if clean, _ := r.IsClean(); !clean {
		t.Error("main tree not clean")
	}

	// The branch holds one commit on top of HEAD, and the worktree is
	// outside the repo.
	if tip, ok, _ := r.BranchCommit(testBranch); !ok || tip != res.Commit {
		t.Errorf("branch tip = %s, %v; want %s", tip, ok, res.Commit)
	}
	if parent := run(t, r, "rev-parse", res.Commit+"^"); parent != base || res.Base != base {
		t.Errorf("parent = %s, base = %s; want %s", parent, res.Base, base)
	}
	if !within(res.Worktree, root) || within(res.Worktree, r.Dir) {
		t.Errorf("worktree %s not under %s", res.Worktree, root)
	}
	if res.VaultDir != res.Worktree {
		t.Errorf("vault dir = %s, want %s", res.VaultDir, res.Worktree)
	}

	// The worktree holds the plan's results, committed.
	want := map[string]string{}
	for k, v := range before {
		want[k] = v
	}
	if len(res.Changes) != 7 {
		t.Errorf("%d changes, want 7", len(res.Changes))
	}
	for _, c := range res.Changes {
		want[c.Path] = string(c.After)
	}
	got := snapshot(t, res.Worktree)
	delete(got, ".git") // the worktree's .git is a file
	stateJSON := got[state.Path]
	delete(got, state.Path)
	if !reflect.DeepEqual(got, want) {
		for k := range want {
			if got[k] != want[k] {
				t.Errorf("%s differs", k)
			}
		}
		t.Errorf("worktree files = %d, want %d", len(got), len(want))
	}
	if !strings.Contains(stateJSON, `"meetings/2026-09-14 Beacon sync.md"`) {
		t.Errorf("state.json = %s", stateJSON)
	}
	if clean, _ := (&gitx.Repo{Dir: res.Worktree}).IsClean(); !clean {
		t.Error("worktree has uncommitted changes")
	}
	if _, err := os.Stat(filepath.Join(res.Worktree, "meetings", "2026-09-14 Beacon sync.md")); err != nil {
		t.Error(err)
	}

	// Every block of every written note is recorded as processed.
	st, err := state.Load(res.Worktree)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Changes {
		if nb := st.NewBlocks(note.Parse(c.Path, c.After)); len(nb) != 0 {
			t.Errorf("%s: %d unprocessed blocks", c.Path, len(nb))
		}
	}

	msg := run(t, r, "log", "-1", "--format=%B", res.Commit)
	if !strings.HasPrefix(msg, "curate: apply plan to 7 notes\n\n") ||
		!strings.Contains(msg, "- create meetings/2026-09-14 Beacon sync.md (split-beacon)") {
		t.Errorf("commit message = %q", msg)
	}
}

func TestApplyDefaultBranchName(t *testing.T) {
	r := gittest.Vault(t)
	res, err := Apply(r.Dir, loadPlan(t), Options{WorktreeRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Branch, BranchPrefix) || len(res.Branch) != len("curate/20261018-142501") {
		t.Errorf("branch = %q", res.Branch)
	}
	if filepath.Base(res.Worktree) != strings.ReplaceAll(res.Branch, "/", "-") ||
		!strings.HasPrefix(filepath.Base(filepath.Dir(res.Worktree)), worktreePrefix) {
		t.Errorf("worktree = %s", res.Worktree)
	}
}

func TestApplyVaultInSubfolder(t *testing.T) {
	r := gittest.Init(t)
	gittest.CopyDir(t, gittest.FixtureVault, filepath.Join(r.Dir, "notes"))
	gittest.WriteFile(t, r, "README.md", "not a note\n")
	gittest.Commit(t, r, "initial")
	v := &gitx.Repo{Dir: filepath.Join(r.Dir, "notes"), Env: r.Env}

	res, err := Apply(v.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.VaultDir != filepath.Join(res.Worktree, "notes") {
		t.Errorf("vault dir = %s", res.VaultDir)
	}
	if _, err := os.Stat(filepath.Join(res.VaultDir, "meetings", "2026-09-14 Beacon sync.md")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(res.VaultDir, filepath.FromSlash(state.Path))); err != nil {
		t.Error(err)
	}
}

func TestApplyRefusesInvalidPlan(t *testing.T) {
	r := gittest.Vault(t)
	before, base, root := snapshot(t, r.Dir), head(t, r), t.TempDir()
	p := loadPlan(t)
	p.Operations[0].Status = plan.Pending

	_, err := Apply(r.Dir, p, Options{Branch: testBranch, WorktreeRoot: root})
	var inv *InvalidPlanError
	if !errors.As(err, &inv) || len(inv.Errors) == 0 {
		t.Fatalf("err = %v, want InvalidPlanError", err)
	}
	requireNoTrace(t, r, root, before, base)
}

func TestApplyRefusesNoChanges(t *testing.T) {
	r := gittest.Vault(t)
	p := loadPlan(t)
	for i := range p.Operations {
		p.Operations[i].Status = plan.Rejected
	}
	if _, err := Apply(r.Dir, p, Options{WorktreeRoot: t.TempDir()}); !errors.Is(err, ErrNoChanges) {
		t.Fatalf("err = %v, want ErrNoChanges", err)
	}
}

// dirty makes modified, staged and untracked changes in the vault.
func dirty(t *testing.T, r *gitx.Repo) {
	t.Helper()
	gittest.AppendFile(t, r, "Unsorted.md", "- more\n")
	gittest.AppendFile(t, r, "daily/2026-09-18.md", "- staged entry\n")
	run(t, r, "add", "daily/2026-09-18.md")
	gittest.WriteFile(t, r, "New note.md", "- new\n")
}

func TestApplySnapshotsDirtyVault(t *testing.T) {
	r := gittest.Vault(t)
	dirty(t, r)
	before, old := snapshot(t, r.Dir), head(t, r)

	res, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	// One snapshot commit on main holds the uncommitted changes, and the
	// branch starts from it.
	if res.Snapshot == "" || res.Snapshot != head(t, r) || res.Base != res.Snapshot {
		t.Errorf("snapshot = %q, base = %q, HEAD = %q", res.Snapshot, res.Base, head(t, r))
	}
	if parent := run(t, r, "rev-parse", res.Snapshot+"^"); parent != old {
		t.Errorf("snapshot parent = %s, want %s", parent, old)
	}
	if msg := run(t, r, "log", "-1", "--format=%s", res.Snapshot); msg != SnapshotMessage {
		t.Errorf("snapshot message = %q", msg)
	}
	files := run(t, r, "show", "--name-only", "--format=", res.Snapshot)
	if files != "New note.md\nUnsorted.md\ndaily/2026-09-18.md" {
		t.Errorf("snapshot files = %q", files)
	}
	if clean, _ := r.IsClean(); !clean {
		t.Error("main tree not clean after snapshot")
	}
	if !reflect.DeepEqual(snapshot(t, r.Dir), before) {
		t.Error("main tree files changed")
	}

	// The worktree has the uncommitted edits plus the plan.
	got := snapshot(t, res.VaultDir)
	if !strings.HasSuffix(got["Unsorted.md"], "- more\n") || got["New note.md"] != "- new\n" ||
		!strings.Contains(got["daily/2026-09-18.md"], "- staged entry\n") {
		t.Error("worktree misses the snapshotted edits")
	}
	if _, ok := got["meetings/2026-09-14 Beacon sync.md"]; !ok {
		t.Error("worktree misses the plan's new note")
	}
}

func TestApplyCleanVaultMakesNoSnapshot(t *testing.T) {
	r := gittest.Vault(t)
	base := head(t, r)
	res, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Snapshot != "" || head(t, r) != base {
		t.Errorf("snapshot = %q, HEAD moved = %v", res.Snapshot, head(t, r) != base)
	}
}

// subfolderVault commits the synthetic vault in notes/ of a larger repo
// and returns the repo and the vault.
func subfolderVault(t *testing.T) (repo, v *gitx.Repo) {
	t.Helper()
	r := gittest.Init(t)
	gittest.CopyDir(t, gittest.FixtureVault, filepath.Join(r.Dir, "notes"))
	gittest.WriteFile(t, r, "README.md", "not a note\n")
	gittest.Commit(t, r, "initial")
	return r, &gitx.Repo{Dir: filepath.Join(r.Dir, "notes"), Env: r.Env}
}

func TestApplySnapshotLeavesOutsideChanges(t *testing.T) {
	r, v := subfolderVault(t)
	gittest.AppendFile(t, v, "Unsorted.md", "- more\n")
	gittest.AppendFile(t, r, "README.md", "unstaged outside\n")
	gittest.WriteFile(t, r, "outside.txt", "untracked outside\n")

	res, err := Apply(v.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if files := run(t, r, "show", "--name-only", "--format=", res.Snapshot); files != "notes/Unsorted.md" {
		t.Errorf("snapshot files = %q", files)
	}
	if status := run(t, r, "status", "--porcelain"); status != "M README.md\n?? outside.txt" {
		t.Errorf("status = %q", status)
	}
}

func TestApplyRefusesStagedOutsideVault(t *testing.T) {
	r, v := subfolderVault(t)
	gittest.AppendFile(t, v, "Unsorted.md", "- more\n")
	gittest.AppendFile(t, r, "README.md", "staged outside\n")
	run(t, r, "add", "README.md")
	before, base, root := snapshot(t, r.Dir), head(t, r), t.TempDir()

	_, err := Apply(v.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: root})
	if !errors.Is(err, gitx.ErrStagedOutside) || !strings.Contains(err.Error(), "README.md") {
		t.Fatalf("err = %v, want ErrStagedOutside naming README.md", err)
	}
	requireNoTrace(t, r, root, before, base)
	if status := run(t, r, "status", "--porcelain"); status != "M  README.md\n M notes/Unsorted.md" {
		t.Errorf("status = %q", status)
	}
}

func TestApplyInvalidPlanLeavesDirtyVault(t *testing.T) {
	r := gittest.Vault(t)
	dirty(t, r)
	before, base, root := snapshot(t, r.Dir), head(t, r), t.TempDir()
	status := run(t, r, "status", "--porcelain")
	p := loadPlan(t)
	p.Operations[0].Status = plan.Pending

	var inv *InvalidPlanError
	if _, err := Apply(r.Dir, p, Options{Branch: testBranch, WorktreeRoot: root}); !errors.As(err, &inv) {
		t.Fatalf("err = %v, want InvalidPlanError", err)
	}
	requireNoTrace(t, r, root, before, base)
	if got := run(t, r, "status", "--porcelain"); got != status {
		t.Errorf("status = %q, want %q", got, status)
	}
}

func TestApplyFailingSnapshotHook(t *testing.T) {
	r := gittest.Vault(t)
	dirty(t, r)
	hook := filepath.Join(r.Dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho hook says no >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	before, base, root := snapshot(t, r.Dir), head(t, r), t.TempDir()
	status := run(t, r, "status", "--porcelain")
	_, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: root})
	if err == nil || !strings.Contains(err.Error(), "snapshot commit") || !strings.Contains(err.Error(), "hook says no") {
		t.Fatalf("err = %v", err)
	}
	requireNoTrace(t, r, root, before, base)
	if got := run(t, r, "status", "--porcelain"); got != status {
		t.Errorf("status = %q, want %q (index not restored)", got, status)
	}
}

func TestApplyRefusesExistingBranch(t *testing.T) {
	r := gittest.Vault(t)
	run(t, r, "branch", testBranch)
	_, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyRefusesWorktreeInsideRepo(t *testing.T) {
	r := gittest.Vault(t)
	_, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: filepath.Join(r.Dir, "daily")})
	if err == nil || !strings.Contains(err.Error(), "inside the repo") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyFailureMidwayLeavesNoTrace(t *testing.T) {
	r := gittest.Vault(t)
	before, base, root := snapshot(t, r.Dir), head(t, r), t.TempDir()
	injected := errors.New("injected failure")
	writes := 0
	afterWrite = func(i int, _ string) error {
		writes++
		if i == 2 {
			return injected
		}
		return nil
	}
	t.Cleanup(func() { afterWrite = nil })

	_, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: root})
	if !errors.Is(err, injected) {
		t.Fatalf("err = %v, want injected failure", err)
	}
	if writes != 3 {
		t.Errorf("failed after %d writes, want 3", writes)
	}
	requireNoTrace(t, r, root, before, base)
}

func TestApplyFailingCommitHookLeavesNoTrace(t *testing.T) {
	r := gittest.Vault(t)
	hook := filepath.Join(r.Dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho hook says no >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	before, base, root := snapshot(t, r.Dir), head(t, r), t.TempDir()

	_, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: root})
	if err == nil || !strings.Contains(err.Error(), "hook says no") {
		t.Fatalf("err = %v, want the hook's failure", err)
	}
	requireNoTrace(t, r, root, before, base)
}

// applyAndMerge applies the example plan and merges its branch into main
// with a merge commit.
func applyAndMerge(t *testing.T, r *gitx.Repo) *Result {
	t.Helper()
	res, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	run(t, r, "merge", "--no-ff", "--quiet", "-m", "merge curation", testBranch)
	return res
}

func TestWatermarkAfterMerge(t *testing.T) {
	r := gittest.Vault(t)
	res := applyAndMerge(t, r)

	wm, err := Watermark(r.Dir, WatermarkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if wm.Commit != res.Commit || wm.Branch != testBranch || wm.Tag != gitx.WatermarkTag {
		t.Errorf("watermark = %+v, want commit %s", wm, res.Commit)
	}
	if tag, ok, _ := r.TagCommit(gitx.WatermarkTag); !ok || tag != res.Commit {
		t.Errorf("tag = %s, %v; want %s", tag, ok, res.Commit)
	}
	if _, ok, _ := r.BranchCommit(testBranch); ok {
		t.Error("branch not deleted")
	}
	if _, err := os.Stat(res.Worktree); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("worktree still there: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(res.Worktree)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("worktree folder still there: %v", err)
	}
	if n := strings.Count(run(t, r, "worktree", "list", "--porcelain"), "worktree "); n != 1 {
		t.Errorf("%d worktrees, want 1", n)
	}

	// A fresh scan finds nothing left to curate.
	s, err := scan.Run(r.Dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Candidates) != 0 {
		t.Errorf("candidates after watermark: %+v", s.Candidates)
	}
}

func TestWatermarkKeepsMainSideChanges(t *testing.T) {
	r := gittest.Vault(t)
	if _, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	// A scheduled sync commits a note edited while the branch waits.
	gittest.AppendFile(t, r, "daily/2026-09-18.md", "- late entry\n")
	gittest.Commit(t, r, "sync")
	run(t, r, "merge", "--no-ff", "--quiet", "-m", "merge curation", testBranch)

	if _, err := Watermark(r.Dir, WatermarkOptions{}); err != nil {
		t.Fatal(err)
	}
	s, err := scan.Run(r.Dir, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, c := range s.Candidates {
		paths = append(paths, c.Path)
	}
	if !reflect.DeepEqual(paths, []string{"daily/2026-09-18.md"}) {
		t.Errorf("candidates = %v, want only the sync edit", paths)
	}
}

func TestWatermarkRefusesUnmerged(t *testing.T) {
	r := gittest.Vault(t)
	res, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, opts := range []WatermarkOptions{{}, {Branch: testBranch}} {
		if _, err := Watermark(r.Dir, opts); !errors.Is(err, ErrNotMerged) {
			t.Errorf("%+v: err = %v, want ErrNotMerged", opts, err)
		}
	}
	if _, ok, _ := r.TagCommit(gitx.WatermarkTag); ok {
		t.Error("tag set")
	}
	if _, ok, _ := r.BranchCommit(testBranch); !ok {
		t.Error("branch deleted")
	}
	if _, err := os.Stat(res.Worktree); err != nil {
		t.Errorf("worktree removed: %v", err)
	}
}

func TestWatermarkRefusesNoBranch(t *testing.T) {
	r := gittest.Vault(t)
	if _, err := Watermark(r.Dir, WatermarkOptions{}); !errors.Is(err, ErrNoBranch) {
		t.Errorf("err = %v, want ErrNoBranch", err)
	}
}

func TestWatermarkRefusesFromWorktree(t *testing.T) {
	r := gittest.Vault(t)
	res, err := Apply(r.Dir, loadPlan(t), Options{Branch: testBranch, WorktreeRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	// From inside the worktree the branch is trivially "merged" into HEAD.
	if _, err := Watermark(res.VaultDir, WatermarkOptions{}); err == nil || !strings.Contains(err.Error(), "checked out here") {
		t.Errorf("err = %v", err)
	}
	if _, ok, _ := r.TagCommit(gitx.WatermarkTag); ok {
		t.Error("tag set")
	}
}

func TestWatermarkRefusesDirtyWorktree(t *testing.T) {
	r := gittest.Vault(t)
	res := applyAndMerge(t, r)
	gittest.AppendFile(t, &gitx.Repo{Dir: res.VaultDir}, "Unsorted.md", "- edited in the worktree\n")
	if _, err := Watermark(r.Dir, WatermarkOptions{}); err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("err = %v", err)
	}
	if _, ok, _ := r.TagCommit(gitx.WatermarkTag); ok {
		t.Error("tag set")
	}
}

func TestWatermarkWorktreeDeletedByHand(t *testing.T) {
	r := gittest.Vault(t)
	res := applyAndMerge(t, r)
	if err := os.RemoveAll(filepath.Dir(res.Worktree)); err != nil {
		t.Fatal(err)
	}
	wm, err := Watermark(r.Dir, WatermarkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if wm.Worktree != "" {
		t.Errorf("worktree = %q, want empty", wm.Worktree)
	}
	if _, ok, _ := r.BranchCommit(testBranch); ok {
		t.Error("branch not deleted")
	}
}

// ignoredPlan is the example plan with its first split moved into private/,
// which the vault's committed .gitignore ignores.
func ignoredPlan(t *testing.T, r *gitx.Repo) *plan.Plan {
	t.Helper()
	gittest.WriteFile(t, r, ".gitignore", "private/\n")
	gittest.Commit(t, r, "ignore private")
	p := loadPlan(t)
	p.Op("split-standup").Path = "private/Harbor standup.md"
	return p
}

func TestApplyRefusesSplitIntoIgnoredPath(t *testing.T) {
	r := gittest.Vault(t)
	p := ignoredPlan(t, r)
	before, base, root := snapshot(t, r.Dir), head(t, r), t.TempDir()

	_, err := Apply(r.Dir, p, Options{Branch: testBranch, WorktreeRoot: root})
	var inv *InvalidPlanError
	if !errors.As(err, &inv) {
		t.Fatalf("err = %v, want InvalidPlanError", err)
	}
	if !strings.Contains(inv.Errors[0].Error(), "split-standup") || !strings.Contains(inv.Errors[0].Error(), "private/Harbor standup.md") {
		t.Errorf("error = %v, want op and path named", inv.Errors[0])
	}
	requireNoTrace(t, r, root, before, base)
}

func TestApplyPostCommitCheckCatchesIgnoredPath(t *testing.T) {
	r := gittest.Vault(t)
	p := ignoredPlan(t, r)
	before, base, root := snapshot(t, r.Dir), head(t, r), t.TempDir()
	ignoreChecker = func(*gitx.Repo) func(string) (bool, error) { return nil }
	t.Cleanup(func() { ignoreChecker = func(r *gitx.Repo) func(string) (bool, error) { return r.IsIgnored } })

	_, err := Apply(r.Dir, p, Options{Branch: testBranch, WorktreeRoot: root})
	if err == nil || !strings.Contains(err.Error(), "private/Harbor standup.md") {
		t.Fatalf("err = %v, want missing-from-commit error naming the path", err)
	}
	requireNoTrace(t, r, root, before, base)
}
