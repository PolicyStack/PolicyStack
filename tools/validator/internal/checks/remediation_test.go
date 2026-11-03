package checks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

func TestPolicy036_RemediationOverride(t *testing.T) {
	const values = "stack:\n  myElement:\n    enabled: true\n    policies:\n" +
		"      - name: a\n        remediationAction: enforce\n" + // line 6
		"    configPolicies:\n      - name: x\n        policyRef: a\n"
	action := func(a string) string { return "          remediationAction: " + a + "\n" }
	cfg := func(name, a string) string { return template("ConfigurationPolicy", name, "", action(a)) }
	const enforce = "  remediationAction: enforce\n"
	cases := []struct {
		name, rendered string
		layers         []string
		want           []want
	}{
		{"template inform under enforce", policyDoc("a", enforce, cfg("a-x", "inform")), nil,
			[]want{{SevError, `policies[0] "a": remediationAction enforce overrides ConfigurationPolicy "a-x"'s inform: ACM applies the Policy's action to every template, so it runs as enforce`, inValues, 6}}},
		{"template enforce under inform", policyDoc("a", "  remediationAction: inform\n", cfg("a-x", "enforce")), nil,
			[]want{{SevError, `policies[0] "a": remediationAction inform overrides ConfigurationPolicy "a-x"'s enforce`, inValues, 6}}},
		{"InformOnly", policyDoc("a", enforce, cfg("a-x", "InformOnly"), cfg("a-y", "informonly")), nil, nil},
		{"same action, any case", policyDoc("a", "  remediationAction: Enforce\n", cfg("a-x", "enforce")), nil, nil},
		{"policy unset", policyDoc("a", "", cfg("a-x", "enforce")), nil, nil},
		{"several templates", policyDoc("a", enforce, cfg("a-w", "enforce"), cfg("a-x", "inform"), cfg("a-y", "INFORM")), nil,
			[]want{{SevError, `policies[0] "a": remediationAction enforce overrides ConfigurationPolicy "a-x"'s inform`, inValues, 6},
				{SevError, `policies[0] "a": remediationAction enforce overrides ConfigurationPolicy "a-y"'s INFORM`, inValues, 6}}},
		{"OperatorPolicy", policyDoc("a", enforce, template("OperatorPolicy", "a-op", "", action("inform"))), nil,
			[]want{{SevError, `policies[0] "a": remediationAction enforce overrides OperatorPolicy "a-op"'s inform`, inValues, 6}}},
		{"operator templates policy-library generates", policyDoc("a", enforce,
			cfg("a-op-ns", "inform"), template("OperatorPolicy", "a-op", "", action("enforce")), cfg("a-op-status", "inform")), nil, nil},
		{"CertificatePolicy", policyDoc("a", enforce, template("CertificatePolicy", "a-c", "", action("inform"))), nil, nil},
		{"dry run", policyDoc("a", "  remediationAction: inform\n", cfg("a-x", "enforce")), []string{"global:\n  remediationAction: inform\n"}, nil},
		{"not from values", policyDoc("b", enforce, cfg("b-x", "inform")), nil,
			[]want{{SevError, `Policy "b-my-element-spoke": remediationAction enforce overrides`, nowhere, 0}}},
		// regression: 9ddc38f, a layer restating a Policy-level enforce over an inform gate.
		{"layer restates enforce", policyDoc("a", enforce, cfg("a-x", "inform")),
			[]string{"stack:\n  myElement:\n    policies:\n      - name: a\n        remediationAction: enforce\n"},
			[]want{{SevError, `policies[0] "a": remediationAction enforce overrides ConfigurationPolicy "a-x"'s inform`, inLayer, 5}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := renderCtx(t, loadElement(t, values, "", nil), tc.rendered, tc.layers...)
			assertFindings(t, "POLICY036", ctx, (&RemediationCheck{}).Run(ctx), tc.want)
		})
	}
}

// TestPolicy036_Engines locates the finding under the rendered lifecycle engine's stack key.
func TestPolicy036_Engines(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"Chart.yaml":         "apiVersion: v2\nname: lifecycle\nversion: 0.1.0\n",
		"values.schema.json": `{"properties": {"install": {"properties": {"engine": {"enum": ["hypershift", "hive"]}}}}}`,
		"values.yaml": `stack:
  hypershift:
    policies:
      - name: present
        remediationAction: enforce
  hive:
    policies:
      - name: present
        remediationAction: enforce
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
	rendered := "---\nkind: Policy\nmetadata: {name: present-lifecycle-spoke, namespace: policy}\nspec:\n  remediationAction: enforce\n  policy-templates:\n" +
		template("ConfigurationPolicy", "present-ready", "", "          remediationAction: inform\n")
	ctx := renderCtx(t, els[1], rendered)
	assertFindings(t, "POLICY036", ctx, (&RemediationCheck{}).Run(ctx),
		[]want{{SevError, `policies[0] "present": remediationAction enforce overrides`, inValues, 9}})
}
