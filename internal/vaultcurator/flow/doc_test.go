package flow

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the agent flow block in docs/design.md")

func TestDesignDocIsCurrent(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "design.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")
	got, err := ReplaceBlock(doc, VaultCurator().DocBlock())
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got != doc {
		t.Fatal("docs/design.md's agent flow block is stale; run: go test ./internal/vaultcurator/flow -run TestDesignDoc -update")
	}
}

func TestReplaceBlockNeedsMarkers(t *testing.T) {
	if _, err := ReplaceBlock("no markers here", "x"); err == nil {
		t.Fatal("missing markers were accepted")
	}
	got, err := ReplaceBlock("a\n"+BeginMarker+"\nold\n"+EndMarker+"\nb", "NEW")
	if err != nil || got != "a\nNEW\nb" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestMermaidShowsExitsAndTerminals(t *testing.T) {
	d := VaultCurator().Mermaid("curate")
	for _, want := range []string{
		"stateDiagram-v2\n",
		`state "curate.merge" as curate_merge`,
		"curate_merge --> curate_abort_merge : merge_conflict",
		"curate_watermark --> exit_merged : ok",
		"[*] --> curate_locate : curate",
		"entities_merge --> curate_validate_draft : ok",
		"parked --> [*]",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("curate diagram lacks %q:\n%s", want, d)
		}
	}
}
