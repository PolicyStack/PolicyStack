package checks

import "testing"

func TestPolicy043_DefaultChanges(t *testing.T) {
	const values = `stack:
  myElement:
    enabled: true
    config:
      # Taints. Default changes to the ODF taint in 2.0.0.
      taints: []
      # Not changing
      size: small
    policies:
      a:
        templates:
          exact:
            # Also revokes.
            # Default changes to true in 2.0.0.
            enabled: false
`
	const (
		taints = "the default of stack.myElement.config.taints changes to the ODF taint in 2.0.0: set it in a values layer"
		exact  = "the default of stack.myElement.policies.a.templates.exact.enabled changes to true in 2.0.0"
	)
	for _, tc := range []struct {
		name, rendered string
		layers         []string
		want           []want
	}{
		{name: "neither chosen", rendered: policyDoc("a", "", ""), want: []want{{SevWarning, taints, inValues, 6}, {SevWarning, exact, inValues, 15}}},
		{name: "that Policy does not render", rendered: policyDoc("b", "", ""), want: []want{{SevWarning, taints, inValues, 6}}},
		{name: "element off", rendered: "\n---\n"},
		{
			name: "the new values chosen", rendered: policyDoc("a", "", ""),
			layers: []string{"stack: {myElement: {config: {taints: [{key: odf}]}, policies: {a: {templates: {exact: {enabled: true}}}}}}\n"},
		},
		{
			name: "the old values chosen", rendered: policyDoc("a", "", ""),
			layers: []string{"stack: {myElement: {config: {taints: []}, policies: {a: {templates: {exact: {enabled: false}}}}}}\n"},
		},
		{
			name: "only one chosen", rendered: policyDoc("a", "", ""),
			layers: []string{"stack: {myElement: {config: {taints: null}}}\n"},
			want:   []want{{SevWarning, exact, inValues, 15}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := renderCtx(t, loadElement(t, values, "", nil), tc.rendered, tc.layers...)
			assertFindings(t, "POLICY043", ctx, (&DefaultChangeCheck{}).Run(ctx), tc.want)
		})
	}
}
