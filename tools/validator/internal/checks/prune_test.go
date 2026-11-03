package checks

import "testing"

func TestPolicy033_Prune(t *testing.T) {
	const values = "stack:\n  myElement:\n    enabled: true\n    policies:\n      - name: a\n    configPolicies:\n" +
		"      - name: x\n        policyRef: a\n        pruneObjectBehavior: DeleteIfCreated\n" // line 9
	const (
		enforce = "          remediationAction: enforce\n"
		inform  = "          remediationAction: inform\n"
		deps    = "extraDependencies:\n        - {kind: ConfigurationPolicy, name: w}\n      "
	)
	prune := func(v string) string { return "          pruneObjectBehavior: " + v + "\n" }
	x := func(header, spec string) string { return template("ConfigurationPolicy", "a-x", header, spec) }
	const dryRunLayer = "global:\n  remediationAction: inform\n"
	cases := []struct {
		name, rendered string
		layers         []string
		want           []want
	}{
		{"enforce without dependencies", policyDoc("a", "", x("", enforce+prune("DeleteIfCreated"))), nil, nil},
		{"None", policyDoc("a", "  dependencies:\n    - {kind: Policy, name: b}\n", x(deps, inform+prune("None"))), nil, nil},
		{"unset", policyDoc("a", "", x(deps, inform)), nil, nil},
		{"invalid value", policyDoc("a", "", x("", enforce+prune("deleteIfCreated"))), nil,
			[]want{{SevError, `configPolicies[0] "x": pruneObjectBehavior = "deleteIfCreated"; allowed: [None DeleteIfCreated DeleteAll]`, inValues, 9}}},
		{"DeleteAll", policyDoc("a", "", x("", enforce+prune("DeleteAll"))), nil,
			[]want{{SevWarning, `configPolicies[0] "x": pruneObjectBehavior DeleteAll also deletes objects that existed before`, inValues, 9}}},

		{"template inform", policyDoc("a", "", x("", inform+prune("DeleteIfCreated"))), nil,
			[]want{{SevError, `configPolicies[0] "x": pruneObjectBehavior under remediationAction inform: ACM prunes only under enforce`, inValues, 9}}},
		{"policy inform overrides template enforce", policyDoc("a", "  remediationAction: inform\n", x("", enforce+prune("DeleteIfCreated"))), nil,
			[]want{{SevError, `configPolicies[0] "x": pruneObjectBehavior under remediationAction inform`, inValues, 9}}},
		{"policy enforce overrides template inform", policyDoc("a", "  remediationAction: enforce\n", x("", inform+prune("DeleteIfCreated"))), nil, nil},
		{"policy enforce keeps template InformOnly", policyDoc("a", "  remediationAction: enforce\n", x("", "          remediationAction: InformOnly\n"+prune("DeleteIfCreated"))), nil,
			[]want{{SevError, `configPolicies[0] "x": pruneObjectBehavior under remediationAction InformOnly`, inValues, 9}}},
		{"dry run", policyDoc("a", "  remediationAction: inform\n", x("", enforce+prune("DeleteIfCreated"))), []string{dryRunLayer}, nil},

		{"template dependency", policyDoc("a", "", x(deps, enforce+prune("DeleteIfCreated"))), nil,
			[]want{{SevError, `configPolicies[0] "x": pruneObjectBehavior with a dependency (w): ACM deletes this template whenever the dependency is unmet`, inValues, 9}}},
		{"policy dependency", policyDoc("a", "  dependencies:\n    - {kind: Policy, name: b-my-element-spoke}\n", x("", enforce+prune("DeleteIfCreated"))), nil,
			[]want{{SevError, `configPolicies[0] "x": pruneObjectBehavior with a dependency (the Policy's dependencies)`, inValues, 9}}},
		{"both dependencies", policyDoc("a", "  dependencies:\n    - {kind: Policy, name: b-my-element-spoke}\n", x(deps, enforce+prune("DeleteIfCreated"))), nil,
			[]want{{SevError, `configPolicies[0] "x": pruneObjectBehavior with a dependency (w, the Policy's dependencies)`, inValues, 9}}},
		{"not a ConfigurationPolicy", policyDoc("a", "", template("CertificatePolicy", "a-x", "", inform+prune("DeleteIfCreated"))), nil, nil},
		{"layer sets prune", policyDoc("a", "", x("", inform+prune("DeleteIfCreated"))),
			[]string{"stack:\n  myElement:\n    configPolicies:\n      - name: x\n        policyRef: a\n        pruneObjectBehavior: DeleteIfCreated\n"},
			[]want{{SevError, `configPolicies[0] "x": pruneObjectBehavior under remediationAction inform`, inLayer, 6}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := renderCtx(t, loadElement(t, values, "", nil), tc.rendered, tc.layers...)
			assertFindings(t, "POLICY033", ctx, (&PruneCheck{}).Run(ctx), tc.want)
		})
	}
}
