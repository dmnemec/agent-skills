// Package flow models every path an agent can take through the vault-curator
// skills (setup, curate, entities, backfill) as one deterministic finite
// automaton, and checks it for dead ends, traps and unbounded loops. The
// skills' step headings name the model's states; docs/design.md ("Agent flow")
// holds the diagrams generated from it.
package flow

import (
	"maps"
	"slices"
	"strings"
)

// State names one step of a skill ("curate.merge"), a terminal ("done"), or
// the start state. A step's namespace, before the dot, is its skill.
type State string

// Skill returns the state's namespace: the skill whose SKILL.md holds its
// step, or "" for the start and terminal states.
func (s State) Skill() Skill {
	if i := strings.IndexByte(string(s), '.'); i > 0 {
		return Skill(s[:i])
	}
	return ""
}

// Input is an outcome the agent observes in a state: a command's result or a
// user answer.
type Input string

// Exit names a way the shared review-to-merge tail ends. It returns to the
// skill that handed off to the tail, at the state Model.Returns gives.
type Exit string

// Skill is a skill's folder name under skills/vault-curator. It is also the
// calling context the DFA tracks for exits.
type Skill string

// Kind says what a state is.
type Kind int

const (
	// Step is a state some skill step heading names.
	Step Kind = iota
	// Terminal ends the run. It has no inputs and no transitions.
	Terminal
	// Start is the single state before any skill runs; the user's request
	// picks the skill.
	Start
)

// Branch is the condition of the curation branch (and its worktree) of the
// current pass.
type Branch int

const (
	// BranchNone: no curation branch or worktree of this pass exists.
	BranchNone Branch = iota
	// BranchUnmerged: apply made a curate/* branch and its worktree; the
	// user's branch does not contain it.
	BranchUnmerged
	// BranchMerged: the branch is merged into the user's branch, and the
	// watermark has not moved yet.
	BranchMerged
)

func (b Branch) String() string {
	switch b {
	case BranchUnmerged:
		return "branch unmerged"
	case BranchMerged:
		return "branch merged"
	default:
		return "no branch"
	}
}

// Vault is the part of the vault's condition the properties reason about.
type Vault struct {
	Branch Branch
	// Merging means a merge into the user's branch stopped on conflicts and
	// is still in progress: a partial write to the user's branch.
	Merging bool
}

func (v Vault) String() string {
	if v.Merging {
		return v.Branch.String() + ", merge in progress"
	}
	return v.Branch.String()
}

// Effect is the change a transition makes to the Vault condition.
type Effect int

const (
	// EffectNone leaves the condition as it is.
	EffectNone Effect = iota
	// EffectBranch: a curation branch now exists (apply made it, or the
	// agent took over an earlier run's branch).
	EffectBranch
	// EffectMerge: the branch merged cleanly into the user's branch.
	EffectMerge
	// EffectConflict: the merge stopped on conflicts.
	EffectConflict
	// EffectDiscard: the merge is aborted and the branch and its worktree
	// are removed.
	EffectDiscard
	// EffectWatermark: watermark moved the tag and removed the branch and
	// its worktree.
	EffectWatermark
)

// apply returns the condition after the effect, or false when the effect
// cannot happen from v.
func (e Effect) apply(v Vault) (Vault, bool) {
	switch e {
	case EffectNone:
		return v, true
	case EffectBranch:
		return Vault{Branch: BranchUnmerged}, v == Vault{}
	case EffectMerge:
		return Vault{Branch: BranchMerged}, v == Vault{Branch: BranchUnmerged}
	case EffectConflict:
		return Vault{Branch: BranchUnmerged, Merging: true}, v == Vault{Branch: BranchUnmerged}
	case EffectDiscard:
		return Vault{}, v == Vault{Branch: BranchUnmerged, Merging: true}
	case EffectWatermark:
		return Vault{}, v == Vault{Branch: BranchMerged}
	}
	return v, false
}

func (e Effect) String() string {
	return [...]string{"", "branch created", "merged", "merge conflicted", "merge aborted, branch removed", "watermarked, branch removed"}[e]
}

// StateDef is one state of the model.
type StateDef struct {
	Name State
	Kind Kind
	// Invariant is what holds in the vault whenever the agent is in this
	// state, in one line.
	Invariant string
	// Vault is the machine-checked part of Invariant.
	Vault Vault
	// Inputs are the outcomes the agent can observe here. Every one needs a
	// transition.
	Inputs []Input
	// Progress, when set, makes this a progress state and names the measure
	// that strictly decreases on every visit (see BoundedLoops).
	Progress string
}

