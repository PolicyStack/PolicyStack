package acmtemplates

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"sigs.k8s.io/yaml"
)

// valuesFile writes body as a values file and returns the all-on files with it on top.
func valuesFile(t *testing.T, body string) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "values.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return append(slices.Clone(allOn), path)
}

// asYAML compares values that differ only in Go types, such as int64 and float64.
func asYAML(t *testing.T, v any) string {
	t.Helper()
	out, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func wantYAML(t *testing.T, got any, want string) {
	t.Helper()
	var w any
	if err := yaml.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if g, w := asYAML(t, got), asYAML(t, w); g != w {
		t.Errorf("got\n%s\nwant\n%s", g, w)
	}
}

// TestLogForwarder: the spec is exactly what values render, so a log type taken out stops forwarding.
func TestLogForwarder(t *testing.T) {
	files := valuesFile(t, "stack: {loki: {config: {forwardedLogTypes: [application]}}}\n")
	o := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/loki"), "loki-spoke", files, "forwarder-clusterlogforwarder"), "ClusterLogForwarder", "logging")
	if o.ComplianceType != "mustonlyhave" {
		t.Errorf("complianceType %q, want mustonlyhave", o.ComplianceType)
	}
	wantYAML(t, nested(o, "spec"), `managementState: Managed
serviceAccount: {name: collector}
outputs:
  - name: logging-lokistack
    type: lokiStack
    lokiStack: {target: {name: logging-lokistack, namespace: openshift-logging}, authentication: {token: {from: serviceAccount}}}
    tls: {ca: {configMapName: openshift-service-ca.crt, key: service-ca.crt}}
pipelines: [{name: logs-to-loki, inputRefs: [application], outputRefs: [logging-lokistack]}]`)
}

// TestQuayComponents: every kind is listed once with the managed flag values set, so the operator
// has none to add and a changed flag replaces the old entry.
func TestQuayComponents(t *testing.T) {
	files := valuesFile(t, "stack: {quay: {config: {components: {clair: false, postgres: false}}}}\n")
	o := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/quay"), "quay", files, "deploy-registry"), "QuayRegistry", "registry")
	if o.ComplianceType != "mustonlyhave" {
		t.Errorf("complianceType %q, want mustonlyhave", o.ComplianceType)
	}
	wantYAML(t, nested(o, "spec"), `configBundleSecret: quay-registry-config-bundle
components:
  - {kind: clair, managed: false}
  - {kind: clairpostgres, managed: true}
  - {kind: horizontalpodautoscaler, managed: true}
  - {kind: mirror, managed: true}
  - {kind: monitoring, managed: true}
  - {kind: objectstorage, managed: true}
  - {kind: postgres, managed: false}
  - {kind: quay, managed: true}
  - {kind: redis, managed: true}
  - {kind: route, managed: true}
  - {kind: tls, managed: true}`)
	// mustonlyhave on these would drop the Secret's other keys and the Namespace's labels.
	bundle := resolveTemplate(t, filepath.Join(repoRoot, "stack/quay"), "quay", files, "deploy-bundle")
	for kind, name := range map[string]string{"Namespace": "quay-enterprise", "Secret": "quay-registry-config-bundle"} {
		if got := find(t, bundle, kind, name).ComplianceType; got != "musthave" {
			t.Errorf("%s complianceType %q, want musthave", kind, got)
		}
	}
}
