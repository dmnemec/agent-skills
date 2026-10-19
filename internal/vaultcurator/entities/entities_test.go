package entities

import (
	"path/filepath"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

func fixture(t *testing.T) *Result {
	t.Helper()
	v, err := vault.Load(filepath.Join("..", "..", "..", "testdata", "vault"), vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return Run(v, Options{})
}

func find(r *Result, name string) *Candidate {
	for i := range r.Candidates {
		if r.Candidates[i].Name == name {
			return &r.Candidates[i]
		}
	}
	return nil
}

func names(r *Result) []string {
	var out []string
	for _, c := range r.Candidates {
		out = append(out, c.Name)
	}
	return out
}

func TestFixtureCandidates(t *testing.T) {
	r := fixture(t)
	for _, name := range []string{"Teodor Quill", "Sana Okafor-Lind", "Oskar Fenwick", "Lio Haverkamp"} {
		c := find(r, name)
		if c == nil {
			t.Errorf("candidate %q missing; got %v", name, names(r))
			continue
		}
		if c.Count < 2 || c.Count != len(c.Occurrences) {
			t.Errorf("%s: count %d, occurrences %d", name, c.Count, len(c.Occurrences))
		}
	}
	// Short forms fold into the full name, and a lowercase mention counts.
	if oskar := find(r, "Oskar Fenwick"); oskar != nil {
		found := false
		for _, o := range oskar.Occurrences {
			if o.Path == "daily/2026-09-16.md" && o.Line == 1 {
				found = true
			}
		}
		if !found {
			t.Errorf("lowercase mention 'oskar' on daily/2026-09-16.md:1 missing: %v", oskar.Occurrences)
		}
	}
	if find(r, "Teodor") != nil {
		t.Error("Teodor should fold into Teodor Quill")
	}
}

func TestFixtureNoise(t *testing.T) {
	r := fixture(t)
	t.Logf("candidates: %v", names(r))
	for _, bad := range []string{"Mira", "Mira Vantongeren", "Harbor", "Tidegate", "Ledgerline", "Beacon", "AI", "Add", "Mermaid"} {
		if c := find(r, bad); c != nil {
			t.Errorf("%q should not be a candidate: %+v", bad, c)
		}
	}
	var mira *Entity
	for i := range r.Entities {
		if r.Entities[i].Title == "Mira Vantongeren" {
			mira = &r.Entities[i]
		}
	}
	if mira == nil || mira.Type != "person" || mira.Backlinks == 0 {
		t.Errorf("Mira entity = %+v", mira)
	}
}

func TestLinksTo(t *testing.T) {
	v, err := vault.Load(filepath.Join("..", "..", "..", "testdata", "vault"), vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// An alias resolves to its note.
	b, err := LinksTo(v, "Mira")
	if err != nil {
		t.Fatal(err)
	}
	if b.Target.Path != "people/Mira Vantongeren.md" || len(b.Notes) == 0 {
		t.Fatalf("backlinks = %+v", b)
	}
	for _, n := range b.Notes {
		if len(n.Links) == 0 {
			t.Errorf("%s lists no links", n.Path)
		}
		if len(n.Blocks) != len(v.Note(n.Path).Blocks) {
			t.Errorf("%s: %d blocks, want %d", n.Path, len(n.Blocks), len(v.Note(n.Path).Blocks))
		}
	}
	if _, err := LinksTo(v, "Nobody"); err == nil {
		t.Error("unknown name: want error")
	}
}

func small(files map[string]string) *vault.Vault {
	var notes []*note.Note
	for p, c := range files {
		notes = append(notes, note.Parse(p, []byte(c)))
	}
	return vault.New(notes, nil, vault.Options{})
}

func TestNoiseFiltered(t *testing.T) {
	v := small(map[string]string{
		"A.md": "# Heading Words Here\n\n" + // 1-2
			"- Monday we met. Friday is next. In March we ship.\n" + // 3
			"- Add retries. Add metrics. Add docs.\n" + // 4
			"- see [[Orlando Park]] and [Orlando Park](x.md) and https://Orlando.Park/Zed\n" + // 5
			"- `Orlando Park` in code\n" + // 6
			"```\nOrlando Park\nOrlando Park\n```\n" + // 7-10
			"- Talked to Zed Quarrel about it\n" + // 11
			"- Zed Quarrel agreed\n" + // 12
			"- one-off Singleton mention\n", // 13
	})
	r := Run(v, Options{})
	if len(r.Candidates) != 1 || r.Candidates[0].Name != "Zed Quarrel" {
		t.Fatalf("candidates = %v, want only Zed Quarrel", names(r))
	}
	c := r.Candidates[0]
	if c.Count != 2 || c.Occurrences[0].Line != 11 || c.Occurrences[1].Line != 12 {
		t.Errorf("occurrences = %+v", c.Occurrences)
	}
}

func TestMinCountAndKnownNames(t *testing.T) {
	v := small(map[string]string{
		"A.md":         "- Talked to Zed Quarrel\n- Zed Quarrel again, and Ana B\n- Ana B too\n",
		"Ana Baker.md": "---\ntype: person\naliases: [Ana B]\n---\n",
	})
	if got := names(Run(v, Options{MinCount: 3})); len(got) != 0 {
		t.Errorf("min 3: %v", got)
	}
	if got := names(Run(v, Options{})); len(got) != 1 || got[0] != "Zed Quarrel" {
		t.Errorf("default: %v (alias Ana B must not be a candidate)", got)
	}
}