// InputDef is one letter of the input alphabet.
type InputDef struct {
	Name Input
	// User marks a user answer, as opposed to a command outcome.
	User bool
	// Means says how the agent observes it.
	Means string
}

// Transition moves the agent from From on input On, either to the state To
// or, when Exit is set, back to the calling skill.
type Transition struct {
	From   State
	On     Input
	To     State
	Exit   Exit
	Effect Effect
}

// Model is the agent flow: a DFA over states written once, expanded over the
// calling skill (see expand) wherever a skill hands off to another and gets
// control back.
type Model struct {
	Start       State
	States      []StateDef
	Inputs      []InputDef
	Transitions []Transition
	// Entries maps each skill's first step to its skill. Entering it sets the
	// calling context; every other transition keeps it.
	Entries map[State]Skill
	// Returns maps a calling context and an exit to the state the agent
	// continues at.
	Returns map[Skill]map[Exit]State
	// ResumeFrom and ResumeOn name the transition by which a later run takes
	// over a curation branch a terminal state left behind.
	ResumeFrom State
	ResumeOn   Input
}

// State returns the definition of s, or nil.
func (m *Model) State(s State) *StateDef {
	for i := range m.States {
		if m.States[i].Name == s {
			return &m.States[i]
		}
	}
	return nil
}

// Skills lists the namespaces of the model's steps, in first-seen order.
func (m *Model) Skills() []Skill {
	var out []Skill
	for _, s := range m.States {
		if sk := s.Name.Skill(); sk != "" && !slices.Contains(out, sk) {
			out = append(out, sk)
		}
	}
	return out
}

func (m *Model) clone() *Model {
	c := *m
	c.States = make([]StateDef, len(m.States))
	for i, s := range m.States {
		s.Inputs = slices.Clone(s.Inputs)
		c.States[i] = s
	}
	c.Inputs = slices.Clone(m.Inputs)
	c.Transitions = slices.Clone(m.Transitions)
	c.Entries = maps.Clone(m.Entries)
	c.Returns = map[Skill]map[Exit]State{}
	for k, v := range m.Returns {
		c.Returns[k] = maps.Clone(v)
	}
	return &c
}

// The input alphabet.
const (
	OK               Input = "ok"
	InvokeSetup      Input = "setup"
	InvokeCurate     Input = "curate"
	InvokeEntities   Input = "entities"
	InvokeBackfill   Input = "backfill"
	Yes              Input = "yes"
	No               Input = "no"
	DecisionsGiven   Input = "decisions_given"
	GoMissing        Input = "go_missing"
	NoVault          Input = "no_vault"
	BinaryMissing    Input = "binary_missing"
	NoCandidates     Input = "no_candidates"
	NoWatermark      Input = "no_watermark"
	AllSkipped       Input = "all_skipped"
	InvalidPlan      Input = "invalid_plan"
	SyncRefused      Input = "sync_refused"
	NoneApproved     Input = "none_approved"
	ApplyRefused     Input = "apply_refused"
	MergeDirtyTree   Input = "merge_dirty_tree"
	MergeConflict    Input = "merge_conflict"
	WatermarkRefused Input = "watermark_refused"
)

// The tail's exits.
const (
	ExitMerged  Exit = "merged"
	ExitNothing Exit = "nothing"
	ExitReplan  Exit = "replan"
)

func in(inputs ...Input) []Input { return inputs }

