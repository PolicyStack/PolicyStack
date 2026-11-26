package acmtemplates

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

var oauthDir = filepath.Join(repoRoot, "stack/oauth")

// oauth resolves ConfigurationPolicy name with the all-on profile, whose inputs hold providers
// local (HTPasswd), sso (OpenID), corp (LDAP), the disabled retired and clusterAdmins [alice], plus
// config on top.
func oauth(t *testing.T, config, name string, live ...string) []object {
	t.Helper()
	return resolveTemplate(t, oauthDir, "oauth", valuesFile(t, "policyNamespace: pstack\nstack: {oauth: {config: "+config+"}}\n"), name, live...)
}

// TestOAuthObjects: one mustonlyhave OAuth cluster with every enabled provider in name order, the
// Secrets and CA bundles it names copied ahead of it, and the cluster-admin Group and binding.
func TestOAuthObjects(t *testing.T) {
	objs := oauth(t, `{identityProviders: {corp: {mappingMethod: add}}}`, "config-oauth")
	want := []string{
		"ClusterRoleBinding/oauth-cluster-admins", "ConfigMap/oauth-sso-ca", "Group/oauth-cluster-admins",
		"OAuth/cluster", "Secret/oauth-corp", "Secret/oauth-local", "Secret/oauth-sso",
	}
	if got := names(objs); !slices.Equal(got, want) {
		t.Fatalf("objects %v, want %v", got, want)
	}
	if objs[len(objs)-3].ObjectDefinition.GetKind() != "OAuth" {
		t.Errorf("OAuth is not after every Secret and ConfigMap: %v", objs)
	}
	for _, o := range objs {
		if o.ComplianceType != "mustonlyhave" {
			t.Errorf("%s complianceType %q, want mustonlyhave", o.ObjectDefinition.GetName(), o.ComplianceType)
		}
	}
	wantYAML(t, nested(find(t, objs, "OAuth", "cluster"), "spec"), `identityProviders:
- name: corp
  mappingMethod: add
  type: LDAP
  ldap:
    url: ldaps://ldap.example.com/ou=users,dc=example,dc=com?uid
    insecure: false
    bindDN: cn=openshift,ou=services,dc=example,dc=com
    bindPassword: {name: oauth-corp}
    attributes: {id: [dn], preferredUsername: [uid], name: [cn], email: [mail]}
- name: local
  mappingMethod: claim
  type: HTPasswd
  htpasswd: {fileData: {name: oauth-local}}
- name: sso
  mappingMethod: claim
  type: OpenID
  openID:
    clientID: openshift
    clientSecret: {name: oauth-sso}
    issuer: https://sso.example.com/realms/openshift
    claims: {preferredUsername: [preferred_username], name: [name], email: [email], groups: [groups]}
    ca: {name: oauth-sso-ca}
`)
	for _, name := range []string{"oauth-corp", "oauth-local", "oauth-sso"} {
		s := find(t, objs, "Secret", name)
		if field(s, "metadata", "namespace") != "openshift-config" || field(s, "type") != "Opaque" {
			t.Errorf("Secret %s %+v, want an Opaque Secret in openshift-config", name, s)
		}
	}
	if got := field(find(t, objs, "ConfigMap", "oauth-sso-ca"), "data", "ca.crt"); got != "-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----\n" {
		t.Errorf("ca.crt %q, want the PEM bundle alone", got)
	}
	wantYAML(t, nested(find(t, objs, "Group", "oauth-cluster-admins"), "users"), "[alice]")
	crb := find(t, objs, "ClusterRoleBinding", "oauth-cluster-admins")
	wantYAML(t, nested(crb, "roleRef"), "{apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: cluster-admin}")
	wantYAML(t, nested(crb, "subjects"), "[{apiGroup: rbac.authorization.k8s.io, kind: Group, name: oauth-cluster-admins}]")
	if got := oauth(t, "{}", "config-oauth", hostedGuest); len(got) != 0 {
		t.Errorf("hosted guest: %v, want every object skipped", names(got))
	}
}

