package acmtemplates

import (
	"strings"
	"testing"
)

// policyYAML wraps one ConfigurationPolicy spec in a Policy in namespace policy.
func policyYAML(policySpec, cpSpec string) string {
	return `apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: p
  namespace: policy
spec:
` + policySpec + `
  policy-templates:
    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: cp
        spec:
` + cpSpec
}

func objectTemplate(definitions ...string) string {
	out := "          object-templates:\n"
	for _, d := range definitions {
		out += "            - complianceType: musthave\n              objectDefinition:\n" + d
	}
	return out
}

func configMap(data string) string {
	return objectTemplate(configMapDefinition("x", "default", data))
}

func configMapDefinition(name, namespace, data string) string {
	return `                apiVersion: v1
                kind: ConfigMap
                metadata:
                  name: ` + name + `
                  namespace: ` + namespace + `
                data:
                  v: '` + data + `'
`
}

// annotate adds a disable-templates annotation to the Policy or, with cp, the ConfigurationPolicy.
func annotate(policy string, cp bool) string {
	if cp {
		return strings.Replace(policy, "  name: cp\n", "  name: cp\n          annotations:\n            "+disableTemplates+": \"true\"\n", 1)
	}
	return strings.Replace(policy, "  name: p\n", "  name: p\n  annotations:\n    "+disableTemplates+": \"true\"\n", 1)
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name, policy, want string
		hub                bool
		template           int
	}{
		{
			name:   "undefined function in a branch that never runs",
			policy: policyYAML("", configMap(`{{ range (lookup "v1" "Node" "" "").items }}{{ lookpu "v1" "Node" "" .metadata.name }}{{ end }}`)),
			want:   `function "lookpu" not defined`,
		},
		{
			name: "undefined function in raw",
			policy: policyYAML("", `          object-templates-raw: |
            {{- range (lookup "v1" "Node" "" "").items }}
            - complianceType: musthave
              objectDefinition: {{ toYml . }}
            {{- end }}
`),
			want:     `function "toYml" not defined`,
			template: -1,
		},
		{
			name:     "hub lookup outside the policy namespace",
			policy:   policyYAML("", configMap(`{{hub fromSecret "stackrox" "tls" "ca.pem" hub}}`)),
			want:     "restricted to policy",
			hub:      true,
			template: -1,
		},
		{
			name:   "hub lookup outside the policy namespace with a ServiceAccount",
			policy: policyYAML("  hubTemplateOptions:\n    serviceAccountName: sa", configMap(`{{hub fromSecret "stackrox" "tls" "ca.pem" hub}}`)),
		},
		{
			name:   "missing object",
			policy: policyYAML("", configMap(`{{ fromSecret "default" "absent" "key" }}`)),
		},
		{
			name:   "missing field",
			policy: policyYAML("", configMap(`https://{{ (lookup "route.openshift.io/v1" "Route" "default" "r").spec.host }}`)),
			want:   "<no value>",
		},
		{
			name:   "missing field skipped",
			policy: policyYAML("", configMap(`{{ $h := (lookup "route.openshift.io/v1" "Route" "default" "r").spec.host }}{{ skipObject (not $h) }}https://{{ $h }}`)),
		},
		{
			name:   "hub variable in a managed template",
			policy: policyYAML("", configMap(`{{ .ManagedClusterName }}`)),
			want:   "can't evaluate field ManagedClusterName",
		},
		{
			name:   "hub variable in a hub template",
			policy: policyYAML("", configMap(`{{hub .ManagedClusterName hub}}`)),
		},
		{
			name:   "index on a missing object",
			policy: policyYAML("", configMap(`{{ index (lookup "v1" "ConfigMap" "default" "absent").data "k" }}`)),
			want:   "index of untyped nil",
		},
		{
			name:   "unknown kind",
			policy: policyYAML("", configMap(`{{ (lookup "v1" "Pdo" "default" "x").kind }}`)),
			want:   "not installed",
		},
		{
			name: "raw that is not a list of object templates",
			policy: policyYAML("", `          object-templates-raw: |
            {{- if true }}
            complianceType: musthave
            {{- end }}
`),
			want:     "other than a list of object templates",
			template: -1,
		},
		{
			name: "raw object without kind",
			policy: policyYAML("", `          object-templates-raw: |
            - complianceType: musthave
              objectDefinition:
                apiVersion: v1
                metadata:
                  name: {{ "x" }}
`),
			want:     "without apiVersion or kind",
			template: -1,
		},
		{
			name:   "templates disabled",
			policy: annotate(policyYAML("", configMap(`{{ lookpu }}`)), true),
		},
		{
			name:   "templates disabled on the Policy",
			policy: annotate(policyYAML("", configMap(`{{hub lookpu hub}}`)), false),
		},
		{
			name:     "hub templates still run when the ConfigurationPolicy disables templates",
			policy:   annotate(policyYAML("", configMap(`{{hub lookpu hub}}`)), true),
			want:     `function "lookpu" not defined`,
			hub:      true,
			template: -1,
		},
		{
			name:   "managed templates still checked after a missing hub object",
			policy: policyYAML("", configMap(`{{hub fromSecret "policy" "absent" "k" hub}}{{ lookpu }}`)),
			want:   `function "lookpu" not defined`,
		},
		{
			name:   "skipObject before a failing call",
			policy: policyYAML("", configMap(`{{ $cm := lookup "v1" "ConfigMap" "default" "src" }}{{ skipObject (not $cm) }}{{ index $cm.data "k" }}`)),
		},
		{
			name:   "skipObject with a non-boolean",
			policy: policyYAML("", configMap(`{{ skipObject "yes" }}`)),
			want:   "expected boolean",
		},
		{
			name:   "skipObject with two arguments",
			policy: policyYAML("", configMap(`{{ skipObject true false }}`)),
			want:   "expected one optional boolean",
		},
		{
			name:     "failure in the second object template",
			policy:   policyYAML("", objectTemplate(configMapDefinition("a", "default", "ok"), configMapDefinition("b", "default", `{{ lookpu }}`))),
			want:     `function "lookpu" not defined`,
			template: 1,
		},
		{
			name:   "object context for a namespaced object",
			policy: policyYAML("", configMap(`{{ .ObjectName }}{{ .ObjectNamespace }}{{ .Object }}`)),
		},
		{
			name:   "no Object for an unnamed namespaced object",
			policy: policyYAML("", objectTemplate(configMapDefinition(`'{{ "x" }}'`, "default", `{{ .Object }}`))),
			want:   "can't evaluate field Object",
		},
		{
			name: "no ObjectNamespace for a cluster-scoped object",
			policy: policyYAML("", objectTemplate(`                apiVersion: v1
                kind: Namespace
                metadata:
                  name: foo
                  labels:
                    v: 'x{{ .ObjectNamespace }}'
`)),
			want: "can't evaluate field ObjectNamespace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate([]byte(tt.policy), "spoke")
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if len(got) > 0 {
					t.Fatalf("want no failure, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want one failure containing %q, got %+v", tt.want, got)
			}
			f := got[0]
			if !strings.Contains(f.Err, tt.want) || f.ConfigPolicy != "cp" || f.Hub != tt.hub || f.Template != tt.template {
				t.Fatalf("want %q (hub %v, template %d), got %+v", tt.want, tt.hub, tt.template, f)
			}
		})
	}
}
