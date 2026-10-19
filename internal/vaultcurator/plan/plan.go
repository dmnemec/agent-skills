// Package plan holds the curation plan: the JSON file an agent writes from
// scan output, the user approves in review, and apply carries out. See
// docs/plan.md for the schema.
//
// Validate is the single place that turns a plan into the post-plan content
// of every note it touches. It also runs every check that gates apply: the
// lossless check, broken links, unapproved and never-tier changes. Apply
// writes Result.Changes when Result.Errors is empty.
package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Version is the plan schema version this package reads and writes.
const Version = 1

// Plan is a list of proposed operations on a vault.
type Plan struct {
	// Version is the schema version; it must be Version.
	Version    int         `json:"version"`
	Operations []Operation `json:"operations"`
}

// Kind is what an operation does.
type Kind string

// Operation kinds.
const (
	// Rewrite replaces a note's whole content with Content (restructure).
	// Sources must map every bullet of the note.
	Rewrite Kind = "rewrite"
	// Split moves the bullets in Sources out of Note into a new note at
	// Path with Content, and leaves Link in their place.
	Split Kind = "split"
	// Create adds a new note at Path with Content, such as an entity stub.
	// It moves no content, so it has no Sources.
	Create Kind = "create"
	// SetFrontmatter sets (or, with a null value, removes) frontmatter
	// fields of Note.
	SetFrontmatter Kind = "set_frontmatter"
	// AddLinks appends one "- [[target]]" bullet per entry of Links to Note.
	AddLinks Kind = "add_links"
	// Keep records "reviewed, keep as is" for Note. It changes no content;
	// apply marks every block of Note processed so scan stops listing it.
	Keep Kind = "keep"
)

// Category groups operations for review, which approves by category.
type Category string

// Categories.
const (
	CategoryRestructure Category = "restructure"
	CategorySplit       Category = "split"
	CategoryEntity      Category = "entity"
	CategoryNote        Category = "note"
	CategoryFrontmatter Category = "frontmatter"
	CategoryLinks       Category = "links"
	CategoryKeep        Category = "keep"
)

// Tier is how an operation is approved (design doc: Autonomy tiers).
type Tier string

// Tiers. TierNever is never valid in a plan; it exists so validation can name
// it.
const (
	// TierAuto changes are approved together by category.
	TierAuto Tier = "auto"
	// TierApproveEach changes are listed and approved one by one.
	TierApproveEach Tier = "approve-each"
	// TierNever changes are not permitted.
	TierNever Tier = "never"
)

// Status is the user's decision on an operation.
type Status string

// Statuses. Only approved operations are applied.
const (
	Pending  Status = "pending"
	Approved Status = "approved"
	Rejected Status = "rejected"
)

// kinds lists each operation kind's allowed categories and the lowest tier
// each may use. A plan may raise an auto change to approve-each, never lower.
var kinds = map[Kind]struct {
	categories map[Category]Tier
}{
	Rewrite:        {map[Category]Tier{CategoryRestructure: TierAuto}},
	Split:          {map[Category]Tier{CategorySplit: TierApproveEach}},
	Create:         {map[Category]Tier{CategoryEntity: TierApproveEach, CategoryNote: TierApproveEach}},
	SetFrontmatter: {map[Category]Tier{CategoryFrontmatter: TierAuto}},
	AddLinks:       {map[Category]Tier{CategoryLinks: TierAuto}},
	Keep:           {map[Category]Tier{CategoryKeep: TierAuto}},
}

// Operation is one proposed change. Which fields apply depends on Op; see
// docs/plan.md.
type Operation struct {
	// ID is unique within the plan. Errors and review refer to it.
	ID       string   `json:"id"`
	Op       Kind     `json:"op"`
	Category Category `json:"category"`
	Tier     Tier     `json:"tier"`
	Status   Status   `json:"status"`
	// Summary is a one-line description for review.
	Summary string `json:"summary,omitempty"`
	// Note is the existing note acted on, by vault-relative path: the note
	// rewritten, split, kept, or given frontmatter or links. SetFrontmatter and
	// AddLinks may also name a note created by another operation.
	Note string `json:"note,omitempty"`
	// Path is the new note's vault-relative path, for Split and Create.
	Path string `json:"path,omitempty"`
	// Content is the full resulting note, for Rewrite, Split and Create.
	Content string `json:"content,omitempty"`
	// Sources map original bullets of Note into Content, for Rewrite and
	// Split.
	Sources []Source `json:"sources,omitempty"`
	// Link is the line a Split leaves in Note where the moved bullets were.
	// Empty means "- [[<new note title>]]".
	Link string `json:"link,omitempty"`
	// Frontmatter is the fields SetFrontmatter sets; null removes a field.
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
	// Links are the link targets AddLinks appends, by note name.
	Links []string `json:"links,omitempty"`
}

// Source is a run of original bullets: those of block Block in Note whose
// bullet line falls within Lines. It must cover whole bullets.
type Source struct {
	// Note is the vault-relative path of the note the bullets come from.
	Note string `json:"note"`
	// Block is the block's hash from scan output.
	Block string `json:"block"`
	// Lines is the 1-based inclusive line range within the block.
	Lines [2]int `json:"lines"`
}

// Parse decodes a plan. Unknown fields are an error, so a typo in a field
// name is caught rather than ignored.
func Parse(data []byte) (*Plan, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var p Plan
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("parse plan: trailing data after the plan object")
	}
	if p.Version != Version {
		return nil, fmt.Errorf("parse plan: version %d, want %d", p.Version, Version)
	}
	return &p, nil
}

// Load reads and parses the plan file at path.
func Load(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Marshal encodes the plan as indented JSON with a trailing newline, the form
// review writes back after updating statuses.
func (p *Plan) Marshal() ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Op returns the operation with the given id, or nil.
func (p *Plan) Op(id string) *Operation {
	for i := range p.Operations {
		if p.Operations[i].ID == id {
			return &p.Operations[i]
		}
	}
	return nil
}
