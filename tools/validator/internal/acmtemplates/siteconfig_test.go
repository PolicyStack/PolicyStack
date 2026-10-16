package acmtemplates

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// hostInventory is the credential platforms/baremetal.yaml names.
func hostInventory(sshKey string) string {
	b := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata: {name: hostinventory, namespace: bm-credentials}
data:
  pullSecret: %s
  ssh-publickey: %s
`, b(`{"auths":{}}`), b(sshKey))
}

const bmc = `apiVersion: v1
kind: Secret
metadata: {name: bmc, namespace: bm-credentials}
data: {username: YWRtaW4=, password: c2VjcmV0}
`

func TestSiteConfigCredentials(t *testing.T) {
	objs := resolve(t, rawTemplate(t, "sno-edge", "present-credentials-sno-edge"), hostInventory(""), bmc)
	s := find(t, objs, "Secret", "sno-edge-bmc")
	if field(s, "data", "username") != "YWRtaW4=" || field(s, "data", "password") != "c2VjcmV0" {
		t.Errorf("BMC secret %v", s.ObjectDefinition.Object["data"])
	}
	find(t, objs, "Namespace", "sno-edge")
	find(t, objs, "Secret", "sno-edge-pull-secret")
}

// TestClusterInstance: SiteConfig rejects spec changes, so the spec is identical across a version
// bump and a rotated credential key; the version lives on a frozen ClusterImageSet.
func TestClusterInstance(t *testing.T) {
	first := resolve(t, rawTemplate(t, "sno-edge", "present-cluster-sno-edge"), hostInventory("ssh-ed25519 KEY"))
	ci := find(t, first, "ClusterInstance", "sno-edge")
	live := []string{
		`apiVersion: hive.openshift.io/v1
kind: ClusterImageSet
metadata: {name: lifecycle-sno-edge}
spec: {releaseImage: "quay.io/openshift-release-dev/ocp-release:4.20.8-multi"}
`,
	}
	liveCI, err := yaml.Marshal(map[string]any{"apiVersion": "siteconfig.open-cluster-management.io/v1alpha1", "kind": "ClusterInstance",
		"metadata": map[string]any{"name": "sno-edge", "namespace": "sno-edge"}, "spec": nested(ci, "spec")})
	if err != nil {
		t.Fatal(err)
	}
	live = append(live, string(liveCI), hostInventory("ssh-ed25519 ROTATED"))
	bumped := resolve(t, rawTemplate(t, "sno-edge", "present-cluster-sno-edge", "version=4.20.9"), live...)

	if got := field(find(t, first, "ClusterImageSet", "lifecycle-sno-edge"), "spec", "releaseImage"); got != fmt.Sprintf(image, "4.20.8") {
		t.Errorf("create image %s", got)
	}
	if got := field(find(t, bumped, "ClusterImageSet", "lifecycle-sno-edge"), "spec", "releaseImage"); got != fmt.Sprintf(image, "4.20.8") {
		t.Errorf("bumped image %s, want the live one", got)
	}
	if a, b := nested(ci, "spec"), nested(find(t, bumped, "ClusterInstance", "sno-edge"), "spec"); !reflect.DeepEqual(a, b) {
		t.Errorf("spec changed across a version bump and key rotation:\n%v\n%v", a, b)
	}
	for path, want := range map[string]string{
		"clusterImageSetNameRef": "lifecycle-sno-edge",
		"sshPublicKey":           "ssh-ed25519 KEY",
		"clusterType":            "SNO",
		"baseDomain":             "dc1.example.com",
	} {
		if got := field(ci, "spec", path); got != want {
			t.Errorf("spec.%s %q, want %q", path, got, want)
		}
	}
	if got := field(ci, "spec", "extraAnnotations", "ManagedCluster", "lifecycle.policystk.io/hub"); got != "acm-dc1" {
		t.Errorf("owner annotation %q", got)
	}
	// The stock ai KlusterletAddonConfig turns application-manager off, which stops Day-2.
	if got := nested(ci, "spec", "suppressedManifests"); !reflect.DeepEqual(got, []any{"KlusterletAddonConfig"}) {
		t.Errorf("suppressedManifests %v", got)
	}
	if nested(find(t, first, "KlusterletAddonConfig", "sno-edge"), "spec", "applicationManager", "enabled") != true {
		t.Error("application-manager is off")
	}
	nodes, _, _ := unstructured.NestedSlice(ci.ObjectDefinition.Object, "spec", "nodes")
	if len(nodes) != 1 {
		t.Fatalf("nodes %v", nodes)
	}
	n := nodes[0].(map[string]any)
	// The fixture's MAC is all digits, which YAML 1.1 could read as a number.
	if n["hostName"] != "sno-edge.dc1.example.com" || n["bootMACAddress"] != "00:11:22:33:44:55" {
		t.Errorf("node %v", n)
	}
	if ref, _, _ := unstructured.NestedString(n, "bmcCredentialsName", "name"); ref != "sno-edge-bmc" {
		t.Errorf("node BMC secret %q", ref)
	}
}

func TestClusterInstanceReady(t *testing.T) {
	raw := rawTemplate(t, "sno-edge", "present-ready-sno-edge")
	ci := func(status string) string {
		return fmt.Sprintf(`apiVersion: siteconfig.open-cluster-management.io/v1alpha1
kind: ClusterInstance
metadata: {name: sno-edge, namespace: sno-edge}
status:
  conditions:
    - {type: RenderedTemplatesApplied, status: "True"}
    - {type: Provisioned, status: "%s"}
`, status)
	}
	for _, tt := range []struct {
		name  string
		live  []string
		ready bool
	}{
		{"nothing yet", nil, false},
		{"installing", []string{ci("False")}, false},
		{"unknown", []string{ci("Unknown")}, false},
		{"provisioned", []string{ci("True")}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			objs := resolve(t, raw, tt.live...)
			if tt.ready != (len(objs) == 0) {
				t.Errorf("ready = %v, got %+v", tt.ready, objs)
			}
		})
	}
}
