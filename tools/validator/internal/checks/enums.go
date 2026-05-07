package checks

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

var (
	allowedSeverity   = []string{"low", "medium", "high", "critical"}
	allowedRemed      = []string{"inform", "enforce"}
	allowedCompliance = []string{"musthave", "mustnothave", "mustonlyhave"}
	allowedUpgrade    = []string{"Automatic", "Manual", "None"}
)

// EnumCheck (POLICY030) validates the small set of enum-shaped fields ACM
// rejects when wrong: severity, remediationAction, complianceType,
// upgradeApproval. Catches typos before they reach a hub.
type EnumCheck struct{}

func (EnumCheck) ID() string  { return "POLICY030" }
func (EnumCheck) Phase() Phase { return PhaseChart }

func (c *EnumCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	comp := ctx.Element.Values.Component
	stackKey := ctx.Element.StackKey
	var out []Finding

	emit := func(field, value string, allowed []string, valuePath ...string) {
		if value == "" || slices.Contains(allowed, value) {
			return
		}
		loc := sourceloc.Find(ctx.Element.ValuesDoc, append([]string{"stack", stackKey}, valuePath...)...)
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevError,
			Element:  ctx.Element.ChartName,
			Message:  fmt.Sprintf("%s = %q; allowed: %v", field, value, allowed),
			File:     ctx.Element.ValuesFile,
			Line:     loc.Line, Col: loc.Col,
		})
	}

	if comp.DefaultPolicy != nil {
		emit("defaultPolicy.severity", comp.DefaultPolicy.Severity, allowedSeverity, "defaultPolicy", "severity")
		emit("defaultPolicy.remediationAction", comp.DefaultPolicy.RemediationAction, allowedRemed, "defaultPolicy", "remediationAction")
	}
	for i, p := range comp.Policies {
		emit("policies["+strconv.Itoa(i)+"].severity", p.Severity, allowedSeverity, "policies", strconv.Itoa(i), "severity")
		emit("policies["+strconv.Itoa(i)+"].remediationAction", p.RemediationAction, allowedRemed, "policies", strconv.Itoa(i), "remediationAction")
	}
	for i, p := range comp.ConfigPolicies {
		emit("configPolicies["+strconv.Itoa(i)+"].severity", p.Severity, allowedSeverity, "configPolicies", strconv.Itoa(i), "severity")
		emit("configPolicies["+strconv.Itoa(i)+"].remediationAction", p.RemediationAction, allowedRemed, "configPolicies", strconv.Itoa(i), "remediationAction")
		emit("configPolicies["+strconv.Itoa(i)+"].complianceType", p.ComplianceType, allowedCompliance, "configPolicies", strconv.Itoa(i), "complianceType")
	}
	for i, p := range comp.OperatorPolicies {
		emit("operatorPolicies["+strconv.Itoa(i)+"].severity", p.Severity, allowedSeverity, "operatorPolicies", strconv.Itoa(i), "severity")
		emit("operatorPolicies["+strconv.Itoa(i)+"].remediationAction", p.RemediationAction, allowedRemed, "operatorPolicies", strconv.Itoa(i), "remediationAction")
		emit("operatorPolicies["+strconv.Itoa(i)+"].complianceType", p.ComplianceType, allowedCompliance, "operatorPolicies", strconv.Itoa(i), "complianceType")
		emit("operatorPolicies["+strconv.Itoa(i)+"].upgradeApproval", p.UpgradeApproval, allowedUpgrade, "operatorPolicies", strconv.Itoa(i), "upgradeApproval")
	}
	for i, p := range comp.CertificatePolicies {
		emit("certificatePolicies["+strconv.Itoa(i)+"].severity", p.Severity, allowedSeverity, "certificatePolicies", strconv.Itoa(i), "severity")
		emit("certificatePolicies["+strconv.Itoa(i)+"].remediationAction", p.RemediationAction, allowedRemed, "certificatePolicies", strconv.Itoa(i), "remediationAction")
	}
	return out
}
