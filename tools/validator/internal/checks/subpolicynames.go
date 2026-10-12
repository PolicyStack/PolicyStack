package checks

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// SubPolicyNameCheck (POLICY003) reports policy-templates with the same name rendered by different
// elements.
//
// ACM requires policy-template names to be unique across every Policy placed on a cluster, because
// the templates are replicated into that cluster's namespace. A duplicate is rejected on the
// managed cluster with "Template name must be unique" and the whole Policy stops processing.
//
// A sub-policy name is "<policyRef>-<name>" with no element or cluster component, so generic names
// (policies[].name=install with configPolicies[].name=ns-monitoring) collide across elements. Helm
// renders them without error and POLICY002 only compares within one element, so this runs
// repo-wide.
type SubPolicyNameCheck struct{}

func (SubPolicyNameCheck) ID() string   { return "POLICY003" }
func (SubPolicyNameCheck) Phase() Phase { return PhaseRepo }

// owner records where a rendered sub-policy name came from.
type owner struct {
	element string
	where   string   // configPolicies[0] or operatorPolicies.<key>
	path    []string // under stack.<key>, to the entry's name
	el      *chart.Element
}

func (c *SubPolicyNameCheck) Run(ctx Context) []Finding {
	// rendered name -> every element/entry that produces it
	seen := map[string][]owner{}

	for _, el := range ctx.AllElements {
		if el == nil || el.Values == nil || el.Values.Component == nil {
			continue
		}
		comp := el.Values.Component
		add := func(name, where string, path ...string) {
			if name == "" {
				return
			}
			seen[name] = append(seen[name], owner{el.ChartName, where, path, el})
		}
		for i, p := range comp.ConfigPolicies {
			if comp.CouldBeEnabled(p.Name, p.Enabled) && p.PolicyRef != "" {
				add(p.PolicyRef+"-"+p.Name, fmt.Sprintf("configPolicies[%d]", i), "configPolicies", strconv.Itoa(i), "name")
			}
		}
		for k, p := range comp.OperatorPolicies.Sorted() {
			if !comp.CouldBeEnabled(p.Name, p.Enabled) || p.PolicyRef == "" {
				continue
			}
			base := p.PolicyRef + "-" + p.Name
			// An operator policy renders three templates.
			for _, n := range []string{base, base + "-ns", base + "-status"} {
				add(n, "operatorPolicies."+k, "operatorPolicies", k)
			}
		}
		for i, p := range comp.CertificatePolicies {
			if comp.CouldBeEnabled(p.Name, p.Enabled) && p.PolicyRef != "" {
				add(p.PolicyRef+"-"+p.Name, fmt.Sprintf("certificatePolicies[%d]", i), "certificatePolicies", strconv.Itoa(i), "name")
			}
		}
	}

	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []Finding
	for _, name := range names {
		owners := seen[name]
		// Only a collision across elements. Repeats within one element are POLICY002.
		elements := map[string]bool{}
		for _, o := range owners {
			elements[o.element] = true
		}
		if len(elements) < 2 {
			continue
		}
		others := make([]string, 0, len(elements))
		for e := range elements {
			others = append(others, e)
		}
		sort.Strings(others)
		for _, o := range owners {
			loc := sourceloc.Find(o.el.ValuesDoc, append([]string{"stack", o.el.StackKey}, o.path...)...)
			out = append(out, Finding{
				RuleID:   c.ID(),
				Severity: SevError,
				Element:  o.element,
				Message: fmt.Sprintf(
					"%s renders policy-template %q, which %v also render. ACM requires template names to be unique across every Policy on a cluster - give the sub-policy an element-specific name",
					o.where, name, others),
				File: o.el.ValuesFile, Line: loc.Line, Col: loc.Col,
			})
		}
	}
	return out
}
