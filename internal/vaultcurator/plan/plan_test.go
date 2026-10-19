package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := Parse([]byte(`{"version": 1, "operations": [{"id": "a", "staus": "approved"}]}`))
	if err == nil || !strings.Contains(err.Error(), "staus") {
		t.Errorf("err = %v", err)
	}
}

func TestParseRejectsOtherVersions(t *testing.T) {
	for _, src := range []string{`{"operations": []}`, `{"version": 2, "operations": []}`} {
		if _, err := Parse([]byte(src)); err == nil || !strings.Contains(err.Error(), "version") {
			t.Errorf("Parse(%s) err = %v", src, err)
		}
	}
	if _, err := Parse([]byte(`{"version": 1} {}`)); err == nil {
		t.Error("trailing data accepted")
	}
}

func TestDocsExampleMatchesFixture(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "plan.md"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(examplePlan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), "```json\n"+string(fixture)+"```\n") {
		t.Error("docs/plan.md's complete example differs from testdata/plans/2026-09-14.json")
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	p, err := Load(examplePlan)
	if err != nil {
		t.Fatal(err)
	}
	p.Op("split-retry").Status = Rejected
	data, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "}\n") || !strings.Contains(string(data), "\n  \"operations\": [\n") {
		t.Errorf("not indented JSON with a trailing newline: %s", data)
	}
	q, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Operations) != len(p.Operations) || q.Op("split-retry").Status != Rejected {
		t.Errorf("round trip lost data: %+v", q)
	}
	if q.Op("nope") != nil {
		t.Error("Op found a missing id")
	}
	if q.Op("fm-daily").Frontmatter["type"] != "daily" {
		t.Errorf("frontmatter = %v", q.Op("fm-daily").Frontmatter)
	}
}
