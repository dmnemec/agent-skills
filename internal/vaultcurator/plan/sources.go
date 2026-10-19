package plan

import (
	"fmt"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
	"github.com/dmnemec/agent-skills/internal/vaultcurator/vault"
)

// Selected returns the original bullets src selects in v, in line order,
// checked as Validate checks sources. Review uses it to show where each
// original bullet goes.
func Selected(v *vault.Vault, src Source) ([]note.Item, error) {
	n := v.Note(src.Note)
	if n == nil {
		return nil, fmt.Errorf("source note %s not found", src.Note)
	}
	c := &checker{v: v, mapped: map[string]map[int]string{}}
	s, ok := c.resolveSource(&Operation{}, n, src)
	if !ok {
		return nil, c.errs[0]
	}
	return s.items, nil
}

// SplitLink returns the line a split leaves in its note where the moved
// bullets were: Link, or "- [[<new note title>]]" when Link is empty.
func (op *Operation) SplitLink() string { return splitLink(op) }
