package plan

import (
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// probeNote is the note from the final review's rewrite probe: frontmatter,
// a heading, prose, a code fence, an external link and a relabeled wikilink.
const probeNote = "---\ntype: meeting\naliases:\n  - P1\nrepos:\n  - harbor-ingest\n---\n" + // lines 1-7
	"# Probe\n" + // line 8
	"\n" +
	"Intro paragraph keep.\n" + // line 10
	"\n" +
	"```python\n" + // line 12
	"def f():\n" +
	"    if x:\n" + // line 14
	"        return 1\n" +
	"```\n" +
	"\n" +
	"- see [spec](https://example.com/spec-v1) and [[Ledgerline|the ledger]]\n" + // line 18
	"- [ ] do the thing\n" // line 19

func probeVault() *vault.Vault {
	return small(map[string]string{
		"Probe.md":      probeNote,
		"Ledgerline.md": "---\ntype: component\n---\n# Ledgerline\n",
		"Beacon.md":     "---\ntype: component\n---\n# Beacon\n",
	})
}

func probeRewrite(v *vault.Vault, tier Tier, content string) *Result {
	op := rewriteAll(v, "Probe.md", content)
	op.Tier = tier
	return Validate(v, &Plan{Version: Version, Operations: []Operation{op}}, Options{})
}

func TestRewriteSafety(t *testing.T) {
	v := probeVault()
	requireOK(t, probeRewrite(v, TierAuto, probeNote))

	fails := []struct {
		name    string
		content string
		substr  string
		path    string
		line    int
	}{
		{
			name:    "dropped frontmatter key",
			content: strings.Replace(probeNote, "repos:\n  - harbor-ingest\n", "", 1),
			substr:  "frontmatter is not kept byte for byte",
			path:    "Probe.md", line: 1,
		},
		{
			name:    "reformatted frontmatter",
			content: strings.Replace(probeNote, "aliases:\n  - P1\n", "aliases: [P1]\n", 1),
			substr:  "frontmatter is not kept byte for byte",
			path:    "Probe.md", line: 1,
		},
		{
			name:    "dropped frontmatter",
			content: probeNote[strings.Index(probeNote, "# Probe"):],
			substr:  "frontmatter is not kept byte for byte",
			path:    "Probe.md", line: 1,
		},
		{
			name:    "invented prose line",
			content: strings.Replace(probeNote, "Intro paragraph keep.\n", "Intro paragraph keep.\n\nThe team agreed to cancel Harbor and lay off Mira.\n", 1),
			substr:  "line maps to no original line; never-tier changes (inventing content) are not permitted",
			path:    "Probe.md", line: 12,
		},
		{
			name:    "invented link-only prose line",
			content: probeNote + "\nsee also [[Beacon]]\n",
			substr:  "inventing content",
			path:    "Probe.md", line: 21,
		},
		{
			name:    "changed URL at auto",
			content: strings.Replace(probeNote, "https://example.com/spec-v1", "https://evil.example/spec-v9", 1),
			substr:  "link [spec](https://example.com/spec-v1) is changed or removed",
			path:    "Probe.md", line: 18,
		},
		{
			name:    "changed wikilink target at auto",
			content: strings.Replace(probeNote, "[[Ledgerline|the ledger]]", "[[Beacon|the ledger]]", 1),
			substr:  "link [[Ledgerline|the ledger]] is changed or removed",
			path:    "Probe.md", line: 18,
		},
		{
			name:    "changed fragment at auto",
			content: strings.Replace(probeNote, "[[Ledgerline|the ledger]]", "[[Ledgerline#Ops|the ledger]]", 1),
			substr:  "is changed or removed",
			path:    "Probe.md", line: 18,
		},
		{
			name:    "unlinked at auto",
			content: strings.Replace(probeNote, "[[Ledgerline|the ledger]]", "the ledger", 1),
			substr:  "is changed or removed",
			path:    "Probe.md", line: 18,
		},
		{
			name:    "de-indented code line",
			content: strings.Replace(probeNote, "    if x:\n        return 1\n", "if x:\nreturn 1\n", 1),
			substr:  "code line not found",
			path:    "Probe.md", line: 14,
		},
		{
			name:    "uppercased code line",
			content: strings.Replace(probeNote, "def f():", "DEF F():", 1),
			substr:  "code line not found",
			path:    "Probe.md", line: 13,
		},
		{
			name:    "changed fence info string",
			content: strings.Replace(probeNote, "```python", "```js", 1),
			substr:  "code line not found",
			path:    "Probe.md", line: 12,
		},
		{
			name:    "invented code line",
			content: strings.Replace(probeNote, "        return 1\n", "        return 1\n    return 2\n", 1),
			substr:  "inventing content",
			path:    "Probe.md", line: 16,
		},
		{
			name:    "invented generated block",
			content: probeNote + "\n<!-- vault-curator:begin tl -->\n- harbor was cancelled\n<!-- vault-curator:end tl -->\n",
			substr:  "generated block tl at lines 21-23 is not in the original note",
			path:    "Probe.md", line: 21,
		},
		{
			name:    "invented mermaid diagram",
			content: probeNote + "\n```mermaid\nflowchart LR\n  harbor-->cancelled\n```\n",
			substr:  "mermaid diagram at lines 21-24 is not in the original note",
			path:    "Probe.md", line: 21,
		},
	}
	for _, tc := range fails {
		t.Run(tc.name, func(t *testing.T) {
			for _, tier := range []Tier{TierAuto, TierApproveEach} {
				if strings.Contains(tc.name, "at auto") && tier != TierAuto {
					continue
				}
				r := probeRewrite(v, tier, tc.content)
				e := requireErr(t, r.Errors, "rw", tc.substr)
				if e.Path != tc.path || e.Line != tc.line {
					t.Errorf("tier %s: error at %s:%d, want %s:%d (%v)", tier, e.Path, e.Line, tc.path, tc.line, e)
				}
				if r.Changes != nil {
					t.Error("a failed validation returned changes")
				}
			}
		})
	}

	// The probe's whole rewrite, every change at once.
	t.Run("probe rewrite", func(t *testing.T) {
		content := "# probe\n\nintro paragraph keep.\n\n```python\ndef f():\nif x:\nreturn 1\n```\n\n" +
			"The team agreed to cancel Harbor and lay off Mira.\n\n" +
			"- see [spec](https://evil.example/spec-v9) and [[Beacon|the ledger]]\n- [ ] DO THE THING\n"
		r := probeRewrite(v, TierAuto, content)
		for _, want := range []string{"frontmatter is not kept", "inventing content", "[spec](https://example.com/spec-v1) is changed", "[[Ledgerline|the ledger]] is changed", "code line not found"} {
			requireErr(t, r.Errors, "rw", want)
		}
	})

	passes := []struct {
		name    string
		tier    Tier
		content string
	}{
		{
			name:    "heading-only addition",
			tier:    TierAuto,
			content: strings.Replace(probeNote, "\n- see", "\n## Notes\n\n- see", 1) + "\n## Action items\n",
		},
		{
			name:    "prose line becomes a heading",
			tier:    TierAuto,
			content: strings.Replace(probeNote, "Intro paragraph keep.", "## Intro paragraph keep.", 1),
		},
		{
			name:    "link added around existing words",
			tier:    TierAuto,
			content: strings.Replace(probeNote, "Intro paragraph keep.", "Intro paragraph [[Beacon|keep]].", 1),
		},
		{
			name:    "link-only bullet added",
			tier:    TierAuto,
			content: probeNote + "- [[Beacon]]\n",
		},
		{
			name:    "trailing whitespace in code",
			tier:    TierAuto,
			content: strings.Replace(probeNote, "def f():\n", "def f():  \n", 1),
		},
		{
			name:    "link retargeted at approve-each",
			tier:    TierApproveEach,
			content: strings.Replace(probeNote, "[[Ledgerline|the ledger]]", "[[Beacon|the ledger]]", 1),
		},
		{
			name:    "url changed at approve-each",
			tier:    TierApproveEach,
			content: strings.Replace(probeNote, "spec-v1", "spec-v2", 1),
		},
		{
			name:    "wikilink target case changed",
			tier:    TierAuto,
			content: strings.Replace(probeNote, "[[Ledgerline|the ledger]]", "[[ledgerline|the ledger]]", 1),
		},
	}
	for _, tc := range passes {
		t.Run(tc.name, func(t *testing.T) {
			requireOK(t, probeRewrite(v, tc.tier, tc.content))
		})
	}
}

func TestRewriteAddsNoFrontmatter(t *testing.T) {
	v := small(map[string]string{"Bare.md": "# Bare\n\n- a point\n"})
	op := rewriteAll(v, "Bare.md", "---\ntype: meeting\n---\n# Bare\n\n- a point\n")
	r := Validate(v, &Plan{Version: Version, Operations: []Operation{op}}, Options{})
	e := requireErr(t, r.Errors, "rw", "frontmatter is not kept byte for byte")
	if e.Path != "Bare.md" || e.Line != 1 {
		t.Errorf("error at %s:%d", e.Path, e.Line)
	}
}

// Code fenced inside a bullet keeps its lines too, relative to the fence.
func TestRewriteKeepsCodeInBullets(t *testing.T) {
	const src = "- run this\n  ```sh\n  make test\n    --verbose\n  ```\n"
	v := small(map[string]string{"Cmd.md": src})
	run := func(content string) *Result {
		op := rewriteAll(v, "Cmd.md", content)
		return Validate(v, &Plan{Version: Version, Operations: []Operation{op}}, Options{})
	}
	requireOK(t, run(src))
	// Reindenting the bullet moves its code with it.
	requireOK(t, run("* run this\n    ```sh\n    make test\n      --verbose\n    ```\n"))
	requireErr(t, run("- run this\n  ```sh\n  make test\n  --verbose\n  ```\n").Errors, "rw", "not found")
	requireErr(t, run("- run this\n  ```sh\n  MAKE TEST\n    --verbose\n  ```\n").Errors, "rw", "not found")
}

func TestSetFrontmatterTiers(t *testing.T) {
	v := probeVault()
	run := func(tier Tier, fields map[string]any) *Result {
		return Validate(v, &Plan{Version: Version, Operations: []Operation{{
			ID: "fm", Op: SetFrontmatter, Category: CategoryFrontmatter, Tier: tier, Status: Approved,
			Note: "Beacon.md", Frontmatter: fields,
		}}}, Options{})
	}
	for _, k := range []string{"depends-on", "owned-by", "calls", "part-of", "related-to", "supersedes", "aliases"} {
		t.Run(k, func(t *testing.T) {
			fields := map[string]any{k: []any{"[[Ledgerline]]"}}
			if k == "aliases" {
				fields[k] = []any{"Bcn"}
			}
			e := requireErr(t, run(TierAuto, fields).Errors, "fm", "frontmatter field "+k+" requires tier approve-each")
			if e.Path != "" || e.Line != 0 {
				t.Errorf("error at %s:%d", e.Path, e.Line)
			}
			// Removing the field is a change to it too.
			requireErr(t, run(TierAuto, map[string]any{k: nil}).Errors, "fm", "requires tier approve-each")
			requireOK(t, run(TierApproveEach, fields))
		})
	}
	requireOK(t, run(TierAuto, map[string]any{"repos": []any{"beacon"}}))
}
