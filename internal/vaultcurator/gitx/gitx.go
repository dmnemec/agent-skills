// Package gitx runs the git operations vault-curator needs by shelling out to
// the git binary: the watermark tag, files changed since it, and the
// worktree, branch, commit and tag steps that apply uses.
//
// Every path gitx returns is relative to the Repo's Dir with forward slashes,
// so a vault that lives in a subfolder of a larger repo works the same as one
// at the repo root.
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// WatermarkTag is the tag on the last curation commit. Files changed since it
// are scan candidates. Only curation moves it; the nightly cron commit never
// does.
const WatermarkTag = "vault-curator/watermark"

// ErrNotRepo is returned by Open when the directory is not inside a git work
// tree.
var ErrNotRepo = errors.New("not a git repository")

// ErrNothingToCommit is returned by CommitAll when the work tree has no
// changes.
var ErrNothingToCommit = errors.New("nothing to commit")

// Repo runs git commands in Dir.
type Repo struct {
	// Dir is the directory commands run in: the vault root, which may be
	// the repo's top level or a folder inside it.
	Dir string
	// Env is extra environment for every command, as "KEY=value". Tests use
	// it to pin author and commit dates.
	Env []string
}

// Open returns a Repo for dir, which must be inside a git work tree.
func Open(dir string) (*Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	r := &Repo{Dir: abs}
	out, err := r.Run("rev-parse", "--is-inside-work-tree")
	if err != nil || out != "true" {
		return nil, fmt.Errorf("%s: %w", abs, ErrNotRepo)
	}
	return r, nil
}

// Run runs git with args in r.Dir and returns its stdout with surrounding
// whitespace trimmed. On failure the error includes git's stderr.
func (r *Repo) Run(args ...string) (string, error) {
	out, err := r.run(args...)
	return strings.TrimSpace(out), err
}

func (r *Repo) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), r.Env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// TopLevel returns the absolute path of the repo's top-level directory.
func (r *Repo) TopLevel() (string, error) {
	return r.Run("rev-parse", "--show-toplevel")
}

// Head returns the commit HEAD points at.
func (r *Repo) Head() (string, error) {
	return r.Run("rev-parse", "--verify", "HEAD^{commit}")
}

// CurrentBranch returns the checked-out branch name, or "" on a detached
// HEAD.
func (r *Repo) CurrentBranch() (string, error) {
	out, err := r.Run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if _, herr := r.Head(); herr == nil {
			return "", nil // detached
		}
		return "", err
	}
	return out, nil
}

// TagCommit returns the commit the tag name points at. ok is false, with a
// nil error, when the tag does not exist.
func (r *Repo) TagCommit(name string) (commit string, ok bool, err error) {
	return r.refCommit("refs/tags/" + name)
}

// BranchCommit returns the commit the local branch name points at. ok is
// false, with a nil error, when the branch does not exist.
func (r *Repo) BranchCommit(name string) (commit string, ok bool, err error) {
	return r.refCommit("refs/heads/" + name)
}

func (r *Repo) refCommit(ref string) (string, bool, error) {
	if _, err := r.Run("show-ref", "--verify", "--quiet", ref); err != nil {
		return "", false, nil
	}
	c, err := r.Run("rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", false, err
	}
	return c, true, nil
}

// SetTag points the lightweight tag name at rev, creating it or moving it.
// Apply calls it with WatermarkTag when curation lands.
func (r *Repo) SetTag(name, rev string) error {
	_, err := r.Run("tag", "--force", name, rev)
	return err
}

// ChangedSince returns the files under r.Dir that differ from rev: changes
// committed since rev, uncommitted and staged changes to tracked files, and
// untracked files that are not ignored. Deleted files are left out. Paths
// are relative to r.Dir, sorted.
func (r *Repo) ChangedSince(rev string) ([]string, error) {
	diff, err := r.run("diff", "--name-only", "-z", "--relative", "--no-renames", "--diff-filter=d", rev, "--")
	if err != nil {
		return nil, err
	}
	untracked, err := r.run("ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return splitPaths(diff, untracked), nil
}

// Files returns every tracked and untracked, non-ignored file under r.Dir,
// relative to it, sorted.
func (r *Repo) Files() ([]string, error) {
	out, err := r.run("ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return splitPaths(out), nil
}

// splitPaths merges NUL-separated path lists into one sorted, deduplicated
// list.
func splitPaths(lists ...string) []string {
	seen := map[string]bool{}
	paths := []string{}
	for _, l := range lists {
		for _, p := range strings.Split(l, "\x00") {
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			paths = append(paths, filepath.ToSlash(p))
		}
	}
	sort.Strings(paths)
	return paths
}

// IsClean reports whether the work tree has no staged, unstaged or untracked
// changes.
func (r *Repo) IsClean() (bool, error) {
	out, err := r.Run("status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return false, err
	}
	return out == "", nil
}

// CommitAll stages every change under the repo (git add --all) and commits
// it with msg, returning the new commit. It returns ErrNothingToCommit when
// there is nothing to commit.
func (r *Repo) CommitAll(msg string) (string, error) {
	top, err := r.TopLevel()
	if err != nil {
		return "", err
	}
	if _, err := r.Run("-C", top, "add", "--all"); err != nil {
		return "", err
	}
	if _, err := r.Run("diff", "--cached", "--quiet"); err == nil {
		return "", ErrNothingToCommit
	}
	if _, err := r.Run("commit", "--quiet", "--no-verify", "-m", msg); err != nil {
		return "", err
	}
	return r.Head()
}

// AddWorktree checks out a new branch, starting at base, in a new worktree
// at path, and returns a Repo for the same vault folder inside it. path must
// not exist yet. If r.Dir is a subfolder of the repo, the returned Repo's Dir
// is that subfolder within the worktree.
func (r *Repo) AddWorktree(path, branch, base string) (*Repo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := r.Run("worktree", "add", "--quiet", "-b", branch, abs, base); err != nil {
		return nil, err
	}
	sub, err := r.Run("rev-parse", "--show-prefix")
	if err != nil {
		return nil, err
	}
	return &Repo{Dir: filepath.Join(abs, filepath.FromSlash(sub)), Env: r.Env}, nil
}

// RemoveWorktree removes the worktree at path. force discards uncommitted
// changes in it. The worktree's branch is kept.
func (r *Repo) RemoveWorktree(path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	_, err = r.Run(append(args, abs)...)
	return err
}

// DeleteBranch deletes the local branch name, even if it is not merged.
func (r *Repo) DeleteBranch(name string) error {
	_, err := r.Run("branch", "-D", name)
	return err
}

// IsAncestor reports whether commit a is an ancestor of (or equal to) b.
func (r *Repo) IsAncestor(a, b string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", a, b)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), r.Env...)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %w", a, b, err)
}
