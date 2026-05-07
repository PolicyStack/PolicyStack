// Package sourceloc walks a yaml.v3 Node tree by path and returns the
// (line, column) of a leaf value. Used so findings can pin to the exact
// values.yaml line a developer needs to fix.
package sourceloc

import (
	"strconv"

	"gopkg.in/yaml.v3"
)

// Loc is a 1-based file location. Zero values mean "unknown".
type Loc struct {
	Line, Col int
}

// Find walks doc by path and returns the location of the resolved node.
// Path segments index by map key (string) or array index (decimal string).
// Returns zero Loc if any segment fails to resolve.
func Find(doc *yaml.Node, path ...string) Loc {
	n := walk(doc, path)
	if n == nil {
		return Loc{}
	}
	return Loc{Line: n.Line, Col: n.Column}
}

// FindNode is Find but returns the node itself (or nil).
func FindNode(doc *yaml.Node, path ...string) *yaml.Node {
	return walk(doc, path)
}

func walk(n *yaml.Node, path []string) *yaml.Node {
	if n == nil {
		return nil
	}
	// Unwrap document node.
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	for _, seg := range path {
		switch n.Kind {
		case yaml.MappingNode:
			next := mapValue(n, seg)
			if next == nil {
				return nil
			}
			n = next
		case yaml.SequenceNode:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(n.Content) {
				return nil
			}
			n = n.Content[i]
		default:
			return nil
		}
	}
	return n
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
