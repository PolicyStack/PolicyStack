package acmtemplates

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

var apiHardeningDir = filepath.Join(repoRoot, "stack/api-hardening")

// apiHardening resolves ConfigurationPolicy name with the all-on profile, body on top and live
// objects in the fake cluster.
func apiHardening(t *testing.T, body, name string, live ...string) []object {
	t.Helper()
	return resolveTemplate(t, apiHardeningDir, "api-hardening", valuesFile(t, body), name, live...)
}

// TestAPIHardeningSettings: each template writes only its own field, with musthave on the shared
// APIServer and IngressController, and skips its objects on a hosted control plane guest.
func TestAPIHardeningSettings(t *testing.T) {
	const body = `stack:
  apiHardening:
    config:
      etcdEncryption: aescbc
      auditProfile: AllRequestBodies
      apiServerTLSProfile: Modern
      ingressTLSProfile: Modern
      consoleBanners:
        maintenance: {text: "It's maintenance", location: BannerBottom, link: {href: "https://status.example.com", text: Status}, color: "#fff", backgroundColor: "#0088ce"}
        legal: {text: Authorized use only}
`
	for _, tt := range []struct {
		cp, kind, name, compliance, want string
	}{
		{"config-etcd-encryption", "APIServer", "cluster", "musthave", "encryption: {type: aescbc}"},
		{"config-audit-profile", "APIServer", "cluster", "musthave", "audit: {profile: AllRequestBodies}"},
		{"config-apiserver-tls", "APIServer", "cluster", "musthave", "tlsSecurityProfile: {type: Modern, modern: {}}"},
		{"config-ingress-tls", "IngressController", "default", "musthave", "tlsSecurityProfile: {type: Modern, modern: {}}"},
		{"config-console-banners", "ConsoleNotification", "maintenance", "mustonlyhave",
			`{text: "It's maintenance", location: BannerBottom, link: {href: "https://status.example.com", text: Status}, color: "#fff", backgroundColor: "#0088ce"}`},
		{"config-console-banners", "ConsoleNotification", "legal", "mustonlyhave", "{text: Authorized use only, location: BannerTop}"},
	} {
		t.Run(tt.cp+"/"+tt.name, func(t *testing.T) {
			o := find(t, apiHardening(t, body, tt.cp), tt.kind, tt.name)
			if o.ComplianceType != tt.compliance {
				t.Errorf("complianceType %q, want %s", o.ComplianceType, tt.compliance)
			}
			wantYAML(t, nested(o, "spec"), tt.want)
			if got := apiHardening(t, body, tt.cp, hostedGuest); len(got) != 0 {
				t.Errorf("hosted guest: %+v, want every object skipped", got)
			}
		})
	}
}