// VaultCurator returns the agent flow of the vault-curator skills.
func VaultCurator() *Model {
	none := Vault{}
	open := Vault{Branch: BranchUnmerged}
	merged := Vault{Branch: BranchMerged}
	conflicted := Vault{Branch: BranchUnmerged, Merging: true}

	return &Model{
		Start: "start",
		Inputs: []InputDef{
			{OK, false, "The step's commands exited 0 and its Done-when criterion holds."},
			{InvokeSetup, true, "The user asks to install or set up vault-curator."},
			{InvokeCurate, true, "The user asks to curate (or picks curate when setup offers it)."},
			{InvokeEntities, true, "The user asks to seed, add, alias or merge entities."},
			{InvokeBackfill, true, "The user asks to backfill (or picks backfill when setup offers it)."},
			{Yes, true, "The user answers yes to the step's question."},
			{No, true, "The user answers no, declines, or asks to stop."},
			{DecisionsGiven, true, "The user has answered every category and every approve-each operation."},
			{GoMissing, false, "`go version` fails, so the CLI cannot be installed."},
			{NoVault, false, "The home-directory AGENTS.md names no existing vault folder in a git work tree."},
			{BinaryMissing, false, "`vault-curator version` is not found or exits non-zero."},
			{NoCandidates, false, "The `scan` or `entities` JSON has an empty `candidates` list (entities: and the user asked for nothing specific)."},
			{NoWatermark, false, "The `scan --json` output has candidates and `watermark.exists` false."},
			{AllSkipped, false, "Every entity candidate got the verdict skip."},
			{InvalidPlan, false, "`validate` or `apply` exits 1 listing plan errors; in a fix step, still after its 3 fix rounds."},
			{SyncRefused, false, "`review --sync` exits 1: an approve-each operation is still pending, or the plan changed since rendering."},
			{NoneApproved, false, "`review --sync` exits 0 and the plan has no approved operation."},
			{ApplyRefused, false, "`apply` exits 1 without plan errors (changes staged outside the vault, a failing hook, a git error); apply rolled back."},
			{MergeDirtyTree, false, "Committing the vault's uncommitted changes fails, or `git merge` exits 1 because local changes would be overwritten; no merge started."},
			{MergeConflict, false, "`git merge` exits 1 with CONFLICT; the merge is in progress."},
			{WatermarkRefused, false, "`vault-curator watermark` exits 1."},
		},
		States: []StateDef{
			{Name: "start", Kind: Start, Vault: none, Inputs: in(InvokeSetup, InvokeCurate, InvokeEntities, InvokeBackfill),
				Invariant: "Nothing has run; the vault is as the user left it."},

			{Name: "setup.binary", Vault: none, Inputs: in(OK, GoMissing),
				Invariant: "No vault file has changed; the CLI may be missing or old."},
			{Name: "setup.vault", Vault: none, Inputs: in(OK, No),
				Invariant: "The CLI is current; no vault file has changed."},
			{Name: "setup.record", Vault: none, Inputs: in(OK, No),
				Invariant: "The vault folder is in a git work tree; no vault file has changed."},
			{Name: "setup.skills", Vault: none, Inputs: in(OK),
				Invariant: "The vault path is recorded in ~/AGENTS.md and the product's global file."},
			{Name: "setup.scan", Vault: none, Inputs: in(InvokeCurate, InvokeBackfill, No),
				Invariant: "The skills are installed; the vault is unchanged."},

			{Name: "curate.locate", Vault: none, Inputs: in(OK, NoVault),
				Invariant: "Nothing has run; the vault is as the user left it."},
			{Name: "curate.binary", Vault: none, Inputs: in(OK, BinaryMissing),
				Invariant: "The vault folder is known and in a git work tree; nothing changed."},
			{Name: "curate.leftover", Vault: none, Inputs: in(OK, Yes, No),
				Invariant: "The CLI runs; this run has no curation branch (earlier runs' curate/* branches may exist)."},
			{Name: "curate.scan", Vault: none, Inputs: in(OK, NoCandidates),
				Invariant: "This run has no curation branch; the vault is unchanged."},
			{Name: "curate.plan", Vault: none, Inputs: in(OK),
				Invariant: "The scan output is in hand; the vault is unchanged."},
			{Name: "curate.validate_draft", Vault: none, Inputs: in(OK, InvalidPlan),
				Invariant: "A plan file exists with every status pending; no note has changed."},
			{Name: "curate.review", Vault: none, Inputs: in(DecisionsGiven, No),
				Invariant: "The plan passes validate --draft and its review file is rendered; no note has changed."},
			{Name: "curate.record", Vault: none, Inputs: in(OK, SyncRefused, NoneApproved),
				Invariant: "The user has decided every category and approve-each operation; the plan is not yet synced."},
			{Name: "curate.apply", Vault: none, Inputs: in(OK, InvalidPlan, ApplyRefused),
				Invariant: "The plan holds the user's synced statuses, at least one approved; no note has changed."},
			{Name: "curate.revise", Vault: none, Inputs: in(OK, InvalidPlan),
				Invariant: "The synced plan fails validation; nothing is applied."},
			{Name: "curate.confirm", Vault: open, Inputs: in(Yes, No),
				Invariant: "A curate/* branch and its worktree hold the approved operations, unmerged; the user's branch has at most a snapshot of their own edits."},
			{Name: "curate.merge", Vault: open, Inputs: in(OK, MergeDirtyTree, MergeConflict),
				Invariant: "The user confirmed this merge; the branch is unmerged."},
			{Name: "curate.abort_merge", Vault: conflicted, Inputs: in(OK),
				Invariant: "A merge of the branch stopped on conflicts and is in progress on the user's branch."},
			{Name: "curate.watermark", Vault: merged, Inputs: in(OK, WatermarkRefused),
				Invariant: "The branch is merged into the user's branch; the watermark has not moved."},

			{Name: "entities.prepare", Vault: none, Inputs: in(OK, NoVault, BinaryMissing),
				Invariant: "Nothing has run; the vault is as the user left it."},
			{Name: "entities.list", Vault: none, Inputs: in(OK, NoCandidates),
				Invariant: "The vault is located and the CLI runs; this pass has no curation branch."},
			{Name: "entities.decide", Vault: none, Inputs: in(OK, AllSkipped),
				Invariant: "The entity and candidate lists are in hand; the vault is unchanged."},
			{Name: "entities.plan", Vault: none, Inputs: in(OK),
				Invariant: "Every candidate has a verdict; the vault is unchanged."},
			{Name: "entities.merge", Vault: none, Inputs: in(OK),
				Invariant: "The plan holds every seed and alias; merges are still to write."},
			{Name: "entities.report", Vault: none, Inputs: in(OK),
				Invariant: "The entity plan is merged and watermarked; no curation branch is left."},

			{Name: "backfill.prepare", Vault: none, Inputs: in(OK, NoVault, BinaryMissing),
				Invariant: "Nothing has run; the vault is as the user left it."},
			{Name: "backfill.start", Vault: none, Inputs: in(OK, NoCandidates, NoWatermark),
				Invariant: "The vault is located and the CLI runs; no batch has run in this backfill."},
			{Name: "backfill.seed", Vault: none, Inputs: in(OK),
				Invariant: "No watermark exists and unprocessed notes remain; seeding has not run in this backfill."},
			{Name: "backfill.plan", Vault: none, Inputs: in(OK),
				Invariant: "A batch from scan --all --limit is in hand; it has no curation branch."},
			{Name: "backfill.next", Vault: none, Inputs: in(OK, NoCandidates),
				Invariant: "The previous batch ended (merged and watermarked, applied nothing, or discarded); no curation branch is left."},

			{Name: "done", Kind: Terminal, Vault: none,
				Invariant: "Finished: everything approved is merged and watermarked; no branch, worktree or merge in progress is left."},
			{Name: "stopped_clean", Kind: Terminal, Vault: none,
				Invariant: "Stopped before the current pass applied anything; no branch, worktree or merge in progress is left."},
			{Name: "parked", Kind: Terminal, Vault: open,
				Invariant: "The user declined the merge: the branch and its worktree stay unmerged, the user's branch is untouched; curate.leftover finds it."},
			{Name: "stopped_merged", Kind: Terminal, Vault: merged,
				Invariant: "The branch is merged but watermark refused; curate.leftover finds the branch, and finishing it runs watermark again."},
		},
		Transitions: []Transition{
			{From: "start", On: InvokeSetup, To: "setup.binary"},
			{From: "start", On: InvokeCurate, To: "curate.locate"},
			{From: "start", On: InvokeEntities, To: "entities.prepare"},
			{From: "start", On: InvokeBackfill, To: "backfill.prepare"},

			{From: "setup.binary", On: OK, To: "setup.vault"},
			{From: "setup.binary", On: GoMissing, To: "stopped_clean"},
			{From: "setup.vault", On: OK, To: "setup.record"},
			{From: "setup.vault", On: No, To: "stopped_clean"},
			{From: "setup.record", On: OK, To: "setup.skills"},
			{From: "setup.record", On: No, To: "stopped_clean"},
			{From: "setup.skills", On: OK, To: "setup.scan"},
			{From: "setup.scan", On: InvokeCurate, To: "curate.locate"},
			{From: "setup.scan", On: InvokeBackfill, To: "backfill.prepare"},
			{From: "setup.scan", On: No, To: "done"},

			{From: "curate.locate", On: OK, To: "curate.binary"},
			{From: "curate.locate", On: NoVault, To: "setup.binary"},
			{From: "curate.binary", On: OK, To: "curate.leftover"},
			{From: "curate.binary", On: BinaryMissing, To: "setup.binary"},
			{From: "curate.leftover", On: OK, To: "curate.scan"},
			{From: "curate.leftover", On: Yes, To: "curate.confirm", Effect: EffectBranch},
			{From: "curate.leftover", On: No, To: "curate.scan"},
			{From: "curate.scan", On: OK, To: "curate.plan"},
			{From: "curate.scan", On: NoCandidates, To: "done"},
			{From: "curate.plan", On: OK, To: "curate.validate_draft"},
			{From: "curate.validate_draft", On: OK, To: "curate.review"},
			{From: "curate.validate_draft", On: InvalidPlan, To: "stopped_clean"},
			{From: "curate.review", On: DecisionsGiven, To: "curate.record"},
			{From: "curate.review", On: No, To: "stopped_clean"},
			{From: "curate.record", On: OK, To: "curate.apply"},
			{From: "curate.record", On: SyncRefused, To: "curate.review"},
			{From: "curate.record", On: NoneApproved, Exit: ExitNothing},
			{From: "curate.apply", On: OK, To: "curate.confirm", Effect: EffectBranch},
			{From: "curate.apply", On: InvalidPlan, To: "curate.revise"},
			{From: "curate.apply", On: ApplyRefused, To: "stopped_clean"},
			{From: "curate.revise", On: OK, To: "curate.review"},
			{From: "curate.revise", On: InvalidPlan, To: "stopped_clean"},
			{From: "curate.confirm", On: Yes, To: "curate.merge"},
			{From: "curate.confirm", On: No, To: "parked"},
			{From: "curate.merge", On: OK, To: "curate.watermark", Effect: EffectMerge},
			{From: "curate.merge", On: MergeDirtyTree, To: "curate.confirm"},
			{From: "curate.merge", On: MergeConflict, To: "curate.abort_merge", Effect: EffectConflict},
			{From: "curate.abort_merge", On: OK, Exit: ExitReplan, Effect: EffectDiscard},
			{From: "curate.watermark", On: OK, Exit: ExitMerged, Effect: EffectWatermark},
			{From: "curate.watermark", On: WatermarkRefused, To: "stopped_merged"},

			{From: "entities.prepare", On: OK, To: "entities.list"},
			{From: "entities.prepare", On: NoVault, To: "setup.binary"},
			{From: "entities.prepare", On: BinaryMissing, To: "setup.binary"},
			{From: "entities.list", On: OK, To: "entities.decide"},
			{From: "entities.list", On: NoCandidates, Exit: ExitNothing},
			{From: "entities.decide", On: OK, To: "entities.plan"},
			{From: "entities.decide", On: AllSkipped, Exit: ExitNothing},
			{From: "entities.plan", On: OK, To: "entities.merge"},
			{From: "entities.merge", On: OK, To: "curate.validate_draft"},
			{From: "entities.report", On: OK, To: "done"},

			{From: "backfill.prepare", On: OK, To: "backfill.start"},
			{From: "backfill.prepare", On: NoVault, To: "setup.binary"},
			{From: "backfill.prepare", On: BinaryMissing, To: "setup.binary"},
			{From: "backfill.start", On: OK, To: "backfill.plan"},
			{From: "backfill.start", On: NoCandidates, To: "done"},
			{From: "backfill.start", On: NoWatermark, To: "backfill.seed"},
			{From: "backfill.seed", On: OK, To: "entities.list"},
			{From: "backfill.plan", On: OK, To: "curate.validate_draft"},
			{From: "backfill.next", On: OK, To: "backfill.plan"},
			{From: "backfill.next", On: NoCandidates, To: "done"},
		},
		Entries: map[State]Skill{
			"setup.binary":     "setup",
			"curate.locate":    "curate",
			"entities.prepare": "entities",
			"backfill.prepare": "backfill",
		},
		Returns: map[Skill]map[Exit]State{
			"curate":   {ExitMerged: "done", ExitNothing: "done", ExitReplan: "curate.scan"},
			"entities": {ExitMerged: "entities.report", ExitNothing: "done", ExitReplan: "entities.list"},
			"backfill": {ExitMerged: "backfill.next", ExitNothing: "backfill.next", ExitReplan: "backfill.next"},
		},
		ResumeFrom: "curate.leftover",
		ResumeOn:   Yes,
	}
}
