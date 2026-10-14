package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/cascade"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

func TestOperatorPolicies_Decode(t *testing.T) {
	cases := []struct {
		name, body string
		legacy     bool
		hasName    bool
	}{
		{"map takes name from key", "      op:\n        policyRef: p\n", false, false},
		{"stray name keeps key", "      op:\n        name: other\n        policyRef: p\n", false, true},
		{"legacy list keyed by name", "      - name: op\n        policyRef: p\n", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := loadElement(t, "stack:\n  myElement:\n    operatorPolicies:\n"+tc.body, "", nil)
			comp := el.Values.Component
			op, ok := comp.OperatorPolicies["op"]
			if !ok || op.Name != "op" || op.PolicyRef != "p" || op.HasName != tc.hasName {
				t.Fatalf("got %+v", comp.OperatorPolicies)
			}
			if comp.LegacyOperatorPolicies != tc.legacy {
				t.Errorf("LegacyOperatorPolicies = %v, want %v", comp.LegacyOperatorPolicies, tc.legacy)
			}
		})
	}
}

func TestOperatorPolicies_DuplicateKey(t *testing.T) {
	var ops chart.OperatorPolicies
	err := yaml.Unmarshal([]byte("op:\n  policyRef: p\nop:\n  policyRef: q\n"), &ops)
	if err == nil || !strings.Contains(err.Error(), `line 3: operatorPolicies key "op" already defined`) {
		t.Fatalf("err = %v", err)
	}
}

func TestPolicy030_OperatorPolicies(t *testing.T) {
	cases := []struct {
		name, entry string
		want        string // "" means no findings
		sev         Severity
	}{
		{"unpinned", "upgradeApproval: None\n        versions: []", "", 0},
		{"pinned", "upgradeApproval: Automatic\n        versions:\n          - op.v1.2.3\n        subscription:\n          startingCSV: op.v1.2.3", "", 0},
		{"comma-separated and templated", "versions:\n          - 'op.v1.2.3, op.v1.2.4'\n          - '{{ fromConfigMap \"x\" \"y\" \"csv\" }}'", "", 0},
		{"stray name", "name: op", ".name is ignored", SevWarning},
		{"Manual", "upgradeApproval: Manual", `.upgradeApproval = "Manual"; allowed: [None Automatic]`, SevError},
		{"installPlanApproval", "subscription:\n          installPlanApproval: Manual", "ACM rejects it", SevError},
		{"bad version", "versions:\n          - Op_v1", `.versions[0]: "Op_v1" is not a valid CSV name`, SevError},
		{"bad comma entry", "versions:\n          - op.v1, op v2", `"op v2" is not a valid CSV name`, SevError},
		{"bad remediationAction", "remediationAction: Enforce", `.remediationAction = "Enforce"`, SevError},
		{"comma in startingCSV", "subscription:\n          startingCSV: op.v1,op.v2", `"op.v1,op.v2" is not a valid CSV name`, SevError},
		{"bad startingCSV", "subscription:\n          startingCSV: op.v1.", `.subscription.startingCSV: "op.v1." is not a valid CSV name`, SevError},
		{"too long", "versions:\n          - " + strings.Repeat("a", 254), "is not a valid CSV name", SevError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := loadElement(t, "stack:\n  myElement:\n    operatorPolicies:\n      op:\n        "+tc.entry+"\n", "", nil)
			got := (&EnumCheck{}).Run(Context{Element: el})
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("expected no findings, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("expected 1 finding, got %d: %+v", len(got), got)
			}
			if !strings.HasPrefix(got[0].Message, "operatorPolicies.op") || !strings.Contains(got[0].Message, tc.want) {
				t.Errorf("message %q does not contain %q", got[0].Message, tc.want)
			}
			if got[0].Severity != tc.sev || got[0].Line == 0 {
				t.Errorf("unexpected finding: %+v", got[0])
			}
		})
	}
}

