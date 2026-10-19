// Command vault-curator is the deterministic half of vault-curator: it scans a
// markdown vault, validates and applies curation plans, and answers graph
// queries. See docs/design.md.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

// resolveVersion picks the version to report: the ldflags value when it is set,
// else the module version from build info (set by go install ...@version and
// go build in a tagged module), else "dev". "(devel)" means a local build with
// no version, so it counts as unset.
func resolveVersion(ldflags string, info *debug.BuildInfo, ok bool) string {
	if ldflags != "dev" {
		return ldflags
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

const usage = `usage: vault-curator <command> [flags]

commands:
  version   print the version
  scan      list notes and blocks changed since the watermark (--vault, --json)
  entities  list entities and candidate entities in the vault (--vault, --json)
  validate  check a curation plan against the vault (--vault, --draft, --json)
  review    render a plan for review, or sync edited statuses back (--vault, --sync)
  apply     apply an approved plan on a new branch in a worktree (--vault, --branch, --json)
  watermark after merging a curation branch, move the watermark and clean up (--vault, --json)
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
		info, ok := debug.ReadBuildInfo()
		fmt.Fprintln(stdout, resolveVersion(version, info, ok))
		return 0
	case "scan":
		return runScan(args[1:], stdout, stderr)
	case "entities":
		return runEntities(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "review":
		return runReview(args[1:], stdout, stderr)
	case "apply":
		return runApply(args[1:], stdout, stderr)
	case "watermark":
		return runWatermark(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}
