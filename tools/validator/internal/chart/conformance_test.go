package chart

import (
	"bytes"
	"cmp"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

// rendered is what TestConformance compares for one policy-template.
type rendered struct {
	Kind, Action, Severity string
	IgnorePending          bool
}

// TestConformance renders testdata/element, and tools/sample-element once it has the 4.0 shape,
// with the element turned on, and checks Load's model against policy-library: which Policies and
// policy-templates render, and their remediationAction, severity and ignorePending.
func TestConformance(t *testing.T) {
	runner := render.New(testenv.Helm(t))
	const root = "../../../.."
	for _, dir := range []string{"testdata/element", filepath.Join(root, "tools/sample-element")} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			el, err := Load(dir, "policy")
			if err != nil {
				t.Fatal(err)
			}
			comp := el.Values.Component
			if comp.V3 {
				t.Skip("policy-library 3.x shape")
			}
			if err := runner.EnsureDeps(t.Context(), dir); err != nil {
				t.Fatal(err)
			}
			enable := filepath.Join(t.TempDir(), "enable.yaml")
			if err := os.WriteFile(enable, []byte("stack:\n  "+el.StackKey+":\n    enabled: true\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			release := el.ChartName + "-spoke"
			files := []string{filepath.Join(root, "values.yaml"), el.ValuesFile, filepath.Join(root, "tools/validator/testdata/baseline.yaml"), enable}
			tr := runner.Template(t.Context(), dir, release, files, nil)
			if tr.Err != nil {
				t.Fatalf("helm template: %v: %s", tr.Err, tr.Stderr)
			}
			got, gotActions := renderedPolicies(t, tr.Stdout)

			want, wantActions := map[string]map[string]rendered{}, map[string]string{}
			for _, p := range comp.Policies.Sorted() {
				entries := map[string]rendered{}
				for _, tpl := range p.Templates.Sorted() {
					if tpl.On() {
						entries[p.Name+"-"+tpl.Name] = rendered{"ConfigurationPolicy", comp.TemplateAction(p, tpl), comp.Severity(p, tpl), tpl.Pending()}
					}
				}
				for k, op := range comp.OperatorPolicies.Sorted() {
					if p.Name != "install" || !op.On() {
						continue
					}
					name, sev := p.Name+"-"+k, cmp.Or(op.Severity, comp.Severity(p, nil))
					entries[name] = rendered{"OperatorPolicy", comp.Action(p), sev, op.Pending()}
					entries[name+"-ns"] = rendered{"ConfigurationPolicy", comp.Action(p), sev, false}
					entries[name+"-status"] = rendered{"ConfigurationPolicy", "inform", sev, false}
				}
				if p.On() && len(entries) > 0 {
					want[p.Name+"-"+release], wantActions[p.Name+"-"+release] = entries, comp.Action(p)
				}
			}
			if !maps.Equal(gotActions, wantActions) {
				t.Errorf("Policy remediationActions:\n got %v\nwant %v", gotActions, wantActions)
			}
			for _, name := range slices.Sorted(maps.Keys(want)) {
				if !maps.Equal(got[name], want[name]) {
					t.Errorf("%s policy-templates:\n got %v\nwant %v", name, got[name], want[name])
				}
			}
		})
	}
}

// renderedPolicies returns the policy-templates of each rendered Policy by name, and each Policy's
// remediationAction.
func renderedPolicies(t *testing.T, out []byte) (map[string]map[string]rendered, map[string]string) {
	t.Helper()
	templates, actions := map[string]map[string]rendered{}, map[string]string{}
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(out), 4096)
	for {
		var p struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				RemediationAction string `json:"remediationAction"`
				PolicyTemplates   []struct {
					IgnorePending    bool `json:"ignorePending"`
					ObjectDefinition struct {
						Kind     string `json:"kind"`
						Metadata struct {
							Name string `json:"name"`
						} `json:"metadata"`
						Spec struct {
							RemediationAction string `json:"remediationAction"`
							Severity          string `json:"severity"`
						} `json:"spec"`
					} `json:"objectDefinition"`
				} `json:"policy-templates"`
			} `json:"spec"`
		}
		if err := dec.Decode(&p); err == io.EOF {
			return templates, actions
		} else if err != nil {
			t.Fatal(err)
		}
		if p.Kind != "Policy" {
			continue
		}
		actions[p.Metadata.Name] = p.Spec.RemediationAction
		templates[p.Metadata.Name] = map[string]rendered{}
		for _, pt := range p.Spec.PolicyTemplates {
			od := pt.ObjectDefinition
			templates[p.Metadata.Name][od.Metadata.Name] = rendered{od.Kind, od.Spec.RemediationAction, od.Spec.Severity, pt.IgnorePending}
		}
	}
}
