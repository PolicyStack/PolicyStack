package checks

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/cascade"
)

// testCRDs returns the committed CRDs in crds/, loaded once.
var testCRDs = sync.OnceValues(func() (CRDs, error) { return LoadCRDs("../../crds") })

func crds(t *testing.T) CRDs {
	t.Helper()
	c, err := testCRDs()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func configPolicy(name, spec string) string {
	return "    - objectDefinition:\n        apiVersion: policy.open-cluster-management.io/v1\n        kind: ConfigurationPolicy\n" +
		"        metadata: {name: " + name + "}\n        spec:\n" + spec +
		"          object-templates:\n            - complianceType: musthave\n              objectDefinition: {apiVersion: v1, kind: Namespace, metadata: {name: x}}\n"
}

const operatorPolicy = `    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1beta1
        kind: OperatorPolicy
        metadata: {name: install-op-spoke}
        spec:
          complianceType: musthave
          remediationAction: enforce
          upgradeApproval: Automatic
          subscription: {name: op, namespace: op}
          upgradeAproval: None
`

func TestPolicy080_CRDSchema(t *testing.T) {
	const values = `stack:
  myElement:
    enabled: true
    policies:
      a:
        remediationAction: enforce
        templates:
          ns:
            severity: high
    operatorPolicies:
      op:
        upgradeApproval: Automatic
`
	policy := func(spec string, templates ...string) string {
		return "---\napiVersion: policy.open-cluster-management.io/v1\nkind: Policy\nmetadata:\n  name: a-my-element-spoke\n" +
			"  namespace: policy\nspec:\n  disabled: false\n" + spec + "  policy-templates:\n" + strings.Join(templates, "")
	}
	type want struct {
		msg  string
		line int
	}
	cases := []struct {
		name, rendered string
		want           []want
	}{
		{"valid", policy("  remediationAction: enforce\n", configPolicy("a-ns", "          remediationAction: enforce\n          severity: high\n")), nil},
		{"defaulted remediationAction", policy("", configPolicy("a-ns", "")), nil},
		// regression: the framework addon creates InformOnly templates as inform; the CRD enum has no InformOnly.
		{"InformOnly", policy("  remediationAction: enforce\n", configPolicy("a-ns", "          remediationAction: InformOnly\n")), nil},
		{"informonly", policy("", configPolicy("a-ns", "          remediationAction: informonly\n")), nil},
		// regression: a typo the chart passes through verbatim.
		{"remediationAction typo", policy("", configPolicy("a-ns", "          remediationAction: enforec\n")),
			[]want{{`policies.a.templates.ns: spec.remediationAction: Unsupported value: "enforec"`, 9}}},
		{"severity typo", policy("", configPolicy("a-ns", "          severity: hgih\n")),
			[]want{{`policies.a.templates.ns: spec.severity: Unsupported value: "hgih"`, 9}}},
		{"unknown field", policy("", configPolicy("a-ns", "          pruneObjectBehaviour: DeleteAll\n")),
			[]want{{`policies.a.templates.ns: spec.pruneObjectBehaviour: unknown field`, 9}}},
		{"suffixed sub-policy", policy("", configPolicy("a-ns-spoke", "          severity: hgih\n")),
			[]want{{`policies.a.templates.ns: spec.severity`, 9}}},
		{"OperatorPolicy", policy("", operatorPolicy), []want{{"operatorPolicies.op: spec.upgradeAproval: unknown field", 12}}},
		{"Policy remediationAction typo", policy("  remediationAction: enforec\n", configPolicy("a-ns", "")),
			[]want{{`policies.a: spec.remediationAction: Unsupported value: "enforec"`, 6}}},
		// The addon normalizes templates only: the Policy enum rejects InformOnly.
		{"Policy InformOnly", policy("  remediationAction: InformOnly\n", configPolicy("a-ns", "")),
			[]want{{`policies.a: spec.remediationAction: Unsupported value: "InformOnly"`, 6}}},
		{"Policy type error", policy("  copyPolicyMetadata: \"no\"\n", configPolicy("a-ns", "")),
			[]want{{`policies.a: spec.copyPolicyMetadata: Invalid value: "string"`, 6}}},
		{"Placement", "---\napiVersion: cluster.open-cluster-management.io/v1beta1\nkind: Placement\nmetadata:\n  name: a-my-element-spoke\nspec:\n  numberOfCluster: 1\n",
			[]want{{`Placement "a-my-element-spoke": spec.numberOfCluster: unknown field`, 0}}},
		{"unknown sub-policy", policy("", configPolicy("other", "          severity: hgih\n")),
			[]want{{`ConfigurationPolicy "other": spec.severity`, 0}}},
		{"no CRD", policy("", "    - objectDefinition: {apiVersion: constraints.gatekeeper.sh/v1beta1, kind: K8sRequiredLabels, metadata: {name: l}}\n") +
			"---\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: c}\n",
			[]want{{"no CRD for constraints.gatekeeper.sh/v1beta1/K8sRequiredLabels, v1/ConfigMap: not schema-checked", 0}}},
		{"empty documents", "\n---\n# comment\n---\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := loadElement(t, values, "", nil)
			got := (&CRDSchemaCheck{CRDs: crds(t)}).Run(renderCtx(t, el, tc.rendered))
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d findings, got %d: %+v", len(tc.want), len(got), got)
			}
			for i, w := range tc.want {
				f := got[i]
				if f.RuleID != "POLICY080" || !strings.HasPrefix(f.Message, w.msg) || f.File != el.ValuesFile || f.Line != w.line || f.Cluster != "" {
					t.Errorf("finding %d = %+v, want %q at line %d", i, f, w.msg, w.line)
				}
				if wantSev := map[bool]Severity{true: SevWarning, false: SevError}[strings.HasPrefix(w.msg, "no CRD")]; f.Severity != wantSev {
					t.Errorf("finding %d severity %v, want %v", i, f.Severity, wantSev)
				}
			}
		})
	}
}

