// Command vault-curator is the deterministic half of vault-curator: it scans a
// markdown vault, validates and applies curation plans, and answers graph
// queries. See docs/design.md.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage: vault-curator <command> [flags]

commands:
  version   print the version
  scan      list notes and blocks changed since the watermark (--vault, --json)
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, version)
		return 0
	case "scan":
		return runScan(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}
