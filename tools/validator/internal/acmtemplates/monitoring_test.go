package acmtemplates

import (
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"testing"

	"sigs.k8s.io/yaml"
)

// clusterMonitoringConfig is the live cluster-monitoring-config holding config.yaml.
func clusterMonitoringConfig(config string) string {
	return `apiVersion: v1
kind: ConfigMap
metadata: {name: cluster-monitoring-config, namespace: openshift-monitoring}
data:
  config.yaml: ` + fmt.Sprintf("%q", config)
}

// monitoringConfig resolves ConfigurationPolicy name of the element in stack/<element> and returns
// the parsed config.yaml of its cluster-monitoring-config.
func monitoringConfig(t *testing.T, element, name string, live ...string) map[string]any {
	t.Helper()
	objs := resolveTemplate(t, filepath.Join(repoRoot, "stack", element), element, allOn, name, live...)
	var config map[string]any
	if err := yaml.Unmarshal([]byte(field(find(t, objs, "ConfigMap", "cluster-monitoring-config"), "data", "config.yaml")), &config); err != nil {
		t.Fatal(err)
	}
	return config
}

// TestUserMonitoringEnable: enableUserWorkload is merged into the live config.yaml, keeping the
// settings other elements own.
func TestUserMonitoringEnable(t *testing.T) {
	for _, tt := range []struct {
		name string
		live []string
		want map[string]any
	}{
		{"absent", nil, map[string]any{"enableUserWorkload": true}},
		{"merged", []string{clusterMonitoringConfig("telemeterClient:\n  enabled: false\n")},
			map[string]any{"enableUserWorkload": true, "telemeterClient": map[string]any{"enabled": false}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := monitoringConfig(t, "user-monitoring", "config-enable", tt.live...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("config.yaml %v, want %v", got, tt.want)
			}
		})
	}
}

// TestUserMonitoringReady: each StatefulSet must have readyReplicas equal to its live spec.replicas,
// 1 when the StatefulSet does not exist yet.
func TestUserMonitoringReady(t *testing.T) {
	const prom = `apiVersion: apps/v1
kind: StatefulSet
metadata: {name: prometheus-user-workload, namespace: openshift-user-workload-monitoring}
spec: {replicas: 2}`
	objs := resolveTemplate(t, filepath.Join(repoRoot, "stack/user-monitoring"), "user-monitoring", allOn, "ready-statefulsets", prom)
	for name, want := range map[string]string{"prometheus-user-workload": "2", "thanos-ruler-user-workload": "1"} {
		got := nested(find(t, objs, "StatefulSet", name), "status", "readyReplicas")
		if _, isString := got.(string); isString || fmt.Sprint(got) != want {
			t.Errorf("%s readyReplicas %#v, want integer %s", name, got, want)
		}
	}
}

// TestInfraMonitoring: every pinned component gets the infra nodeSelector and tolerations, merged
// into the live config.yaml without dropping its other settings.
func TestInfraMonitoring(t *testing.T) {
	live := clusterMonitoringConfig("enableUserWorkload: true\nprometheusK8s:\n  retention: 7d\n")
	config := monitoringConfig(t, "infra-nodes", "workloads-monitoring", live)
	if config["enableUserWorkload"] != true {
		t.Errorf("enableUserWorkload dropped: %v", config)
	}
	placement := map[string]any{
		"nodeSelector": map[string]any{"node-role.kubernetes.io/infra": ""},
		"tolerations": []any{
			map[string]any{"key": "node-role.kubernetes.io/infra", "effect": "NoSchedule"},
			map[string]any{"key": "node-role.kubernetes.io/infra", "effect": "NoExecute"},
		},
	}
	for _, comp := range []string{"alertmanagerMain", "prometheusK8s"} {
		got, _ := config[comp].(map[string]any)
		want := maps.Clone(placement)
		if comp == "prometheusK8s" {
			want["retention"] = "7d"
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s %v, want %v", comp, got, want)
		}
	}
}
