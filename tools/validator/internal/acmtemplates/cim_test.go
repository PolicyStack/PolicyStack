package acmtemplates

import (
	"bytes"
	"path/filepath"
	"reflect"
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
		for i := 0; i < len(kv); i += 2 {
			out = append(out, map[string]any{"type": kv[i], "status": kv[i+1]})
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
