package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/review"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

const reviewUsage = `usage: vault-curator review --vault <path> [--out <review.md>] [--force] <plan.json>
       vault-curator review --sync <review.md> <plan.json>

Renders a curation plan as a markdown review document, or, with --sync, reads
the statuses edited in that document back into the plan. The plan must pass
validate --draft to be rendered.

flags:
`

// runReview implements "vault-curator review".
func runReview(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("review", flag.ContinueOnError)
	fl.SetOutput(stderr)
	fl.Usage = func() {
		fmt.Fprint(stderr, reviewUsage)
		fl.PrintDefaults()
	}
	vaultDir := fl.String("vault", "", "path to the vault `directory`")
	out := fl.String("out", "", "review `file` to write (default: the plan path with .review.md for .json; - for stdout)")
	force := fl.Bool("force", false, "overwrite an existing review file")
	sync := fl.String("sync", "", "read statuses from this review `file` into the plan")
	// Allow flags after the plan path as well as before it.
	var paths []string
	for {
		if err := fl.Parse(args); err != nil {
			return 2
		}
		if fl.NArg() == 0 {
			break
		}
		paths = append(paths, fl.Arg(0))
		args = fl.Args()[1:]
	}
	if len(paths) != 1 || (*sync == "" && *vaultDir == "") {
		fl.Usage()
		return 2
	}
	planPath := paths[0]

	p, err := plan.Load(planPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *sync != "" {
		return syncReview(*sync, planPath, p, stdout, stderr)
	}

	reviewPath := *out
	if reviewPath == "" {
		reviewPath = strings.TrimSuffix(planPath, ".json") + ".review.md"
	}
	v, err := vault.Load(*vaultDir, vault.Options{})
	if err != nil {
		fmt.Fprintf(stderr, "load vault: %v\n", err)
		return 1
	}
	shown := reviewPath
	if shown == "-" {
		shown = "<review.md>"
	}
	doc, err := review.Render(v, p, review.Options{PlanPath: planPath, ReviewPath: shown})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if reviewPath == "-" {
		if _, err := stdout.Write(doc); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if !*force {
		if _, err := os.Stat(reviewPath); err == nil {
			fmt.Fprintf(stderr, "%s already exists and may hold your edits; sync it first, or pass --force to overwrite it\n", reviewPath)
			return 1
		} else if !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := os.WriteFile(reviewPath, doc, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s: %d operations to review\n", reviewPath, len(p.Operations))
	return 0
}

func syncReview(reviewPath, planPath string, p *plan.Plan, stdout, stderr io.Writer) int {
	doc, err := os.ReadFile(reviewPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := review.Sync(doc, p); err != nil {
		fmt.Fprintf(stderr, "%s: %v\nreview not synced; the plan is unchanged\n", reviewPath, err)
		return 1
	}
	data, err := p.Marshal()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	counts := map[plan.Status]int{}
	for _, op := range p.Operations {
		counts[op.Status]++
	}
	fmt.Fprintf(stdout, "synced %s into %s: %d approved, %d rejected, %d pending\n",
		reviewPath, planPath, counts[plan.Approved], counts[plan.Rejected], counts[plan.Pending])
	if counts[plan.Pending] > 0 {
		fmt.Fprintln(stdout, "validate and apply need every operation approved or rejected")
	}
	return 0
}
