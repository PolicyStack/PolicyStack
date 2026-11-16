package acmtemplates

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

// precheckSet sets install.precheck to a PipelineRun that must report Succeeded and an AnsibleJob
// that must report successful.
var precheckSet = []string{
	"install.precheck[0].apiVersion=tekton.dev/v1",
	"install.precheck[0].kind=PipelineRun",
	"install.precheck[0].metadata.name=infra-checks-1",
	"install.precheck[0].metadata.namespace=precheck",
	"install.precheck[0].status.conditions[0].type=Succeeded",
	"install.precheck[0].status.conditions[0].status=True",
	"install.precheck[1].apiVersion=tower.ansible.com/v1alpha1",
	"install.precheck[1].kind=AnsibleJob",
	"install.precheck[1].metadata.name=infra-checks-aap",
	"install.precheck[1].metadata.namespace=precheck",
	"install.precheck[1].spec.job_template_name=checks",
	"install.precheck[1].status.ansibleJobResult.status=successful",
}

// precheckEngines are a fixture per engine with its owned root object.
var precheckEngines = []struct{ cluster, root string }{
	{"hcp-agent", live("hypershift.openshift.io/v1beta1", "HostedCluster", "clusters", "hcp-agent", "acm-dc1")},
	{"hive-aws", live("hive.openshift.io/v1", "ClusterDeployment", "hive-aws", "hive-aws", "acm-dc1")},
	{"sno-edge", live("siteconfig.open-cluster-management.io/v1alpha1", "ClusterInstance", "sno-edge", "sno-edge", "acm-dc1")},
}

// TestPrecheck: on every engine, install.precheck is created with its status until the root object
// exists, then renders nothing, so only the first build waits on it.
func TestPrecheck(t *testing.T) {
	for _, tt := range precheckEngines {
		t.Run(tt.cluster, func(t *testing.T) {
			raw := rawTemplate(t, tt.cluster, "present-precheck-"+tt.cluster, append([]string{"version=4.20.8"}, precheckSet...)...)

			objs := resolve(t, raw)
			if len(objs) != 2 {
				t.Fatalf("before the build: %d objects, want 2: %+v", len(objs), objs)
			}
			for _, o := range objs {
				if o.ComplianceType != "musthave" {
					t.Errorf("%s %s, want musthave", o.ObjectDefinition.GetKind(), o.ComplianceType)
				}
			}
			if conds, _ := nested(find(t, objs, "PipelineRun", "infra-checks-1"), "status", "conditions").([]any); len(conds) != 1 {
				t.Errorf("PipelineRun status.conditions %v, want the Succeeded condition to wait on", conds)
			}
			if got := field(find(t, objs, "AnsibleJob", "infra-checks-aap"), "status", "ansibleJobResult", "status"); got != "successful" {
				t.Errorf("AnsibleJob status %q, want successful", got)
			}

			if objs := resolve(t, raw, tt.root); len(objs) != 0 {
				t.Errorf("after the build: %+v, want nothing", objs)
			}
		})
	}
}

// TestPrecheckGate: present-cluster waits on present-precheck when install.precheck is set and on
// present-manifests otherwise, so the build neither skips the checks nor waits on a disabled step.
// Without install.precheck no precheck step renders, so existing clusters see no new template.
func TestPrecheckGate(t *testing.T) {
	for _, tt := range precheckEngines {
		c := tt.cluster
		for _, set := range [][]string{precheckSet, nil} {
			want := "present-manifests-" + c
			if set != nil {
				want = "present-precheck-" + c
			}
			release, files := lifecycleFiles(t, c)
			out := helmTemplate(t, filepath.Join(repoRoot, "lifecycle"), release, files, append([]string{"version=4.20.8"}, set...)...)
			dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(out), 4096)
			var deps []string
			for {
				var p struct {
					Spec struct {
						PolicyTemplates []struct {
							ExtraDependencies []struct{ Name string } `json:"extraDependencies"`
							ObjectDefinition  struct {
								Metadata struct{ Name string } `json:"metadata"`
							} `json:"objectDefinition"`
						} `json:"policy-templates"`
					} `json:"spec"`
				}
				if err := dec.Decode(&p); err == io.EOF {
					break
				} else if err != nil {
					t.Fatal(err)
				}
				for _, pt := range p.Spec.PolicyTemplates {
					if pt.ObjectDefinition.Metadata.Name == "present-cluster-"+c {
						for _, d := range pt.ExtraDependencies {
							deps = append(deps, d.Name)
						}
					}
				}
			}
			if len(deps) != 1 || deps[0] != want {
				t.Errorf("%s: present-cluster depends on %v, want [%s]", c, deps, want)
			}
			if rendered := bytes.Contains(out, []byte("name: present-precheck-"+c)); rendered != (set != nil) {
				t.Errorf("%s: present-precheck rendered %v with install.precheck set %v", c, rendered, set != nil)
			}
		}
	}
}

