package vault

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeVault creates files (vault-relative path to content) under a temp dir.
func writeVault(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

var fixture = map[string]string{
	"projects/SMP.md":      "---\ntype: project\naliases: [SMP project, Smart Meter Platform]\n---\n# SMP\n",
	"components/Parser.md": "---\ntype: component\n---\n- part of [[the smp project]]\n",
	"people/Alice.md":      "---\ntype: person\n---\n- leads [[Parser]]\n",
	"people/Bob.md":        "---\ntype: person\n---\n- no links\n",
	"Unsorted.md":          "# Unsorted\n",
	"daily/2026-10-17.md": "---\ntype: daily\n---\n" +
		"- [[2026-10-17 SMP sync]]\n" +
		"- see ![[diagram.png]] and [the spec](../specs/Spec.md#Scope)\n" +
		"- [[Nobody]] and [[smart meter platform#Goals]] and [[#Local]]\n",
	"meetings/2026-10-17 SMP sync.md": "---\ntype: meeting\nproject: \"[[SMP]]\"\n---\n- met [[Alice|Al]]\n",
	"specs/Spec.md":                   "- spec text, links nowhere\n",
	"loose/Idea.md":                   "- links to [[Spec]] only\n",
	"filed/Parked.md":                 "- parked in [[unsorted]]\n",
	"assets/diagram.png":              "png",
	".git/HEAD.md":                    "- must be skipped [[Ghost]]\n",
	".vault-curator/state.md":         "- must be skipped [[Ghost]]\n",
	"external.md":                     "- [site](https://example.com) and [mail](mailto:a@b.c) -> [[Unsorted]]\n",
}

func load(t *testing.T) *Vault {
	t.Helper()
	v, err := Load(writeVault(t, fixture), Options{})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLoadSkipsToolDirs(t *testing.T) {
	v := load(t)
	if len(v.Notes) != 11 {
		var ps []string
		for _, n := range v.Notes {
			ps = append(ps, n.Path)
		}
		t.Fatalf("loaded %d notes: %v", len(v.Notes), ps)
	}
	if v.Note("daily/2026-10-17.md") == nil || v.Note("DAILY/2026-10-17") == nil {
		t.Error("lookup by path failed")
	}
	if !reflect.DeepEqual(v.Attachments, []string{"assets/diagram.png"}) {
		t.Errorf("attachments = %v", v.Attachments)
	}
}

func TestResolveByTitleAndAlias(t *testing.T) {
	v := load(t)
	cases := map[string]string{
		"SMP":                  "projects/SMP.md",
		"smp":                  "projects/SMP.md",
		"Smart Meter Platform": "projects/SMP.md",
		"the SMP project":      "",
		"SMP project":          "projects/SMP.md",
		"Parser.md":            "components/Parser.md",
		"people/alice":         "people/Alice.md",
		"Alice":                "people/Alice.md",
		"Nobody":               "",
		"Ghost":                "",
	}
	for target, want := range cases {
		got := ""
		if n := v.Resolve(target); n != nil {
			got = n.Path
		}
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", target, got, want)
		}
	}
}

func TestResolveTitleBeatsAliasAndLowestPathWins(t *testing.T) {
	root := writeVault(t, map[string]string{
		"b/Dup.md":  "",
		"a/Dup.md":  "",
		"Other.md":  "---\naliases: [Dup]\n---\n",
		"Target.md": "",
		"Alias.md":  "---\naliases: [Target]\n---\n",
	})
	v, err := Load(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if n := v.Resolve("Dup"); n == nil || n.Path != "a/Dup.md" {
		t.Errorf("Resolve(Dup) = %v", n)
	}
	if n := v.Resolve("Target"); n == nil || n.Path != "Target.md" {
		t.Errorf("Resolve(Target) = %v", n)
	}
}

func TestBrokenLinks(t *testing.T) {
	v := load(t)
	var got []string
	for _, b := range v.Broken {
		got = append(got, b.From+" -> "+b.Link.Raw)
	}
	want := []string{
		"components/Parser.md -> [[the smp project]]",
		"daily/2026-10-17.md -> [[Nobody]]",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("broken = %v, want %v", got, want)
	}
}

func TestGraphAndBacklinks(t *testing.T) {
	v := load(t)
	var out []string
	for _, e := range v.Outgoing("daily/2026-10-17.md") {
		out = append(out, e.To)
	}
	wantOut := []string{
		"meetings/2026-10-17 SMP sync.md",
		"specs/Spec.md",
		"projects/SMP.md",
		"daily/2026-10-17.md",
	}
	if !reflect.DeepEqual(out, wantOut) {
		t.Errorf("outgoing = %v, want %v", out, wantOut)
	}
	var in []string
	for _, e := range v.Backlinks("projects/SMP.md") {
		in = append(in, e.From)
	}
	wantIn := []string{"daily/2026-10-17.md", "meetings/2026-10-17 SMP sync.md"}
	if !reflect.DeepEqual(in, wantIn) {
		t.Errorf("backlinks = %v, want %v", in, wantIn)
	}
	if bl := v.Backlinks("meetings/2026-10-17 SMP sync.md"); len(bl) != 1 || bl[0].Link.Raw != "[[2026-10-17 SMP sync]]" {
		t.Errorf("meeting backlinks = %+v", bl)
	}
}

func TestEntities(t *testing.T) {
	v := load(t)
	got := map[string]bool{}
	for _, e := range v.Entities {
		got[e.Title] = e.Hub
	}
	want := map[string]bool{"SMP": true, "Parser": true, "Alice": false, "Bob": false, "Unsorted": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("entities (title: hub) = %v, want %v", got, want)
	}
}

func TestOrphans(t *testing.T) {
	v := load(t)
	// Reach a hub: Parser (is one), Alice -> Parser, meeting -> SMP via
	// frontmatter, daily -> meeting, Parked -> Unsorted, external -> Unsorted.
	// Bob links nowhere; Spec links nowhere; Idea only reaches Spec.
	want := []string{"loose/Idea.md", "people/Bob.md", "specs/Spec.md"}
	if !reflect.DeepEqual(v.Orphans, want) {
		t.Errorf("orphans = %v, want %v", v.Orphans, want)
	}

	custom := New(v.Notes, nil, Options{FallbackHub: "Parked"})
	if custom.FallbackHub() != "Parked" {
		t.Errorf("fallback = %q", custom.FallbackHub())
	}
	for _, o := range custom.Orphans {
		if o == "filed/Parked.md" {
			t.Error("custom fallback hub reported as orphan")
		}
	}
}
