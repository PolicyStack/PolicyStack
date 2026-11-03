package checks

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// defaultCatalogs are the CatalogSources OpenShift ships in openshift-marketplace.
var defaultCatalogs = []string{"redhat-operators", "certified-operators", "community-operators", "redhat-marketplace"}

// SourceCheck (POLICY038) reports an operatorPolicies subscription.source that is neither a default
// catalog nor a global.catalogSources key in any values layer. OLM finds no such CatalogSource, so
// the operator never installs. It checks element values.yaml files and the layers that override them.
type SourceCheck struct{}

func (SourceCheck) ID() string   { return "POLICY038" }
func (SourceCheck) Phase() Phase { return PhaseRepo }

func (c *SourceCheck) Run(ctx Context) []Finding {
	allowed := slices.Clone(defaultCatalogs)
	type layer struct {
		file string
		doc  *yaml.Node
	}
	var layers []layer
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
		var v struct {
			Global struct {
				CatalogSources map[string]any `yaml:"catalogSources"`
			} `yaml:"global"`
		}
		_ = doc.Decode(&v)
		allowed = slices.AppendSeq(allowed, maps.Keys(v.Global.CatalogSources))
		layers = append(layers, layer{file, &doc})
	}

	var out []Finding
	check := func(el *chart.Element, file string, doc *yaml.Node, ops chart.OperatorPolicies) {
		for k, op := range ops.Sorted() {
			src := op.Subscription.Source
			if src == "" || strings.Contains(src, "{{") || slices.Contains(allowed, src) {
				continue
			}
			loc := sourceloc.Find(doc, "stack", el.StackKey, "operatorPolicies", k, "subscription", "source")
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevError, Element: el.ChartName,
				Message: fmt.Sprintf("operatorPolicies.%s.subscription.source %q is not a default catalog (%s) or a global.catalogSources key",
					k, src, strings.Join(defaultCatalogs, ", ")),
				File: file, Line: loc.Line, Col: loc.Col,
			})
		}
	}
	for _, el := range ctx.AllElements {
		if el.StackKey == "" || el.Values == nil || el.Values.Component == nil {
			continue
		}
		check(el, el.ValuesFile, el.ValuesDoc, el.Values.Component.OperatorPolicies)
		for _, l := range layers {
			var ops chart.OperatorPolicies
			if n := sourceloc.FindNode(l.doc, "stack", el.StackKey, "operatorPolicies"); n != nil && n.Decode(&ops) == nil {
				check(el, l.file, l.doc, ops)
			}
		}
	}
	return out
}
