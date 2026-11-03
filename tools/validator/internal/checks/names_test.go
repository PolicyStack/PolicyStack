package checks

import (
	"strings"
	"testing"
)

func TestPolicy001_NameLength(t *testing.T) {
	// policy.<name>-my-element-spoke is 63 characters for a 39-character name.
	fits, long, sub := strings.Repeat("f", 39), strings.Repeat("l", 40), strings.Repeat("s", 58)
	values := "stack:\n  myElement:\n    enabled: true\n    policies:\n" +
		"      - name: short\n" + // line 5
		"      - name: " + fits + "\n" +
		"      - name: " + long + "\n" + // line 7
		"    configPolicies:\n" +
		"      - name: " + sub + "\n" + // line 9
		"        policyRef: short\n" +
		"    operatorPolicies:\n" +
		"      " + sub[:51] + ":\n" +
		"        policyRef: short\n" // line 13
	layer := "stack:\n  myElement:\n    policies:\n      - name: short\n      - name: " + long + "x\n" // line 5
	cfg := func(name string) string {
		return template("ConfigurationPolicy", name, "", "          remediationAction: inform\n")
	}
	cases := []struct {
		name, rendered string
		layers         []string
		want           []want
	}{
		{"short names", policyDoc("short", "", cfg("short-x")), nil, nil},
		{"replicated name at the limit", policyDoc(fits, "", cfg("short-x")), nil, nil},
		{"replicated name over the limit", policyDoc(long, "", cfg("short-x")), nil,
			[]want{{SevError, `policies[2] "` + long + `": Policy replicated name "policy.` + long + `-my-element-spoke" is 64 chars > 63`, inValues, 7}}},
		{"template name over the limit", policyDoc("short", "", cfg("short-"+sub)), nil,
			[]want{{SevError, `configPolicies[0] "` + sub + `": ConfigurationPolicy metadata.name "short-` + sub + `" is 64 chars > 63`, inValues, 9}}},
		{"operator status template", policyDoc("short", "", cfg("short-"+sub[:51]+"-status")), nil,
			[]want{{SevError, "operatorPolicies." + sub[:51] + `: ConfigurationPolicy metadata.name "short-` + sub[:51] + `-status" is 64 chars > 63`, inValues, 13}}},
		// regression: a values layer renames a Policy past the limit (audit probe B).
		{"layer rename", policyDoc(long+"x", "", cfg("short-x")), []string{layer},
			[]want{{SevError, `policies[1] "` + long + `x": Policy replicated name`, inLayer, 5}}},
		{"not from values", policyDoc("other-"+long, "", cfg("short-x")), nil,
			[]want{{SevError, `Policy "other-` + long + `-my-element-spoke": Policy replicated name`, nowhere, 0}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := renderCtx(t, loadElement(t, values, "", nil), tc.rendered, tc.layers...)
			got := (&NameLengthCheck{}).Run(ctx)
			assertFindings(t, "POLICY001", ctx, got, tc.want)
			for _, f := range got {
				if f.Cluster != "spoke" {
					t.Errorf("finding %+v has no cluster", f)
				}
			}
		})
	}
}
