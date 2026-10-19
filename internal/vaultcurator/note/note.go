// Package note parses a single vault note: frontmatter, headings, top-level
// bullet blocks with stable hashes, links, and the regions that must survive
// curation byte for byte (generated blocks, user-drawn Mermaid, other code).
//
// Parsing never fails and never loses bytes: a note's Segments tile its
// source exactly, so Bytes reproduces the input and Rewrite can replace any
// region by byte offsets.
package note

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Note is one parsed markdown file.
type Note struct {
	// Path is the vault-relative path with forward slashes, e.g.
	// "meetings/2026-10-17 SMP sync.md". It is whatever Parse was given.
	Path string `json:"path"`
	// Title is the file name without its .md extension. Wikilinks resolve
	// against it.
	Title string `json:"title"`
	// Src is the raw file content.
	Src []byte `json:"-"`

	Frontmatter Frontmatter `json:"frontmatter"`
	// Segments tile Src in order: concatenating their bytes yields Src.
	Segments []Segment `json:"segments"`
	Headings []Heading `json:"headings"`
	// Blocks are the top-level bullet blocks, in order. Each also appears in
	// Segments with Kind KindBlock.
	Blocks []Block `json:"blocks"`
	// Links are every wikilink and markdown link outside code, in order,
	// including those in frontmatter.
	Links []Link `json:"links"`
}

// Kind classifies a Segment.
type Kind string

// Segment kinds.
const (
	// KindFrontmatter is the YAML block between the opening and closing
	// "---" lines, both included.
	KindFrontmatter Kind = "frontmatter"
	// KindHeading is a single ATX heading line ("# Title").
	KindHeading Kind = "heading"
	// KindBlock is a top-level bullet plus everything nested under it.
	KindBlock Kind = "block"
	// KindGenerated is a CLI-owned region between GeneratedBegin and
	// GeneratedEnd marker comments, both included.
	KindGenerated Kind = "generated"
	// KindMermaid is a top-level fenced code block with info string
	// "mermaid", drawn by the user and never regenerated.
	KindMermaid Kind = "mermaid"
	// KindCode is any other top-level fenced code block.
	KindCode Kind = "code"
	// KindText is everything else: paragraphs, blank lines, tables, quotes.
	KindText Kind = "text"
)

// Segment is a contiguous byte range of a note's source.
type Segment struct {
	Kind Kind `json:"kind"`
	// Start and End are byte offsets into Note.Src, End exclusive. End
	// includes the final line terminator.
	Start int `json:"start"`
	End   int `json:"end"`
	// StartLine and EndLine are 1-based and inclusive.
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
	// Name is the generated block's name for KindGenerated, empty otherwise.
	Name string `json:"name,omitempty"`
}

// Heading is an ATX heading.
type Heading struct {
	// Level is 1 to 6.
	Level int `json:"level"`
	// Text is the heading text without the leading and closing #s.
	Text string `json:"text"`
	// Line is 1-based.
	Line  int `json:"line"`
	Start int `json:"start"`
	End   int `json:"end"`
}

// Block is a top-level bullet plus everything nested under it: the unit scan
// tracks and the agent restructures.
type Block struct {
	// Start and End are byte offsets into Note.Src, End exclusive.
	Start int `json:"start"`
	End   int `json:"end"`
	// StartLine and EndLine are 1-based and inclusive.
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
	// Heading is the text of the nearest heading above the block, or "".
	Heading string `json:"heading,omitempty"`
	// Text is the block's raw source.
	Text string `json:"text"`
	// Items are the bullets in the block, the top-level one first.
	Items []Item `json:"items"`
	// Hash identifies the block's normalized content (see Normalize). It is
	// unchanged by reindenting, changing bullet markers, reflowing lines or
	// changing case.
	Hash string `json:"hash"`
}

// Item is one bullet at any depth in a Block, with its continuation lines.
type Item struct {
	// Line is the 1-based line of the bullet.
	Line int `json:"line"`
	// Depth is 0 for the block's top bullet, 1 for its children, and so on.
	Depth int `json:"depth"`
	// Text is the bullet's raw lines (marker included), joined with "\n",
	// without blank lines or line terminators.
	Text string `json:"text"`
	// Normalized is Normalize(Text): the form the lossless check compares.
	Normalized string `json:"normalized"`
}

// Parse parses src as the note at the vault-relative path p. It never fails:
// malformed frontmatter is reported in Frontmatter.Err and the rest of the
// note is still parsed.
func Parse(p string, src []byte) *Note {
	n := &Note{Path: p, Title: titleOf(p), Src: src}
	parse(n)
	n.Links = extractLinks(src, 0)
	if fm := n.frontmatterSegment(); fm != nil {
		for i := range n.Links {
			if n.Links[i].Start < fm.End {
				n.Links[i].InFrontmatter = true
			}
		}
	}
	return n
}

// titleOf returns the file name of p without a .md extension.
func titleOf(p string) string {
	base := path.Base(p)
	if ext := path.Ext(base); strings.EqualFold(ext, ".md") {
		base = base[:len(base)-len(ext)]
	}
	return base
}

func (n *Note) frontmatterSegment() *Segment {
	if len(n.Segments) > 0 && n.Segments[0].Kind == KindFrontmatter {
		return &n.Segments[0]
	}
	return nil
}

// Type returns the note's frontmatter type, lowercased ("" if none).
func (n *Note) Type() string {
	return strings.ToLower(strings.TrimSpace(n.Frontmatter.Type))
}

// Bytes re-serializes the note from its segments. For a parsed note it is
// byte-identical to Src.
func (n *Note) Bytes() []byte {
	out := make([]byte, 0, len(n.Src))
	for _, s := range n.Segments {
		out = append(out, n.Src[s.Start:s.End]...)
	}
	return out
}

// SegmentText returns the source bytes of s as a string.
func (n *Note) SegmentText(s Segment) string {
	return string(n.Src[s.Start:s.End])
}

// Edit replaces Src[Start:End] with Text. Start == End inserts.
type Edit struct {
	Start int
	End   int
	Text  string
}

// Rewrite returns Src with the edits applied. Edits may come in any order but
// must lie within Src and must not overlap; insertions at the same offset are
// applied in the order given. Every byte outside the edits is kept as is.
// The Note itself is not modified; parse the result to get a fresh Note.
func (n *Note) Rewrite(edits ...Edit) ([]byte, error) {
	sorted := make([]Edit, len(edits))
	copy(sorted, edits)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	var b strings.Builder
	pos := 0
	for _, e := range sorted {
		if e.Start < 0 || e.End < e.Start || e.End > len(n.Src) {
			return nil, fmt.Errorf("edit [%d,%d) out of range for %d bytes", e.Start, e.End, len(n.Src))
		}
		if e.Start < pos {
			return nil, fmt.Errorf("edit [%d,%d) overlaps a previous edit ending at %d", e.Start, e.End, pos)
		}
		b.Write(n.Src[pos:e.Start])
		b.WriteString(e.Text)
		pos = e.End
	}
	b.Write(n.Src[pos:])
	return []byte(b.String()), nil
}
