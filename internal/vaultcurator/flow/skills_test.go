package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readSkills(t *testing.T) map[Skill]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "skills", "vault-curator", "*", "SKILL.md"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no skills found: %v", err)
	}
	skills := map[Skill]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		skills[Skill(filepath.Base(filepath.Dir(p)))] = string(b)
	}
	return skills
}

func TestSkillsNameTheirStates(t *testing.T) {
	if err := CheckSkills(VaultCurator(), readSkills(t)); err != nil {
		t.Fatal(err)
	}
}

func TestCheckSkillsCatchesMismatches(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(map[Skill]string)
		wants string
	}{
		{"a step with no state", func(s map[Skill]string) {
			s["curate"] += "\n## 10. Celebrate\n"
		}, "names no state"},
		{"a step with an unknown state", func(s map[Skill]string) {
			s["curate"] += "\n## 10. Celebrate (state: curate.celebrate)\n"
		}, "unknown state curate.celebrate"},
		{"a step with another skill's state", func(s map[Skill]string) {
			s["backfill"] += "\n## 6. Merge (state: curate.merge)\n"
		}, "another skill"},
		{"a state with two steps", func(s map[Skill]string) {
			s["curate"] += "\n### Merge again (state: curate.merge)\n"
		}, "already has a step"},
		{"a state with no step", func(s map[Skill]string) {
			s["curate"] = strings.Replace(s["curate"], " (state: curate.abort_merge)", "", 1)
		}, "curate.abort_merge has no step"},
		{"a skill folder the model does not know", func(s map[Skill]string) {
			s["context"] = "## 1. Build (state: context.build)\n"
		}, "skill folder context"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			skills := readSkills(t)
			c.edit(skills)
			err := CheckSkills(VaultCurator(), skills)
			if err == nil || !strings.Contains(err.Error(), c.wants) {
				t.Fatalf("got %v, want an error containing %q", err, c.wants)
			}
		})
	}
}

func TestParseSkillSkipsFencedCode(t *testing.T) {
	tags, err := ParseSkill("setup", "## 1. Binary (state: setup.binary)\n\n```markdown\n## Notes vault\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].State != "setup.binary" {
		t.Fatalf("tags = %+v", tags)
	}
}
