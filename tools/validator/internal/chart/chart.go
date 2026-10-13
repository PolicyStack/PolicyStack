// Package chart discovers element charts under stack/ and loads each into an
// Element. values.yaml is parsed into a yaml.v3 Node tree, which keeps source
// positions for findings, and the typed Component is decoded from that tree.
package chart

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Element is a single chart under stack/<name>/ or the sample-element/.
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
	// Typed values; only the keys the checks inspect.
	Values *Values
	// yaml.v3 root node for values.yaml, used for finding line/column.
	ValuesDoc *yaml.Node
	// Key under `stack:`. Empty when values.yaml has no stack block.
	StackKey string
}

// Dependency is a single entry from Chart.yaml `dependencies:`.
type Dependency struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
}

type chartYaml struct {
	APIVersion   string       `yaml:"apiVersion"`
	Name         string       `yaml:"name"`
	Version      string       `yaml:"version"`
	Dependencies []Dependency `yaml:"dependencies"`
}

// Values is the typed view of `stack.<key>` plus the root-level fields the
// validator needs. Only fields the checks inspect are modelled.
type Values struct {
	PolicyNamespace string     // from root values.yaml (not per element)
	Component       *Component // values.stack[stackKey]
}

// Component mirrors the per-element block under `stack.<key>`.
//
// policy-library reads `enabled` and `default`. The legacy `enable` and `defaultPolicy` spellings
// are modelled separately so DeadKeyCheck can report them; an element using them renders wrong
// without any error.
type Component struct {
	Enabled             bool                `yaml:"enabled"`
	Policies            []Policy            `yaml:"policies"`
	ConfigPolicies      []SubPolicy         `yaml:"configPolicies"`
	OperatorPolicies    OperatorPolicies    `yaml:"operatorPolicies"`
	CertificatePolicies []CertificatePolicy `yaml:"certificatePolicies"`
	PolicySets          []PolicySet         `yaml:"policySets"`
	Default             *Default            `yaml:"default"`
	// Toggles overrides an entry's Enabled by name (chart 1.3.0).
	Toggles        map[string]bool `yaml:"toggles"`
	OrderPolicies  bool            `yaml:"orderPolicies"`
	OrderManifests bool            `yaml:"orderManifests"`

	// Legacy spellings the chart never reads, modelled only so they can be reported.
	LegacyEnable        *bool    `yaml:"enable"`
	LegacyDefaultPolicy *Default `yaml:"defaultPolicy"`
	// LegacyOperatorPolicies is set when operatorPolicies is the pre-3.0 list, which policy-library
	// 3.x rejects.
	LegacyOperatorPolicies bool `yaml:"-"`
}

// Default is the per-component defaults block. policy-library reads only
// categories/controls/standards from it; Severity, RemediationAction and Disabled are modelled so
// DeadKeyCheck can report that they have no effect here.
type Default struct {
	Severity          string `yaml:"severity"`
	RemediationAction string `yaml:"remediationAction"`
	Disabled          *bool  `yaml:"disabled"`
}

// CouldBeEnabled reports whether an entry is enabled, or is governed by a toggle and so may be
// enabled on some cluster. Structural checks use this instead of IsEnabled so that references inside
// a sub-feature that is off by default are still validated before a cluster turns it on.
func (c *Component) CouldBeEnabled(name string, declared bool) bool {
	if c != nil {
		if _, ok := c.Toggles[name]; ok {
			return true
		}
	}
	return c.IsEnabled(name, declared)
}

// IsToggled reports whether an entry's enablement is governed by the toggles map.
func (c *Component) IsToggled(name string) bool {
	if c == nil {
		return false
	}
	_, ok := c.Toggles[name]
	return ok
}

// IsEnabled returns an entry's effective enabled state. A toggle keyed by the entry's name
// overrides its declared `enabled` in either direction.
func (c *Component) IsEnabled(name string, declared bool) bool {
	if c == nil {
		return declared
	}
	if v, ok := c.Toggles[name]; ok {
		return v
	}
	return declared
}

// Policy is a parent ACM Policy entry.
type Policy struct {
	Name              string     `yaml:"name"`
	Enabled           bool       `yaml:"enabled"`
	Severity          string     `yaml:"severity"`
	RemediationAction string     `yaml:"remediationAction"`
	Dependencies      []DepEntry `yaml:"dependencies"`
	// OrderManifests overrides the component's orderManifests for this policy when set.
	OrderManifests *bool `yaml:"orderManifests"`
}

// DepEntry is one entry of policies[].dependencies or *.extraDependencies.
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

// SubPolicy is the union shape used by configPolicies (ConfigurationPolicy).
type SubPolicy struct {
	Name              string          `yaml:"name"`
	Enabled           bool            `yaml:"enabled"`
	PolicyRef         string          `yaml:"policyRef"`
	Severity          string          `yaml:"severity"`
	RemediationAction string          `yaml:"remediationAction"`
	ComplianceType    string          `yaml:"complianceType"`
	TemplateNames     []TemplateName  `yaml:"templateNames"`
	ExtraDependencies []DepEntry      `yaml:"extraDependencies"`
	WaitForOperator   WaitForOperator `yaml:"waitForOperator"`
	IgnorePending     bool            `yaml:"ignorePending"`
	RawTemplate       bool            `yaml:"rawTemplate"`
	// PruneObjectBehavior is passed through verbatim; ACM defaults it to None.
	PruneObjectBehavior string `yaml:"pruneObjectBehavior"`
}

