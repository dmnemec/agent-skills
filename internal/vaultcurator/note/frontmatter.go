package note

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Frontmatter is the parsed YAML block at the top of a note. The typed fields
// cover the design's note model; Fields holds every key as decoded YAML.
type Frontmatter struct {
	// Present reports whether the note has a frontmatter block.
	Present bool `json:"present"`
	// Type is the note type: daily, meeting, project, component or person.
	Type string `json:"type,omitempty"`
	// Date is the meeting or capture date, as written.
	Date string `json:"date,omitempty"`
	// Project and Topics are the primary subjects. Either may be written as
	// a single value or a list; values are kept as written (e.g. "[[SMP]]").
	Project []string `json:"project,omitempty"`
	Topics  []string `json:"topics,omitempty"`
	// Aliases are alternate names that wikilinks resolve to this note by.
	Aliases []string `json:"aliases,omitempty"`
	// Fields is the whole frontmatter decoded into generic values. Timestamps
	// stay strings.
	Fields map[string]any `json:"fields,omitempty"`
	// Err is set when the YAML could not be decoded; the typed fields are
	// then empty.
	Err error `json:"-"`
}

// scalar decodes any YAML scalar as its literal text.
type scalar string

func (s *scalar) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode && n.Tag != "!!null" {
		*s = scalar(n.Value)
	}
	return nil
}

// stringList decodes a scalar or a sequence of scalars.
type stringList []string

func (l *stringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Tag != "!!null" && n.Value != "" {
			*l = []string{n.Value}
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			if c.Kind == yaml.ScalarNode && c.Tag != "!!null" {
				*l = append(*l, c.Value)
			}
		}
	}
	return nil
}

type typedFrontmatter struct {
	Type    scalar     `yaml:"type"`
	Date    scalar     `yaml:"date"`
	Project stringList `yaml:"project"`
	Topics  stringList `yaml:"topics"`
	Aliases stringList `yaml:"aliases"`
}

func parseFrontmatter(body []byte) Frontmatter {
	fm := Frontmatter{Present: true}
	var doc yaml.Node
	if err := yaml.Unmarshal(body, &doc); err != nil {
		fm.Err = err
		return fm
	}
	if len(doc.Content) == 0 {
		return fm // empty frontmatter
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		fm.Err = fmt.Errorf("frontmatter is a YAML %s, not a mapping", kindName(root.Kind))
		return fm
	}
	var t typedFrontmatter
	if err := root.Decode(&t); err != nil {
		fm.Err = err
		return fm
	}
	fields, err := generic(root)
	if err != nil {
		fm.Err = err
		return fm
	}
	fm.Fields = fields.(map[string]any)
	fm.Type = string(t.Type)
	fm.Date = string(t.Date)
	fm.Project = t.Project
	fm.Topics = t.Topics
	fm.Aliases = t.Aliases
	return fm
}

// generic decodes n into maps, slices and scalars like yaml.Unmarshal into
// an any, except that timestamps stay strings as written.
func generic(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.AliasNode:
		return generic(n.Alias)
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			v, err := generic(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m[n.Content[i].Value] = v
		}
		return m, nil
	case yaml.SequenceNode:
		s := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := generic(c)
			if err != nil {
				return nil, err
			}
			s = append(s, v)
		}
		return s, nil
	default:
		if n.ShortTag() == "!!timestamp" {
			return n.Value, nil
		}
		var v any
		err := n.Decode(&v)
		return v, err
	}
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	default:
		return "node"
	}
}
