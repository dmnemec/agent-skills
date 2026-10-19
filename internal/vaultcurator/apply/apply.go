// Package apply carries out a validated curation plan in a separate git
// worktree on its own branch, and moves the watermark once the user merges
// that branch (design doc: "Architecture" step 4, and "Git, packaging and
// testing" under Git workflow).
//
// The vault's main working tree is never written: Obsidian and the nightly
// cron push see nothing until the user merges. A failed apply removes its
// worktree and branch, so it leaves no trace.
package apply

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gitx"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/state"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// BranchPrefix starts the name of every curation branch.
const BranchPrefix = "curate/"

// worktreePrefix starts the name of the temp folder holding a worktree.
const worktreePrefix = "vault-curator-"

// SnapshotMessage is the message of the commit Apply makes of the vault's
// uncommitted changes before branching.
const SnapshotMessage = "curate: snapshot before apply"

// ErrNoChanges is returned by Apply when the plan has no approved operation
// that writes a note or keeps one.
var ErrNoChanges = errors.New("the plan changes nothing: no operation is approved")

// InvalidPlanError is returned by Apply when the plan fails validation.
type InvalidPlanError struct {
	Errors []*plan.Error
}

func (e *InvalidPlanError) Error() string {
	noun := "errors"
	if len(e.Errors) == 1 {
		noun = "error"
	}
	return fmt.Sprintf("plan invalid: %d %s", len(e.Errors), noun)
}

// Options configures Apply.
type Options struct {
	// Branch is the new branch's name. Default: BranchPrefix plus the UTC
	// time, as in "curate/20261018-142501".
	Branch string
	// WorktreeRoot is the folder the worktree's temp folder is made in.
	// Default: the OS temp folder. It must be outside the repo.
	WorktreeRoot string
	// Now returns the time for the default branch name. Default: time.Now.
	Now func() time.Time
	// Vault configures loading the vault for validation.
	Vault vault.Options
}

// Result describes a successful apply.
type Result struct {
	// Branch is the curation branch, holding one commit on top of Base.
	Branch string `json:"branch"`
	// Worktree is the absolute path of the worktree's top level.
	Worktree string `json:"worktree"`
	// VaultDir is the vault folder inside the worktree: Worktree itself,
	// or the vault's subfolder when the vault is a folder in its repo.
	VaultDir string `json:"vault"`
	// Snapshot is the commit Apply made of the vault's uncommitted changes
	// on the current branch, or "" when the vault was clean.
	Snapshot string `json:"snapshot,omitempty"`
	// Base is the commit the branch starts from: the vault's HEAD, after
	// the snapshot if there is one.
	Base string `json:"base"`
	// Commit is the curation commit, the branch's tip.
	Commit string `json:"commit"`
	// Changes are the notes written, sorted by path.
	Changes []plan.Change `json:"-"`
	// Kept are the notes kept as is, sorted by path: their blocks are
	// marked processed in the state file, and their content is untouched.
	Kept []string `json:"-"`
}

// afterWrite, when set, runs after each note is written. Tests use it to
// inject a failure midway.
var afterWrite func(i int, path string) error

