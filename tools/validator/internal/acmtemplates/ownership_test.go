package acmtemplates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
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
		{[]string{"hive-aws", "hive-retired", "hive-vsphere"}, "hive.openshift.io/v1", "ClusterDeployment", ""},
		{[]string{"sno-edge", "bm-ha", "sno-retired"}, "siteconfig.open-cluster-management.io/v1alpha1", "ClusterInstance", ""},
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

// TestEnforceIsRaw: only a raw template can refuse, so every lifecycle step the render enforces is
// object-templates-raw, for every hub-built fixture.
func TestEnforceIsRaw(t *testing.T) {
	clusters, err := fixtures.LoadDir(filepath.Join(repoRoot, "tools/validator/testdata/clusters"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range clusters {
		if c.Hub == "" {
			continue
		}
		release, files := lifecycleFiles(t, c.Name)
		dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(helmTemplate(t, filepath.Join(repoRoot, "lifecycle"), release, files)), 4096)
		for {
			var p policy
			if err := dec.Decode(&p); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			for _, pt := range p.Spec.PolicyTemplates {
				var cp struct {
					Kind     string `json:"kind"`
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
					Spec struct {
						RemediationAction string `json:"remediationAction"`
						Raw               string `json:"object-templates-raw"`
					} `json:"spec"`
				}
				if json.Unmarshal(pt.ObjectDefinition, &cp) == nil && cp.Kind == "ConfigurationPolicy" && cp.Spec.RemediationAction == "enforce" && cp.Spec.Raw == "" {
					t.Errorf("%s: %s is enforced without object-templates-raw", c.Name, cp.Metadata.Name)
				}
			}
		}
	}
}

// TestGatesInformOnly: every lifecycle, upgrade and storage-nodes template that is not enforced is
// InformOnly, so a Policy-level enforce cannot turn a gate into a write or delete.
func TestGatesInformOnly(t *testing.T) {
	for _, dir := range []string{"lifecycle", "stack/upgrade", "stack/storage-nodes"} {
		el, err := chart.Load(filepath.Join(repoRoot, dir), "policy")
		if err != nil {
			t.Fatal(err)
		}
		for _, nc := range el.Components {
			c := nc.Component
			for _, p := range c.Policies.Sorted() {
				for _, tpl := range p.Templates.Sorted() {
					if a := c.TemplateAction(p, tpl); a != "enforce" && a != "InformOnly" {
						t.Errorf("%s %s %s-%s: remediationAction %q, want enforce or InformOnly", dir, nc.Key, p.Name, tpl.Name, a)
					}
				}
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
