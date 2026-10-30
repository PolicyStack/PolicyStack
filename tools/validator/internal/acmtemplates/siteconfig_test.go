package acmtemplates

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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

// x86Image is the release a siteconfig cluster installs: assisted rates multi-arch Tech Preview.
const x86Image = "quay.io/openshift-release-dev/ocp-release:%s-x86_64"

// TestClusterInstance: SiteConfig rejects spec changes, so the spec is identical across a version
// bump and a rotated credential key; the version lives on a frozen ClusterImageSet.
func TestClusterInstance(t *testing.T) {
	first := resolve(t, rawTemplate(t, "sno-edge", "present-cluster-sno-edge"), hostInventory("ssh-ed25519 KEY"))
	ci := find(t, first, "ClusterInstance", "sno-edge")
	live := []string{
		`apiVersion: hive.openshift.io/v1
kind: ClusterImageSet
metadata: {name: lifecycle-sno-edge}
spec: {releaseImage: "quay.io/openshift-release-dev/ocp-release:4.20.8-x86_64"}
`,
	}
	liveCI, err := yaml.Marshal(map[string]any{"apiVersion": "siteconfig.open-cluster-management.io/v1alpha1", "kind": "ClusterInstance",
		"metadata": map[string]any{"name": "sno-edge", "namespace": "sno-edge", "annotations": map[string]any{"lifecycle.policystk.io/hub": "acm-dc1"}}, "spec": nested(ci, "spec")})
	if err != nil {
		t.Fatal(err)
	}
	live = append(live, string(liveCI), hostInventory("ssh-ed25519 ROTATED"))
	bumped := resolve(t, rawTemplate(t, "sno-edge", "present-cluster-sno-edge", "version=4.20.9"), live...)

	if got := field(find(t, first, "ClusterImageSet", "lifecycle-sno-edge"), "spec", "releaseImage"); got != fmt.Sprintf(x86Image, "4.20.8") {
		t.Errorf("create image %s", got)
	}
	if got := field(find(t, bumped, "ClusterImageSet", "lifecycle-sno-edge"), "spec", "releaseImage"); got != fmt.Sprintf(x86Image, "4.20.8") {
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
	if ci.ComplianceType != "mustonlyhave" || ci.MetadataComplianceType != "musthave" {
		t.Errorf("compliance %s, metadata %s", ci.ComplianceType, ci.MetadataComplianceType)
	}
	if got := field(ci, "metadata", "annotations", "lifecycle.policystk.io/hub"); got != "acm-dc1" {
		t.Errorf("marker %q", got)
	}
	if got := field(ci, "spec", "extraAnnotations", "ManagedCluster", "lifecycle.policystk.io/hub"); got != "acm-dc1" {
		t.Errorf("ManagedCluster marker %q", got)
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

// liveCI is a live ClusterInstance built from the create render, with the given Provisioned reason
// and generations, and each node passed through edit.
func liveCI(t *testing.T, cluster string, reason string, generation, observed int, edit func(n map[string]any) map[string]any) string {
	t.Helper()
	ci := find(t, resolve(t, rawTemplate(t, cluster, "present-cluster-"+cluster), hostInventory("ssh-ed25519 KEY")), "ClusterInstance", cluster)
	spec := nested(ci, "spec").(map[string]any)
	var nodes []any
	for _, n := range spec["nodes"].([]any) {
		if edit != nil {
			if n = edit(n.(map[string]any)); n.(map[string]any) == nil {
				continue
			}
		}
		nodes = append(nodes, n)
	}
	spec["nodes"] = nodes
	b, err := yaml.Marshal(map[string]any{
		"apiVersion": "siteconfig.open-cluster-management.io/v1alpha1", "kind": "ClusterInstance",
		"metadata": map[string]any{"name": cluster, "namespace": cluster, "generation": generation, "annotations": map[string]any{marker: "acm-dc1"}},
		"spec":     spec,
		"status": map[string]any{"observedGeneration": observed, "conditions": []any{
			map[string]any{"type": "Provisioned", "status": "True", "reason": reason},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// bmh is a live BareMetalHost in namespace bm-ha carrying annotations.
func bmh(name string, annotations ...string) string {
	m := ""
	for _, a := range annotations {
		m += fmt.Sprintf("%q: \"true\", ", a)
	}
	return fmt.Sprintf("apiVersion: metal3.io/v1alpha1\nkind: BareMetalHost\nmetadata: {name: %s, namespace: bm-ha, annotations: {%s}}\n", name, m)
}

// agent is a live Agent of host in namespace bm-ha at stage, bound when stage is set.
func agent(host, stage string) string {
	bound := ""
	if stage != "" {
		bound = "spec: {clusterDeploymentName: {name: bm-ha, namespace: bm-ha}}\n"
	}
	return fmt.Sprintf("apiVersion: agent-install.openshift.io/v1beta1\nkind: Agent\nmetadata: {name: a-%s, namespace: bm-ha, labels: {agent-install.openshift.io/bmh: %s}}\n%sstatus: {progress: {currentStage: %q}}\n", host, host, bound, stage)
}

// ciNodes returns the resolved ClusterInstance's nodes by hostName.
func ciNodes(t *testing.T, objs []object, cluster string) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	list, _, _ := unstructured.NestedSlice(find(t, objs, "ClusterInstance", cluster).ObjectDefinition.Object, "spec", "nodes")
	for _, n := range list {
		out[n.(map[string]any)["hostName"].(string)] = n.(map[string]any)
	}
	return out
}

const (
	w0          = "w0.bm-ha.dc1.example.com"
	removeAnnot = "bmac.agent-install.openshift.io/remove-agent-and-node-on-delete"
	drainAnnot  = "bmac.agent-install.openshift.io/drain-timeout"
)

// TestClusterInstanceTypes: clusterType follows the masters; a worker of a one-master cluster waits
// for scale-out.
func TestClusterInstanceTypes(t *testing.T) {
	ha := find(t, resolve(t, rawTemplate(t, "bm-ha", "present-cluster-bm-ha"), hostInventory("k")), "ClusterInstance", "bm-ha")
	if got := field(ha, "spec", "clusterType"); got != "HighlyAvailable" {
		t.Errorf("bm-ha clusterType %q", got)
	}
	if got := nested(ha, "spec", "apiVIPs"); !reflect.DeepEqual(got, []any{"10.20.40.5"}) {
		t.Errorf("apiVIPs %v", got)
	}
	if got := nested(ha, "spec", "machineNetwork"); !reflect.DeepEqual(got, []any{map[string]any{"cidr": "10.20.40.0/24"}}) {
		t.Errorf("machineNetwork %v", got)
	}
	nodes := ciNodes(t, []object{ha}, "bm-ha")
	if len(nodes) != 4 {
		t.Fatalf("nodes %v", nodes)
	}
	m0 := nodes["m0.bm-ha.dc1.example.com"]
	if got, _, _ := unstructured.NestedSlice(m0, "nodeNetwork", "interfaces"); !reflect.DeepEqual(got, []any{map[string]any{"name": "eno1", "macAddress": "00:11:22:33:44:01"}}) {
		t.Errorf("interfaces %v", got)
	}
	if nodes[w0]["role"] != "worker" {
		t.Errorf("worker %v", nodes[w0])
	}
	for _, key := range []string{"remove", "network", "automatedCleaningMode", "bootMode"} {
		if _, ok := nodes[w0][key]; ok {
			t.Errorf("node carries %s: %v", key, nodes[w0])
		}
	}

	// SNO plus a worker: the worker waits for the install, and the type stays SNO.
	host := "install.siteconfig.nodes.w0\\.sno-edge\\.dc1\\.example\\.com"
	set := []string{host + ".role=worker", host + ".bmcAddress=redfish-virtualmedia://10.20.0.12/s", host + ".bootMACAddress=00:11:22:33:44:66"}
	raw := rawTemplate(t, "sno-edge", "present-cluster-sno-edge", set...)
	objs := resolve(t, raw, hostInventory("k"))
	if got := ciNodes(t, objs, "sno-edge"); len(got) != 1 {
		t.Errorf("worker at create: %v", got)
	}
	if got := field(find(t, objs, "ClusterInstance", "sno-edge"), "spec", "clusterType"); got != "SNO" {
		t.Errorf("SNO + worker clusterType %q", got)
	}
	live := liveCI(t, "sno-edge", "InProgress", 1, 1, nil)
	if got := ciNodes(t, resolve(t, raw, live), "sno-edge"); len(got) != 1 {
		t.Errorf("worker while installing: %v", got)
	}
	live = liveCI(t, "sno-edge", "Completed", 1, 1, nil)
	if got := ciNodes(t, resolve(t, raw, live), "sno-edge"); len(got) != 2 {
		t.Errorf("worker after install: %v", got)
	}
}

// TestClusterInstanceScale walks the scale-in sequence and the guards around it.
func TestClusterInstanceScale(t *testing.T) {
	raw := removing(t, "present-cluster-bm-ha")
	base := rawTemplate(t, "bm-ha", "present-cluster-bm-ha")
	annotated := func(n map[string]any) map[string]any {
		if n["hostName"] == w0 {
			n["extraAnnotations"] = map[string]any{"BareMetalHost": map[string]any{removeAnnot: "true"}}
		}
		return n
	}
	pruning := func(n map[string]any) map[string]any {
		if n["hostName"] == w0 {
			annotated(n)
			n["pruneManifests"] = []any{map[string]any{"apiVersion": "metal3.io/v1alpha1", "kind": "BareMetalHost"}}
		}
		return n
	}
	dropped := func(n map[string]any) map[string]any {
		if n["hostName"] == w0 {
			return nil
		}
		return n
	}
	const (
		absent = iota
		annotate
		prune
	)
	for _, tt := range []struct {
		name string
		live []string
		want int
	}{
		{"annotate first", []string{liveCI(t, "bm-ha", "Completed", 1, 1, nil), bmh(w0), agent(w0, "Done")}, annotate},
		{"annotation sent, not yet on the BMH", []string{liveCI(t, "bm-ha", "Completed", 2, 1, annotated), bmh(w0), agent(w0, "Done")}, annotate},
		{"annotated BMH prunes", []string{liveCI(t, "bm-ha", "Completed", 2, 2, annotated), bmh(w0, removeAnnot), agent(w0, "Done")}, prune},
		{"no Agent", []string{liveCI(t, "bm-ha", "Completed", 1, 1, nil), bmh(w0)}, annotate},
		{"unbound Agent", []string{liveCI(t, "bm-ha", "Completed", 1, 1, nil), bmh(w0), agent(w0, "")}, annotate},
		{"BMH still deleting", []string{liveCI(t, "bm-ha", "Completed", 3, 3, pruning), bmh(w0, removeAnnot)}, prune},
		{"BMH gone, generation stale", []string{liveCI(t, "bm-ha", "Completed", 3, 2, pruning)}, prune},
		{"BMH gone, generation done", []string{liveCI(t, "bm-ha", "Completed", 3, 3, pruning)}, absent},
		{"dropped stays dropped", []string{liveCI(t, "bm-ha", "Completed", 4, 4, dropped)}, absent},
		{"never joined with drain-timeout", []string{liveCI(t, "bm-ha", "Completed", 1, 1, nil), bmh(w0, drainAnnot), agent(w0, "Joined")}, annotate},
		{"still installing leaves it alone", []string{liveCI(t, "bm-ha", "InProgress", 1, 1, nil), bmh(w0), agent(w0, "Joined")}, -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n, ok := ciNodes(t, resolve(t, raw, tt.live...), "bm-ha")[w0]
			_, hasAnnot, _ := unstructured.NestedString(n, "extraAnnotations", "BareMetalHost", removeAnnot)
			_, hasPrune := n["pruneManifests"]
			switch tt.want {
			case absent:
				if ok {
					t.Errorf("node kept: %v", n)
				}
			case annotate:
				if !hasAnnot || hasPrune {
					t.Errorf("want the annotation alone: %v", n)
				}
			case prune:
				refs, _ := n["pruneManifests"].([]any)
				if !hasAnnot || len(refs) != 3 {
					t.Errorf("want annotation and 3 prune refs: %v", n)
				}
			default:
				if !ok || hasAnnot || hasPrune {
					t.Errorf("want the plain node: %v", n)
				}
			}
		})
	}

	for _, tt := range []struct {
		name, raw, want string
		live            []string
	}{
		{"refuses a node still joining", raw, "has not finished joining", []string{liveCI(t, "bm-ha", "Completed", 1, 1, nil), bmh(w0), agent(w0, "Rebooting")}},
		{"refuses dropping a node from the fleet", rawTemplate(t, "sno-edge", "present-cluster-sno-edge"), "not install.siteconfig.nodes", []string{liveCI(t, "sno-edge", "Completed", 1, 1, func(n map[string]any) map[string]any {
			n["hostName"] = "renamed.dc1.example.com"
			return n
		})}},
		{"refuses a new master", base, "is a new master", []string{liveCI(t, "bm-ha", "Completed", 1, 1, func(n map[string]any) map[string]any {
			if n["hostName"] == "m2.bm-ha.dc1.example.com" {
				return nil
			}
			return n
		})}},
		{"refuses cancelling after prune", base, "cannot be cancelled", []string{liveCI(t, "bm-ha", "Completed", 3, 3, pruning)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tryResolve(t, tt.raw, tt.live...); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, want %q", err, tt.want)
			}
		})
	}

	// Cancelling before prune renders the plain node; scale-out renders the new worker once complete.
	objs := resolve(t, base, liveCI(t, "bm-ha", "Completed", 2, 2, annotated), bmh(w0, removeAnnot))
	if n := ciNodes(t, objs, "bm-ha")[w0]; n["extraAnnotations"] != nil {
		t.Errorf("cancelled node %v", n)
	}
	objs = resolve(t, base, liveCI(t, "bm-ha", "Completed", 4, 4, dropped))
	if _, ok := ciNodes(t, objs, "bm-ha")[w0]; !ok {
		t.Error("scale-out did not add the worker")
	}
	objs = resolve(t, base, liveCI(t, "bm-ha", "InProgress", 1, 1, dropped))
	if _, ok := ciNodes(t, objs, "bm-ha")[w0]; ok {
		t.Error("scale-out while installing")
	}
}

// TestClusterInstanceSSH: one key at create, trimmed; the live key after, with no credential.
func TestClusterInstanceSSH(t *testing.T) {
	raw := rawTemplate(t, "sno-edge", "present-cluster-sno-edge")
	if got := field(find(t, resolve(t, raw, hostInventory("ssh-ed25519 KEY\n")), "ClusterInstance", "sno-edge"), "spec", "sshPublicKey"); got != "ssh-ed25519 KEY" {
		t.Errorf("key %q", got)
	}
	if _, err := tryResolve(t, raw, hostInventory("ssh-ed25519 ONE\nssh-ed25519 TWO\n")); err == nil || !strings.Contains(err.Error(), "must hold one key") {
		t.Errorf("two keys: %v", err)
	}
	if got := field(find(t, resolve(t, raw, liveCI(t, "sno-edge", "Completed", 1, 1, nil)), "ClusterInstance", "sno-edge"), "spec", "sshPublicKey"); got != "ssh-ed25519 KEY" {
		t.Errorf("live key %q", got)
	}
}

func TestSiteConfigCredentialsEmptyKey(t *testing.T) {
	raw := rawTemplate(t, "sno-edge", "present-credentials-sno-edge")
	for _, data := range []string{"{username: YWRtaW4=}", "{password: c2VjcmV0}"} {
		empty := "apiVersion: v1\nkind: Secret\nmetadata: {name: bmc, namespace: bm-credentials}\ndata: " + data + "\n"
		if _, err := tryResolve(t, raw, hostInventory(""), empty); err == nil || !strings.Contains(err.Error(), "non-empty username and password") {
			t.Errorf("%s: got %v", data, err)
		}
	}
}

// TestSiteConfigSchema: values the schema or the converter refuses before anything reaches the hub.
func TestSiteConfigSchema(t *testing.T) {
	node := "install.siteconfig.nodes.sno-edge\\.dc1\\.example\\.com."
	ha := "install.siteconfig.nodes.m0\\.bm-ha\\.dc1\\.example\\.com."
	w := "install.siteconfig.nodes.w0\\.bm-ha\\.dc1\\.example\\.com."
	// hosts sets n masters on no fleet file, which every fixture's own keys would otherwise satisfy.
	hosts := func(n int) []string {
		out := []string{"hub=acm-dc1", "version=4.20.8"}
		for i := range n {
			host := fmt.Sprintf("install.siteconfig.nodes.m%d.", i)
			out = append(out, fmt.Sprintf("%sbmcAddress=redfish-virtualmedia://10.0.0.%d/s", host, i), fmt.Sprintf("%sbootMACAddress=00:11:22:33:44:%02d", host, i))
		}
		return out
	}
	for _, tt := range []struct {
		name, cluster, want string
		set                 []string
	}{
		{"passthrough key", "sno-edge", "clusterInstance", []string{"install.siteconfig.clusterInstance.clusterType=SNO"}},
		{"node passthrough", "sno-edge", "hostRef", []string{node + "hostRef.name=x"}},
		{"redfish scheme", "sno-edge", "bmcAddress", []string{node + "bmcAddress=redfish://10.0.0.1/s"}},
		{"ipmi scheme", "sno-edge", "bmcAddress", []string{node + "bmcAddress=ipmi://10.0.0.1"}},
		{"no scheme", "sno-edge", "bmcAddress", []string{node + "bmcAddress=10.0.0.1"}},
		{"bad MAC", "sno-edge", "bootMACAddress", []string{node + "bootMACAddress=00:11:22"}},
		{"by-id device", "sno-edge", "deviceName", []string{node + "rootDeviceHints.deviceName=/dev/disk/by-id/x"}},
		{"remove without worker", "sno-edge", "role", []string{node + "remove=true"}},
		{"noProxy alone", "sno-edge", "proxy", []string{"install.siteconfig.proxy.noProxy=.example.com"}},
		{"three VIPs", "bm-ha", "apiVIPs", []string{"install.siteconfig.apiVIPs={1.1.1.1,1.1.1.2,1.1.1.3}"}},
		{"ibi with VIPs", "sno-edge", "Must not validate the schema", []string{"install.siteconfig.templates=ibi", "install.siteconfig.apiVIPs={1.1.1.1}"}},
		{"ibi with remove", "sno-edge", "Must not validate the schema", []string{"install.siteconfig.templates=ibi", node + "role=worker", node + "remove=true"}},
		{"ibi with clusterNetwork", "sno-edge", "Must not validate the schema", []string{"install.siteconfig.templates=ibi", "install.siteconfig.clusterNetwork[0].cidr=10.128.0.0/14", "install.siteconfig.clusterNetwork[0].hostPrefix=23"}},
		{"ibi with serviceNetwork", "sno-edge", "Must not validate the schema", []string{"install.siteconfig.templates=ibi", "install.siteconfig.serviceNetwork={172.30.0.0/16}"}},
		{"uneven VIPs", "bm-ha", "same number", []string{"install.siteconfig.apiVIPs={1.1.1.1,fd00::1}"}},
		{"uppercase host", "sno-edge", "UP", []string{"install.siteconfig.nodes.UP.bmcAddress=redfish-virtualmedia://1.1.1.1/s", "install.siteconfig.nodes.UP.bootMACAddress=00:11:22:33:44:77"}},
		{"two masters", "", "2 masters", append(hosts(2), "install.siteconfig.apiVIPs={1.1.1.1}", "install.siteconfig.ingressVIPs={1.1.1.2}")},
		{"four masters", "bm-ha", "4 masters", []string{w + "role=master"}},
		{"arbiter too old", "bm-ha", "4.19 or later", []string{w + "role=arbiter", "version=4.18.0"}},
		{"one master with VIPs", "sno-edge", "forbidden with one master", []string{"install.siteconfig.apiVIPs={1.1.1.1}", "install.siteconfig.ingressVIPs={1.1.1.2}"}},
		{"three masters without VIPs", "", "required with more than one master", hosts(3)},
		{"one master without machineNetwork", "", "machineNetwork is required", hosts(1)},
		{"duplicate MAC", "bm-ha", "share boot MAC", []string{ha + "bootMACAddress=00:11:22:33:44:02"}},
		{"duplicate BMC, other scheme", "bm-ha", "share boot MAC or BMC", []string{ha + "bmcAddress=idrac-virtualmedia://10.20.0.22/redfish/v1/Systems/1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := renderError(t, tt.cluster, tt.set...); !strings.Contains(err, tt.want) {
				t.Errorf("want %q, got %q", tt.want, err)
			}
		})
	}
	if err := renderError(t, "bm-ha", w+"role=arbiter", "version=4.19.0"); err != "" {
		t.Errorf("arbiter on 4.19: %s", err)
	}
}

// renderError returns the helm template error for the fixture's lifecycle release, "" when it
// renders. An empty cluster renders the bare-metal layers without a fleet file.
func renderError(t *testing.T, cluster string, set ...string) string {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not on PATH")
	}
	release, files := lifecycleFiles(t, cmp.Or(cluster, "bm-ha"))
	if cluster == "" {
		files = files[:len(files)-1]
	}
	args := []string{"template", release, filepath.Join(repoRoot, "lifecycle")}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	for _, s := range set {
		args = append(args, "--set", s)
	}
	if out, err := exec.Command(helm, args...).CombinedOutput(); err != nil {
		return string(out)
	}
	return ""
}

// withValues renders the fixture's lifecycle release with a values file above its fleet file, or, with
// noFleet, in place of it.
func withValues(t *testing.T, cluster, body string, noFleet bool) map[string]string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "values.yaml")
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	release, files := lifecycleFiles(t, cluster)
	if noFleet {
		files = files[:len(files)-1]
	}
	return renderRaw(t, filepath.Join(repoRoot, "lifecycle"), release, append(files, f))
}

// removing is the bm-ha ConfigurationPolicy name with remove: true on its worker.
func removing(t *testing.T, name string) string {
	t.Helper()
	raw, ok := withValues(t, "bm-ha", "install:\n  siteconfig:\n    nodes:\n      "+w0+":\n        remove: true\n", false)[name]
	if !ok {
		t.Fatalf("no ConfigurationPolicy %q", name)
	}
	return raw
}

// TestClusterInstanceSpec compares the whole spec, so each curated key is pinned to its field.
func TestClusterInstanceSpec(t *testing.T) {
	node := func(host, extra string) string {
		return fmt.Sprintf(`{hostName: %s, bmcAddress: "redfish-virtualmedia://10.0.0.1/%s", bootMACAddress: "00:11:22:33:44:%s", bmcCredentialsName: {name: bm-ha-bmc}, templateRefs: [{name: ai-node-templates-v1, namespace: open-cluster-management}]%s}`, host, host, host[:2], extra)
	}
	host := func(name, extra string) string {
		return fmt.Sprintf("      %s: {bmcAddress: \"redfish-virtualmedia://10.0.0.1/%s\", bootMACAddress: \"00:11:22:33:44:%s\"%s}\n", name, name, name[:2], extra)
	}
	const vips = "    apiVIPs: [10.20.40.5]\n    ingressVIPs: [10.20.40.6]\n"
	common := func(clusterType string) string {
		return fmt.Sprintf(`clusterName: bm-ha
baseDomain: dc1.example.com
clusterImageSetNameRef: lifecycle-bm-ha
clusterType: %s
pullSecretRef: {name: bm-ha-pull-secret}
sshPublicKey: KEY
suppressedManifests: [KlusterletAddonConfig]
extraAnnotations: {ManagedCluster: {%s: acm-dc1}}
templateRefs: [{name: ai-cluster-templates-v1, namespace: open-cluster-management}]
`, clusterType, marker)
	}
	for _, tt := range []struct{ name, values, want string }{
		{"three masters", "version: 4.20.8\ninstall:\n  siteconfig:\n" + vips + "    nodes:\n" + host("a0", "") + host("a1", "") + host("a2", ""),
			common("HighlyAvailable") + "apiVIPs: [10.20.40.5]\ningressVIPs: [10.20.40.6]\nnodes:\n- " + node("a0", "") + "\n- " + node("a1", "") + "\n- " + node("a2", "") + "\n"},
		{"arbiter", "version: 4.19.0\ninstall:\n  siteconfig:\n" + vips + "    nodes:\n" + host("a0", "") + host("a1", "") + host("a2", ", role: arbiter"),
			common("HighlyAvailableArbiter") + "apiVIPs: [10.20.40.5]\ningressVIPs: [10.20.40.6]\nnodes:\n- " + node("a0", "") + "\n- " + node("a1", "") + "\n- " + node("a2", ", role: arbiter") + "\n"},
		{"every key", `version: 4.20.8
install:
  siteconfig:
    apiVIPs: [10.20.40.5, "fd00::5"]
    ingressVIPs: [10.20.40.6, "fd00::6"]
    machineNetwork: [10.20.40.0/24]
    clusterNetwork: [{cidr: 10.128.0.0/14, hostPrefix: 23}]
    serviceNetwork: [172.30.0.0/16]
    ntpSources: [ntp.example.com]
    proxy: {httpProxy: "http://p:3128", httpsProxy: "http://p:3128", noProxy: .example.com}
    extraManifests: [enable-crun]
    nodes:
` + host("a0", ", role: master, rootDeviceHints: {deviceName: /dev/sda, minSizeGigabytes: 100}, network: {interfaces: {eno1: \"00:11:22:33:44:a0\"}, config: {dns-resolver: {config: {server: [1.1.1.1]}}}}") + host("a1", "") + host("a2", "") + host("a3", ", role: worker"),
			common("HighlyAvailable") + `apiVIPs: [10.20.40.5, "fd00::5"]
ingressVIPs: [10.20.40.6, "fd00::6"]
machineNetwork: [{cidr: 10.20.40.0/24}]
clusterNetwork: [{cidr: 10.128.0.0/14, hostPrefix: 23}]
serviceNetwork: [{cidr: 172.30.0.0/16}]
additionalNTPSources: [ntp.example.com]
proxy: {httpProxy: "http://p:3128", httpsProxy: "http://p:3128", noProxy: .example.com}
extraManifestsRefs: [{name: enable-crun}]
nodes:
- ` + node("a0", `, role: master, rootDeviceHints: {deviceName: /dev/sda, minSizeGigabytes: 100}, nodeNetwork: {interfaces: [{name: eno1, macAddress: "00:11:22:33:44:a0"}], config: {dns-resolver: {config: {server: [1.1.1.1]}}}}`) + "\n- " + node("a1", "") + "\n- " + node("a2", "") + "\n- " + node("a3", ", role: worker") + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := withValues(t, "bm-ha", "hub: acm-dc1\n"+tt.values, true)["present-cluster-bm-ha"]
			if raw == "" {
				t.Fatal("no present-cluster-bm-ha")
			}
			got, err := json.Marshal(nested(find(t, resolve(t, raw, hostInventory("KEY")), "ClusterInstance", "bm-ha"), "spec"))
			if err != nil {
				t.Fatal(err)
			}
			want, err := yaml.YAMLToJSON([]byte(tt.want))
			if err != nil {
				t.Fatal(err)
			}
			var a, b any
			if err := errors.Join(json.Unmarshal(got, &a), json.Unmarshal(want, &b)); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(a, b) {
				t.Errorf("spec\ngot  %s\nwant %s", got, want)
			}
		})
	}

	// ibi: its templates, the node architecture its BareMetalHost reads, and no VIPs.
	raw := withValues(t, "sno-edge", "install:\n  siteconfig:\n    templates: ibi\n", false)["present-cluster-sno-edge"]
	ci := find(t, resolve(t, raw, hostInventory("k")), "ClusterInstance", "sno-edge")
	if got, _, _ := unstructured.NestedSlice(ci.ObjectDefinition.Object, "spec", "templateRefs"); got[0].(map[string]any)["name"] != "ibi-cluster-templates-v1" {
		t.Errorf("templateRefs %v", got)
	}
	n := ciNodes(t, []object{ci}, "sno-edge")["sno-edge.dc1.example.com"]
	if n["cpuArchitecture"] != "x86_64" {
		t.Errorf("ibi node %v", n)
	}
}

