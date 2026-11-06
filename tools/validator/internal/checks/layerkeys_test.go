package checks

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

func TestPolicy035_LayerKeys(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enabled: false
    config:
      data: {}
      mode: a
    policies:
      extra:
        enabled: false
        templates:
          cm:
            file: _shared/cm
    operatorPolicies:
      my-op:
        subscription: {name: my-op}
`, "", []string{"config/cm.yaml", "_shared/cm.yaml"})
	bare := loadElement(t, `
stack:
  bare:
    enabled: false
`, "apiVersion: v2\nname: bare\nversion: 0.1.0\n", nil)
	elements := []*chart.Element{el, bare}

	tests := []struct {
		name  string
		layer string
		want  []string // finding messages
		lines []int
	}{
		{
			name: "known keys are silent",
			layer: `policyNamespace: pstack
global:
  remediationAction: inform
  catalogSources: {redhat-operators: cs-redhat}
placement: {}
install: {engine: hive}
stack:
  myElement:
    enabled: true
    policies:
      install: {disabled: true}
      config:
        enabled: false
        templates:
          cm: {enabled: true}
          added: {file: _shared/cm}
      extra: {enabled: true}
    operatorPolicies:
      my-op: {enabled: false}
    config:
      data: {k: v}
      mode: b
  bare:
    enabled: true
`,
		},
		{
			name: "typos",
			layer: `stak: {}
global:
  remediationActon: inform
stack:
  myElemnt:
    enabled: true
  myElement:
    policies:
      confg: {enabled: true}
      config:
        templates:
          cn: {enabled: false}
    config:
      dta: {}
  bare:
    config:
      anything: 1
`,
			want: []string{
				`top-level key "stak" is read by nothing`,
				`global.remediationActon is read by nothing`,
				`stack.myElemnt names no element`,
				`stack.myElement.policies.confg names no policy of the element`,
				`stack.myElement.policies.config.templates.cn names no template of the policy: add converters/config/cn.yaml or set file`,
				`stack.myElement.config.dta is not in the element's values.yaml`,
				`stack.bare.config.anything is not in the element's values.yaml`,
			},
			lines: []int{1, 3, 5, 9, 12, 14, 17},
		},
		{
			name: "every component key is accepted",
			layer: `stack:
  myElement:
    enabled: true
    default: {categories: [CM]}
    config: {}
    policies: {}
    operatorPolicies: {}
    policySets: []
    suffixTemplateNames: true
    usePolicySetsPlacements: false
    disablePlacements: false
`,
		},
		{
			name: "an unknown component key is reported at the layer",
			layer: `stack:
  bare:
    enabled: true
  myElement:
    enabeld: true
`,
			want:  []string{`stack.myElement.enabeld is not a policy-library component key`},
			lines: []int{5},
		},
		{
			name: "3.x keys are reported at the layer",
			layer: `stack:
  myElement:
    toggles: {extra: true}
    configPolicies: []
`,
			want:  []string{`stack.myElement.toggles is not a policy-library component key`, `stack.myElement.configPolicies is not a policy-library component key`},
			lines: []int{3, 4},
		},
		{
			name:  "invalid YAML is left to RENDER000",
			layer: "stack: [\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "layer.yaml")
			if err := os.WriteFile(file, []byte(tt.layer), 0o644); err != nil {
				t.Fatal(err)
			}
			got := (&LayerKeyCheck{}).Run(Context{AllElements: elements, Layers: []string{file, file + ".missing"}})
			var msgs []string
			var lines []int
			for _, f := range got {
				if f.RuleID != "POLICY035" || f.Severity != SevError || f.File != file {
					t.Errorf("unexpected finding shape: %+v", f)
				}
				msgs = append(msgs, f.Message)
				lines = append(lines, f.Line)
			}
			if !slices.Equal(msgs, tt.want) {
				t.Errorf("messages:\n got %q\nwant %q", msgs, tt.want)
			}
			if tt.lines != nil && !slices.Equal(lines, tt.lines) {
				t.Errorf("lines: got %v, want %v", lines, tt.lines)
			}
		})
	}
}

// TestComponentKeys compares componentKeys with definitions.component in lib/policy-library.
func TestComponentKeys(t *testing.T) {
	data, err := os.ReadFile("../../../../lib/policy-library/values.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Definitions struct {
			Component struct {
				Properties           map[string]any `json:"properties"`
				AdditionalProperties *bool          `json:"additionalProperties"`
			} `json:"component"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	c := schema.Definitions.Component
	if c.AdditionalProperties == nil || *c.AdditionalProperties {
		t.Error("definitions.component allows additional properties")
	}
	if got, want := slices.Sorted(maps.Keys(c.Properties)), slices.Sorted(slices.Values(componentKeys)); !slices.Equal(got, want) {
		t.Errorf("definitions.component properties\n got %q\nwant %q (componentKeys)", got, want)
	}
}
