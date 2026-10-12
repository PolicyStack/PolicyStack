package checks

import (
	"fmt"
	"strconv"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// PolicyRefCheck (POLICY010) verifies every enabled sub-policy's policyRef
// names a parent that exists in policies[] and is enabled. The two failures
// are reported with different messages.
type PolicyRefCheck struct{}

func (PolicyRefCheck) ID() string   { return "POLICY010" }
func (PolicyRefCheck) Phase() Phase { return PhaseChart }

func (c *PolicyRefCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	comp := ctx.Element.Values.Component

	policies := map[string]bool{}
	// Parents governed by a toggle are exempt from "exists but is disabled": shipping a
	// sub-feature off and enabling it per cluster is the intended pattern.
	toggled := map[string]bool{}
	for _, p := range comp.Policies {
		if p.Name == "" {
			continue
		}
		policies[p.Name] = comp.IsEnabled(p.Name, p.Enabled)
		if _, ok := comp.Toggles[p.Name]; ok {
			toggled[p.Name] = true
		}
	}

	var out []Finding
	emit := func(ref, sub, valuePath, idx string) {
		if ref == "" {
			return
		}
		enabled, exists := policies[ref]
		if !exists {
			loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey, valuePath, idx, "policyRef")
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevError,
				Element: ctx.Element.ChartName,
				Message: fmt.Sprintf("%s %q: policyRef %q is not defined in policies[]", valuePath, sub, ref),
				File:    ctx.Element.ValuesFile, Line: loc.Line, Col: loc.Col,
			})
			return
		}
		if !enabled {
			if toggled[ref] {
				return
			}
			loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey, valuePath, idx, "policyRef")
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevError,
				Element: ctx.Element.ChartName,
				Message: fmt.Sprintf("%s %q: policyRef %q exists but is disabled", valuePath, sub, ref),
				File:    ctx.Element.ValuesFile, Line: loc.Line, Col: loc.Col,
			})
		}
	}

	for i, p := range comp.ConfigPolicies {
		if !comp.IsEnabled(p.Name, p.Enabled) {
			continue
		}
		emit(p.PolicyRef, p.Name, "configPolicies", strconv.Itoa(i))
	}
	for k, p := range comp.OperatorPolicies.Sorted() {
		if !comp.IsEnabled(p.Name, p.Enabled) {
			continue
		}
		emit(p.PolicyRef, p.Name, "operatorPolicies", k)
	}
	for i, p := range comp.CertificatePolicies {
		if !comp.IsEnabled(p.Name, p.Enabled) {
			continue
		}
		emit(p.PolicyRef, p.Name, "certificatePolicies", strconv.Itoa(i))
	}
	return out
}

var _ = chart.Policy{}