// TestClusterInstanceCompletedIdentical: a live Completed cluster with unchanged values renders its
// own spec, or every installed cluster goes NonCompliant.
func TestClusterInstanceCompletedIdentical(t *testing.T) {
	for _, c := range []string{"sno-edge", "bm-ha"} {
		live := liveCI(t, c, "Completed", 1, 1, nil)
		var m map[string]any
		if err := yaml.Unmarshal([]byte(live), &m); err != nil {
			t.Fatal(err)
		}
		got := nested(find(t, resolve(t, rawTemplate(t, c, "present-cluster-"+c), live), "ClusterInstance", c), "spec")
		if !reflect.DeepEqual(got, m["spec"]) {
			t.Errorf("%s spec changed:\n%v\n%v", c, got, m["spec"])
		}
	}
}

// TestNewestAgent: BMAC acts on the newest Agent of a host, and a stale one must not decide either
// gate, whichever way the list sorts.
func TestNewestAgent(t *testing.T) {
	timed := func(name, stage, at string) string {
		return fmt.Sprintf("apiVersion: agent-install.openshift.io/v1beta1\nkind: Agent\nmetadata: {name: %s, namespace: bm-ha, creationTimestamp: %q, labels: {agent-install.openshift.io/bmh: %s}}\nspec: {clusterDeploymentName: {name: bm-ha, namespace: bm-ha}}\nstatus: {progress: {currentStage: %q}}\n", name, at, w0, stage)
	}
	const before, after = "2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"
	ci := liveCI(t, "bm-ha", "Completed", 1, 1, nil)
	var m []string
	for i := range 3 {
		m = append(m, agent(fmt.Sprintf("m%d.bm-ha.dc1.example.com", i), "Done"))
	}
	ready := rawTemplate(t, "bm-ha", "present-ready-bm-ha")
	remove := removing(t, "present-cluster-bm-ha")
	for _, tt := range []struct {
		name         string
		agents       []string
		ready, allow bool
	}{
		{"stale stuck sorts last", []string{timed("aaa-new", "Done", after), timed("zzz-old", "Installing", before)}, true, true},
		{"stale done sorts last", []string{timed("aaa-new", "Installing", after), timed("zzz-old", "Done", before)}, false, false},
		{"stale stuck sorts first", []string{timed("aaa-old", "Installing", before), timed("zzz-new", "Done", after)}, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			live := append(append([]string{ci, bmh(w0)}, m...), tt.agents...)
			if objs := resolve(t, ready, live...); tt.ready != (len(objs) == 0) {
				t.Errorf("ready = %v, got %+v", tt.ready, objs)
			}
			if _, err := tryResolve(t, remove, live...); tt.allow != (err == nil) {
				t.Errorf("allow = %v, got %v", tt.allow, err)
			}
		})
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
	bmhAgent := func(stage string) string {
		return fmt.Sprintf(`apiVersion: agent-install.openshift.io/v1beta1
kind: Agent
metadata: {name: a, namespace: sno-edge, labels: {agent-install.openshift.io/bmh: sno-edge.dc1.example.com}}
status: {progress: {currentStage: %s}}
`, stage)
	}
	for _, tt := range []struct {
		name  string
		live  []string
		ready bool
	}{
		{"nothing yet", nil, false},
		{"installing", []string{ci("False")}, false},
		{"unknown", []string{ci("Unknown")}, false},
		{"provisioned, host joining", []string{ci("True"), bmhAgent("Joined")}, false},
		{"provisioned, host joined", []string{ci("True"), bmhAgent("Done")}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			objs := resolve(t, raw, tt.live...)
			if tt.ready != (len(objs) == 0) {
				t.Errorf("ready = %v, got %+v", tt.ready, objs)
			}
		})
	}
}

