package checks

import (
	"fmt"
	"strconv"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// PolicySetCheck (POLICY040) verifies every name listed in
// policySets[].policies[] is defined in policies[]. Disabled parents are OK
// here — ACM treats disabled policies as still-present resources.
type PolicySetCheck struct{}

func (PolicySetCheck) ID() string  { return "POLICY040" }
func (PolicySetCheck) Phase() Phase { return PhaseChart }

func (c *PolicySetCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	comp := ctx.Element.Values.Component
	defined := map[string]struct{}{}
	for _, p := range comp.Policies {
		if p.Name != "" {
			defined[p.Name] = struct{}{}
		}
	}
	var out []Finding
	for i, ps := range comp.PolicySets {
		for j, name := range ps.Policies {
			if _, ok := defined[name]; ok {
				continue
			}
			loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey,
				"policySets", strconv.Itoa(i), "policies", strconv.Itoa(j))
			out = append(out, Finding{
				RuleID:   c.ID(),
				Severity: SevError,
				Element:  ctx.Element.ChartName,
				Message: fmt.Sprintf("policySets[%d] %q references policy %q which is not defined in policies[]",
					i, ps.Name, name),
				File: ctx.Element.ValuesFile,
				Line: loc.Line, Col: loc.Col,
			})
		}
	}
	return out
}
