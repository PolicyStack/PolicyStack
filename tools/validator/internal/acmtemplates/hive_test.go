package acmtemplates

import (
	"encoding/base64"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// credential is the console credential platforms/aws.yaml names, with sshKey as ssh-publickey
// when set.
func credential(sshKey string) string {
	b := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata: {name: aws, namespace: hive-credentials}
data:
  aws_access_key_id: %s
  aws_secret_access_key: %s
  pullSecret: %s
  ssh-privatekey: %s
  ssh-publickey: %s
`, b("AKID"), b("SECRET"), b(`{"auths":{}}`), b("PRIVATE"), b(sshKey))
}

func TestHiveCredentials(t *testing.T) {
	raw := rawTemplate(t, "hive-aws", "present-credentials-hive-aws")
	for _, sshKey := range []string{"ssh-ed25519 AAAA ops", ""} {
		t.Run(fmt.Sprintf("sshKey %q", sshKey), func(t *testing.T) {
			objs := resolve(t, raw, credential(sshKey))
			if got := field(find(t, objs, "Secret", "hive-aws-pull-secret"), "data", ".dockerconfigjson"); got != base64.StdEncoding.EncodeToString([]byte(`{"auths":{}}`)) {
				t.Errorf("pull secret %q, want the credential's pullSecret as stored", got)
			}
			// Only the keys Hive reads: no pull secret or private key in the platform credentials.
			creds, _ := nested(find(t, objs, "Secret", "hive-aws-creds"), "data").(map[string]any)
			if keys := slices.Sorted(maps.Keys(creds)); !slices.Equal(keys, []string{"aws_access_key_id", "aws_secret_access_key"}) {
				t.Errorf("creds keys %v", keys)
			}
			enc := field(find(t, objs, "Secret", "hive-aws-install-config"), "data", "install-config.yaml")
			dec, err := base64.StdEncoding.DecodeString(enc)
			if err != nil {
				t.Fatal(err)
			}
			var ic struct {
				Metadata   struct{ Name string } `json:"metadata"`
				BaseDomain string                `json:"baseDomain"`
				Platform   struct {
					AWS struct{ Region string } `json:"aws"`
				} `json:"platform"`
				SSHKey     *string `json:"sshKey"`
				PullSecret *string `json:"pullSecret"`
				Compute    []struct {
					Architecture string `json:"architecture"`
				} `json:"compute"`
			}
			if err := yaml.Unmarshal(dec, &ic); err != nil {
				t.Fatalf("install-config: %v\n%s", err, dec)
			}
			// Hive accepts a mismatched name or baseDomain silently, so they must match the ClusterDeployment.
			if ic.Metadata.Name != "hive-aws" || ic.BaseDomain != "aws.example.com" || ic.Platform.AWS.Region != "us-east-2" {
				t.Errorf("install-config identity %+v", ic)
			}
			if ic.PullSecret != nil {
				t.Error("install-config sets pullSecret; Hive fills it in")
			}
			if len(ic.Compute) != 1 || ic.Compute[0].Architecture != "amd64" {
				t.Errorf("compute %+v", ic.Compute)
			}
			switch {
			case sshKey == "" && ic.SSHKey != nil:
				t.Errorf("sshKey %q set without ssh-publickey", *ic.SSHKey)
			case sshKey != "" && (ic.SSHKey == nil || *ic.SSHKey != sshKey):
				t.Errorf("sshKey %v, want %q\n%s", ic.SSHKey, sshKey, dec)
			}
		})
	}
	// A missing credential is a template error: the policy stays NonCompliant and nothing follows.
	if _, err := tryResolve(t, raw); err == nil {
		t.Error("resolved without the credential")
	}
}

// TestHiveUserSSHKey: an sshKey in install.hive.installConfig wins over the credential's.
func TestHiveUserSSHKey(t *testing.T) {
	raw := rawTemplate(t, "hive-aws", "present-credentials-hive-aws", "install.hive.installConfig.sshKey=ssh-ed25519 USER")
	enc := field(find(t, resolve(t, raw, credential("ssh-ed25519 CREDENTIAL")), "Secret", "hive-aws-install-config"), "data", "install-config.yaml")
	dec, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	var ic struct {
		SSHKey string `json:"sshKey"`
	}
	if err := yaml.Unmarshal(dec, &ic); err != nil {
		t.Fatal(err)
	}
	if ic.SSHKey != "ssh-ed25519 USER" || strings.Count(string(dec), "sshKey:") != 1 {
		t.Errorf("sshKey %q, want the user's, once:\n%s", ic.SSHKey, dec)
	}
}

// TestHiveReleaseImageFrozen: Hive rejects provisioning changes, so a version bump keeps the live
// ClusterDeployment's release image.
func TestHiveReleaseImageFrozen(t *testing.T) {
	raw := rawTemplate(t, "hive-aws", "present-cluster-hive-aws", "version=4.20.9")
	live := `apiVersion: hive.openshift.io/v1
kind: ClusterDeployment
metadata: {name: hive-aws, namespace: hive-aws}
spec:
  installed: true
  provisioning: {releaseImage: "quay.io/openshift-release-dev/ocp-release:4.20.8-multi"}
`
	for name, tt := range map[string]struct {
		live []string
		want string
	}{
		"create": {nil, fmt.Sprintf(image, "4.20.9")},
		"bump":   {[]string{live}, fmt.Sprintf(image, "4.20.8")},
	} {
		t.Run(name, func(t *testing.T) {
			objs := resolve(t, raw, tt.live...)
			cd := find(t, objs, "ClusterDeployment", "hive-aws")
			if got := field(cd, "spec", "provisioning", "releaseImage"); got != tt.want {
				t.Errorf("releaseImage %s, want %s", got, tt.want)
			}
			if nested(cd, "spec", "installed") != nil {
				t.Error("spec.installed is Hive's to set")
			}
			mc := find(t, objs, "ManagedCluster", "hive-aws")
			if mc.ObjectDefinition.GetAnnotations()["lifecycle.policystk.io/hub"] != "acm-dc1" {
				t.Errorf("ManagedCluster annotations %v", mc.ObjectDefinition.GetAnnotations())
			}
			find(t, objs, "KlusterletAddonConfig", "hive-aws")
		})
	}
}

// managedCluster is a live ManagedCluster with annotations.
func managedCluster(name string, annotations map[string]string) string {
	a, _ := yaml.Marshal(annotations)
	return fmt.Sprintf("apiVersion: cluster.open-cluster-management.io/v1\nkind: ManagedCluster\nmetadata:\n  name: %s\n  annotations:\n%s", name, indent(string(a), "    "))
}

func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(strings.TrimSuffix(s, "\n"), "\n", "\n"+prefix) + "\n"
}

// TestOwnership: each engine's guard passes and detach deletes only a ManagedCluster carrying that
// engine's owner annotations. The hub's own ManagedCluster is local-cluster in the fake hub.
func TestOwnership(t *testing.T) {
	hcpOwner := map[string]string{"open-cluster-management/created-via": "hypershift", "import.open-cluster-management.io/hosting-cluster-name": "local-cluster"}
	hiveOwner := map[string]string{"lifecycle.policystk.io/hub": "acm-dc1"}
	imported := map[string]string{"open-cluster-management/created-via": "other"}
	otherHub := map[string]string{"lifecycle.policystk.io/hub": "acm-dc2"}
	for _, tt := range []struct {
		cluster string
		owner   map[string]string
		foreign []map[string]string
		detach  bool // SiteConfig deletes the ManagedCluster itself, so its absent chain has none
	}{
		{"hcp-retired", hcpOwner, []map[string]string{imported, hiveOwner}, true},
		{"hive-retired", hiveOwner, []map[string]string{imported, hcpOwner, otherHub}, true},
		{"sno-retired", hiveOwner, []map[string]string{imported, hcpOwner, otherHub}, false},
	} {
		t.Run(tt.cluster, func(t *testing.T) {
			raws := rawTemplates(t, tt.cluster)
			guard := hubResolved(t, raws["absent-guard-"+tt.cluster])
			detach, ok := raws["absent-detach-"+tt.cluster]
			if ok != tt.detach {
				t.Fatalf("absent-detach rendered: %v, want %v", ok, tt.detach)
			}
			detach = hubResolved(t, detach)
			// No ManagedCluster: the guard renders nothing (Compliant), detach has nothing to delete.
			if objs := resolve(t, guard); len(objs) != 0 {
				t.Errorf("guard without a ManagedCluster: %+v", objs)
			}
			own := managedCluster(tt.cluster, tt.owner)
			if objs := resolve(t, guard, own); len(objs) != 1 || !maps.Equal(objs[0].ObjectDefinition.GetAnnotations(), tt.owner) {
				t.Errorf("guard of an owned ManagedCluster: %+v", objs)
			}
			if tt.detach {
				if objs := resolve(t, detach, own); len(objs) != 1 || objs[0].ComplianceType != "mustnothave" {
					t.Errorf("detach of an owned ManagedCluster: %+v", objs)
				}
			}
			for _, f := range tt.foreign {
				mc := managedCluster(tt.cluster, f)
				if tt.detach {
					if objs := resolve(t, detach, mc); len(objs) != 0 {
						t.Errorf("detach deletes a ManagedCluster annotated %v", f)
					}
				}
				// The guard asserts the owner annotations, so a foreign ManagedCluster is NonCompliant.
				objs := resolve(t, guard, mc)
				if len(objs) != 1 || objs[0].ComplianceType != "musthave" || !maps.Equal(objs[0].ObjectDefinition.GetAnnotations(), tt.owner) {
					t.Errorf("guard for %v: %+v", f, objs)
				}
			}
		})
	}
}

// hubResolved replaces the {{hub ... hub}} templates the governance propagator resolves before
// the policy reaches the hub's own ManagedCluster, local-cluster.
func hubResolved(t *testing.T, raw string) string {
	t.Helper()
	return strings.ReplaceAll(raw, "{{hub .ManagedClusterName hub}}", "local-cluster")
}
