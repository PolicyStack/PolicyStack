package checks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/cascade"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

// TestACMTemplateCheck renders nothing: it feeds a Policy as Helm would render it and checks the
// finding names the unsuffixed sub-policy, the selected engine's converter and its line.
func TestACMTemplateCheck(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("other", "data:\n  v: '{{ \"{{\" }} lookpu {{ \"}}\" }}'\n")
	write("first", "kind: ConfigMap\n")
	write("second", "# comment\ndata:\n  a: ok\n  v: '{{ \"{{-\" }} lookpu {{ \"}}\" }}'\n")

	component := func(templates ...string) *chart.Component {
		cp := chart.SubPolicy{Name: "ready", PolicyRef: "present"}
		for _, n := range templates {
			cp.TemplateNames = append(cp.TemplateNames, chart.TemplateName{Name: n})
		}
		return &chart.Component{ConfigPolicies: []chart.SubPolicy{cp}}
	}
	// An engine copy of the lifecycle chart: Components holds every engine, Values.Component the
	// selected one.
	selected := component("first", "second")
	el := &chart.Element{
		ChartName:     "lifecycle",
		ValuesFile:    filepath.Join(dir, "values.yaml"),
		ConvertersDir: dir,
		Values:        &chart.Values{Component: selected},
		Components: []chart.NamedComponent{
			{Key: "other", Component: component("other", "other")},
			{Key: "selected", Component: selected},
		},
	}
	rendered := `apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: p
  namespace: policy
spec:
  policy-templates:
    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: present-ready-spoke
        spec:
          object-templates:
            - complianceType: musthave
              objectDefinition:
                apiVersion: v1
                kind: ConfigMap
                metadata: {name: a, namespace: default}
            - complianceType: musthave
              objectDefinition:
                apiVersion: v1
                kind: ConfigMap
                metadata: {name: b, namespace: default}
                data:
                  v: '{{ lookpu }}'
`
	got := (&ACMTemplateCheck{}).Run(Context{
		Element:  el,
		Cluster:  &cascade.Resolved{ClusterName: "spoke", ReleaseName: "lifecycle-spoke"},
		Rendered: []byte(rendered),
	})
	want := Finding{
		RuleID:   "POLICY081",
		Severity: SevError,
		Element:  "lifecycle",
		Message:  `present-ready (managed template): function "lookpu" not defined`,
		File:     filepath.Join(dir, "second.yaml"),
		Line:     4,
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
