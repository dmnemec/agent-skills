package plan

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

var (
	vaultDir    = filepath.Join("..", "..", "..", "testdata", "vault")
	examplePlan = filepath.Join("..", "..", "..", "testdata", "plans", "2026-09-14.json")
)

func loadExample(t *testing.T) (*vault.Vault, *Plan) {
	t.Helper()
	v, err := vault.Load(vaultDir, vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := Load(examplePlan)
	if err != nil {
		t.Fatal(err)
	}
	return v, p
}

// findErr returns the first error for op whose message contains substr.
func findErr(errs []*Error, op, substr string) *Error {
	for _, e := range errs {
		if e.Op == op && strings.Contains(e.Msg, substr) {
			return e
		}
	}
	return nil
}

func requireErr(t *testing.T, errs []*Error, op, substr string) *Error {
	t.Helper()
	e := findErr(errs, op, substr)
	if e == nil {
		t.Fatalf("no error for op %q containing %q; got:\n%s", op, substr, joinErrs(errs))
	}
	return e
}

func joinErrs(errs []*Error) string {
	var b strings.Builder
	for _, e := range errs {
		b.WriteString("  " + e.Error() + "\n")
	}
	return b.String()
}

func requireOK(t *testing.T, r *Result) {
	t.Helper()
	if len(r.Errors) > 0 {
		t.Fatalf("unexpected errors:\n%s", joinErrs(r.Errors))
	}
}

func change(t *testing.T, r *Result, p string) Change {
	t.Helper()
	for _, c := range r.Changes {
		if c.Path == p {
			return c
		}
	}
	t.Fatalf("no change for %q", p)
	return Change{}
}

func TestExamplePlanPasses(t *testing.T) {
	v, p := loadExample(t)
	r := Validate(v, p, Options{})
	requireOK(t, r)

	var paths []string
	for _, c := range r.Changes {
		paths = append(paths, c.Path)
	}
	want := []string{
		"Coffee machine options.md",
		"daily/2026-09-14.md",
		"meetings/2026-09-14 1-1 Teodor.md",
		"meetings/2026-09-14 Beacon sync.md",
		"meetings/2026-09-14 Harbor standup.md",
		"meetings/2026-09-14 Tidegate retry design.md",
		"people/Teodor Quill.md",
	}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Errorf("changed paths = %q, want %q", paths, want)
	}

	daily := change(t, r, "daily/2026-09-14.md")
	wantDaily := "---\ntype: daily\ndate: 2026-09-14\n---\n" +
		"- [[2026-09-14 Harbor standup]]\n" +
		"- [[2026-09-14 Tidegate retry design]]\n" +
		"- [[2026-09-14 Beacon sync]]\n" +
		"\n" +
		"- [[2026-09-14 1-1 Teodor|1:1 Teodor]]\n"
	if string(daily.After) != wantDaily {
		t.Errorf("daily note after plan:\n%s\nwant:\n%s", daily.After, wantDaily)
	}
	if daily.Before == nil || !strings.HasPrefix(string(daily.Before), "- standup\n") {
		t.Errorf("daily Before = %q", daily.Before)
	}
	if got := strings.Join(daily.Ops, ","); got != "split-standup,split-retry,split-beacon,split-teodor,fm-daily" {
		t.Errorf("daily ops = %s", got)
	}

	retry := change(t, r, "meetings/2026-09-14 Tidegate retry design.md")
	if retry.Before != nil {
		t.Error("a new note has Before")
	}
	if string(retry.After) != p.Op("split-retry").Content {
		t.Errorf("new note content = %q", retry.After)
	}

	coffee := change(t, r, "Coffee machine options.md")
	if !strings.HasSuffix(string(coffee.After), "- ask facilities who signs off\n- [[Unsorted]]\n") {
		t.Errorf("coffee note = %q", coffee.After)
	}
}

