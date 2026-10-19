package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/cascade"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

// Rendered Policy fragments, as policy-library emits them.
const (
	tplEmptyRaw = `    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata: {name: a-raw}
        spec:
          remediationAction: inform
          object-templates-raw: |
            []
`
	tplBlankRaw = `    - objectDefinition:
        kind: ConfigurationPolicy
        metadata: {name: a-blank}
        spec:
          object-templates-raw: "  \n"
`
	tplNullObjects = `    - objectDefinition:
        kind: ConfigurationPolicy
        metadata: {name: a-null}
        spec:
          object-templates:
`
	tplEmptyObjects = `    - objectDefinition:
        kind: ConfigurationPolicy
        metadata: {name: a-empty}
        spec:
          object-templates: []
`
	tplNoObjects = `    - objectDefinition:
        kind: ConfigurationPolicy
        metadata: {name: a-none}
        spec:
          remediationAction: inform
`
	tplObjects = `    - objectDefinition:
        kind: ConfigurationPolicy
        metadata: {name: a-objects}
        spec:
          object-templates:
            - complianceType: musthave
              objectDefinition: {apiVersion: v1, kind: Namespace, metadata: {name: x}}
`
	tplRaw = `    - objectDefinition:
        kind: ConfigurationPolicy
        metadata: {name: a-raw}
        spec:
          object-templates-raw: |
            {{- range $k, $v := (lookup "v1" "Node" "" "").items }}
            - complianceType: musthave
            {{- end }}
`
	tplOperator = `    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1beta1
        kind: OperatorPolicy
        metadata: {name: a-op}
        spec: {complianceType: musthave}
`
	tplCertificate = `    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: CertificatePolicy
        metadata: {name: a-cert}
        spec: {minimumDuration: 300h}
`
)

func renderedPolicy(name string, templates ...string) string {
	return "---\napiVersion: policy.open-cluster-management.io/v1\nkind: Policy\nmetadata:\n  name: " + name +
		"\n  namespace: policy\nspec:\n  policy-templates:\n" + strings.Join(templates, "")
}