func TestPolicy032_ValuesLayers(t *testing.T) {
	const defaults = `
stack:
  myElement:
    enabled: true
    policies:
      - name: p
        enabled: true
    operatorPolicies:
      op:
        enabled: true
        policyRef: p
        upgradeApproval: None
        versions: []
`
	cases := []struct {
		name, layer string
		want        string // "" means no findings
		line        int
	}{
		{"valid pin", "stack:\n  myElement:\n    operatorPolicies:\n      op:\n        upgradeApproval: Automatic\n        versions: [op.v1.2.3]\n", "", 0},
		{"other element", "stack:\n  other:\n    operatorPolicies:\n      typo: {}\n", "", 0},
		{"no operatorPolicies", "stack:\n  myElement:\n    enabled: false\n", "", 0},
		{"typo key", "stack:\n  myElement:\n    operatorPolicies:\n      opp:\n        versions: [op.v1.2.3]\n", "operatorPolicies.opp is not in the element's values.yaml", 5},
		{"bad value", "stack:\n  myElement:\n    operatorPolicies:\n      op:\n        versions:\n          - op.v1.2.3\n          - OP\n", `operatorPolicies.op.versions[1]: "OP" is not a valid CSV name`, 7},
		{"bad enum", "stack:\n  myElement:\n    operatorPolicies:\n      op:\n        severity: urgent\n", `operatorPolicies.op.severity = "urgent"`, 5},
		{"legacy list", "stack:\n  myElement:\n    operatorPolicies:\n      - name: op\n", "requires a map keyed by entry name", 4},
		{"wrong type", "stack:\n  myElement:\n    operatorPolicies:\n      op:\n        versions: op.v1\n", "operatorPolicies: yaml:", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := loadElement(t, defaults, "", nil)
			layer := filepath.Join(t.TempDir(), "prod.yaml")
			if err := os.WriteFile(layer, []byte(tc.layer), 0o644); err != nil {
				t.Fatal(err)
			}
			cl := &cascade.Resolved{ClusterName: "x", ReleaseName: "my-element-x", ValueFiles: []string{el.ValuesFile, layer}}
			got := (&LayerOperatorCheck{}).Run(Context{Element: el, Cluster: cl})
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("expected no findings, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("expected 1 finding, got %d: %+v", len(got), got)
			}
			f := got[0]
			if !strings.Contains(f.Message, tc.want) {
				t.Errorf("message %q does not contain %q", f.Message, tc.want)
			}
			if f.RuleID != "POLICY032" || f.Severity != SevError || f.File != layer || f.Line != tc.line || f.Cluster != "" {
				t.Errorf("unexpected finding: %+v", f)
			}
		})
	}
}

func TestPolicy034_FleetKeysInLayers(t *testing.T) {
	cases := []struct {
		name, layer string
		lines       []int // one finding per line, version before channel
	}{
		{"neither", "stack: {}\n", nil},
		{"version", "stack: {}\nversion: 4.20.8\n", []int{2}},
		{"channel", "channel: stable-4.20\n", []int{1}},
		{"both", "channel: stable-4.20\nversion: 4.20.8\n", []int{2, 1}},
		{"nested keys are not top-level", "install:\n  version: 4.20.8\nstack:\n  x:\n    channel: a\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The element's own defaults may declare both keys.
			el := loadElement(t, "version: \"\"\nchannel: \"\"\nstack:\n  myElement: {}\n", "", nil)
			layer := filepath.Join(t.TempDir(), "prod.yaml")
			if err := os.WriteFile(layer, []byte(tc.layer), 0o644); err != nil {
				t.Fatal(err)
			}
			cl := &cascade.Resolved{ClusterName: "x", ValueFiles: []string{el.ValuesFile, layer}}
			got := (&LayerFleetKeyCheck{}).Run(Context{Element: el, Cluster: cl})
			if len(got) != len(tc.lines) {
				t.Fatalf("got %d findings, want %d: %+v", len(got), len(tc.lines), got)
			}
			for i, f := range got {
				if f.RuleID != "POLICY034" || f.File != layer || f.Line != tc.lines[i] || f.Cluster != "" || f.Element != "" {
					t.Errorf("unexpected finding: %+v", f)
				}
			}
			// The lifecycle chart's cascade ends with the fleet file, which sets both by design.
			el.Engines = []string{"hypershift"}
			if got := (&LayerFleetKeyCheck{}).Run(Context{Element: el, Cluster: cl}); len(got) != 0 {
				t.Errorf("lifecycle pair: got %+v, want none", got)
			}
		})
	}
}
