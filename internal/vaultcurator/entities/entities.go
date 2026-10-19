// Package entities lists the entities a vault already has and proposes
// candidate entities: names and terms the notes mention repeatedly in plain
// text that no note title or alias covers.
//
// The candidate heuristic is deliberately simple (see Run):
//
//  1. Mask what is not prose: fenced code, inline code, wikilinks, markdown
//     links, bare URLs, headings and generated blocks.
//  2. Find runs of capitalized words joined by single spaces ("Teodor Quill",
//     "Sana"). Weekdays, months, pronouns, articles, "AI" and other
//     stopwords split runs.
//  3. A single word seen only at the start of a sentence or bullet ("Add
//     retries") is not evidence of a name; it needs a capitalized mention
//     somewhere mid-sentence.
//  4. A single word that is part of exactly one multi-word run ("Teodor" in
//     "Teodor Quill") folds into it.
//  5. A name that resolves to a note title or alias is not a candidate.
//  6. A candidate needs MinCount mentions (default 2). Mentions are then
//     recounted case-insensitively, so "oskar" counts too.
package entities

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// DefaultMinCount is how many mentions make a candidate.
const DefaultMinCount = 2

// Options configures Run.
type Options struct {
	// MinCount is the least number of mentions for a candidate. Zero means
	// DefaultMinCount.
	MinCount int
}

// Entity is an existing entity note with how many notes link to it.
type Entity struct {
	vault.Entity
	Backlinks int `json:"backlinks"`
}

// Occurrence is one mention of a candidate.
type Occurrence struct {
	Path string `json:"path"`
	// Line is 1-based.
	Line int `json:"line"`
}

// Candidate is a name or term that may deserve an entity note.
type Candidate struct {
	// Name is the longest form seen, as first written.
	Name string `json:"name"`
	// Variants are shorter forms folded into Name ("Teodor" for "Teodor Quill").
	Variants []string `json:"variants,omitempty"`
	// Count is the number of mentions: len(Occurrences).
	Count       int          `json:"count"`
	Occurrences []Occurrence `json:"occurrences"`
}

// Result is the entity listing.
type Result struct {
	// Entities are the existing entity notes, sorted by path.
	Entities []Entity `json:"entities"`
	// Candidates are sorted by count (most first), then name.
	Candidates []Candidate `json:"candidates"`
}

var (
	maskRes = []*regexp.Regexp{
		regexp.MustCompile("`[^`]*`"),
		regexp.MustCompile(`!?\[\[[^\]\n]*\]\]`),
		regexp.MustCompile(`!?\[[^\]\n]*\]\([^)\n]*\)`),
		regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://\S+`),
	}
	prefixRe  = regexp.MustCompile(`^\s*(?:(?:[-*+]|\d+[.)])\s+)?(?:\[[ xX]\]\s+)?(?:>\s*)*`)
	headingRe = regexp.MustCompile(`^\s{0,3}#{1,6}(?:\s|$)`)
	fenceRe   = regexp.MustCompile("^\\s*(```|~~~)")
	wordRe    = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}'’_-]*`)
)

// word is a token of a masked line.
type word struct {
	text       string
	start, end int
	initial    bool // first word of a sentence or bullet
}

// run is a maximal sequence of capitalized words.
type run struct {
	words   []string
	strong  int // mentions not at the start of a sentence (always counts for multi-word)
	weak    int // sentence-initial single-word mentions
	display string
}

func (r *run) key() string { return strings.ToLower(strings.Join(r.words, " ")) }

