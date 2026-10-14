package acmtemplates

import (
	"fmt"
	"testing"
)

const image = "quay.io/openshift-release-dev/ocp-release:%s-multi"

// hostedCluster is a live HostedCluster at desired with the given history and availableUpdates.
func hostedCluster(name, desired, history, available string) string {
	return fmt.Sprintf(`apiVersion: hypershift.openshift.io/v1beta1
kind: HostedCluster
metadata: {name: %s, namespace: clusters}
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

// TestHostedClusterUpgrade bumps hcp-agent, which sets channel, from 4.20.8 to 4.20.9. The
// HostedCluster image waits for HyperShift to offer the version; each NodePool waits for the
// control plane to complete it.
func TestHostedClusterUpgrade(t *testing.T) {
	raw := rawTemplate(t, "hcp-agent", "present-cluster-hcp-agent", "version=4.20.9")
	old, next := fmt.Sprintf(image, "4.20.8"), fmt.Sprintf(image, "4.20.9")
	done := `[{version: 4.20.8, state: Completed}]`
	tests := []struct {
		name                   string
		live                   []string
		hostedCluster, compute string
		gpu                    string // the gpu pool never exists, so it always starts on next
	}{
		{"create", nil, next, next, next},
		{"held: not an available update", []string{hostedCluster("hcp-agent", "4.20.8", done, "[]"), nodePool("hcp-agent-compute", "4.20.8")}, old, old, next},
		{"available", []string{hostedCluster("hcp-agent", "4.20.8", done, "[{version: 4.20.9, image: x}]"), nodePool("hcp-agent-compute", "4.20.8")}, next, old, next},
		{"control plane upgrading", []string{hostedCluster("hcp-agent", "4.20.9", `[{version: 4.20.9, state: Partial}, {version: 4.20.8, state: Completed}]`, "[]"), nodePool("hcp-agent-compute", "4.20.8")}, next, old, next},
		{"control plane done", []string{hostedCluster("hcp-agent", "4.20.9", `[{version: 4.20.9, state: Completed}, {version: 4.20.8, state: Completed}]`, "[]"), nodePool("hcp-agent-compute", "4.20.8")}, next, next, next},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

// TestHostedClusterNoChannel: without channel a bump reaches the HostedCluster at once.
func TestHostedClusterNoChannel(t *testing.T) {
	raw := rawTemplate(t, "hcp-kubevirt", "present-cluster-hcp-kubevirt", "version=4.20.9")
	objs := resolve(t, raw, hostedCluster("hcp-kubevirt", "4.20.8", `[{version: 4.20.8, state: Completed}]`, "[]"))
	hc := find(t, objs, "HostedCluster", "hcp-kubevirt")
	if got, want := field(hc, "spec", "release", "image"), fmt.Sprintf(image, "4.20.9"); got != want {
		t.Errorf("image %s, want %s", got, want)
	}
	if got := field(hc, "spec", "channel"); got != "" {
		t.Errorf("channel %q, want unset", got)
	}
}

// TestHostedClusterReady: Compliant (nothing rendered) only once the control plane and every
// NodePool report version.
func TestHostedClusterReady(t *testing.T) {
	raw := rawTemplate(t, "hcp-agent", "present-ready-hcp-agent")
	done := hostedCluster("hcp-agent", "4.20.8", `[{version: 4.20.8, state: Completed}]`, "[]")
	tests := []struct {
		name  string
		live  []string
		ready bool
	}{
		{"nothing yet", nil, false},
		{"control plane only", []string{done}, false},
		{"one pool behind", []string{done, nodePool("hcp-agent-compute", "4.20.8"), nodePool("hcp-agent-gpu", "4.20.7")}, false},
		{"control plane partial", []string{hostedCluster("hcp-agent", "4.20.8", `[{version: 4.20.8, state: Partial}]`, "[]"), nodePool("hcp-agent-compute", "4.20.8"), nodePool("hcp-agent-gpu", "4.20.8")}, false},
		{"done", []string{done, nodePool("hcp-agent-compute", "4.20.8"), nodePool("hcp-agent-gpu", "4.20.8")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
