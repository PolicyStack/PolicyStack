// Package chart discovers element charts under stack/ and loads each into an
// Element. values.yaml is parsed into a yaml.v3 Node tree, which keeps source
// positions for findings, and the typed Component is decoded from that tree.
package chart

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Element is a single chart under stack/<name>/ or tools/sample-element/.
type Element struct {
	// Name from Chart.yaml (kebab-case). Prefix of the helm Release.Name.
	ChartName string
	// Absolute filesystem path of the element directory.
	Dir string
	// Absolute path of the element's values.yaml (chart defaults).
	ValuesFile string
	// Path to converters/ (may not exist).
	ConvertersDir string
	// Dependencies as declared in Chart.yaml.
	Dependencies []Dependency
	// Deprecated is Chart.yaml's deprecated flag.
	Deprecated bool
	// Typed values; only the keys the checks inspect.
	Values *Values
	// yaml.v3 root node for values.yaml, used for finding line/column.
	ValuesDoc *yaml.Node
	// Key under `stack:`. Empty when values.yaml has no stack block.
	StackKey string
	// Components holds every component under `stack:`, in file order. StackKey and
	// Values.Component are the one this Element checks.
	Components []NamedComponent
	// Engines is the install.engine enum from values.schema.json: each names a stack component, of
	// which install.engine renders one. Set only for the lifecycle chart.
	Engines []string
}

// NamedComponent is one component under `stack:` and its key.
type NamedComponent struct {
	Key       string
	Component *Component
}

