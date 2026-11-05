package checks

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicy001_NameLength(t *testing.T) {
	// policy.<name>-my-element-spoke is 63 characters for a 39-character name.
	fits, long, sub := strings.Repeat("f", 39), strings.Repeat("l", 40), strings.Repeat("s", 58)
	op := sub[:49] // install-<op>-status is 64 characters
	values := "stack:\n  myElement:\n    enabled: true\n    policies:\n" +
		"      short:\n        templates:\n" +
		"          " + sub + ": {}\n" + // line 7
		"      " + fits + ": {}\n" +
		"      " + long + ": {}\n" + // line 9
		"    operatorPolicies:\n" +
		"      " + op + ": {}\n" // line 11
	layer := "stack:\n  myElement:\n    policies:\n      " + long + "x: {}\n" // line 4
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
			[]want{{SevError, `policies.` + long + `: Policy replicated name "policy.` + long + `-my-element-spoke" is 64 chars > 63`, inValues, 9}}},
		{"template name over the limit", policyDoc("short", "", cfg("short-"+sub)), nil,
			[]want{{SevError, `policies.short.templates.` + sub + `: ConfigurationPolicy metadata.name "short-` + sub + `" is 64 chars > 63`, inValues, 7}}},
		{"operator status template", policyDoc("short", "", cfg("install-"+op+"-status")), nil,
			[]want{{SevError, "operatorPolicies." + op + `: ConfigurationPolicy metadata.name "install-` + op + `-status" is 64 chars > 63`, inValues, 11}}},
		// regression: a values layer renames a Policy past the limit (audit probe B).
		{"layer rename", policyDoc(long+"x", "", cfg("short-x")), []string{layer},
			[]want{{SevError, `policies.` + long + `x: Policy replicated name`, inLayer, 4}}},
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

func TestPolicy002_DuplicateName(t *testing.T) {
	const prefix = "stack:\n  myElement:\n    policies:\n      a:\n        templates:\n          b-c: {}\n"
	cases := []struct {
		name, values string
		converters   []string
		file         string // relative to the element directory
		line         int
	}{
		{"both in values", prefix + "      a-b:\n        templates:\n          c: {}\n", nil, "values.yaml", 9},
		{"discovered template", prefix, []string{"a-b/c.yaml"}, "converters/a-b/c.yaml", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := loadElement(t, tc.values, "", tc.converters)
			got := (&DuplicateNameCheck{}).Run(renderCtx(t, el, ""))
			want := `rendered name "a-b-c" from policies.a-b.templates.c collides with prior policies.a.templates.b-c in same element`
			if len(got) != 1 || got[0].Message != want || got[0].File != filepath.Join(el.Dir, tc.file) || got[0].Line != tc.line {
				t.Errorf("got %+v, want %s at %s:%d", got, want, tc.file, tc.line)
			}
		})
	}
}
