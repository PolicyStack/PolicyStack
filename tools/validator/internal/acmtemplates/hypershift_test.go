package acmtemplates

import (
	"fmt"
	"strings"
	"testing"
)

const image = "quay.io/openshift-release-dev/ocp-release:%s-multi"

// hostedCluster is a live HostedCluster at desired with the given history and availableUpdates.
func hostedCluster(name, desired, history, available string) string {
	return fmt.Sprintf(`apiVersion: hypershift.openshift.io/v1beta1
kind: HostedCluster
metadata: {name: %s, namespace: clusters, annotations: {lifecycle.policystk.io/hub: acm-dc1}}
spec:
  release: {image: "%s"}
status:
  version:
    desired: {version: %s}
    history: %s
    availableUpdates: %s
`, name, fmt.Sprintf(image, "4.20.8"), desired, history, available)
}

func nodePool(name, version string) string {
	return fmt.Sprintf(`apiVersion: hypershift.openshift.io/v1beta1
kind: NodePool
metadata: {name: %s, namespace: clusters}
spec:
  release: {image: "%s"}
status: {version: %s}
`, name, fmt.Sprintf(image, version), version)
}

// TestHostedClusterUpgrade moves hcp-agent, which sets channel, from 4.20.8. The HostedCluster
// image waits for HyperShift to offer the version and never goes down; each NodePool waits for the
// control plane's newest Completed version to be the target.
func TestHostedClusterUpgrade(t *testing.T) {
	old, next, prev := fmt.Sprintf(image, "4.20.8"), fmt.Sprintf(image, "4.20.9"), fmt.Sprintf(image, "4.20.7")
	done := `[{version: 4.20.8, state: Completed}]`
	tests := []struct {
		name, version          string
		live                   []string
		hostedCluster, compute string
		gpu                    string // the gpu pool never exists, so it always starts on version
	}{
		{"create", "4.20.9", nil, next, next, next},
		{"held: not an available update", "4.20.9", []string{hostedCluster("hcp-agent", "4.20.8", done, "[]"), nodePool("hcp-agent-compute", "4.20.8")}, old, old, next},
		{"available", "4.20.9", []string{hostedCluster("hcp-agent", "4.20.8", done, "[{version: 4.20.9, image: x}]"), nodePool("hcp-agent-compute", "4.20.8")}, next, old, next},
		{"control plane upgrading", "4.20.9", []string{hostedCluster("hcp-agent", "4.20.9", `[{version: 4.20.9, state: Partial}, {version: 4.20.8, state: Completed}]`, "[]"), nodePool("hcp-agent-compute", "4.20.8")}, next, old, next},
		{"control plane done", "4.20.9", []string{hostedCluster("hcp-agent", "4.20.9", `[{version: 4.20.9, state: Completed}, {version: 4.20.8, state: Completed}]`, "[]"), nodePool("hcp-agent-compute", "4.20.8")}, next, next, next},
		{"downgrade held", "4.20.7", []string{hostedCluster("hcp-agent", "4.20.8", done, "[]"), nodePool("hcp-agent-compute", "4.20.8")}, old, old, prev},
		{"older version in history", "4.20.7", []string{hostedCluster("hcp-agent", "4.20.8", `[{version: 4.20.8, state: Completed}, {version: 4.20.7, state: Completed}]`, "[]"), nodePool("hcp-agent-compute", "4.20.8")}, old, old, prev},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := rawTemplate(t, "hcp-agent", "present-cluster-hcp-agent", "version="+tt.version)
			objs := resolve(t, raw, tt.live...)
			hc := find(t, objs, "HostedCluster", "hcp-agent")
			if got := field(hc, "spec", "release", "image"); got != tt.hostedCluster {
				t.Errorf("HostedCluster image %s, want %s", got, tt.hostedCluster)
			}
			if got := field(hc, "spec", "channel"); got != "stable-4.20" {
				t.Errorf("HostedCluster channel %q, want stable-4.20", got)
			}
			for pool, want := range map[string]string{"hcp-agent-compute": tt.compute, "hcp-agent-gpu": tt.gpu} {
				if got := field(find(t, objs, "NodePool", pool), "spec", "release", "image"); got != want {
					t.Errorf("%s image %s, want %s", pool, got, want)
				}
			}
		})
	}
}

