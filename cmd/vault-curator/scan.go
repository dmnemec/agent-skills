package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/scan"
)

// runScan implements `vault-curator scan`. The --json shape is documented in
// docs/cli.md.
func runScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	vaultDir := fs.String("vault", ".", "vault root (inside a git work tree)")
	asJSON := fs.Bool("json", false, "print the scan as JSON")
	fallback := fs.String("fallback-hub", "", "title of the fallback hub note (default Unsorted)")
	all := fs.Bool("all", false, "ignore the watermark: list every note with an unprocessed block")
	limit := fs.Int("limit", 0, "list at most N candidates, in path order (0 = no cap)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "scan: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	if *limit < 0 {
		fmt.Fprintln(stderr, "scan: --limit must not be negative")
		return 2
	}

	res, err := scan.Run(*vaultDir, scan.Options{FallbackHub: *fallback, All: *all, Limit: *limit})
	if err != nil {
		fmt.Fprintf(stderr, "scan: %v\n", err)
		return 1
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintf(stderr, "scan: %v\n", err)
			return 1
		}
		return 0
	}

	if *all {
		fmt.Fprintln(stdout, "--all: watermark ignored, candidates are notes with unprocessed blocks")
	} else if res.Watermark.Exists {
		fmt.Fprintf(stdout, "watermark %s at %.12s\n", res.Watermark.Tag, res.Watermark.Commit)
	} else {
		fmt.Fprintf(stdout, "no watermark tag %s: every note is a candidate\n", res.Watermark.Tag)
	}
	fmt.Fprintf(stdout, "%d candidate notes, %d new blocks\n", len(res.Candidates), res.NewBlockCount())
	if res.Remaining > 0 {
		fmt.Fprintf(stdout, "%d more candidates left out by --limit\n", res.Remaining)
	}
	for _, c := range res.Candidates {
		fmt.Fprintf(stdout, "  %s: %d new, %d processed\n", c.Path, len(c.NewBlocks), c.ProcessedBlocks)
		for _, b := range c.Blocks {
			mark := "new"
			if b.Processed {
				mark = "processed"
			}
			fmt.Fprintf(stdout, "    lines %d-%d  %s  %s\n", b.StartLine, b.EndLine, b.Hash, mark)
		}
	}
	fmt.Fprintf(stdout, "%d notes, %d entities, %d links, %d broken links, %d orphans\n",
		len(res.Notes), len(res.Entities), len(res.Links), len(res.BrokenLinks), len(res.Orphans))
	return 0
}
