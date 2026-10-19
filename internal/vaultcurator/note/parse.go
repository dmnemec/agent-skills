package note

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
)

// Generated-block markers. A generated block is the lines from a begin marker
// to the matching end marker with the same name, both included:
//
//	<!-- vault-curator:begin timeline -->
//	...
//	<!-- vault-curator:end timeline -->
//
// A begin marker with no matching end marker is ordinary text.
const (
	GeneratedBegin = "<!-- vault-curator:begin %s -->"
	GeneratedEnd   = "<!-- vault-curator:end %s -->"
)

var (
	genBeginRe = regexp.MustCompile(`^\s*<!--\s*vault-curator:begin\s+(\S+)\s*-->\s*$`)
	genEndRe   = regexp.MustCompile(`^\s*<!--\s*vault-curator:end\s+(\S+)\s*-->\s*$`)
	headingRe  = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
)

// tabWidth is the column width a tab advances indentation to.
const tabWidth = 4

// line is one source line. end includes the terminator; text excludes it.
type line struct {
	start, end int
	text       string
}

func splitLines(src []byte) []line {
	var ls []line
	for i := 0; i < len(src); {
		end := len(src)
		if j := bytes.IndexByte(src[i:], '\n'); j >= 0 {
			end = i + j + 1
		}
		t := bytes.TrimSuffix(src[i:end], []byte("\n"))
		t = bytes.TrimSuffix(t, []byte("\r"))
		ls = append(ls, line{start: i, end: end, text: string(t)})
		i = end
	}
	return ls
}

// indent returns the indentation column of s (tabs advance to the next
// multiple of tabWidth) and the rest of s after it.
func indent(s string) (int, string) {
	col := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			col++
		case '\t':
			col = (col/tabWidth + 1) * tabWidth
		default:
			return col, s[i:]
		}
	}
	return col, ""
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

// bulletMarker reports the length of the list marker at the start of s:
// "-", "*", "+", or 1-9 digits followed by "." or ")", then whitespace or the
// end of the line.
func bulletMarker(s string) (int, bool) {
	n := 0
	switch {
	case s == "":
		return 0, false
	case s[0] == '-' || s[0] == '*' || s[0] == '+':
		n = 1
	default:
		for n < len(s) && n < 9 && s[n] >= '0' && s[n] <= '9' {
			n++
		}
		if n == 0 || n >= len(s) || (s[n] != '.' && s[n] != ')') {
			return 0, false
		}
		n++
	}
	if n < len(s) && !unicode.IsSpace(rune(s[n])) {
		return 0, false
	}
	return n, true
}

// fence describes an open fenced code block.
type fence struct {
	char  byte
	count int
	info  string
}

// openFence reports whether rest (a line after its indentation) opens a
// fenced code block.
func openFence(rest string) (fence, bool) {
	if len(rest) < 3 || (rest[0] != '`' && rest[0] != '~') {
		return fence{}, false
	}
	c := rest[0]
	n := 0
	for n < len(rest) && rest[n] == c {
		n++
	}
	if n < 3 {
		return fence{}, false
	}
	info := strings.TrimSpace(rest[n:])
	if c == '`' && strings.Contains(info, "`") {
		return fence{}, false
	}
	return fence{char: c, count: n, info: info}, true
}

// closes reports whether text closes f.
func (f fence) closes(text string) bool {
	t := strings.TrimSpace(text)
	if len(t) < f.count {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] != f.char {
			return false
		}
	}
	return true
}

func parseHeading(text string) (int, string, bool) {
	m := headingRe.FindStringSubmatch(text)
	if m == nil {
		return 0, "", false
	}
	return len(m[1]), strings.TrimSpace(m[2]), true
}

type parser struct {
	n       *Note
	ls      []line
	text    int // first line of the pending text segment, or -1
	heading string
}

func parse(n *Note) {
	p := &parser{n: n, ls: splitLines(n.Src), text: -1}
	i := p.frontmatter()
	for i < len(p.ls) {
		i = p.step(i)
	}
	p.flushText(len(p.ls))
}

// frontmatter parses a leading frontmatter block and returns the first body
// line.
func (p *parser) frontmatter() int {
	if len(p.ls) == 0 || strings.TrimPrefix(p.ls[0].text, "\ufeff") != "---" {
		return 0
	}
	for j := 1; j < len(p.ls); j++ {
		if t := strings.TrimRight(p.ls[j].text, " \t"); t == "---" || t == "..." {
			p.emit(KindFrontmatter, 0, j, "")
			body := p.n.Src[p.ls[0].end:p.ls[j].start]
			p.n.Frontmatter = parseFrontmatter(body)
			return j + 1
		}
	}
	return 0
}

