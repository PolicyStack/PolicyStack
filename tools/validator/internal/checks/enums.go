package checks

import (
	"fmt"
	"slices"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

var (
	allowedSeverity   = []string{"low", "medium", "high", "critical"}
	allowedRemed      = []string{"inform", "enforce"}
	allowedCompliance = []string{"musthave", "mustnothave", "mustonlyhave"}
	allowedUpgrade    = []string{"None", "Automatic"}

	// A template's own action is InformOnly or nothing: otherwise it takes its Policy's.
	allowedTemplateRemed = []string{"InformOnly"}
)

// EnumCheck (POLICY030) validates the enum fields ACM rejects when invalid:
// severity, remediationAction, complianceType and upgradeApproval, plus the other
// operatorPolicies fields operatorFindings covers. A template's remediationAction is InformOnly or
// unset.
type EnumCheck struct{}

func (EnumCheck) ID() string   { return "POLICY030" }
func (EnumCheck) Phase() Phase { return PhaseChart }

func (c *EnumCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	comp := ctx.Element.Values.Component
	stackKey := ctx.Element.StackKey
	var out []Finding

	emit := func(value string, allowed []string, valuePath ...string) {
		if value == "" || slices.Contains(allowed, value) {
			return
		}
		loc := sourceloc.Find(ctx.Element.ValuesDoc, append([]string{"stack", stackKey}, valuePath...)...)
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevError,
			Element:  ctx.Element.ChartName,
			Message:  fmt.Sprintf("%s = %q; allowed: %v", strings.Join(valuePath, "."), value, allowed),
			File:     ctx.Element.ValuesFile,
			Line:     loc.Line, Col: loc.Col,
		})
	}

	// policy-library reads `default`, not `defaultPolicy`; POLICY031 reports the legacy spelling.
	if comp.Default != nil {
		emit(comp.Default.Severity, allowedSeverity, "default", "severity")
		emit(comp.Default.RemediationAction, allowedRemed, "default", "remediationAction")
	}
	for _, p := range comp.Policies.Sorted() {
		emit(p.Severity, allowedSeverity, append(slices.Clone(p.Path), "severity")...)
		emit(p.RemediationAction, allowedRemed, append(slices.Clone(p.Path), "remediationAction")...)
		for _, t := range p.Templates.Sorted() {
			emit(t.Severity, allowedSeverity, append(slices.Clone(t.Path), "severity")...)
			emit(t.RemediationAction, allowedTemplateRemed, append(slices.Clone(t.Path), "remediationAction")...)
			emit(t.ComplianceType, allowedCompliance, append(slices.Clone(t.Path), "complianceType")...)
		}
	}
	for k, p := range comp.OperatorPolicies.Sorted() {
		out = append(out, operatorFindings(c.ID(), ctx.Element, ctx.Element.ValuesFile, ctx.Element.ValuesDoc, k, p)...)
	}
	return out
}
