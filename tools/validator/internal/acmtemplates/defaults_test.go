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

// TestQuayDefaults: a registry enabled with nothing else set can't be claimed by the first visitor.
func TestQuayDefaults(t *testing.T) {
	o := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/quay"), "quay", quayOn(t, ""), "deploy-bundle"), "Secret", "quay-registry-config-bundle")
	var config map[string]any
	if err := yaml.Unmarshal([]byte(field(o, "stringData", "config.yaml")), &config); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{"FEATURE_USER_INITIALIZE": false, "FEATURE_USER_CREATION": false} {
		if config[key] != want {
			t.Errorf("%s = %v, want %v", key, config[key], want)
		}
	}
}

// TestQuayBootstrap: configure renders only with the initialize API on, and its Job waits for Quay's
// answer before it writes a Secret. The HTML 404 of pods without the API names access_token_uuid.
func TestQuayBootstrap(t *testing.T) {
	dir := filepath.Join(repoRoot, "stack/quay")
	runner := render.New(testenv.Helm(t))
	if err := runner.EnsureDeps(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	res := runner.Template(t.Context(), dir, "quay", quayOn(t, ", policies: {configure: {enabled: true}}"), nil)
	if res.Err == nil || !strings.Contains(res.Stderr, "needs config.featureUserInitialize: true") {
		t.Errorf("configure without the initialize API: %v %s", res.Err, res.Stderr)
	}
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
	if err := json.Unmarshal(objectDefinition(t, dir, "Job", "create-admin-user"), &job); err != nil {
		t.Fatal(err)
	}
	script := strings.Join(job.Spec.Template.Spec.Containers[0].Command, "\n")
	const guard = `grep -q -e '"access_token" *:' -e "non-empty database"; do`
	if wait, create := strings.Index(script, guard), strings.Index(script, "create secret generic"); wait < 0 || create < wait {
		t.Errorf("wait for the answer at %d, first Secret at %d", wait, create)
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

// TestStorageTaint: the MachineSets carry the one taint ODF and the LocalVolumeSet tolerate,
// whatever the role is named.
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
		for _, role := range []string{"storage", "ceph"} {
			t.Run(tt.policy+"/"+role, func(t *testing.T) {
				files := valuesFile(t, "stack: {storageNodes: {config: {role: "+role+"}}}\n")
				objs := resolveTemplate(t, filepath.Join(repoRoot, "stack/storage-nodes"), "storage-nodes", files,
					tt.policy+"-storage-machinesets", infra, worker+tt.value)
				if len(objs) != 1 {
					t.Fatalf("%d MachineSets, want 1", len(objs))
				}
				wantYAML(t, nested(objs[0], "spec", "template", "spec", "taints"),
					`[{key: node.ocs.openshift.io/storage, value: "true", effect: NoSchedule}]`)
			})
		}
	}

	lvs := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/local-storage"), "local-storage-spoke", allOn, "volumes-volumeset"), "LocalVolumeSet", "nvme-localvolumeset")
	wantYAML(t, nested(lvs, "spec", "tolerations"), `[{key: node.ocs.openshift.io/storage, operator: Exists, effect: NoSchedule}]`)
}

// TestNFD: PCI devices are labelled by vendor alone (pci-10de.present), and the operator picks the
// operand image.
func TestNFD(t *testing.T) {
	o := find(t, resolveTemplate(t, filepath.Join(repoRoot, "stack/nfd"), "nfd", allOn, "install-nfd-instance"), "NodeFeatureDiscovery", "nfd-instance")
	if image := nested(o, "spec", "operand", "image"); image != nil {
		t.Errorf("operand image %v, want the operator's", image)
	}
	var config struct {
		Sources struct {
			PCI struct {
				DeviceClassWhitelist []string `json:"deviceClassWhitelist"`
				DeviceLabelFields    []string `json:"deviceLabelFields"`
			} `json:"pci"`
		} `json:"sources"`
	}
	if err := yaml.Unmarshal([]byte(field(o, "spec", "workerConfig", "configData")), &config); err != nil {
		t.Fatal(err)
	}
	pci := config.Sources.PCI
	if !slices.Equal(pci.DeviceLabelFields, []string{"vendor"}) || !slices.Contains(pci.DeviceClassWhitelist, "03") {
		t.Errorf("pci %+v, want display controllers labelled by vendor", pci)
	}
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
