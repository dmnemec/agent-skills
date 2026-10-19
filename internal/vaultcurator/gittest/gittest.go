// Package gittest builds throwaway git repos for tests: a copy of the
// synthetic vault in testdata/vault, committed with pinned identities and
// dates. It never touches a real vault.
package gittest

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gitx"
)

// FixtureVault is the absolute path of testdata/vault in this repo.
var FixtureVault = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "vault")
}()

// env pins identity and dates so commits are reproducible.
var env = []string{
	"GIT_AUTHOR_NAME=Vault Curator Test",
	"GIT_AUTHOR_EMAIL=test@example.invalid",
	"GIT_AUTHOR_DATE=2026-09-20T12:00:00Z",
	"GIT_COMMITTER_NAME=Vault Curator Test",
	"GIT_COMMITTER_EMAIL=test@example.invalid",
	"GIT_COMMITTER_DATE=2026-09-20T12:00:00Z",
}

// Init creates an empty git repo in a new temp dir, with user.name and
// user.email set locally, and returns it.
func Init(t *testing.T) *gitx.Repo {
	t.Helper()
	dir := t.TempDir()
	r := &gitx.Repo{Dir: dir, Env: env}
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"config", "user.name", "Vault Curator Test"},
		{"config", "user.email", "test@example.invalid"},
		{"config", "core.autocrlf", "false"},
		{"config", "commit.gpgsign", "false"},
		{"config", "tag.gpgsign", "false"},
	} {
		if _, err := r.Run(args...); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// Vault copies the synthetic vault into a new git repo, commits it, and
// returns the repo. Its Dir is the vault root.
func Vault(t *testing.T) *gitx.Repo {
	t.Helper()
	r := Init(t)
	CopyDir(t, FixtureVault, r.Dir)
	Commit(t, r, "initial vault")
	return r
}

// Commit commits every change in r and returns the commit.
func Commit(t *testing.T, r *gitx.Repo, msg string) string {
	t.Helper()
	c, err := r.CommitAll(msg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// CopyDir copies the files under src into dst, creating folders as needed.
func CopyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// WriteFile writes content to the vault-relative path rel under r.Dir.
func WriteFile(t *testing.T, r *gitx.Repo, rel, content string) {
	t.Helper()
	p := filepath.Join(r.Dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// AppendFile appends text to the vault-relative path rel under r.Dir.
func AppendFile(t *testing.T, r *gitx.Repo, rel, text string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(r.Dir, filepath.FromSlash(rel)), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}
