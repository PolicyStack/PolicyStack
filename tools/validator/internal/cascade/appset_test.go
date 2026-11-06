package cascade

import (
	"bytes"
	"maps"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"testing"
	"text/template"

	"github.com/Masterminds/semver/v3"
	"github.com/Masterminds/sprig/v3"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
	"gopkg.in/yaml.v3"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

// TestTemplatePatch renders the install chart, evaluates each ApplicationSet's templatePatch as
// Argo CD does, and checks that manifest-generate-paths covers every values file the
// Application reads and the policy-library chart in lib/, that valuesObject injects exactly the
// documented root keys, and that policystack.libraryMajors lists lib/policy-library's major.
func TestTemplatePatch(t *testing.T) {
	helm := testenv.Helm(t)
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

	data, err := os.ReadFile("../../../../lib/policy-library/Chart.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var lib struct{ Version string }
	if err := yaml.Unmarshal(data, &lib); err != nil {
		t.Fatal(err)
	}
	libVersion, err := semver.StrictNewVersion(lib.Version)
	if err != nil {
		t.Fatal(err)
	}

	stack := map[string]any{"path": "stack/acm", "basenameNormalized": "acm"}
	fleet := []any{"environments/prod.yaml", "datacenters/dc1.yaml"}
	for _, tc := range []struct {
		name, appset string
		dir          string // the Application's source path
		params       map[string]any
		want         string
		keys         []string // valuesObject root keys, sorted
	}{
		{
			name:   "spoke",
			appset: "policystack",
			dir:    "stack/acm",
			params: map[string]any{
				"name": "prod-east-1", "revision": "main", "valueFiles": fleet, "path": stack,
				"metadata": map[string]any{"labels": map[string]string{"clusterID": "id"}},
			},
			want: ".;../../lib;../../values.yaml;../../values/environments/prod.yaml;../../values/datacenters/dc1.yaml;../../values/clusters/prod-east-1.yaml",
			keys: []string{"policystack", "selector"},
		},
		{
			name:   "hub",
			appset: "policystack",
			dir:    "stack/acm",
			params: map[string]any{
				"name": "local-cluster", "revision": "main", "valueFiles": fleet, "path": stack, "version": "4.20.1",
				"metadata": map[string]any{"labels": map[string]string{"clusterID": "id", "local-cluster": "true"}},
			},
			want: ".;../../lib;../../values.yaml;../../values/environments/prod.yaml;../../values/datacenters/dc1.yaml;../../values/clusters/acm-dc1.yaml",
			keys: []string{"policystack", "selector", "version"},
		},
		{
			name:   "global hub default mode, no valueFiles",
			appset: "policystack",
			dir:    "stack/acm",
			params: map[string]any{
				"name": "mh1", "revision": "main", "path": stack,
				"metadata": map[string]any{"labels": map[string]string{
					"clusterID": "id", "global-hub.open-cluster-management.io/deploy-mode": "default",
				}},
			},
			want: ".;../../lib;../../values.yaml;../../values/clusters/mh1.yaml",
			keys: []string{"policystack", "selector"},
		},
		{
			name:   "lifecycle",
			appset: "policystack-lifecycle",
			dir:    "lifecycle",
			params: map[string]any{
				"name": "local-cluster", "revision": "main", "valueFiles": fleet, "hub": "acm-dc1", "version": "4.20.1",
				"path": map[string]any{"path": "fleet", "filename": "hcp-kubevirt.yaml"},
			},
			want: ".;../lib;../values.yaml;../values/environments/prod.yaml;../values/datacenters/dc1.yaml;../values/clusters/hcp-kubevirt.yaml",
			keys: []string{"hub", "install", "policystack", "selector", "state", "version"},
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
							ValueFiles   []string       `yaml:"valueFiles"`
							ValuesObject map[string]any `yaml:"valuesObject"`
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
			if !slices.ContainsFunc(paths, func(p string) bool { return path.Join(tc.dir, p) == "lib" }) {
				t.Errorf("manifest-generate-paths %q misses lib, relative to %s", got, tc.dir)
			}
			for _, f := range app.Spec.Source.Helm.ValueFiles {
				if f != "values.yaml" && !slices.Contains(paths, f) {
					t.Errorf("valueFiles entry %s is not in manifest-generate-paths %q", f, got)
				}
			}
			values := app.Spec.Source.Helm.ValuesObject
			if keys := slices.Sorted(maps.Keys(values)); !slices.Equal(keys, tc.keys) {
				t.Errorf("valuesObject keys = %q, want %q", keys, tc.keys)
			}
			// policy-library fails the render when its own major is not listed.
			ps, _ := values["policystack"].(map[string]any)
			majors, _ := ps["libraryMajors"].([]any)
			if !slices.Contains(majors, any(int(libVersion.Major()))) {
				t.Errorf("policystack.libraryMajors = %v, want it to list lib/policy-library's major %d (install/templates/_helpers.tpl)", majors, libVersion.Major())
			}
		})
	}
}
