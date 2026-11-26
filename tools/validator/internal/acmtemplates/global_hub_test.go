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

var globalHubDir = filepath.Join(repoRoot, "stack/global-hub")

// TestGlobalHubInstance: the MulticlusterGlobalHub carries only the curated settings, leaving the
// rest to CRD defaults, and points the operator's AMQ Streams Subscription at the catalog
// global.catalogSources maps redhat-operators to.
func TestGlobalHubInstance(t *testing.T) {
	// testdata/inputs.yaml sets storageClass gp3-csi on top of the defaults.
	const mirror, inputs = "cs-redhat-operator-index-v4-20", `{availabilityConfig: High, dataLayer: {storageClass: gp3-csi, postgres: {retention: 18m}}}`
	for _, tt := range []struct {
		name, body, wantSpec string
		wantAnnotations      map[string]any
	}{
		{"inputs", "", inputs, nil},
		{
			"storageClass empty",
			"stack: {globalHub: {config: {storageClass: \"\"}}}\n",
			`{availabilityConfig: High, dataLayer: {postgres: {retention: 18m}}}`,
			nil,
		},
		{
			"settings",
			"stack: {globalHub: {config: {availabilityConfig: Basic, retention: 1y6m, storageClass: lvms-vg1}}}\n",
			`{availabilityConfig: Basic, dataLayer: {storageClass: lvms-vg1, postgres: {retention: 1y6m}}}`,
			nil,
		},
		{
			"disconnected",
			"global: {catalogSources: {redhat-operators: " + mirror + "}}\n",
			inputs,
			map[string]any{
				"global-hub.open-cluster-management.io/strimzi-catalog-source-name":      mirror,
				"global-hub.open-cluster-management.io/strimzi-catalog-source-namespace": "openshift-marketplace",
			},
		},
		{"other catalog renamed", "global: {catalogSources: {certified-operators: cs-certified-operator-index-v4-20}}\n", inputs, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var files []string
			if tt.body != "" {
				files = valuesFile(t, tt.body)
			}
			var o struct {
				Metadata struct {
					Namespace   string         `json:"namespace"`
					Annotations map[string]any `json:"annotations"`
				} `json:"metadata"`
				Spec map[string]any `json:"spec"`
			}
			if err := json.Unmarshal(objectDefinition(t, globalHubDir, "MulticlusterGlobalHub", "multiclusterglobalhub", files...), &o); err != nil {
				t.Fatal(err)
			}
			if o.Metadata.Namespace != "multicluster-global-hub" {
				t.Errorf("namespace %q, want multicluster-global-hub", o.Metadata.Namespace)
			}
			wantYAML(t, o.Spec, tt.wantSpec)
			if asYAML(t, o.Metadata.Annotations) != asYAML(t, tt.wantAnnotations) {
				t.Errorf("annotations %v, want %v", o.Metadata.Annotations, tt.wantAnnotations)
			}
		})
	}
}

// TestGlobalHubRejected: values the CRD would accept but the operator misreads fail the render.
func TestGlobalHubRejected(t *testing.T) {
	runner := render.New(testenv.Helm(t))
	if err := runner.EnsureDeps(context.Background(), globalHubDir); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, config, want string }{
		{"availability lowercase", `{availabilityConfig: high}`, "must be High or Basic"},
		{"availability empty", `{availabilityConfig: ""}`, "must be High or Basic"},
		{"retention empty", `{retention: ""}`, "must be years and months"},
		{"retention days", `{retention: 30d}`, "must be years and months"},
		{"retention unit order", `{retention: 6m1y}`, "must be years and months"},
		{"retention number", `{retention: 18}`, "must be years and months"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := valuesFile(t, "stack: {globalHub: {config: "+tt.config+"}}\n")
			tr := runner.Template(context.Background(), globalHubDir, "global-hub", files, nil)
			if tr.Err == nil || !strings.Contains(string(tr.Stderr), tt.want) {
				t.Fatalf("want %q, got err %v: %s", tt.want, tr.Err, tr.Stderr)
			}
		})
	}
}
