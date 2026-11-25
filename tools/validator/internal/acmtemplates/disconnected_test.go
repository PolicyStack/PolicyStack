package acmtemplates

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

const (
	installerCA = "-----BEGIN CERTIFICATE-----\nINSTALLER\n-----END CERTIFICATE-----\n"
	gitCA       = "-----BEGIN CERTIFICATE-----\nGIT\n-----END CERTIFICATE-----\n"
	helmCA      = "-----BEGIN CERTIFICATE-----\nHELM\n-----END CERTIFICATE-----\n"
)

// trustedCARaw renders the disconnected element's trusted-ca template with config.trustedCA set to ca.
func trustedCARaw(t *testing.T, ca string) string {
	t.Helper()
	raw, ok := renderRaw(t, filepath.Join(repoRoot, "stack/disconnected"), "disconnected", allOn,
		"stack.disconnected.config.trustedCA="+ca)["trusted-ca-proxy"]
	if !ok {
		t.Fatal("no ConfigurationPolicy trusted-ca-proxy rendered")
	}
	return raw
}

// userCABundle is the installer's ConfigMap holding additionalTrustBundle.
func userCABundle(data string) string {
	return "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: user-ca-bundle, namespace: openshift-config}\n" + data
}

// TestTrustedCA: user-ca-bundle is the installer's, so it is never written; trusted-cas carries its
// CAs followed by config.trustedCA, exactly, so a CA removed from config leaves the bundle.
func TestTrustedCA(t *testing.T) {
	for _, tt := range []struct {
		name, ca, want string
		live           []string
	}{
		{"user-ca-bundle present", gitCA + helmCA, installerCA + gitCA + helmCA,
			[]string{userCABundle("data:\n  ca-bundle.crt: |\n" + indent(installerCA, "    "))}},
		{"CA removed from config", gitCA, installerCA + gitCA,
			[]string{userCABundle("data:\n  ca-bundle.crt: |\n" + indent(installerCA, "    "))}},
		{"user-ca-bundle absent", gitCA, gitCA, nil},
		{"user-ca-bundle without ca-bundle.crt", gitCA, gitCA, []string{userCABundle("")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			objs := resolve(t, trustedCARaw(t, tt.ca), tt.live...)
			if got := field(find(t, objs, "ConfigMap", "trusted-cas"), "data", "ca-bundle.crt"); got != tt.want {
				t.Errorf("ca-bundle.crt %q, want %q", got, tt.want)
			}
			if got := field(find(t, objs, "Proxy", "cluster"), "spec", "trustedCA", "name"); got != "trusted-cas" {
				t.Errorf("Proxy trustedCA %q", got)
			}
			if len(objs) != 2 {
				t.Errorf("%d objects, want trusted-cas and the Proxy only: %+v", len(objs), objs)
			}
		})
	}
	if objs := resolve(t, trustedCARaw(t, "")); len(objs) != 0 {
		t.Errorf("empty trustedCA resolved to %+v", objs)
	}
}

// sigKey is a binaryData key as oc-mirror writes it.
var sigKey = "sha256-" + strings.Repeat("a", 64) + "-1"

// disconnectedFile writes a values layer that enables the element and returns the all-on profile,
// without testdata/inputs.yaml so its releaseSignatures doesn't merge in, with the layer on top.
func disconnectedFile(t *testing.T, config string) []string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "disconnected.yaml")
	if err := os.WriteFile(f, []byte("stack: {disconnected: {enabled: true, config: "+config+"}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{allOn[0], allOn[1], f}
}

// TestReleaseSignatures: config.releaseSignatures becomes oc-mirror's ConfigMap, labelled for the CVO,
// mustonlyhave on binaryData so a removed key leaves the cluster, musthave on metadata. Empty, the
// policy does not render at all.
func TestReleaseSignatures(t *testing.T) {
	dir := filepath.Join(repoRoot, "stack/disconnected")
	files := disconnectedFile(t, "{releaseSignatures: {"+sigKey+": c2ln}}")
	if out := string(helmTemplate(t, dir, "disconnected", files)); !strings.Contains(out, "- complianceType: mustonlyhave\n              metadataComplianceType: musthave\n              objectDefinition:\n                apiVersion: v1\n                kind: ConfigMap\n                metadata:\n                  name: mirrored-release-signatures") {
		t.Error("mirrored-release-signatures is not mustonlyhave with metadataComplianceType musthave")
	}
	objs := resolveTemplate(t, dir, "disconnected", files, "signatures-release")
	if len(objs) != 1 || objs[0].ComplianceType != "mustonlyhave" {
		t.Fatalf("resolved %+v, want one mustonlyhave ConfigMap", objs)
	}
	cm := find(t, objs, "ConfigMap", "mirrored-release-signatures")
	if got := cm.ObjectDefinition.GetNamespace(); got != "openshift-config-managed" {
		t.Errorf("namespace %q", got)
	}
	if _, ok := cm.ObjectDefinition.GetLabels()["release.openshift.io/verification-signatures"]; !ok {
		t.Errorf("labels %v lack release.openshift.io/verification-signatures", cm.ObjectDefinition.GetLabels())
	}
	if got := nested(cm, "binaryData"); !reflect.DeepEqual(got, map[string]any{sigKey: "c2ln"}) {
		t.Errorf("binaryData %v", got)
	}
	out := string(helmTemplate(t, dir, "disconnected", disconnectedFile(t, "{releaseSignatures: {}}")))
	if strings.Contains(out, "signatures-") {
		t.Error("signatures policy rendered with config.releaseSignatures empty")
	}
}

// TestReleaseSignaturesInvalid: a key the CVO would never read, a value that is not base64 (null
// included, which Helm keeps between layers), and the same ConfigMap still in clusterResources fail
// the render.
func TestReleaseSignaturesInvalid(t *testing.T) {
	dir := filepath.Join(repoRoot, "stack/disconnected")
	helm := testenv.Helm(t)
	for name, config := range map[string]string{
		`key "sha256-abc"`:                            "{releaseSignatures: {sha256-abc: c2ln}}",
		sigKey + " is not a base64":                   "{releaseSignatures: {" + sigKey + ": 'not base64!'}}",
		sigKey + " is not a base64 signature. A null": "{releaseSignatures: {" + sigKey + ": null}}",
		"which config.releaseSignatures owns":         "{releaseSignatures: {" + sigKey + ": c2ln}, clusterResources: [{apiVersion: v1, kind: ConfigMap, metadata: {name: mirrored-release-signatures, namespace: openshift-config-managed}}]}",
	} {
		t.Run(name, func(t *testing.T) {
			tr := render.New(helm).Template(context.Background(), dir, "disconnected", disconnectedFile(t, config), nil)
			if tr.Err == nil || !strings.Contains(string(tr.Stderr), name) {
				t.Fatalf("err %v: %s, want one containing %q", tr.Err, tr.Stderr, name)
			}
		})
	}
}
