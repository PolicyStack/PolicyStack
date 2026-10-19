package acmtemplates

import (
	"path/filepath"
	"testing"
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
