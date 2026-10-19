package plan

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// Options configures Validate.
type Options struct {
	// Draft treats pending operations as approved, so an agent can check a
	// plan before the user reviews it. Apply never sets it.
	Draft bool
	// Ignored reports whether git ignores a vault-relative slash path. A new
	// path (split, create) that it ignores is rejected, since a note there
	// would never be committed. Nil skips the check, for pure unit tests.
	Ignored func(path string) (bool, error)
}

// Error is one validation failure, naming the operation and, where there is
// one, the note and line it concerns.
type Error struct {
	// Op is the operation's id, or "" for the plan as a whole.
	Op string `json:"op,omitempty"`
	// Path is the vault-relative note path Line refers to: an original note
	// for a source bullet, or the post-plan note for its content.
	Path string `json:"path,omitempty"`
	// Line is 1-based, or 0 when the error is not about one line.
	Line int    `json:"line,omitempty"`
	Msg  string `json:"message"`
}

func (e *Error) Error() string {
	var b strings.Builder
	if e.Op != "" {
		fmt.Fprintf(&b, "op %s: ", e.Op)
	}
	if e.Path != "" {
		b.WriteString(e.Path)
		if e.Line > 0 {
			fmt.Fprintf(&b, ":%d", e.Line)
		}
		b.WriteString(": ")
	}
	b.WriteString(e.Msg)
	return b.String()
}

// Change is the post-plan content of one note the plan touches.
type Change struct {
	// Path is the vault-relative path.
	Path string `json:"path"`
	// Before is the note's current content, nil for a note the plan creates.
	Before []byte `json:"-"`
	// After is the note's content once every applied operation has run.
	After []byte `json:"-"`
	// Ops are the ids of the applied operations that touch the note, in plan
	// order.
	Ops []string `json:"ops"`
}

// Created reports whether the plan creates the note.
func (c Change) Created() bool { return c.Before == nil }

// Result is the outcome of Validate.
type Result struct {
	// Changes are the touched notes sorted by path. They are nil whenever
	// Errors is not empty, so a failed plan cannot be applied by mistake.
	Changes []Change
	// Kept are the paths of the notes the plan keeps as is, sorted: the
	// notes of its applied keep operations. They are in no Change, since
	// their content does not change; apply marks their blocks processed. Nil
	// whenever Errors is not empty.
	Kept []string
	// Errors are every check that failed, roughly in plan order.
	Errors []*Error
}

// OK reports whether the plan passed every check.
func (r *Result) OK() bool { return len(r.Errors) == 0 }

// selection is a resolved Source: whole bullets of one block.
type selection struct {
	src   Source
	note  *note.Note
	items []note.Item
	// start and end are the byte range of the selected lines in note.Src.
	start, end int
}

// state collects what the plan does to one note.
type state struct {
	path    string
	orig    *note.Note // nil for a created note
	creator *Operation // split or create that makes the note
	rewrite *Operation
	splits  []*Operation
	edits   []*Operation // set_frontmatter and add_links, in plan order
	keeps   []*Operation // keep, in plan order; never in ops
	ops     []*Operation // every applied op touching the note, in plan order
	after   string
}

type checker struct {
	v    *vault.Vault
	opts Options
	errs []*Error
	// ops are the operations still being applied, in plan order.
	ops []*Operation
	bad map[*Operation]bool
	// gone maps the key of a path an excluded op would have created to why.
	gone   map[string]string
	states map[string]*state // by key(path)
	sel    map[*Operation][]selection
	// mapped records which op maps each original bullet, by path and line.
	mapped map[string]map[int]string
}

// Validate checks p against v and computes the post-plan content of every
// touched note. Only approved operations (and pending ones, with
// Options.Draft) are applied; rejected ones are skipped. It is the one place
// that defines what a plan does: apply writes the returned Changes.
func Validate(v *vault.Vault, p *Plan, opts Options) *Result {
	c := &checker{
		v: v, opts: opts,
		bad:    map[*Operation]bool{},
		gone:   map[string]string{},
		states: map[string]*state{},
		sel:    map[*Operation][]selection{},
		mapped: map[string]map[int]string{},
	}
	if p.Version != Version {
		c.fail(nil, "", 0, "plan version %d, want %d", p.Version, Version)
	}
	c.screen(p)
	c.registerCreated()
	c.resolveSources()
	c.resolveTargets()
	c.checkKeeps()
	c.compute()
	c.checkLossless()
	changes := c.changes()
	c.checkLinks(changes)
	if len(c.errs) > 0 {
		return &Result{Errors: c.errs}
	}
	return &Result{Changes: changes, Kept: c.kept()}
}

func (c *checker) fail(op *Operation, p string, line int, format string, args ...any) {
	e := &Error{Path: p, Line: line, Msg: fmt.Sprintf(format, args...)}
	if op != nil {
		e.Op = op.ID
	}
	c.errs = append(c.errs, e)
}

