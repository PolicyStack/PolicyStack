package acmtemplates

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// TestImageAllowlist: allowlist-exact's lists replace the live ones, so a registry taken out of values
// is revoked, and every other field of the spec is kept.
func TestImageAllowlist(t *testing.T) {
	const image = "apiVersion: config.openshift.io/v1\nkind: Image\nmetadata: {name: cluster}\n"
	const layer = `stack:
  remediations:
    config:
      allowedRegistries: %s
      allowedRegistriesForImport: %s
`
	for _, tt := range []struct {
		name, allowed, imports, want string
		live                         []string
	}{
		{
			name: "absent", allowed: "[quay.io]", imports: "[{domainName: quay.io}]",
			want: "{registrySources: {allowedRegistries: [quay.io]}, allowedRegistriesForImport: [{domainName: quay.io, insecure: false}]}",
		},
		{
			name: "no spec", allowed: "[quay.io]", imports: "[]", live: []string{image},
			want: "{registrySources: {allowedRegistries: [quay.io]}, allowedRegistriesForImport: []}",
		},
		{
			name: "revoked and others kept", allowed: "[quay.io]", imports: "[{domainName: quay.io}]",
			live: []string{image + `spec:
  additionalTrustedCA: {name: registry-cas}
  externalRegistryHostnames: [registry.example.com]
  allowedRegistriesForImport: [{domainName: revoked.example, insecure: true}, {domainName: quay.io, insecure: false}]
  registrySources:
    allowedRegistries: [revoked.example, quay.io]
    insecureRegistries: [lab.example]
    containerRuntimeSearchRegistries: [quay.io]
`},
			want: `additionalTrustedCA: {name: registry-cas}
externalRegistryHostnames: [registry.example.com]
allowedRegistriesForImport: [{domainName: quay.io, insecure: false}]
registrySources: {allowedRegistries: [quay.io], insecureRegistries: [lab.example], containerRuntimeSearchRegistries: [quay.io]}`,
		},
		{
			name: "every entry removed", allowed: "[]", imports: "[]",
			live: []string{image + "spec: {registrySources: {allowedRegistries: [revoked.example]}}\n"},
			want: "{registrySources: {allowedRegistries: []}, allowedRegistriesForImport: []}",
		},
		{
			name: "null lists", allowed: "null", imports: "null",
			want: "{registrySources: {allowedRegistries: []}, allowedRegistriesForImport: []}",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := valuesFile(t, fmt.Sprintf(layer, tt.allowed, tt.imports))
			o := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/remediations"), "remediations", files, "registries-allowlist-exact", tt.live...), "Image", "cluster")
			if o.ComplianceType != "mustonlyhave" {
				t.Errorf("complianceType %q, want mustonlyhave", o.ComplianceType)
			}
			wantYAML(t, nested(o, "spec"), tt.want)
		})
	}
}

