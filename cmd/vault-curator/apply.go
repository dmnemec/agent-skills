package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/apply"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
)

const applyUsage = `usage: vault-curator apply --vault <path> [--branch <name>] [--json] <plan.json>

Validates the plan, then writes its approved operations in a new git worktree
on a new branch, outside the vault, records the written blocks as processed
and commits there. The vault's own working tree is not touched. Merge the
branch, then run "vault-curator watermark".

The vault must have no uncommitted changes. On any failure the worktree and
branch are removed. Exit code 0 on success, 1 on failure, 2 on bad usage.

flags:
`

// runApply implements "vault-curator apply".
func runApply(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, applyUsage)
		fs.PrintDefaults()
	}
	vaultDir := fs.String("vault", "", "path to the vault `directory`")
	branch := fs.String("branch", "", "name of the new `branch` (default curate/<UTC time>)")
	worktreeRoot := fs.String("worktree-root", "", "`folder` to create the worktree in, outside the repo (default the OS temp folder)")
	jsonOut := fs.Bool("json", false, "print the result as JSON")
	paths, ok := parseInterspersed(fs, args)
	if !ok {
		return 2
	}
	if *vaultDir == "" || len(paths) != 1 {
		fs.Usage()
		return 2
	}

	p, err := plan.Load(paths[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	res, err := apply.Apply(*vaultDir, p, apply.Options{Branch: *branch, WorktreeRoot: *worktreeRoot})

	var inv *apply.InvalidPlanError
	switch {
	case errors.As(err, &inv):
		if *jsonOut {
			if code := writeJSON(stdout, stderr, struct {
				OK     bool          `json:"ok"`
				Errors []*plan.Error `json:"errors"`
			}{Errors: inv.Errors}); code != 0 {
				return code
			}
		}
		for _, e := range inv.Errors {
			fmt.Fprintln(stderr, e)
		}
		fmt.Fprintln(stderr, inv)
		return 1
	case err != nil:
		fmt.Fprintf(stderr, "apply: %v\n", err)
		return 1
	}

	if *jsonOut {
		type change struct {
			Path    string   `json:"path"`
			Created bool     `json:"created"`
			Ops     []string `json:"ops"`
		}
		out := struct {
			OK bool `json:"ok"`
			*apply.Result
			Changes []change `json:"changes"`
		}{OK: true, Result: res, Changes: []change{}}
		for _, c := range res.Changes {
			out.Changes = append(out.Changes, change{Path: c.Path, Created: c.Created(), Ops: c.Ops})
		}
		return writeJSON(stdout, stderr, out)
	}
	noun := "notes"
	if len(res.Changes) == 1 {
		noun = "note"
	}
	fmt.Fprintf(stdout, "applied %d %s on branch %s\n", len(res.Changes), noun, res.Branch)
	fmt.Fprintf(stdout, "worktree: %s\n", res.Worktree)
	for _, c := range res.Changes {
		verb := "update"
		if c.Created() {
			verb = "create"
		}
		fmt.Fprintf(stdout, "  %s %s (%s)\n", verb, c.Path, strings.Join(c.Ops, ", "))
	}
	fmt.Fprintf(stdout, "merge %s into your branch, then run: vault-curator watermark --vault %s\n", res.Branch, *vaultDir)
	return 0
}

// parseInterspersed parses fs from args, allowing flags after positional
// arguments as well as before, and returns the positional arguments. ok is
// false on a flag error.
func parseInterspersed(fs *flag.FlagSet, args []string) (pos []string, ok bool) {
	for {
		if err := fs.Parse(args); err != nil {
			return nil, false
		}
		if fs.NArg() == 0 {
			return pos, true
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// writeJSON prints v as indented JSON and returns the exit code.
func writeJSON(stdout, stderr io.Writer, v any) int {
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