func TestExampleMutationsFail(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(p *Plan)
		draft  bool
		op     string
		substr string
		path   string
		line   int
	}{
		{
			name: "dropped bullet",
			mutate: func(p *Plan) {
				o := p.Op("split-retry")
				o.Content = strings.Replace(o.Content, "- open q: dead-letter after the cap or just drop?\n", "", 1)
			},
			op: "split-retry", substr: "not found in", path: "daily/2026-09-14.md", line: 10,
		},
		{
			name: "reworded bullet",
			mutate: func(p *Plan) {
				o := p.Op("split-retry")
				o.Content = strings.Replace(o.Content, "cap at 5, jitter", "cap at five, jitter", 1)
			},
			op: "split-retry", substr: "not found in", path: "daily/2026-09-14.md", line: 7,
		},
		{
			name: "reworded bullet is invented content",
			mutate: func(p *Plan) {
				o := p.Op("split-retry")
				o.Content = strings.Replace(o.Content, "cap at 5, jitter", "cap at five, jitter", 1)
			},
			op: "split-retry", substr: "inventing content", path: "meetings/2026-09-14 Tidegate retry design.md", line: 13,
		},
		{
			name: "broken link",
			mutate: func(p *Plan) {
				o := p.Op("split-beacon")
				o.Content = strings.Replace(o.Content, "[[Beacon|beacon]]", "[[Beacon v2|beacon]]", 1)
			},
			op: "split-beacon", substr: "broken link [[Beacon v2|beacon]]", path: "meetings/2026-09-14 Beacon sync.md", line: 12,
		},
		{
			name:   "link to a note created by a rejected op",
			mutate: func(p *Plan) { p.Op("entity-teodor").Status = Rejected },
			op:     "split-standup", substr: "broken link [[Teodor Quill|Teodor]]", path: "meetings/2026-09-14 Harbor standup.md", line: 9,
		},
		{
			name:   "unapproved op",
			mutate: func(p *Plan) { p.Op("split-beacon").Status = Pending },
			op:     "split-beacon", substr: "pending",
		},
		{
			name:   "missing status",
			mutate: func(p *Plan) { p.Op("fm-daily").Status = "" },
			op:     "fm-daily", substr: "status",
		},
		{
			name:   "never tier",
			mutate: func(p *Plan) { p.Op("split-retry").Tier = TierNever },
			op:     "split-retry", substr: "never-tier",
		},
		{
			name:   "split downgraded to auto",
			mutate: func(p *Plan) { p.Op("split-retry").Tier = TierAuto },
			op:     "split-retry", substr: "requires tier approve-each",
		},
		{
			name:   "wrong category",
			mutate: func(p *Plan) { p.Op("split-retry").Category = CategoryLinks },
			op:     "split-retry", substr: "category",
		},
		{
			name:   "stale block hash",
			mutate: func(p *Plan) { p.Op("split-teodor").Sources[0].Block = "0000000000000000" },
			op:     "split-teodor", substr: "no block 0000000000000000", path: "daily/2026-09-14.md", line: 18,
		},
		{
			name:   "bullet mapped twice",
			mutate: func(p *Plan) { p.Op("split-beacon").Sources[0].Lines = [2]int{11, 16} },
			op:     "split-beacon", substr: "also mapped by split-retry", path: "daily/2026-09-14.md", line: 11,
		},
		{
			name:   "partial range leaves a bullet in both notes",
			mutate: func(p *Plan) { p.Op("split-standup").Sources[0].Lines = [2]int{1, 3} },
			op:     "split-standup", substr: "inventing content",
		},
		{
			name:   "range starts on a non-bullet line",
			mutate: func(p *Plan) { p.Op("split-teodor").Sources[0].Lines = [2]int{17, 21} },
			op:     "split-teodor", substr: "outside block",
		},
		{
			name: "split link does not point at the new note",
			mutate: func(p *Plan) {
				p.Op("split-teodor").Link = "- [[2026-09-14 Harbor standup]]"
			},
			op: "split-teodor", substr: "must link to",
		},
		{
			name:   "duplicate id",
			mutate: func(p *Plan) { p.Op("fm-daily").ID = "split-retry" },
			op:     "split-retry", substr: "duplicate",
		},
		{
			name:   "new note already exists",
			mutate: func(p *Plan) { p.Op("entity-teodor").Path = "people/Mira Vantongeren.md" },
			op:     "entity-teodor", substr: "already exists",
		},
		{
			name:   "new note title already taken",
			mutate: func(p *Plan) { p.Op("entity-teodor").Path = "elsewhere/Harbor.md" },
			op:     "entity-teodor", substr: "already used",
		},
		{
			name:   "illegal file name",
			mutate: func(p *Plan) { p.Op("split-teodor").Path = "meetings/2026-09-14 1:1 Teodor.md" },
			op:     "split-teodor", substr: "not allowed in a file name",
		},
		{
			name:   "unknown note",
			mutate: func(p *Plan) { p.Op("links-coffee").Note = "Tea machine options.md" },
			op:     "links-coffee", substr: "no note",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, p := loadExample(t)
			tc.mutate(p)
			r := Validate(v, p, Options{Draft: tc.draft})
			if len(r.Errors) == 0 {
				t.Fatal("mutated plan passed")
			}
			e := requireErr(t, r.Errors, tc.op, tc.substr)
			if tc.path != "" && e.Path != tc.path {
				t.Errorf("error path = %q, want %q (%v)", e.Path, tc.path, e)
			}
			if tc.line != 0 && e.Line != tc.line {
				t.Errorf("error line = %d, want %d (%v)", e.Line, tc.line, e)
			}
			if r.Changes != nil {
				t.Error("a failed validation returned changes")
			}
		})
	}
}

