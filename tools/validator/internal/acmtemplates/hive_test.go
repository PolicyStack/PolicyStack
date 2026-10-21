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
			for _, o := range objs {
				if o.ObjectDefinition.GetName() == "hive-aws-certs" {
					t.Error("hive-aws-certs rendered off vSphere")
				}
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

// vsphereCredential is the console vSphere credential platforms/vmware.yaml names.
func vsphereCredential(password string) string {
	b := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata: {name: vsphere, namespace: hive-credentials}
data:
  vCenter: %s
  username: %s
  password: %s
  cacertificate: %s
  pullSecret: %s
  ssh-publickey: %s
`, b("vcenter.example.com"), b("administrator@vsphere.local"), b(password), b("CA"), b(`{"auths":{}}`), b("ssh-ed25519 AAAA ops"))
}

// TestHiveVSphere: the vCenter login reaches only the install-config, the CA its own Secret, and
// the ClusterDeployment gets the same platform as infrastructure with a blank login.
func TestHiveVSphere(t *testing.T) {
	const password = `p@ss: "w'rd #1` // YAML-significant characters
	objs := resolve(t, rawTemplate(t, "hive-vsphere", "present-credentials-hive-vsphere"), vsphereCredential(password))
	creds, _ := nested(find(t, objs, "Secret", "hive-vsphere-creds"), "data").(map[string]any)
	if keys := slices.Sorted(maps.Keys(creds)); !slices.Equal(keys, []string{"password", "username"}) {
		t.Errorf("creds keys %v", keys)
	}
	if got := field(find(t, objs, "Secret", "hive-vsphere-certs"), "data", ".cacert"); got != base64.StdEncoding.EncodeToString([]byte("CA")) {
		t.Errorf("certs .cacert %q", got)
	}
	dec, err := base64.StdEncoding.DecodeString(field(find(t, objs, "Secret", "hive-vsphere-install-config"), "data", "install-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var ic struct {
		Metadata struct{ Name string } `json:"metadata"`
		Platform struct {
			VSphere struct {
				VCenters       []struct{ Server, User, Password string } `json:"vcenters"`
				FailureDomains []any                                     `json:"failureDomains"`
			} `json:"vsphere"`
		} `json:"platform"`
		SSHKey string `json:"sshKey"`
	}
	if err := yaml.Unmarshal(dec, &ic); err != nil {
		t.Fatalf("install-config: %v\n%s", err, dec)
	}
	vs := ic.Platform.VSphere
	if ic.Metadata.Name != "hive-vsphere" || len(vs.FailureDomains) != 1 || ic.SSHKey != "ssh-ed25519 AAAA ops" {
		t.Errorf("install-config %+v", ic)
	}
	if len(vs.VCenters) != 1 || vs.VCenters[0].Server != "vcenter.example.com" || vs.VCenters[0].User != "administrator@vsphere.local" || vs.VCenters[0].Password != password {
		t.Errorf("install-config vcenters %+v", vs.VCenters)
	}

	cd := find(t, resolve(t, rawTemplate(t, "hive-vsphere", "present-cluster-hive-vsphere")), "ClusterDeployment", "hive-vsphere")
	if c, k := field(cd, "spec", "platform", "vsphere", "credentialsSecretRef", "name"), field(cd, "spec", "platform", "vsphere", "certificatesSecretRef", "name"); c != "hive-vsphere-creds" || k != "hive-vsphere-certs" {
		t.Errorf("secret refs %q %q", c, k)
	}
	vcs, _ := nested(cd, "spec", "platform", "vsphere", "infrastructure", "vcenters").([]any)
	if len(vcs) != 1 {
		t.Fatalf("infrastructure vcenters %v", vcs)
	}
	if v, _ := vcs[0].(map[string]any); v["user"] != "" || v["password"] != "" || v["server"] != "vcenter.example.com" {
		t.Errorf("infrastructure vcenter %v, want a blank login", v)
	}
	if fds, _ := nested(cd, "spec", "platform", "vsphere", "infrastructure", "failureDomains").([]any); len(fds) != 1 {
		t.Errorf("infrastructure failureDomains %v", fds)
	}
}

// TestHiveReleaseImageFrozen: Hive rejects provisioning changes, so a version bump keeps the live
// ClusterDeployment's release image.
func TestHiveReleaseImageFrozen(t *testing.T) {
	raw := rawTemplate(t, "hive-aws", "present-cluster-hive-aws", "version=4.20.9")
	live := `apiVersion: hive.openshift.io/v1
kind: ClusterDeployment
metadata: {name: hive-aws, namespace: hive-aws, annotations: {lifecycle.policystk.io/hub: acm-dc1}}
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
			for _, o := range []object{cd, find(t, objs, "ManagedCluster", "hive-aws")} {
				if o.ObjectDefinition.GetAnnotations()["lifecycle.policystk.io/hub"] != "acm-dc1" {
					t.Errorf("%s annotations %v", o.ObjectDefinition.GetKind(), o.ObjectDefinition.GetAnnotations())
				}
			}
			find(t, objs, "KlusterletAddonConfig", "hive-aws")
		})
	}
}

func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(strings.TrimSuffix(s, "\n"), "\n", "\n"+prefix) + "\n"
}
