package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// IsIgnored reports whether git ignores path, a slash path relative to r.Dir
// that need not exist (git check-ignore).
func (r *Repo) IsIgnored(path string) (bool, error) {
	cmd := exec.Command("git", "check-ignore", "-q", "--", path)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), r.Env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &ee) && ee.ExitCode() == 1:
		return false, nil
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = err.Error()
	}
	return false, fmt.Errorf("git check-ignore %s: %s", path, msg)
}

// MissingFromCommit returns the paths (slash paths relative to r.Dir) that
// the tree of commit does not contain, in the order given (git ls-tree).
func (r *Repo) MissingFromCommit(commit string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	args := append([]string{"ls-tree", "-r", "--name-only", "-z", commit, "--"}, paths...)
	out, err := r.run(args...)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, p := range strings.Split(out, "\x00") {
		have[p] = true
	}
	var missing []string
	for _, p := range paths {
		if !have[p] {
			missing = append(missing, p)
		}
	}
	return missing, nil
}
