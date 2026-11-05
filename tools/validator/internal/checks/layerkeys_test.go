package checks

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
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

// TestComponentKeys compares componentKeys with definitions.component in the policy-library archive
// each element pins. The archives are gitignored, so it skips when none is fetched.
func TestComponentKeys(t *testing.T) {
	charts, _ := filepath.Glob("../../../../stack/*/Chart.yaml")
	seen := map[string]bool{}
	var archives []string
	for _, c := range charts {
		el, err := chart.Load(filepath.Dir(c), "policy")
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range el.Dependencies {
			a := filepath.Join(el.Dir, "charts", "policy-library-"+d.Version+".tgz")
			if _, err := os.Stat(a); d.Name == "policy-library" && err == nil && !seen[filepath.Base(a)] {
				seen[filepath.Base(a)] = true
				archives = append(archives, a)
			}
		}
	}
	if len(archives) == 0 {
		testenv.Skip(t, "no pinned policy-library archive under stack/*/charts")
	}
	for _, a := range archives {
		var schema struct {
			Definitions struct {
				Component struct {
					Properties           map[string]any `json:"properties"`
					AdditionalProperties *bool          `json:"additionalProperties"`
				} `json:"component"`
			} `json:"definitions"`
		}
		if err := json.Unmarshal(archiveFile(t, a, "policy-library/values.schema.json"), &schema); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		c := schema.Definitions.Component
		if c.AdditionalProperties == nil || *c.AdditionalProperties {
			t.Errorf("%s: definitions.component allows additional properties", filepath.Base(a))
		}
		if got, want := slices.Sorted(maps.Keys(c.Properties)), slices.Sorted(slices.Values(componentKeys)); !slices.Equal(got, want) {
			t.Errorf("%s: definitions.component properties\n got %q\nwant %q (componentKeys)", filepath.Base(a), got, want)
		}
	}
}

// archiveFile returns name from the gzipped tar at path.
func archiveFile(t *testing.T, path, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			t.Fatalf("%s: no %s", path, name)
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == name {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
	}
}
