package note

import (
	"bytes"
	"strings"
	"testing"
)

const messyDaily = "---\n" +
	"type: daily\n" +
	"date: 2026-10-17\n" +
	"aliases: Oct 17\n" +
	"project: \"[[SMP]]\"\n" +
	"---\n" +
	"# Friday\n" +
	"\n" +
	"- standup with [[Alice|Al]]\n" +
	"  - shipped the [[SMP#Parser]] fix\n" +
	"\t* review `[[not a link]]` tomorrow\n" +
	"\n" +
	"    continued after a blank line\n" +
	"\n" +
	" - SMP sync (inconsistent top indent)\n" +
	"    1. decided to drop [the cache](notes/Cache%20design.md#Why)\n" +
	"    2) see https://example.com and [docs](https://example.com/docs)\n" +
	"Some loose text\n" +
	"\n" +
	"```mermaid\n" +
	"flowchart LR\n" +
	"  A[[subroutine]] --> B\n" +
	"```\n" +
	"<!-- vault-curator:begin timeline -->\n" +
	"- [[2026-10-16 SMP sync]]\n" +
	"<!-- vault-curator:end timeline -->\n" +
	"- last block\r\n" +
	"  - crlf child\r\n" +
	"- no trailing newline"

func TestRoundTripIsByteExact(t *testing.T) {
	inputs := map[string]string{
		"messy daily":          messyDaily,
		"empty":                "",
		"only newline":         "\n",
		"no frontmatter close": "---\ntype: daily\n- a\n",
		"bad yaml":             "---\ntype: [unclosed\n---\n- a\n",
		"unclosed fence":       "- a\n```go\n- not a block\n",
		"unmatched marker":     "<!-- vault-curator:begin x -->\n- a\n",
		"bom":                  "\ufeff---\ntype: meeting\n---\nbody\n",
		"crlf":                 "# T\r\n\r\n- a\r\n\t- b\r\n",
		"trailing blanks":      "- a\n\n\n",
	}
	for name, in := range inputs {
		t.Run(name, func(t *testing.T) {
			n := Parse("x.md", []byte(in))
			if got := n.Bytes(); !bytes.Equal(got, []byte(in)) {
				t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, in)
			}
			pos, line := 0, 1
			for _, s := range n.Segments {
				if s.Start != pos || s.StartLine != line || s.End <= s.Start {
					t.Fatalf("segment %+v does not continue at byte %d line %d", s, pos, line)
				}
				pos, line = s.End, s.EndLine+1
			}
			if pos != len(in) {
				t.Fatalf("segments end at %d, want %d", pos, len(in))
			}
		})
	}
}

func kinds(n *Note) string {
	var ks []string
	for _, s := range n.Segments {
		ks = append(ks, string(s.Kind))
	}
	return strings.Join(ks, " ")
}

func TestSegmentsAndBlocks(t *testing.T) {
	n := Parse("daily/2026-10-17.md", []byte(messyDaily))
	want := "frontmatter heading text block text block text mermaid generated block block"
	if got := kinds(n); got != want {
		t.Fatalf("kinds:\n got %s\nwant %s", got, want)
	}
	if n.Title != "2026-10-17" {
		t.Errorf("title = %q", n.Title)
	}
	if len(n.Blocks) != 4 {
		t.Fatalf("got %d blocks, want 4", len(n.Blocks))
	}
	b := n.Blocks[0]
	if b.StartLine != 9 || b.EndLine != 13 || b.Heading != "Friday" {
		t.Errorf("block 0 = lines %d-%d under %q", b.StartLine, b.EndLine, b.Heading)
	}
	if len(b.Items) != 3 {
		t.Fatalf("block 0 items = %d, want 3", len(b.Items))
	}
	if d := []int{b.Items[0].Depth, b.Items[1].Depth, b.Items[2].Depth}; d[0] != 0 || d[1] != 1 || d[2] != 2 {
		t.Errorf("depths = %v, want [0 1 2] (a tab is four columns)", d)
	}
	if got := b.Items[2].Normalized; got != "review `[[not a link]]` tomorrow continued after a blank line" {
		t.Errorf("normalized continuation = %q", got)
	}
	if got := n.Blocks[1].Items; len(got) != 3 || got[1].Normalized != "decided to drop [the cache](notes/cache%20design.md#why)" {
		t.Errorf("block 1 items = %+v", got)
	}
	if b := n.Blocks[2]; b.StartLine != 27 || b.EndLine != 28 || n.Blocks[3].Text != "- no trailing newline" {
		t.Errorf("crlf block lines %d-%d, last block %q", b.StartLine, b.EndLine, n.Blocks[3].Text)
	}
	if n.Segments[8].Name != "timeline" {
		t.Errorf("generated name = %q", n.Segments[8].Name)
	}
}

