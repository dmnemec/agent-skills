// Package scan finds what curation should look at: notes changed since the
// watermark tag and, inside each, every top-level block, marking those not yet
// processed as new.
// It also reports the vault-wide context the agent plans against: the notes,
// the entity index, the link graph, broken links and orphans. The Result is
// the `vault-curator scan --json` output, documented in docs/cli.md.
package scan

import (
	"sort"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/gitx"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/state"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// Options configures a scan.
type Options struct {
	// Tag is the watermark tag name. Empty means gitx.WatermarkTag.
	Tag string
	// FallbackHub is passed to vault.Load.
	FallbackHub string
	// All ignores the watermark: the candidates are the notes with at least
	// one unprocessed block, which is how backfill walks old notes.
	All bool
	// Limit caps the number of candidates, taken in path order. Zero means
	// no cap.
	Limit int
}

// Result is a scan's output.
type Result struct {
	Watermark Watermark `json:"watermark"`
	// Candidates are the notes changed since the watermark (every note when
	// there is none), sorted by path, with their unprocessed blocks.
	Candidates []Candidate `json:"candidates"`
	// Remaining counts the candidates left out by Limit; 0 when nothing was
	// cut. Backfill is finished when it reaches 0.
	Remaining int `json:"remaining"`
	// Notes are every note in the vault, sorted by path.
	Notes []NoteInfo `json:"notes"`
	// Entities are the project, component and person notes and the
	// fallback hub.
	Entities []vault.Entity `json:"entities"`
	// Links are the resolved links between notes: the link graph.
	Links []Link `json:"links"`
	// BrokenLinks are links that resolve to no note or attachment.
	BrokenLinks []Link `json:"broken_links"`
	// Orphans are the paths of non-hub notes that reach no hub.
	Orphans []string `json:"orphans"`

	// Vault and State are what the scan read, for callers in this module.
	Vault *vault.Vault `json:"-"`
	State *state.State `json:"-"`
}

// Watermark describes the watermark tag.
type Watermark struct {
	Tag string `json:"tag"`
	// Exists is false before the first curation; every note is then a
	// candidate.
	Exists bool `json:"exists"`
	// Commit is the tagged commit, empty when the tag does not exist.
	Commit string `json:"commit,omitempty"`
}

// Candidate is a note curation should look at.
type Candidate struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	// Type is the frontmatter type, lowercased, or empty.
	Type string `json:"type,omitempty"`
	// Tracked reports whether curation has processed this note before.
	Tracked bool `json:"tracked"`
	// ProcessedBlocks counts the note's blocks already processed and so
	// left out of NewBlocks.
	ProcessedBlocks int `json:"processed_blocks"`
	// NewBlocks are the top-level blocks not yet processed, in order. It is
	// empty when the note changed outside its blocks, or has none.
	NewBlocks []Block `json:"new_blocks"`
	// Blocks are all of the note's top-level blocks, in order, each marked
	// Processed or not. A rewrite must map every one of them.
	Blocks []Block `json:"blocks"`
}

// Block is a top-level block of a candidate note.
type Block struct {
	// StartLine and EndLine are 1-based and inclusive.
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Hash      string `json:"hash"`
	// Heading is the nearest heading above the block, if any.
	Heading string `json:"heading,omitempty"`
	// Text is the block's raw source.
	Text string `json:"text"`
	// Processed reports whether an earlier curation already processed the
	// block. It is always false in NewBlocks.
	Processed bool `json:"processed"`
}

