package note

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// LinkKind is the syntax a link was written in.
type LinkKind string

// Link kinds.
const (
	// Wikilink is [[target]], [[target|label]] or [[target#heading]].
	Wikilink LinkKind = "wikilink"
	// Markdown is [label](target).
	Markdown LinkKind = "markdown"
)

// Link is one link found in a note.
type Link struct {
	Kind LinkKind `json:"kind"`
	// Embed reports a leading "!" (![[x]] or ![alt](x)).
	Embed bool `json:"embed,omitempty"`
	// Target is the linked note or file, without fragment, URL-decoded for
	// markdown links. It is empty for a link to a heading in the same note
	// ([[#heading]] or [x](#heading)).
	Target string `json:"target"`
	// Fragment is the part after "#": a heading or a block reference (^id).
	Fragment string `json:"fragment,omitempty"`
	// Label is a wikilink's alias or a markdown link's text.
	Label string `json:"label,omitempty"`
	// External reports a markdown link with a URL scheme (https:, mailto:,
	// obsidian:, ...). External links are never resolved against the vault.
	External bool `json:"external,omitempty"`
	// InFrontmatter reports a link written inside the frontmatter block.
	InFrontmatter bool `json:"in_frontmatter,omitempty"`
	// Raw is the link exactly as written.
	Raw string `json:"raw"`
	// Start and End are byte offsets of Raw in the parsed text.
	Start int `json:"start"`
	End   int `json:"end"`
	// Line is 1-based.
	Line int `json:"line"`
}

var (
	wikiRe   = regexp.MustCompile(`(!?)\[\[([^\[\]\n]+)\]\]`)
	mdRe     = regexp.MustCompile(`(!?)\[((?:[^\[\]\n]|\[[^\[\]\n]*\])*)\]\(\s*(<[^<>\n]*>|[^\s()]*(?:\([^\s()]*\)[^\s()]*)*)(?:\s+(?:"[^"\n]*"|'[^'\n]*'))?\s*\)`)
	schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// ParseLinks returns the links in markdown text, skipping fenced code blocks
// and inline code spans. Offsets are relative to text. Use it to check links
// in text that is not yet a note, such as a plan's proposed content.
func ParseLinks(text string) []Link {
	return extractLinks([]byte(text), 0)
}

func extractLinks(src []byte, base int) []Link {
	var links []Link
	var f fence
	inFence := false
	for k, l := range splitLines(src) {
		if inFence {
			inFence = !f.closes(l.text)
			continue
		}
		_, rest := indent(l.text)
		if f, inFence = openFence(rest); inFence {
			continue
		}
		t := maskCode(l.text)
		off := base + l.start
		for _, m := range wikiRe.FindAllStringSubmatchIndex(t, -1) {
			links = append(links, wikiLink(l.text, m, off, k+1))
		}
		for _, m := range mdRe.FindAllStringSubmatchIndex(t, -1) {
			links = append(links, mdLink(l.text, m, off, k+1))
		}
	}
	sortLinks(links)
	return links
}

func wikiLink(t string, m []int, off, line int) Link {
	inner := t[m[4]:m[5]]
	target, label, _ := strings.Cut(inner, "|")
	target = strings.TrimSuffix(target, `\`) // escaped pipe inside a table
	target, frag, _ := strings.Cut(target, "#")
	return Link{
		Kind:     Wikilink,
		Embed:    m[3] > m[2],
		Target:   strings.TrimSpace(target),
		Fragment: strings.TrimSpace(frag),
		Label:    strings.TrimSpace(label),
		Raw:      t[m[0]:m[1]],
		Start:    off + m[0],
		End:      off + m[1],
		Line:     line,
	}
}

func mdLink(t string, m []int, off, line int) Link {
	dest := t[m[6]:m[7]]
	dest = strings.TrimSuffix(strings.TrimPrefix(dest, "<"), ">")
	l := Link{
		Kind:  Markdown,
		Embed: m[3] > m[2],
		Label: t[m[4]:m[5]],
		Raw:   t[m[0]:m[1]],
		Start: off + m[0],
		End:   off + m[1],
		Line:  line,
	}
	if schemeRe.MatchString(dest) {
		l.External = true
		l.Target = dest
		return l
	}
	target, frag, _ := strings.Cut(dest, "#")
	if dec, err := url.PathUnescape(target); err == nil {
		target = dec
	}
	l.Target = strings.TrimSpace(target)
	l.Fragment = frag
	return l
}

// maskCode replaces inline code spans (matched backtick runs on one line)
// with spaces, keeping byte offsets.
func maskCode(t string) string {
	if !strings.Contains(t, "`") {
		return t
	}
	b := []byte(t)
	for i := 0; i < len(b); {
		if b[i] != '`' {
			i++
			continue
		}
		n := 0
		for i+n < len(b) && b[i+n] == '`' {
			n++
		}
		end := -1
		for j := i + n; j < len(b); {
			if b[j] != '`' {
				j++
				continue
			}
			m := 0
			for j+m < len(b) && b[j+m] == '`' {
				m++
			}
			if m == n {
				end = j + m
				break
			}
			j += m
		}
		if end < 0 {
			i += n
			continue
		}
		for k := i; k < end; k++ {
			b[k] = ' '
		}
		i = end
	}
	return string(b)
}

func sortLinks(links []Link) {
	sort.SliceStable(links, func(i, j int) bool { return links[i].Start < links[j].Start })
}
