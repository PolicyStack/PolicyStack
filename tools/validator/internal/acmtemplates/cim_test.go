package acmtemplates

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// cimRaw renders stack/cim with the all-on profile and returns its raw templates by name.
func cimRaw(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(repoRoot, "stack/cim")
	return renderRaw(t, dir, "cim-acm-dc1", allOn)
}

func TestCIMProvisioning(t *testing.T) {
	raw := cimRaw(t)["hub-cim-provisioning"]
	created := find(t, resolve(t, raw), "Provisioning", "provisioning-configuration")
	if nested(created, "spec", "watchAllNamespaces") != true || field(created, "spec", "provisioningNetwork") != "Disabled" {
		t.Errorf("created %v", created.ObjectDefinition.Object)
	}
	live := "apiVersion: metal3.io/v1alpha1\nkind: Provisioning\nmetadata: {name: provisioning-configuration}\nspec: {provisioningNetwork: Managed}\n"
	patched := find(t, resolve(t, raw, live), "Provisioning", "provisioning-configuration")
	if nested(patched, "spec", "watchAllNamespaces") != true || nested(patched, "spec", "provisioningNetwork") != nil {
		t.Errorf("patched %v", patched.ObjectDefinition.Object)
	}
}

func TestCIMSiteConfig(t *testing.T) {
	raw := cimRaw(t)["hub-cim-siteconfig"]
	if objs := resolve(t, raw); len(objs) != 0 {
		t.Errorf("no MultiClusterHub: %+v", objs)
	}
	mch := "apiVersion: operator.open-cluster-management.io/v1\nkind: MultiClusterHub\nmetadata: {name: my-hub, namespace: rhacm}\nspec: {}\n"
	o := find(t, resolve(t, raw, mch), "MultiClusterHub", "my-hub")
	if o.ObjectDefinition.GetNamespace() != "rhacm" {
		t.Errorf("namespace %q", o.ObjectDefinition.GetNamespace())
	}
	want := []any{map[string]any{"name": "siteconfig", "enabled": true}}
	if got := nested(o, "spec", "overrides", "components"); !reflect.DeepEqual(got, want) {
		t.Errorf("components %v", got)
	}
}

func TestCIMHealth(t *testing.T) {
	raw := cimRaw(t)["ready-cim-health"]
	if objs := resolve(t, raw); len(objs) != 4 {
		t.Errorf("no MultiClusterHub: %+v", objs)
	}
	mch := "apiVersion: operator.open-cluster-management.io/v1\nkind: MultiClusterHub\nmetadata: {name: my-hub, namespace: rhacm}\n"
	objs := resolve(t, raw, mch)
	if d := find(t, objs, "Deployment", "siteconfig-controller-manager"); d.ObjectDefinition.GetNamespace() != "rhacm" {
		t.Errorf("deployment namespace %q", d.ObjectDefinition.GetNamespace())
	}
	find(t, objs, "ConfigMap", "ai-cluster-templates-v1")
	find(t, objs, "CustomResourceDefinition", "baremetalhosts.metal3.io")
	cond := func(kv ...string) []any {
		var out []any
		for c := range slices.Chunk(kv, 2) {
			out = append(out, map[string]any{"type": c[0], "status": c[1]})
		}
		return out
	}
	for _, c := range []struct {
		kind, name string
		want       []any
	}{
		{"ClusterOperator", "baremetal", cond("Disabled", "False")},
		{"Deployment", "metal3-baremetal-operator", cond("Available", "True")},
		{"AgentServiceConfig", "agent", cond("ReconcileCompleted", "True", "DeploymentsHealthy", "True")},
		{"Deployment", "siteconfig-controller-manager", cond("Available", "True")},
	} {
		if got := nested(find(t, objs, c.kind, c.name), "status", "conditions"); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %s conditions %v", c.kind, c.name, got)
		}
	}
}

// TestCIMAgentStorage: an empty storageClass is omitted, since an empty string disables provisioning.
func TestCIMAgentStorage(t *testing.T) {
	od := objectDefinition(t, filepath.Join(repoRoot, "stack/cim"), "AgentServiceConfig", "agent")
	for _, want := range []string{`"databaseStorage"`, `"filesystemStorage"`, `"imageStorage"`, `"50Gi"`} {
		if !bytes.Contains(od, []byte(want)) {
			t.Errorf("no %s in %s", want, od)
		}
	}
	if bytes.Contains(od, []byte("storageClassName")) {
		t.Errorf("storageClassName rendered: %s", od)
	}
}

