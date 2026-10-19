package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/entities"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// runEntities implements `vault-curator entities`. The --json shape is
// documented in docs/cli.md.
func runEntities(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("entities", flag.ContinueOnError)
	fs.SetOutput(stderr)
	vaultDir := fs.String("vault", ".", "vault root")
	asJSON := fs.Bool("json", false, "print the listing as JSON")
	minCount := fs.Int("min-count", entities.DefaultMinCount, "least number of mentions for a candidate")
	fallback := fs.String("fallback-hub", "", "title of the fallback hub note (default Unsorted)")
	linksTo := fs.String("links-to", "", "list the links to this note (title, alias or path) and the blocks of each linking note, for merging")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "entities: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if *minCount < 1 {
		fmt.Fprintln(stderr, "entities: --min-count must be at least 1")
		return 2
	}

	v, err := vault.Load(*vaultDir, vault.Options{FallbackHub: *fallback})
	if err != nil {
		fmt.Fprintf(stderr, "entities: %v\n", err)
		return 1
	}
	var res any
	if *linksTo != "" {
		b, err := entities.LinksTo(v, *linksTo)
		if err != nil {
			fmt.Fprintf(stderr, "entities: %v\n", err)
			return 1
		}
		if !*asJSON {
			fmt.Fprintf(stdout, "%d notes link to %s\n", len(b.Notes), b.Target.Path)
			for _, n := range b.Notes {
				for _, l := range n.Links {
					fmt.Fprintf(stdout, "  %s:%d  %s\n", n.Path, l.Line, l.Raw)
				}
			}
			return 0
		}
		res = b
	} else {
		r := entities.Run(v, entities.Options{MinCount: *minCount})
		if !*asJSON {
			printListing(stdout, r)
			return 0
		}
		res = r
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		fmt.Fprintf(stderr, "entities: %v\n", err)
		return 1
	}
	return 0
}

func printListing(w io.Writer, res *entities.Result) {
	fmt.Fprintf(w, "%d entities, %d candidates\n", len(res.Entities), len(res.Candidates))
	for _, e := range res.Entities {
		fmt.Fprintf(w, "  entity %s (%s, %d backlinks)\n", e.Path, e.Type, e.Backlinks)
	}
	for _, c := range res.Candidates {
		fmt.Fprintf(w, "  candidate %s: %d mentions\n", c.Name, c.Count)
		for _, o := range c.Occurrences {
			fmt.Fprintf(w, "    %s:%d\n", o.Path, o.Line)
		}
	}
}