// TestPolicy080_Layer: a defect a values layer introduces is reported at the layer.
func TestPolicy080_Layer(t *testing.T) {
	const values = `stack:
  myElement:
    policies:
      a:
        templates:
          ns: {}
    operatorPolicies:
      op:
        upgradeApproval: Automatic
`
	op := strings.Replace(operatorPolicy, "upgradeAproval: None", "severity: hihg", 1)
	cases := []struct {
		name, rendered, layer string
		want                  []want
	}{
		// Helm merges maps: the layer sets one field of a template values.yaml declares.
		{"template field", policyDoc("a", "  disabled: false\n", configPolicy("a-ns", "          severity: hgih\n")),
			"stack:\n  myElement:\n    policies:\n      a:\n        templates:\n          ns:\n            severity: hgih\n",
			[]want{{SevError, `policies.a.templates.ns: spec.severity: Unsupported value: "hgih"`, inLayer, 7}}},
		// Helm merges maps: the layer sets one field of an entry values.yaml defines.
		{"map field", policyDoc("a", "  disabled: false\n", op),
			"stack:\n  myElement:\n    operatorPolicies:\n      op:\n        severity: hihg\n",
			[]want{{SevError, `operatorPolicies.op: spec.severity: Unsupported value: "hihg"`, inLayer, 5}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := renderCtx(t, loadElement(t, values, "", nil), tc.rendered, tc.layer)
			assertFindings(t, "POLICY080", ctx, (&CRDSchemaCheck{CRDs: crds(t)}).Run(ctx), tc.want)
		})
	}

	// A template no values file sets is reported at its converter.
	el := loadElement(t, values, "", []string{"b/cm.yaml"})
	got := (&CRDSchemaCheck{CRDs: crds(t)}).Run(renderCtx(t, el, policyDoc("b", "  disabled: false\n", configPolicy("b-cm", "          severity: hgih\n"))))
	if len(got) != 1 || got[0].File != filepath.Join(el.ConvertersDir, "b/cm.yaml") || got[0].Line != 1 || !strings.HasPrefix(got[0].Message, "policies.b.templates.cm: spec.severity") {
		t.Errorf("got %+v, want one finding at converters/b/cm.yaml:1", got)
	}
}

func TestPolicy080_Validated(t *testing.T) {
	c := &CRDSchemaCheck{CRDs: crds(t)}
	el := loadElement(t, "stack:\n  myElement: {}\n", "", nil)
	rendered := renderedPolicy("p", configPolicy("a", ""), operatorPolicy)
	c.Run(Context{Element: el, Cluster: &cascade.Resolved{ReleaseName: "my-element-spoke"}, Rendered: []byte(rendered)})
	if got := c.Validated(); got != 3 {
		t.Errorf("validated %d objects, want 3", got)
	}
	if got := (&CRDSchemaCheck{}).Run(Context{Element: el, Rendered: []byte(rendered)}); got != nil {
		t.Errorf("without CRDs got %+v, want none", got)
	}
	got := c.Run(Context{Element: el, Rendered: []byte("kind: [")})
	if len(got) != 1 || !strings.HasPrefix(got[0].Message, "rendered output: ") {
		t.Errorf("got %+v, want one rendered output finding", got)
	}
}

func TestLoadCRDs(t *testing.T) {
	if _, err := LoadCRDs(t.TempDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("empty dir: got %v, want fs.ErrNotExist", err)
	}
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "crd.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("kind: [")
	if _, err := LoadCRDs(dir); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("malformed: got %v, want a parse error", err)
	}
	write(`apiVersion: v1
kind: ConfigMap
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
spec:
  group: example.com
  names: {kind: Widget}
  versions:
    - {name: v1, served: true, schema: {openAPIV3Schema: {type: object, properties: {spec: {type: object, properties: {size: {type: integer}}}}}}}
    - {name: v0, served: false, schema: {openAPIV3Schema: {type: object}}}
`)
	got, err := LoadCRDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d versions, want the served one", len(got))
	}
	for _, v := range got {
		if errs := v.validate(map[string]any{"spec": map[string]any{"size": int64(3)}}); len(errs) != 0 {
			t.Errorf("valid integer: %v", errs)
		}
		if errs := v.validate(map[string]any{"spec": map[string]any{"size": "3"}}); len(errs) != 1 {
			t.Errorf("string for integer: got %v, want one error", errs)
		}
	}
}

func TestLeaf(t *testing.T) {
	for err, want := range map[string]string{
		`spec.remediationAction: Unsupported value: "enforec"`:  "remediationAction",
		"spec.object-templates[0].complianceTyp: unknown field": "complianceTyp",
		"spec.ipv4[2][0]: Invalid value":                        "ipv4",
		"spec: Required value":                                  "spec",
		"no path":                                               "",
	} {
		if got := leaf(err); got != want {
			t.Errorf("leaf(%q) = %q, want %q", err, got, want)
		}
	}
}
