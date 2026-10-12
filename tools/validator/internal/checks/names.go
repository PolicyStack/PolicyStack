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
// policy-library produces. Formulas from PolicyStack-chart `_policies.tpl`
// and `_policy-sets.tpl`:
//
//	Policy.metadata.name        = <value-name>-<release-name>
//	Policy (replicated by ACM)  = <ns>.<value-name>-<release-name>   <-- bound
//	PolicySet.metadata.name     = <value-name>-<release-name>
//	PolicySet (replicated)      = <ns>.<value-name>-<release-name>   <-- bound
//	ConfigurationPolicy/
//	OperatorPolicy/
//	CertificatePolicy           = <parent-value-name>-<sub-value-name>  <-- bound
//
// Sub-policies are objectDefinitions embedded in the parent Policy spec and
// carry neither the namespace prefix nor Release.Name. ACM enforces the
// DNS-1123 label limit on the replicated policy name on each managed cluster.
//
// release-name = <chart-name>-<cluster>, set by appset.yaml.
type NameLengthCheck struct{}

func (NameLengthCheck) ID() string   { return "POLICY001" }
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
	// path locates the entry's name: <list>, <i>, name, or operatorPolicies, <key>.
	emitSub := func(parent, sub, kind string, path ...string) {
		full := fmt.Sprintf("%s-%s", parent, sub)
		if len(full) <= MaxPolicyNameLen {
			return
		}
		loc := sourceloc.Find(ctx.Element.ValuesDoc, append([]string{"stack", ctx.Element.StackKey}, path...)...)
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
		emitSub(p.PolicyRef, p.Name, "ConfigurationPolicy", "configPolicies", strconv.Itoa(i), "name")
	}
	for k, p := range comp.OperatorPolicies.Sorted() {
		if p.Name == "" {
			continue
		}
		emitSub(p.PolicyRef, p.Name, "OperatorPolicy", "operatorPolicies", k)
	}
	for i, p := range comp.CertificatePolicies {
		if p.Name == "" {
			continue
		}
		emitSub(p.PolicyRef, p.Name, "CertificatePolicy", "certificatePolicies", strconv.Itoa(i), "name")
	}
	return out
}

// DuplicateNameCheck (POLICY002) reports entries within one element that render to the same
// metadata.name. Cross-element collisions are reported by run.Run (parent policies, POLICY002) and
// by POLICY003 (sub-policy templates).
type DuplicateNameCheck struct{}

func (DuplicateNameCheck) ID() string   { return "POLICY002" }
func (DuplicateNameCheck) Phase() Phase { return PhaseCluster }

func (c *DuplicateNameCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Cluster == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	rel := ctx.Cluster.ReleaseName
	comp := ctx.Element.Values.Component
	seen := map[string]string{}
	var out []Finding

	check := func(rendered, source string, path ...string) {
		if prior, ok := seen[rendered]; ok {
			loc := sourceloc.Find(ctx.Element.ValuesDoc, append([]string{"stack", ctx.Element.StackKey}, path...)...)
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
		check(fmt.Sprintf("%s-%s", p.Name, rel), "policies["+strconv.Itoa(i)+"].name", "policies", strconv.Itoa(i), "name")
	}
	for i, p := range comp.ConfigPolicies {
		if p.Name == "" {
			continue
		}
		check(fmt.Sprintf("%s-%s", p.PolicyRef, p.Name), "configPolicies["+strconv.Itoa(i)+"].name", "configPolicies", strconv.Itoa(i), "name")
	}
	for k, p := range comp.OperatorPolicies.Sorted() {
		if p.Name == "" {
			continue
		}
		check(fmt.Sprintf("%s-%s", p.PolicyRef, p.Name), "operatorPolicies."+k, "operatorPolicies", k)
	}
	for i, p := range comp.CertificatePolicies {
		if p.Name == "" {
			continue
		}
		check(fmt.Sprintf("%s-%s", p.PolicyRef, p.Name), "certificatePolicies["+strconv.Itoa(i)+"].name", "certificatePolicies", strconv.Itoa(i), "name")
	}
	return out
}

// Cross-element POLICY002 needs every element and runs in run.Run.
var _ = chart.Element{}
