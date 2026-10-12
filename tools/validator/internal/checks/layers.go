package checks

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

const legacyOperatorPolicies = "operatorPolicies is a list; policy-library 3.x requires a map keyed by entry name. Move each entry under its name and drop `name:`"

// LayerOperatorCheck (POLICY032) checks stack.<key>.operatorPolicies in each values layer of the
// cascade. Maps merge, so a layer sets only the fields it changes, and policy-library fails on an
// entry the element does not define. Findings carry no cluster, so run.Run reports each once.
type LayerOperatorCheck struct{}

func (LayerOperatorCheck) ID() string   { return "POLICY032" }
func (LayerOperatorCheck) Phase() Phase { return PhaseCluster }

func (c *LayerOperatorCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Cluster == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	el := ctx.Element
	var out []Finding
	for _, file := range ctx.Cluster.ValueFiles {
		if file == el.ValuesFile {
			continue
		}
		// An unreadable or invalid layer fails the render, which RENDER000 reports.
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var doc yaml.Node
		if yaml.Unmarshal(data, &doc) != nil {
			continue
		}
		path := []string{"stack", el.StackKey, "operatorPolicies"}
		n := sourceloc.FindNode(&doc, path...)
		if n == nil {
			continue
		}
		add := func(msg string) {
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevError,
				Element: el.ChartName, Message: msg,
				File: file, Line: n.Line, Col: n.Column,
			})
		}
		if chart.IsLegacyOperatorPolicies(n) {
			add(legacyOperatorPolicies)
			continue
		}
		var ops chart.OperatorPolicies
		if err := n.Decode(&ops); err != nil {
			add(fmt.Sprintf("operatorPolicies: %v", err))
			continue
		}
		for k, op := range ops.Sorted() {
			if _, ok := el.Values.Component.OperatorPolicies[k]; !ok {
				loc := sourceloc.Find(&doc, append(path, k)...)
				out = append(out, Finding{
					RuleID: c.ID(), Severity: SevError,
					Element: el.ChartName,
					Message: fmt.Sprintf("operatorPolicies.%s is not in the element's values.yaml; a layer can only override an existing entry", k),
					File:    file, Line: loc.Line, Col: loc.Col,
				})
				continue
			}
			out = append(out, operatorFindings(c.ID(), el, file, &doc, k, op)...)
		}
	}
	return out
}