func TestRejectedSplitKeepsBullets(t *testing.T) {
	v, p := loadExample(t)
	p.Op("split-beacon").Status = Rejected
	r := Validate(v, p, Options{})
	requireOK(t, r)
	daily := string(change(t, r, "daily/2026-09-14.md").After)
	if !strings.Contains(daily, "- [[2026-09-14 Tidegate retry design]]\n\t- beacon sync (Lio joined late") {
		t.Errorf("rejected split's bullets not kept in place:\n%s", daily)
	}
	for _, c := range r.Changes {
		if strings.Contains(c.Path, "Beacon sync") {
			t.Error("rejected split still created its note")
		}
	}
}

func TestDraftTreatsPendingAsApproved(t *testing.T) {
	v, p := loadExample(t)
	for i := range p.Operations {
		p.Operations[i].Status = Pending
	}
	requireOK(t, Validate(v, p, Options{Draft: true}))
	r := Validate(v, p, Options{})
	if len(r.Errors) != len(p.Operations) {
		t.Errorf("want one pending error per op, got:\n%s", joinErrs(r.Errors))
	}
}

// small builds a vault from path-to-content pairs.
func small(files map[string]string) *vault.Vault {
	var notes []*note.Note
	for p, c := range files {
		notes = append(notes, note.Parse(p, []byte(c)))
	}
	return vault.New(notes, nil, vault.Options{})
}

func hashOf(v *vault.Vault, p string, i int) string { return v.Note(p).Blocks[i].Hash }

const meeting = "---\ntype: meeting\ntopics: [\"[[Hub]]\"]\n---\n# Sync\n\n" +
	"- talked about [[Missing]] and hub\n" + // line 7
	"    * decided: ship it\n" + // line 8
	"- [ ] AI me: write it up\n" + // line 9
	"\n" +
	"```mermaid\nflowchart LR\n  a-->b\n```\n" // lines 11-14

func smallVault() *vault.Vault {
	return small(map[string]string{
		"Hub.md":        "---\ntype: project\n---\n# Hub\n",
		"Sync.md":       meeting,
		"Other.md":      "plain text\n",
		"Elsewhere.md":  "- see [[Hub]]\n",
		"Unsorted.md":   "# Unsorted\n",
		"Old/Thing.md":  "- old\n",
		"Person/Ana.md": "---\ntype: person\naliases: [Ana B]\n---\n",
	})
}

func rewriteOp(v *vault.Vault, content string) Operation {
	return Operation{
		ID: "rw", Op: Rewrite, Category: CategoryRestructure, Tier: TierAuto, Status: Approved,
		Note: "Sync.md", Content: content,
		Sources: []Source{
			{Note: "Sync.md", Block: hashOf(v, "Sync.md", 0), Lines: [2]int{7, 8}},
			{Note: "Sync.md", Block: hashOf(v, "Sync.md", 1), Lines: [2]int{9, 9}},
		},
	}
}

const restructured = "---\ntype: meeting\ntopics: [\"[[Hub]]\"]\n---\n# Sync\n\n" +
	"## Notes\n\n- talked about [[Missing]] and [[Hub|hub]]\n\n" +
	"## Decisions\n\n- decided: ship it\n\n" +
	"## Action items\n\n- [ ] AI me: write it up\n- [[Elsewhere]]\n\n" +
	"```mermaid\nflowchart LR\n  a-->b\n```\n"

