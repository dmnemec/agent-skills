// Package review renders a curation plan as a markdown review document and
// reads the user's decisions back into the plan (design doc: "Architecture",
// step 3).
//
// The document groups operations by category, with one status line per
// category and one per approve-each operation:
//
//   - category `split` status: pending
//   - op `split-beacon` status: rejected
//
// Sync reads only these lines, outside fenced code. An auto operation whose
// own status is pending (or which has no line) takes its category's status,
// so the user approves a category and then marks the exceptions. An
// approve-each operation never does: it needs its own approved or rejected,
// and sync fails while one is still pending. A comment at the top records the
// plan's hash, so a review cannot be synced into a plan that changed since it
// was rendered.
package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/plan"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// Options names the files the document refers to.
type Options struct {
	// PlanPath and ReviewPath are the plan and review files, as shown in
	// the document's title and its sync command.
	PlanPath   string
	ReviewPath string
	// Ignored is passed to plan.Options.Ignored. Nil skips the check.
	Ignored func(path string) (bool, error)
}

// categories lists every category in the order review shows them: the ones
// approved one by one first.
var categories = []struct {
	cat   plan.Category
	title string
}{
	{plan.CategorySplit, "Splits"},
	{plan.CategoryEntity, "New entities"},
	{plan.CategoryNote, "New notes"},
	{plan.CategoryRestructure, "Restructures"},
	{plan.CategoryFrontmatter, "Frontmatter"},
	{plan.CategoryLinks, "Links"},
	{plan.CategoryKeep, "Kept notes"},
}

var statuses = []plan.Status{plan.Pending, plan.Approved, plan.Rejected}

