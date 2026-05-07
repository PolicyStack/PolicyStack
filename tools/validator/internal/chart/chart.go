// Package chart discovers element charts under stack/ and parses each one
// into a strongly-typed Element. Source values.yaml is parsed twice: once
// as a typed struct (for check logic) and once as a yaml.v3 Node tree (for
// source-line-accurate findings).
package chart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Element is a single chart under stack/<name>/ or the sample-element/.
type Element struct {
	// Name from Chart.yaml — kebab-case, used as Helm Release.Name suffix.
	ChartName string
	// Absolute filesystem path of the element directory.
	Dir string
	// Absolute path of the element's values.yaml (chart defaults).
	ValuesFile string
	// Path to converters/ (may not exist).
	ConvertersDir string
	// Dependencies as declared in Chart.yaml.
	Dependencies []Dependency
	// Parsed values (typed; only the keys we care about).
	Values *Values
	// yaml.v3 root node for values.yaml — preserves line/column for findings.
	ValuesDoc *yaml.Node
	// Top-level key under `stack:` — may be empty if values.yaml has no stack.
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

// Values is the typed view of `stack.<key>` plus root-level fields we need.
// We only model the fields the validator inspects; helm sees the full map.
type Values struct {
	PolicyNamespace string             // from root values.yaml (not per element)
	Component       *Component         // values.stack[stackKey]
}

// Component mirrors the per-element block under `stack.<key>`.
type Component struct {
	Enable              bool                 `yaml:"enable"`
	Policies            []Policy             `yaml:"policies"`
	ConfigPolicies      []SubPolicy          `yaml:"configPolicies"`
	OperatorPolicies    []OperatorPolicy     `yaml:"operatorPolicies"`
	CertificatePolicies []CertificatePolicy  `yaml:"certificatePolicies"`
	PolicySets          []PolicySet          `yaml:"policySets"`
	DefaultPolicy       *DefaultPolicy       `yaml:"defaultPolicy"`
}

// DefaultPolicy is the per-component defaults block. Enums also validated here.
type DefaultPolicy struct {
	Severity          string `yaml:"severity"`
	RemediationAction string `yaml:"remediationAction"`
}

// Policy is a parent ACM Policy entry.
type Policy struct {
	Name              string `yaml:"name"`
	Enabled           bool   `yaml:"enabled"`
	Severity          string `yaml:"severity"`
	RemediationAction string `yaml:"remediationAction"`
}

// SubPolicy is the union shape used by configPolicies (ConfigurationPolicy).
type SubPolicy struct {
	Name              string         `yaml:"name"`
	Enabled           bool           `yaml:"enabled"`
	PolicyRef         string         `yaml:"policyRef"`
	Severity          string         `yaml:"severity"`
	RemediationAction string         `yaml:"remediationAction"`
	ComplianceType    string         `yaml:"complianceType"`
	TemplateNames     []TemplateName `yaml:"templateNames"`
}

// OperatorPolicy mirrors operatorPolicies entries.
type OperatorPolicy struct {
	Name              string `yaml:"name"`
	Enabled           bool   `yaml:"enabled"`
	PolicyRef         string `yaml:"policyRef"`
	Severity          string `yaml:"severity"`
	RemediationAction string `yaml:"remediationAction"`
	ComplianceType    string `yaml:"complianceType"`
	UpgradeApproval   string `yaml:"upgradeApproval"`
}

// CertificatePolicy mirrors certificatePolicies entries.
type CertificatePolicy struct {
	Name              string `yaml:"name"`
	Enabled           bool   `yaml:"enabled"`
	PolicyRef         string `yaml:"policyRef"`
	Severity          string `yaml:"severity"`
	RemediationAction string `yaml:"remediationAction"`
}

// PolicySet groups policies into a PolicySet ACM resource.
type PolicySet struct {
	Name     string   `yaml:"name"`
	Enabled  bool     `yaml:"enabled"`
	Policies []string `yaml:"policies"`
}

// TemplateName supports the bare-string OR mapping form documented in
// chart-readme.md ("templateNames: [foo]" or "templateNames: [{name: foo}]").
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

// LoadAll discovers element charts under stackDir + sampleDir (if present)
// and the root policyNamespace from rootValuesFile.
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
	// Take the first key under stack: — convention is one element per chart.
	keyNode := stackNode.Content[0]
	valueNode := stackNode.Content[1]
	var c Component
	if err := valueNode.Decode(&c); err != nil {
		return keyNode.Value, nil, fmt.Errorf("decode stack.%s: %w", keyNode.Value, err)
	}
	return keyNode.Value, &c, nil
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

// CamelFromKebab converts a kebab-case chart name to the camelCase form used
// as the key under `stack:` (per chart-readme.md). Matches tools/create-element.sh.
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
