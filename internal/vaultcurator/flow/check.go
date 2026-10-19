package flow

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Property is one machine-checked property of a model.
type Property struct {
	Name  string
	Check func(*Model) error
}

// Properties are checked by the tests and listed in docs/design.md.
var Properties = []Property{
	{"Well-formed", WellFormed},
	{"Deterministic", Deterministic},
	{"Complete", Complete},
	{"Reachable", Reachable},
	{"Co-reachable", CoReachable},
	{"Clean terminals", CleanTerminals},
	{"Bounded loops", BoundedLoops},
}

// Check runs every property and joins their errors.
func Check(m *Model) error {
	var errs []error
	for _, p := range Properties {
		if err := p.Check(m); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.Name, err))
		}
	}
	return errors.Join(errs...)
}

// WellFormed checks that the model's names line up.
func WellFormed(m *Model) error {
	var errs []error
	alphabet := map[Input]bool{}
	for _, d := range m.Inputs {
		alphabet[d.Name] = true
	}
	seen := map[State]bool{}
	for _, s := range m.States {
		if seen[s.Name] {
			errs = append(errs, fmt.Errorf("state %s is declared twice", s.Name))
		}
		seen[s.Name] = true
		if s.Invariant == "" {
			errs = append(errs, fmt.Errorf("state %s has no invariant", s.Name))
		}
		if (s.Kind == Step) != (s.Name.Skill() != "") {
			errs = append(errs, fmt.Errorf("state %s: steps, and only steps, have a skill namespace", s.Name))
		}
		for _, i := range s.Inputs {
			if !alphabet[i] {
				errs = append(errs, fmt.Errorf("state %s lists input %s, which is not in the alphabet", s.Name, i))
			}
		}
	}
	if s := m.State(m.Start); s == nil || s.Kind != Start {
		errs = append(errs, fmt.Errorf("start state %s is not declared as the start", m.Start))
	}
	used := map[Input]bool{}
	for _, t := range m.Transitions {
		used[t.On] = true
		if m.State(t.From) == nil {
			errs = append(errs, fmt.Errorf("transition (%s, %s) leaves an unknown state", t.From, t.On))
		}
		if !alphabet[t.On] {
			errs = append(errs, fmt.Errorf("transition (%s, %s) uses an input not in the alphabet", t.From, t.On))
		}
		switch {
		case (t.To == "") == (t.Exit == ""):
			errs = append(errs, fmt.Errorf("transition (%s, %s) needs exactly one of a next state or an exit", t.From, t.On))
		case t.To != "" && m.State(t.To) == nil:
			errs = append(errs, fmt.Errorf("transition (%s, %s) goes to unknown state %s", t.From, t.On, t.To))
		}
	}
	for _, d := range m.Inputs {
		if !used[d.Name] {
			errs = append(errs, fmt.Errorf("input %s is never used", d.Name))
		}
	}
	for s := range m.Entries {
		if m.State(s) == nil {
			errs = append(errs, fmt.Errorf("entry %s is not a state", s))
		}
	}
	for sk, rs := range m.Returns {
		for e, s := range rs {
			if m.State(s) == nil {
				errs = append(errs, fmt.Errorf("return %s/%s goes to unknown state %s", sk, e, s))
			}
		}
	}
	return errors.Join(errs...)
}

// Deterministic checks that each (state, input) has at most one transition.
func Deterministic(m *Model) error {
	var errs []error
	seen := map[[2]string]bool{}
	for _, t := range m.Transitions {
		k := [2]string{string(t.From), string(t.On)}
		if seen[k] {
			errs = append(errs, fmt.Errorf("state %s has more than one transition on %s", t.From, t.On))
		}
		seen[k] = true
	}
	return errors.Join(errs...)
}