func TestClusterInstanceReadyHosts(t *testing.T) {
	m := func(i int) string { return fmt.Sprintf("m%d.bm-ha.dc1.example.com", i) }
	done := []string{agent(m(0), "Done"), agent(m(1), "Done"), agent(m(2), "Done"), agent(w0, "Done"), bmh(m(0)), bmh(m(1)), bmh(m(2)), bmh(w0)}
	ci := liveCI(t, "bm-ha", "Completed", 1, 1, nil)
	raw := rawTemplate(t, "bm-ha", "present-ready-bm-ha")
	remove := removing(t, "present-ready-bm-ha")
	dropped := liveCI(t, "bm-ha", "Completed", 2, 2, func(n map[string]any) map[string]any {
		if n["hostName"] == w0 {
			return nil
		}
		return n
	})
	for _, tt := range []struct {
		name string
		raw  string
		live []string
		kind string // the pending object's kind, "" when ready
	}{
		{"all joined", raw, append(done[:len(done):len(done)], ci), ""},
		{"not provisioned", raw, []string{liveCI(t, "bm-ha", "InProgress", 1, 1, nil)}, "ClusterInstance"},
		{"worker joining", raw, []string{ci, agent(m(0), "Done"), agent(m(1), "Done"), agent(m(2), "Done"), agent(w0, "Joined"), bmh(w0)}, "BareMetalHost"},
		{"worker not rendered yet", raw, []string{dropped, agent(m(0), "Done"), agent(m(1), "Done"), agent(m(2), "Done")}, "ClusterInstance"},
		{"restored: no Agent, detached", raw, []string{ci, bmh(m(0), "baremetalhost.metal3.io/detached"), bmh(m(1), "baremetalhost.metal3.io/detached"), bmh(m(2), "baremetalhost.metal3.io/detached"), bmh(w0, "baremetalhost.metal3.io/detached")}, ""},
		{"removal pending", remove, append(done[:len(done):len(done)], ci), "BareMetalHost"},
		{"removal waits for the drop", remove, []string{ci, agent(m(0), "Done"), agent(m(1), "Done"), agent(m(2), "Done")}, "ClusterInstance"},
		{"removal done", remove, []string{dropped, agent(m(0), "Done"), agent(m(1), "Done"), agent(m(2), "Done")}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			objs := resolve(t, tt.raw, tt.live...)
			if tt.kind == "" {
				if len(objs) != 0 {
					t.Errorf("want ready, got %+v", objs)
				}
				return
			}
			if len(objs) == 0 || objs[0].ObjectDefinition.GetKind() != tt.kind || objs[0].ComplianceType != "mustnothave" {
				t.Errorf("want pending %s, got %+v", tt.kind, objs)
			}
		})
	}
}
