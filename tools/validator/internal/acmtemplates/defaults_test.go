package acmtemplates

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

// quayOn is the baseline and Quay turned on, with nothing else set.
func quayOn(t *testing.T, extra string) []string {
	t.Helper()
	files := valuesFile(t, "stack: {quay: {enabled: true"+extra+"}}\n")
	return []string{files[0], files[len(files)-1]}
}

// TestQuayInitialize: the initialize API follows configure unless values set it, so a registry
// enabled with nothing else set can't be claimed by the first visitor.
func TestQuayInitialize(t *testing.T) {
	for _, tt := range []struct {
		name, values string
		want         bool
	}{
		{"nothing set", "", false},
		{"null", ", config: {featureUserInitialize: null}", false},
		{"configure on", ", policies: {configure: {enabled: true}}", true},
		{"configure off", ", policies: {configure: {enabled: false}}", false},
		{"configure enabled null is on as policy-library reads it", ", policies: {configure: {enabled: null}}", true},
		{"set true", ", config: {featureUserInitialize: true}", true},
		{"set false wins over configure", ", config: {featureUserInitialize: false}, policies: {configure: {enabled: true}}", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/quay"), "quay", quayOn(t, tt.values), "deploy-registry"), "Secret", "quay-registry-config-bundle")
			var config map[string]any
			if err := yaml.Unmarshal([]byte(field(o, "stringData", "config.yaml")), &config); err != nil {
				t.Fatal(err)
			}
			if config["FEATURE_USER_INITIALIZE"] != tt.want || config["FEATURE_USER_CREATION"] != false {
				t.Errorf("config %v, want initialize %v", config, tt.want)
			}
		})
	}
}

// TestQuayBootstrapOnce: an existing Job is left alone, since it can't be changed, unless
// mustnothave is removing it.
func TestQuayBootstrapOnce(t *testing.T) {
	dir := filepath.Join(repoRoot, "stack/quay")
	const job = "apiVersion: batch/v1\nkind: Job\nmetadata: {name: create-admin-user, namespace: quay-enterprise}\n"
	for _, tt := range []struct {
		name, values string
		live         []string
		want         bool // the Job is in what ACM applies
	}{
		{"absent", "", nil, true},
		{"exists", "", []string{job}, false},
		{"exists and being removed", ", templates: {bootstrap: {complianceType: mustnothave}}", []string{job}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := helmTemplate(t, dir, "quay", quayOn(t, ", policies: {configure: {enabled: true"+tt.values+"}}"))
			resolved, _, err := Resolve(out, "local-cluster", liveObjects(t, tt.live...)...)
			if err != nil {
				t.Fatal(err)
			}
			got := slices.ContainsFunc(resolved, func(o Object) bool {
				var m struct {
					Kind string `json:"kind"`
				}
				return o.ConfigPolicy == "configure-bootstrap" && json.Unmarshal(o.Definition, &m) == nil && m.Kind == "Job"
			})
			if got != tt.want {
				t.Errorf("Job applied %v, want %v", got, tt.want)
			}
		})
	}
}

