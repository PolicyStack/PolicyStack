package checks

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// RemediationCheck (POLICY036) reports a configPolicies or operatorPolicies entry whose
// remediationAction differs from its parent's policies[].remediationAction. ACM applies the
// Policy-level action to every template, so the template's own is silently overridden: an inform
// mustnothave gate becomes an enforced delete. Entries governed by a toggle count as enabled, since
// a cluster layer may turn them on. An unset configPolicies action is the chart's default, inform; an
// unset operatorPolicies action inherits the parent's.
type RemediationCheck struct{}

func (RemediationCheck) ID() string   { return "POLICY036" }
func (RemediationCheck) Phase() Phase { return PhaseChart }

func (c *RemediationCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	el := ctx.Element
	comp := el.Values.Component

	// parents[name] is the first renderable policies[] entry of that name.
	type parent struct {
		idx    int
		action string
	}
	parents := map[string]parent{}
	for i, p := range comp.Policies {
		if _, dup := parents[p.Name]; !dup && comp.CouldBeEnabled(p.Name, p.Enabled) {
			parents[p.Name] = parent{i, p.RemediationAction}
		}
	}

	var out []Finding
	check := func(name, ref, action string, enabled bool, where string, path ...string) {
		par := parents[ref]
		if action == "" || par.action == "" || strings.EqualFold(action, par.action) || !comp.CouldBeEnabled(name, enabled) {
			return
		}
		loc := sourceloc.Find(el.ValuesDoc, append([]string{"stack", el.StackKey}, path...)...)
		// Fall back to the entry itself when remediationAction is unset.
		if loc.Line == 0 {
			loc = sourceloc.Find(el.ValuesDoc, append([]string{"stack", el.StackKey}, path[:len(path)-1]...)...)
		}
		out = append(out, Finding{
			RuleID: c.ID(), Severity: SevError,
			Element: el.ChartName,
			Message: fmt.Sprintf("%s: remediationAction %s is overridden by policies[%d].remediationAction %s: ACM applies the Policy's action to every template, so this one runs as %s",
				where, action, par.idx, par.action, par.action),
			File: el.ValuesFile, Line: loc.Line, Col: loc.Col,
		})
	}
	for i, p := range comp.ConfigPolicies {
		check(p.Name, p.PolicyRef, cmp.Or(p.RemediationAction, "inform"), p.Enabled, fmt.Sprintf("configPolicies[%d] %q", i, p.Name),
			"configPolicies", strconv.Itoa(i), "remediationAction")
	}
	for k, p := range comp.OperatorPolicies.Sorted() {
		check(p.Name, p.PolicyRef, p.RemediationAction, p.Enabled, "operatorPolicies."+k,
			"operatorPolicies", k, "remediationAction")
	}
	return out
}
