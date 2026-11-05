package checks

import (
	"strings"
	"testing"
)

func TestPolicy037_ProtectedObjects(t *testing.T) {
	const values = "stack:\n  myElement:\n    enabled: true\n    policies:\n      a:\n        templates:\n" +
		"          x: {}\n" + // line 7
		"    operatorPolicies:\n      op: {}\n" // line 9
	// objects renders an object-templates spec under action.
	objects := func(action string, ots ...string) string {
		return "          remediationAction: " + action + "\n          object-templates:\n            - " + strings.Join(ots, "\n            - ") + "\n"
	}
	ot := func(ct, apiVersion, kind, ns, name string) string {
		return "{complianceType: " + ct + ", objectDefinition: {apiVersion: " + apiVersion + ", kind: " + kind +
			", metadata: {namespace: '" + ns + "', name: '" + name + "'}}}"
	}
	x := func(spec string) string { return policyDoc("a", "", template("ConfigurationPolicy", "a-x", "", spec)) }
	const msg = `policies.a.templates.x: `
	cases := []struct {
		name, rendered string
		want           []want
	}{
		// regression: 9ddc38f, mustonlyhave on the installer-owned user-ca-bundle.
		{"user-ca-bundle mustonlyhave", x(objects("enforce", ot("mustonlyhave", "v1", "ConfigMap", "openshift-config", "user-ca-bundle"))),
			[]want{{SevError, msg + "mustonlyhave ConfigMap openshift-config/user-ca-bundle, which OpenShift owns: enforced, it replaces or deletes platform state. Use musthave, or InformOnly for a gate", inValues, 7}}},
		// regression: dd0df35, mustnothave on an operator's namespace, which other operators share.
		{"operator namespace mustnothave", policyDoc("install", "  remediationAction: enforce\n",
			template("ConfigurationPolicy", "install-op-ns", "", objects("inform", ot("mustnothave", "v1", "Namespace", "", "openshift-gitops-operator")))),
			[]want{{SevError, "operatorPolicies.op: mustnothave Namespace openshift-gitops-operator, which OpenShift owns", inValues, 9}}},
		{"inform counts", x(objects("inform", ot("MustNotHave", "config.openshift.io/v1", "ClusterVersion", "", "version"))),
			[]want{{SevError, msg + "MustNotHave ClusterVersion version", inValues, 7}}},
		{"every protected object", x(objects("enforce",
			ot("mustonlyhave", "v1", "Secret", "openshift-config", "pull-secret"),
			ot("mustonlyhave", "config.openshift.io/v1", "Proxy", "", "cluster"),
			ot("mustnothave", "config.openshift.io/v1", "Image", "", "cluster"),
			ot("mustnothave", "v1", "Namespace", "", "kube-system"))),
			[]want{{SevError, msg + "mustonlyhave Secret openshift-config/pull-secret", inValues, 7},
				{SevError, msg + "mustonlyhave Proxy cluster", inValues, 7},
				{SevError, msg + "mustnothave Image cluster", inValues, 7},
				{SevError, msg + "mustnothave Namespace kube-system", inValues, 7}}},
		{"unprotected objects", x(objects("enforce",
			ot("mustonlyhave", "v1", "ConfigMap", "openshift-config", "trusted-cas"),
			ot("mustonlyhave", "v1", "Secret", "my-app", "pull-secret"),
			ot("mustnothave", "image.openshift.io/v1", "Image", "", "cluster"),
			ot("mustnothave", "v1", "Namespace", "", "my-app"),
			ot("mustnothave", "config.openshift.io/v1", "Infrastructure", "", "cluster"))), nil},
		{"musthave", x(objects("enforce", ot("musthave", "v1", "ConfigMap", "openshift-config", "user-ca-bundle"))), nil},
		{"InformOnly", policyDoc("a", "  remediationAction: enforce\n",
			template("ConfigurationPolicy", "a-x", "", objects("InformOnly", ot("mustnothave", "config.openshift.io/v1", "ClusterVersion", "", "version")))), nil},
		{"annotated", policyDoc("a", "", "    - objectDefinition:\n        kind: ConfigurationPolicy\n        metadata:\n          name: a-x\n"+
			"          annotations: {validator.policystk.io/allow-destructive: owns the bundle}\n        spec:\n"+
			objects("enforce", ot("mustonlyhave", "v1", "ConfigMap", "openshift-config", "user-ca-bundle"))), nil},
		{"templated raw, every branch", x("          remediationAction: enforce\n          object-templates-raw: |\n" +
			"            {{- if eq (lookup \"v1\" \"Namespace\" \"\" \"x\").metadata.name \"x\" }}\n" +
			"            - complianceType: mustnothave\n              objectDefinition:\n                apiVersion: v1\n                kind: Namespace\n" +
			"                metadata:\n                  name: openshift-{{ .x }}\n            {{- else }}\n" +
			"            - complianceType: musthave\n              objectDefinition: {apiVersion: v1, kind: Namespace, metadata: {name: a}}\n" +
			"            {{- end }}\n"),
			[]want{{SevError, msg + "mustnothave Namespace openshift-, which OpenShift owns", inValues, 7}}},
		{"templated name", x(objects("enforce", ot("mustnothave", "v1", "Namespace", "", "{{ .x }}"))), nil},
		{"raw that is not YAML", x("          object-templates-raw: |\n            - complianceType: [\n"),
			[]want{{SevWarning, msg + "object-templates-raw is not YAML once its template actions are removed, so it is not checked", inValues, 7}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := renderCtx(t, loadElement(t, values, "", nil), tc.rendered)
			assertFindings(t, "POLICY037", ctx, (&ProtectedObjectCheck{}).Run(ctx), tc.want)
		})
	}
}
