package checks

import (
	"fmt"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

// GateCheck (POLICY042) reports a template the chart ships InformOnly that renders with another
// action. A values layer clears InformOnly with null or "", on the key or on the entry, and the
// template then takes its Policy's action: under enforce, a mustnothave gate deletes the object it
// only reports on.
type GateCheck struct{}

func (GateCheck) ID() string   { return "POLICY042" }
func (GateCheck) Phase() Phase { return PhaseCluster }

func (c *GateCheck) Run(ctx Context) []Finding {
	if len(ctx.Rendered) == 0 || ctx.Element == nil || ctx.Cluster == nil {
		return nil
	}
	gates := gateNames(ctx.Element, ctx.Cluster.ClusterName)
	if len(gates) == 0 {
		return nil
	}
	pols, bad := policies(ctx, c.ID())
	if bad != nil {
		return bad
	}
	var out []Finding
	for _, p := range pols {
		for _, t := range p.Spec.PolicyTemplates {
			od := t.ObjectDefinition
			if od.Kind != "ConfigurationPolicy" || !gates[od.Metadata.Name] || informOnly(od.Spec.RemediationAction) {
				continue
			}
			where, file, loc := origin(ctx, od.Kind, od.Metadata.Name, "remediationAction")
			out = append(out, Finding{RuleID: c.ID(), Severity: SevError, Element: ctx.Element.ChartName,
				Message: fmt.Sprintf("%s: the chart ships this gate InformOnly and it renders %q instead, so an enforce Policy would write or delete what it should only report. Remove the override; to drop the gate, set enabled: false",
					where, od.Spec.RemediationAction),
				File: file, Line: loc.Line, Col: loc.Col})
		}
	}
	return out
}

// gateNames holds the rendered names of the templates el's values.yaml sets InformOnly, with and
// without the cluster suffix suffixTemplateNames adds.
func gateNames(el *chart.Element, cluster string) map[string]bool {
	out := map[string]bool{}
	if el.Values == nil || el.Values.Component == nil {
		return out
	}
	for _, p := range el.Values.Component.Policies.Sorted() {
		for _, t := range p.Templates.Sorted() {
			if informOnly(t.RemediationAction) {
				name := p.Name + "-" + t.Name
				out[name], out[name+"-"+cluster] = true, true
			}
		}
	}
	return out
}