// NoteInfo names a note.
type NoteInfo struct {
	Path    string   `json:"path"`
	Title   string   `json:"title"`
	Type    string   `json:"type,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
}

// Link is one link in the graph, or a broken one.
type Link struct {
	From string `json:"from"`
	// To is the resolved note's path; empty for a broken link.
	To string `json:"to,omitempty"`
	// Target is the link target as written, without heading or label.
	Target string `json:"target"`
	Raw    string `json:"raw"`
	Line   int    `json:"line"`
	// InFrontmatter reports a link written in frontmatter, such as a typed
	// edge or a project field.
	InFrontmatter bool `json:"in_frontmatter,omitempty"`
}

// Run scans the vault at root, which must be inside a git work tree.
func Run(root string, opts Options) (*Result, error) {
	tag := opts.Tag
	if tag == "" {
		tag = gitx.WatermarkTag
	}
	repo, err := gitx.Open(root)
	if err != nil {
		return nil, err
	}
	v, err := vault.Load(repo.Dir, vault.Options{FallbackHub: opts.FallbackHub})
	if err != nil {
		return nil, err
	}
	st, err := state.Load(repo.Dir)
	if err != nil {
		return nil, err
	}
	commit, ok, err := repo.TagCommit(tag)
	if err != nil {
		return nil, err
	}

	r := &Result{
		Watermark:   Watermark{Tag: tag, Exists: ok, Commit: commit},
		Candidates:  []Candidate{},
		Notes:       []NoteInfo{},
		Entities:    v.Entities,
		Links:       []Link{},
		BrokenLinks: []Link{},
		Orphans:     v.Orphans,
		Vault:       v,
		State:       st,
	}
	if r.Entities == nil {
		r.Entities = []vault.Entity{}
	}
	if r.Orphans == nil {
		r.Orphans = []string{}
	}

	changed := v.Notes
	if opts.All {
		changed = nil
		for _, n := range v.Notes {
			if len(st.NewBlocks(n)) > 0 {
				changed = append(changed, n)
			}
		}
	} else if ok {
		paths, err := repo.ChangedSince(commit)
		if err != nil {
			return nil, err
		}
		changed = nil
		for _, p := range paths {
			// Only notes vault.Load read count: this drops attachments,
			// the state file and anything under skipped folders.
			if n := v.Note(p); n != nil && n.Path == p {
				changed = append(changed, n)
			}
		}
	}
	sort.Slice(changed, func(i, j int) bool { return changed[i].Path < changed[j].Path })
	if opts.Limit > 0 && len(changed) > opts.Limit {
		r.Remaining = len(changed) - opts.Limit
		changed = changed[:opts.Limit]
	}
	for _, n := range changed {
		r.Candidates = append(r.Candidates, candidate(n, st))
	}

	for _, n := range v.Notes {
		r.Notes = append(r.Notes, NoteInfo{Path: n.Path, Title: n.Title, Type: n.Type(), Aliases: n.Frontmatter.Aliases})
	}
	for _, e := range v.Edges {
		r.Links = append(r.Links, link(e.From, e.To, e.Link))
	}
	for _, b := range v.Broken {
		r.BrokenLinks = append(r.BrokenLinks, link(b.From, "", b.Link))
	}
	return r, nil
}

func candidate(n *note.Note, st *state.State) Candidate {
	c := Candidate{Path: n.Path, Title: n.Title, Type: n.Type(), Tracked: st.Tracked(n.Path), NewBlocks: []Block{}, Blocks: []Block{}}
	// Blocks start at distinct lines, so a block's start line identifies it.
	isNew := map[int]bool{}
	for _, b := range st.NewBlocks(n) {
		isNew[b.StartLine] = true
	}
	for _, b := range n.Blocks {
		blk := Block{
			StartLine: b.StartLine, EndLine: b.EndLine, Hash: b.Hash,
			Heading: b.Heading, Text: b.Text, Processed: !isNew[b.StartLine],
		}
		c.Blocks = append(c.Blocks, blk)
		if blk.Processed {
			c.ProcessedBlocks++
		} else {
			c.NewBlocks = append(c.NewBlocks, blk)
		}
	}
	return c
}

func link(from, to string, l note.Link) Link {
	return Link{From: from, To: to, Target: l.Target, Raw: l.Raw, Line: l.Line, InFrontmatter: l.InFrontmatter}
}

// NewBlockCount returns the total number of new blocks across candidates.
func (r *Result) NewBlockCount() int {
	n := 0
	for _, c := range r.Candidates {
		n += len(c.NewBlocks)
	}
	return n
}