// TestOAuthOptional: token settings render only when set, no clusterAdmins writes no Group or
// binding, and enabled: false drops a provider with its Secret.
func TestOAuthOptional(t *testing.T) {
	objs := oauth(t, `{clusterAdmins: [], identityProviders: {sso: {enabled: false}, corp: {enabled: false}}}`, "config-oauth")
	if got, want := names(objs), []string{"OAuth/cluster", "Secret/oauth-local"}; !slices.Equal(got, want) {
		t.Fatalf("objects %v, want %v", got, want)
	}
	if v := nested(find(t, objs, "OAuth", "cluster"), "spec", "tokenConfig"); v != nil {
		t.Errorf("tokenConfig %v, want none", v)
	}
	o := find(t, oauth(t, `{accessTokenMaxAgeSeconds: 28800, accessTokenInactivityTimeout: 1h30m}`, "config-oauth"), "OAuth", "cluster")
	wantYAML(t, nested(o, "spec", "tokenConfig"), "{accessTokenMaxAgeSeconds: 28800, accessTokenInactivityTimeout: 1h30m}")
	o = find(t, oauth(t, `{accessTokenInactivityTimeout: 300s}`, "config-oauth"), "OAuth", "cluster")
	wantYAML(t, nested(o, "spec", "tokenConfig"), "{accessTokenInactivityTimeout: 300s}")
}

// TestOAuthQuoting: a CA bundle with trailing whitespace or CRLF, a provider name YAML would read
// as a number and a user name with an apostrophe all reach the cluster as written.
func TestOAuthQuoting(t *testing.T) {
	objs := oauth(t, `{identityProviders: {"123": {type: HTPasswd, secret: s}, sso: {ca: "-----BEGIN CERTIFICATE-----  \r\nMIIB\t\r\n  \r\n-----END CERTIFICATE-----\n"}}, clusterAdmins: ["o'brien"]}`, "config-oauth")
	if got := field(find(t, objs, "ConfigMap", "oauth-sso-ca"), "data", "ca.crt"); got != "-----BEGIN CERTIFICATE-----\nMIIB\n\n-----END CERTIFICATE-----\n" {
		t.Errorf("ca.crt %q, want trailing whitespace removed", got)
	}
	idps, _ := nested(find(t, objs, "OAuth", "cluster"), "spec", "identityProviders").([]any)
	if name, _ := idps[0].(map[string]any)["name"].(string); name != "123" {
		t.Errorf("first provider name %#v, want the string 123", idps[0].(map[string]any)["name"])
	}
	wantYAML(t, nested(find(t, objs, "Group", "oauth-cluster-admins"), "users"), `["o'brien"]`)
}

// TestOAuthReady: the authentication ClusterOperator must be Available and not Degraded, except on a
// hosted guest.
func TestOAuthReady(t *testing.T) {
	co := find(t, oauth(t, "{}", "config-oauth-ready"), "ClusterOperator", "authentication")
	wantYAML(t, nested(co, "status", "conditions"), "[{type: Available, status: 'True'}, {type: Degraded, status: 'False'}]")
	if got := oauth(t, "{}", "config-oauth-ready", hostedGuest); len(got) != 0 {
		t.Errorf("hosted guest: %v, want nothing", names(got))
	}
}

