package apply

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gitx"
)

// ErrNoBranch is returned by Watermark when there is no curation branch.
var ErrNoBranch = errors.New("no curation branch found")

// ErrNotMerged is returned by Watermark when the curation branch is not an
// ancestor of HEAD, so the curation has not landed yet.
var ErrNotMerged = errors.New("curation branch is not merged into HEAD")

// WatermarkOptions configures Watermark.
type WatermarkOptions struct {
	// Branch is the merged curation branch. Default: the only curation
	// branch merged into HEAD.
	Branch string
	// Tag is the watermark tag. Default: gitx.WatermarkTag.
	Tag string
}

// WatermarkResult describes a moved watermark.
type WatermarkResult struct {
	Tag string `json:"tag"`
	// Commit is where the tag now points: the curation commit (the
	// branch's tip), which is in HEAD's history.
	Commit string `json:"commit"`
	// Branch is the curation branch, now deleted.
	Branch string `json:"branch"`
	// Worktree is the removed worktree's path, empty when it was already
	// gone.
	Worktree string `json:"worktree,omitempty"`
}

// Watermark finishes a curation once the user has merged its branch into
// the vault's checked-out branch: it moves the watermark tag to the
// curation commit, then removes the worktree and deletes the branch.
//
// The tag goes on the branch tip rather than the merge commit, so notes the
// commit made outside curation on the main branch between apply and merge still
// differ from the tag and stay scan candidates. The curated notes are the
// same in the tip and the merge, so they are not.
//
// It refuses, changing nothing, when the branch is not an ancestor of HEAD
// (ErrNotMerged), when it is the checked-out branch, or when its worktree
// has uncommitted changes.
func Watermark(vaultDir string, opts WatermarkOptions) (*WatermarkResult, error) {
	repo, err := gitx.Open(vaultDir)
	if err != nil {
		return nil, err
	}
	tag := opts.Tag
	if tag == "" {
		tag = gitx.WatermarkTag
	}
	head, err := repo.Head()
	if err != nil {
		return nil, err
	}

	branch, tip, err := mergedBranch(repo, opts.Branch, head)
	if err != nil {
		return nil, err
	}
	if cur, err := repo.CurrentBranch(); err != nil {
		return nil, err
	} else if cur == branch {
		return nil, fmt.Errorf("%s is checked out here; run watermark from the vault's main working tree", branch)
	}

	wt, err := worktreeOf(repo, branch)
	if err != nil {
		return nil, err
	}
	if wt != "" {
		if _, serr := os.Stat(wt); serr != nil {
			wt = "" // deleted by hand; prune below
		} else if clean, err := (&gitx.Repo{Dir: wt, Env: repo.Env}).IsClean(); err != nil {
			return nil, err
		} else if !clean {
			return nil, fmt.Errorf("worktree %s has uncommitted changes; commit and merge them, or discard them, first", wt)
		}
	}

	if err := repo.SetTag(tag, tip); err != nil {
		return nil, err
	}
	if wt != "" {
		if err := repo.RemoveWorktree(wt, false); err != nil {
			return nil, err
		}
		// Remove the temp folder apply made around the worktree, if empty.
		if parent := filepath.Dir(wt); strings.HasPrefix(filepath.Base(parent), worktreePrefix) {
			_ = os.Remove(parent)
		}
	}
	if _, err := repo.Run("worktree", "prune"); err != nil {
		return nil, err
	}
	if err := repo.DeleteBranch(branch); err != nil {
		return nil, err
	}
	return &WatermarkResult{Tag: tag, Commit: tip, Branch: branch, Worktree: wt}, nil
}

// mergedBranch picks the curation branch to finish and returns its tip.
func mergedBranch(repo *gitx.Repo, want, head string) (string, string, error) {
	if want != "" {
		tip, ok, err := repo.BranchCommit(want)
		if err != nil {
			return "", "", err
		}
		if !ok {
			return "", "", fmt.Errorf("%w: %s does not exist", ErrNoBranch, want)
		}
		merged, err := repo.IsAncestor(tip, head)
		if err != nil {
			return "", "", err
		}
		if !merged {
			return "", "", fmt.Errorf("%w: merge %s first", ErrNotMerged, want)
		}
		return want, tip, nil
	}

	out, err := repo.Run("for-each-ref", "--format=%(refname:short)", "refs/heads/"+BranchPrefix)
	if err != nil {
		return "", "", err
	}
	var all, merged, tips []string
	for _, b := range strings.Fields(out) {
		tip, ok, err := repo.BranchCommit(b)
		if err != nil {
			return "", "", err
		}
		if !ok {
			continue
		}
		all = append(all, b)
		if m, err := repo.IsAncestor(tip, head); err != nil {
			return "", "", err
		} else if m {
			merged = append(merged, b)
			tips = append(tips, tip)
		}
	}
	switch {
	case len(all) == 0:
		return "", "", ErrNoBranch
	case len(merged) == 0:
		return "", "", fmt.Errorf("%w: merge one of %s first", ErrNotMerged, strings.Join(all, ", "))
	case len(merged) > 1:
		return "", "", fmt.Errorf("several curation branches are merged (%s); pick one with --branch", strings.Join(merged, ", "))
	}
	return merged[0], tips[0], nil
}

// worktreeOf returns the path of the worktree that has branch checked out,
// or "" when none does.
func worktreeOf(repo *gitx.Repo, branch string) (string, error) {
	out, err := repo.Run("worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	var path string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case line == "branch refs/heads/"+branch:
			return filepath.Clean(path), nil
		}
	}
	return "", nil
}