func TestRewrite(t *testing.T) {
	v := smallVault()
	p := &Plan{Version: Version, Operations: []Operation{rewriteOp(v, restructured)}}
	r := Validate(v, p, Options{})
	requireOK(t, r) // [[Missing]] was already broken before the plan
	if got := string(change(t, r, "Sync.md").After); got != restructured {
		t.Errorf("after = %q", got)
	}

	t.Run("unmapped bullet", func(t *testing.T) {
		o := rewriteOp(v, restructured)
		o.Sources = o.Sources[:1]
		r := Validate(v, &Plan{Version: Version, Operations: []Operation{o}}, Options{})
		e := requireErr(t, r.Errors, "rw", "not mapped")
		if e.Path != "Sync.md" || e.Line != 9 {
			t.Errorf("error at %s:%d", e.Path, e.Line)
		}
	})
	t.Run("closed action item", func(t *testing.T) {
		o := rewriteOp(v, strings.Replace(restructured, "- [ ] AI me", "- [x] AI me", 1))
		r := Validate(v, &Plan{Version: Version, Operations: []Operation{o}}, Options{})
		e := requireErr(t, r.Errors, "rw", "closes an action item")
		if e.Line != 9 {
			t.Errorf("error line = %d", e.Line)
		}
		if findErr(r.Errors, "rw", "inventing") != nil {
			t.Error("closed item also reported as invented")
		}
	})
	t.Run("invented bullet", func(t *testing.T) {
		o := rewriteOp(v, restructured+"- we also agreed on a launch date\n")
		r := Validate(v, &Plan{Version: Version, Operations: []Operation{o}}, Options{})
		e := requireErr(t, r.Errors, "rw", "inventing content")
		if e.Line != 24 {
			t.Errorf("error line = %d", e.Line)
		}
	})
	t.Run("dropped mermaid", func(t *testing.T) {
		o := rewriteOp(v, strings.Replace(restructured, "  a-->b", "    a --> b", 1))
		r := Validate(v, &Plan{Version: Version, Operations: []Operation{o}}, Options{})
		e := requireErr(t, r.Errors, "rw", "mermaid")
		if e.Line != 11 {
			t.Errorf("error line = %d", e.Line)
		}
	})
	t.Run("new broken link", func(t *testing.T) {
		o := rewriteOp(v, strings.Replace(restructured, "[[Hub|hub]]", "[[Hubb|hub]]", 1))
		r := Validate(v, &Plan{Version: Version, Operations: []Operation{o}}, Options{})
		requireErr(t, r.Errors, "rw", "broken link [[Hubb|hub]]")
	})
	t.Run("source from another note", func(t *testing.T) {
		o := rewriteOp(v, restructured)
		o.Sources = append(o.Sources, Source{Note: "Old/Thing.md", Block: hashOf(v, "Old/Thing.md", 0), Lines: [2]int{1, 1}})
		r := Validate(v, &Plan{Version: Version, Operations: []Operation{o}}, Options{})
		requireErr(t, r.Errors, "rw", "must come from")
	})
	t.Run("conflicts with a split", func(t *testing.T) {
		split := Operation{
			ID: "sp", Op: Split, Category: CategorySplit, Tier: TierApproveEach, Status: Approved,
			Note: "Sync.md", Path: "Sync part.md", Content: "- [ ] AI me: write it up\n",
			Sources: []Source{{Note: "Sync.md", Block: hashOf(v, "Sync.md", 1), Lines: [2]int{9, 9}}},
		}
		r := Validate(v, &Plan{Version: Version, Operations: []Operation{rewriteOp(v, restructured), split}}, Options{})
		requireErr(t, r.Errors, "sp", "also rewritten by rw")
	})
}

