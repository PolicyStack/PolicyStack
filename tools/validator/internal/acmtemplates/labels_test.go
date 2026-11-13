package acmtemplates

import (
	"maps"
	"strings"
	"testing"
)

// deployMode is the install.labels entry that makes a hub the parent builds a Global Hub managed
// hub in default mode, escaped for --set-string.
const deployMode = `install.labels.global-hub\.open-cluster-management\.io/deploy-mode=default`

// TestInstallLabels: install.labels reach the ManagedCluster of a hive and a siteconfig cluster,
// and are absent unless set.
func TestInstallLabels(t *testing.T) {
	want := map[string]any{"global-hub.open-cluster-management.io/deploy-mode": "default"}
	for _, tc := range []struct {
		cluster, policy string
		live            []string
		labels          func(objs []object) any
	}{
		{"hive-aws", "present-cluster-hive-aws", nil, func(objs []object) any {
			return nested(find(t, objs, "ManagedCluster", "hive-aws"), "metadata", "labels")
		}},
		{"sno-edge", "present-cluster-sno-edge", []string{hostInventory("ssh-ed25519 KEY")}, func(objs []object) any {
			return nested(find(t, objs, "ClusterInstance", "sno-edge"), "spec", "extraLabels", "ManagedCluster")
		}},
	} {
		t.Run(tc.cluster, func(t *testing.T) {
			if got := tc.labels(resolve(t, rawTemplate(t, tc.cluster, tc.policy), tc.live...)); got != nil {
				t.Errorf("labels %v without install.labels", got)
			}
			got, _ := tc.labels(resolve(t, rawTemplate(t, tc.cluster, tc.policy, deployMode), tc.live...)).(map[string]any)
			if !maps.Equal(got, want) {
				t.Errorf("labels %v, want %v", got, want)
			}
		})
	}
}

// TestInstallLabelsHypershift: the hypershift-addon imports a hosted cluster itself, so labels
// would never reach it.
func TestInstallLabelsHypershift(t *testing.T) {
	if msg := renderError(t, "hcp-kubevirt", deployMode); !strings.Contains(msg, "install.labels needs engine hive or siteconfig") {
		t.Errorf("render error %q", msg)
	}
}
