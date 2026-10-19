package checks

import (
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
    toggles:
      cm: true
    config:
      data: {}
      mode: a
    policies:
      - name: install
        enabled: true
    configPolicies:
      - name: cm
        enabled: true
        policyRef: install
    operatorPolicies:
      my-op:
        enabled: true
        policyRef: install
`, "", nil)
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
    toggles: {install: true, cm: false, my-op: true}
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
    toggles:
      confg: true
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
				`stack.myElement.toggles.confg names no entry of the element`,
				`stack.myElement.config.dta is not in the element's values.yaml`,
				`stack.bare.config.anything is not in the element's values.yaml`,
			},
			lines: []int{1, 3, 5, 9, 11, 14},
		},
		{
			name: "a toggle may name an entry the layer itself declares",
			layer: `stack:
  myElement:
    toggles: {extra: true}
    configPolicies:
      - name: extra
        policyRef: install
`,
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
