// Package vault loads every note in a vault and builds what curation needs
// across notes: wikilink resolution by title and alias, the link graph and
// backlinks, the entity index, orphans and broken links.
//
// Folders carry no structure: a note is found by its title (file name) or an
// alias wherever it lives.
package vault

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
)

// DefaultFallbackHub is the title of the hub for notes that fit no other.
const DefaultFallbackHub = "Unsorted"

// skipDirs are directory names never descended into.
var skipDirs = map[string]bool{".git": true, ".vault-curator": true}

// Entity note types. Notes of these types are entities; projects and
// components are also hubs.
const (
	TypeProject   = "project"
	TypeComponent = "component"
	TypePerson    = "person"
)

// Options configures how a vault is interpreted.
type Options struct {
	// FallbackHub is the title of the fallback hub note. Empty means
	// DefaultFallbackHub.
	FallbackHub string
}

// Vault is a loaded, indexed vault. It is read-only once built.
type Vault struct {
	// Root is the directory the vault was loaded from ("" for New).
	Root string
	// Notes are every markdown note, sorted by Path.
	Notes []*note.Note
	// Attachments are the vault-relative paths of non-markdown files,
	// sorted. Links to them are neither edges nor broken.
	Attachments []string
	// Edges are every resolved link between notes, in note then link order.
	Edges []Edge
	// Broken are links that resolve to no note and no attachment.
	Broken []BrokenLink
	// Entities are the project, component and person notes, plus the
	// fallback hub note if it exists, sorted by Path.
	Entities []Entity
	// Orphans are the paths of notes that are not hubs and reach no hub by
	// following links, sorted.
	Orphans []string

	fallback string
	byPath   map[string]*note.Note   // lowercased path without .md
	byTitle  map[string][]*note.Note // lowercased title
	byAlias  map[string][]*note.Note // lowercased alias
	attach   map[string]bool         // lowercased attachment path and base name
	out      map[string][]Edge
	in       map[string][]Edge
}

// Edge is a link from one note to another.
type Edge struct {
	// From and To are note paths.
	From string    `json:"from"`
	To   string    `json:"to"`
	Link note.Link `json:"link"`
}

// BrokenLink is a link whose target does not exist in the vault.
type BrokenLink struct {
	From string    `json:"from"`
	Link note.Link `json:"link"`
}

// Entity is a note that names a project, component or person, or the
// fallback hub.
type Entity struct {
	Path    string   `json:"path"`
	Title   string   `json:"title"`
	Type    string   `json:"type"`
	Aliases []string `json:"aliases,omitempty"`
	// Hub reports a project, component or the fallback hub: the notes every
	// other note must reach.
	Hub bool `json:"hub"`
}

// Resolution says what a link points at.
type Resolution int

// Link resolutions.
const (
	// Resolved links point at a note in the vault.
	Resolved Resolution = iota
	// External links have a URL scheme and are not checked.
	External
	// Attachment links point at a non-markdown file in the vault.
	Attachment
	// Broken links point at nothing in the vault.
	Broken
)

// Load reads every .md file under root, skipping .git and .vault-curator
// directories, and builds the vault. Other files are recorded as attachments.
func Load(root string, opts Options) (*Vault, error) {
	var notes []*note.Note
	var attachments []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.EqualFold(path.Ext(rel), ".md") {
			attachments = append(attachments, rel)
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		notes = append(notes, note.Parse(rel, src))
		return nil
	})
	if err != nil {
		return nil, err
	}
	v := New(notes, attachments, opts)
	v.Root = root
	return v, nil
}

// New builds a vault from parsed notes and attachment paths, all
// vault-relative with forward slashes. It does no I/O.
func New(notes []*note.Note, attachments []string, opts Options) *Vault {
	v := &Vault{
		Notes:       append([]*note.Note(nil), notes...),
		Attachments: append([]string(nil), attachments...),
		fallback:    opts.FallbackHub,
		byPath:      map[string]*note.Note{},
		byTitle:     map[string][]*note.Note{},
		byAlias:     map[string][]*note.Note{},
		attach:      map[string]bool{},
		out:         map[string][]Edge{},
		in:          map[string][]Edge{},
	}
	if v.fallback == "" {
		v.fallback = DefaultFallbackHub
	}
	sort.Slice(v.Notes, func(i, j int) bool { return v.Notes[i].Path < v.Notes[j].Path })
	sort.Strings(v.Attachments)
	for _, n := range v.Notes {
		v.byPath[key(stripMD(n.Path))] = n
		v.byTitle[key(n.Title)] = append(v.byTitle[key(n.Title)], n)
		for _, a := range n.Frontmatter.Aliases {
			v.byAlias[key(a)] = append(v.byAlias[key(a)], n)
		}
	}
	for _, a := range v.Attachments {
		v.attach[key(a)] = true
		v.attach[key(path.Base(a))] = true
	}
	v.buildGraph()
	v.buildEntities()
	v.buildOrphans()
	return v
}