// step consumes the construct starting at line i and returns the next line.
func (p *parser) step(i int) int {
	t := p.ls[i].text
	col, rest := indent(t)
	if m := genBeginRe.FindStringSubmatch(t); m != nil {
		for j := i + 1; j < len(p.ls); j++ {
			if e := genEndRe.FindStringSubmatch(p.ls[j].text); e != nil && e[1] == m[1] {
				p.emit(KindGenerated, i, j, m[1])
				return j + 1
			}
		}
	}
	if col <= 3 {
		if f, ok := openFence(rest); ok {
			j := i + 1
			for j < len(p.ls) && !f.closes(p.ls[j].text) {
				j++
			}
			if j == len(p.ls) {
				j-- // an unclosed fence runs to the end of the note
			}
			kind := KindCode
			if word, _, _ := strings.Cut(f.info, " "); strings.EqualFold(word, "mermaid") {
				kind = KindMermaid
			}
			p.emit(kind, i, j, "")
			return j + 1
		}
		if level, text, ok := parseHeading(t); ok {
			p.emit(KindHeading, i, i, "")
			p.n.Headings = append(p.n.Headings, Heading{
				Level: level, Text: text, Line: i + 1,
				Start: p.ls[i].start, End: p.ls[i].end,
			})
			p.heading = text
			return i + 1
		}
	}
	if _, ok := bulletMarker(rest); ok {
		j := p.blockEnd(i, col)
		p.emit(KindBlock, i, j, "")
		p.n.Blocks = append(p.n.Blocks, p.block(i, j))
		return j + 1
	}
	if p.text < 0 {
		p.text = i
	}
	return i + 1
}

// blockEnd returns the last line of the block whose top bullet is line i at
// column top. A line belongs to the block if it is a bullet indented at least
// two columns deeper than the top bullet, any other line indented deeper than
// the top bullet, or a line inside a fence opened in the block. Blank lines
// belong only when a later line does; an unindented non-bullet line ends the
// block.
func (p *parser) blockEnd(i, top int) int {
	last := i
	var f fence
	inFence := false
	for j := i + 1; j < len(p.ls); j++ {
		t := p.ls[j].text
		if inFence {
			last = j
			inFence = !f.closes(t)
			continue
		}
		if isBlank(t) {
			continue
		}
		col, rest := indent(t)
		if _, ok := bulletMarker(rest); ok {
			if col < top+2 {
				break
			}
		} else if col <= top {
			break
		}
		last = j
		f, inFence = openFence(rest)
	}
	return last
}

// block builds the Block for lines i..j.
func (p *parser) block(i, j int) Block {
	b := Block{
		Start: p.ls[i].start, End: p.ls[j].end,
		StartLine: i + 1, EndLine: j + 1,
		Heading: p.heading,
		Text:    string(p.n.Src[p.ls[i].start:p.ls[j].end]),
	}
	var stack []int // indentation columns of open ancestors
	var f fence
	inFence := false
	for k := i; k <= j; k++ {
		t := p.ls[k].text
		if inFence {
			appendLine(&b, t)
			inFence = !f.closes(t)
			continue
		}
		if isBlank(t) {
			continue
		}
		col, rest := indent(t)
		if _, ok := bulletMarker(rest); ok {
			for len(stack) > 0 && stack[len(stack)-1] >= col {
				stack = stack[:len(stack)-1]
			}
			b.Items = append(b.Items, Item{Line: k + 1, Depth: len(stack), Text: t})
			stack = append(stack, col)
		} else {
			appendLine(&b, t)
		}
		f, inFence = openFence(rest)
	}
	norm := make([]string, len(b.Items))
	for k := range b.Items {
		b.Items[k].Normalized = Normalize(b.Items[k].Text)
		norm[k] = uncheck(b.Items[k].Normalized)
	}
	b.Hash = hashText(strings.Join(norm, "\n"))
	return b
}

func appendLine(b *Block, t string) {
	it := &b.Items[len(b.Items)-1]
	it.Text += "\n" + t
}

// emit records a segment for lines i..j, first flushing pending text.
func (p *parser) emit(k Kind, i, j int, name string) {
	p.flushText(i)
	p.n.Segments = append(p.n.Segments, Segment{
		Kind: k, Start: p.ls[i].start, End: p.ls[j].end,
		StartLine: i + 1, EndLine: j + 1, Name: name,
	})
}

// flushText emits the pending text segment, which ends before line i.
func (p *parser) flushText(i int) {
	if p.text < 0 || p.text >= i {
		p.text = -1
		return
	}
	s, e := p.text, i-1
	p.text = -1
	p.n.Segments = append(p.n.Segments, Segment{
		Kind: KindText, Start: p.ls[s].start, End: p.ls[e].end,
		StartLine: s + 1, EndLine: e + 1,
	})
}

// Normalize returns the comparison form of bullet text: on each line, leading
// whitespace and one list marker are removed, then the text is lowercased and
// every run of whitespace (line breaks included) becomes a single space.
// Checkbox state ("[ ]", "[x]") and all other punctuation are kept here; only
// the block hash ignores checkbox state.
func Normalize(s string) string {
	var words []string
	for _, ln := range strings.Split(s, "\n") {
		rest := strings.TrimLeftFunc(ln, unicode.IsSpace)
		if n, ok := bulletMarker(rest); ok {
			rest = rest[n:]
		}
		words = append(words, strings.Fields(strings.ToLower(rest))...)
	}
	return strings.Join(words, " ")
}

// uncheck folds a leading checkbox ("[ ]" or "[x]", already lowercased) of a
// normalized item to "[ ]", so ticking an action item keeps its block's hash.
func uncheck(s string) string {
	if strings.HasPrefix(s, "[x]") && (len(s) == 3 || s[3] == ' ') {
		return "[ ]" + s[3:]
	}
	return s
}

// hashText returns the first 16 hex digits of the SHA-256 of s.
func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}
