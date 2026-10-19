package plan

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/note"
	"gopkg.in/yaml.v3"
)

// fieldOrder is where new frontmatter fields go: the note model's fields in
// this order, then any others alphabetically. Existing fields keep their
// place.
var fieldOrder = []string{"type", "date", "project", "topics", "aliases", "attendees", "repos", "links", "supersedes"}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// setFrontmatter returns src with fields set in its frontmatter (a nil value
// removes the field), adding a frontmatter block if there is none. The body
// is kept byte for byte; the frontmatter is re-encoded.
func setFrontmatter(src string, fields map[string]any) (string, error) {
	n := note.Parse("", []byte(src))
	var body, rest string
	rest = src
	if len(n.Segments) > 0 && n.Segments[0].Kind == note.KindFrontmatter {
		if n.Frontmatter.Err != nil {
			return "", fmt.Errorf("cannot set frontmatter: it is malformed: %v", n.Frontmatter.Err)
		}
		seg := n.SegmentText(n.Segments[0])
		open := strings.IndexByte(seg, '\n') + 1
		closing := strings.LastIndexByte(strings.TrimSuffix(seg, "\n"), '\n') + 1
		body = seg[open:closing]
		rest = src[n.Segments[0].End:]
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return "", fmt.Errorf("cannot set frontmatter: %v", err)
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	for _, k := range orderedKeys(fields) {
		i := -1
		for j := 0; j+1 < len(root.Content); j += 2 {
			if root.Content[j].Value == k {
				i = j
				break
			}
		}
		v := fields[k]
		if v == nil {
			if i >= 0 {
				root.Content = append(root.Content[:i], root.Content[i+2:]...)
			}
			continue
		}
		val, err := valueNode(v)
		if err != nil {
			return "", fmt.Errorf("frontmatter field %s: %v", k, err)
		}
		if i >= 0 {
			root.Content[i+1] = val
			continue
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, val)
	}
	if len(root.Content) == 0 {
		return rest, nil
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return "---\n" + b.String() + "---\n" + rest, nil
}

func orderedKeys(fields map[string]any) []string {
	rank := map[string]int{}
	for i, k := range fieldOrder {
		rank[k] = i + 1
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := rank[keys[i]], rank[keys[j]]
		switch {
		case ri > 0 && rj > 0:
			return ri < rj
		case ri > 0 || rj > 0:
			return ri > 0
		}
		return keys[i] < keys[j]
	})
	return keys
}

// valueNode encodes a JSON value as YAML in the vault's style: wikilinks
// double-quoted ("[[Harbor]]"), dates plain (2026-09-14).
func valueNode(v any) (*yaml.Node, error) {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
			switch {
			case strings.Contains(n.Value, "[["):
				n.Style = yaml.DoubleQuotedStyle
			case dateRe.MatchString(n.Value):
				n.Tag, n.Style = "!!timestamp", 0
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&n)
	return &n, nil
}