// Run lists v's entities and candidates.
func Run(v *vault.Vault, opts Options) *Result {
	min := opts.MinCount
	if min <= 0 {
		min = DefaultMinCount
	}
	res := &Result{Entities: []Entity{}, Candidates: []Candidate{}}
	for _, e := range v.Entities {
		res.Entities = append(res.Entities, Entity{Entity: e, Backlinks: len(v.Backlinks(e.Path))})
	}

	lines := map[string][]string{} // path -> masked prose lines ("" for skipped)
	runs := map[string]*run{}
	for _, n := range v.Notes {
		ls := proseLines(n)
		lines[n.Path] = ls
		for _, l := range ls {
			collect(l, runs)
		}
	}

	groups := fold(runs)
	for _, g := range groups {
		if g.strong == 0 || g.strong+g.weak < min {
			continue
		}
		if v.Resolve(g.name) != nil {
			continue
		}
		known := false
		for _, s := range g.variants {
			if v.Resolve(s) != nil {
				known = true
			}
		}
		if known {
			continue
		}
		c := Candidate{Name: g.name, Variants: g.variants}
		c.Occurrences = occurrences(v, lines, append([]string{g.name}, g.variants...))
		c.Count = len(c.Occurrences)
		if c.Count < min {
			continue
		}
		res.Candidates = append(res.Candidates, c)
	}
	sort.SliceStable(res.Candidates, func(i, j int) bool {
		a, b := res.Candidates[i], res.Candidates[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Name < b.Name
	})
	return res
}

type group struct {
	name     string
	variants []string
	strong   int
	weak     int
}

// fold merges each single-word run into the one multi-word run containing
// it, and returns one group per remaining name.
func fold(runs map[string]*run) []*group {
	var multi, single []*run
	for _, r := range runs {
		if len(r.words) > 1 {
			multi = append(multi, r)
		} else {
			single = append(single, r)
		}
	}
	sort.Slice(multi, func(i, j int) bool { return multi[i].key() < multi[j].key() })
	sort.Slice(single, func(i, j int) bool { return single[i].key() < single[j].key() })
	byKey := map[string]*group{}
	var out []*group
	for _, r := range multi {
		g := &group{name: r.display, strong: r.strong + r.weak}
		byKey[r.key()] = g
		out = append(out, g)
	}
	for _, s := range single {
		var hosts []*run
		for _, m := range multi {
			for _, w := range m.words {
				if strings.EqualFold(w, s.words[0]) {
					hosts = append(hosts, m)
					break
				}
			}
		}
		if len(hosts) == 1 {
			g := byKey[hosts[0].key()]
			g.variants = append(g.variants, s.display)
			g.strong += s.strong
			g.weak += s.weak
			continue
		}
		out = append(out, &group{name: s.display, strong: s.strong, weak: s.weak})
	}
	return out
}

// collect adds the capitalized runs of one masked line to runs.
func collect(line string, runs map[string]*run) {
	if strings.TrimSpace(line) == "" {
		return
	}
	var ws []word
	for _, m := range wordRe.FindAllStringIndex(line, -1) {
		ws = append(ws, word{text: line[m[0]:m[1]], start: m[0], end: m[1]})
	}
	for i := range ws {
		if i == 0 {
			ws[i].initial = strings.TrimSpace(line[:ws[i].start]) == ""
			continue
		}
		gap := strings.TrimSpace(line[ws[i-1].end:ws[i].start])
		ws[i].initial = strings.HasSuffix(gap, ".") || strings.HasSuffix(gap, "!") || strings.HasSuffix(gap, "?")
	}
	var cur []word
	flush := func() {
		if len(cur) == 0 {
			return
		}
		texts := make([]string, len(cur))
		for i, w := range cur {
			texts[i] = w.text
		}
		r := &run{words: texts, display: strings.Join(texts, " ")}
		k := r.key()
		if ex := runs[k]; ex != nil {
			r = ex
		} else {
			runs[k] = r
		}
		switch {
		case len(cur) > 1 || !cur[0].initial:
			r.strong++
		default:
			r.weak++
		}
		cur = nil
	}
	for i, w := range ws {
		if !capitalized(w.text) || stopword(w.text) {
			flush()
			continue
		}
		if len(cur) > 0 && line[ws[i-1].end:w.start] != " " {
			flush()
		}
		cur = append(cur, w)
	}
	flush()
}

func capitalized(w string) bool {
	r, _ := utf8.DecodeRuneInString(w)
	return unicode.IsUpper(r)
}

// stopword reports a capitalized word that is never part of a name:
// stopwords, weekdays, months, and shorthand such as "AI" (action item).
func stopword(w string) bool { return stopwords[strings.ToLower(w)] }

// proseLines returns n's lines with everything that is not prose replaced by
// spaces (same length), or "" for lines skipped whole: fenced code, headings
// and generated blocks. Index i is line i+1.
func proseLines(n *note.Note) []string {
	src := strings.Split(string(n.Src), "\n")
	skip := map[int]bool{}
	for _, s := range n.Segments {
		if s.Kind == note.KindGenerated || s.Kind == note.KindMermaid || s.Kind == note.KindCode {
			for l := s.StartLine; l <= s.EndLine; l++ {
				skip[l] = true
			}
		}
	}
	out := make([]string, len(src))
	fence := ""
	for i, l := range src {
		l = strings.TrimSuffix(l, "\r")
		if m := fenceRe.FindStringSubmatch(l); m != nil {
			switch fence {
			case "":
				fence = m[1]
			case m[1]:
				fence = ""
			}
			continue
		}
		if fence != "" || skip[i+1] || headingRe.MatchString(l) {
			continue
		}
		for _, re := range maskRes {
			l = re.ReplaceAllStringFunc(l, func(s string) string { return strings.Repeat(" ", len(s)) })
		}
		pre := prefixRe.FindString(l)
		out[i] = strings.Repeat(" ", len(pre)) + l[len(pre):]
	}
	return out
}

// occurrences finds every case-insensitive whole-word mention of any of
// names, longest name first so "Teodor Quill" is not also counted as "Teodor".
func occurrences(v *vault.Vault, lines map[string][]string, names []string) []Occurrence {
	sort.SliceStable(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	var parts []string
	for _, nm := range names {
		parts = append(parts, regexp.QuoteMeta(nm))
	}
	re := regexp.MustCompile(`(?i)` + strings.Join(parts, "|"))
	var out []Occurrence
	for _, n := range v.Notes {
		for i, l := range lines[n.Path] {
			if l == "" {
				continue
			}
			for _, m := range re.FindAllStringIndex(l, -1) {
				if wordBoundary(l, m[0], m[1]) {
					out = append(out, Occurrence{Path: n.Path, Line: i + 1})
				}
			}
		}
	}
	return out
}

func wordBoundary(s string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:start])
		if isWordRune(r) {
			return false
		}
	}
	if end < len(s) {
		r, _ := utf8.DecodeRuneInString(s[end:])
		if isWordRune(r) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '\'' || r == '’'
}

var stopwords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`
		monday tuesday wednesday thursday friday saturday sunday
		mon tue tues wed thu thur thurs fri sat sun
		january february march april may june july august september october november december
		jan feb mar apr jun jul aug sep sept oct nov dec
		the a an this that these those there here it its we our us you your they their them
		he she his her him i my me and or but so if when then than also not no yes
		for from to of in on at by with as is are was were be been do does did has have had
		will would can could should shall might must about after before because while
		ai ok tbd todo fixme note notes`) {
		m[w] = true
	}
	return m
}()