// mirrorValues is a hub layer with oc-mirror's mirror sets, a second registry CA and osImages. The
// all-on profile adds a CatalogSource, the first registry CA and trustedCA.
const mirrorValues = `stack:
  cim:
    config:
      osImages:
        "4.20": {version: 9.6.20251015-1, url: "https://images.example.com/rhcos-4.20.iso"}
  disconnected:
    config:
      clusterResources:
        - apiVersion: operators.coreos.com/v1alpha1
          kind: CatalogSource
          metadata: {name: cs-redhat-operator-index-v4-20, namespace: openshift-marketplace}
          spec: {image: "mirror.example.com:8443/redhat/redhat-operator-index:v4.20", sourceType: grpc}
        - apiVersion: config.openshift.io/v1
          kind: ImageDigestMirrorSet
          metadata: {name: idms-release-0}
          spec:
            imageDigestMirrors:
              - {source: quay.io/openshift-release-dev/ocp-release, mirrors: ["mirror.example.com:8443/ocp/release", "mirror2.example.com/ocp/release"]}
        - apiVersion: config.openshift.io/v1
          kind: ImageDigestMirrorSet
          metadata: {name: idms-operator-0}
          spec:
            imageDigestMirrors:
              - {source: registry.redhat.io/multicluster-engine, mirrors: ["mirror.example.com:8443/mce"]}
        - apiVersion: config.openshift.io/v1
          kind: ImageTagMirrorSet
          metadata: {name: itms-0}
          spec: {imageTagMirrors: [{source: quay.io/x, mirrors: ["mirror.example.com:8443/x"]}]}
      registryCAs:
        mirror2.example.com: "B\n"
`

// TestCIMMirror: registries.conf holds every digest mirror, never a tag mirror; ca-bundle.crt the
// registry CAs by host; and the AgentServiceConfig references both ConfigMaps and lists osImages.
// Without mirrors or osImages, nothing renders and the AgentServiceConfig keeps its storage only.
func TestCIMMirror(t *testing.T) {
	f := filepath.Join(t.TempDir(), "hub.yaml")
	if err := os.WriteFile(f, []byte(mirrorValues), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repoRoot, "stack/cim")
	objs := resolve(t, renderRaw(t, dir, "cim-acm-dc1", append(slices.Clone(allOn), f))["hub-cim-mirror"])
	mirror := find(t, objs, "ConfigMap", "assisted-mirror")
	const conf = `[[registry]]
  prefix = ""
  location = "quay.io/openshift-release-dev/ocp-release"
  mirror-by-digest-only = true
  [[registry.mirror]]
    location = "mirror.example.com:8443/ocp/release"
  [[registry.mirror]]
    location = "mirror2.example.com/ocp/release"
[[registry]]
  prefix = ""
  location = "registry.redhat.io/multicluster-engine"
  mirror-by-digest-only = true
  [[registry.mirror]]
    location = "mirror.example.com:8443/mce"
`
	if got := field(mirror, "data", "registries.conf"); got != conf {
		t.Errorf("registries.conf\n%s\nwant\n%s", got, conf)
	}
	if got, want := field(mirror, "data", "ca-bundle.crt"), "-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----\nB\n"; got != want {
		t.Errorf("ca-bundle.crt %q, want %q", got, want)
	}
	if mirror.ObjectDefinition.GetNamespace() != "multicluster-engine" || mirror.ObjectDefinition.GetLabels()["app"] != "assisted-service" {
		t.Errorf("metadata %v", mirror.ObjectDefinition.Object["metadata"])
	}
	if got := field(find(t, objs, "ConfigMap", "image-service-additional-ca"), "data", "ca-bundle.crt"); got != "-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----\n" {
		t.Errorf("OS image CA %q", got)
	}

	var asc struct {
		Spec map[string]any `json:"spec"`
	}
	if err := json.Unmarshal(objectDefinition(t, dir, "AgentServiceConfig", "agent", f), &asc); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"mirrorRegistryRef": map[string]any{"name": "assisted-mirror"},
		"OSImageCACertRef":  map[string]any{"name": "image-service-additional-ca"},
		"osImages": []any{map[string]any{"openshiftVersion": "4.20", "version": "9.6.20251015-1",
			"url": "https://images.example.com/rhcos-4.20.iso", "cpuArchitecture": "x86_64"}},
	} {
		if !reflect.DeepEqual(asc.Spec[key], want) {
			t.Errorf("%s %v, want %v", key, asc.Spec[key], want)
		}
	}

	// all-on alone: a CatalogSource and trustedCA, but no mirror set or osImages.
	if objs := resolve(t, cimRaw(t)["hub-cim-mirror"]); len(objs) != 0 {
		t.Errorf("no mirrors: %+v", objs)
	}
	od := objectDefinition(t, dir, "AgentServiceConfig", "agent")
	for _, key := range []string{"mirrorRegistryRef", "OSImageCACertRef", "osImages"} {
		if bytes.Contains(od, []byte(key)) {
			t.Errorf("%s rendered without input: %s", key, od)
		}
	}
}
