package checks

import (
	"fmt"
	"maps"
	"os"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// layerRootKeys are the top-level keys a values layer may set: what the elements and lifecycle read.
// version and channel are allowed here because POLICY034 reports them. componentKeys are the keys
// policy-library's schema allows under stack.<element> (definitions.component); TestComponentKeys
// compares them with the vendored archive.
var (
	layerRootKeys   = []string{"policyNamespace", "stack", "global", "placement", "install", "version", "channel"}
	layerGlobalKeys = []string{"remediationAction", "catalogSources"}
	componentKeys   = []string{
		"enabled", "default", "config", "policies", "operatorPolicies", "policySets", "suffixTemplateNames",
		"usePolicySetsPlacements", "disablePlacements",
	}
)

// LayerKeyCheck (POLICY035) reports keys in a values layer that nothing reads: an unknown top-level
// or global key, a stack key that names no element, an element key policy-library does not accept, a
// policies key that names no policy of its element, a templates key that names no template of its
// policy and sets no file, and a config key the element's values.yaml does not define. Helm merges them without complaint and the setting silently has no effect. An unknown
// element key fails the schema instead, but RENDER000 reports it once per element at the element's
// values.yaml, not at the layer. It runs on every layer file, used by a fleet file or not.
type LayerKeyCheck struct{}

func (LayerKeyCheck) ID() string   { return "POLICY035" }
func (LayerKeyCheck) Phase() Phase { return PhaseRepo }

func (c *LayerKeyCheck) Run(ctx Context) []Finding {
	elements := map[string]*chart.Element{}
	for _, el := range ctx.AllElements {
		if el.StackKey != "" {
			elements[el.StackKey] = el
		}
	}
	var out []Finding
	for _, file := range ctx.Layers {
		// An unreadable or invalid layer fails the render, which RENDER000 reports.
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var doc yaml.Node
		if yaml.Unmarshal(data, &doc) != nil {
			continue
		}
		add := func(n *yaml.Node, el, msg string) {
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevError, Element: el, Message: msg,
				File: file, Line: n.Line, Col: n.Column,
			})
		}
		unknown := func(path []string, allowed []string, msg func(k string) string, el string) {
			n := sourceloc.FindNode(&doc, path...)
			if n == nil || n.Kind != yaml.MappingNode {
				return
			}
			for i := 0; i+1 < len(n.Content); i += 2 {
				if k := n.Content[i]; !slices.Contains(allowed, k.Value) {
					add(k, el, msg(k.Value))
				}
			}
		}
		unknown(nil, layerRootKeys, func(k string) string {
			return fmt.Sprintf("top-level key %q is read by nothing", k)
		}, "")
		unknown([]string{"global"}, layerGlobalKeys, func(k string) string {
			return fmt.Sprintf("global.%s is read by nothing", k)
		}, "")

		stack := sourceloc.FindNode(&doc, "stack")
		if stack == nil || stack.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(stack.Content); i += 2 {
			key := stack.Content[i]
			el, ok := elements[key.Value]
			if !ok {
				add(key, "", fmt.Sprintf("stack.%s names no element", key.Value))
				continue
			}
			unknown([]string{"stack", key.Value}, componentKeys, func(k string) string {
				return fmt.Sprintf("stack.%s.%s is not a policy-library component key", key.Value, k)
			}, el.ChartName)
			pols := el.Values.Component.Policies
			unknown([]string{"stack", key.Value, "policies"}, slices.Collect(maps.Keys(pols)), func(k string) string {
				return fmt.Sprintf("stack.%s.policies.%s names no policy of the element", key.Value, k)
			}, el.ChartName)
			for name, p := range pols.Sorted() {
				path := []string{"stack", key.Value, "policies", name, "templates"}
				unknown(path, append(slices.Collect(maps.Keys(p.Templates)), filed(sourceloc.FindNode(&doc, path...))...), func(k string) string {
					return fmt.Sprintf("stack.%s.policies.%s.templates.%s names no template of the policy: add converters/%s/%s.yaml or set file", key.Value, name, k, name, k)
				}, el.ChartName)
			}
			unknown([]string{"stack", key.Value, "config"}, configKeys(el), func(k string) string {
				return fmt.Sprintf("stack.%s.config.%s is not in the element's values.yaml", key.Value, k)
			}, el.ChartName)
		}
	}
	return out
}

// filed returns the keys of a templates map whose entry sets file: a layer may add such a template.
func filed(templates *yaml.Node) []string {
	var out []string
	if templates == nil || templates.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(templates.Content); i += 2 {
		if sourceloc.FindNode(templates.Content[i+1], "file") != nil {
			out = append(out, templates.Content[i].Value)
		}
	}
	return out
}

// configKeys returns the top-level config keys in the element's values.yaml.
func configKeys(el *chart.Element) []string {
	return mapKeys(sourceloc.FindNode(el.ValuesDoc, "stack", el.StackKey, "config"))
}

// mapKeys returns the keys of a mapping node, nil for any other node.
func mapKeys(n *yaml.Node) []string {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	var keys []string
	for i := 0; i < len(n.Content); i += 2 {
		keys = append(keys, n.Content[i].Value)
	}
	return keys
}
