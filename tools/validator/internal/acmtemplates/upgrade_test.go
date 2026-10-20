package acmtemplates

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/stolostron/go-template-utils/v7/pkg/templates"
	"sigs.k8s.io/yaml"
)

const osus = "https://osus.example.com/api/upgrades_info/v1/graph"

var upgradeDir = filepath.Join(repoRoot, "stack/upgrade")

// upgradeRaw renders the upgrade element with the all-on profile and setString.
func upgradeRaw(t *testing.T, setString ...string) map[string]string {
	t.Helper()
	return renderRaw(t, upgradeDir, "upgrade", allOn, setString...)
}

// clusterVersion is a ClusterVersion that completed 4.18.3, with desiredUpdate, Progressing and extra
// status fields filled in.
func clusterVersion(desiredUpdate, progressing, extraStatus string) string {
	return fmt.Sprintf(`apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata: {name: version}
spec:
  clusterID: x
  desiredUpdate: %s
status:
  desired: {version: 4.18.3, image: "quay.io/r@sha256:A"}
  history: [{state: Completed, version: 4.18.3, image: "quay.io/r@sha256:A"}]
  conditions:
  - {type: Progressing, status: "%s", message: "Working towards 4.19.2"}
  - {type: Failing, status: "False", message: "not failing"}
  - {type: Invalid, status: "True", message: "spec.channel is invalid"}
  - {type: Upgradeable, status: "False", message: "admin ack required"}
  - {type: RetrievedUpdates, status: "False", reason: RemoteFailed, message: "Unable to retrieve available updates"}
  - {type: ReleaseAccepted, status: "False", reason: VerifyPayloadVersion, message: "Verifying payload failed"}
%s`, desiredUpdate, progressing, extraStatus)
}

// brownfield is what oc adm upgrade --force leaves: the running image, forced.
const brownfield = `{version: 4.18.3, image: "quay.io/r@sha256:A", force: true}`

// reachable lists 4.19.2 in availableUpdates.
const reachable = `  availableUpdates: [{version: 4.19.2, image: "quay.io/r@sha256:B"}]`

const hostedGuest = `apiVersion: config.openshift.io/v1
kind: Infrastructure
metadata: {name: cluster}
status: {controlPlaneTopology: External}
`

// TestUpgradeTrigger: the trigger clears a leftover image and force so the CVO reads version, leaves
// architecture and upstream alone, and renders nothing mid-upgrade, once done, or on an HCP guest.
func TestUpgradeTrigger(t *testing.T) {
	raw := upgradeRaw(t, "version=4.19.2", "stack.upgrade.config.upstream="+osus)["upgrade-trigger"]
	for _, tt := range []struct {
		name string
		live []string
		want bool
	}{
		{"brownfield", []string{clusterVersion(brownfield, "False", reachable)}, true},
		{"multi-arch", []string{clusterVersion(`{version: 4.18.3, image: "quay.io/r@sha256:A", architecture: Multi}`, "False", reachable)}, true},
		{"progressing", []string{clusterVersion(brownfield, "True", reachable)}, false},
		{"done", []string{strings.ReplaceAll(clusterVersion("{}", "False", ""), "4.18.3", "4.19.2")}, false},
		{"hosted guest", []string{clusterVersion(brownfield, "False", reachable), hostedGuest}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			objs := resolve(t, raw, tt.live...)
			if !tt.want {
				if objs != nil {
					t.Fatalf("resolved %+v, want nothing", objs)
				}
				return
			}
			o := find(t, objs, "ClusterVersion", "version")
			want := map[string]any{"version": "4.19.2", "image": "", "force": false}
			if got := nested(o, "spec", "desiredUpdate"); !reflect.DeepEqual(got, want) {
				t.Errorf("desiredUpdate %v, want %v", got, want)
			}
			if got := nested(o, "spec", "upstream"); got != nil {
				t.Errorf("trigger writes upstream %v", got)
			}
		})
	}
}

// TestUpgradeDisconnected: with no availableUpdates the guard blocks, and channel-pin still writes
// the OSUS upstream (without a channel when none is set) so the CVO can fill them. channel-pin
// renders only for a channel or upstream, and toggles.channel gates both.
func TestUpgradeDisconnected(t *testing.T) {
	raws := upgradeRaw(t, "version=4.19.2", "stack.upgrade.config.upstream="+osus)
	live := clusterVersion("{}", "False", "")
	if objs := resolve(t, raws["upgrade-guard"], live); len(objs) != 1 || objs[0].ComplianceType != "mustnothave" {
		t.Errorf("guard resolved %+v, want one mustnothave", objs)
	}
	o := find(t, resolve(t, raws["channel-pin"], live), "ClusterVersion", "version")
	if got := field(o, "spec", "upstream"); got != osus {
		t.Errorf("channel-pin upstream %q, want %q", got, osus)
	}
	if got := nested(o, "spec", "channel"); got != nil {
		t.Errorf("channel-pin writes channel %v with none set", got)
	}
	if _, ok := upgradeRaw(t, "version=4.19.2")["channel-pin"]; ok {
		t.Error("channel-pin rendered with neither channel nor upstream")
	}
	off := filepath.Join(t.TempDir(), "off.yaml")
	if err := os.WriteFile(off, []byte("stack: {upgrade: {toggles: {channel: false}}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := renderRaw(t, upgradeDir, "upgrade", append(allOn, off), "version=4.19.2", "channel=stable-4.19",
		"stack.upgrade.config.upstream="+osus)["channel-pin"]; ok {
		t.Error("channel-pin rendered with toggles.channel off")
	}
}

// TestUpgradeReason: the guard and progress customMessage, run as the config-policy-controller does
// with ACM's sprig functions, append the ClusterVersion conditions that explain a hold, and fall back
// to the default message when related objects carry no status.
func TestUpgradeReason(t *testing.T) {
	all, fm := sprig.TxtFuncMap(), template.FuncMap{}
	for _, f := range templates.AvailableSprigFunctions() {
		fm[f] = all[f]
	}
	var cv map[string]any
	if err := yaml.Unmarshal([]byte(clusterVersion("{}", "True", "")), &cv); err != nil {
		t.Fatal(err)
	}
	msgs := customMessages(t, upgradeDir, "upgrade", allOn, "version=4.19.2")
	for _, name := range []string{"upgrade-guard", "upgrade-progress"} {
		t.Run(name, func(t *testing.T) {
			tmpl, err := template.New(name).Funcs(fm).Parse(msgs[name])
			if err != nil {
				t.Fatal(err)
			}
			run := func(object map[string]any) string {
				var b strings.Builder
				policy := map[string]any{"status": map[string]any{"relatedObjects": []any{map[string]any{"object": object}}}}
				if err := tmpl.Execute(&b, map[string]any{"DefaultMessage": "D", "Policy": policy}); err != nil {
					t.Fatal(err)
				}
				return b.String()
			}
			got := run(cv)
			for _, want := range []string{"D; ", "Progressing: Working towards 4.19.2", "Invalid: spec.channel is invalid",
				"RetrievedUpdates: Unable to retrieve available updates", "ReleaseAccepted: Verifying payload failed"} {
				if !strings.Contains(got, want) {
					t.Errorf("message %q lacks %q", got, want)
				}
			}
			for _, unwanted := range []string{"Upgradeable", "Failing"} {
				if strings.Contains(got, unwanted) {
					t.Errorf("message %q has %q", got, unwanted)
				}
			}
			if got := run(map[string]any{"kind": "ClusterVersion"}); got != "D" {
				t.Errorf("message without status %q, want the default", got)
			}
		})
	}
}
