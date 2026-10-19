package flow

import (
	"slices"
	"strings"
	"testing"
)

func TestVaultCuratorHoldsEveryProperty(t *testing.T) {
	m := VaultCurator()
	for _, p := range Properties {
		if err := p.Check(m); err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}
	}
}

// retarget points the transition (from, on) somewhere else.
func retarget(t *testing.T, m *Model, from State, on Input, tr Transition) {
	t.Helper()
	for i, x := range m.Transitions {
		if x.From == from && x.On == on {
			tr.From, tr.On = from, on
			m.Transitions[i] = tr
			return
		}
	}
	t.Fatalf("no transition (%s, %s)", from, on)
}

func remove(t *testing.T, m *Model, from State, on Input) {
	t.Helper()
	n := len(m.Transitions)
	m.Transitions = slices.DeleteFunc(m.Transitions, func(x Transition) bool { return x.From == from && x.On == on })
	if len(m.Transitions) == n {
		t.Fatalf("no transition (%s, %s)", from, on)
	}
}

// Each property must reject a model broken in the way it guards against.
func TestPropertiesCatchBrokenModels(t *testing.T) {
	cases := []struct {
		name     string
		property func(*Model) error
		breakIt  func(*testing.T, *Model)
		want     string
	}{
		{"well-formed: transition to an unknown state", WellFormed, func(t *testing.T, m *Model) {
			retarget(t, m, "curate.scan", OK, Transition{To: "curate.plna"})
		}, "curate.plna"},
		{"well-formed: input outside the alphabet", WellFormed, func(t *testing.T, m *Model) {
			m.State("curate.scan").Inputs = append(m.State("curate.scan").Inputs, "scan_crashed")
		}, "scan_crashed"},
		{"deterministic: two moves on one input", Deterministic, func(t *testing.T, m *Model) {
			m.Transitions = append(m.Transitions, Transition{From: "curate.scan", On: OK, To: "curate.review"})
		}, "curate.scan"},
		{"complete: merge conflict has no move (finding 6)", Complete, func(t *testing.T, m *Model) {
			remove(t, m, "curate.merge", MergeConflict)
		}, "merge_conflict"},
		{"complete: validation failing after the decisions has no move (finding 7)", Complete, func(t *testing.T, m *Model) {
			remove(t, m, "curate.apply", InvalidPlan)
		}, "invalid_plan"},
		{"complete: a move on an input the state cannot observe", Complete, func(t *testing.T, m *Model) {
			m.Transitions = append(m.Transitions, Transition{From: "curate.plan", On: NoCandidates, To: "done"})
		}, "curate.plan"},
		{"complete: an exit the caller does not handle", Complete, func(t *testing.T, m *Model) {
			delete(m.Returns["backfill"], ExitReplan)
		}, "replan"},
		{"reachable: a state no path leads to", Reachable, func(t *testing.T, m *Model) {
			m.States = append(m.States, StateDef{Name: "curate.orphan", Invariant: "x", Inputs: in(OK)})
			m.Transitions = append(m.Transitions, Transition{From: "curate.orphan", On: OK, To: "done"})
		}, "curate.orphan"},
		{"co-reachable: abort loops on itself", CoReachable, func(t *testing.T, m *Model) {
			retarget(t, m, "curate.abort_merge", OK, Transition{To: "curate.abort_merge"})
		}, "curate.abort_merge"},
		{"co-reachable: a terminal with a move", CoReachable, func(t *testing.T, m *Model) {
			m.Transitions = append(m.Transitions, Transition{From: "done", On: OK, To: "curate.scan"})
		}, "done"},
		{"clean: a terminal with a merge in progress", CleanTerminals, func(t *testing.T, m *Model) {
			m.States = append(m.States, StateDef{Name: "stopped_conflicted", Kind: Terminal, Invariant: "x", Vault: Vault{Branch: BranchUnmerged, Merging: true}})
			retarget(t, m, "curate.abort_merge", OK, Transition{To: "stopped_conflicted"})
		}, "merge in progress"},
		{"clean: a conflict that stops without aborting", CleanTerminals, func(t *testing.T, m *Model) {
			retarget(t, m, "curate.merge", MergeConflict, Transition{To: "stopped_clean", Effect: EffectConflict})
		}, "stopped_clean"},
		{"clean: a branch left with no way to resume it", CleanTerminals, func(t *testing.T, m *Model) {
			remove(t, m, "curate.leftover", Yes)
			m.State("curate.leftover").Inputs = in(OK, No)
		}, "resume"},
		{"bounded loops: the draft fix loop runs without the user", BoundedLoops, func(t *testing.T, m *Model) {
			retarget(t, m, "curate.validate_draft", InvalidPlan, Transition{To: "curate.plan"})
		}, "curate.plan"},
		{"bounded loops: a dirty tree retried without the user", BoundedLoops, func(t *testing.T, m *Model) {
			retarget(t, m, "curate.merge", MergeDirtyTree, Transition{To: "curate.merge"})
		}, "curate.merge"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := VaultCurator().clone()
			c.breakIt(t, m)
			err := c.property(m)
			if err == nil {
				t.Fatal("broken model passed")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

func TestProgressStateBreaksALoop(t *testing.T) {
	m := VaultCurator().clone()
	retarget(t, m, "curate.validate_draft", InvalidPlan, Transition{To: "curate.plan"})
	m.State("curate.plan").Progress = "fix rounds left"
	if err := BoundedLoops(m); err != nil {
		t.Fatalf("a cycle through a progress state was rejected: %v", err)
	}
}

func TestCloneIsDeep(t *testing.T) {
	m := VaultCurator()
	c := m.clone()
	c.State("curate.scan").Inputs[0] = "x"
	c.Returns["curate"][ExitMerged] = "x"
	c.Transitions[0].To = "x"
	if m.State("curate.scan").Inputs[0] == "x" || m.Returns["curate"][ExitMerged] == "x" || m.Transitions[0].To == "x" {
		t.Fatal("clone shares state with the original")
	}
}

// Finding 8: backfill decided "seed entities" from watermark.exists, so a
// first batch that merged nothing came back to seeding forever. That loop
// passes through the user's review answers, so BoundedLoops allows it; this
// test pins that seeding runs at most once per backfill.
func TestBackfillSeedsAtMostOnce(t *testing.T) {
	if OnCycle(VaultCurator(), "backfill.seed") {
		t.Fatal("backfill.seed is on a cycle")
	}
	m := VaultCurator().clone()
	m.Returns["backfill"][ExitNothing] = "backfill.start"
	if !OnCycle(m, "backfill.seed") {
		t.Fatal("returning to backfill.start after an empty seed batch should put backfill.seed on a cycle")
	}
}

func TestTerminals(t *testing.T) {
	// The terminal states each skill run can end in, so a refactor that drops
	// one (for example the declined merge) shows up here.
	want := []State{"done", "stopped_clean", "parked", "stopped_merged"}
	var got []State
	for _, s := range VaultCurator().States {
		if s.Kind == Terminal {
			got = append(got, s.Name)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("terminals = %v, want %v", got, want)
	}
}
