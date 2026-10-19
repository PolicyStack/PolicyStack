package checks

import (
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// layerRootKeys are the top-level keys a values layer may set: what the elements and lifecycle read.
// version and channel are allowed here because POLICY034 reports them.
var (
	layerRootKeys   = []string{"policyNamespace", "stack", "global", "placement", "install", "version", "channel"}
	layerGlobalKeys = []string{"remediationAction", "catalogSources"}
)

// LayerKeyCheck (POLICY035) reports keys in a values layer that nothing reads: an unknown top-level
// or global key, a stack key that names no element, a toggle that names no entry of its element, and
// a config key the element's values.yaml does not define. Helm merges them without complaint and the
// setting silently has no effect. It runs on every layer file, used by a fleet file or not.
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
			key, comp := stack.Content[i], stack.Content[i+1]
			el, ok := elements[key.Value]
			if !ok {
				add(key, "", fmt.Sprintf("stack.%s names no element", key.Value))
				continue
			}
			unknown([]string{"stack", key.Value, "toggles"}, entryNames(el, comp), func(k string) string {
				return fmt.Sprintf("stack.%s.toggles.%s names no entry of the element", key.Value, k)
			}, el.ChartName)
			unknown([]string{"stack", key.Value, "config"}, configKeys(el), func(k string) string {
				return fmt.Sprintf("stack.%s.config.%s is not in the element's values.yaml", key.Value, k)
			}, el.ChartName)
		}
	}
	return out
}

// entryNames returns the names a toggle can switch: the element's entries plus any the layer itself
// declares, since a layer may restate a list.
func entryNames(el *chart.Element, layer *yaml.Node) []string {
	var names []string
	add := func(c *chart.Component) {
		if c == nil {
			return
		}
		for _, p := range c.Policies {
			names = append(names, p.Name)
		}
		for _, p := range c.ConfigPolicies {
			names = append(names, p.Name)
		}
		for k := range c.OperatorPolicies {
			names = append(names, k)
		}
		for _, p := range c.CertificatePolicies {
			names = append(names, p.Name)
		}
		for _, p := range c.PolicySets {
			names = append(names, p.Name)
		}
	}
	if el.Values != nil {
		add(el.Values.Component)
	}
	var own chart.Component
	if layer.Decode(&own) == nil {
		add(&own)
	}
	return names
}

// configKeys returns the top-level config keys in the element's values.yaml.
func configKeys(el *chart.Element) []string {
	n := sourceloc.FindNode(el.ValuesDoc, "stack", el.StackKey, "config")
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	var keys []string
	for i := 0; i < len(n.Content); i += 2 {
		keys = append(keys, n.Content[i].Value)
	}
	return keys
}
