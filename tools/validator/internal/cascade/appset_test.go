package cascade

import (
	"bytes"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v3"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

// TestTemplatePatch renders the install chart, evaluates each ApplicationSet's templatePatch as
// Argo CD does, and checks that manifest-generate-paths covers every values file the
// Application reads.
func TestTemplatePatch(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not on PATH")
	}
	out, err := exec.CommandContext(t.Context(), helm, "template", "ci", "../../../../install",
		"--set", "hubName=acm-dc1", "--set", "gitRepo=https://example.com/policystack.git").Output()
	if err != nil {
		t.Fatalf("helm template: %v", err)
	}
	patches := map[string]string{}
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(out), 4096)
	for {
		var doc struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				TemplatePatch string `json:"templatePatch"`
			} `json:"spec"`
		}
		if err := dec.Decode(&doc); err != nil {
			break
		}
		if doc.Kind == "ApplicationSet" {
			patches[doc.Metadata.Name] = doc.Spec.TemplatePatch
		}
	}

	stack := map[string]any{"path": "stack/acm", "basenameNormalized": "acm"}
	fleet := []any{"environments/prod.yaml", "datacenters/dc1.yaml"}
	for _, tc := range []struct {
		name, appset string
		params       map[string]any
		want         string
	}{
		{
			name:   "spoke",
			appset: "policystack",
			params: map[string]any{
				"name": "prod-east-1", "revision": "main", "valueFiles": fleet, "path": stack,
				"metadata": map[string]any{"labels": map[string]string{"clusterID": "id"}},
			},
			want: ".;../../values.yaml;../../values/environments/prod.yaml;../../values/datacenters/dc1.yaml;../../values/clusters/prod-east-1.yaml",
		},
		{
			name:   "hub",
			appset: "policystack",
			params: map[string]any{
				"name": "local-cluster", "revision": "main", "valueFiles": fleet, "path": stack, "version": "4.20.1",
				"metadata": map[string]any{"labels": map[string]string{"clusterID": "id", "local-cluster": "true"}},
			},
			want: ".;../../values.yaml;../../values/environments/prod.yaml;../../values/datacenters/dc1.yaml;../../values/clusters/acm-dc1.yaml",
		},
		{
			name:   "global hub default mode, no valueFiles",
			appset: "policystack",
			params: map[string]any{
				"name": "mh1", "revision": "main", "path": stack,
				"metadata": map[string]any{"labels": map[string]string{
					"clusterID": "id", "global-hub.open-cluster-management.io/deploy-mode": "default",
				}},
			},
			want: ".;../../values.yaml;../../values/clusters/mh1.yaml",
		},
		{
			name:   "lifecycle",
			appset: "policystack-lifecycle",
			params: map[string]any{
				"name": "local-cluster", "revision": "main", "valueFiles": fleet, "hub": "acm-dc1", "version": "4.20.1",
				"path": map[string]any{"path": "fleet", "filename": "hcp-kubevirt.yaml"},
			},
			want: ".;../values.yaml;../values/environments/prod.yaml;../values/datacenters/dc1.yaml;../values/clusters/hcp-kubevirt.yaml",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, err := template.New(tc.appset).Option("missingkey=error").Funcs(sprig.TxtFuncMap()).Parse(patches[tc.appset])
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, tc.params); err != nil {
				t.Fatal(err)
			}
			// yaml.v3 also fails on duplicate keys, such as a second metadata block.
			var app struct {
				Metadata struct {
					Annotations map[string]string `yaml:"annotations"`
				} `yaml:"metadata"`
				Spec struct {
					Source struct {
						Helm struct {
							ValueFiles []string `yaml:"valueFiles"`
						} `yaml:"helm"`
					} `yaml:"source"`
				} `yaml:"spec"`
			}
			if err := yaml.Unmarshal(buf.Bytes(), &app); err != nil {
				t.Fatalf("%v\n%s", err, buf.String())
			}
			got := app.Metadata.Annotations["argocd.argoproj.io/manifest-generate-paths"]
			if got != tc.want {
				t.Errorf("manifest-generate-paths = %q, want %q", got, tc.want)
			}
			paths := strings.Split(got, ";")
			if !slices.Contains(paths, ".") {
				t.Errorf("manifest-generate-paths %q misses the chart directory", got)
			}
			for _, f := range app.Spec.Source.Helm.ValueFiles {
				if f != "values.yaml" && !slices.Contains(paths, f) {
					t.Errorf("valueFiles entry %s is not in manifest-generate-paths %q", f, got)
				}
			}
		})
	}
}
