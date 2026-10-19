// Package fixture checks that the synthetic vault in testdata/ and its
// expectations stay consistent with each other.
package fixture

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	vaultDir    = filepath.Join("..", "..", "..", "testdata", "vault")
	expectedDir = filepath.Join("..", "..", "..", "testdata", "expected")
)

type expectation struct {
	Note     string    `json:"note"`
	Date     string    `json:"date"`
	Segments []segment `json:"segments"`
}

type segment struct {
	Lines   [2]int   `json:"lines"`
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Primary []string `json:"primary"`
	Cue     string   `json:"cue"`
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// messyDailyNotes returns daily notes with no frontmatter, relative to the vault.
func messyDailyNotes(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(vaultDir, "daily", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	var messy []string
	for _, p := range paths {
		if lines := readLines(t, p); lines[0] != "---" {
			rel, _ := filepath.Rel(vaultDir, p)
			messy = append(messy, filepath.ToSlash(rel))
		}
	}
	if len(messy) < 3 {
		t.Fatalf("want at least 3 messy daily notes, got %d", len(messy))
	}
	return messy
}

// noteNames returns every note's basename without extension.
func noteNames(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	err := filepath.WalkDir(vaultDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			names[strings.TrimSuffix(d.Name(), ".md")] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func loadExpectation(t *testing.T, path string) expectation {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var e expectation
	if err := dec.Decode(&e); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return e
}

func TestEveryMessyDailyNoteHasExpectation(t *testing.T) {
	for _, note := range messyDailyNotes(t) {
		name := strings.TrimSuffix(filepath.Base(note), ".md") + ".json"
		e := loadExpectation(t, filepath.Join(expectedDir, name))
		if e.Note != note {
			t.Errorf("%s: note = %q, want %q", name, e.Note, note)
		}
	}
}

func TestExpectationsPartitionTheirNote(t *testing.T) {
	names := noteNames(t)
	paths, err := filepath.Glob(filepath.Join(expectedDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		e := loadExpectation(t, p)
		lines := readLines(t, filepath.Join(vaultDir, filepath.FromSlash(e.Note)))
		if len(e.Segments) == 0 {
			t.Errorf("%s: no segments", p)
		}
		covered := make([]int, len(lines)+1)
		prevEnd := 0
		for _, s := range e.Segments {
			start, end := s.Lines[0], s.Lines[1]
			if start <= prevEnd || end < start || end > len(lines) {
				t.Errorf("%s: segment %q has bad or out-of-order range %v", p, s.Title, s.Lines)
				continue
			}
			prevEnd = end
			if strings.TrimSpace(lines[start-1]) == "" || strings.TrimSpace(lines[end-1]) == "" {
				t.Errorf("%s: segment %q starts or ends on a blank line", p, s.Title)
			}
			for i := start; i <= end; i++ {
				covered[i]++
			}
			if s.Kind != "meeting" && s.Kind != "other" {
				t.Errorf("%s: segment %q kind = %q", p, s.Title, s.Kind)
			}
			if s.Title == "" || s.Cue == "" {
				t.Errorf("%s: segment %v missing title or cue", p, s.Lines)
			}
			if len(s.Primary) < 1 || len(s.Primary) > 2 {
				t.Errorf("%s: segment %q wants 1 or 2 primary topics, got %d", p, s.Title, len(s.Primary))
			}
			for _, topic := range s.Primary {
				if !names[topic] {
					t.Errorf("%s: segment %q primary %q is not a note in the vault", p, s.Title, topic)
				}
			}
		}
		for i := 1; i <= len(lines); i++ {
			if strings.TrimSpace(lines[i-1]) != "" && covered[i] != 1 {
				t.Errorf("%s: line %d is covered %d times, want 1", p, i, covered[i])
			}
		}
	}
}

// TestDailyNotesStayMessy guards against editors or formatters tidying the fixture.
func TestDailyNotesStayMessy(t *testing.T) {
	var all strings.Builder
	for _, note := range messyDailyNotes(t) {
		data, err := os.ReadFile(filepath.Join(vaultDir, filepath.FromSlash(note)))
		if err != nil {
			t.Fatal(err)
		}
		all.Write(data)
	}
	text := "\n" + all.String()
	for _, want := range []string{"\n\t", "\n  ", "\n- ", "\n* ", "\n+ ", "\t+ ", "\t  - "} {
		if !strings.Contains(text, want) {
			t.Errorf("messy daily notes no longer contain %q", want)
		}
	}
}

func TestFixtureIsNotAGitRepo(t *testing.T) {
	if _, err := os.Stat(filepath.Join(vaultDir, ".git")); err == nil {
		t.Error("testdata/vault must be plain files; tests git init a temp copy")
	}
}
