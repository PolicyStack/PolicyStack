package checks

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// DependencyCheck (POLICY011) resolves every dependency the way policy-library does and reports
// those that can never be satisfied.
//
// A bad dependency renders and is accepted by ACM, then leaves the Policy Pending on every managed
// cluster, along with everything that depends on it.
//
// Runs repo-wide so `element:` references resolve against the other elements under stack/. An
// `element:` naming no element under stack/ is reported only when a fleet file's render holds the
// Policy declaring it with that dependency, so a copy may leave out the elements its disabled
// policies name, or that a layer replaced the dependency on. UnrenderedDependencies covers what
// only the renders show.
type DependencyCheck struct{}

func (DependencyCheck) ID() string   { return "POLICY011" }
func (DependencyCheck) Phase() Phase { return PhaseRepo }

// elementIndex is the resolved view of one element used for lookups. Every entry counts, enabled or
// not, since a values layer may turn it on.
type elementIndex struct {
	el *chart.Element
	// rendered[name] holds the dependencies of the Policy of policies.<name> in fleet file renders.
	rendered map[string]map[string]bool
	// policyRenders[name] reports whether the chart emits the Policy, which needs a template or an
	// operator. Every policy of the element is a key.
	policyRenders map[string]bool
	// subPolicies[metadata.name] is the kind of that policy-template.
	subPolicies map[string]string
}

func indexElement(el *chart.Element) *elementIndex {
	ix := &elementIndex{el: el, policyRenders: map[string]bool{}, subPolicies: map[string]string{}}
	comp := el.Values.Component
	if comp == nil {
		return ix
	}
	for name := range comp.Policies {
		ix.policyRenders[name] = false
	}
	for _, s := range comp.SubPolicies() {
		ix.subPolicies[s.Name] = s.Kind
		ix.policyRenders[s.Policy] = true
	}
	return ix
}

func (c *DependencyCheck) Run(ctx Context) []Finding {
	// byChart resolves element: references. Each lifecycle component is an Element of its own and
	// is checked against itself.
	byChart := map[string]*elementIndex{}
	var ixs []*elementIndex
	for _, el := range ctx.AllElements {
		if el == nil || el.Values == nil {
			continue
		}
		ix := indexElement(el)
		ix.rendered = ctx.RenderedDependencies[el.ChartName]
		byChart[el.ChartName] = ix
		ixs = append(ixs, ix)
	}
	slices.SortStableFunc(ixs, func(a, b *elementIndex) int { return strings.Compare(a.el.ChartName, b.el.ChartName) })

	var out []Finding
	for _, ix := range ixs {
		out = append(out, c.checkElement(ix, byChart)...)
	}
	return out
}

func (c *DependencyCheck) checkElement(ix *elementIndex, all map[string]*elementIndex) []Finding {
	el := ix.el
	comp := el.Values.Component
	if comp == nil {
		return nil
	}
	var out []Finding
	add := func(msg string, path ...string) {
		loc := sourceloc.Find(el.ValuesDoc, append([]string{"stack", el.StackKey}, path...)...)
		out = append(out, Finding{
			RuleID: c.ID(), Severity: SevError,
			Element: el.ChartName,
			Message: msg,
			File:    el.ValuesFile, Line: loc.Line, Col: loc.Col,
		})
	}

	// policies.<p>.dependencies default to kind Policy.
	for _, p := range comp.Policies.Sorted() {
		for j, d := range p.Dependencies {
			path := append(slices.Clone(p.Path), "dependencies", strconv.Itoa(j), "name")
			out = append(out, c.checkDep(ix, all, d, "Policy", p.Name, strings.Join(p.Path, "."), path)...)
		}
	}

	// *.extraDependencies default to kind ConfigurationPolicy; waitForOperator names operatorPolicies keys.
	type sub struct {
		path  []string // the entry under stack.<key>
		ref   string
		extra []chart.DepEntry
		wait  chart.StringList
	}
	var subs []sub
	for _, p := range comp.Policies.Sorted() {
		for _, t := range p.Templates.Sorted() {
			subs = append(subs, sub{t.Path, p.Name, t.ExtraDependencies, t.WaitForOperator})
		}
	}
	for k, op := range comp.OperatorPolicies.Sorted() {
		subs = append(subs, sub{[]string{"operatorPolicies", k}, "install", op.ExtraDependencies, op.WaitForOperator})
	}
	for _, s := range subs {
		where := strings.Join(s.path, ".")
		for j, d := range s.extra {
			path := append(slices.Clone(s.path), "extraDependencies", strconv.Itoa(j), "name")
			out = append(out, c.checkDep(ix, all, d, "ConfigurationPolicy", s.ref, where, path)...)
		}
		for _, op := range s.wait {
			if _, ok := comp.OperatorPolicies[op]; !ok {
				add(fmt.Sprintf("%s: waitForOperator %q matches no operatorPolicies.%s", where, op, op),
					append(slices.Clone(s.path), "waitForOperator")...)
			}
		}
	}
	return out
}

