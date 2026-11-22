package checks

import "testing"

func TestPolicy042_Gates(t *testing.T) {
	const values = "stack:\n  myElement:\n    enabled: true\n    policies:\n      a:\n        templates:\n" +
		"          gate:\n            remediationAction: InformOnly\n" + // lines 7, 8
		"          write: {}\n"
	const layer = "stack:\n  myElement:\n    policies:\n      a:\n        templates:\n          gate:\n            remediationAction: null\n" // line 7
	render := func(name, action string) string {
		return policyDoc("a", "  remediationAction: enforce\n", template("ConfigurationPolicy", name, "", "          remediationAction: "+action+"\n"))
	}
	const msg = `policies.a.templates.gate: the chart ships this gate InformOnly and it renders `
	for _, tc := range []struct {
		name, rendered string
		layers         []string
		want           []want
	}{
		{name: "gate kept", rendered: render("a-gate", "InformOnly")},
		{name: "any case", rendered: render("a-gate", "informonly")},
		{name: "dry run keeps it", rendered: policyDoc("a", "  remediationAction: inform\n", template("ConfigurationPolicy", "a-gate", "", "          remediationAction: InformOnly\n"))},
		{name: "not a gate", rendered: render("a-write", "enforce")},
		{name: "same name on another kind", rendered: policyDoc("a", "", template("OperatorPolicy", "a-gate", "", "          remediationAction: enforce\n"))},
		{name: "gate turned off", rendered: render("a-write", "enforce"), layers: []string{"stack: {myElement: {policies: {a: {templates: {gate: {enabled: false}}}}}}\n"}},
		{
			name: "cleared by a layer", rendered: render("a-gate", "enforce"), layers: []string{layer},
			want: []want{{SevError, msg + `"enforce" instead`, inLayer, 7}},
		},
		{
			name: "cleared under inform", rendered: render("a-gate", "inform"), layers: []string{layer},
			want: []want{{SevError, msg + `"inform" instead`, inLayer, 7}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := renderCtx(t, loadElement(t, values, "", nil), tc.rendered, tc.layers...)
			assertFindings(t, "POLICY042", ctx, (&GateCheck{}).Run(ctx), tc.want)
		})
	}

	t.Run("suffixed name", func(t *testing.T) {
		ctx := renderCtx(t, loadElement(t, values, "", nil), render("a-gate-spoke", "enforce"))
		if got := (&GateCheck{}).Run(ctx); len(got) != 1 || got[0].RuleID != "POLICY042" || got[0].Severity != SevError {
			t.Errorf("got %+v, want one POLICY042 error", got)
		}
	})
}
