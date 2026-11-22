package checks

import "testing"

func TestPolicy041_Placement(t *testing.T) {
	const values = "stack:\n  myElement:\n    enabled: true\n"
	const cluster = "  - requiredClusterSelector: {labelSelector: {matchExpressions: [{key: name, operator: In, values: [spoke]}]}}\n"
	placement := func(predicates string) string {
		return "---\napiVersion: cluster.open-cluster-management.io/v1beta1\nkind: Placement\nmetadata: {name: my-element-spoke}\nspec:\n  predicates:\n" + predicates
	}
	const extra = "  - requiredClusterSelector: {}\n"
	for _, tc := range []struct {
		name, rendered string
		layers         []string
		want           []want
	}{
		{name: "cluster selector only", rendered: placement(cluster)},
		{name: "predicates set empty", rendered: placement(cluster), layers: []string{"placement:\n  predicates: []\n"}},
		{name: "claimSelector narrows", rendered: placement("  - requiredClusterSelector: {claimSelector: {}, labelSelector: {}}\n")},
		{name: "not a Placement", rendered: "---\nkind: ConfigMap\nspec: {predicates: [1, 2]}\n"},
		{name: "no Placement", rendered: policyDoc("a", "", "")},
		{
			name: "from a layer", rendered: placement(cluster + extra),
			layers: []string{"placement:\n  predicates:\n    - requiredClusterSelector: {}\n"},
			want:   []want{{SevError, "the Placement renders 2 predicates, which ACM ORs", inLayer, 3}},
		},
		{
			name: "the last layer setting it takes it", rendered: placement(cluster + extra + extra),
			layers: []string{"# base\nplacement:\n\n  predicates: [{}, {}]\n", "placement:\n  tolerations: []\n"},
			want:   []want{{SevError, "the Placement renders 3 predicates", inLayer, 4}},
		},
		{
			name: "a template's own Placement has no values origin", rendered: placement(cluster + extra),
			want: []want{{SevError, "the Placement renders 2 predicates", nowhere, 0}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := renderCtx(t, loadElement(t, values, "", nil), tc.rendered, tc.layers...)
			got := (&PlacementCheck{}).Run(ctx)
			for i := range got {
				// A layer's finding names no element, so a run reports it once for all that read it.
				if (got[i].Element == "") != (got[i].File != "") {
					t.Errorf("element %q with file %q", got[i].Element, got[i].File)
				}
				got[i].Element = ctx.Element.ChartName
			}
			assertFindings(t, "POLICY041", ctx, got, tc.want)
		})
	}
}
