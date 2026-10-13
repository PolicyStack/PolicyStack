package checks

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

var allowedPrune = []string{"None", "DeleteIfCreated", "DeleteAll"}

// PruneCheck (POLICY033) checks configPolicies[].pruneObjectBehavior.
//
// ACM prunes only under an effective enforce. When a template's dependency is unmet, template-sync
// deletes the spoke ConfigurationPolicy without resetting prune, so prune with any dependency
// deletes the template's objects whenever that dependency flaps. Dependencies are resolved the way
// policy-library chains them: config templates render first in a policy, in list order, and
// orderPolicies chains only Policies that render. Entries governed by a toggle count as enabled,
// since a cluster layer may turn them on.
type PruneCheck struct{}

func (PruneCheck) ID() string   { return "POLICY033" }
func (PruneCheck) Phase() Phase { return PhaseChart }

func (c *PruneCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	el := ctx.Element
	comp := el.Values.Component

	// hasTemplate[name] reports whether policy name has a template that could be enabled, so the Policy renders.
	hasTemplate := map[string]bool{}
	for _, p := range comp.ConfigPolicies {
		hasTemplate[p.PolicyRef] = hasTemplate[p.PolicyRef] || comp.CouldBeEnabled(p.Name, p.Enabled)
	}
	for _, p := range comp.OperatorPolicies {
		hasTemplate[p.PolicyRef] = hasTemplate[p.PolicyRef] || comp.CouldBeEnabled(p.Name, p.Enabled)
	}
	for _, p := range comp.CertificatePolicies {
		hasTemplate[p.PolicyRef] = hasTemplate[p.PolicyRef] || comp.CouldBeEnabled(p.Name, p.Enabled)
	}

	type parent struct {
		idx    int
		policy chart.Policy
		first  bool // first rendered Policy, which orderPolicies leaves unchained
	}
	parents := map[string]parent{}
	for i, p := range comp.Policies {
		if !comp.CouldBeEnabled(p.Name, p.Enabled) || !hasTemplate[p.Name] {
			continue
		}
		if _, dup := parents[p.Name]; !dup {
			parents[p.Name] = parent{i, p, len(parents) == 0}
		}
	}

	var out []Finding
	rendered := map[string]bool{} // policies that already rendered a config template
	for i, sp := range comp.ConfigPolicies {
		if !comp.CouldBeEnabled(sp.Name, sp.Enabled) {
			continue
		}
		firstTemplate := !rendered[sp.PolicyRef]
		rendered[sp.PolicyRef] = true
		par, ok := parents[sp.PolicyRef]
		if !ok || sp.PruneObjectBehavior == "" || sp.PruneObjectBehavior == "None" {
			continue
		}
		where := fmt.Sprintf("configPolicies[%d] %q", i, sp.Name)
		add := func(sev Severity, msg string) {
			loc := sourceloc.Find(el.ValuesDoc, "stack", el.StackKey, "configPolicies", strconv.Itoa(i), "pruneObjectBehavior")
			out = append(out, Finding{
				RuleID: c.ID(), Severity: sev,
				Element: el.ChartName,
				Message: where + ": " + msg,
				File:    el.ValuesFile, Line: loc.Line, Col: loc.Col,
			})
		}

		if !slices.Contains(allowedPrune, sp.PruneObjectBehavior) {
			add(SevError, fmt.Sprintf("pruneObjectBehavior = %q; allowed: %v", sp.PruneObjectBehavior, allowedPrune))
			continue
		}
		if sp.PruneObjectBehavior == "DeleteAll" {
			add(SevWarning, "pruneObjectBehavior DeleteAll also deletes objects that existed before the policy, such as shared namespaces; prefer DeleteIfCreated")
		}

		// A Policy-level remediationAction overrides every template's; policy-library defaults templates to inform.
		action, from := cmp.Or(sp.RemediationAction, "inform"), "remediationAction"
		if par.policy.RemediationAction != "" {
			action, from = par.policy.RemediationAction, fmt.Sprintf("policies[%d].remediationAction", par.idx)
		}
		if !strings.EqualFold(action, "enforce") {
			add(SevError, fmt.Sprintf("pruneObjectBehavior with %s %s: ACM prunes only under enforce", from, action))
		}

		var deps []string
		if len(sp.WaitForOperator) > 0 {
			deps = append(deps, "waitForOperator")
		}
		if len(sp.ExtraDependencies) > 0 {
			deps = append(deps, "extraDependencies")
		}
		if len(par.policy.Dependencies) > 0 {
			deps = append(deps, fmt.Sprintf("policies[%d].dependencies", par.idx))
		}
		orderManifests := comp.OrderManifests
		if par.policy.OrderManifests != nil {
			orderManifests = *par.policy.OrderManifests
		}
		if orderManifests && !firstTemplate {
			deps = append(deps, "orderManifests")
		}
		if comp.OrderPolicies && !par.first {
			deps = append(deps, "orderPolicies")
		}
		if len(deps) > 0 {
			add(SevError, fmt.Sprintf("pruneObjectBehavior with a dependency (%s): ACM deletes this template whenever the dependency is unmet, and prune deletes its objects", strings.Join(deps, ", ")))
		}
	}
	return out
}