func key(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func stripMD(p string) string {
	if ext := path.Ext(p); strings.EqualFold(ext, ".md") {
		return p[:len(p)-len(ext)]
	}
	return p
}

// FallbackHub returns the fallback hub's title.
func (v *Vault) FallbackHub() string { return v.fallback }

// Note returns the note at a vault-relative path, or nil.
func (v *Vault) Note(p string) *note.Note {
	return v.byPath[key(stripMD(p))]
}

// Resolve finds the note a wikilink target names, ignoring case and a
// trailing .md. A target containing "/" is matched as a vault-relative path;
// otherwise titles are tried first, then aliases. When several notes match,
// the one with the lowest path wins. It returns nil if nothing matches.
func (v *Vault) Resolve(target string) *note.Note {
	t := strings.TrimPrefix(stripMD(strings.TrimSpace(target)), "/")
	if t == "" {
		return nil
	}
	if strings.Contains(t, "/") {
		return v.byPath[key(path.Clean(t))]
	}
	if ns := v.byTitle[key(t)]; len(ns) > 0 {
		return ns[0]
	}
	if ns := v.byAlias[key(t)]; len(ns) > 0 {
		return ns[0]
	}
	return nil
}

// ResolveLink resolves a link written in note from. A link with an empty
// target ([[#heading]]) resolves to from itself. Markdown links are tried
// relative to from's folder, then as a vault path, then by title. Headings
// and block references are not checked.
func (v *Vault) ResolveLink(from *note.Note, l note.Link) (*note.Note, Resolution) {
	if l.External {
		return nil, External
	}
	if l.Target == "" {
		return from, Resolved
	}
	if l.Kind == note.Markdown && from != nil {
		rel := path.Join(path.Dir(from.Path), l.Target)
		if n := v.byPath[key(stripMD(rel))]; n != nil {
			return n, Resolved
		}
	}
	if n := v.Resolve(l.Target); n != nil {
		return n, Resolved
	}
	if v.isAttachment(from, l) {
		return nil, Attachment
	}
	return nil, Broken
}

func (v *Vault) isAttachment(from *note.Note, l note.Link) bool {
	t := strings.TrimPrefix(l.Target, "/")
	if v.attach[key(path.Clean(t))] || v.attach[key(path.Base(t))] {
		return true
	}
	if from != nil {
		return v.attach[key(path.Join(path.Dir(from.Path), t))]
	}
	return false
}

// Outgoing returns the edges from the note at path p.
func (v *Vault) Outgoing(p string) []Edge { return v.out[v.canon(p)] }

// Backlinks returns the edges into the note at path p.
func (v *Vault) Backlinks(p string) []Edge { return v.in[v.canon(p)] }

func (v *Vault) canon(p string) string {
	if n := v.Note(p); n != nil {
		return n.Path
	}
	return p
}

// IsHub reports whether n is a project, a component or the fallback hub.
func (v *Vault) IsHub(n *note.Note) bool {
	t := n.Type()
	return t == TypeProject || t == TypeComponent || strings.EqualFold(n.Title, v.fallback)
}

func (v *Vault) buildGraph() {
	for _, n := range v.Notes {
		for _, l := range n.Links {
			to, res := v.ResolveLink(n, l)
			switch res {
			case Resolved:
				e := Edge{From: n.Path, To: to.Path, Link: l}
				v.Edges = append(v.Edges, e)
				v.out[n.Path] = append(v.out[n.Path], e)
				v.in[to.Path] = append(v.in[to.Path], e)
			case Broken:
				v.Broken = append(v.Broken, BrokenLink{From: n.Path, Link: l})
			}
		}
	}
}

func (v *Vault) buildEntities() {
	for _, n := range v.Notes {
		t := n.Type()
		isEntity := t == TypeProject || t == TypeComponent || t == TypePerson
		if !isEntity && !strings.EqualFold(n.Title, v.fallback) {
			continue
		}
		v.Entities = append(v.Entities, Entity{
			Path: n.Path, Title: n.Title, Type: t,
			Aliases: n.Frontmatter.Aliases, Hub: v.IsHub(n),
		})
	}
}

// buildOrphans walks backlinks out from every hub; any non-hub note not
// reached links to no hub, directly or through other notes.
func (v *Vault) buildOrphans() {
	reached := map[string]bool{}
	var queue []string
	for _, n := range v.Notes {
		if v.IsHub(n) {
			reached[n.Path] = true
			queue = append(queue, n.Path)
		}
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, e := range v.in[p] {
			if !reached[e.From] {
				reached[e.From] = true
				queue = append(queue, e.From)
			}
		}
	}
	for _, n := range v.Notes {
		if !reached[n.Path] {
			v.Orphans = append(v.Orphans, n.Path)
		}
	}
}
