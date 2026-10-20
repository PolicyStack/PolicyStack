package acmtemplates

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

const marker = "lifecycle.policystk.io/hub"

// live is a live object apiVersion/kind namespace/name, carrying the marker when hub is set.
func live(apiVersion, kind, namespace, name, hub string) string {
	meta := "name: " + name
	if namespace != "" {
		meta += ", namespace: " + namespace
	}
	if hub != "" {
		meta += fmt.Sprintf(", annotations: {%s: %s}", marker, hub)
	}
	return fmt.Sprintf("apiVersion: %s\nkind: %s\nmetadata: {%s}\n", apiVersion, kind, meta)
}

func managedCluster(name, hub string) string {
	return live("cluster.open-cluster-management.io/v1", "ManagedCluster", "", name, hub)
}

// TestOwnership: every lifecycle step that writes or deletes, in both chains of every engine,
// refuses an object named <cluster> lifecycle did not build, and the guard reports it. A root
// marked by another hub, as after a restore, and a marked ManagedCluster without a root, as after a
// failed ClusterDeployment create, are owned.
func TestOwnership(t *testing.T) {
	for _, tt := range []struct {
		clusters                    []string
		apiVersion, kind, namespace string // the root; namespace "" is <cluster>
	}{
		{[]string{"hcp-agent", "hcp-retired"}, "hypershift.openshift.io/v1beta1", "HostedCluster", "clusters"},
		{[]string{"hive-aws", "hive-retired"}, "hive.openshift.io/v1", "ClusterDeployment", ""},
		{[]string{"sno-edge", "sno-retired"}, "siteconfig.open-cluster-management.io/v1alpha1", "ClusterInstance", ""},
	} {
		for _, c := range tt.clusters {
			t.Run(c, func(t *testing.T) {
				ns := tt.namespace
				if ns == "" {
					ns = c
				}
				owned := map[string][]string{
					"nothing":               nil,
					"owned root":            {live(tt.apiVersion, tt.kind, ns, c, "acm-dc1"), managedCluster(c, "")},
					"root of another hub":   {live(tt.apiVersion, tt.kind, ns, c, "acm-dc2")},
					"marked ManagedCluster": {managedCluster(c, "acm-dc1")},
				}
				foreign := map[string][]string{
					"unmarked root":           {live(tt.apiVersion, tt.kind, ns, c, ""), managedCluster(c, "acm-dc1")},
					"imported ManagedCluster": {managedCluster(c, "")},
				}
				if tt.kind == "ClusterInstance" {
					foreign["adoptable ClusterDeployment"] = []string{live("hive.openshift.io/v1", "ClusterDeployment", c, c, "")}
				}
				creds := []string{credential("ssh-ed25519 KEY"), hostInventory("ssh-ed25519 KEY")}
				guards, steps := 0, 0
				for name, raw := range rawTemplates(t, c, "version=4.20.8") {
					switch strings.TrimSuffix(name[strings.Index(name, "-")+1:], "-"+c) {
					case "guard":
						guards++
						for o, l := range owned {
							if objs := resolve(t, raw, l...); len(objs) != 0 {
								t.Errorf("%s, %s: %+v", name, o, objs)
							}
						}
						for f, l := range foreign {
							if objs := resolve(t, raw, l...); len(objs) != 1 || objs[0].ComplianceType != "mustnothave" {
								t.Errorf("%s, %s: %+v", name, f, objs)
							}
						}
					case "ready", "manifests":
					default:
						steps++
						for o, l := range owned {
							if _, err := tryResolve(t, raw, append(l, creds...)...); err != nil && strings.Contains(err.Error(), "error calling fail: ") {
								t.Errorf("%s refuses %s: %v", name, o, err)
							}
						}
						for f, l := range foreign {
							if _, err := tryResolve(t, raw, append(l, creds...)...); err == nil || !strings.Contains(err.Error(), "error calling fail: ") {
								t.Errorf("%s, %s: want refusal, got %v", name, f, err)
							}
						}
					}
				}
				if guards == 0 || steps == 0 {
					t.Errorf("rendered %d guards, %d steps", guards, steps)
				}
			})
		}
	}
}

// TestEnforceIsRaw: only a raw template can refuse, so every enforced lifecycle step is one.
func TestEnforceIsRaw(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot, "lifecycle/values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Stack map[string]struct {
			ConfigPolicies []struct {
				Name              string `json:"name"`
				PolicyRef         string `json:"policyRef"`
				RemediationAction string `json:"remediationAction"`
				RawTemplate       bool   `json:"rawTemplate"`
			} `json:"configPolicies"`
		} `json:"stack"`
	}
	if err := yaml.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	for engine, s := range v.Stack {
		for _, cp := range s.ConfigPolicies {
			if cp.RemediationAction == "enforce" && !cp.RawTemplate {
				t.Errorf("%s %s-%s is enforced without rawTemplate", engine, cp.PolicyRef, cp.Name)
			}
		}
	}
}

// TestMarkerWins: the marker on the root survives install.*.metadata.annotations.
func TestMarkerWins(t *testing.T) {
	for _, tt := range []struct{ cluster, kind, set string }{
		{"hcp-kubevirt", "HostedCluster", "install.hostedCluster.metadata.annotations.lifecycle\\.policystk\\.io/hub=x"},
		{"hive-aws", "ClusterDeployment", "install.hive.clusterDeployment.metadata.annotations.lifecycle\\.policystk\\.io/hub=x"},
	} {
		objs := resolve(t, rawTemplate(t, tt.cluster, "present-cluster-"+tt.cluster, tt.set))
		if got := field(find(t, objs, tt.kind, tt.cluster), "metadata", "annotations", marker); got != "acm-dc1" {
			t.Errorf("%s marker %q", tt.kind, got)
		}
	}
}