func TestSetFrontmatter(t *testing.T) {
	v := smallVault()
	op := func(note string, fields map[string]any) *Plan {
		return &Plan{Version: Version, Operations: []Operation{{
			ID: "fm", Op: SetFrontmatter, Category: CategoryFrontmatter, Tier: TierAuto, Status: Approved,
			Note: note, Frontmatter: fields,
		}}}
	}
	r := Validate(v, op("Sync.md", map[string]any{
		"topics":    nil,
		"project":   "[[Hub]]",
		"date":      "2026-10-17",
		"attendees": []any{"[[Ana B]]", "Bo"},
		"type":      "meeting",
		"zeta":      true,
	}), Options{})
	requireOK(t, r)
	got := string(change(t, r, "Sync.md").After)
	want := "---\ntype: meeting\ndate: 2026-10-17\nproject: \"[[Hub]]\"\nattendees:\n  - \"[[Ana B]]\"\n  - Bo\nzeta: true\n---\n# Sync\n"
	if !strings.HasPrefix(got, want) {
		t.Errorf("frontmatter:\n%s\nwant prefix:\n%s", got, want)
	}
	if !strings.HasSuffix(got, meeting[strings.Index(meeting, "# Sync"):]) {
		t.Error("body changed")
	}
	if fm := note.Parse("Sync.md", []byte(got)).Frontmatter; fm.Date != "2026-10-17" || len(fm.Project) != 1 || fm.Project[0] != "[[Hub]]" {
		t.Errorf("reparsed frontmatter = %+v", fm)
	}

	r = Validate(v, op("Other.md", map[string]any{"type": "note"}), Options{})
	requireOK(t, r)
	if got := string(change(t, r, "Other.md").After); got != "---\ntype: note\n---\nplain text\n" {
		t.Errorf("added frontmatter = %q", got)
	}

	r = Validate(v, op("Sync.md", map[string]any{"project": "[[Nowhere]]"}), Options{})
	e := requireErr(t, r.Errors, "fm", "broken link [[Nowhere]]")
	if e.Line != 4 {
		t.Errorf("error line = %d", e.Line)
	}

	r = Validate(v, op("Sync.md", map[string]any{}), Options{})
	requireErr(t, r.Errors, "fm", "no fields")
}

func TestAddLinks(t *testing.T) {
	v := small(map[string]string{
		"Hub.md":   "---\ntype: project\n---\n",
		"Loose.md": "- an idea",
	})
	p := &Plan{Version: Version, Operations: []Operation{{
		ID: "ln", Op: AddLinks, Category: CategoryLinks, Tier: TierAuto, Status: Approved,
		Note: "Loose.md", Links: []string{"Hub", "[[Gone]]"},
	}}}
	r := Validate(v, p, Options{})
	requireErr(t, r.Errors, "ln", "link target")
	p.Operations[0].Links = []string{"Hub", "Nowhere"}
	r = Validate(v, p, Options{})
	requireErr(t, r.Errors, "ln", "broken link [[Nowhere]]")
	p.Operations[0].Links = []string{"Hub"}
	r = Validate(v, p, Options{})
	requireOK(t, r)
	if got := string(change(t, r, "Loose.md").After); got != "- an idea\n- [[Hub]]\n" {
		t.Errorf("after = %q", got)
	}
}

func TestCreateMustNotInvent(t *testing.T) {
	v := smallVault()
	p := &Plan{Version: Version, Operations: []Operation{{
		ID: "new", Op: Create, Category: CategoryEntity, Tier: TierApproveEach, Status: Approved,
		Path: "people/Bo.md", Content: "---\ntype: person\n---\n# Bo\n\n- works on [[Hub]]\n",
	}}}
	requireErr(t, Validate(v, p, Options{}).Errors, "new", "inventing content")
	p.Operations[0].Content = "---\ntype: person\n---\n# Bo\n\n- [[Hub]]\n"
	r := Validate(v, p, Options{})
	requireOK(t, r)
	if c := change(t, r, "people/Bo.md"); c.Before != nil || len(c.Ops) != 1 {
		t.Errorf("change = %+v", c)
	}
}

func TestOpOnCreatedNote(t *testing.T) {
	v := smallVault()
	create := Operation{
		ID: "new", Op: Create, Category: CategoryNote, Tier: TierApproveEach, Status: Approved,
		Path: "Fresh.md", Content: "# Fresh",
	}
	links := Operation{
		ID: "ln", Op: AddLinks, Category: CategoryLinks, Tier: TierAuto, Status: Approved,
		Note: "Fresh.md", Links: []string{"Hub"},
	}
	r := Validate(v, &Plan{Version: Version, Operations: []Operation{create, links}}, Options{})
	requireOK(t, r)
	if got := string(change(t, r, "Fresh.md").After); got != "# Fresh\n- [[Hub]]\n" {
		t.Errorf("after = %q", got)
	}
	create.Status = Rejected
	r = Validate(v, &Plan{Version: Version, Operations: []Operation{create, links}}, Options{})
	requireErr(t, r.Errors, "ln", "created by rejected operation new")
}

func TestErrorString(t *testing.T) {
	cases := map[string]*Error{
		"op a: x.md:3: boom": {Op: "a", Path: "x.md", Line: 3, Msg: "boom"},
		"op a: x.md: boom":   {Op: "a", Path: "x.md", Msg: "boom"},
		"op a: boom":         {Op: "a", Msg: "boom"},
		"boom":               {Msg: "boom"},
	}
	for want, e := range cases {
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	}
}