func TestFencedCodeInsideBlockStaysInBlock(t *testing.T) {
	src := "- a\n  ```\n# not a heading\n- not a bullet\n  ```\n- b\n"
	n := Parse("x.md", []byte(src))
	if got := kinds(n); got != "block block" {
		t.Fatalf("kinds = %s", got)
	}
	if n.Blocks[0].EndLine != 5 || len(n.Headings) != 0 {
		t.Errorf("block 0 ends at %d, headings %v", n.Blocks[0].EndLine, n.Headings)
	}
}

func TestFrontmatter(t *testing.T) {
	n := Parse("x.md", []byte(messyDaily))
	fm := n.Frontmatter
	if !fm.Present || fm.Err != nil || n.Type() != "daily" || fm.Date != "2026-10-17" {
		t.Fatalf("frontmatter = %+v", fm)
	}
	if len(fm.Aliases) != 1 || fm.Aliases[0] != "Oct 17" || len(fm.Project) != 1 || fm.Project[0] != "[[SMP]]" {
		t.Errorf("aliases %v project %v", fm.Aliases, fm.Project)
	}
	if fm.Fields["date"] != "2026-10-17" {
		t.Errorf("fields date = %#v", fm.Fields["date"])
	}

	list := Parse("x.md", []byte("---\ntype: Project\naliases:\n  - SMP\n  - the SMP project\n---\n"))
	if list.Type() != "project" || len(list.Frontmatter.Aliases) != 2 {
		t.Errorf("list frontmatter = %+v", list.Frontmatter)
	}

	bad := Parse("x.md", []byte("---\ntype: [unclosed\n---\n- a\n"))
	if !bad.Frontmatter.Present || bad.Frontmatter.Err == nil || len(bad.Blocks) != 1 {
		t.Errorf("bad yaml: %+v, %d blocks", bad.Frontmatter, len(bad.Blocks))
	}

	none := Parse("x.md", []byte("- a\n---\n"))
	if none.Frontmatter.Present {
		t.Error("frontmatter detected without a leading ---")
	}
}

func TestBlockHashIgnoresCheckboxState(t *testing.T) {
	hash := func(src string) string {
		return Parse("x.md", []byte(src)).Blocks[0].Hash
	}
	open := hash("- [ ] Send the report [owner:: [[Alice]]]\n- [ ] Book room\n")
	for _, s := range []string{
		"- [x] Send the report [owner:: [[Alice]]]\n- [ ] Book room\n",
		"- [X] Send the report [owner:: [[Alice]]]\n- [X] Book room\n",
		"* [x]   send the report [owner:: [[Alice]]]\n- [x] Book room\n",
	} {
		if h := hash(s); h != open {
			t.Errorf("%q hashes %s, want %s", s, h, open)
		}
	}
	if h := hash("- [ ] Send the reports [owner:: [[Alice]]]\n- [ ] Book room\n"); h == open {
		t.Error("different text hashes the same")
	}
	n := Parse("x.md", []byte("- [x] Book room\n"))
	if got := n.Blocks[0].Items[0].Normalized; got != "[x] book room" {
		t.Errorf("Normalized = %q, want checkbox kept", got)
	}
}

func TestBlockHashIgnoresFormatting(t *testing.T) {
	hash := func(src string) string {
		t.Helper()
		n := Parse("x.md", []byte(src))
		if len(n.Blocks) != 1 {
			t.Fatalf("%q: got %d blocks", src, len(n.Blocks))
		}
		return n.Blocks[0].Hash
	}
	base := hash("- Met with Alice\n  - Decided to ship\n")
	same := []string{
		"* met with alice\n\t+ decided to ship\n",
		"1. MET   WITH ALICE\n    2) Decided to ship\n",
		"-   Met with Alice\n        - Decided\n          to ship\n",
		"- Met with Alice\r\n\r\n  - Decided to ship  \r\n",
	}
	for _, s := range same {
		if h := hash(s); h != base {
			t.Errorf("%q hashes %s, want %s", s, h, base)
		}
	}
	diff := []string{
		"- Met with Bob\n  - Decided to ship\n",
		"- Met with Alice\n  - Decided not to ship\n",
		"- Met with Alice\n",
		"- Met with Alice Decided to ship\n",
		"- [x] Met with Alice\n  - Decided to ship\n",
	}
	for _, s := range diff {
		if h := hash(s); h == base {
			t.Errorf("%q hashes the same as the base block", s)
		}
	}
}