// drop stops applying op after an error, so one mistake is reported once.
func (c *checker) drop(op *Operation, why string) {
	if c.bad[op] {
		return
	}
	c.bad[op] = true
	if op.Path != "" {
		if _, ok := c.gone[key(op.Path)]; !ok {
			c.gone[key(op.Path)] = why
		}
	}
}

func (c *checker) live() []*Operation {
	var out []*Operation
	for _, op := range c.ops {
		if !c.bad[op] {
			out = append(out, op)
		}
	}
	return out
}

// screen checks each operation on its own and keeps those to apply.
func (c *checker) screen(p *Plan) {
	seen := map[string]bool{}
	for i := range p.Operations {
		op := &p.Operations[i]
		if op.ID == "" {
			c.fail(op, "", 0, "operation %d has no id", i+1)
			continue
		}
		if seen[op.ID] {
			c.fail(op, "", 0, "duplicate operation id")
			continue
		}
		seen[op.ID] = true
		switch op.Status {
		case Approved:
		case Pending:
			if !c.opts.Draft {
				c.fail(op, "", 0, "operation is pending: approve or reject it in review before apply")
				c.markGone(op, "pending")
				continue
			}
		case Rejected:
			c.markGone(op, "rejected")
			continue
		default:
			c.fail(op, "", 0, "status %q must be pending, approved or rejected", op.Status)
			c.markGone(op, "invalid")
			continue
		}
		if c.checkShape(op) {
			c.ops = append(c.ops, op)
		} else {
			c.markGone(op, "invalid")
		}
	}
}

func (c *checker) markGone(op *Operation, how string) {
	if op.Path != "" {
		c.gone[key(op.Path)] = fmt.Sprintf("created by %s operation %s", how, op.ID)
	}
}

// checkShape checks the kind, category, tier and fields of op.
func (c *checker) checkShape(op *Operation) bool {
	k, ok := kinds[op.Op]
	if !ok {
		c.fail(op, "", 0, "unknown op %q (want rewrite, split, create, set_frontmatter, add_links or keep)", op.Op)
		return false
	}
	okay := true
	minTier, ok := k.categories[op.Category]
	if !ok {
		var cats []string
		for cat := range k.categories {
			cats = append(cats, string(cat))
		}
		sort.Strings(cats)
		c.fail(op, "", 0, "category %q is not valid for op %s (want %s)", op.Category, op.Op, strings.Join(cats, " or "))
		okay = false
	}
	switch op.Tier {
	case TierAuto, TierApproveEach:
		if ok && minTier == TierApproveEach && op.Tier == TierAuto {
			c.fail(op, "", 0, "category %s requires tier approve-each", op.Category)
			okay = false
		}
	case TierNever:
		c.fail(op, "", 0, "never-tier changes are not permitted")
		okay = false
	default:
		c.fail(op, "", 0, "tier %q must be auto or approve-each", op.Tier)
		okay = false
	}
	if !c.checkFieldTiers(op) {
		okay = false
	}
	need := func(set bool, field string) {
		if !set {
			c.fail(op, "", 0, "op %s needs %q", op.Op, field)
			okay = false
		}
	}
	forbid := func(set bool, field string) {
		if set {
			c.fail(op, "", 0, "op %s does not take %q", op.Op, field)
			okay = false
		}
	}
	hasNote, hasPath, hasContent := op.Note != "", op.Path != "", op.Content != ""
	hasSources, hasFM, hasLinks := len(op.Sources) > 0, op.Frontmatter != nil, len(op.Links) > 0
	switch op.Op {
	case Rewrite:
		need(hasNote, "note")
		need(hasContent, "content")
		need(hasSources, "sources")
		forbid(hasPath, "path")
		forbid(op.Link != "", "link")
		forbid(hasFM, "frontmatter")
		forbid(hasLinks, "links")
	case Split:
		need(hasNote, "note")
		need(hasPath, "path")
		need(hasContent, "content")
		need(hasSources, "sources")
		forbid(hasFM, "frontmatter")
		forbid(hasLinks, "links")
	case Create:
		need(hasPath, "path")
		need(hasContent, "content")
		forbid(hasNote, "note")
		forbid(hasSources, "sources")
		forbid(op.Link != "", "link")
		forbid(hasFM, "frontmatter")
		forbid(hasLinks, "links")
	case SetFrontmatter:
		need(hasNote, "note")
		forbid(hasPath, "path")
		forbid(hasContent, "content")
		forbid(hasSources, "sources")
		forbid(op.Link != "", "link")
		forbid(hasLinks, "links")
		if len(op.Frontmatter) == 0 {
			c.fail(op, "", 0, "frontmatter has no fields")
			okay = false
		}
		for k := range op.Frontmatter {
			if strings.TrimSpace(k) == "" {
				c.fail(op, "", 0, "frontmatter has an empty field name")
				okay = false
			}
		}
	case Keep:
		need(hasNote, "note")
		forbid(hasPath, "path")
		forbid(hasContent, "content")
		forbid(hasSources, "sources")
		forbid(op.Link != "", "link")
		forbid(hasFM, "frontmatter")
		forbid(hasLinks, "links")
	case AddLinks:
		need(hasNote, "note")
		need(hasLinks, "links")
		forbid(hasPath, "path")
		forbid(hasContent, "content")
		forbid(hasSources, "sources")
		forbid(op.Link != "", "link")
		forbid(hasFM, "frontmatter")
		for _, l := range op.Links {
			if strings.TrimSpace(l) == "" || strings.ContainsAny(l, "[]|#\n") {
				c.fail(op, "", 0, "link target %q must be a bare note name, such as \"Harbor\"", l)
				okay = false
			}
		}
	}
	if hasPath {
		if err := checkNewPath(op.Path); err != "" {
			c.fail(op, op.Path, 0, "%s", err)
			okay = false
		} else if c.opts.Ignored != nil {
			ign, ierr := c.opts.Ignored(op.Path)
			switch {
			case ierr != nil:
				c.fail(op, op.Path, 0, "cannot check whether git ignores the path: %v", ierr)
				okay = false
			case ign:
				c.fail(op, op.Path, 0, "path is ignored by git, so the note would not be committed")
				okay = false
			}
		}
	}
	if op.Op == Split && op.Link != "" {
		if strings.ContainsAny(strings.TrimRight(op.Link, "\n"), "\n") {
			c.fail(op, "", 0, "link must be a single line")
			okay = false
		} else if !linkOnly(op.Link) {
			c.fail(op, "", 0, "link %q must hold only links", op.Link)
			okay = false
		}
	}
	return okay
}

