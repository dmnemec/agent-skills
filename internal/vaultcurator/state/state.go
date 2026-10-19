// Package state persists which top-level blocks curation has already
// processed, so scan offers only new blocks in a changed file (a daily note
// appended to all day). The state is a JSON file committed in the vault at
// Path; it holds block hashes only, never note content.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
)

// Path is the state file's location, relative to the vault root.
const Path = ".vault-curator/state.json"

// Version is the state file format version this package reads and writes.
const Version = 1

// State maps each note to the hashes of its processed blocks.
type State struct {
	Version int `json:"version"`
	// Files maps a vault-relative note path to the hashes of its processed
	// blocks, in block order. A hash appears once per processed copy, so two
	// identical blocks in one note are tracked separately.
	Files map[string][]string `json:"files"`
}

// New returns an empty State.
func New() *State {
	return &State{Version: Version, Files: map[string][]string{}}
}

// Load reads the state file under vaultRoot. A missing file is an empty
// State, not an error.
func Load(vaultRoot string) (*State, error) {
	b, err := os.ReadFile(filepath.Join(vaultRoot, filepath.FromSlash(Path)))
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	s := New()
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("%s: %w", Path, err)
	}
	if s.Version != Version {
		return nil, fmt.Errorf("%s: unsupported version %d (want %d)", Path, s.Version, Version)
	}
	if s.Files == nil {
		s.Files = map[string][]string{}
	}
	return s, nil
}

// Save writes the state file under vaultRoot, creating its folder. Output is
// stable (sorted paths, two-space indent) so commits diff cleanly.
func (s *State) Save(vaultRoot string) error {
	p := filepath.Join(vaultRoot, filepath.FromSlash(Path))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	out := State{Version: Version, Files: s.Files}
	if out.Files == nil {
		out.Files = map[string][]string{}
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// Tracked reports whether the note at path has a state entry, i.e. curation
// has processed it at least once.
func (s *State) Tracked(path string) bool {
	_, ok := s.Files[path]
	return ok
}

// Paths returns the tracked note paths, sorted.
func (s *State) Paths() []string {
	ps := make([]string, 0, len(s.Files))
	for p := range s.Files {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

// NewBlocks returns n's blocks that are not yet processed, in order. Each
// recorded hash accounts for one block, so a block repeated in the note is
// new once per copy beyond the recorded ones.
func (s *State) NewBlocks(n *note.Note) []note.Block {
	left := map[string]int{}
	for _, h := range s.Files[n.Path] {
		left[h]++
	}
	out := []note.Block{}
	for _, b := range n.Blocks {
		if left[b.Hash] > 0 {
			left[b.Hash]--
			continue
		}
		out = append(out, b)
	}
	return out
}

// MarkNote records every block of n as processed, replacing what was
// recorded for its path. Apply calls it for each note it writes, after
// curation, with the note re-parsed from the written bytes.
func (s *State) MarkNote(n *note.Note) {
	hashes := make([]string, len(n.Blocks))
	for i, b := range n.Blocks {
		hashes[i] = b.Hash
	}
	s.ensure()
	s.Files[n.Path] = hashes
}

// MarkBlocks adds hashes to path's processed blocks, keeping what is
// already recorded.
func (s *State) MarkBlocks(path string, hashes ...string) {
	s.ensure()
	s.Files[path] = append(append([]string{}, s.Files[path]...), hashes...)
}

// Forget drops path's entry, for a note that was deleted or merged away.
func (s *State) Forget(path string) {
	delete(s.Files, path)
}

// Rename moves from's entry to to, for a note that was renamed or moved. It
// replaces any entry already at to and does nothing if from is untracked.
func (s *State) Rename(from, to string) {
	h, ok := s.Files[from]
	if !ok || from == to {
		return
	}
	s.Files[to] = h
	delete(s.Files, from)
}

func (s *State) ensure() {
	if s.Files == nil {
		s.Files = map[string][]string{}
	}
	if s.Version == 0 {
		s.Version = Version
	}
}