// TestAPIHardeningKubeadmin: removal deletes the Secret only with an identity provider and a
// cluster-admin binding to a User or Group outside system: and kube:admin, never on a hosted
// guest; absent reports the Secret everywhere but a hosted guest.
func TestAPIHardeningKubeadmin(t *testing.T) {
	const (
		idp = `{apiVersion: config.openshift.io/v1, kind: OAuth, metadata: {name: cluster},
  spec: {identityProviders: [{name: ldap, type: LDAP}]}}`
		noIDP = `{apiVersion: config.openshift.io/v1, kind: OAuth, metadata: {name: cluster}, spec: {}}`
	)
	crb := func(role, kind, name string) string {
		return `{apiVersion: rbac.authorization.k8s.io/v1, kind: ClusterRoleBinding, metadata: {name: b-` + strings.ToLower(strings.NewReplacer(":", "-").Replace(name)) + `},
  roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: ` + role + `},
  subjects: [{kind: ` + kind + `, name: "` + name + `"}]}`
	}
	// Every cluster carries this binding; it never counts.
	installed := crb("cluster-admin", "Group", "system:masters")
	for _, tt := range []struct {
		name   string
		live   []string
		remove bool
	}{
		{"nothing", nil, false},
		{"identity provider only", []string{idp, installed}, false},
		{"admin user only", []string{noIDP, crb("cluster-admin", "User", "alice")}, false},
		{"admin user", []string{idp, installed, crb("cluster-admin", "User", "alice")}, true},
		{"admin group", []string{idp, crb("cluster-admin", "Group", "ocp-admins")}, true},
		{"system group", []string{idp, crb("cluster-admin", "Group", "system:cluster-admins")}, false},
		{"kubeadmin user", []string{idp, crb("cluster-admin", "User", "kube:admin")}, false},
		{"service account", []string{idp, crb("cluster-admin", "ServiceAccount", "robot")}, false},
		{"other role", []string{idp, crb("admin", "User", "alice")}, false},
		{"hosted guest", []string{idp, crb("cluster-admin", "User", "alice"), hostedGuest}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := apiHardening(t, "", "kubeadmin-removal", tt.live...)
			if !tt.remove {
				if len(got) != 0 {
					t.Fatalf("removal rendered %+v, want nothing", got)
				}
				return
			}
			if o := find(t, got, "Secret", "kubeadmin"); o.ComplianceType != "mustnothave" || field(o, "metadata", "namespace") != "kube-system" {
				t.Errorf("removal %+v, want mustnothave kube-system/kubeadmin", o)
			}
		})
	}
	if o := find(t, apiHardening(t, "", "kubeadmin-absent"), "Secret", "kubeadmin"); o.ComplianceType != "mustnothave" {
		t.Errorf("absent complianceType %q, want mustnothave", o.ComplianceType)
	}
	if got := apiHardening(t, "", "kubeadmin-absent", hostedGuest); len(got) != 0 {
		t.Errorf("absent on hosted guest: %+v, want nothing", got)
	}
}

// TestAPIHardeningKubeadminRequireLogin: with kubeadminRequireLogin, removal also needs a
// cluster-admin User, or a member of a cluster-admin Group, with an identity from a provider OAuth
// lists now, and the authentication ClusterOperator Available and not Degraded.
func TestAPIHardeningKubeadminRequireLogin(t *testing.T) {
	const (
		idp       = `{apiVersion: config.openshift.io/v1, kind: OAuth, metadata: {name: cluster}, spec: {identityProviders: [{name: local, type: HTPasswd}]}}`
		userCRB   = `{apiVersion: rbac.authorization.k8s.io/v1, kind: ClusterRoleBinding, metadata: {name: u}, roleRef: {kind: ClusterRole, name: cluster-admin}, subjects: [{kind: User, name: alice}]}`
		groupCRB  = `{apiVersion: rbac.authorization.k8s.io/v1, kind: ClusterRoleBinding, metadata: {name: g}, roleRef: {kind: ClusterRole, name: cluster-admin}, subjects: [{kind: Group, name: oauth-cluster-admins}]}`
		group     = `{apiVersion: user.openshift.io/v1, kind: Group, metadata: {name: oauth-cluster-admins}, users: [bob, alice]}`
		empty     = `{apiVersion: user.openshift.io/v1, kind: Group, metadata: {name: oauth-cluster-admins}, users: null}`
		loggedIn  = `{apiVersion: user.openshift.io/v1, kind: User, metadata: {name: alice}, identities: ["local:alice"]}`
		otherIdP  = `{apiVersion: user.openshift.io/v1, kind: User, metadata: {name: alice}, identities: ["removed:alice"]}`
		lookalike = `{apiVersion: user.openshift.io/v1, kind: User, metadata: {name: alice}, identities: ["local-old:alice"]}`
	)
	co := func(available, progressing, degraded string) string {
		return `{apiVersion: config.openshift.io/v1, kind: ClusterOperator, metadata: {name: authentication},
  status: {conditions: [{type: Available, status: "` + available + `"}, {type: Progressing, status: "` + progressing + `"}, {type: Degraded, status: "` + degraded + `"}]}}`
	}
	healthy := co("True", "False", "False")
	noIDP := `{apiVersion: config.openshift.io/v1, kind: OAuth, metadata: {name: cluster}, spec: {}}`
	for _, tt := range []struct {
		name   string
		live   []string
		remove bool
	}{
		{"user logged in", []string{idp, userCRB, loggedIn, healthy}, true},
		{"group member logged in", []string{idp, groupCRB, group, loggedIn, healthy}, true},
		{"user never logged in", []string{idp, userCRB, healthy}, false},
		{"empty group", []string{idp, groupCRB, empty, loggedIn, healthy}, false},
		{"group missing", []string{idp, groupCRB, loggedIn, healthy}, false},
		{"identity from removed provider", []string{idp, userCRB, otherIdP, healthy}, false},
		{"provider name prefix", []string{idp, userCRB, lookalike, healthy}, false},
		{"authentication degraded", []string{idp, userCRB, loggedIn, co("True", "False", "True")}, false},
		{"authentication unavailable", []string{idp, userCRB, loggedIn, co("False", "False", "False")}, false},
		{"authentication progressing", []string{idp, userCRB, loggedIn, co("True", "True", "False")}, false},
		{"no identity provider", []string{noIDP, userCRB, loggedIn, healthy}, false},
		{"authentication missing", []string{idp, userCRB, loggedIn}, false},
		{"hosted guest", []string{idp, userCRB, loggedIn, healthy, hostedGuest}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := apiHardening(t, "stack: {apiHardening: {config: {kubeadminRequireLogin: true}}}\n", "kubeadmin-removal", tt.live...)
			if !tt.remove {
				if len(got) != 0 {
					t.Fatalf("removal rendered %+v, want nothing", got)
				}
				return
			}
			if o := find(t, got, "Secret", "kubeadmin"); o.ComplianceType != "mustnothave" {
				t.Errorf("removal complianceType %q, want mustnothave", o.ComplianceType)
			}
		})
	}
	// Off, a binding to a Group nobody belongs to still passes, as documented.
	if got := apiHardening(t, "", "kubeadmin-removal", idp, groupCRB, empty); len(got) != 1 {
		t.Errorf("default removal %+v, want the Secret deleted", got)
	}
}

