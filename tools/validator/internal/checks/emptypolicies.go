package checks

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// EmptyPolicyCheck (POLICY082) reports a rendered Policy that checks nothing: every template is a
// ConfigurationPolicy without objects. ACM reports such a Policy Compliant. policy-library renders a
// raw converter that outputs nothing as `[]`, which is what an element looks like when the values
// map its converter ranges over is empty. A Policy with only some empty templates is not reported.
type EmptyPolicyCheck struct{}

func (EmptyPolicyCheck) ID() string   { return "POLICY082" }
func (EmptyPolicyCheck) Phase() Phase { return PhaseCluster }

func (c *EmptyPolicyCheck) Run(ctx Context) []Finding {
	if len(ctx.Rendered) == 0 || ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil || ctx.Cluster == nil {
		return nil
	}
	el := ctx.Element
	pols, bad := policies(ctx, c.ID())
	if bad != nil {
		return bad
	}
	var out []Finding
	for _, p := range pols {
		if p.Kind != "Policy" {
			continue
		}
		empty := true
		for _, pt := range p.Spec.PolicyTemplates {
			od := pt.ObjectDefinition
			raw := strings.TrimSpace(od.Spec.Raw)
			if od.Kind != "ConfigurationPolicy" || len(od.Spec.ObjectTemplates) > 0 || (raw != "" && raw != "[]") {
				empty = false
				break
			}
		}
		if !empty {
			continue
		}
		where, loc := fmt.Sprintf("Policy %q", p.Metadata.Name), sourceloc.Loc{}
		for i, pol := range el.Values.Component.Policies {
			if pol.Name+"-"+ctx.Cluster.ReleaseName == p.Metadata.Name {
				where = fmt.Sprintf("policies[%d] %q", i, pol.Name)
				loc = sourceloc.Find(el.ValuesDoc, "stack", el.StackKey, "policies", strconv.Itoa(i), "name")
				break
			}
		}
		out = append(out, Finding{
			RuleID: c.ID(), Severity: SevError,
			Element: el.ChartName, Cluster: ctx.Cluster.ClusterName,
			Message: where + " checks nothing: every ConfigurationPolicy has no objects and there is no OperatorPolicy or CertificatePolicy, so ACM reports it Compliant: set the element's input or turn the toggle off",
			File:    el.ValuesFile, Line: loc.Line, Col: loc.Col,
		})
	}
	return out
}