// TestPrecheckCleanup: cleanup deletes install.precheck next to install.manifests once the cluster
// is gone, by identity alone, so a rebuild runs the checks again instead of passing on the old
// result.
func TestPrecheckCleanup(t *testing.T) {
	set := append([]string{
		"install.manifests[0].apiVersion=v1",
		"install.manifests[0].kind=ConfigMap",
		"install.manifests[0].metadata.name=extra",
		"install.manifests[0].metadata.namespace=precheck",
		"install.manifests[0].data.k=v",
	}, precheckSet...)
	for _, c := range []string{"hcp-retired", "hive-retired", "sno-retired"} {
		t.Run(c, func(t *testing.T) {
			objs := resolve(t, rawTemplate(t, c, "absent-cleanup-"+c, set...))
			for _, o := range []object{
				find(t, objs, "ConfigMap", "extra"),
				find(t, objs, "PipelineRun", "infra-checks-1"),
				find(t, objs, "AnsibleJob", "infra-checks-aap"),
			} {
				if o.ComplianceType != "mustnothave" {
					t.Errorf("%s %s, want mustnothave", o.ObjectDefinition.GetKind(), o.ComplianceType)
				}
				for _, k := range []string{"spec", "status", "data"} {
					if nested(o, k) != nil {
						t.Errorf("%s keeps %s", o.ObjectDefinition.GetKind(), k)
					}
				}
			}
		})
	}
}

// TestPrecheckNull: a layer that nulls install.precheck or install.manifests removes the key, and
// cleanup still renders, so destroy is never blocked.
func TestPrecheckNull(t *testing.T) {
	for _, c := range []string{"hcp-retired", "hive-retired", "sno-retired"} {
		for _, key := range []string{"precheck", "manifests"} {
			if _, ok := withValues(t, c, "install:\n  "+key+":\n", false)["absent-cleanup-"+c]; !ok {
				t.Errorf("%s: install.%s null: no absent-cleanup-%s", c, key, c)
			}
		}
	}
}

// TestPrecheckSchema: an entry without a status to wait on, or without a name, fails the render.
func TestPrecheckSchema(t *testing.T) {
	entry := []string{"install.precheck[0].apiVersion=tekton.dev/v1", "install.precheck[0].kind=PipelineRun"}
	for name, set := range map[string][]string{
		"no status":    append(entry, "install.precheck[0].metadata.name=x"),
		"status empty": append(entry, "install.precheck[0].metadata.name=x", "install.precheck[0].status="),
		"no name":      append(entry, "install.precheck[0].metadata.namespace=x", "install.precheck[0].status.conditions[0].type=Succeeded"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := renderError(t, "hive-aws", set...); !strings.Contains(err, "meet the specifications of the schema") {
				t.Errorf("want a schema error, got %q", err)
			}
		})
	}
	if err := renderError(t, "hive-aws", append(entry, "install.precheck[0].metadata.name=x", "install.precheck[0].status.conditions[0].type=Succeeded")...); err != "" {
		t.Errorf("valid entry: %s", err)
	}
}
