package plan

import (
	"strings"
	"testing"
)

func TestSelected(t *testing.T) {
	v, p := loadExample(t)
	items, err := Selected(v, p.Op("split-beacon").Sources[0])
	if err != nil {
		t.Fatal(err)
	}
	var lines []int
	for _, it := range items {
		lines = append(lines, it.Line)
	}
	if len(lines) != 5 || lines[0] != 12 || lines[4] != 16 {
		t.Errorf("lines = %v, want 12 to 16", lines)
	}

	_, err = Selected(v, Source{Note: "daily/2026-09-14.md", Block: "94300d2390c6855f", Lines: [2]int{5, 20}})
	if err == nil || !strings.Contains(err.Error(), "outside block") {
		t.Errorf("err = %v, want a range outside the block", err)
	}
	if _, err := Selected(v, Source{Note: "missing.md", Block: "x", Lines: [2]int{1, 1}}); err == nil {
		t.Error("missing note accepted")
	}
}

func TestSplitLink(t *testing.T) {
	_, p := loadExample(t)
	if got := p.Op("split-standup").SplitLink(); got != "- [[2026-09-14 Harbor standup]]" {
		t.Errorf("default link = %q", got)
	}
	if got := p.Op("split-teodor").SplitLink(); got != "- [[2026-09-14 1-1 Teodor|1:1 Teodor]]" {
		t.Errorf("link = %q", got)
	}
}
