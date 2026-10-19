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
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "scan: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	res, err := scan.Run(*vaultDir, scan.Options{FallbackHub: *fallback})
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

	if res.Watermark.Exists {
		fmt.Fprintf(stdout, "watermark %s at %.12s\n", res.Watermark.Tag, res.Watermark.Commit)
	} else {
		fmt.Fprintf(stdout, "no watermark tag %s: every note is a candidate\n", res.Watermark.Tag)
	}
	fmt.Fprintf(stdout, "%d candidate notes, %d new blocks\n", len(res.Candidates), res.NewBlockCount())
	for _, c := range res.Candidates {
		fmt.Fprintf(stdout, "  %s: %d new, %d processed\n", c.Path, len(c.NewBlocks), c.ProcessedBlocks)
		for _, b := range c.NewBlocks {
			fmt.Fprintf(stdout, "    lines %d-%d  %s\n", b.StartLine, b.EndLine, b.Hash)
		}
	}
	fmt.Fprintf(stdout, "%d notes, %d entities, %d links, %d broken links, %d orphans\n",
		len(res.Notes), len(res.Entities), len(res.Links), len(res.BrokenLinks), len(res.Orphans))
	return 0
}
