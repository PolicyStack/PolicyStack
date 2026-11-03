package checks

import (
	"fmt"
	"slices"
	"strings"
)

var allowedPrune = []string{"None", "DeleteIfCreated", "DeleteAll"}

// PruneCheck (POLICY033) checks each rendered ConfigurationPolicy's pruneObjectBehavior.
//
// ACM prunes only under an effective enforce. When a template's dependency is unmet, template-sync
// deletes the spoke ConfigurationPolicy without resetting prune, so prune with any dependency
// deletes the template's objects whenever that dependency flaps. policy-library renders
// waitForOperator, extraDependencies and orderManifests as the template's extraDependencies, and
// orderPolicies and policies[].dependencies as the Policy's dependencies.
type PruneCheck struct{}

func (PruneCheck) ID() string   { return "POLICY033" }
func (PruneCheck) Phase() Phase { return PhaseCluster }

func (c *PruneCheck) Run(ctx Context) []Finding {
	if len(ctx.Rendered) == 0 || ctx.Element == nil || ctx.Cluster == nil {
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
			prune := od.Spec.PruneObjectBehavior
			if od.Kind != "ConfigurationPolicy" || prune == "" || prune == "None" {
				continue
			}
			add := func(sev Severity, msg string) {
				where, file, loc := origin(ctx, od.Kind, od.Metadata.Name, "pruneObjectBehavior")
				out = append(out, Finding{RuleID: c.ID(), Severity: sev, Element: ctx.Element.ChartName,
					Message: where + ": " + msg, File: file, Line: loc.Line, Col: loc.Col})
			}

			if !slices.Contains(allowedPrune, prune) {
				add(SevError, fmt.Sprintf("pruneObjectBehavior = %q; allowed: %v", prune, allowedPrune))
				continue
			}
			if prune == "DeleteAll" {
				add(SevWarning, "pruneObjectBehavior DeleteAll also deletes objects that existed before the policy, such as shared namespaces; prefer DeleteIfCreated")
			}
			// A dry run renders every Policy inform on purpose.
			if action := p.action(od.Spec.RemediationAction); !strings.EqualFold(action, "enforce") && !dryRun(ctx) {
				add(SevError, fmt.Sprintf("pruneObjectBehavior under remediationAction %s: ACM prunes only under enforce", action))
			}
			var deps []string
			for _, d := range t.ExtraDependencies {
				deps = append(deps, d.Name)
			}
			if len(p.Spec.Dependencies) > 0 {
				deps = append(deps, "the Policy's dependencies")
			}
			if len(deps) > 0 {
				add(SevError, fmt.Sprintf("pruneObjectBehavior with a dependency (%s): ACM deletes this template whenever the dependency is unmet, and prune deletes its objects", strings.Join(deps, ", ")))
			}
		}
	}
	return out
}
