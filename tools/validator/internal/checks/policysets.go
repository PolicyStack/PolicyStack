package checks

import (
	"fmt"
	"strconv"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// PolicySetCheck (POLICY040) verifies every name in policySets[].policies[] is a policy of the
// element: a policies key, a converters/ directory, or install. Disabled policies are accepted, since
// a values layer may turn them on.
type PolicySetCheck struct{}

func (PolicySetCheck) ID() string   { return "POLICY040" }
func (PolicySetCheck) Phase() Phase { return PhaseChart }

func (c *PolicySetCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	comp := ctx.Element.Values.Component
	var out []Finding
	for i, ps := range comp.PolicySets {
		for j, name := range ps.Policies {
			if _, ok := comp.Policies[name]; ok {
				continue
			}
			loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey,
				"policySets", strconv.Itoa(i), "policies", strconv.Itoa(j))
			out = append(out, Finding{
				RuleID:   c.ID(),
				Severity: SevError,
				Element:  ctx.Element.ChartName,
				Message: fmt.Sprintf("policySets[%d] %q references policy %q, which is not a policy of the element",
					i, ps.Name, name),
				File: ctx.Element.ValuesFile,
				Line: loc.Line, Col: loc.Col,
			})
		}
	}
	return out
}
