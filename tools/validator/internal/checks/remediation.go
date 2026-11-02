package checks

import (
	"fmt"
	"strings"
)

// RemediationCheck (POLICY036) reports a rendered ConfigurationPolicy or OperatorPolicy whose
// remediationAction differs from its Policy's. ACM applies the Policy-level action to every
// template, so the template's own is silently overridden: an inform mustnothave gate becomes an
// enforced delete. InformOnly is the exception and is skipped, and so is a dry run
// (global.remediationAction), which overrides every Policy on purpose, and the templates
// policy-library generates for an OperatorPolicy. Findings point at the
// Policy's remediationAction, in whichever values file sets it.
type RemediationCheck struct{}

func (RemediationCheck) ID() string   { return "POLICY036" }
func (RemediationCheck) Phase() Phase { return PhaseCluster }

func (c *RemediationCheck) Run(ctx Context) []Finding {
	if len(ctx.Rendered) == 0 || ctx.Element == nil || ctx.Cluster == nil {
		return nil
	}
	pols, bad := policies(ctx, c.ID())
	if bad != nil {
		return bad
	}
	var out []Finding
	for _, p := range pols {
		pa := p.Spec.RemediationAction
		// policy-library generates <op>-ns and <op>-status for each OperatorPolicy; no values set their action.
		generated := map[string]bool{}
		for _, t := range p.Spec.PolicyTemplates {
			if od := t.ObjectDefinition; od.Kind == "OperatorPolicy" {
				generated[od.Metadata.Name+"-ns"], generated[od.Metadata.Name+"-status"] = true, true
			}
		}
		for _, t := range p.Spec.PolicyTemplates {
			od := t.ObjectDefinition
			ta := od.Spec.RemediationAction
			if (od.Kind != "ConfigurationPolicy" && od.Kind != "OperatorPolicy") || generated[od.Metadata.Name] ||
				pa == "" || ta == "" || informOnly(ta) || strings.EqualFold(ta, pa) || dryRun(ctx) {
				continue
			}
			where, file, loc := origin(ctx, p.Kind, p.Metadata.Name, "remediationAction")
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevError, Element: ctx.Element.ChartName,
				Message: fmt.Sprintf("%s: remediationAction %s overrides %s %q's %s: ACM applies the Policy's action to every template, so it runs as %s",
					where, pa, od.Kind, od.Metadata.Name, ta, pa),
				File: file, Line: loc.Line, Col: loc.Col,
			})
		}
	}
	return out
}
