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

// NameLengthCheck (POLICY001) enforces the 63-char limit on every name
// the policy-library chart produces. Formulas verified against the chart
// templates in PolicyStack-chart `_policies.tpl` / `_policy-sets.tpl`:
//
//	Policy.metadata.name        = <value-name>-<release-name>
//	Policy (replicated by ACM)  = <ns>.<value-name>-<release-name>   <-- bound
//	PolicySet.metadata.name     = <value-name>-<release-name>
//	PolicySet (replicated)      = <ns>.<value-name>-<release-name>   <-- bound
//	ConfigurationPolicy/
//	OperatorPolicy/
//	CertificatePolicy           = <parent-value-name>-<sub-value-name>  <-- bound
//
// Sub-policies are embedded objectDefinitions inside the parent Policy spec;
// they do NOT carry the namespace prefix or Release.Name. The 63-char rule
// is the k8s DNS-1123 label limit which ACM enforces on the propagated
// (replicated) policy name on each managed cluster.
//
// release-name = <chart-name>-<cluster> (set by appset.yaml:26).
type NameLengthCheck struct{}

func (NameLengthCheck) ID() string  { return "POLICY001" }
func (NameLengthCheck) Phase() Phase { return PhaseCluster }

func (c *NameLengthCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Cluster == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	ns := ctx.Element.Values.PolicyNamespace
	rel := ctx.Cluster.ReleaseName
	comp := ctx.Element.Values.Component

	var out []Finding

	// Parent Policy / PolicySet: ACM-replicated name <ns>.<name>-<rel> ≤ 63.
	emitReplicated := func(name, kind, valuePath string, idx int) {
		hubName := fmt.Sprintf("%s-%s", name, rel)
		replicated := fmt.Sprintf("%s.%s", ns, hubName)
		if len(replicated) <= MaxPolicyNameLen {
			return
		}
		loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey, valuePath, strconv.Itoa(idx), "name")
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevError,
			Element:  ctx.Element.ChartName,
			Cluster:  ctx.Cluster.ClusterName,
			Message: fmt.Sprintf("%s %q renders metadata.name=%q (%d chars); ACM-replicated name %q is %d chars > %d limit",
				kind, name, hubName, len(hubName), replicated, len(replicated), MaxPolicyNameLen),
			File: ctx.Element.ValuesFile,
			Line: loc.Line, Col: loc.Col,
		})
	}

	// Sub-policy: literal metadata.name = <parent>-<sub> ≤ 63.
	emitSub := func(parent, sub, kind, valuePath string, idx int) {
		full := fmt.Sprintf("%s-%s", parent, sub)
		if len(full) <= MaxPolicyNameLen {
			return
		}
		loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey, valuePath, strconv.Itoa(idx), "name")
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevError,
			Element:  ctx.Element.ChartName,
			Cluster:  ctx.Cluster.ClusterName,
			Message: fmt.Sprintf("%s %q renders metadata.name=%q (%d chars > %d limit)",
				kind, sub, full, len(full), MaxPolicyNameLen),
			File: ctx.Element.ValuesFile,
			Line: loc.Line, Col: loc.Col,
		})
	}

	for i, p := range comp.Policies {
		if p.Name == "" {
			continue
		}
		emitReplicated(p.Name, "Policy", "policies", i)
	}
	for i, p := range comp.PolicySets {
		if p.Name == "" {
			continue
		}
		emitReplicated(p.Name, "PolicySet", "policySets", i)
	}
	for i, p := range comp.ConfigPolicies {
		if p.Name == "" {
			continue
		}
		emitSub(p.PolicyRef, p.Name, "ConfigurationPolicy", "configPolicies", i)
	}
	for i, p := range comp.OperatorPolicies {
		if p.Name == "" {
			continue
		}
		emitSub(p.PolicyRef, p.Name, "OperatorPolicy", "operatorPolicies", i)
	}
	for i, p := range comp.CertificatePolicies {
		if p.Name == "" {
			continue
		}
		emitSub(p.PolicyRef, p.Name, "CertificatePolicy", "certificatePolicies", i)
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
		check(fmt.Sprintf("%s-%s", p.PolicyRef, p.Name), "configPolicies["+strconv.Itoa(i)+"].name", "configPolicies", i)
	}
	for i, p := range comp.OperatorPolicies {
		if p.Name == "" {
			continue
		}
		check(fmt.Sprintf("%s-%s", p.PolicyRef, p.Name), "operatorPolicies["+strconv.Itoa(i)+"].name", "operatorPolicies", i)
	}
	for i, p := range comp.CertificatePolicies {
		if p.Name == "" {
			continue
		}
		check(fmt.Sprintf("%s-%s", p.PolicyRef, p.Name), "certificatePolicies["+strconv.Itoa(i)+"].name", "certificatePolicies", i)
	}
	return out
}

// elementsByCluster returns nothing here — POLICY002 cross-element collision
// detection is performed at the runner level since it needs a global view.
// We keep the per-element pass above for fast, deterministic intra-element
// duplicate detection.
var _ = chart.Element{}
