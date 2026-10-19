package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

func TestPolicy036_RemediationOverride(t *testing.T) {
	policy := func(name, action string) string {
		s := "      - name: " + name + "\n        enabled: true\n"
		if action != "" {
			s += "        remediationAction: " + action + "\n"
		}
		return s
	}
	entry := func(name, ref, action string) string {
		s := "      - name: " + name + "\n        enabled: true\n        policyRef: " + ref + "\n"
		if action != "" {
			s += "        remediationAction: " + action + "\n"
		}
		return s
	}
	operator := func(name, ref, action string) string {
		return "      " + name + ":\n        enabled: true\n        policyRef: " + ref + "\n        remediationAction: " + action + "\n"
	}
	cases := []struct {
		name, body string
		want       []string // message prefixes, one per finding
	}{
		{"template inform under enforce", "    policies:\n" + policy("a", "enforce") + "    configPolicies:\n" + entry("x", "a", "inform"),
			[]string{`configPolicies[0] "x": remediationAction inform is overridden by policies[0].remediationAction enforce: ACM applies the Policy's action to every template, so this one runs as enforce`}},
		{"template enforce under inform", "    policies:\n" + policy("b", "") + policy("a", "inform") + "    configPolicies:\n" + entry("x", "a", "enforce"),
			[]string{`configPolicies[0] "x": remediationAction enforce is overridden by policies[1].remediationAction inform`}},
		{"same action", "    policies:\n" + policy("a", "enforce") + "    configPolicies:\n" + entry("x", "a", "enforce"), nil},
		{"case-insensitive", "    policies:\n" + policy("a", "Enforce") + "    configPolicies:\n" + entry("x", "a", "enforce"), nil},
		{"policy unset", "    policies:\n" + policy("a", "") + "    configPolicies:\n" + entry("x", "a", "enforce"), nil},
		{"template unset under enforce", "    policies:\n" + policy("a", "enforce") + "    configPolicies:\n" + entry("x", "a", ""),
			[]string{`configPolicies[0] "x": remediationAction inform is overridden by policies[0].remediationAction enforce`}},
		{"template unset under inform", "    policies:\n" + policy("a", "inform") + "    configPolicies:\n" + entry("x", "a", ""), nil},
		{"unknown policyRef", "    policies:\n" + policy("a", "enforce") + "    configPolicies:\n" + entry("x", "b", "inform"), nil},
		{"first of duplicate policies", "    policies:\n" + policy("a", "enforce") + policy("a", "inform") + "    configPolicies:\n" + entry("x", "a", "inform"),
			[]string{`configPolicies[0] "x": remediationAction inform is overridden by policies[0].remediationAction enforce`}},
		{"several templates", "    policies:\n" + policy("a", "enforce") + "    configPolicies:\n" + entry("w", "a", "") + entry("x", "a", "inform") + entry("y", "a", "INFORM"),
			[]string{`configPolicies[0] "w"`, `configPolicies[1] "x"`, `configPolicies[2] "y": remediationAction INFORM`}},

		{"disabled template", "    policies:\n" + policy("a", "enforce") +
			"    configPolicies:\n      - name: x\n        enabled: false\n        policyRef: a\n        remediationAction: inform\n", nil},
		{"toggled-off template", "    toggles:\n      x: false\n    policies:\n" + policy("a", "enforce") + "    configPolicies:\n" + entry("x", "a", "inform"),
			[]string{`configPolicies[0] "x"`}},
		{"toggled-on template", "    toggles:\n      x: true\n    policies:\n" + policy("a", "enforce") +
			"    configPolicies:\n      - name: x\n        enabled: false\n        policyRef: a\n        remediationAction: inform\n",
			[]string{`configPolicies[0] "x"`}},
		{"disabled parent", "    policies:\n      - name: a\n        enabled: false\n        remediationAction: enforce\n" +
			"    configPolicies:\n" + entry("x", "a", "inform"), nil},
		{"toggled-off parent", "    toggles:\n      a: false\n    policies:\n" + policy("a", "enforce") + "    configPolicies:\n" + entry("x", "a", "inform"),
			[]string{`configPolicies[0] "x"`}},

		{"operator inform under enforce", "    policies:\n" + policy("a", "enforce") + "    operatorPolicies:\n" + operator("op", "a", "inform"),
			[]string{"operatorPolicies.op: remediationAction inform is overridden by policies[0].remediationAction enforce"}},
		{"operator same action", "    policies:\n" + policy("a", "enforce") + "    operatorPolicies:\n" + operator("op", "a", "Enforce"), nil},
		{"operators sorted after configs", "    policies:\n" + policy("a", "inform") + "    operatorPolicies:\n" + operator("z", "a", "enforce") + operator("b", "a", "enforce") +
			"    configPolicies:\n" + entry("x", "a", "enforce"),
			[]string{`configPolicies[0] "x"`, "operatorPolicies.b:", "operatorPolicies.z:"}},
		{"toggled-off operator", "    toggles:\n      op: false\n    policies:\n" + policy("a", "enforce") + "    operatorPolicies:\n" + operator("op", "a", "inform"),
			[]string{"operatorPolicies.op:"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := loadElement(t, "stack:\n  myElement:\n    enabled: true\n"+tc.body, "", nil)
			got := (&RemediationCheck{}).Run(Context{Element: el})
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d findings, got %d: %+v", len(tc.want), len(got), got)
			}
			for i, w := range tc.want {
				f := got[i]
				if f.RuleID != "POLICY036" || f.Severity != SevError || !strings.HasPrefix(f.Message, w) {
					t.Errorf("finding %d = %+v, want error starting %q", i, f, w)
				}
				if f.File != el.ValuesFile || f.Element != "my-element" || f.Line == 0 {
					t.Errorf("finding %d location wrong: %+v", i, f)
				}
			}
		})
	}
}

// TestPolicy036_Engines checks each lifecycle engine as its own Element and locates the finding
// under that engine's stack key.
func TestPolicy036_Engines(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"Chart.yaml":         "apiVersion: v2\nname: lifecycle\nversion: 0.1.0\n",
		"values.schema.json": `{"properties": {"install": {"properties": {"engine": {"enum": ["hypershift", "hive"]}}}}}`,
		"values.yaml": `stack:
  hypershift:
    policies:
      - name: present
        enabled: true
        remediationAction: enforce
    configPolicies:
      - name: ready
        enabled: true
        policyRef: present
        remediationAction: enforce
  hive:
    policies:
      - name: present
        enabled: true
        remediationAction: enforce
    configPolicies:
      - name: ready
        enabled: true
        policyRef: present
        remediationAction: inform
`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	els, err := chart.LoadComponents(dir, "policy")
	if err != nil {
		t.Fatal(err)
	}
	var got []Finding
	for _, el := range els {
		got = append(got, (&RemediationCheck{}).Run(Context{Element: el})...)
	}
	if len(got) != 1 || got[0].Line != 21 || got[0].Element != "lifecycle" || !strings.HasPrefix(got[0].Message, `configPolicies[0] "ready": remediationAction inform`) {
		t.Errorf("got %+v, want one finding at line 21", got)
	}
}
