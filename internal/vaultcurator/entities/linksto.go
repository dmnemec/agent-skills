package entities

import (
	"fmt"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// Target is the note a LinksTo query resolved.
type Target struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// LinkRef is one link to the target.
type LinkRef struct {
	// Line is 1-based.
	Line int `json:"line"`
	// Raw is the link exactly as written.
	Raw           string `json:"raw"`
	InFrontmatter bool   `json:"in_frontmatter,omitempty"`
}

// BlockRef identifies a block by the hash a plan source uses.
type BlockRef struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Hash      string `json:"hash"`
}

// Linker is a note that links to the target.
type Linker struct {
	Path  string    `json:"path"`
	Links []LinkRef `json:"links"`
	// Blocks are every top-level block of the note, which a `rewrite` of the
	// note must map as sources. scan lists blocks only for changed notes.
	Blocks []BlockRef `json:"blocks"`
}

// Backlinks is the answer to LinksTo.
type Backlinks struct {
	Target Target   `json:"target"`
	Notes  []Linker `json:"notes"`
}

// LinksTo lists every link that resolves to the note named by name (a title,
// alias or path), grouped by linking note, with the blocks a rewrite of each
// needs. It is what merging entity B into A uses to find the links to
// redirect. Links from the target to itself are left out.
func LinksTo(v *vault.Vault, name string) (*Backlinks, error) {
	t := v.Resolve(name)
	if t == nil {
		return nil, fmt.Errorf("no note named %q", name)
	}
	res := &Backlinks{Target: Target{Path: t.Path, Title: t.Title}, Notes: []Linker{}}
	idx := map[string]int{}
	for _, e := range v.Backlinks(t.Path) {
		if e.From == t.Path {
			continue
		}
		i, ok := idx[e.From]
		if !ok {
			l := Linker{Path: e.From, Links: []LinkRef{}, Blocks: []BlockRef{}}
			for _, b := range v.Note(e.From).Blocks {
				l.Blocks = append(l.Blocks, BlockRef{StartLine: b.StartLine, EndLine: b.EndLine, Hash: b.Hash})
			}
			res.Notes = append(res.Notes, l)
			i = len(res.Notes) - 1
			idx[e.From] = i
		}
		res.Notes[i].Links = append(res.Notes[i].Links, LinkRef{Line: e.Link.Line, Raw: e.Link.Raw, InFrontmatter: e.Link.InFrontmatter})
	}
	return res, nil
}