// Complete checks that every observable input has a move, and every exit a
// return.
func Complete(m *Model) error {
	var errs []error
	has := map[[2]string]bool{}
	for _, t := range m.Transitions {
		has[[2]string{string(t.From), string(t.On)}] = true
		if s := m.State(t.From); s != nil && !slices.Contains(s.Inputs, t.On) {
			errs = append(errs, fmt.Errorf("state %s has a transition on %s, which it does not list as possible", t.From, t.On))
		}
	}
	for _, s := range m.States {
		for _, i := range s.Inputs {
			if !has[[2]string{string(s.Name), string(i)}] {
				errs = append(errs, fmt.Errorf("state %s has no transition on %s", s.Name, i))
			}
		}
	}
	errs = append(errs, m.expand().dangling...)
	return errors.Join(errs...)
}

// Reachable checks that every state is reachable from the start.
func Reachable(m *Model) error {
	g := m.expand()
	reached := map[State]bool{}
	for _, n := range g.nodes {
		reached[n.State] = true
	}
	var errs []error
	for _, s := range m.States {
		if !reached[s.Name] {
			errs = append(errs, fmt.Errorf("state %s is not reachable from %s", s.Name, m.Start))
		}
	}
	return errors.Join(errs...)
}

// CoReachable checks that terminal states have no moves and that every
// reachable state reaches a terminal.
func CoReachable(m *Model) error {
	var errs []error
	for _, t := range m.Transitions {
		if s := m.State(t.From); s != nil && s.Kind == Terminal {
			errs = append(errs, fmt.Errorf("terminal state %s has a transition on %s", t.From, t.On))
		}
	}
	for _, s := range m.States {
		if s.Kind == Terminal && len(s.Inputs) > 0 {
			errs = append(errs, fmt.Errorf("terminal state %s lists inputs", s.Name))
		}
	}
	g := m.expand()
	reverse := map[node][]node{}
	var queue []node
	ok := map[node]bool{}
	for _, n := range g.nodes {
		for _, e := range g.edges[n] {
			reverse[e.to] = append(reverse[e.to], n)
		}
		if s := m.State(n.State); s != nil && s.Kind == Terminal {
			ok[n] = true
			queue = append(queue, n)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, p := range reverse[n] {
			if !ok[p] {
				ok[p] = true
				queue = append(queue, p)
			}
		}
	}
	for _, n := range g.nodes {
		if !ok[n] {
			errs = append(errs, fmt.Errorf("dead end: %s reaches no terminal state", n))
		}
	}
	return errors.Join(errs...)
}

// CleanTerminals checks that the declared vault conditions follow from the
// transitions' effects, and that no terminal leaves a half-applied state.
func CleanTerminals(m *Model) error {
	var errs []error
	g := m.expand()
	for _, n := range g.nodes {
		from := m.State(n.State)
		for _, e := range g.edges[n] {
			to := m.State(e.to.State)
			if from == nil || to == nil {
				continue
			}
			got, ok := e.effect.apply(from.Vault)
			switch {
			case !ok:
				errs = append(errs, fmt.Errorf("transition (%s, %s): effect %q cannot happen with %s", n.State, e.on, e.effect, from.Vault))
			case got != to.Vault:
				errs = append(errs, fmt.Errorf("transition (%s, %s) leaves %s, but %s declares %s", n.State, e.on, got, e.to.State, to.Vault))
			}
		}
	}
	resume := false
	for _, t := range m.Transitions {
		if t.From == m.ResumeFrom && t.On == m.ResumeOn && t.To != "" {
			if s := m.State(t.To); s != nil && s.Vault.Branch != BranchNone {
				resume = true
			}
		}
	}
	for _, s := range m.States {
		if s.Kind != Terminal {
			continue
		}
		if s.Vault.Merging {
			errs = append(errs, fmt.Errorf("terminal %s ends with a merge in progress", s.Name))
		}
		if s.Vault.Branch != BranchNone && !resume {
			errs = append(errs, fmt.Errorf("terminal %s leaves a curation branch, but no resume transition (%s, %s) takes it over", s.Name, m.ResumeFrom, m.ResumeOn))
		}
	}
	return errors.Join(errs...)
}

// BoundedLoops checks that every cycle passes through a user answer or a
// progress state.
func BoundedLoops(m *Model) error {
	g := m.expand()
	progress := func(s State) bool { d := m.State(s); return d != nil && d.Progress != "" }
	user := map[Input]bool{}
	for _, d := range m.Inputs {
		user[d.Name] = d.User
	}
	if c := g.cycle(func(e edge) bool { return !user[e.on] && !progress(e.to.State) }); c != nil {
		return fmt.Errorf("cycle with no user answer and no progress state: %s", formatCycle(c))
	}
	return nil
}

// OnCycle reports whether state s lies on any cycle of the expanded DFA.
func OnCycle(m *Model, s State) bool {
	g := m.expand()
	for _, n := range g.nodes {
		if n.State != s {
			continue
		}
		seen := map[node]bool{}
		stack := []node{}
		for _, e := range g.edges[n] {
			stack = append(stack, e.to)
		}
		for len(stack) > 0 {
			x := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if x.State == s {
				return true
			}
			if seen[x] {
				continue
			}
			seen[x] = true
			for _, e := range g.edges[x] {
				stack = append(stack, e.to)
			}
		}
	}
	return false
}

func formatCycle(c []node) string {
	parts := make([]string, len(c))
	for i, n := range c {
		parts[i] = n.String()
	}
	return strings.Join(parts, " -> ")
}

// node is a state of the expanded DFA: a model state plus the skill that
// will receive the tail's exits.
type node struct {
	State  State
	Caller Skill
}

func (n node) String() string {
	if n.Caller == "" || n.Caller == n.State.Skill() {
		return string(n.State)
	}
	return fmt.Sprintf("%s (for %s)", n.State, n.Caller)
}

type edge struct {
	on     Input
	to     node
	effect Effect
}

type graph struct {
	nodes    []node // reachable from the start, in discovery order
	edges    map[node][]edge
	dangling []error // exits with no return for their caller
}

// expand builds the reachable part of the DFA the properties check. Its
// states are (model state, caller) pairs: entering a skill's entry state sets
// the caller to that skill, every other transition keeps it, and an exit goes
// to Returns[caller][exit]. The caller is the call stack of a depth-one
// call, so the expansion is finite and a plain DFA.
func (m *Model) expand() *graph {
	g := &graph{edges: map[node][]edge{}}
	start := node{State: m.Start}
	seen := map[node]bool{start: true}
	queue := []node{start}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		g.nodes = append(g.nodes, n)
		for _, t := range m.Transitions {
			if t.From != n.State {
				continue
			}
			to := node{State: t.To, Caller: n.Caller}
			if t.Exit != "" {
				s, ok := m.Returns[n.Caller][t.Exit]
				if !ok {
					g.dangling = append(g.dangling, fmt.Errorf("exit %s from %s has no return for caller %q", t.Exit, n.State, n.Caller))
					continue
				}
				to.State = s
			}
			if m.State(to.State) == nil {
				continue
			}
			if sk, ok := m.Entries[to.State]; ok {
				to.Caller = sk
			}
			g.edges[n] = append(g.edges[n], edge{on: t.On, to: to, effect: t.Effect})
			if !seen[to] {
				seen[to] = true
				queue = append(queue, to)
			}
		}
	}
	return g
}

// cycle returns one cycle that uses only edges keep accepts, or nil.
func (g *graph) cycle(keep func(edge) bool) []node {
	const (
		white = iota
		grey
		black
	)
	color := map[node]int{}
	var path []node
	var found []node
	var visit func(n node) bool
	visit = func(n node) bool {
		color[n] = grey
		path = append(path, n)
		for _, e := range g.edges[n] {
			if !keep(e) {
				continue
			}
			switch color[e.to] {
			case grey:
				i := slices.Index(path, e.to)
				found = append(slices.Clone(path[i:]), e.to)
				return true
			case white:
				if visit(e.to) {
					return true
				}
			}
		}
		path = path[:len(path)-1]
		color[n] = black
		return false
	}
	for _, n := range g.nodes {
		if color[n] == white && visit(n) {
			return found
		}
	}
	return nil
}
