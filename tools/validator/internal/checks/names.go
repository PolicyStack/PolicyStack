package checks

import (
	"fmt"
	"strconv"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// MaxPolicyNameLen is the k8s DNS-1123 label limit applied to
// `<policyNamespace>.<policy-name>`. ACM rejects anything longer.
const MaxPolicyNameLen = 63

// NameLengthCheck (POLICY001) enforces the 63-char rule for every rendered
// policy and sub-policy name produced for a given (element, cluster).
//
// Formulas (from policy-library _policies.tpl + appset.yaml:26):
//
//   Policy:        <ns>.<value-name>-<release-name>
//   Sub-policy:    <ns>.<parent-name>-<sub-name>-<release-name>
//   release-name = <element-chart-name>-<cluster>
type NameLengthCheck struct{}

func (NameLengthCheck) ID() string { return "POLICY001" }
func (NameLengthCheck) Phase() Phase { return PhaseCluster }

func (c *NameLengthCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Cluster == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	ns := ctx.Element.Values.PolicyNamespace
	rel := ctx.Cluster.ReleaseName
	comp := ctx.Element.Values.Component

	var out []Finding
	emit := func(name, kind, valuePath string, idx int) {
		full := fmt.Sprintf("%s.%s-%s", ns, name, rel)
		if len(full) <= MaxPolicyNameLen {
			return
		}
		loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey, valuePath, strconv.Itoa(idx), "name")
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevError,
			Element:  ctx.Element.ChartName,
			Cluster:  ctx.Cluster.ClusterName,
			Message: fmt.Sprintf("%s %q would render as %q (%d chars > %d limit)",
				kind, name, full, len(full), MaxPolicyNameLen),
			File: ctx.Element.ValuesFile,
			Line: loc.Line,
			Col:  loc.Col,
		})
	}

	for i, p := range comp.Policies {
		if p.Name == "" {
			continue
		}
		emit(p.Name, "Policy", "policies", i)
	}
	for i, p := range comp.ConfigPolicies {
		if p.Name == "" {
			continue
		}
		emit(p.PolicyRef+"-"+p.Name, "ConfigurationPolicy", "configPolicies", i)
	}
	for i, p := range comp.OperatorPolicies {
		if p.Name == "" {
			continue
		}
		emit(p.PolicyRef+"-"+p.Name, "OperatorPolicy", "operatorPolicies", i)
	}
	for i, p := range comp.CertificatePolicies {
		if p.Name == "" {
			continue
		}
		emit(p.PolicyRef+"-"+p.Name, "CertificatePolicy", "certificatePolicies", i)
	}
	for i, p := range comp.PolicySets {
		if p.Name == "" {
			continue
		}
		emit(p.Name, "PolicySet", "policySets", i)
	}
	return out
}

// DuplicateNameCheck (POLICY002) catches two policies (across all elements
// in the same policyNamespace) that would render to the same metadata.name.
type DuplicateNameCheck struct{}

func (DuplicateNameCheck) ID() string { return "POLICY002" }
func (DuplicateNameCheck) Phase() Phase { return PhaseCluster }

func (c *DuplicateNameCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Cluster == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	rel := ctx.Cluster.ReleaseName
	comp := ctx.Element.Values.Component
	seen := map[string]string{}
	var out []Finding

	check := func(rendered, source, valuePath string, idx int) {
		if prior, ok := seen[rendered]; ok {
			loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey, valuePath, strconv.Itoa(idx), "name")
			out = append(out, Finding{
				RuleID:   "POLICY002",
				Severity: SevError,
				Element:  ctx.Element.ChartName,
				Cluster:  ctx.Cluster.ClusterName,
				Message:  fmt.Sprintf("rendered name %q from %s collides with prior %s in same element", rendered, source, prior),
				File:     ctx.Element.ValuesFile,
				Line:     loc.Line,
				Col:      loc.Col,
			})
			return
		}
		seen[rendered] = source
	}

	for i, p := range comp.Policies {
		if p.Name == "" {
			continue
		}
		check(fmt.Sprintf("%s-%s", p.Name, rel), "policies["+strconv.Itoa(i)+"].name", "policies", i)
	}
	for i, p := range comp.ConfigPolicies {
		if p.Name == "" {
			continue
		}
		check(fmt.Sprintf("%s-%s-%s", p.PolicyRef, p.Name, rel), "configPolicies["+strconv.Itoa(i)+"].name", "configPolicies", i)
	}
	for i, p := range comp.OperatorPolicies {
		if p.Name == "" {
			continue
		}
		check(fmt.Sprintf("%s-%s-%s", p.PolicyRef, p.Name, rel), "operatorPolicies["+strconv.Itoa(i)+"].name", "operatorPolicies", i)
	}
	for i, p := range comp.CertificatePolicies {
		if p.Name == "" {
			continue
		}
		check(fmt.Sprintf("%s-%s-%s", p.PolicyRef, p.Name, rel), "certificatePolicies["+strconv.Itoa(i)+"].name", "certificatePolicies", i)
	}
	return out
}

// elementsByCluster returns nothing here — POLICY002 cross-element collision
// detection is performed at the runner level since it needs a global view.
// We keep the per-element pass above for fast, deterministic intra-element
// duplicate detection.
var _ = chart.Element{}
