package checks

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// DeprecationCheck (POLICY092) warns where an element whose Chart.yaml sets deprecated: true
// is turned on: stack.<key>.enabled: true in a values layer, or in the element's own values.yaml,
// which turns it on for every cluster. helm template prints the flag only as a warning on stderr,
// which Argo CD discards. It runs on every layer file, used by a fleet file or not.
type DeprecationCheck struct{}

func (DeprecationCheck) ID() string   { return "POLICY092" }
func (DeprecationCheck) Phase() Phase { return PhaseRepo }

func (c *DeprecationCheck) Run(ctx Context) []Finding {
	var deprecated []*chart.Element
	for _, el := range ctx.AllElements {
		if el.Deprecated && el.StackKey != "" {
			deprecated = append(deprecated, el)
		}
	}
	if len(deprecated) == 0 {
		return nil
	}
	var out []Finding
	check := func(file string, doc *yaml.Node, el *chart.Element) {
		n := sourceloc.FindNode(doc, "stack", el.StackKey, "enabled")
		var on bool
		if n == nil || n.Decode(&on) != nil || !on {
			return
		}
		out = append(out, Finding{
			RuleID: c.ID(), Severity: SevWarning, Element: el.ChartName,
			Message: fmt.Sprintf("turns on element %q, which its Chart.yaml marks deprecated: it is removed in a later major, so move to its replacement and turn it off", el.ChartName),
			File:    file, Line: n.Line, Col: n.Column,
		})
	}
	for _, el := range deprecated {
		check(el.ValuesFile, el.ValuesDoc, el)
	}
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
		for _, el := range deprecated {
			check(file, &doc, el)
		}
	}
	return out
}