// checkDep resolves one dependency entry against the chart's own naming rules.
func (c *DependencyCheck) checkDep(ix *elementIndex, all map[string]*elementIndex, d chart.DepEntry, defaultKind, owner, where string, path []string) []Finding {
	el := ix.el
	var out []Finding
	add := func(msg string) {
		loc := sourceloc.Find(el.ValuesDoc, append([]string{"stack", el.StackKey}, path...)...)
		out = append(out, Finding{
			RuleID: c.ID(), Severity: SevError,
			Element: el.ChartName, Message: msg,
			File: el.ValuesFile, Line: loc.Line, Col: loc.Col,
		})
	}

	if d.Name == "" {
		add(where + ": dependency entry is missing a name")
		return out
	}
	// raw names are used verbatim and release pins an external release; neither can be resolved here.
	if d.Raw || d.Release != "" {
		return out
	}
	kind := d.Kind
	if kind == "" {
		kind = defaultKind
	}

	if kind != "Policy" {
		if d.Element != "" {
			add(fmt.Sprintf("%s: dependency %q sets element: with kind %s - element: is only valid for kind Policy, because template kinds carry no release suffix", where, d.Name, kind))
			return out
		}
		ref := d.PolicyRef
		if ref == "" {
			ref = owner
		}
		got, ok := ix.subPolicies[ref+"-"+d.Name]
		if !ok {
			add(fmt.Sprintf("%s: dependency %q resolves to %q, which no template or operator of policy %q renders", where, d.Name, ref+"-"+d.Name, ref))
			return out
		}
		if got != kind {
			add(fmt.Sprintf("%s: dependency %q is declared as %s, not %s", where, d.Name, got, kind))
		}
		return out
	}

	// kind Policy: this element, or the sibling named by element:.
	target := ix
	if d.Element != "" {
		t, ok := all[d.Element]
		if !ok {
			if !ix.rendered[owner][d.Name+"-"+d.Element] {
				return out
			}
			var known []string
			for n := range all {
				known = append(known, n)
			}
			sort.Strings(known)
			add(fmt.Sprintf("%s: dependency %q names element %q, which is not an element under stack/ (known: %v)", where, d.Name, d.Element, known))
			return out
		}
		target = t
	}
	renders, exists := target.policyRenders[d.Name]
	scope := "this element"
	if d.Element != "" {
		scope = fmt.Sprintf("element %q", d.Element)
	}
	if !exists {
		add(fmt.Sprintf("%s: dependency %q is not a policy of %s", where, d.Name, scope))
		return out
	}
	if !renders {
		add(fmt.Sprintf("%s: dependency %q in %s has no template or operator, so policy-library never emits a Policy for it and this dependency can never be satisfied", where, d.Name, scope))
	}
	return out
}

// RenderedDependency is a kind Policy dependency of a Policy in one fleet file's render.
type RenderedDependency struct {
	Policy  string  // policies.<key> of the declaring Policy
	Target  string  // the name as rendered: <policy>-<element>-<cluster> for a PolicyStack Policy
	Finding Finding // the warning UnrenderedDependencies reports for it
}

// PolicyDependencies returns the kind Policy dependencies of every Policy in ctx.Rendered, its own
// and its templates' extraDependencies, each warning located at the policy's entry in the last
// cascade file defining it.
func PolicyDependencies(ctx Context) []RenderedDependency {
	pols, _ := policies(ctx, "")
	var out []RenderedDependency
	for _, p := range pols {
		if p.Kind != "Policy" {
			continue
		}
		deps := slices.Clone(p.Spec.Dependencies)
		for _, t := range p.Spec.PolicyTemplates {
			deps = append(deps, t.ExtraDependencies...)
		}
		deps = slices.DeleteFunc(deps, func(d dependency) bool { return d.Kind != "Policy" })
		if len(deps) == 0 {
			continue
		}
		where, file, loc := origin(ctx, "Policy", p.Metadata.Name, "dependencies")
		for _, d := range deps {
			out = append(out, RenderedDependency{
				Policy: strings.TrimSuffix(p.Metadata.Name, "-"+ctx.Cluster.ReleaseName),
				Target: d.Name,
				Finding: Finding{
					RuleID: "POLICY011", Severity: SevWarning,
					Element: ctx.Element.ChartName, Cluster: ctx.Cluster.ClusterName,
					Message: fmt.Sprintf("%s: dependency %q is a Policy no element renders for this cluster, so ACM leaves this Policy Pending. Turn that policy on, or replace the dependency in a layer", where, d.Name),
					File:    file, Line: loc.Line, Col: loc.Col,
				},
			})
		}
	}
	return out
}

// UnrenderedDependencies reports the dependencies in one fleet file's renders on a PolicyStack
// Policy those renders don't hold, such as acm observability waiting on odf ready with odf off.
// rendered holds the Policy names of every element's render for the fleet file and cluster is its
// cluster suffix. Only targets named like a policy of an element in all that has a template or
// operator are reported: anything else may be outside PolicyStack.
func UnrenderedDependencies(all []*chart.Element, cluster string, rendered map[string]bool, deps []RenderedDependency) []Finding {
	known := map[string]bool{}
	for _, el := range all {
		if el == nil || el.Values == nil {
			continue
		}
		for name, renders := range indexElement(el).policyRenders {
			if renders {
				known[name+"-"+el.ChartName+"-"+cluster] = true
			}
		}
	}
	var out []Finding
	for _, d := range deps {
		if known[d.Target] && !rendered[d.Target] {
			out = append(out, d.Finding)
		}
	}
	return out
}
