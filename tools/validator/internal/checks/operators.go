package checks

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// csvName matches a DNS-1123 subdomain, the format of a ClusterServiceVersion name.
var csvName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// operatorFindings checks the fields of operatorPolicies.<key> that ACM rejects or policy-library
// ignores. file is the element's values.yaml or a values layer, and doc is its parsed tree.
func operatorFindings(rule string, el *chart.Element, file string, doc *yaml.Node, key string, op chart.OperatorPolicy) []Finding {
	var out []Finding
	add := func(sev Severity, msg string, sub ...string) {
		loc := sourceloc.Find(doc, append([]string{"stack", el.StackKey, "operatorPolicies", key}, sub...)...)
		out = append(out, Finding{
			RuleID: rule, Severity: sev,
			Element: el.ChartName,
			Message: "operatorPolicies." + key + msg,
			File:    file, Line: loc.Line, Col: loc.Col,
		})
	}
	csv := func(field, v string, sub ...string) {
		if v != "" && (len(v) > 253 || !csvName.MatchString(v)) {
			add(SevError, fmt.Sprintf(".%s: %q is not a valid CSV name (DNS-1123 subdomain)", field, v), sub...)
		}
	}
	enum := func(field, v string, allowed []string) {
		if v != "" && !slices.Contains(allowed, v) {
			add(SevError, fmt.Sprintf(".%s = %q; allowed: %v", field, v, allowed), field)
		}
	}

	if op.HasName {
		add(SevWarning, ".name is ignored; the map key is the name", "name")
	}
	enum("severity", op.Severity, allowedSeverity)
	enum("remediationAction", op.RemediationAction, allowedRemed)
	enum("complianceType", op.ComplianceType, allowedCompliance)
	enum("upgradeApproval", op.UpgradeApproval, allowedUpgrade)
	if op.Subscription.InstallPlanApproval != "" {
		add(SevError, ".subscription.installPlanApproval: ACM rejects it; use upgradeApproval and versions", "subscription", "installPlanApproval")
	}
	// versions entries may be comma-separated (ACM 2.14+) or templated (2.13+); startingCSV is one name.
	for i, value := range op.Versions {
		if strings.Contains(value, "{{") {
			continue
		}
		for v := range strings.SplitSeq(value, ",") {
			csv("versions["+strconv.Itoa(i)+"]", strings.TrimSpace(v), "versions", strconv.Itoa(i))
		}
	}
	if !strings.Contains(op.Subscription.StartingCSV, "{{") {
		csv("subscription.startingCSV", op.Subscription.StartingCSV, "subscription", "startingCSV")
	}
	return out
}
