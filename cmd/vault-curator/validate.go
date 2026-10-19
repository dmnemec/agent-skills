package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gitx"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// gitIgnored returns the git-ignore check for the vault at dir, or nil when
// dir is not in a git repo (nothing is ignored then).
func gitIgnored(dir string) func(string) (bool, error) {
	repo, err := gitx.Open(dir)
	if err != nil {
		return nil
	}
	return repo.IsIgnored
}

const validateUsage = `usage: vault-curator validate --vault <path> [--draft] [--json] <plan.json>

Checks a curation plan against the vault without changing anything: the
lossless check, broken links, unapproved and never-tier changes. Exit code 0
means apply would accept the plan, 1 that it would not.

flags:
`

// runValidate implements "vault-curator validate".
func runValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, validateUsage)
		fs.PrintDefaults()
	}
	vaultDir := fs.String("vault", "", "path to the vault `directory`")
	draft := fs.Bool("draft", false, "treat pending operations as approved, to check a plan before review")
	jsonOut := fs.Bool("json", false, "print the result as JSON")
	// Allow flags after the plan path as well as before it.
	var paths []string
	for {
		if err := fs.Parse(args); err != nil {
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		paths = append(paths, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if *vaultDir == "" || len(paths) != 1 {
		fs.Usage()
		return 2
	}

	v, err := vault.Load(*vaultDir, vault.Options{})
	if err != nil {
		fmt.Fprintf(stderr, "load vault: %v\n", err)
		return 1
	}
	p, err := plan.Load(paths[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	r := plan.Validate(v, p, plan.Options{Draft: *draft, Ignored: gitIgnored(*vaultDir)})

	if *jsonOut {
		type change struct {
			Path    string   `json:"path"`
			Created bool     `json:"created"`
			Ops     []string `json:"ops"`
		}
		out := struct {
			OK      bool          `json:"ok"`
			Errors  []*plan.Error `json:"errors"`
			Changes []change      `json:"changes"`
			Kept    []string      `json:"kept"`
		}{OK: r.OK(), Errors: r.Errors, Changes: []change{}, Kept: append([]string{}, r.Kept...)}
		if out.Errors == nil {
			out.Errors = []*plan.Error{}
		}
		for _, c := range r.Changes {
			out.Changes = append(out.Changes, change{Path: c.Path, Created: c.Created(), Ops: c.Ops})
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else if r.OK() {
		fmt.Fprintf(stdout, "plan OK: %d notes to change\n", len(r.Changes))
		for _, c := range r.Changes {
			verb := "update"
			if c.Created() {
				verb = "create"
			}
			fmt.Fprintf(stdout, "  %s %s (%s)\n", verb, c.Path, strings.Join(c.Ops, ", "))
		}
		for _, k := range r.Kept {
			fmt.Fprintf(stdout, "  keep %s\n", k)
		}
	} else {
		for _, e := range r.Errors {
			fmt.Fprintln(stderr, e)
		}
		noun := "errors"
		if len(r.Errors) == 1 {
			noun = "error"
		}
		fmt.Fprintf(stderr, "plan invalid: %d %s\n", len(r.Errors), noun)
	}
	if !r.OK() {
		return 1
	}
	return 0
}