// Apply validates p against the vault at vaultDir and, if it passes, writes
// every change in a new worktree on a new branch, records the written notes'
// and the kept notes' blocks as processed in the state file, and commits
// there.
//
// The plan is validated against the working tree, which is what scan read.
// If the vault folder then has uncommitted changes (modified, staged or
// untracked), Apply first commits them, and only them, on the current branch
// (SnapshotMessage), so the worktree starts from exactly what was validated.
// It refuses when changes outside the vault folder are already staged
// (gitx.ErrStagedOutside), since the snapshot would sweep them in.
//
// It refuses, before creating anything, when the plan is invalid
// (*InvalidPlanError) or changes nothing (ErrNoChanges). Any failure after
// the snapshot removes the worktree and branch; the snapshot commit stays.
func Apply(vaultDir string, p *plan.Plan, opts Options) (res *Result, err error) {
	repo, err := gitx.Open(vaultDir)
	if err != nil {
		return nil, err
	}

	v, err := vault.Load(repo.Dir, opts.Vault)
	if err != nil {
		return nil, fmt.Errorf("load vault: %w", err)
	}
	r := plan.Validate(v, p, plan.Options{})
	if !r.OK() {
		return nil, &InvalidPlanError{Errors: r.Errors}
	}
	if len(r.Changes) == 0 && len(r.Kept) == 0 {
		return nil, ErrNoChanges
	}
	for _, c := range r.Changes {
		if !filepath.IsLocal(filepath.FromSlash(c.Path)) {
			return nil, fmt.Errorf("%s: path leaves the vault", c.Path)
		}
	}

	branch := opts.Branch
	if branch == "" {
		now := time.Now
		if opts.Now != nil {
			now = opts.Now
		}
		branch = BranchPrefix + now().UTC().Format("20060102-150405")
	}
	if _, exists, err := repo.BranchCommit(branch); err != nil {
		return nil, err
	} else if exists {
		return nil, fmt.Errorf("branch %s already exists", branch)
	}

	root := opts.WorktreeRoot
	if root == "" {
		root = os.TempDir()
	}
	top, err := repo.TopLevel()
	if err != nil {
		return nil, err
	}
	if within(root, top) {
		return nil, fmt.Errorf("worktree folder %s is inside the repo %s", root, top)
	}

	// Commit the vault's own uncommitted changes, so HEAD matches the
	// working tree the plan was validated against.
	snapshot, err := repo.CommitDir(SnapshotMessage)
	switch {
	case errors.Is(err, gitx.ErrNothingToCommit):
		snapshot = ""
	case err != nil:
		return nil, fmt.Errorf("snapshot commit: %w", err)
	}
	base, err := repo.Head()
	if err != nil {
		return nil, err
	}

	parent, err := os.MkdirTemp(root, worktreePrefix)
	if err != nil {
		return nil, err
	}
	wt := filepath.Join(parent, strings.ReplaceAll(branch, "/", "-"))

	// From here on, any failure removes everything this call made.
	created := false
	defer func() {
		if err == nil {
			return
		}
		var cleanup []error
		if created {
			cleanup = append(cleanup,
				repo.RemoveWorktree(wt, true),
				repo.DeleteBranch(branch))
		}
		cleanup = append(cleanup, os.RemoveAll(parent))
		if cerr := errors.Join(cleanup...); cerr != nil {
			err = fmt.Errorf("%w (cleanup also failed: %v)", err, cerr)
		}
		res = nil
	}()

	w, err := repo.AddWorktree(wt, branch, base)
	if err != nil {
		return nil, err
	}
	created = true

	// The worktree starts from HEAD and the vault was clean, so every note
	// must match what was validated. Check all before writing any.
	for _, c := range r.Changes {
		cur, rerr := os.ReadFile(filepath.Join(w.Dir, filepath.FromSlash(c.Path)))
		switch {
		case c.Created() && rerr == nil:
			return nil, fmt.Errorf("%s: already exists in the worktree", c.Path)
		case c.Created() && !errors.Is(rerr, os.ErrNotExist):
			return nil, rerr
		case !c.Created() && rerr != nil:
			return nil, fmt.Errorf("%s: not in HEAD (is it ignored by git?): %w", c.Path, rerr)
		case !c.Created() && !bytes.Equal(cur, c.Before):
			return nil, fmt.Errorf("%s: differs from HEAD", c.Path)
		}
	}

	st, err := state.Load(w.Dir)
	if err != nil {
		return nil, err
	}
	for i, c := range r.Changes {
		fp := filepath.Join(w.Dir, filepath.FromSlash(c.Path))
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(fp, c.After, 0o644); err != nil {
			return nil, err
		}
		if afterWrite != nil {
			if err := afterWrite(i, c.Path); err != nil {
				return nil, err
			}
		}
		st.MarkNote(note.Parse(c.Path, c.After))
	}
	for _, k := range r.Kept {
		st.MarkNote(v.Note(k))
	}
	if err := st.Save(w.Dir); err != nil {
		return nil, err
	}

	commit, err := w.CommitAll(commitMessage(r.Changes, r.Kept))
	if err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	wtTop, err := w.TopLevel()
	if err != nil {
		return nil, err
	}
	return &Result{
		Branch:   branch,
		Worktree: filepath.Clean(wtTop),
		VaultDir: w.Dir,
		Snapshot: snapshot,
		Base:     base,
		Commit:   commit,
		Changes:  r.Changes,
		Kept:     r.Kept,
	}, nil
}

// commitMessage summarizes the changes, one line per note, and then the
// notes kept as is.
func commitMessage(changes []plan.Change, kept []string) string {
	var b strings.Builder
	if len(changes) == 0 {
		fmt.Fprintf(&b, "curate: keep %s as is\n\n", count(len(kept)))
	} else {
		fmt.Fprintf(&b, "curate: apply plan to %s\n\n", count(len(changes)))
	}
	for _, c := range changes {
		verb := "update"
		if c.Created() {
			verb = "create"
		}
		fmt.Fprintf(&b, "- %s %s (%s)\n", verb, c.Path, strings.Join(c.Ops, ", "))
	}
	for _, k := range kept {
		fmt.Fprintf(&b, "- keep %s\n", k)
	}
	return b.String()
}

func count(n int) string {
	if n == 1 {
		return "1 note"
	}
	return fmt.Sprintf("%d notes", n)
}

// within reports whether path is dir or inside it, after resolving
// symlinks where possible.
func within(path, dir string) bool {
	path, dir = realPath(path), realPath(dir)
	rel, err := filepath.Rel(dir, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

func realPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return filepath.Clean(p)
}