// OperatorPolicy mirrors operatorPolicies entries. Name is the map key.
type OperatorPolicy struct {
	Name              string          `yaml:"name"`
	Enabled           bool            `yaml:"enabled"`
	PolicyRef         string          `yaml:"policyRef"`
	Severity          string          `yaml:"severity"`
	RemediationAction string          `yaml:"remediationAction"`
	ComplianceType    string          `yaml:"complianceType"`
	UpgradeApproval   string          `yaml:"upgradeApproval"`
	Versions          []string        `yaml:"versions"`
	Subscription      Subscription    `yaml:"subscription"`
	ExtraDependencies []DepEntry      `yaml:"extraDependencies"`
	WaitForOperator   WaitForOperator `yaml:"waitForOperator"`
	IgnorePending     bool            `yaml:"ignorePending"`
	// HasName reports an explicit `name:` in a map entry, which policy-library ignores.
	HasName bool `yaml:"-"`
}

// Subscription is the part of operatorPolicies.<key>.subscription the checks inspect.
type Subscription struct {
	StartingCSV         string `yaml:"startingCSV"`
	InstallPlanApproval string `yaml:"installPlanApproval"`
}

// OperatorPolicies is keyed by entry name (policy-library 3.x).
type OperatorPolicies map[string]OperatorPolicy

// UnmarshalYAML decodes the map shape, taking Name from the key. The pre-3.0 list shape is keyed by
// each entry's name, so older trees still load; Component.LegacyOperatorPolicies reports it.
func (o *OperatorPolicies) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.MappingNode:
		*o = OperatorPolicies{}
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
		*o = OperatorPolicies{}
		for _, p := range items {
			(*o)[p.Name] = p
		}
		return nil
	default:
		return fmt.Errorf("operatorPolicies: unexpected yaml kind %d at line %d", n.Kind, n.Line)
	}
}

// Sorted yields the entries in key order, so findings are deterministic.
func (o OperatorPolicies) Sorted() iter.Seq2[string, OperatorPolicy] {
	return func(yield func(string, OperatorPolicy) bool) {
		for _, k := range slices.Sorted(maps.Keys(o)) {
			if !yield(k, o[k]) {
				return
			}
		}
	}
}

// CertificatePolicy mirrors certificatePolicies entries.
type CertificatePolicy struct {
	Name              string          `yaml:"name"`
	Enabled           bool            `yaml:"enabled"`
	PolicyRef         string          `yaml:"policyRef"`
	Severity          string          `yaml:"severity"`
	RemediationAction string          `yaml:"remediationAction"`
	ExtraDependencies []DepEntry      `yaml:"extraDependencies"`
	WaitForOperator   WaitForOperator `yaml:"waitForOperator"`
	IgnorePending     bool            `yaml:"ignorePending"`
}

// WaitForOperator accepts either a single operator name or a list of them.
type WaitForOperator []string

// UnmarshalYAML coerces a bare scalar into a one-element list.
func (w *WaitForOperator) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Value != "" {
			*w = WaitForOperator{n.Value}
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
		return fmt.Errorf("waitForOperator: unexpected yaml kind %d at line %d", n.Kind, n.Line)
	}
}

// PolicySet groups policies into a PolicySet ACM resource.
type PolicySet struct {
	Name     string   `yaml:"name"`
	Enabled  bool     `yaml:"enabled"`
	Policies []string `yaml:"policies"`
}

// TemplateName accepts both forms documented in chart-readme.md:
// "templateNames: [foo]" and "templateNames: [{name: foo}]".
type TemplateName struct {
	Name string `yaml:"name"`
}

// UnmarshalYAML coerces a bare scalar into a TemplateName{Name: scalar}.
func (t *TemplateName) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		t.Name = n.Value
		return nil
	}
	if n.Kind == yaml.MappingNode {
		type raw TemplateName
		return n.Decode((*raw)(t))
	}
	return fmt.Errorf("templateNames entry: unexpected yaml kind %d at line %d", n.Kind, n.Line)
}

// LoadAll loads every element chart under stackDir, plus sampleDir when it
// contains a Chart.yaml, and returns policyNamespace from rootValuesFile.
func LoadAll(stackDir, sampleDir, rootValuesFile string) ([]*Element, string, error) {
	ns, err := loadPolicyNamespace(rootValuesFile)
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

	stackKey, comp, err := decodeComponent(&doc)
	if err != nil {
		return nil, err
	}

	el := &Element{
		ChartName:     cy.Name,
		Dir:           dir,
		ValuesFile:    valuesFile,
		ConvertersDir: filepath.Join(dir, "converters"),
		Dependencies:  cy.Dependencies,
		Values: &Values{
			PolicyNamespace: policyNamespace,
			Component:       comp,
		},
		ValuesDoc: &doc,
		StackKey:  stackKey,
	}
	return el, nil
}

// decodeComponent finds the single key under `stack:` in values.yaml and
// decodes that subtree into a Component.
func decodeComponent(doc *yaml.Node) (string, *Component, error) {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return "", nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return "", nil, nil
	}
	stackNode := mapValue(root, "stack")
	if stackNode == nil || stackNode.Kind != yaml.MappingNode {
		return "", nil, nil
	}
	if len(stackNode.Content) < 2 {
		return "", nil, nil
	}
	// One element per chart, so the first key under stack: is the element.
	keyNode := stackNode.Content[0]
	valueNode := stackNode.Content[1]
	var c Component
	if err := valueNode.Decode(&c); err != nil {
		return keyNode.Value, nil, fmt.Errorf("decode stack.%s: %w", keyNode.Value, err)
	}
	c.LegacyOperatorPolicies = IsLegacyOperatorPolicies(mapValue(valueNode, "operatorPolicies"))
	return keyNode.Value, &c, nil
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

func loadPolicyNamespace(path string) (string, error) {
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
	out := strings.ToLower(parts[0])
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		out += strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	}
	return out
}