// Hash identifies everything in p except its statuses, so syncing a review
// (which changes only statuses) keeps the hash, and any other change to the
// plan does not.
func Hash(p *plan.Plan) (string, error) {
	q := *p
	q.Operations = append([]plan.Operation(nil), p.Operations...)
	for i := range q.Operations {
		q.Operations[i].Status = ""
	}
	data, err := q.Marshal()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

const headerFormat = "<!-- vault-curator review plan-sha256=%s -->"

var headerRE = regexp.MustCompile(`^<!-- vault-curator review plan-sha256=([0-9a-f]{64}) -->$`)

// Render returns the review document for p. The plan must pass
// plan.Validate with Options.Draft, since review is for plans the agent has
// already checked.
func Render(v *vault.Vault, p *plan.Plan, opts Options) ([]byte, error) {
	r := plan.Validate(v, p, plan.Options{Draft: true, Ignored: opts.Ignored})
	if !r.OK() {
		var b strings.Builder
		b.WriteString("plan does not pass validate --draft:")
		for _, e := range r.Errors {
			b.WriteString("\n  " + e.Error())
		}
		return nil, errors.New(b.String())
	}
	hash, err := Hash(p)
	if err != nil {
		return nil, err
	}
	w := &writer{v: v, changes: map[string]plan.Change{}}
	for _, c := range r.Changes {
		w.changes[c.Path] = c
	}
	w.header(p, hash, opts)
	w.summary(p, r.Changes)
	for _, c := range categories {
		ops := opsIn(p, c.cat)
		if len(ops) > 0 {
			if err := w.category(c.title, c.cat, ops); err != nil {
				return nil, err
			}
		}
	}
	return []byte(strings.TrimRight(w.String(), "\n") + "\n"), nil
}

func opsIn(p *plan.Plan, cat plan.Category) []*plan.Operation {
	var out []*plan.Operation
	for i := range p.Operations {
		if p.Operations[i].Category == cat {
			out = append(out, &p.Operations[i])
		}
	}
	return out
}

// categoryStatus is the status a category line shows: the one all its
// operations share, or pending.
func categoryStatus(ops []*plan.Operation) plan.Status {
	for _, op := range ops[1:] {
		if op.Status != ops[0].Status {
			return plan.Pending
		}
	}
	return ops[0].Status
}

// ownLine reports whether op gets its own status line: every approve-each
// operation does, and an auto one whose status differs from its category's.
func ownLine(op *plan.Operation, cat plan.Status) bool {
	return op.Tier != plan.TierAuto || op.Status != cat
}

type writer struct {
	strings.Builder
	v       *vault.Vault
	changes map[string]plan.Change
}

func (w *writer) f(format string, args ...any) { fmt.Fprintf(w, format, args...) }

func (w *writer) header(p *plan.Plan, hash string, opts Options) {
	w.f(headerFormat+"\n", hash)
	w.f("# Curation review: %s\n\n", path.Base(filepath.ToSlash(opts.PlanPath)))
	w.f("Decide each change by setting its `status:` to `approved` or `rejected`. ")
	w.f("A category's status decides its auto operations (listed as decided by the category status), ")
	w.f("so approve a category and then reject the exceptions. ")
	w.f("Every other operation has its own `op` line and is decided one by one: ")
	w.f("sync fails until each is `approved` or `rejected`, whatever its category says. ")
	w.f("Only the `status:` lines are read back; other edits are ignored. ")
	w.f("Then write the decisions into the plan:\n\n")
	w.f("```sh\nvault-curator review --sync %s %s\n```\n\n", shellQuote(opts.ReviewPath), shellQuote(opts.PlanPath))
}

func (w *writer) summary(p *plan.Plan, changes []plan.Change) {
	w.f("## Summary\n\n")
	w.f("| Category | Auto | Approve each | Status |\n| --- | ---: | ---: | --- |\n")
	var auto, each int
	all := map[plan.Status]int{}
	for _, c := range categories {
		ops := opsIn(p, c.cat)
		if len(ops) == 0 {
			continue
		}
		var a, e int
		counts := map[plan.Status]int{}
		for _, op := range ops {
			if op.Tier == plan.TierAuto {
				a++
			} else {
				e++
			}
			counts[op.Status]++
			all[op.Status]++
		}
		auto += a
		each += e
		w.f("| %s | %d | %d | %s |\n", c.title, a, e, countStatuses(counts))
	}
	w.f("| **Total** | %d | %d | %s |\n\n", auto, each, countStatuses(all))

	if len(changes) == 0 {
		return
	}
	w.f("Notes the plan changes, if every pending operation is approved:\n\n")
	for _, c := range changes {
		verb := "update"
		if c.Created() {
			verb = "create"
		}
		w.f("- %s `%s` (%s)\n", verb, c.Path, strings.Join(c.Ops, ", "))
	}
	w.f("\n")
}

func countStatuses(counts map[plan.Status]int) string {
	var parts []string
	for _, s := range statuses {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	return strings.Join(parts, ", ")
}

func (w *writer) category(title string, cat plan.Category, ops []*plan.Operation) error {
	status := categoryStatus(ops)
	w.f("## %s\n\n", title)
	w.f("- category `%s` status: %s\n\n", cat, status)
	if cat == plan.CategorySplit {
		if err := w.boundaries(ops); err != nil {
			return err
		}
	}
	// Auto edits without their own status line are listed compactly; the
	// rest get a section each.
	var compact, full []*plan.Operation
	for _, op := range ops {
		if !ownLine(op, status) && (op.Op == plan.SetFrontmatter || op.Op == plan.AddLinks || op.Op == plan.Keep) {
			compact = append(compact, op)
		} else {
			full = append(full, op)
		}
	}
	if len(compact) > 0 {
		w.f("Decided by the category status:\n\n")
		for _, op := range compact {
			w.f("- **%s**: %s\n", op.ID, w.describe(op))
		}
		w.f("\n")
	}
	for _, op := range full {
		w.f("### %s\n\n", op.ID)
		if ownLine(op, status) {
			w.f("- op `%s` status: %s\n\n", op.ID, op.Status)
		} else {
			w.f("Decided by the category status.\n\n")
		}
		if op.Summary != "" {
			w.f("%s\n\n", op.Summary)
		}
		if err := w.details(op); err != nil {
			return err
		}
	}
	return nil
}

// describe is one line saying what a set_frontmatter, add_links or keep op does.
func (w *writer) describe(op *plan.Operation) string {
	var b strings.Builder
	if op.Summary != "" {
		b.WriteString(strings.TrimSuffix(op.Summary, ".") + ". ")
	}
	switch op.Op {
	case plan.SetFrontmatter:
		keys := make([]string, 0, len(op.Frontmatter))
		for k := range op.Frontmatter {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var set, unset []string
		for _, k := range keys {
			val := op.Frontmatter[k]
			if val == nil {
				unset = append(unset, "`"+k+"`")
				continue
			}
			s, ok := val.(string)
			if !ok {
				data, _ := json.Marshal(val)
				s = string(data)
			}
			set = append(set, "`"+k+": "+s+"`")
		}
		fmt.Fprintf(&b, "In `%s`", op.Note)
		if len(set) > 0 {
			b.WriteString(", sets " + strings.Join(set, ", "))
		}
		if len(unset) > 0 {
			b.WriteString(", removes " + strings.Join(unset, ", "))
		}
		b.WriteString(".")
	case plan.Keep:
		fmt.Fprintf(&b, "Keeps `%s` as is: no change, and scan stops listing its current blocks.", op.Note)
	case plan.AddLinks:
		links := make([]string, len(op.Links))
		for i, l := range op.Links {
			links[i] = "`- [[" + strings.TrimSpace(l) + "]]`"
		}
		fmt.Fprintf(&b, "Appends %s to `%s`.", strings.Join(links, ", "), op.Note)
	}
	return b.String()
}

func (w *writer) details(op *plan.Operation) error {
	switch op.Op {
	case plan.Split:
		var ranges []string
		for _, s := range op.Sources {
			ranges = append(ranges, lineRange(s.Lines))
		}
		w.f("Moves %s of `%s` into the new note `%s`, leaving `%s` in their place:\n\n",
			strings.Join(ranges, ", "), op.Note, op.Path, op.SplitLink())
		w.fence("markdown", op.Content)
	case plan.Create:
		w.f("Creates `%s`:\n\n", op.Path)
		w.fence("markdown", op.Content)
	case plan.Rewrite:
		n := w.v.Note(op.Note)
		if n == nil {
			return fmt.Errorf("op %s: note %s not found", op.ID, op.Note)
		}
		w.f("Rewrites `%s`. Before:\n\n", n.Path)
		w.fence("markdown", string(n.Src))
		w.f("After:\n\n")
		w.fence("markdown", op.Content)
	default:
		w.f("%s\n\n", w.describe(op))
	}
	return nil
}

func lineRange(l [2]int) string {
	if l[0] == l[1] {
		return fmt.Sprintf("line %d", l[0])
	}
	return fmt.Sprintf("lines %d-%d", l[0], l[1])
}

// boundaries shows, for each note the splits act on, every original bullet
// grouped by where it goes: the lossless mapping.
func (w *writer) boundaries(ops []*plan.Operation) error {
	var notes []*note.Note
	dest := map[*note.Note]map[int]*plan.Operation{}
	for _, op := range ops {
		if op.Op != plan.Split {
			continue
		}
		n := w.v.Note(op.Note)
		if n == nil {
			return fmt.Errorf("op %s: note %s not found", op.ID, op.Note)
		}
		if dest[n] == nil {
			dest[n] = map[int]*plan.Operation{}
			notes = append(notes, n)
		}
		for _, src := range op.Sources {
			items, err := plan.Selected(w.v, src)
			if err != nil {
				return fmt.Errorf("op %s: %w", op.ID, err)
			}
			for _, it := range items {
				dest[n][it.Line] = op
			}
		}
	}
	for _, n := range notes {
		w.f("### Boundaries in %s\n\n", n.Path)
		w.f("Every bullet of `%s`, by line, under the note it goes to:\n\n", n.Path)
		var b strings.Builder
		group := "-"
		for _, blk := range n.Blocks {
			for _, it := range blk.Items {
				g := "stays in " + n.Path
				if op := dest[n][it.Line]; op != nil {
					g = op.ID + " → " + op.Path
				}
				if g != group {
					if group != "-" {
						b.WriteString("\n")
					}
					b.WriteString(g + "\n")
					group = g
				}
				for i, l := range strings.Split(it.Text, "\n") {
					num := ""
					if i == 0 {
						num = fmt.Sprint(it.Line)
					}
					fmt.Fprintf(&b, "%4s  %s\n", num, strings.ReplaceAll(l, "\t", "    "))
				}
			}
		}
		w.fence("text", b.String())
		if c, ok := w.changes[n.Path]; ok {
			w.f("After the plan, `%s` reads:\n\n", n.Path)
			w.fence("markdown", string(c.After))
		}
	}
	return nil
}

// fence writes text as a fenced code block, with a fence longer than any
// backtick run inside it. Code keeps wikilinks in the review from becoming
// links, so a review saved in the vault adds nothing to its graph.
func (w *writer) fence(lang, text string) {
	n, run := 3, 0
	for _, r := range text {
		if r == '`' {
			run++
			if run >= n {
				n = run + 1
			}
		} else {
			run = 0
		}
	}
	f := strings.Repeat("`", n)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	w.f("%s%s\n%s%s\n\n", f, lang, text, f)
}

// shellQuote quotes p for the sync command if it needs it.
func shellQuote(p string) string {
	if p != "" && !strings.ContainsAny(p, " \t'\"\\$`!*?[]{}()<>|&;#~") {
		return p
	}
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}

var (
	markerRE = regexp.MustCompile("^\\s*[-*+]\\s+(op|category)\\s+`([^`]+)`\\s+status:\\s*(.*?)\\s*$")
	// looseRE catches status lines that markerRE does not parse, so a
	// mangled line fails instead of being ignored.
	looseRE = regexp.MustCompile(`^\s*[-*+]\s+(op|category)\b.*\bstatus:`)
)

type marker struct {
	status plan.Status
	line   int
}

// Sync reads the status lines of the review document doc and writes the
// decisions into p. It fails, changing nothing, when doc was rendered from a
// different plan, names an operation or category p does not have, lacks a
// line the review needs, or holds a status it cannot read.
func Sync(doc []byte, p *plan.Plan) error {
	var errs []error
	failf := func(line int, format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if line > 0 {
			msg = fmt.Sprintf("line %d: %s", line, msg)
		}
		errs = append(errs, errors.New(msg))
	}

	cats := map[plan.Category]bool{}
	for _, op := range p.Operations {
		cats[op.Category] = true
	}
	ops := map[string]marker{}
	catMarks := map[plan.Category]marker{}
	var hash string
	var fence string
	for i, line := range strings.Split(string(doc), "\n") {
		n := i + 1
		line = strings.TrimSuffix(line, "\r")
		if fence != "" {
			if t := strings.TrimSpace(line); strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				fence = ""
			}
			continue
		}
		if f := openFence(line); f != "" {
			fence = f
			continue
		}
		if m := headerRE.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			if hash != "" {
				failf(n, "second review header")
			}
			hash = m[1]
			continue
		}
		m := markerRE.FindStringSubmatch(line)
		if m == nil {
			if looseRE.MatchString(line) {
				failf(n, "cannot read status line %q; write it as - op `<id>` status: <status>", strings.TrimSpace(line))
			}
			continue
		}
		kind, id := m[1], m[2]
		st := plan.Status(strings.ToLower(m[3]))
		if st != plan.Pending && st != plan.Approved && st != plan.Rejected {
			failf(n, "%s %s: status %q is not pending, approved or rejected", kind, id, m[3])
			continue
		}
		mk := marker{status: st, line: n}
		if kind == "category" {
			c := plan.Category(id)
			if !cats[c] {
				failf(n, "category %s has no operations in the plan", id)
				continue
			}
			if prev, dup := catMarks[c]; dup {
				failf(n, "category %s already has a status at line %d", id, prev.line)
				continue
			}
			catMarks[c] = mk
			continue
		}
		if p.Op(id) == nil {
			failf(n, "op %s is not in the plan", id)
			continue
		}
		if prev, dup := ops[id]; dup {
			failf(n, "op %s already has a status at line %d", id, prev.line)
			continue
		}
		ops[id] = mk
	}

	want, err := Hash(p)
	if err != nil {
		return err
	}
	switch {
	case hash == "":
		failf(0, "no review header (%s); render the review again", fmt.Sprintf(headerFormat, "..."))
	case hash != want:
		failf(0, "the plan changed since this review was rendered; render the review again")
	}
	var missing []string
	for c := range cats {
		if _, ok := catMarks[c]; !ok {
			missing = append(missing, string(c))
		}
	}
	sort.Strings(missing)
	for _, c := range missing {
		failf(0, "category %s has no status line", c)
	}
	var undecided []string
	for _, op := range p.Operations {
		if op.Tier == plan.TierAuto {
			continue
		}
		mk, ok := ops[op.ID]
		switch {
		case !ok:
			failf(0, "op %s is approve-each but has no status line", op.ID)
		case mk.status == plan.Pending:
			undecided = append(undecided, op.ID)
		}
	}
	if len(undecided) > 0 {
		failf(0, "approve-each operations need their own approved or rejected, a category status does not decide them; still pending: %s", strings.Join(undecided, ", "))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	for i := range p.Operations {
		op := &p.Operations[i]
		if mk, ok := ops[op.ID]; ok && mk.status != plan.Pending {
			op.Status = mk.status
		} else {
			op.Status = catMarks[op.Category].status
		}
	}
	return nil
}

// openFence returns the fence a line opens (a run of three or more
// backticks or tildes), or "".
func openFence(line string) string {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 || len(t) < 3 || (t[0] != '`' && t[0] != '~') {
		return ""
	}
	n := 0
	for n < len(t) && t[n] == t[0] {
		n++
	}
	if n < 3 {
		return ""
	}
	return t[:n]
}