func TestBlockBoundaries(t *testing.T) {
	cases := []struct {
		src    string
		blocks int
	}{
		{"- a\n- b\n", 2},
		{"- a\n - b\n", 2},       // one-space slip is a sibling
		{"- a\n  - b\n", 1},      // two columns deeper is a child
		{"- a\n\t- b\n", 1},      // tab child
		{"- a\n\n- b\n", 2},      // blank line then top-level bullet
		{"- a\n\n  - b\n", 1},    // blank line then child
		{"- a\ntext\n- b\n", 2},  // unindented text ends a block
		{"- a\n# H\n  - b\n", 2}, // heading ends a block; indented bullet after it starts one
		{"---\n- a\n", 1},        // unclosed frontmatter is text
		{"-not a bullet\n1.5 nor this\n", 0},
	}
	for _, c := range cases {
		if n := Parse("x.md", []byte(c.src)); len(n.Blocks) != c.blocks {
			t.Errorf("%q: got %d blocks, want %d", c.src, len(n.Blocks), c.blocks)
		}
	}
}

func TestLinks(t *testing.T) {
	n := Parse("daily/x.md", []byte(messyDaily))
	type want struct {
		kind              LinkKind
		target, frag, lbl string
		ext, fm           bool
	}
	wants := []want{
		{Wikilink, "SMP", "", "", false, true},
		{Wikilink, "Alice", "", "Al", false, false},
		{Wikilink, "SMP", "Parser", "", false, false},
		{Markdown, "notes/Cache design.md", "Why", "the cache", false, false},
		{Markdown, "https://example.com/docs", "", "docs", true, false},
		{Wikilink, "2026-10-16 SMP sync", "", "", false, false},
	}
	if len(n.Links) != len(wants) {
		t.Fatalf("got %d links %+v, want %d", len(n.Links), n.Links, len(wants))
	}
	for i, w := range wants {
		l := n.Links[i]
		if l.Kind != w.kind || l.Target != w.target || l.Fragment != w.frag || l.Label != w.lbl || l.External != w.ext || l.InFrontmatter != w.fm {
			t.Errorf("link %d = %+v, want %+v", i, l, w)
		}
		if string(n.Src[l.Start:l.End]) != l.Raw {
			t.Errorf("link %d offsets [%d,%d) do not cover %q", i, l.Start, l.End, l.Raw)
		}
	}

	more := ParseLinks("![[diagram.png]] [[#Local]] [x](#frag) [[Table\\|alias]] [y](<a b.md>)")
	if len(more) != 5 {
		t.Fatalf("got %d links: %+v", len(more), more)
	}
	if !more[0].Embed || more[0].Target != "diagram.png" {
		t.Errorf("embed = %+v", more[0])
	}
	if more[1].Target != "" || more[1].Fragment != "Local" || more[2].Target != "" || more[2].Fragment != "frag" {
		t.Errorf("same-note links = %+v %+v", more[1], more[2])
	}
	if more[3].Target != "Table" || more[3].Label != "alias" || more[4].Target != "a b.md" {
		t.Errorf("table link %+v, angle link %+v", more[3], more[4])
	}
}

func TestRewrite(t *testing.T) {
	n := Parse("x.md", []byte(messyDaily))
	b := n.Blocks[1]
	out, err := n.Rewrite(
		Edit{Start: b.Start, End: b.End, Text: "- [[2026-10-17 SMP sync]]\n"},
		Edit{Start: 0, End: 0, Text: ""},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := messyDaily[:b.Start] + "- [[2026-10-17 SMP sync]]\n" + messyDaily[b.End:]
	if string(out) != want {
		t.Fatalf("rewrite =\n%s", out)
	}
	if _, err := n.Rewrite(Edit{Start: 0, End: 10}, Edit{Start: 5, End: 12}); err == nil {
		t.Error("overlapping edits accepted")
	}
	if _, err := n.Rewrite(Edit{Start: 0, End: len(messyDaily) + 1}); err == nil {
		t.Error("out-of-range edit accepted")
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"- Foo  Bar":           "foo bar",
		"\t12. Foo\n   bar":    "foo bar",
		"- [ ] Call Bob":       "[ ] call bob",
		"-- not a marker":      "-- not a marker",
		"*emphasis* not a dot": "*emphasis* not a dot",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
