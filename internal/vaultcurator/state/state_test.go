package state

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
)

func hashes(bs []note.Block) []string {
	out := []string{}
	for _, b := range bs {
		out = append(out, b.Hash)
	}
	return out
}

func TestLoadMissingIsEmpty(t *testing.T) {
	s, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.Version != Version || len(s.Files) != 0 {
		t.Errorf("Load = %+v, want empty", s)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	s := New()
	s.MarkBlocks("daily/b.md", "2222")
	s.MarkBlocks("daily/a.md", "1111", "3333")
	if err := s.Save(root); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, ".vault-curator", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"version\": 1,\n  \"files\": {\n    \"daily/a.md\": [\n      \"1111\",\n      \"3333\"\n    ],\n    \"daily/b.md\": [\n      \"2222\"\n    ]\n  }\n}\n"
	if string(b) != want {
		t.Errorf("state file =\n%s\nwant\n%s", b, want)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Errorf("Load = %+v, want %+v", got, s)
	}
	if p := got.Paths(); !reflect.DeepEqual(p, []string{"daily/a.md", "daily/b.md"}) {
		t.Errorf("Paths = %q", p)
	}
}

func TestLeftovers(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".vault-curator"), 0o755)
	// A state written before leftovers existed still loads.
	os.WriteFile(filepath.Join(root, ".vault-curator", "state.json"), []byte(`{"version": 1, "files": {}}`), 0o644)
	s, err := Load(root)
	if err != nil || len(s.Leftovers) != 0 {
		t.Fatalf("old state: %+v, %v", s, err)
	}

	s.AddLeftover("b.md")
	s.AddLeftover("a.md")
	s.AddLeftover("a.md")
	if err := s.Save(root); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil || !reflect.DeepEqual(got.Leftovers, []string{"a.md", "b.md"}) {
		t.Fatalf("leftovers = %v, %v", got.Leftovers, err)
	}

	got.MarkNote(note.Parse("a.md", []byte("- x\n")))
	if got.IsLeftover("a.md") || !got.IsLeftover("b.md") {
		t.Errorf("after MarkNote: %v", got.Leftovers)
	}
	got.Rename("b.md", "c.md")
	if got.IsLeftover("b.md") || !got.IsLeftover("c.md") {
		t.Errorf("after Rename: %v", got.Leftovers)
	}
	got.Forget("c.md")
	if len(got.Leftovers) != 0 {
		t.Errorf("after Forget: %v", got.Leftovers)
	}
}

func TestLoadRejectsBadFile(t *testing.T) {
	for name, content := range map[string]string{
		"bad json":    "{",
		"bad version": `{"version": 2, "files": {}}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			os.MkdirAll(filepath.Join(root, ".vault-curator"), 0o755)
			os.WriteFile(filepath.Join(root, ".vault-curator", "state.json"), []byte(content), 0o644)
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), Path) {
				t.Errorf("err = %v, want one naming %s", err, Path)
			}
		})
	}
}

func TestNewBlocksAfterAppend(t *testing.T) {
	src := "- standup\n  - item\n\n- 1:1\n  - item\n"
	n := note.Parse("daily/d.md", []byte(src))
	s := New()
	if got := s.NewBlocks(n); len(got) != 2 || s.Tracked(n.Path) {
		t.Fatalf("untracked note: %d new blocks, tracked = %v", len(got), s.Tracked(n.Path))
	}
	s.MarkNote(n)
	if got := s.NewBlocks(n); len(got) != 0 {
		t.Fatalf("after MarkNote: %d new blocks", len(got))
	}

	// Appending a block, and repeating an identical one, makes only those new.
	n2 := note.Parse("daily/d.md", []byte(src+"\n- retro\n\n- standup\n  - item\n"))
	got := s.NewBlocks(n2)
	if want := []string{n2.Blocks[2].Hash, n2.Blocks[3].Hash}; !reflect.DeepEqual(hashes(got), want) {
		t.Errorf("NewBlocks = %q, want %q", hashes(got), want)
	}
	if got[0].StartLine != 7 || got[1].StartLine != 9 {
		t.Errorf("new block lines = %d, %d; want 7, 9", got[0].StartLine, got[1].StartLine)
	}

	// Editing a processed block makes it new again.
	n3 := note.Parse("daily/d.md", []byte("- standup\n  - item edited\n\n- 1:1\n  - item\n"))
	if got := s.NewBlocks(n3); len(got) != 1 || got[0].StartLine != 1 {
		t.Errorf("edited: NewBlocks = %+v", got)
	}
}

func TestRenameForget(t *testing.T) {
	s := New()
	s.MarkBlocks("a.md", "1")
	s.Rename("a.md", "b.md")
	s.Rename("missing.md", "c.md")
	if s.Tracked("a.md") || !s.Tracked("b.md") || s.Tracked("c.md") {
		t.Errorf("after Rename: %+v", s.Files)
	}
	s.Forget("b.md")
	if len(s.Files) != 0 {
		t.Errorf("after Forget: %+v", s.Files)
	}
	var zero State
	zero.MarkBlocks("x.md", "1")
	if zero.Version != Version || !zero.Tracked("x.md") {
		t.Errorf("zero State after MarkBlocks: %+v", zero)
	}
}
