package checks

import (
	"strings"
	"testing"
)

func TestPolicy033_Prune(t *testing.T) {
	const (
		prune   = "        pruneObjectBehavior: DeleteIfCreated\n"
		enforce = "        remediationAction: enforce\n"
	)
	policy := func(name, extra string) string {
		return "      - name: " + name + "\n        enabled: true\n" + extra
	}
	entry := func(name, ref, extra string) string {
		return "      - name: " + name + "\n        enabled: true\n        policyRef: " + ref + "\n" + extra
	}
	type want struct {
		sev Severity
		msg string
	}
	depErr := func(src string) []want {
		return []want{{SevError, "pruneObjectBehavior with a dependency (" + src + "): ACM deletes this template"}}
	}
	cases := []struct {
		name, body string
		want       []want
	}{
		{"DeleteIfCreated enforce no dependencies", "    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("x", "a", enforce+prune), nil},
		{"None", "    orderManifests: true\n    policies:\n" + policy("a", "        dependencies:\n          - name: b\n") +
			"    configPolicies:\n" + entry("w", "a", "") + entry("x", "a", "        pruneObjectBehavior: None\n"), nil},
		{"unset", "    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("x", "a", "        waitForOperator: op\n"), nil},
		{"invalid value", "    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("x", "a", enforce+"        pruneObjectBehavior: deleteIfCreated\n"),
			[]want{{SevError, `pruneObjectBehavior = "deleteIfCreated"; allowed: [None DeleteIfCreated DeleteAll]`}}},
		{"DeleteAll", "    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("x", "a", enforce+"        pruneObjectBehavior: DeleteAll\n"),
			[]want{{SevWarning, "pruneObjectBehavior DeleteAll also deletes objects that existed before"}}},

		{"template inform", "    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("x", "a", "        remediationAction: inform\n"+prune),
			[]want{{SevError, "pruneObjectBehavior with remediationAction inform: ACM prunes only under enforce"}}},
		{"template default inform", "    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("x", "a", prune),
			[]want{{SevError, "pruneObjectBehavior with remediationAction inform"}}},
		{"policy inform overrides template enforce", "    policies:\n" + policy("a", "        remediationAction: inform\n") +
			"    configPolicies:\n" + entry("x", "a", enforce+prune),
			[]want{{SevError, "pruneObjectBehavior with policies[0].remediationAction inform"}}},
		{"policy enforce overrides template inform", "    policies:\n" + policy("a", enforce) +
			"    configPolicies:\n" + entry("x", "a", "        remediationAction: inform\n"+prune), nil},

		{"waitForOperator", "    policies:\n" + policy("a", "") +
			"    operatorPolicies:\n      op:\n        enabled: true\n        policyRef: a\n" +
			"    configPolicies:\n" + entry("x", "a", enforce+prune+"        waitForOperator: op\n"), depErr("waitForOperator")},
		{"extraDependencies", "    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("w", "a", "") + entry("x", "a", enforce+prune+"        extraDependencies:\n          - name: w\n"),
			depErr("extraDependencies")},
		{"policy dependencies", "    policies:\n" + policy("b", "") + policy("a", "        dependencies:\n          - name: b\n") +
			"    configPolicies:\n" + entry("w", "b", "") + entry("x", "a", enforce+prune), depErr("policies[1].dependencies")},
		{"all sources", "    orderPolicies: true\n    orderManifests: true\n    policies:\n" + policy("b", "") + policy("a", "        dependencies:\n          - name: b\n") +
			"    configPolicies:\n" + entry("w", "b", "") + entry("v", "a", "") +
			entry("x", "a", enforce+prune+"        waitForOperator: op\n        extraDependencies:\n          - name: v\n"),
			depErr("waitForOperator, extraDependencies, policies[1].dependencies, orderManifests, orderPolicies")},

		{"orderManifests first template", "    orderManifests: true\n    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("x", "a", enforce+prune) + entry("y", "a", ""), nil},
		{"orderManifests second template", "    orderManifests: true\n    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("w", "a", "") + entry("x", "a", enforce+prune), depErr("orderManifests")},
		{"orderManifests first template of its policy", "    orderManifests: true\n    policies:\n" + policy("b", "") + policy("a", "") +
			"    configPolicies:\n" + entry("w", "b", "") + entry("x", "a", enforce+prune), nil},
		{"orderManifests after a disabled template", "    orderManifests: true\n    policies:\n" + policy("a", "") +
			"    configPolicies:\n      - name: w\n        enabled: false\n        policyRef: a\n" + entry("x", "a", enforce+prune), nil},
		{"orderManifests after a toggled-off template", "    orderManifests: true\n    toggles:\n      w: false\n    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("w", "a", "") + entry("x", "a", enforce+prune), depErr("orderManifests")},
		{"policy orderManifests overrides component", "    policies:\n" + policy("a", "        orderManifests: true\n") +
			"    configPolicies:\n" + entry("w", "a", "") + entry("x", "a", enforce+prune), depErr("orderManifests")},
		{"policy orderManifests false overrides component", "    orderManifests: true\n    policies:\n" + policy("a", "        orderManifests: false\n") +
			"    configPolicies:\n" + entry("w", "a", "") + entry("x", "a", enforce+prune), nil},

		{"orderPolicies first policy", "    orderPolicies: true\n    policies:\n" + policy("a", "") + policy("b", "") +
			"    configPolicies:\n" + entry("x", "a", enforce+prune) + entry("w", "b", ""), nil},
		{"orderPolicies second policy", "    orderPolicies: true\n    policies:\n" + policy("b", "") + policy("a", "") +
			"    configPolicies:\n" + entry("w", "b", "") + entry("x", "a", enforce+prune), depErr("orderPolicies")},
		{"orderPolicies after a policy that does not render", "    orderPolicies: true\n    policies:\n" + policy("b", "") + policy("a", "") +
			"    configPolicies:\n" + entry("x", "a", enforce+prune), nil},
		{"orderPolicies after a disabled policy", "    orderPolicies: true\n    policies:\n" + "      - name: b\n        enabled: false\n" + policy("a", "") +
			"    configPolicies:\n" + entry("w", "b", "") + entry("x", "a", enforce+prune), nil},
		{"orderPolicies after a toggled-off policy", "    orderPolicies: true\n    toggles:\n      b: false\n    policies:\n" + policy("b", "") + policy("a", "") +
			"    configPolicies:\n" + entry("w", "b", "") + entry("x", "a", enforce+prune), depErr("orderPolicies")},
		{"orderPolicies after a policy whose templates are toggled off", "    orderPolicies: true\n    toggles:\n      w: false\n    policies:\n" + policy("b", "") + policy("a", "") +
			"    configPolicies:\n" + entry("w", "b", "") + entry("x", "a", enforce+prune), depErr("orderPolicies")},

		{"disabled entry", "    policies:\n" + policy("a", "") +
			"    configPolicies:\n      - name: x\n        enabled: false\n        policyRef: a\n" + prune, nil},
		{"toggled-off entry", "    toggles:\n      x: false\n    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("x", "a", prune),
			[]want{{SevError, "pruneObjectBehavior with remediationAction inform"}}},
		{"toggled-on entry", "    toggles:\n      x: true\n    policies:\n" + policy("a", "") +
			"    configPolicies:\n      - name: x\n        enabled: false\n        policyRef: a\n" + prune,
			[]want{{SevError, "pruneObjectBehavior with remediationAction inform"}}},
		{"parent disabled", "    policies:\n      - name: a\n        enabled: false\n" +
			"    configPolicies:\n" + entry("x", "a", prune), nil},
		{"parent toggled off", "    toggles:\n      a: false\n    policies:\n" + policy("a", "") +
			"    configPolicies:\n" + entry("x", "a", prune),
			[]want{{SevError, "pruneObjectBehavior with remediationAction inform"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := loadElement(t, "stack:\n  myElement:\n    enabled: true\n"+tc.body, "", nil)
			got := (&PruneCheck{}).Run(Context{Element: el})
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d findings, got %d: %+v", len(tc.want), len(got), got)
			}
			for i, w := range tc.want {
				f := got[i]
				if f.RuleID != "POLICY033" || f.Severity != w.sev || !strings.Contains(f.Message, w.msg) {
					t.Errorf("finding %d = %+v, want %s containing %q", i, f, w.sev, w.msg)
				}
				if !strings.HasPrefix(f.Message, `configPolicies[`) || f.File != el.ValuesFile || f.Line == 0 {
					t.Errorf("finding %d location/prefix wrong: %+v", i, f)
				}
			}
		})
	}
}
