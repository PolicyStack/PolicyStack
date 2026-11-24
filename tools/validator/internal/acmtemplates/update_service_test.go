package acmtemplates

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

var updateServiceDir = filepath.Join(repoRoot, "stack/update-service")

const mirrorCA = "-----BEGIN CERTIFICATE-----\nMIRROR\n-----END CERTIFICATE-----\n"

// TestUpdateServiceRegistryCA: registry-cas gains updateservice-registry, the CA of the releases
// host, only while update-service is enabled, so the disconnected element's output is unchanged
// without it.
func TestUpdateServiceRegistryCA(t *testing.T) {
	for _, tt := range []struct {
		name, enabled, releases, want string
	}{
		{"host with port", "true", "mirror.example.com:8443/openshift/release-images", mirrorCA},
		{"host without port", "true", "registry.example.com/ocp4/openshift4-release-images", mirrorCA},
		{"host without CA", "true", "quay.io/openshift-release-dev/ocp-release", ""},
		{"disabled", "false", "mirror.example.com:8443/openshift/release-images", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := valuesFile(t, `stack:
  disconnected:
    config:
      registryCAs:
        mirror.example.com..8443: "`+strings.ReplaceAll(mirrorCA, "\n", `\n`)+`"
        registry.example.com: "`+strings.ReplaceAll(mirrorCA, "\n", `\n`)+`"
  updateService:
    enabled: `+tt.enabled+`
    config:
      releases: `+tt.releases+"\n")
			raw := renderRaw(t, filepath.Join(repoRoot, "stack/disconnected"), "disconnected", files)["registry-ca-trust"]
			data := nested(find(t, resolve(t, raw), "ConfigMap", "registry-cas"), "data").(map[string]any)
			if got, _ := data["updateservice-registry"].(string); got != tt.want {
				t.Errorf("updateservice-registry %q, want %q", got, tt.want)
			}
			wantKeys := 2
			if tt.want != "" {
				wantKeys++
			}
			if len(data) != wantKeys {
				t.Errorf("registry-cas %d keys, want %d: %v", len(data), wantKeys, data)
			}
		})
	}
}

// TestUpdateServiceInstance: the UpdateService takes releases and graphDataImage as given.
func TestUpdateServiceInstance(t *testing.T) {
	var o struct {
		Spec map[string]any `json:"spec"`
	}
	if err := json.Unmarshal(objectDefinition(t, updateServiceDir, "UpdateService", "update-service"), &o); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"replicas":       float64(2),
		"releases":       "mirror.example.com:8443/openshift/release-images",
		"graphDataImage": "mirror.example.com:8443/openshift/graph-image:latest",
	}
	if asYAML(t, o.Spec) != asYAML(t, want) {
		t.Errorf("spec %v, want %v", o.Spec, want)
	}
}

// TestUpdateServiceRejected: a releases value the operator cannot split, or a missing input, fails
// the render instead of an UpdateService that never reports a status.
func TestUpdateServiceRejected(t *testing.T) {
	runner := render.New(testenv.Helm(t))
	if err := runner.EnsureDeps(context.Background(), updateServiceDir); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, config, want string }{
		{"no repository", `{releases: mirror.example.com:8443}`, "must be <registry>/<repository>"},
		{"tag", `{releases: "mirror.example.com:8443/openshift/release-images:4.20.8-x86_64"}`, "must be <registry>/<repository>"},
		{"digest", `{releases: "mirror.example.com/openshift/release-images@sha256:abc"}`, "must be <registry>/<repository>"},
		{"trailing slash", `{releases: "mirror.example.com/openshift/"}`, "must be <registry>/<repository>"},
		{"releases empty", `{releases: ""}`, "releases is required"},
		{"graphDataImage empty", `{graphDataImage: ""}`, "graphDataImage is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := valuesFile(t, "stack: {updateService: {config: "+tt.config+"}}\n")
			tr := runner.Template(context.Background(), updateServiceDir, "update-service", files, nil)
			if tr.Err == nil || !strings.Contains(string(tr.Stderr), tt.want) {
				t.Fatalf("want %q, got err %v: %s", tt.want, tr.Err, tr.Stderr)
			}
		})
	}
}
