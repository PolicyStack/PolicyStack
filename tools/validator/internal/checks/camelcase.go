package checks

import (
	"fmt"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// CamelCaseCheck (POLICY090) ensures the single key under `stack:` in
// values.yaml matches the camelCase form of Chart.yaml's `name:`.
// A mismatch causes policy-library to silently render zero policies.
type CamelCaseCheck struct{}

func (CamelCaseCheck) ID() string  { return "POLICY090" }
func (CamelCaseCheck) Phase() Phase { return PhaseChart }

func (c *CamelCaseCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.StackKey == "" {
		return nil
	}
	want := chart.CamelFromKebab(ctx.Element.ChartName)
	if ctx.Element.StackKey == want {
		return nil
	}
	loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey)
	return []Finding{{
		RuleID:   c.ID(),
		Severity: SevError,
		Element:  ctx.Element.ChartName,
		Message: fmt.Sprintf("stack key %q does not match camelCase form of Chart.yaml name %q (expected %q)",
			ctx.Element.StackKey, ctx.Element.ChartName, want),
		File: ctx.Element.ValuesFile,
		Line: loc.Line, Col: loc.Col,
	}}
}