// TestHostedClusterNoChannel: without channel a bump reaches the HostedCluster at once, but a
// version below the control plane's desired one is never applied.
func TestHostedClusterNoChannel(t *testing.T) {
	old := fmt.Sprintf(image, "4.20.8")
	live := hostedCluster("hcp-kubevirt", "4.20.8", `[{version: 4.20.8, state: Completed}]`, "[]")
	tests := []struct {
		name, version string
		live          []string
		want          string
	}{
		{"create", "4.20.9", nil, fmt.Sprintf(image, "4.20.9")},
		{"upgrade", "4.20.9", []string{live}, fmt.Sprintf(image, "4.20.9")},
		{"same", "4.20.8", []string{live}, old},
		{"z-down", "4.20.7", []string{live}, old},
		{"y-down", "4.19.20", []string{live}, old},
		{"revert mid-upgrade", "4.20.8", []string{strings.ReplaceAll(hostedCluster("hcp-kubevirt", "4.20.9", `[{version: 4.20.9, state: Partial}, {version: 4.20.8, state: Completed}]`, "[]"), old, fmt.Sprintf(image, "4.20.9"))}, fmt.Sprintf(image, "4.20.9")},
		{"pre-release target", "4.21.0-rc.1", []string{live}, fmt.Sprintf(image, "4.21.0-rc.1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := rawTemplate(t, "hcp-kubevirt", "present-cluster-hcp-kubevirt", "version="+tt.version)
			hc := find(t, resolve(t, raw, tt.live...), "HostedCluster", "hcp-kubevirt")
			if got := field(hc, "spec", "release", "image"); got != tt.want {
				t.Errorf("image %s, want %s", got, tt.want)
			}
			if got := field(hc, "spec", "channel"); got != "" {
				t.Errorf("channel %q, want unset", got)
			}
		})
	}
}

// TestHostedClusterReady: Compliant (nothing rendered) only once the control plane's newest
// Completed version and every NodePool are at version.
func TestHostedClusterReady(t *testing.T) {
	done := hostedCluster("hcp-agent", "4.20.8", `[{version: 4.20.8, state: Completed}]`, "[]")
	tests := []struct {
		name, version string
		live          []string
		ready         bool
	}{
		{"nothing yet", "4.20.8", nil, false},
		{"control plane only", "4.20.8", []string{done}, false},
		{"one pool behind", "4.20.8", []string{done, nodePool("hcp-agent-compute", "4.20.8"), nodePool("hcp-agent-gpu", "4.20.7")}, false},
		{"control plane partial", "4.20.8", []string{hostedCluster("hcp-agent", "4.20.8", `[{version: 4.20.8, state: Partial}]`, "[]"), nodePool("hcp-agent-compute", "4.20.8"), nodePool("hcp-agent-gpu", "4.20.8")}, false},
		{"done", "4.20.8", []string{done, nodePool("hcp-agent-compute", "4.20.8"), nodePool("hcp-agent-gpu", "4.20.8")}, true},
		{"older version in history", "4.20.7", []string{hostedCluster("hcp-agent", "4.20.8", `[{version: 4.20.8, state: Completed}, {version: 4.20.7, state: Completed}]`, "[]"), nodePool("hcp-agent-compute", "4.20.7"), nodePool("hcp-agent-gpu", "4.20.7")}, false},
		{"downgrade held mid-upgrade", "4.20.8", []string{hostedCluster("hcp-agent", "4.20.9", `[{version: 4.20.9, state: Partial}, {version: 4.20.8, state: Completed}]`, "[]"), nodePool("hcp-agent-compute", "4.20.8"), nodePool("hcp-agent-gpu", "4.20.8")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := rawTemplate(t, "hcp-agent", "present-ready-hcp-agent", "version="+tt.version)
			objs := resolve(t, raw, tt.live...)
			if tt.ready != (len(objs) == 0) {
				t.Errorf("ready = %v, got %+v", tt.ready, objs)
			}
			if !tt.ready && find(t, objs, "HostedCluster", "hcp-agent").ComplianceType != "mustnothave" {
				t.Errorf("not ready must force NonCompliant with mustnothave on the HostedCluster")
			}
		})
	}
}