// TestOAuthRejected: invalid config fails the render.
func TestOAuthRejected(t *testing.T) {
	runner := render.New(testenv.Helm(t))
	if err := runner.EnsureDeps(context.Background(), oauthDir); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, config, want string }{
		{"all disabled", `{identityProviders: {local: {enabled: false}, sso: {enabled: false}, corp: {enabled: false}}}`, "needs at least one enabled provider"},
		{"name", `{identityProviders: {Local_1: {type: HTPasswd, secret: s}}}`, "identityProviders.Local_1: the name must be"},
		{"not a map", `{identityProviders: {x: htpasswd}}`, "identityProviders.x must be a map"},
		{"null", `{identityProviders: {local: null}}`, "identityProviders.local must be a map. To remove a provider, set enabled: false"},
		{"type", `{identityProviders: {x: {type: GitHub}}}`, `x.type: "GitHub" must be one of HTPasswd, OpenID, LDAP`},
		{"no type", `{identityProviders: {x: {secret: s}}}`, "x.type is required"},
		{"foreign key", `{identityProviders: {local: {url: "ldap://x"}}}`, "local.url is not a HTPasswd setting"},
		{"mapping", `{identityProviders: {local: {mappingMethod: generate}}}`, `local.mappingMethod: "generate" must be one of claim, lookup, add`},
		{"htpasswd secret", `{identityProviders: {x: {type: HTPasswd}}}`, "x.secret is required: a hub Secret with key htpasswd"},
		{"secret type", `{identityProviders: {local: {secret: [a]}}}`, "local.secret must be a Secret name"},
		{"oidc secret", `{identityProviders: {x: {type: OpenID, clientID: c, issuer: "https://i"}}}`, "x.secret is required: a hub Secret with key clientSecret"},
		{"oidc client", `{identityProviders: {sso: {clientID: null}}}`, "sso.clientID is required"},
		{"oidc client number", `{identityProviders: {sso: {clientID: 1234567}}}`, "sso.clientID must be a string; quote it"},
		{"oidc issuer", `{identityProviders: {sso: {issuer: "http://i"}}}`, "sso.issuer must start with https://"},
		{"ca", `{identityProviders: {sso: {ca: "not pem"}}}`, "sso.ca must be a PEM CA bundle"},
		{"ca bom", `{identityProviders: {sso: {ca: "\ufeff-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----"}}}`, "sso.ca must be a PEM CA bundle, printable ASCII only"},
		{"issuer slash", `{identityProviders: {sso: {issuer: "https://i/realms/x/"}}}`, "sso.issuer must start with https:// and have no query, fragment or trailing /"},
		{"issuer query", `{identityProviders: {sso: {issuer: "https://i?x=1"}}}`, "sso.issuer must start with https://"},
		{"claims key", `{identityProviders: {sso: {claims: {username: [sub]}}}}`, "sso.claims.username is not one of preferredUsername, name, email, groups"},
		{"claims list", `{identityProviders: {sso: {claims: {email: email}}}}`, "sso.claims.email must be a list"},
		{"scope space", `{identityProviders: {sso: {extraScopes: ["openid profile"]}}}`, "sso.extraScopes: openid profile is not a scope"},
		{"authorize number", `{identityProviders: {sso: {extraAuthorizeParameters: {max_age: 300}}}}`, "sso.extraAuthorizeParameters.max_age must be a string"},
		{"insecure ca", `{identityProviders: {corp: {url: "ldap://l", insecure: true, ca: "-----BEGIN CERTIFICATE-----"}}}`, "corp: insecure connects without TLS, so it takes no ca"},
		{"attribute list", `{identityProviders: {corp: {attributes: {name: cn}}}}`, "corp.attributes.name must be a list"},
		{"ldap url", `{identityProviders: {corp: {url: "https://x"}}}`, "corp.url must start with ldap:// or ldaps://"},
		{"ldaps insecure", `{identityProviders: {corp: {insecure: true}}}`, "corp.insecure: ldaps:// always uses TLS"},
		{"bind pair", `{identityProviders: {corp: {bindDN: null}}}`, "corp: bindDN and secret are set together"},
		{"ldap id", `{identityProviders: {corp: {attributes: {id: []}}}}`, "corp.attributes.id is required"},
		{"admin empty", `{clusterAdmins: [""]}`, "clusterAdmins: every entry must be a user name"},
		{"admin twice", `{clusterAdmins: [alice, alice]}`, "clusterAdmins: alice appears more than once"},
		{"max age negative", `{accessTokenMaxAgeSeconds: -1}`, "accessTokenMaxAgeSeconds must be a whole number of seconds"},
		{"max age fraction", `{accessTokenMaxAgeSeconds: 1.5}`, "accessTokenMaxAgeSeconds must be a whole number of seconds"},
		{"timeout format", `{accessTokenInactivityTimeout: 10 minutes}`, `"10 minutes" must be a duration`},
		{"timeout minimum", `{accessTokenInactivityTimeout: 4m59s}`, `"4m59s" is under the 300s minimum`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tr := runner.Template(context.Background(), oauthDir, "oauth", valuesFile(t, "stack: {oauth: {enabled: true, config: "+tt.config+"}}\n"), nil)
			if tr.Err == nil || !strings.Contains(string(tr.Stderr), tt.want) {
				t.Fatalf("want %q, got err %v: %s", tt.want, tr.Err, tr.Stderr)
			}
		})
	}
}
