package flow

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// A skill step heading names its state with a trailing tag:
//
//	## 9. Confirm, merge, watermark (state: curate.confirm)
//	### Merge conflict (state: curate.abort_merge)
//
// Every "##" heading of a SKILL.md is a step and needs a tag. A "###" heading
// with a tag is a sub-step holding a recovery state; untagged "###" headings
// are plain sections.
var headingRe = regexp.MustCompile(`^(#{2,3}) (.*?)(?: \(state: ([^)]*)\))?\s*$`)

// StepTag is one tagged heading of a skill.
type StepTag struct {
	Skill   Skill
	Line    int
	Heading string
	State   State
}

// ParseSkill returns the tagged step headings of one SKILL.md, and an error
// for each "##" heading without a tag. Fenced code is skipped.
func ParseSkill(sk Skill, text string) ([]StepTag, error) {
	var tags []StepTag
	var errs []error
	fenced := false
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		g := headingRe.FindStringSubmatch(line)
		if g == nil {
			continue
		}
		if g[3] == "" {
			if g[1] == "##" {
				errs = append(errs, fmt.Errorf("%s/SKILL.md:%d: step %q names no state; end it with (state: <name>)", sk, i+1, g[2]))
			}
			continue
		}
		tags = append(tags, StepTag{Skill: sk, Line: i + 1, Heading: g[2], State: State(g[3])})
	}
	return tags, errors.Join(errs...)
}

// CheckSkills checks the skills against the model: every step names a state
// of its own skill, no state has two steps, and every step state of the model
// has one. skills maps a skill's folder name to its SKILL.md text.
func CheckSkills(m *Model, skills map[Skill]string) error {
	var errs []error
	where := map[State]StepTag{}
	names := make([]Skill, 0, len(skills))
	for sk := range skills {
		names = append(names, sk)
	}
	slices.Sort(names)
	for _, sk := range names {
		if !slices.Contains(m.Skills(), sk) {
			errs = append(errs, fmt.Errorf("skill folder %s has no states in the model", sk))
			continue
		}
		tags, err := ParseSkill(sk, skills[sk])
		if err != nil {
			errs = append(errs, err)
		}
		for _, t := range tags {
			s := m.State(t.State)
			switch {
			case s == nil:
				errs = append(errs, fmt.Errorf("%s/SKILL.md:%d: step %q names unknown state %s", sk, t.Line, t.Heading, t.State))
				continue
			case t.State.Skill() != sk:
				errs = append(errs, fmt.Errorf("%s/SKILL.md:%d: step %q names %s, a state of another skill", sk, t.Line, t.Heading, t.State))
				continue
			}
			if prev, ok := where[t.State]; ok {
				errs = append(errs, fmt.Errorf("%s/SKILL.md:%d: state %s already has a step at line %d", sk, t.Line, t.State, prev.Line))
				continue
			}
			where[t.State] = t
		}
	}
	for _, s := range m.States {
		if s.Kind != Step {
			continue
		}
		if _, ok := skills[s.Name.Skill()]; !ok {
			errs = append(errs, fmt.Errorf("state %s: skill %s has no SKILL.md", s.Name, s.Name.Skill()))
			continue
		}
		if _, ok := where[s.Name]; !ok {
			errs = append(errs, fmt.Errorf("state %s has no step in %s/SKILL.md", s.Name, s.Name.Skill()))
		}
	}
	return errors.Join(errs...)
}