// Dependency is a single entry from Chart.yaml `dependencies:`.
type Dependency struct {
	Name       string `yaml:"name"`
	Alias      string `yaml:"alias"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
}

type chartYaml struct {
	APIVersion   string       `yaml:"apiVersion"`
	Name         string       `yaml:"name"`
	Version      string       `yaml:"version"`
	Deprecated   bool         `yaml:"deprecated"`
	Dependencies []Dependency `yaml:"dependencies"`
}

// Values is the typed view of `stack.<key>` plus the root-level fields the
// validator needs. Only fields the checks inspect are modelled.
type Values struct {
	PolicyNamespace string     // from root values.yaml (not per element)
	Component       *Component // values.stack[stackKey]
}

// Component mirrors stack.<key>. Only the keys the checks inspect are modelled.
//
// policy-library 4 keys policies and their templates by name, and Load adds the templates
// converters/ holds, as policy-library.model does. The 3.x shape (a policies list, configPolicies,
// toggles) only sets V3, so Render Diff can load a revision pinned before 4.0. Entries keep the
// values as written; Action, Severity and Pending apply policy-library's defaults.
//
// The legacy `enable` and `defaultPolicy` spellings are modelled so DeadKeyCheck can report them.
type Component struct {
	Enabled          bool             `yaml:"enabled"`
	Default          *Default         `yaml:"default"`
	Policies         Policies         `yaml:"-"`
	OperatorPolicies OperatorPolicies `yaml:"operatorPolicies"`
	PolicySets       []PolicySet      `yaml:"policySets"`

	// Legacy spellings the chart never reads, modelled only so they can be reported.
	LegacyEnable        *bool    `yaml:"enable"`
	LegacyDefaultPolicy *Default `yaml:"defaultPolicy"`
	// LegacyOperatorPolicies is set when operatorPolicies is the pre-3.0 list.
	LegacyOperatorPolicies bool `yaml:"-"`
	// V3 is set for the policy-library 3.x shape: a policies list, configPolicies or toggles.
	V3 bool `yaml:"-"`
}

// Default is the per-component defaults block. Disabled is modelled so DeadKeyCheck can report that
// it has no effect here.
type Default struct {
	Severity          string `yaml:"severity"`
	RemediationAction string `yaml:"remediationAction"`
	Disabled          *bool  `yaml:"disabled"`
}

// UnmarshalYAML decodes the 4.0 shape and detects the 3.x one, leaving its policies unread.
func (c *Component) UnmarshalYAML(n *yaml.Node) error {
	type plain Component
	var v struct {
		plain    `yaml:",inline"`
		Policies yaml.Node `yaml:"policies"`
	}
	if err := n.Decode(&v); err != nil {
		return err
	}
	*c = Component(v.plain)
	c.LegacyOperatorPolicies = IsLegacyOperatorPolicies(mapValue(n, "operatorPolicies"))
	c.V3 = v.Policies.Kind == yaml.SequenceNode || mapValue(n, "configPolicies") != nil || mapValue(n, "toggles") != nil
	if !c.V3 && v.Policies.Kind == yaml.MappingNode {
		return v.Policies.Decode(&c.Policies)
	}
	return nil
}

// Action is the remediationAction policy-library renders on p and its OperatorPolicies: p's own,
// else default.remediationAction, else inform.
func (c *Component) Action(p *Policy) string {
	return cmp.Or(p.RemediationAction, c.defaults().RemediationAction, "inform")
}

// TemplateAction is t's remediationAction under p: InformOnly when t sets it, else p's Action.
func (c *Component) TemplateAction(p *Policy, t *Template) string {
	if t.RemediationAction == "InformOnly" {
		return t.RemediationAction
	}
	return c.Action(p)
}

// Severity is the severity of t, or of p when t is nil: the entry's own, then p's, then
// default.severity, else low.
func (c *Component) Severity(p *Policy, t *Template) string {
	own := ""
	if t != nil {
		own = t.Severity
	}
	return cmp.Or(own, p.Severity, c.defaults().Severity, "low")
}

func (c *Component) defaults() Default {
	if c.Default == nil {
		return Default{}
	}
	return *c.Default
}

// Policies is keyed by policy name.
type Policies map[string]*Policy

// UnmarshalYAML decodes the 4.0 map, naming each entry and its templates by key.
func (ps *Policies) UnmarshalYAML(n *yaml.Node) error {
	var m map[string]*Policy
	if err := n.Decode(&m); err != nil {
		return err
	}
	*ps = Policies{}
	for name, p := range m {
		if p == nil {
			p = &Policy{}
		}
		p.Name, p.Path = name, []string{"policies", name}
		for tn, t := range p.Templates {
			if t == nil {
				t = &Template{}
				p.Templates[tn] = t
			}
			t.Name, t.Path, t.Files = tn, append(slices.Clone(p.Path), "templates", tn), t.File
		}
		(*ps)[name] = p
	}
	return nil
}

// Sorted yields the policies in name order, so findings are deterministic.
func (ps Policies) Sorted() iter.Seq2[string, *Policy] { return sorted(ps) }

// get returns policy name, adding it when absent.
func (ps Policies) get(name string) *Policy {
	if ps[name] == nil {
		ps[name] = &Policy{Name: name, Path: []string{"policies", name}}
	}
	return ps[name]
}

// Policy is one ACM Policy: policies.<name>, a converters/<name>/ directory, or install.
type Policy struct {
	Name              string     `yaml:"-"`
	Enabled           *bool      `yaml:"enabled"`
	Severity          string     `yaml:"severity"`
	RemediationAction string     `yaml:"remediationAction"`
	Dependencies      []DepEntry `yaml:"dependencies"`
	// OrderManifests lists the templates that render first, in order, each waiting for the last.
	OrderManifests []string  `yaml:"orderManifests"`
	Templates      Templates `yaml:"templates"`
	// Path is the entry's path under stack.<key>, whether or not values.yaml sets it.
	Path []string `yaml:"-"`
}

// On reports whether p renders: every entry is on unless it sets enabled: false.
func (p *Policy) On() bool { return on(p.Enabled) }

// template returns p's template name, adding it when absent.
func (p *Policy) template(name string) *Template {
	if p.Templates == nil {
		p.Templates = Templates{}
	}
	if p.Templates[name] == nil {
		p.Templates[name] = &Template{Name: name, Path: append(slices.Clone(p.Path), "templates", name)}
	}
	return p.Templates[name]
}

// Templates is keyed by template name.
type Templates map[string]*Template

// Sorted yields the templates in name order.
func (ts Templates) Sorted() iter.Seq2[string, *Template] { return sorted(ts) }

// Template is one ConfigurationPolicy, named <policy>-<template>.
type Template struct {
	Name              string     `yaml:"-"`
	Enabled           *bool      `yaml:"enabled"`
	Severity          string     `yaml:"severity"`
	RemediationAction string     `yaml:"remediationAction"`
	ComplianceType    string     `yaml:"complianceType"`
	ExtraDependencies []DepEntry `yaml:"extraDependencies"`
	WaitForOperator   StringList `yaml:"waitForOperator"`
	IgnorePending     *bool      `yaml:"ignorePending"`
	// PruneObjectBehavior is passed through verbatim; ACM defaults it to None.
	PruneObjectBehavior string `yaml:"pruneObjectBehavior"`
	// File is templates.<t>.file as written: paths under converters/ without .yaml.
	File StringList `yaml:"file"`
	// Files are the converters the template renders: File, else the one discovered for it.
	Files []string `yaml:"-"`
	Path  []string `yaml:"-"`
}

// On reports whether t renders.
func (t *Template) On() bool { return on(t.Enabled) }

// Pending is the template's ignorePending: as set, else whether it sets waitForOperator.
func (t *Template) Pending() bool { return pending(t.IgnorePending, t.WaitForOperator) }

// DepEntry is one entry of policies.<p>.dependencies or *.extraDependencies.
type DepEntry struct {
	Name       string `yaml:"name"`
	Kind       string `yaml:"kind"`
	APIVersion string `yaml:"apiVersion"`
	Namespace  string `yaml:"namespace"`
	Compliance string `yaml:"compliance"`
	PolicyRef  string `yaml:"policyRef"`
	// Release pins an exact Helm release; Element names a sibling element on the same cluster.
	Release string `yaml:"release"`
	Element string `yaml:"element"`
	Raw     bool   `yaml:"raw"`
}

// OperatorPolicy mirrors operatorPolicies entries. Name is the map key.
type OperatorPolicy struct {
	Name              string       `yaml:"name"`
	Enabled           *bool        `yaml:"enabled"`
	Severity          string       `yaml:"severity"`
	ComplianceType    string       `yaml:"complianceType"`
	UpgradeApproval   string       `yaml:"upgradeApproval"`
	Versions          []string     `yaml:"versions"`
	Subscription      Subscription `yaml:"subscription"`
	ExtraDependencies []DepEntry   `yaml:"extraDependencies"`
	WaitForOperator   StringList   `yaml:"waitForOperator"`
	IgnorePending     *bool        `yaml:"ignorePending"`
	// HasName reports an explicit `name:` in a map entry, which policy-library ignores.
	HasName bool `yaml:"-"`
}

// On reports whether o renders.
func (o OperatorPolicy) On() bool { return on(o.Enabled) }

// Pending is the OperatorPolicy's ignorePending, as Template.Pending.
func (o OperatorPolicy) Pending() bool { return pending(o.IgnorePending, o.WaitForOperator) }

// Subscription is the part of operatorPolicies.<key>.subscription the checks inspect.
type Subscription struct {
	Source              string `yaml:"source"`
	StartingCSV         string `yaml:"startingCSV"`
	InstallPlanApproval string `yaml:"installPlanApproval"`
}

// OperatorPolicies is keyed by entry name.
type OperatorPolicies map[string]OperatorPolicy

// UnmarshalYAML decodes the map shape, taking Name from the key. The pre-3.0 list shape is keyed by
// each entry's name, so older trees still load; Component.LegacyOperatorPolicies reports it.
func (o *OperatorPolicies) UnmarshalYAML(n *yaml.Node) error {
	*o = OperatorPolicies{}
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			var p OperatorPolicy
			if err := n.Content[i+1].Decode(&p); err != nil {
				return err
			}
			p.Name, p.HasName = n.Content[i].Value, mapValue(n.Content[i+1], "name") != nil
			if _, dup := (*o)[p.Name]; dup {
				return fmt.Errorf("line %d: operatorPolicies key %q already defined", n.Content[i].Line, p.Name)
			}
			(*o)[p.Name] = p
		}
		return nil
	case yaml.SequenceNode:
		var items []OperatorPolicy
		if err := n.Decode(&items); err != nil {
			return err
		}
		for _, p := range items {
			(*o)[p.Name] = p
		}
		return nil
	default:
		return fmt.Errorf("operatorPolicies: unexpected yaml kind %d at line %d", n.Kind, n.Line)
	}
}

// Sorted yields the entries in key order, so findings are deterministic.
func (o OperatorPolicies) Sorted() iter.Seq2[string, OperatorPolicy] { return sorted(o) }

// StringList accepts either a single name or a list of them, as waitForOperator and file do.
type StringList []string

// UnmarshalYAML coerces a bare scalar into a one-element list.
func (w *StringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Value != "" && n.Tag != "!!null" {
			*w = StringList{n.Value}
		}
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := n.Decode(&items); err != nil {
			return err
		}
		*w = items
		return nil
	default:
		return fmt.Errorf("unexpected yaml kind %d at line %d", n.Kind, n.Line)
	}
}

// PolicySet groups policies into a PolicySet ACM resource.
type PolicySet struct {
	Name     string   `yaml:"name"`
	Enabled  *bool    `yaml:"enabled"`
	Policies []string `yaml:"policies"`
}

// SubPolicy is one policy-template a component renders with every entry on.
type SubPolicy struct {
	// Name is metadata.name without the suffixTemplateNames suffix.
	Name, Kind string
	Policy     string
	// Path is the values entry that renders it, under stack.<key>.
	Path []string
	// Files are a ConfigurationPolicy's converters, empty for an operator's.
	Files []string
}

// Where describes the values entry, such as policies.install.templates.instance.
func (s SubPolicy) Where() string { return strings.Join(s.Path, ".") }

// SubPolicies returns every policy-template c renders when all its entries are on, in name order:
// each template, and an operator's OperatorPolicy with its -ns and -status ConfigurationPolicies,
// all in install.
func (c *Component) SubPolicies() []SubPolicy {
	var out []SubPolicy
	for _, p := range c.Policies.Sorted() {
		for _, t := range p.Templates.Sorted() {
			out = append(out, SubPolicy{p.Name + "-" + t.Name, "ConfigurationPolicy", p.Name, t.Path, t.Files})
		}
	}
	for k := range c.OperatorPolicies.Sorted() {
		base, path := "install-"+k, []string{"operatorPolicies", k}
		out = append(out,
			SubPolicy{base, "OperatorPolicy", "install", path, nil},
			SubPolicy{base + "-ns", "ConfigurationPolicy", "install", path, nil},
			SubPolicy{base + "-status", "ConfigurationPolicy", "install", path, nil})
	}
	return out
}

// discover adds what policy-library.model finds beyond values.yaml: a template per
// converters/<policy>/<template>.yaml in found, unless the template sets file, and the install
// Policy operators render into. A 3.x component is not modelled.
func (c *Component) discover(found map[string][]string) {
	if c.V3 {
		return
	}
	if c.Policies == nil {
		c.Policies = Policies{}
	}
	for p, templates := range found {
		pol := c.Policies.get(p)
		for _, name := range templates {
			if t := pol.template(name); len(t.File) == 0 {
				t.Files = []string{p + "/" + name}
			}
		}
	}
	if len(c.OperatorPolicies) > 0 {
		c.Policies.get("install")
	}
}

// Discover returns the templates policy-library discovers in dir, converters/, by policy:
// <policy>/<template>.yaml, skipping partials, any name starting with _.
func Discover(dir string) map[string][]string {
	paths, _ := filepath.Glob(filepath.Join(dir, "*", "*.yaml"))
	out := map[string][]string{}
	for _, path := range paths {
		p, t := filepath.Base(filepath.Dir(path)), strings.TrimSuffix(filepath.Base(path), ".yaml")
		if !strings.HasPrefix(p, "_") && !strings.HasPrefix(t, "_") {
			out[p] = append(out[p], t)
		}
	}
	return out
}

func on(enabled *bool) bool { return enabled == nil || *enabled }

// pending is policy-library's ignorePending: as set, else true when waitForOperator is set.
func pending(ignore *bool, waitFor StringList) bool {
	if ignore != nil {
		return *ignore
	}
	return len(waitFor) > 0
}

// sorted yields m's entries in key order.
func sorted[M ~map[string]V, V any](m M) iter.Seq2[string, V] {
	return func(yield func(string, V) bool) {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if !yield(k, m[k]) {
				return
			}
		}
	}
}

// AllOn returns a values file that turns on each element and all its policies, templates and
// operators, over any layer that turns one off: the all-on profile, which renders every converter.
func AllOn(elements []*Element) ([]byte, error) {
	on := func() map[string]any { return map[string]any{"enabled": true} }
	stack := map[string]any{}
	for _, el := range elements {
		if el.StackKey == "" || el.Values == nil || el.Values.Component == nil {
			continue
		}
		c, entry := el.Values.Component, on()
		policies, operators := map[string]any{}, map[string]any{}
		for name, p := range c.Policies.Sorted() {
			pol, templates := on(), map[string]any{}
			for t := range p.Templates.Sorted() {
				templates[t] = on()
			}
			if len(templates) > 0 {
				pol["templates"] = templates
			}
			policies[name] = pol
		}
		for name := range c.OperatorPolicies.Sorted() {
			operators[name] = on()
		}
		if len(policies) > 0 {
			entry["policies"] = policies
		}
		if len(operators) > 0 {
			entry["operatorPolicies"] = operators
		}
		stack[el.StackKey] = entry
	}
	return yaml.Marshal(map[string]any{"stack": stack})
}

// LoadAll loads every element chart under stackDir, plus sampleDir when it
// contains a Chart.yaml, and returns policyNamespace from rootValuesFile.
func LoadAll(stackDir, sampleDir, rootValuesFile string) ([]*Element, string, error) {
	ns, err := LoadPolicyNamespace(rootValuesFile)
	if err != nil {
		return nil, "", fmt.Errorf("read root values: %w", err)
	}

	var dirs []string
	if entries, err := os.ReadDir(stackDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(stackDir, e.Name()))
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", fmt.Errorf("read stack dir: %w", err)
	}
	if sampleDir != "" {
		if _, err := os.Stat(filepath.Join(sampleDir, "Chart.yaml")); err == nil {
			dirs = append(dirs, sampleDir)
		}
	}

	var out []*Element
	for _, d := range dirs {
		el, err := Load(d, ns)
		if err != nil {
			return nil, ns, fmt.Errorf("load %s: %w", d, err)
		}
		out = append(out, el)
	}
	return out, ns, nil
}

// Load parses a single element directory.
func Load(dir, policyNamespace string) (*Element, error) {
	chartFile := filepath.Join(dir, "Chart.yaml")
	cyData, err := os.ReadFile(chartFile)
	if err != nil {
		return nil, fmt.Errorf("read Chart.yaml: %w", err)
	}
	var cy chartYaml
	if err := yaml.Unmarshal(cyData, &cy); err != nil {
		return nil, fmt.Errorf("parse Chart.yaml: %w", err)
	}

	valuesFile := filepath.Join(dir, "values.yaml")
	vData, err := os.ReadFile(valuesFile)
	if err != nil {
		return nil, fmt.Errorf("read values.yaml: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(vData, &doc); err != nil {
		return nil, fmt.Errorf("parse values.yaml: %w", err)
	}

	engines, err := loadEngines(filepath.Join(dir, "values.schema.json"))
	if err != nil {
		return nil, err
	}
	comps, err := decodeComponents(&doc, engines != nil)
	if err != nil {
		return nil, err
	}
	found := Discover(filepath.Join(dir, "converters"))
	for _, c := range comps {
		c.Component.discover(found)
	}
	var first NamedComponent
	if len(comps) > 0 {
		first = comps[0]
	}

	el := &Element{
		ChartName:     cy.Name,
		Dir:           dir,
		ValuesFile:    valuesFile,
		ConvertersDir: filepath.Join(dir, "converters"),
		Dependencies:  cy.Dependencies,
		Deprecated:    cy.Deprecated,
		Values: &Values{
			PolicyNamespace: policyNamespace,
			Component:       first.Component,
		},
		ValuesDoc:  &doc,
		StackKey:   first.Key,
		Components: comps,
		Engines:    engines,
	}
	return el, nil
}

// LoadComponents loads a chart as one Element per stack component: the lifecycle chart renders
// one engine's component per cluster, so each is checked like an element. Any other chart loads as
// a single Element.
func LoadComponents(dir, policyNamespace string) ([]*Element, error) {
	el, err := Load(dir, policyNamespace)
	if err != nil {
		return nil, err
	}
	if el.Engines == nil || len(el.Components) < 2 {
		return []*Element{el}, nil
	}
	out := make([]*Element, len(el.Components))
	for i, c := range el.Components {
		e := *el
		e.StackKey = c.Key
		e.Values = &Values{PolicyNamespace: policyNamespace, Component: c.Component}
		out[i] = &e
	}
	return out, nil
}

// decodeComponents decodes the first key under `stack:` in values.yaml, the element, or every key
// in file order when all is set: the lifecycle chart has one component per engine.
func decodeComponents(doc *yaml.Node, all bool) ([]NamedComponent, error) {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil
	}
	stackNode := mapValue(root, "stack")
	if stackNode == nil || stackNode.Kind != yaml.MappingNode {
		return nil, nil
	}
	var out []NamedComponent
	for i := 0; i+1 < len(stackNode.Content) && (all || len(out) == 0); i += 2 {
		keyNode, valueNode := stackNode.Content[i], stackNode.Content[i+1]
		var c Component
		if err := valueNode.Decode(&c); err != nil {
			return nil, fmt.Errorf("decode stack.%s: %w", keyNode.Value, err)
		}
		out = append(out, NamedComponent{Key: keyNode.Value, Component: &c})
	}
	return out, nil
}

// IsLegacyOperatorPolicies reports whether an operatorPolicies node has the pre-3.0 list shape.
func IsLegacyOperatorPolicies(n *yaml.Node) bool {
	return n != nil && n.Kind == yaml.SequenceNode
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// loadEngines returns properties.install.properties.engine.enum from a values.schema.json, nil when
// the file or the enum is absent.
func loadEngines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s struct {
		Properties struct {
			Install struct {
				Properties struct {
					Engine struct {
						Enum []string `json:"enum"`
					} `json:"engine"`
				} `json:"properties"`
			} `json:"install"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse values.schema.json: %w", err)
	}
	return s.Properties.Install.Properties.Engine.Enum, nil
}

// LoadPolicyNamespace reads policyNamespace from a values file. Empty is an error.
func LoadPolicyNamespace(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var v struct {
		PolicyNamespace string `yaml:"policyNamespace"`
	}
	if err := yaml.Unmarshal(data, &v); err != nil {
		return "", err
	}
	if v.PolicyNamespace == "" {
		return "", fmt.Errorf("%s: policyNamespace is empty", path)
	}
	return v.PolicyNamespace, nil
}

// CamelFromKebab converts a kebab-case chart name to the camelCase key used
// under `stack:` (see chart-readme.md). Matches tools/create-element.sh.
func CamelFromKebab(s string) string {
	parts := strings.Split(s, "-")
	if len(parts) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString(strings.ToLower(parts[0]))
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		out.WriteString(strings.ToUpper(p[:1]) + strings.ToLower(p[1:]))
	}
	return out.String()
}
