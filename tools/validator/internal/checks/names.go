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

// NameLengthCheck (POLICY001) enforces the 63-char limit on every name policy-library rendered,
// layer overrides included:
//
//	Policy, PolicySet   <ns>.<name>-<release>, the name ACM replicates it under
//	policy-templates    metadata.name, <parent>-<sub>
//
// ACM enforces the DNS-1123 label limit on the replicated name on each managed cluster, and on the
// templates it creates there.
type NameLengthCheck struct{}

func (NameLengthCheck) ID() string   { return "POLICY001" }
func (NameLengthCheck) Phase() Phase { return PhaseCluster }

func (c *NameLengthCheck) Run(ctx Context) []Finding {
	if len(ctx.Rendered) == 0 || ctx.Element == nil || ctx.Cluster == nil {
		return nil
	}
	pols, bad := policies(ctx, c.ID())
	if bad != nil {
		return bad
	}
	var out []Finding
	// check reports name, which kind/obj is created under, over the limit.
	check := func(kind, obj, label, name string) {
		if len(name) <= MaxPolicyNameLen {
			return
		}
		where, file, loc := origin(ctx, kind, obj, "name")
		out = append(out, Finding{
			RuleID: c.ID(), Severity: SevError,
			Element: ctx.Element.ChartName, Cluster: ctx.Cluster.ClusterName,
			Message: fmt.Sprintf("%s: %s %s %q is %d chars > %d", where, kind, label, name, len(name), MaxPolicyNameLen),
			File:    file, Line: loc.Line, Col: loc.Col,
		})
	}
	for _, p := range pols {
		check(p.Kind, p.Metadata.Name, "replicated name", p.Metadata.Namespace+"."+p.Metadata.Name)
		for _, t := range p.Spec.PolicyTemplates {
			od := t.ObjectDefinition
			check(od.Kind, od.Metadata.Name, "metadata.name", od.Metadata.Name)
		}
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