// TestAPIHardeningRejected: a value outside a key's documented set fails the render.
func TestAPIHardeningRejected(t *testing.T) {
	runner := render.New(testenv.Helm(t))
	if err := runner.EnsureDeps(context.Background(), apiHardeningDir); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, config, want string }{
		{"kms", `{etcdEncryption: KMS}`, `etcdEncryption: "KMS" must be one of aesgcm, aescbc, identity`},
		{"audit none", `{auditProfile: None}`, `auditProfile: "None" must be one of`},
		{"apiserver old", `{apiServerTLSProfile: Old}`, `apiServerTLSProfile: "Old" must be one of Intermediate, Modern`},
		{"ingress custom", `{ingressTLSProfile: Custom}`, `ingressTLSProfile: "Custom" must be one of`},
		{"banner text", `{consoleBanners: {b: {location: BannerTop}}}`, "consoleBanners.b.text is required"},
		{"banner location", `{consoleBanners: {b: {text: x, location: Top}}}`, `consoleBanners.b.location: "Top" must be one of`},
		{"banner http", `{consoleBanners: {b: {text: x, link: {href: "http://x", text: y}}}}`, "consoleBanners.b.link.href must start with https://"},
		{"require login string", `{kubeadminRequireLogin: "false"}`, "kubeadminRequireLogin must be true or false"},
		{"banner link text", `{consoleBanners: {b: {text: x, link: {href: "https://x"}}}}`, "consoleBanners.b.link.text is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tr := runner.Template(context.Background(), apiHardeningDir, "api-hardening", valuesFile(t, "stack: {apiHardening: {config: "+tt.config+"}}\n"), nil)
			if tr.Err == nil || !strings.Contains(string(tr.Stderr), tt.want) {
				t.Fatalf("want %q, got err %v: %s", tt.want, tr.Err, tr.Stderr)
			}
		})
	}
}
