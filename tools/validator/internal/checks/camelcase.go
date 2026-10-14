package checks

import (
	"fmt"
	"slices"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// CamelCaseCheck (POLICY090) checks that the key under `stack:` in
// values.yaml is the camelCase form of Chart.yaml's `name:`. On a mismatch
// policy-library renders no policies and reports no error. A chart with
// engines (lifecycle/) instead needs exactly one stack key per engine.
type CamelCaseCheck struct{}

func (CamelCaseCheck) ID() string   { return "POLICY090" }
func (CamelCaseCheck) Phase() Phase { return PhaseChart }

func (c *CamelCaseCheck) Run(ctx Context) []Finding {
	el := ctx.Element
	if el == nil || el.StackKey == "" {
		return nil
	}
	finding := func(msg string, path ...string) Finding {
		loc := sourceloc.Find(el.ValuesDoc, path...)
		return Finding{RuleID: c.ID(), Severity: SevError, Element: el.ChartName, Message: msg, File: el.ValuesFile, Line: loc.Line, Col: loc.Col}
	}
	if el.Engines != nil {
		var out []Finding
		var keys []string
		for _, c := range el.Components {
			keys = append(keys, c.Key)
		}
		for _, k := range keys {
			if !slices.Contains(el.Engines, k) {
				out = append(out, finding(fmt.Sprintf("stack key %q is not an install.engine in values.schema.json %v, so it never renders", k, el.Engines), "stack", k))
			}
		}
		for _, e := range el.Engines {
			if !slices.Contains(keys, e) {
				out = append(out, finding(fmt.Sprintf("install.engine %q in values.schema.json has no stack.%s component", e, e), "stack"))
			}
		}
		return out
	}
	want := chart.CamelFromKebab(el.ChartName)
	if el.StackKey == want {
		return nil
	}
	return []Finding{finding(fmt.Sprintf("stack key %q does not match camelCase form of Chart.yaml name %q (expected %q)",
		el.StackKey, el.ChartName, want), "stack", el.StackKey)}
}
