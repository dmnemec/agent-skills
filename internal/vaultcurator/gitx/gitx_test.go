package gitx_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gittest"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/gitx"
)

func TestOpenNotRepo(t *testing.T) {
	if _, err := gitx.Open(t.TempDir()); !errors.Is(err, gitx.ErrNotRepo) {
		t.Fatalf("err = %v, want ErrNotRepo", err)
	}
}

func TestTagAndChangedSince(t *testing.T) {
	r := gittest.Vault(t)
	if _, ok, err := r.TagCommit(gitx.WatermarkTag); err != nil || ok {
		t.Fatalf("TagCommit before tagging: ok = %v, err = %v", ok, err)
	}
	head, err := r.Head()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetTag(gitx.WatermarkTag, "HEAD"); err != nil {
		t.Fatal(err)
	}
	c, ok, err := r.TagCommit(gitx.WatermarkTag)
	if err != nil || !ok || c != head {
		t.Fatalf("TagCommit = %q, %v, %v; want %q", c, ok, err, head)
	}

	// Committed, uncommitted and untracked changes all count; deletions don't.
	gittest.AppendFile(t, r, "daily/2026-09-18.md", "- committed\n")
	gittest.Commit(t, r, "nightly")
	gittest.AppendFile(t, r, "Unsorted.md", "- uncommitted\n")
	gittest.WriteFile(t, r, "new folder/Brand new.md", "- untracked\n")
	if err := os.Remove(filepath.Join(r.Dir, "Coffee machine options.md")); err != nil {
		t.Fatal(err)
	}
	got, err := r.ChangedSince(gitx.WatermarkTag)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Unsorted.md", "daily/2026-09-18.md", "new folder/Brand new.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChangedSince = %q, want %q", got, want)
	}

	// The cron commit does not move the tag.
	if c2, _, _ := r.TagCommit(gitx.WatermarkTag); c2 != head {
		t.Errorf("tag moved to %s", c2)
	}
}

func TestCommitAllRunsHooks(t *testing.T) {
	r := gittest.Vault(t)
	head, _ := r.Head()
	hook := filepath.Join(r.Dir, ".git", "hooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho rejected by hook >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	gittest.AppendFile(t, r, "Unsorted.md", "- curated\n")
	_, err := r.CommitAll("curate")
	if err == nil || !strings.Contains(err.Error(), "rejected by hook") {
		t.Fatalf("CommitAll with failing pre-commit hook: err = %v", err)
	}
	if h, _ := r.Head(); h != head {
		t.Errorf("HEAD moved to %s despite failing hook", h)
	}
}

func TestSubfolderVault(t *testing.T) {
	r := gittest.Init(t)
	gittest.WriteFile(t, r, "notes/a.md", "- a\n")
	gittest.WriteFile(t, r, "other/b.md", "- b\n")
	gittest.Commit(t, r, "init")
	if err := r.SetTag(gitx.WatermarkTag, "HEAD"); err != nil {
		t.Fatal(err)
	}
	gittest.AppendFile(t, r, "notes/a.md", "- more\n")
	gittest.AppendFile(t, r, "other/b.md", "- more\n")
	gittest.WriteFile(t, r, "notes/sub/c.md", "- c\n")

	sub, err := gitx.Open(filepath.Join(r.Dir, "notes"))
	if err != nil {
		t.Fatal(err)
	}
	sub.Env = r.Env
	got, err := sub.ChangedSince(gitx.WatermarkTag)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.md", "sub/c.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ChangedSince = %q, want %q", got, want)
	}
	files, err := sub.Files()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.md", "sub/c.md"}; !reflect.DeepEqual(files, want) {
		t.Errorf("Files = %q, want %q", files, want)
	}
}

func TestWorktreeCommitTag(t *testing.T) {
	r := gittest.Vault(t)
	base, _ := r.Head()
	if clean, err := r.IsClean(); err != nil || !clean {
		t.Fatalf("IsClean = %v, %v", clean, err)
	}
	if _, err := r.CommitAll("empty"); !errors.Is(err, gitx.ErrNothingToCommit) {
		t.Fatalf("CommitAll with no changes: err = %v", err)
	}

	wtPath := filepath.Join(t.TempDir(), "wt")
	wt, err := r.AddWorktree(wtPath, "vault-curator/run-1", base)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := wt.CurrentBranch(); err != nil || b != "vault-curator/run-1" {
		t.Fatalf("CurrentBranch = %q, %v", b, err)
	}
	gittest.AppendFile(t, wt, "Unsorted.md", "- curated\n")
	c, err := wt.CommitAll("curate")
	if err != nil {
		t.Fatal(err)
	}
	if bc, ok, err := r.BranchCommit("vault-curator/run-1"); err != nil || !ok || bc != c {
		t.Fatalf("BranchCommit = %q, %v, %v; want %q", bc, ok, err, c)
	}
	if anc, err := r.IsAncestor(base, c); err != nil || !anc {
		t.Errorf("IsAncestor(base, c) = %v, %v", anc, err)
	}
	if anc, err := r.IsAncestor(c, base); err != nil || anc {
		t.Errorf("IsAncestor(c, base) = %v, %v", anc, err)
	}
	if err := r.SetTag(gitx.WatermarkTag, c); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveWorktree(wtPath, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree still exists: %v", err)
	}
	// The main tree is untouched by the worktree commit.
	if clean, _ := r.IsClean(); !clean {
		t.Error("main tree not clean")
	}
	if err := r.DeleteBranch("vault-curator/run-1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.BranchCommit("vault-curator/run-1"); ok {
		t.Error("branch still exists")
	}
}
