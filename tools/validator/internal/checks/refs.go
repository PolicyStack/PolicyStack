package checks

import (
	"fmt"
	"strconv"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// PolicyRefCheck (POLICY010) verifies every sub-policy's policyRef points
// to a parent name that (a) exists in policies[] and (b) is enabled.
// Distinguishes the two cases in the message.
type PolicyRefCheck struct{}

func (PolicyRefCheck) ID() string  { return "POLICY010" }
func (PolicyRefCheck) Phase() Phase { return PhaseChart }

func (c *PolicyRefCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	comp := ctx.Element.Values.Component

	policies := map[string]bool{}
	for _, p := range comp.Policies {
		if p.Name == "" {
			continue
		}
		policies[p.Name] = p.Enabled
	}

	var out []Finding
	emit := func(ref, sub, valuePath string, idx int) {
		if ref == "" {
			return
		}
		enabled, exists := policies[ref]
		if !exists {
			loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey, valuePath, strconv.Itoa(idx), "policyRef")
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevError,
				Element: ctx.Element.ChartName,
				Message: fmt.Sprintf("%s %q: policyRef %q is not defined in policies[]", valuePath, sub, ref),
				File:    ctx.Element.ValuesFile, Line: loc.Line, Col: loc.Col,
			})
			return
		}
		if !enabled {
			loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey, valuePath, strconv.Itoa(idx), "policyRef")
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevError,
				Element: ctx.Element.ChartName,
				Message: fmt.Sprintf("%s %q: policyRef %q exists but is disabled", valuePath, sub, ref),
				File:    ctx.Element.ValuesFile, Line: loc.Line, Col: loc.Col,
			})
		}
	}

	for i, p := range comp.ConfigPolicies {
		if !p.Enabled {
			continue
		}
		emit(p.PolicyRef, p.Name, "configPolicies", i)
	}
	for i, p := range comp.OperatorPolicies {
		if !p.Enabled {
			continue
		}
		emit(p.PolicyRef, p.Name, "operatorPolicies", i)
	}
	for i, p := range comp.CertificatePolicies {
		if !p.Enabled {
			continue
		}
		emit(p.PolicyRef, p.Name, "certificatePolicies", i)
	}
	return out
}

var _ = chart.Policy{}
