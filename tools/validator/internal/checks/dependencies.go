package checks

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// DependencyCheck (POLICY011) resolves every dependency reference the way policy-library does and
// reports the ones that can never be satisfied.
//
// This matters because a bad dependency is invisible until runtime: `helm template` succeeds, ACM
// accepts the Policy, and it then sits Pending forever on every managed cluster waiting for an
// object that will never exist. With elements cross-referencing each other, a single typo strands a
// whole chain.
//
// Runs repo-wide so `element:` references can be resolved against the other elements under stack/.
type DependencyCheck struct{}

func (DependencyCheck) ID() string   { return "POLICY011" }
func (DependencyCheck) Phase() Phase { return PhaseRepo }

// elementIndex is the resolved view of one element used for lookups.
type elementIndex struct {
	el *chart.Element
	// policyEnabled[name] is the policy's effective enabled state.
	policyEnabled map[string]bool
	// policyRenders[name] reports whether the chart emits a Policy at all: it skips any policy
	// with no enabled sub-policy attached.
	policyRenders map[string]bool
	// subPolicies["<policyRef>/<name>"] = kind of the sub-policy declared there.
	subPolicies map[string]string
	// operators[name] is an operatorPolicy's owning policy.
	operators map[string]string
	// policyToggled[name] reports whether a policy's enablement is toggle-governed.
	policyToggled map[string]bool
}

func indexElement(el *chart.Element) *elementIndex {
	ix := &elementIndex{
		el:            el,
		policyEnabled: map[string]bool{},
		policyRenders: map[string]bool{},
		subPolicies:   map[string]string{},
		operators:     map[string]string{},
		policyToggled: map[string]bool{},
	}
	comp := el.Values.Component
	if comp == nil {
		return ix
	}
	for _, p := range comp.Policies {
		if p.Name != "" {
			ix.policyEnabled[p.Name] = comp.CouldBeEnabled(p.Name, p.Enabled)
			ix.policyToggled[p.Name] = comp.IsToggled(p.Name)
		}
	}
	mark := func(ref, name, kind string) {
		if ref == "" || name == "" {
			return
		}
		ix.subPolicies[ref+"/"+name] = kind
		ix.policyRenders[ref] = true
	}
	for _, p := range comp.ConfigPolicies {
		if comp.CouldBeEnabled(p.Name, p.Enabled) {
			mark(p.PolicyRef, p.Name, "ConfigurationPolicy")
		}
	}
	for _, p := range comp.OperatorPolicies {
		if comp.CouldBeEnabled(p.Name, p.Enabled) {
			mark(p.PolicyRef, p.Name, "OperatorPolicy")
			ix.operators[p.Name] = p.PolicyRef
			// operator policies also generate <policyRef>-<name>-ns and -status checks
			ix.subPolicies[p.PolicyRef+"/"+p.Name+"-ns"] = "ConfigurationPolicy"
			ix.subPolicies[p.PolicyRef+"/"+p.Name+"-status"] = "ConfigurationPolicy"
		}
	}
	for _, p := range comp.CertificatePolicies {
		if comp.CouldBeEnabled(p.Name, p.Enabled) {
			mark(p.PolicyRef, p.Name, "CertificatePolicy")
		}
	}
	return ix
}

func (c *DependencyCheck) Run(ctx Context) []Finding {
	byChart := map[string]*elementIndex{}
	var names []string
	for _, el := range ctx.AllElements {
		if el == nil || el.Values == nil {
			continue
		}
		byChart[el.ChartName] = indexElement(el)
		names = append(names, el.ChartName)
	}
	sort.Strings(names)

	var out []Finding
	for _, n := range names {
		out = append(out, c.checkElement(byChart[n], byChart)...)
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

	// policies[].dependencies -> default kind Policy
	for i, p := range comp.Policies {
		if !comp.CouldBeEnabled(p.Name, p.Enabled) {
			continue
		}
		for j, d := range p.Dependencies {
			path := []string{"policies", strconv.Itoa(i), "dependencies", strconv.Itoa(j), "name"}
			out = append(out, c.checkDep(ix, all, d, "Policy", p.Name, fmt.Sprintf("policies[%d] %q", i, p.Name), path)...)
		}
	}

	// *.extraDependencies -> default kind ConfigurationPolicy; waitForOperator -> operatorPolicies[]
	type sub struct {
		list  string
		idx   int
		name  string
		ref   string
		extra []chart.DepEntry
		wait  chart.WaitForOperator
	}
	var subs []sub
	for i, p := range comp.ConfigPolicies {
		if comp.CouldBeEnabled(p.Name, p.Enabled) {
			subs = append(subs, sub{"configPolicies", i, p.Name, p.PolicyRef, p.ExtraDependencies, p.WaitForOperator})
		}
	}
	for i, p := range comp.OperatorPolicies {
		if comp.CouldBeEnabled(p.Name, p.Enabled) {
			subs = append(subs, sub{"operatorPolicies", i, p.Name, p.PolicyRef, p.ExtraDependencies, p.WaitForOperator})
		}
	}
	for i, p := range comp.CertificatePolicies {
		if comp.CouldBeEnabled(p.Name, p.Enabled) {
			subs = append(subs, sub{"certificatePolicies", i, p.Name, p.PolicyRef, p.ExtraDependencies, p.WaitForOperator})
		}
	}
	for _, s := range subs {
		where := fmt.Sprintf("%s[%d] %q", s.list, s.idx, s.name)
		for j, d := range s.extra {
			path := []string{s.list, strconv.Itoa(s.idx), "extraDependencies", strconv.Itoa(j), "name"}
			out = append(out, c.checkDep(ix, all, d, "ConfigurationPolicy", s.ref, where, path)...)
		}
		for _, op := range s.wait {
			if _, ok := ix.operators[op]; !ok {
				add(fmt.Sprintf("%s: waitForOperator %q matches no enabled entry in operatorPolicies[]", where, op),
					s.list, strconv.Itoa(s.idx), "waitForOperator")
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
	// raw names are verbatim and release pins an arbitrary external release: neither is resolvable.
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
		got, ok := ix.subPolicies[ref+"/"+d.Name]
		if !ok {
			add(fmt.Sprintf("%s: dependency %q resolves to %q, which no enabled configPolicies/operatorPolicies/certificatePolicies entry declares under policy %q", where, d.Name, ref+"-"+d.Name, ref))
			return out
		}
		if got != kind {
			add(fmt.Sprintf("%s: dependency %q is declared as %s, not %s", where, d.Name, got, kind))
		}
		return out
	}

	// kind Policy: same element by default, or a sibling element via element:
	target := ix
	if d.Element != "" {
		t, ok := all[d.Element]
		if !ok {
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
	enabled, exists := target.policyEnabled[d.Name]
	scope := "this element"
	if d.Element != "" {
		scope = fmt.Sprintf("element %q", d.Element)
	}
	if !exists {
		add(fmt.Sprintf("%s: dependency %q is not defined in policies[] of %s", where, d.Name, scope))
		return out
	}
	if !enabled && !target.policyToggled[d.Name] {
		add(fmt.Sprintf("%s: dependency %q exists in %s but is disabled, so the Policy is never created", where, d.Name, scope))
		return out
	}
	if !target.policyRenders[d.Name] {
		add(fmt.Sprintf("%s: dependency %q in %s has no enabled sub-policy attached, so policy-library never emits a Policy for it and this dependency can never be satisfied", where, d.Name, scope))
	}
	return out
}
