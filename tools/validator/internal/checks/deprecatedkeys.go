package checks

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

var (
	// deprecatedNote finds the deprecation in a key's description: the comment block directly above
	// it in values.yaml, its lines joined by spaces, as tools/doc-generator.py reads it.
	deprecatedNote = regexp.MustCompile(`\bDeprecated:\s*(.*)`)
	// deprecatedForm is the one form a deprecation is written in: "use <key>; removed in <version>".
	deprecatedForm = regexp.MustCompile(`^use (\S+); removed in (\S+?)\.?(?:\s|$)`)
)

// DeprecatedKeyCheck (POLICY039) warns where a values layer or fleet file sets a key whose
// description in an element's or lifecycle's values.yaml says "Deprecated: use <key>; removed in
// <version>". The key keeps working, and POLICY035 keeps knowing it, until that version. A
// "Deprecated:" note in any other form is a warning at the values.yaml.
type DeprecatedKeyCheck struct{}

func (DeprecatedKeyCheck) ID() string   { return "POLICY039" }
func (DeprecatedKeyCheck) Phase() Phase { return PhaseRepo }

// deprecatedKey is a deprecated values key: its path from the top of the values, the key to use
// instead, and the version that removes it.
type deprecatedKey struct {
	element, use, removed string
	path                  []string
}

func (c *DeprecatedKeyCheck) Run(ctx Context) []Finding {
	var out []Finding
	var keys []deprecatedKey
	seen := map[string]bool{} // lifecycle's components share one values.yaml
	for _, el := range ctx.AllElements {
		if seen[el.ValuesFile] || el.ValuesDoc == nil {
			continue
		}
		seen[el.ValuesFile] = true
		data, err := os.ReadFile(el.ValuesFile)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		walkKeys(el.ValuesDoc, nil, func(k *yaml.Node, path []string) {
			m := deprecatedNote.FindStringSubmatch(description(lines, k))
			if m == nil {
				return
			}
			if f := deprecatedForm.FindStringSubmatch(m[1]); f != nil {
				keys = append(keys, deprecatedKey{element: el.ChartName, use: f[1], removed: f[2], path: path})
				return
			}
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevWarning, Element: el.ChartName,
				Message: fmt.Sprintf("%s: write the deprecation as \"Deprecated: use <key>; removed in <version>\", not %q",
					strings.Join(path, "."), m[0]),
				File: el.ValuesFile, Line: k.Line, Col: k.Column,
			})
		})
	}
	if len(keys) == 0 {
		return out
	}
	files := slices.Clone(ctx.Layers)
	for _, cl := range ctx.Clusters {
		files = append(files, cl.SourceFile)
	}
	for _, file := range files {
		// An unreadable or invalid file fails the render or POLICY050, which report it.
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var doc yaml.Node
		if yaml.Unmarshal(data, &doc) != nil {
			continue
		}
		for _, d := range keys {
			if k := keyNode(&doc, d.path); k != nil {
				out = append(out, Finding{
					RuleID: c.ID(), Severity: SevWarning, Element: d.element,
					Message: fmt.Sprintf("%s is deprecated and removed in %s: use %s instead", strings.Join(d.path, "."), d.removed, d.use),
					File:    file, Line: k.Line, Col: k.Column,
				})
			}
		}
	}
	return out
}

// walkKeys calls fn with every mapping key under n and its path, through mappings only.
func walkKeys(n *yaml.Node, path []string, fn func(key *yaml.Node, path []string)) {
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		p := append(slices.Clip(path), n.Content[i].Value)
		fn(n.Content[i], p)
		walkKeys(n.Content[i+1], p, fn)
	}
}

// keyNode returns the key node at path in doc, or nil.
func keyNode(doc *yaml.Node, path []string) *yaml.Node {
	parent := sourceloc.FindNode(doc, path[:len(path)-1]...)
	if parent == nil || parent.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == path[len(path)-1] {
			return parent.Content[i]
		}
	}
	return nil
}

// description is the comment block directly above key k at its indent, lines joined by spaces, as
// tools/doc-generator.py's comments() reads it. Empty when k is not first on its line.
func description(lines []string, k *yaml.Node) string {
	line := lines[k.Line-1]
	prefix := line[:min(k.Column-1, len(line))]
	if p := strings.TrimSpace(prefix); p != "" && p != "-" {
		return ""
	}
	indent := strings.ReplaceAll(prefix, "-", " ") + "#"
	var block []string
	for i := k.Line - 2; i >= 0 && strings.HasPrefix(lines[i], indent); i-- {
		if text := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(lines[i]), "#")); text != "" {
			block = append(block, text)
		}
	}
	slices.Reverse(block)
	return strings.Join(block, " ")
}
