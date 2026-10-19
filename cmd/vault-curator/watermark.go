package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/apply"
)

const watermarkUsage = `usage: vault-curator watermark [--vault <path>] [--branch <name>] [--json]

Run after merging a curation branch into the vault's branch. Moves the
watermark tag to the curation commit, then removes the branch's worktree and
deletes the branch. Refuses, changing nothing, if the branch is not merged
into HEAD. Exit code 0 on success, 1 on failure, 2 on bad usage.

flags:
`

// runWatermark implements "vault-curator watermark".
func runWatermark(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("watermark", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, watermarkUsage)
		fs.PrintDefaults()
	}
	vaultDir := fs.String("vault", ".", "path to the vault `directory`")
	branch := fs.String("branch", "", "the merged curation `branch` (default the only one merged into HEAD)")
	jsonOut := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}

	res, err := apply.Watermark(*vaultDir, apply.WatermarkOptions{Branch: *branch})
	if err != nil {
		fmt.Fprintf(stderr, "watermark: %v\n", err)
		return 1
	}
	if *jsonOut {
		return writeJSON(stdout, stderr, res)
	}
	fmt.Fprintf(stdout, "moved %s to %s (%s)\n", res.Tag, shortHash(res.Commit), res.Branch)
	if res.Worktree != "" {
		fmt.Fprintf(stdout, "removed worktree %s\n", res.Worktree)
	}
	fmt.Fprintf(stdout, "deleted branch %s\n", res.Branch)
	return 0
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