func TestPolicy082_EmptyPolicy(t *testing.T) {
	const values = "stack:\n  myElement:\n    enabled: true\n    policies:\n      - name: a\n        enabled: true\n      - name: b\n        enabled: true\n"
	const placement = "---\napiVersion: cluster.open-cluster-management.io/v1beta1\nkind: Placement\nmetadata:\n  name: a-my-element-spoke\nspec: {}\n"
	cases := []struct {
		name, rendered string
		want           []string // message prefixes, one per finding
		lines          []int
	}{
		{"raw []", renderedPolicy("a-my-element-spoke", tplEmptyRaw), []string{`policies[0] "a" checks nothing`}, []int{5}},
		{"raw blank", renderedPolicy("a-my-element-spoke", tplBlankRaw), []string{`policies[0] "a"`}, []int{5}},
		{"object-templates null", renderedPolicy("a-my-element-spoke", tplNullObjects), []string{`policies[0] "a"`}, []int{5}},
		{"object-templates []", renderedPolicy("a-my-element-spoke", tplEmptyObjects), []string{`policies[0] "a"`}, []int{5}},
		{"no object-templates", renderedPolicy("a-my-element-spoke", tplNoObjects), []string{`policies[0] "a"`}, []int{5}},
		{"every template empty", renderedPolicy("b-my-element-spoke", tplEmptyRaw, tplEmptyObjects, tplNoObjects), []string{`policies[1] "b"`}, []int{7}},

		{"objects", renderedPolicy("a-my-element-spoke", tplObjects), nil, nil},
		{"raw with objects", renderedPolicy("a-my-element-spoke", tplRaw), nil, nil},
		{"partly empty", renderedPolicy("a-my-element-spoke", tplEmptyRaw, tplObjects, tplNullObjects), nil, nil},
		{"OperatorPolicy", renderedPolicy("a-my-element-spoke", tplEmptyRaw, tplOperator), nil, nil},
		{"CertificatePolicy", renderedPolicy("a-my-element-spoke", tplCertificate, tplEmptyObjects), nil, nil},
		{"other kinds", placement, nil, nil},

		{"one of two Policies", placement + renderedPolicy("a-my-element-spoke", tplObjects) + renderedPolicy("b-my-element-spoke", tplEmptyRaw),
			[]string{`policies[1] "b"`}, []int{7}},
		{"both Policies", renderedPolicy("a-my-element-spoke", tplEmptyRaw) + renderedPolicy("b-my-element-spoke", tplNullObjects),
			[]string{`policies[0] "a"`, `policies[1] "b"`}, []int{5, 7}},
		{"Policy not in values.yaml", renderedPolicy("c-my-element-spoke", tplEmptyRaw), []string{`Policy "c-my-element-spoke" checks nothing`}, []int{0}},
		{"other release", renderedPolicy("a-my-element-other", tplEmptyRaw), []string{`Policy "a-my-element-other"`}, []int{0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := loadElement(t, values, "", nil)
			got := (&EmptyPolicyCheck{}).Run(Context{
				Element:  el,
				Cluster:  &cascade.Resolved{ClusterName: "spoke", ReleaseName: "my-element-spoke"},
				Rendered: []byte(tc.rendered),
			})
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d findings, got %d: %+v", len(tc.want), len(got), got)
			}
			for i, w := range tc.want {
				f := got[i]
				if f.RuleID != "POLICY082" || f.Severity != SevError || !strings.HasPrefix(f.Message, w) ||
					!strings.HasSuffix(f.Message, "so ACM reports it Compliant: set the element's input or turn the toggle off") {
					t.Errorf("finding %d = %+v, want error starting %q", i, f, w)
				}
				if f.File != el.ValuesFile || f.Line != tc.lines[i] || f.Element != "my-element" || f.Cluster != "spoke" {
					t.Errorf("finding %d location wrong, want line %d: %+v", i, tc.lines[i], f)
				}
			}
		})
	}
}

func TestPolicy082_Invalid(t *testing.T) {
	el := loadElement(t, "stack:\n  myElement: {}\n", "", nil)
	cl := &cascade.Resolved{ClusterName: "spoke", ReleaseName: "my-element-spoke"}
	for _, rendered := range []string{"", "\n---\n"} {
		if got := (&EmptyPolicyCheck{}).Run(Context{Element: el, Cluster: cl, Rendered: []byte(rendered)}); len(got) != 0 {
			t.Errorf("%q: got %+v, want none", rendered, got)
		}
	}
	got := (&EmptyPolicyCheck{}).Run(Context{Element: el, Cluster: cl, Rendered: []byte("kind: [")})
	if len(got) != 1 || !strings.HasPrefix(got[0].Message, "rendered output: ") {
		t.Errorf("got %+v, want one rendered output finding", got)
	}
}

// TestPolicy082_Engines locates the finding under the selected lifecycle engine's stack key.
func TestPolicy082_Engines(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"Chart.yaml":         "apiVersion: v2\nname: lifecycle\nversion: 0.1.0\n",
		"values.schema.json": `{"properties": {"install": {"properties": {"engine": {"enum": ["hypershift", "hive"]}}}}}`,
		"values.yaml":        "stack:\n  hypershift:\n    policies:\n      - name: present\n  hive:\n    policies:\n      - name: present\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	els, err := chart.LoadComponents(dir, "policy")
	if err != nil {
		t.Fatal(err)
	}
	got := (&EmptyPolicyCheck{}).Run(Context{
		Element:  els[1],
		Cluster:  &cascade.Resolved{ClusterName: "spoke", ReleaseName: "lifecycle-spoke"},
		Rendered: []byte(renderedPolicy("present-lifecycle-spoke", tplEmptyRaw, tplNullObjects)),
	})
	if len(got) != 1 || got[0].Line != 7 || !strings.HasPrefix(got[0].Message, `policies[0] "present"`) {
		t.Errorf("got %+v, want one finding at line 7", got)
	}
}