// TestLogForwarder: clusterlogforwarder-exact's inputRefs follow values, and outputs, pipelines and fields values don't set are kept.
func TestLogForwarder(t *testing.T) {
	const ours = `{name: logging-lokistack, type: lokiStack,
  lokiStack: {target: {name: logging-lokistack, namespace: openshift-logging}, authentication: {token: {from: serviceAccount}}},
  tls: {ca: {configMapName: openshift-service-ca.crt, key: service-ca.crt}}}`
	const live = `apiVersion: observability.openshift.io/v1
kind: ClusterLogForwarder
metadata: {name: logging, namespace: openshift-logging}
spec:
  managementState: Unmanaged
  serviceAccount: {name: collector}
  collector: {tolerations: [{key: node-role.kubernetes.io/infra, operator: Exists}]}
  outputs:
    - ` + ours + `
    - {name: siem, type: syslog}
  pipelines:
    - {name: logs-to-loki, inputRefs: [application, audit, infrastructure], outputRefs: [logging-lokistack, siem], filterRefs: [parse]}
    - {name: to-siem, inputRefs: [audit], outputRefs: [siem]}
`
	files := valuesFile(t, "stack: {loki: {config: {forwardedLogTypes: [application]}}}\n")
	for _, tt := range []struct {
		name, want string
		live       []string
	}{
		{name: "absent", want: `managementState: Managed
serviceAccount: {name: collector}
outputs: [` + ours + `]
pipelines: [{name: logs-to-loki, inputRefs: [application], outputRefs: [logging-lokistack]}]`},
		{name: "revoked and others kept", live: []string{live}, want: `managementState: Managed
serviceAccount: {name: collector}
collector: {tolerations: [{key: node-role.kubernetes.io/infra, operator: Exists}]}
outputs:
  - ` + ours + `
  - {name: siem, type: syslog}
pipelines:
  - {name: logs-to-loki, inputRefs: [application], outputRefs: [logging-lokistack, siem], filterRefs: [parse]}
  - {name: to-siem, inputRefs: [audit], outputRefs: [siem]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/loki"), "loki-spoke", files, "forwarder-clusterlogforwarder-exact", tt.live...), "ClusterLogForwarder", "logging")
			if o.ComplianceType != "mustonlyhave" {
				t.Errorf("complianceType %q, want mustonlyhave", o.ComplianceType)
			}
			wantYAML(t, nested(o, "spec"), tt.want)
		})
	}
}

// TestQuayComponents: under registry-exact each kind is listed once with the managed flag values set, keeping overrides,
// kinds values don't name and the rest of the spec.
func TestQuayComponents(t *testing.T) {
	files := valuesFile(t, `stack:
  quay:
    config:
      components: {quay: true, clair: false, postgres: false, clairpostgres: null, objectstorage: null,
        redis: null, horizontalpodautoscaler: null, route: null, mirror: null, monitoring: null, tls: null}
`)
	const live = `apiVersion: quay.redhat.com/v1
kind: QuayRegistry
metadata: {name: registry, namespace: quay-enterprise}
spec:
  configBundleSecret: changed-by-hand
  components:
    - {kind: postgres, managed: false}
    - {kind: postgres, managed: true, overrides: {volumeSize: 100Gi}}
    - {kind: quay, managed: false, overrides: {replicas: 3}}
    - {kind: cache, managed: false}
`
	for _, tt := range []struct {
		name, want string
		live       []string
	}{
		{name: "absent", want: `configBundleSecret: quay-registry-config-bundle
components: [{kind: clair, managed: false}, {kind: postgres, managed: false}, {kind: quay, managed: true}]`},
		{name: "changed and others kept", live: []string{live}, want: `configBundleSecret: quay-registry-config-bundle
components:
  - {kind: cache, managed: false}
  - {kind: clair, managed: false}
  - {kind: postgres, managed: false, overrides: {volumeSize: 100Gi}}
  - {kind: quay, managed: true, overrides: {replicas: 3}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/quay"), "quay", files, "deploy-registry-exact", tt.live...), "QuayRegistry", "registry")
			if o.ComplianceType != "mustonlyhave" {
				t.Errorf("complianceType %q, want mustonlyhave", o.ComplianceType)
			}
			wantYAML(t, nested(o, "spec"), tt.want)
		})
	}
}

// TestListsIdempotent: resolving over a spec a template wrote gives that spec again.
func TestListsIdempotent(t *testing.T) {
	for _, tt := range []struct{ element, template, kind, name string }{
		{"remediations", "registries-allowlist-exact", "Image", "cluster"},
		{"loki", "forwarder-clusterlogforwarder-exact", "ClusterLogForwarder", "logging"},
		{"quay", "deploy-registry-exact", "QuayRegistry", "registry"},
	} {
		t.Run(tt.element, func(t *testing.T) {
			dir := filepath.Join(repoRoot, "stack", tt.element)
			release := tt.element + "-spoke"
			first := find(t, resolveTemplate(t, dir, release, allOn, tt.template), tt.kind, tt.name)
			live := asYAML(t, first.ObjectDefinition.Object)
			second := find(t, resolveTemplate(t, dir, release, allOn, tt.template, live), tt.kind, tt.name)
			if !reflect.DeepEqual(first.ObjectDefinition.Object, second.ObjectDefinition.Object) {
				t.Errorf("second evaluation differs\n got %v\nwant %v", second.ObjectDefinition.Object, first.ObjectDefinition.Object)
			}
		})
	}
}
