package flow

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// The markers around the generated part of docs/design.md's "Agent flow".
const (
	BeginMarker = "<!-- vault-curator:begin agent-flow -->"
	EndMarker   = "<!-- vault-curator:end agent-flow -->"
)

func mermaidID(s State) string { return strings.ReplaceAll(string(s), ".", "_") }

// Mermaid renders the part of the model that touches one skill as a Mermaid
// stateDiagram-v2: the skill's transitions, the transitions into it from
// elsewhere, and its exits.
func (m *Model) Mermaid(sk Skill) string {
	type pair struct{ from, to string }
	var order []pair
	labels := map[pair][]string{}
	var declared []State
	declare := func(s State) {
		if !slices.Contains(declared, s) {
			declared = append(declared, s)
		}
	}
	var exits []Exit
	var terminals []State
	for _, t := range m.Transitions {
		fromIn := t.From.Skill() == sk
		toIn := t.Exit == "" && t.To.Skill() == sk
		if !fromIn && !toIn {
			continue
		}
		var p pair
		if t.From == m.Start {
			p.from = "[*]"
		} else {
			declare(t.From)
			p.from = mermaidID(t.From)
		}
		if t.Exit != "" {
			p.to = "exit_" + string(t.Exit)
			if !slices.Contains(exits, t.Exit) {
				exits = append(exits, t.Exit)
			}
		} else {
			declare(t.To)
			p.to = mermaidID(t.To)
			if s := m.State(t.To); s != nil && s.Kind == Terminal && !slices.Contains(terminals, t.To) {
				terminals = append(terminals, t.To)
			}
		}
		if _, ok := labels[p]; !ok {
			order = append(order, p)
		}
		labels[p] = append(labels[p], string(t.On))
	}
	var b strings.Builder
	b.WriteString("stateDiagram-v2\n")
	for _, s := range declared {
		fmt.Fprintf(&b, "    state \"%s\" as %s\n", s, mermaidID(s))
	}
	for _, e := range exits {
		fmt.Fprintf(&b, "    state \"return to caller (%s)\" as exit_%s\n", e, e)
	}
	for _, p := range order {
		fmt.Fprintf(&b, "    %s --> %s : %s\n", p.from, p.to, strings.Join(labels[p], ", "))
	}
	for _, s := range terminals {
		fmt.Fprintf(&b, "    %s --> [*]\n", mermaidID(s))
	}
	return b.String()
}

// DocBlock renders the generated part of docs/design.md's "Agent flow",
// markers included.
func (m *Model) DocBlock() string {
	var b strings.Builder
	b.WriteString(BeginMarker + "\n")
	b.WriteString("<!-- Generated from internal/vaultcurator/flow. After changing the model, run: go test ./internal/vaultcurator/flow -run TestDesignDoc -update -->\n\n")

	steps, terminals := 0, 0
	for _, s := range m.States {
		switch s.Kind {
		case Step:
			steps++
		case Terminal:
			terminals++
		}
	}
	fmt.Fprintf(&b, "### States\n\n%d states: the start, %d steps and %d terminals. **Vault** is the machine-checked part of the invariant.\n\n", len(m.States), steps, terminals)
	b.WriteString("| State | Vault | Invariant |\n| --- | --- | --- |\n")
	for _, s := range m.States {
		name := "`" + string(s.Name) + "`"
		if s.Kind == Terminal {
			name = "**" + name + "** (terminal)"
		}
		inv := s.Invariant
		if s.Progress != "" {
			inv += " Progress: " + s.Progress + "."
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", name, s.Vault, inv)
	}

	fmt.Fprintf(&b, "\n### Inputs\n\n%d inputs.\n\n| Input | Kind | Observed as |\n| --- | --- | --- |\n", len(m.Inputs))
	for _, d := range m.Inputs {
		kind := "outcome"
		if d.User {
			kind = "user answer"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", d.Name, kind, d.Means)
	}

	b.WriteString("\n### Exits\n\nThe review-to-merge tail (`curate.validate_draft` to `curate.watermark`) is shared. It ends at an exit, and the agent continues where the skill that handed off to it says.\n\n")
	callers := m.callers()
	b.WriteString("| Exit |")
	for _, c := range callers {
		fmt.Fprintf(&b, " from %s |", c)
	}
	b.WriteString("\n| --- |" + strings.Repeat(" --- |", len(callers)) + "\n")
	for _, e := range m.exits() {
		fmt.Fprintf(&b, "| `%s` |", e)
		for _, c := range callers {
			fmt.Fprintf(&b, " `%s` |", m.Returns[c][e])
		}
		b.WriteString("\n")
	}

	b.WriteString("\n### Diagrams\n\nOne diagram per skill, with the transitions into it from other skills.\n")
	for _, sk := range m.Skills() {
		fmt.Fprintf(&b, "\n#### %s\n\n```mermaid\n%s```\n", sk, m.Mermaid(sk))
	}

	fmt.Fprintf(&b, "\n### Transitions\n\n<details>\n<summary>All %d transitions</summary>\n\n| State | On | Next | Effect |\n| --- | --- | --- | --- |\n", len(m.Transitions))
	for _, t := range m.Transitions {
		next := "`" + string(t.To) + "`"
		if t.Exit != "" {
			next = "exit `" + string(t.Exit) + "`"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s |\n", t.From, t.On, next, t.Effect)
	}
	b.WriteString("\n</details>\n")
	b.WriteString(EndMarker)
	return b.String()
}

// callers lists the skills that have returns, in skill order.
func (m *Model) callers() []Skill {
	var out []Skill
	for _, sk := range m.Skills() {
		if _, ok := m.Returns[sk]; ok {
			out = append(out, sk)
		}
	}
	return out
}

// exits lists the exits the transitions use, in first-seen order.
func (m *Model) exits() []Exit {
	var out []Exit
	for _, t := range m.Transitions {
		if t.Exit != "" && !slices.Contains(out, t.Exit) {
			out = append(out, t.Exit)
		}
	}
	return out
}

// ReplaceBlock returns doc with the text between (and including) the agent
// flow markers replaced by block.
func ReplaceBlock(doc, block string) (string, error) {
	i := strings.Index(doc, BeginMarker)
	j := strings.Index(doc, EndMarker)
	if i < 0 || j < i {
		return "", errors.New("agent flow markers not found")
	}
	return doc[:i] + block + doc[j+len(EndMarker):], nil
}