// TestQuayBootstrap: the Job waits for Quay's answer before it writes a Secret, and gives up. The
// HTML 404 of pods without the API names access_token_uuid.
func TestQuayBootstrap(t *testing.T) {
	dir := filepath.Join(repoRoot, "stack/quay")
	var job struct {
		Spec struct {
			Template struct {
				Spec struct {
					Containers []struct {
						Command []string `json:"command"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(bootstrapJob(t, dir), &job); err != nil {
		t.Fatal(err)
	}
	script := strings.Join(job.Spec.Template.Spec.Containers[0].Command, "\n")
	const guard = `grep -q -e '"access_token" *:' -e "non-empty database"; do`
	if wait, create := strings.Index(script, guard), strings.Index(script, "create secret generic"); wait < 0 || create < wait {
		t.Errorf("wait for the answer at %d, first Secret at %d", wait, create)
	}
	if !strings.Contains(script, "if [ $TRIES -ge 90 ]; then") {
		t.Error("the wait has no limit")
	}
	done := regexp.MustCompile(`"access_token" *:|non-empty database`)
	for body, want := range map[string]bool{
		`{"username":"quayadmin","access_token":"t"}`:                        true,
		`{"message":"Cannot initialize user in a non-empty database"}`:       true,
		`<script>{"/api/v1/user/authorizations/<access_token_uuid>":{}}`:     false,
		`{"message":"Cannot initialize user, FEATURE_USER_INITIALIZE is F"}`: false,
	} {
		if done.MatchString(body) != want {
			t.Errorf("%s: want %v", body, want)
		}
	}
}

// bootstrapJob is the rendered bootstrap Job, unresolved.
func bootstrapJob(t *testing.T, dir string) []byte {
	t.Helper()
	resolved, _, err := Resolve(helmTemplate(t, dir, "quay", allOn), "local-cluster")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range resolved {
		var m struct {
			Kind string `json:"kind"`
		}
		if o.ConfigPolicy == "configure-bootstrap" && json.Unmarshal(o.Definition, &m) == nil && m.Kind == "Job" {
			return o.Definition
		}
	}
	t.Fatal("no bootstrap Job rendered")
	return nil
}

// TestStorageTaint: config.taints reaches the MachineSets, as the one taint ODF and the
// LocalVolumeSet tolerate needs, and empty keeps the role's taints.
func TestStorageTaint(t *testing.T) {
	const infra = `apiVersion: config.openshift.io/v1
kind: Infrastructure
metadata: {name: cluster}
status: {infrastructureName: demo}`
	const worker = `apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata: {name: demo-worker-us-east-1a, namespace: openshift-machine-api}
spec:
  template:
    spec:
      providerSpec:
        value:
`
	for _, tt := range []struct{ policy, value string }{
		{"aws", `          ami: {id: ami-1}
          instanceType: m6i.4xlarge
          userDataSecret: {name: worker-user-data}
          credentialsSecret: {name: aws-cloud-credentials}
          placement: {availabilityZone: us-east-1a, region: us-east-1}
          blockDevices: [{ebs: {encrypted: true, iops: 0, volumeType: gp3, volumeSize: 120, kmsKey: {arn: ""}}}]
          securityGroups: [{filters: [{name: "tag:Name", values: [demo-node]}]}]
          subnet: {filters: [{name: "tag:Name", values: [demo-subnet-private-us-east-1a]}]}
          tags: [{name: owner, value: demo}]
`},
		{"vmware", `          numCPUs: 8
          numCoresPerSocket: 2
          memoryMiB: 32768
          diskGiB: 120
          template: rhcos
          workspace: {server: vc.example}
          network: {devices: [{networkName: vm}]}
          userDataSecret: {name: worker-user-data}
          credentialsSecret: {name: vsphere-cloud-credentials}
`},
	} {
		for _, c := range []struct{ name, config, want string }{
			{"default", "taints: []", `[{key: node-role.kubernetes.io/storage, effect: NoSchedule}, {key: node-role.kubernetes.io/storage, effect: NoExecute}]`},
			{"null", "taints: null", `[{key: node-role.kubernetes.io/storage, effect: NoSchedule}, {key: node-role.kubernetes.io/storage, effect: NoExecute}]`},
			{"another role", "role: ceph, taints: []", `[{key: node-role.kubernetes.io/ceph, effect: NoSchedule}, {key: node-role.kubernetes.io/ceph, effect: NoExecute}]`},
			{"odf", `taints: [{key: node.ocs.openshift.io/storage, value: "true", effect: NoSchedule}]`, `[{key: node.ocs.openshift.io/storage, value: "true", effect: NoSchedule}]`},
		} {
			t.Run(tt.policy+"/"+c.name, func(t *testing.T) {
				files := valuesFile(t, "stack: {storageNodes: {config: {"+c.config+"}}}\n")
				objs := resolveTemplate(t, filepath.Join(repoRoot, "stack/storage-nodes"), "storage-nodes", files,
					tt.policy+"-storage-machinesets", infra, worker+tt.value)
				if len(objs) != 1 {
					t.Fatalf("%d MachineSets, want 1", len(objs))
				}
				wantYAML(t, nested(objs[0], "spec", "template", "spec", "taints"), c.want)
			})
		}
	}

	lvs := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/local-storage"), "local-storage-spoke", allOn, "volumes-volumeset"), "LocalVolumeSet", "nvme-localvolumeset")
	wantYAML(t, nested(lvs, "spec", "tolerations"), `[{key: node.ocs.openshift.io/storage, operator: Exists, effect: NoSchedule}]`)
}

// TestNFD: the defaults keep the class labels and the pinned image. GPU detection needs vendor
// alone (pci-10de.present), and an empty operandImage leaves the image to the operator.
func TestNFD(t *testing.T) {
	const pinned = "registry.redhat.io/openshift4/ose-node-feature-discovery-rhel9:v4.20"
	for _, tt := range []struct {
		name, config string
		image        any
		fields       []string
	}{
		{"defaults", "", pinned, []string{"class"}},
		{"vendor", ", config: {pciLabelFields: [vendor]}", pinned, []string{"vendor"}},
		{"operator's image", `, config: {operandImage: ""}`, nil, []string{"class"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			layer := valuesFile(t, "stack: {nfd: {enabled: true"+tt.config+"}}\n")
			files := []string{layer[0], layer[len(layer)-1]}
			o := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/nfd"), "nfd", files, "install-nfd-instance"), "NodeFeatureDiscovery", "nfd-instance")
			if got := nested(o, "spec", "operand", "image"); got != tt.image {
				t.Errorf("operand image %v, want %v", got, tt.image)
			}
			var config struct {
				Sources struct {
					PCI struct {
						DeviceLabelFields []string `json:"deviceLabelFields"`
					} `json:"pci"`
				} `json:"sources"`
			}
			if err := yaml.Unmarshal([]byte(field(o, "spec", "workerConfig", "configData")), &config); err != nil {
				t.Fatal(err)
			}
			if got := config.Sources.PCI.DeviceLabelFields; !slices.Equal(got, tt.fields) {
				t.Errorf("deviceLabelFields %v, want %v", got, tt.fields)
			}
		})
	}
}

// TestExactListsLiteral: an exact template takes literal values, since a template in one would be
// written as text, revoking the value it stood for. Off, templated values keep resolving.
func TestExactListsLiteral(t *testing.T) {
	dir := filepath.Join(repoRoot, "stack/remediations")
	runner := render.New(testenv.Helm(t))
	if err := runner.EnsureDeps(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`'{{ printf "%s" "mirror.example:8443" }}'`, `'{{hub "mirror.example" hub}}'`, `"it's.example"`, `"x` + "`" + `"`} {
		files := valuesFile(t, "stack: {remediations: {config: {allowedRegistries: ["+value+"]}}}\n")
		if res := runner.Template(t.Context(), dir, "remediations", files, nil); res.Err == nil || !strings.Contains(res.Stderr, "take literal values") {
			t.Errorf("exact with %s: %v %s", value, res.Err, res.Stderr)
		}
	}
	files := valuesFile(t, `stack: {remediations: {config: {allowedRegistries: ['{{ printf "%s" "mirror.example:8443" }}']},
  policies: {registries: {templates: {allowlist-exact: {enabled: false}}}}}}
`)
	o := find(t, resolveTemplate(t, dir, "remediations", files, "registries-allowlist"), "Image", "cluster")
	if o.ComplianceType != "musthave" {
		t.Errorf("allowlist complianceType %q, want musthave", o.ComplianceType)
	}
	wantYAML(t, nested(o, "spec", "registrySources", "allowedRegistries"), `["mirror.example:8443"]`)
}

// TestLifecycleGatesPinned: no values layer can clear a gate's InformOnly, in any engine or state.
func TestLifecycleGatesPinned(t *testing.T) {
	for _, tt := range []struct{ cluster, engine, policy string }{
		{"hcp-kubevirt", "hypershift", "present"}, {"hcp-retired", "hypershift", "absent"},
		{"hive-aws", "hive", "present"}, {"hive-retired", "hive", "absent"},
		{"sno-edge", "siteconfig", "present"}, {"sno-retired", "siteconfig", "absent"},
	} {
		gates := []string{"guard", "ready"}
		if tt.policy == "absent" {
			gates = []string{"guard", "detached", "destroyed"}
			if tt.engine == "siteconfig" {
				gates = []string{"guard", "destroyed"}
			}
		}
		for _, override := range []string{"null", `""`} {
			t.Run(tt.cluster+"/"+override, func(t *testing.T) {
				var layer strings.Builder
				layer.WriteString("stack: {" + tt.engine + ": {policies: {" + tt.policy + ": {templates: {")
				for _, g := range gates {
					layer.WriteString(g + ": {remediationAction: " + override + "}, ")
				}
				layer.WriteString("}}}}}\n")
				release, files := lifecycleFiles(t, tt.cluster)
				extra := valuesFile(t, layer.String())
				out := string(helmTemplate(t, filepath.Join(repoRoot, "lifecycle"), release, append(files, extra[len(extra)-1])))
				for _, g := range gates {
					_, spec, ok := strings.Cut(out, "name: "+tt.policy+"-"+g+"-"+tt.cluster+"\n          annotations:")
					if !ok {
						t.Fatalf("gate %s not rendered", g)
					}
					_, action, _ := strings.Cut(spec, "remediationAction: ")
					if action, _, _ = strings.Cut(action, "\n"); action != "InformOnly" {
						t.Errorf("gate %s renders %q", g, action)
					}
				}
			})
		}
	}
}