// approveEachFields are the frontmatter fields a set_frontmatter may touch
// only at tier approve-each (design.md, "Autonomy tiers"): typed edges,
// supersedes links and aliases, which change the graph and what links
// resolve to.
var approveEachFields = map[string]bool{
	"depends-on": true, "owned-by": true, "calls": true, "part-of": true, "related-to": true,
	"supersedes": true, "aliases": true,
}

// checkFieldTiers rejects an auto set_frontmatter that sets or removes an
// approveEachFields field.
func (c *checker) checkFieldTiers(op *Operation) bool {
	if op.Op != SetFrontmatter || op.Tier != TierAuto {
		return true
	}
	var keys []string
	for k := range op.Frontmatter {
		if approveEachFields[strings.ToLower(strings.TrimSpace(k))] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		c.fail(op, "", 0, "frontmatter field %s requires tier approve-each (typed edges, supersedes and aliases are approved one by one)", k)
	}
	return len(keys) == 0
}

// forbiddenChars are not allowed in Obsidian file names (and some not on
// Windows); "#^[]|" would also break wikilinks.
const forbiddenChars = `*"\<>:|?#^[]`

func checkNewPath(p string) string {
	switch {
	case strings.Contains(p, `\`):
		return "path must use forward slashes"
	case strings.HasPrefix(p, "/") || path.Clean(p) != p || p == "." || strings.HasPrefix(p, "../"):
		return "path must be a clean vault-relative path"
	case !strings.EqualFold(path.Ext(p), ".md"):
		return "path must end in .md"
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".git" || seg == ".vault-curator" {
			return fmt.Sprintf("path is inside %s, which the vault skips", seg)
		}
		if i := strings.IndexAny(seg, forbiddenChars); i >= 0 {
			return fmt.Sprintf("%q is not allowed in a file name", seg[i:i+1])
		}
	}
	return ""
}

// registerCreated records the notes splits and creates make.
func (c *checker) registerCreated() {
	titles := map[string]*Operation{}
	for _, op := range c.ops {
		if op.Op != Split && op.Op != Create {
			continue
		}
		k := key(op.Path)
		if n := c.v.Note(op.Path); n != nil {
			c.fail(op, op.Path, 0, "note %s already exists", n.Path)
			c.drop(op, "")
			continue
		}
		if st := c.states[k]; st != nil {
			c.fail(op, op.Path, 0, "note is also created by operation %s", st.creator.ID)
			c.drop(op, "")
			continue
		}
		title := titleOf(op.Path)
		if n := c.v.Resolve(title); n != nil {
			c.fail(op, op.Path, 0, "title %q is already used by %s, so links to it would be ambiguous", title, n.Path)
			c.drop(op, fmt.Sprintf("created by operation %s, which has errors", op.ID))
			continue
		}
		if other := titles[key(title)]; other != nil {
			c.fail(op, op.Path, 0, "title %q is also used by %s, created by operation %s", title, other.Path, other.ID)
			c.drop(op, fmt.Sprintf("created by operation %s, which has errors", op.ID))
			continue
		}
		titles[key(title)] = op
		c.states[k] = &state{path: op.Path, creator: op}
	}
}

// target finds the note op acts on: an existing note, or (if allowCreated)
// one made earlier in the plan.
func (c *checker) target(op *Operation, allowCreated bool) *state {
	if n := c.v.Note(op.Note); n != nil {
		k := key(n.Path)
		st := c.states[k]
		if st == nil {
			st = &state{path: n.Path, orig: n}
			c.states[k] = st
		}
		return st
	}
	k := key(op.Note)
	if st := c.states[k]; st != nil && st.creator != nil && !c.bad[st.creator] {
		if !allowCreated {
			c.fail(op, op.Note, 0, "note is created by operation %s in this plan; put the content there instead", st.creator.ID)
			return nil
		}
		return st
	}
	if why, ok := c.gone[k]; ok && why != "" {
		c.fail(op, op.Note, 0, "note is %s", why)
		return nil
	}
	c.fail(op, op.Note, 0, "no note %s in the vault", op.Note)
	return nil
}

// resolveSources checks rewrite and split sources against the vault.
func (c *checker) resolveSources() {
	rewrites := map[string]*Operation{}
	for _, op := range c.live() {
		if op.Op == Rewrite {
			if _, ok := rewrites[key(op.Note)]; !ok {
				rewrites[key(op.Note)] = op
			}
		}
	}
	for _, op := range c.live() {
		if op.Op != Rewrite && op.Op != Split {
			continue
		}
		if rw := rewrites[key(op.Note)]; op.Op == Split && rw != nil {
			c.fail(op, op.Note, 0, "note is also rewritten by %s; a note is either rewritten or split, so put the split in the rewrite", rw.ID)
			c.drop(op, fmt.Sprintf("created by operation %s, which has errors", op.ID))
			continue
		}
		st := c.target(op, false)
		if st == nil {
			c.drop(op, fmt.Sprintf("created by operation %s, which has errors", op.ID))
			continue
		}
		ok := true
		var sels []selection
		for _, src := range op.Sources {
			s, good := c.resolveSource(op, st.orig, src)
			if !good {
				ok = false
				continue
			}
			sels = append(sels, s)
		}
		if !ok {
			c.drop(op, fmt.Sprintf("created by operation %s, which has errors", op.ID))
			continue
		}
		c.sel[op] = sels
	}
}

func (c *checker) resolveSource(op *Operation, n *note.Note, src Source) (selection, bool) {
	a, b := src.Lines[0], src.Lines[1]
	if sn := c.v.Note(src.Note); sn != n {
		c.fail(op, src.Note, a, "sources must come from %s, the note the operation acts on", n.Path)
		return selection{}, false
	}
	if a < 1 || b < a {
		c.fail(op, n.Path, a, "lines [%d, %d] are not a valid 1-based range", a, b)
		return selection{}, false
	}
	var blk *note.Block
	var where []string
	for i := range n.Blocks {
		if n.Blocks[i].Hash != src.Block {
			continue
		}
		bl := &n.Blocks[i]
		where = append(where, fmt.Sprintf("%d-%d", bl.StartLine, bl.EndLine))
		if bl.StartLine <= a && b <= bl.EndLine {
			blk = bl
			break
		}
	}
	if blk == nil {
		if where == nil {
			c.fail(op, n.Path, a, "no block %s in %s (has the note changed since scan? scan again)", src.Block, n.Path)
		} else {
			c.fail(op, n.Path, a, "lines %d-%d are outside block %s (lines %s)", a, b, src.Block, strings.Join(where, ", "))
		}
		return selection{}, false
	}
	s := selection{src: src, note: n}
	last := -1
	for i, it := range blk.Items {
		if it.Line >= a && it.Line <= b {
			s.items = append(s.items, it)
			last = i
		}
	}
	if len(s.items) == 0 || s.items[0].Line != a {
		c.fail(op, n.Path, a, "line %d is not the start of a bullet in block %s", a, src.Block)
		return selection{}, false
	}
	ends := itemEnd(n, blk, last)
	if b < ends {
		c.fail(op, n.Path, b, "lines %d-%d end inside the bullet at line %d, which ends at line %d", a, b, blk.Items[last].Line, ends)
		return selection{}, false
	}
	starts := lineStarts(n.Src)
	s.start = starts[a-1]
	s.end = starts[b]
	pathKey := key(n.Path)
	if c.mapped[pathKey] == nil {
		c.mapped[pathKey] = map[int]string{}
	}
	ok := true
	for _, it := range s.items {
		if other, dup := c.mapped[pathKey][it.Line]; dup {
			c.fail(op, n.Path, it.Line, "bullet is also mapped by %s; each bullet maps to exactly one destination", other)
			ok = false
			continue
		}
		c.mapped[pathKey][it.Line] = op.ID
	}
	return s, ok
}

// itemEnd returns the last non-blank line of item i of blk.
func itemEnd(n *note.Note, blk *note.Block, i int) int {
	if i == len(blk.Items)-1 {
		return blk.EndLine
	}
	end := blk.Items[i+1].Line - 1
	lines := strings.Split(string(n.Src), "\n")
	for end > blk.Items[i].Line && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
}

// lineStarts returns the byte offset of each line's start, plus len(src) as
// the start of the line after the last.
func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' && i+1 < len(src) {
			starts = append(starts, i+1)
		}
	}
	return append(starts, len(src))
}

// resolveTargets attaches every remaining op to the note it touches.
func (c *checker) resolveTargets() {
	for _, op := range c.live() {
		switch op.Op {
		case Create:
			st := c.states[key(op.Path)]
			st.ops = append(st.ops, op)
		case Split:
			st := c.target(op, false)
			st.splits = append(st.splits, op)
			st.ops = append(st.ops, op)
			nst := c.states[key(op.Path)]
			nst.ops = append(nst.ops, op)
		case Rewrite:
			st := c.target(op, false)
			if st.rewrite != nil {
				c.fail(op, st.path, 0, "note is also rewritten by %s; combine them into one rewrite", st.rewrite.ID)
				c.drop(op, "")
				continue
			}
			st.rewrite = op
			st.ops = append(st.ops, op)
		case SetFrontmatter, AddLinks:
			st := c.target(op, true)
			if st == nil {
				c.drop(op, "")
				continue
			}
			st.edits = append(st.edits, op)
			st.ops = append(st.ops, op)
		case Keep:
			st := c.target(op, false)
			if st == nil {
				c.drop(op, "")
				continue
			}
			st.keeps = append(st.keeps, op)
		}
	}
}

// checkKeeps rejects a keep on a note another applied operation touches, and
// a second keep on one note.
func (c *checker) checkKeeps() {
	for _, st := range c.sortedStates() {
		for i, op := range st.keeps {
			switch {
			case i > 0:
				c.fail(op, st.path, 0, "note is also kept by %s; keep a note once", st.keeps[0].ID)
				c.drop(op, "")
			case len(st.ops) > 0:
				c.fail(op, st.path, 0, "note is also changed by %s; keep is for a note the plan leaves as it is", st.ops[0].ID)
				c.drop(op, "")
			}
		}
	}
}

// kept returns the sorted paths of the notes with an applied keep.
func (c *checker) kept() []string {
	var out []string
	for _, st := range c.sortedStates() {
		for _, op := range st.keeps {
			if !c.bad[op] {
				out = append(out, st.path)
				break
			}
		}
	}
	return out
}

// compute builds each touched note's post-plan content. Order within a note:
// the note's content (created, rewritten, or with splits cut out), then
// frontmatter changes, then appended links, each in plan order.
func (c *checker) compute() {
	for _, st := range c.sortedStates() {
		switch {
		case st.creator != nil:
			if c.bad[st.creator] {
				continue
			}
			st.after = withNL(st.creator.Content)
		case st.rewrite != nil && !c.bad[st.rewrite]:
			st.after = withNL(st.rewrite.Content)
		default:
			st.after = string(st.orig.Src)
			var edits []note.Edit
			var splits []*Operation
			for _, sp := range st.splits {
				if c.bad[sp] {
					continue
				}
				splits = append(splits, sp)
				sels := append([]selection(nil), c.sel[sp]...)
				sort.Slice(sels, func(i, j int) bool { return sels[i].start < sels[j].start })
				for i, s := range sels {
					e := note.Edit{Start: s.start, End: s.end}
					if i == 0 {
						e.Text = withNL(splitLink(sp))
					}
					edits = append(edits, e)
				}
			}
			if len(edits) > 0 {
				out, err := st.orig.Rewrite(edits...)
				if err != nil {
					for _, sp := range splits {
						c.fail(sp, st.path, 0, "splits overlap: %v", err)
					}
					continue
				}
				st.after = string(out)
			}
		}
		for _, op := range st.edits {
			if c.bad[op] {
				continue
			}
			switch op.Op {
			case SetFrontmatter:
				out, err := setFrontmatter(st.after, op.Frontmatter)
				if err != nil {
					c.fail(op, st.path, 0, "%v", err)
					continue
				}
				st.after = out
			case AddLinks:
				st.after = withNL(st.after)
				for _, l := range op.Links {
					st.after += "- [[" + strings.TrimSpace(l) + "]]\n"
				}
			}
		}
	}
}

func (c *checker) sortedStates() []*state {
	var out []*state
	for _, st := range c.states {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

func withNL(s string) string {
	if s != "" && !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}

// splitLink is the line a split leaves behind.
func splitLink(op *Operation) string {
	if op.Link != "" {
		return strings.TrimRight(op.Link, "\n")
	}
	return "- [[" + titleOf(op.Path) + "]]"
}

// destBullet is one bullet of an operation's content.
type destBullet struct {
	item note.Item
	key  string
	used bool
}

// checkLossless checks every content-moving op: each source bullet appears
// in the op's content, a rewrite maps every bullet of its note, and the
// content holds no bullet that is neither from a source nor only links.
func (c *checker) checkLossless() {
	for _, op := range c.live() {
		if op.Op != Rewrite && op.Op != Split && op.Op != Create {
			continue
		}
		dest := op.Path
		if op.Op == Rewrite {
			dest = c.v.Note(op.Note).Path
		}
		dn := note.Parse(dest, []byte(op.Content))
		var bullets []*destBullet
		for _, b := range dn.Blocks {
			for _, it := range b.Items {
				bullets = append(bullets, &destBullet{item: it, key: matchKey(it.Text)})
			}
		}
		take := func(k string) bool {
			for _, d := range bullets {
				if !d.used && d.key == k {
					d.used = true
					return true
				}
			}
			return false
		}
		type miss struct {
			n  *note.Note
			it note.Item
		}
		var missing []miss
		for _, s := range c.sel[op] {
			for _, it := range s.items {
				if !take(matchKey(it.Text)) {
					missing = append(missing, miss{s.note, it})
				}
			}
		}
		for _, m := range missing {
			k := matchKey(m.it.Text)
			if rest, open := strings.CutPrefix(k, "[ ] "); open && take("[x] "+rest) {
				c.fail(op, m.n.Path, m.it.Line, "closes an action item (%q); never-tier changes are not permitted", firstLine(m.it.Text))
				continue
			}
			c.fail(op, m.n.Path, m.it.Line, "bullet not found in %s (dropped or reworded): %q", dest, firstLine(m.it.Text))
		}
		for _, d := range bullets {
			if !d.used && !linkOnly(d.item.Text) {
				c.fail(op, dest, d.item.Line, "bullet maps to no source bullet; never-tier changes (inventing content) are not permitted: %q", firstLine(d.item.Text))
			}
		}
		if op.Op == Rewrite {
			n := c.v.Note(op.Note)
			mine := c.mapped[key(n.Path)]
			for _, b := range n.Blocks {
				for _, it := range b.Items {
					if mine[it.Line] != op.ID {
						c.fail(op, n.Path, it.Line, "bullet is not mapped to any destination: %q", firstLine(it.Text))
					}
				}
			}
			c.checkFrontmatterKept(op, n, dn)
			c.checkProse(op, n, dn)
			for _, seg := range n.Segments {
				if seg.Kind == note.KindMermaid && !strings.Contains(op.Content, n.SegmentText(seg)) {
					c.fail(op, n.Path, seg.StartLine, "mermaid diagram at lines %d-%d is not preserved byte for byte", seg.StartLine, seg.EndLine)
				}
			}
			c.checkAddedSegments(op, n, dn)
			c.checkRewriteLinks(op, n, dn)
		}
		if op.Op == Split {
			c.checkSplitLink(op)
		}
	}
}

// proseLine is one non-bullet line of a note: a heading, a prose line or a
// line of an ordinary code fence.
type proseLine struct {
	line int
	text string // the raw line
	// key is the matchKey of the line; a heading's key is its text only, and
	// a code line's is its codeKey.
	key     string
	heading bool
	code    bool
}

// proseLines returns the non-blank lines of n outside frontmatter, bullet
// blocks, generated blocks and Mermaid diagrams, in order.
func proseLines(n *note.Note) []proseLine {
	headings := map[int]string{}
	for _, h := range n.Headings {
		headings[h.Line] = h.Text
	}
	var out []proseLine
	for _, seg := range n.Segments {
		if seg.Kind != note.KindHeading && seg.Kind != note.KindText && seg.Kind != note.KindCode {
			continue
		}
		for i, ln := range strings.Split(strings.TrimRight(n.SegmentText(seg), "\r\n"), "\n") {
			ln = strings.TrimRight(ln, "\r")
			if strings.TrimSpace(ln) == "" {
				continue
			}
			pl := proseLine{line: seg.StartLine + i, text: ln, key: matchKey(ln)}
			if seg.Kind == note.KindCode {
				pl.code, pl.key = true, codeKey(ln)
			} else if h, ok := headings[pl.line]; ok {
				pl.heading, pl.key = true, matchKey(h)
			}
			out = append(out, pl)
		}
	}
	return out
}

// checkProse checks that a rewrite keeps every original heading, prose line
// and code line, and adds no line but headings. Headings and prose match on
// normalized text like bullets: a prose line may become a heading and the
// reverse, and the heading level is free. Lines of an ordinary code fence
// match exactly, ignoring only trailing whitespace. Any other non-blank line
// the content adds is invented content. Bullets, frontmatter, generated
// blocks and Mermaid diagrams are checked elsewhere.
func (c *checker) checkProse(op *Operation, n, dn *note.Note) {
	orig, dest := proseLines(n), proseLines(dn)
	avail := map[string]int{}
	for _, d := range dest {
		avail[d.key]++
	}
	left := map[string]int{}
	for _, o := range orig {
		left[o.key]++
		if o.key == "" || avail[o.key] > 0 {
			avail[o.key]--
			continue
		}
		switch {
		case o.code:
			c.fail(op, n.Path, o.line, "code line not found in %s (code must match exactly, ignoring trailing whitespace): %q", n.Path, strings.TrimRight(o.text, " \t"))
		case o.heading:
			c.fail(op, n.Path, o.line, "heading not found in %s (dropped or reworded): %q", n.Path, strings.TrimSpace(o.text))
		default:
			c.fail(op, n.Path, o.line, "line not found in %s (dropped or reworded): %q", n.Path, strings.TrimSpace(o.text))
		}
	}
	// Lines that are not headings take the original lines first, so a
	// heading the content adds never leaves a kept line unmatched.
	for _, d := range dest {
		if d.heading || d.key == "" {
			continue
		}
		if left[d.key] > 0 {
			left[d.key]--
			continue
		}
		c.fail(op, dn.Path, d.line, "line maps to no original line; never-tier changes (inventing content) are not permitted, and a rewrite may add only headings: %q", strings.TrimSpace(d.text))
	}
}

// checkFrontmatterKept checks that a rewrite leaves the note's frontmatter
// byte for byte, neither changed, dropped nor added: frontmatter changes go
// through set_frontmatter, where review shows them field by field.
func (c *checker) checkFrontmatterKept(op *Operation, n, dn *note.Note) {
	text := func(n *note.Note) string {
		if len(n.Segments) > 0 && n.Segments[0].Kind == note.KindFrontmatter {
			return strings.TrimSuffix(n.SegmentText(n.Segments[0]), "\n")
		}
		return ""
	}
	if text(n) != text(dn) {
		c.fail(op, n.Path, 1, "frontmatter is not kept byte for byte; a rewrite leaves frontmatter as it is, so change fields with set_frontmatter")
	}
}

// checkAddedSegments rejects Mermaid diagrams and generated blocks that a
// rewrite's content holds but the original note does not.
func (c *checker) checkAddedSegments(op *Operation, n, dn *note.Note) {
	have := map[string]int{}
	segKey := func(n *note.Note, seg note.Segment) string {
		return string(seg.Kind) + "\x00" + strings.TrimSuffix(n.SegmentText(seg), "\n")
	}
	for _, seg := range n.Segments {
		if seg.Kind == note.KindMermaid || seg.Kind == note.KindGenerated {
			have[segKey(n, seg)]++
		}
	}
	for _, seg := range dn.Segments {
		if seg.Kind != note.KindMermaid && seg.Kind != note.KindGenerated {
			continue
		}
		if k := segKey(dn, seg); have[k] > 0 {
			have[k]--
			continue
		}
		if seg.Kind == note.KindMermaid {
			c.fail(op, dn.Path, seg.StartLine, "mermaid diagram at lines %d-%d is not in the original note; never-tier changes (inventing content) are not permitted", seg.StartLine, seg.EndLine)
		} else {
			c.fail(op, dn.Path, seg.StartLine, "generated block %s at lines %d-%d is not in the original note; generated blocks are written by vault-curator, not by a plan", seg.Name, seg.StartLine, seg.EndLine)
		}
	}
}

// checkRewriteLinks checks that a rewrite at tier auto keeps every link of
// the note's body: the same multiset of kind, target, fragment and URL, plus
// any links the content adds. Labels are checked as text by the lossless
// match. Retargeting, such as redirecting links in an entity merge, needs
// tier approve-each, where review shows the operation by itself.
// Frontmatter links are kept with the frontmatter; links in generated blocks
// are not the user's.
func (c *checker) checkRewriteLinks(op *Operation, n, dn *note.Note) {
	if op.Tier != TierAuto {
		return
	}
	avail := map[string]int{}
	for _, l := range bodyLinks(dn) {
		avail[linkKey(l)]++
	}
	for _, l := range bodyLinks(n) {
		if k := linkKey(l); avail[k] > 0 {
			avail[k]--
			continue
		}
		c.fail(op, n.Path, l.Line, "link %s is changed or removed; a rewrite at tier auto keeps every link's target, heading and URL (retarget links at tier approve-each)", l.Raw)
	}
}

// bodyLinks returns n's links outside frontmatter and generated blocks.
func bodyLinks(n *note.Note) []note.Link {
	var out []note.Link
outer:
	for _, l := range n.Links {
		if l.InFrontmatter {
			continue
		}
		for _, seg := range n.Segments {
			if seg.Kind == note.KindGenerated && seg.Start <= l.Start && l.Start < seg.End {
				continue outer
			}
		}
		out = append(out, l)
	}
	return out
}

// linkKey is what a rewrite keeps of a link: kind, embed, target and
// fragment. A note target matches case-insensitively and with or without
// .md, as links resolve; an external URL matches exactly.
func linkKey(l note.Link) string {
	target := l.Target
	if !l.External {
		target = key(target)
	}
	return fmt.Sprintf("%s\x00%t\x00%t\x00%s\x00%s", l.Kind, l.Embed, l.External, target, l.Fragment)
}

// checkSplitLink checks that a split's leftover line links to its new note.
func (c *checker) checkSplitLink(op *Operation) {
	title := titleOf(op.Path)
	for _, l := range note.ParseLinks(splitLink(op)) {
		t := stripMD(l.Target)
		if l.Kind == note.Wikilink && (strings.EqualFold(t, title) || strings.EqualFold(t, stripMD(op.Path))) {
			return
		}
	}
	c.fail(op, op.Note, 0, "link %q must link to the new note [[%s]]", splitLink(op), title)
}

func (c *checker) changes() []Change {
	var out []Change
	for _, st := range c.sortedStates() {
		var ops []string
		for _, op := range st.ops {
			if !c.bad[op] {
				ops = append(ops, op.ID)
			}
		}
		if len(ops) == 0 {
			continue
		}
		ch := Change{Path: st.path, After: []byte(st.after), Ops: ops}
		if st.orig != nil {
			if st.after == string(st.orig.Src) {
				continue
			}
			ch.Before = st.orig.Src
		}
		out = append(out, ch)
	}
	return out
}

// checkLinks reports links in changed notes that resolve to nothing once the
// plan is applied, except broken links the note already had.
func (c *checker) checkLinks(changes []Change) {
	changed := map[string]Change{}
	var notes []*note.Note
	for _, ch := range changes {
		changed[key(ch.Path)] = ch
		notes = append(notes, note.Parse(ch.Path, ch.After))
	}
	for _, n := range c.v.Notes {
		if _, ok := changed[key(n.Path)]; !ok {
			notes = append(notes, n)
		}
	}
	post := vault.New(notes, c.v.Attachments, vault.Options{FallbackHub: c.v.FallbackHub()})
	for _, ch := range changes {
		before := map[string]int{}
		for _, bl := range c.v.Broken {
			if key(bl.From) == key(ch.Path) {
				before[key(bl.Link.Target)]++
			}
		}
		pn := post.Note(ch.Path)
		st := c.states[key(ch.Path)]
		for _, l := range pn.Links {
			if _, res := post.ResolveLink(pn, l); res != vault.Broken {
				continue
			}
			if before[key(l.Target)] > 0 {
				before[key(l.Target)]--
				continue
			}
			c.fail(c.blame(st, l.Raw), ch.Path, l.Line, "broken link %s: no note %q after the plan", l.Raw, l.Target)
		}
	}
}

// blame picks the op that wrote raw into st's note.
func (c *checker) blame(st *state, raw string) *Operation {
	var last *Operation
	for _, op := range st.ops {
		if c.bad[op] {
			continue
		}
		last = op
		text := op.Content
		switch op.Op {
		case Split:
			if key(op.Path) != key(st.path) {
				text = splitLink(op)
			}
		case SetFrontmatter:
			text = fmt.Sprint(op.Frontmatter)
		case AddLinks:
			text = "[[" + strings.Join(op.Links, "]] [[") + "]]"
		}
		if strings.Contains(text, raw) {
			return op
		}
	}
	return last
}

// matchKey is the form the lossless check compares: links are replaced by
// the text they display, then the text is normalized as for block hashes.
// Linking a word ("tidegate" to "[[Tidegate|tidegate]]") keeps the key.
// Lines of a fenced code block (a fence inside a bullet) are not normalized:
// they are kept as written, less trailing whitespace and the fence's own
// indentation, so reindenting the bullet keeps the key but changing the
// code does not.
func matchKey(text string) string {
	fenced := note.FencedLines(text)
	links := note.ParseLinks(text)
	for i := len(links) - 1; i >= 0; i-- {
		l := links[i]
		if l.Embed {
			continue
		}
		shown := l.Label
		if shown == "" && l.Kind == note.Wikilink {
			shown = strings.TrimSuffix(strings.TrimPrefix(l.Raw, "[["), "]]")
		}
		text = text[:l.Start] + shown + text[l.End:]
	}
	// Links never span lines, so text still has the lines fenced describes.
	var parts, prose []string
	flush := func() {
		if k := note.Normalize(strings.Join(prose, "\n")); k != "" {
			parts = append(parts, k)
		}
		prose = nil
	}
	for i, ln := range strings.Split(text, "\n") {
		if fenced[i] < 0 {
			prose = append(prose, ln)
			continue
		}
		flush()
		parts = append(parts, codeKey(dedent(ln, fenced[i])))
	}
	flush()
	return strings.Join(parts, "\n")
}

// codeKey is the key of a line of code: the line less trailing whitespace,
// marked so it never equals the key of a prose line.
func codeKey(ln string) string {
	return "\x00" + strings.TrimRightFunc(ln, unicode.IsSpace)
}

// dedent removes up to col columns of indentation from ln, a tab advancing
// to the next multiple of 4 as in the note parser.
func dedent(ln string, col int) string {
	c := 0
	for i := 0; i < len(ln); i++ {
		if c >= col {
			return ln[i:]
		}
		switch ln[i] {
		case ' ':
			c++
		case '\t':
			c = (c/4 + 1) * 4
		default:
			return ln[i:]
		}
	}
	return ""
}

// linkOnly reports whether bullet text holds at least one link and nothing
// else but a list marker, whitespace and punctuation.
func linkOnly(text string) bool {
	links := note.ParseLinks(text)
	if len(links) == 0 {
		return false
	}
	for i := len(links) - 1; i >= 0; i-- {
		text = text[:links[i].Start] + text[links[i].End:]
	}
	for _, r := range note.Normalize(text) {
		if !unicode.IsSpace(r) && !unicode.IsPunct(r) {
			return false
		}
	}
	return true
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return strings.TrimSpace(s)
}

func key(s string) string { return strings.ToLower(strings.TrimSpace(stripMD(s))) }

func stripMD(p string) string {
	if ext := path.Ext(p); strings.EqualFold(ext, ".md") {
		return p[:len(p)-len(ext)]
	}
	return p
}

func titleOf(p string) string { return stripMD(path.Base(p)) }
