package compat

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

func TestCanonical(t *testing.T) {
	const base = `apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: a
  annotations: {description: old}
spec:
  policy-templates:
    - objectDefinition:
        kind: ConfigurationPolicy
        metadata: {name: a-x, annotations: {description: old}}
        spec: {severity: low}
`
	for _, tc := range []struct {
		name, after string
		same        bool
	}{
		{"identical", base, true},
		{"comments and formatting", "# a comment\n---\n" + strings.Replace(base, "spec: {severity: low}", "spec:\n          severity: low # why", 1), true},
		{"descriptions", strings.ReplaceAll(base, "description: old", "description: new"), true},
		{"description added or dropped", strings.ReplaceAll(strings.ReplaceAll(base, ", annotations: {description: old}", ""), "  annotations: {description: old}\n", ""), true},
		{"value", strings.Replace(base, "severity: low", "severity: high", 1), false},
		{"another annotation", strings.Replace(base, "annotations: {description: old}\n", "annotations: {description: old, owner: x}\n", 1), false},
		{"object added", base + "---\nkind: ConfigMap\nmetadata: {name: b}\n", false},
		{"nothing rendered", "\n---\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, errA := Canonical([]byte(base))
			b, errB := Canonical([]byte(tc.after))
			if errA != nil || errB != nil {
				t.Fatal(errA, errB)
			}
			if (string(a) == string(b)) != tc.same {
				t.Errorf("equal %v, want %v\n%s\n%s", !tc.same, tc.same, a, b)
			}
		})
	}
}

func TestDiffering(t *testing.T) {
	ok, other, bad := []byte("kind: A\n"), []byte("kind: B\n"), []byte("# RENDER000: failed\n")
	before := map[string][]byte{"same": ok, "changed": ok, "gone": ok, "breaks": ok, "was broken": bad, "both broken": bad}
	after := map[string][]byte{"same": ok, "changed": other, "new": ok, "breaks": bad, "was broken": ok, "both broken": bad}
	got, err := differing(before, after)
	if want := []string{"breaks", "changed", "gone", "new"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("got %q, %v; want %q", got, err, want)
	}
}

func TestLabel(t *testing.T) {
	report := func(after string) *Report {
		r, err := newReport([]string{"p"}, nil, map[string][]byte{"p": []byte(after)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	a, b := report("kind: A\n"), report("kind: B\n")
	if a.Label == b.Label || len(a.Label) > 50 || a.Label != report("# formatting only\nkind: A\n").Label {
		t.Errorf("labels %q, %q", a.Label, b.Label)
	}
	if x, y := report("# RENDER000: /tmp/one\n"), report("# RENDER000: /tmp/two\n"); x.Label != y.Label {
		t.Error("the label holds a failed render's paths")
	}
	if a.Approve([]string{b.Label, ""}); a.Approved {
		t.Error("another difference's label approved this one")
	}
	if a.Approve([]string{"bug", a.Label}); !a.Approved {
		t.Error("its own label did not approve")
	}
	none := &Report{}
	if none.Approve([]string{""}); none.Approved {
		t.Error("no change needs no approval")
	}
}

// TestDiff renders small trees: output that differs from the release's and the base's is a change,
// and a value's old default keeps it out.
func TestDiff(t *testing.T) {
	helm := testenv.Helm(t)
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	chartYAML, err := os.ReadFile(filepath.Join(repo, "tools/validator/internal/chart/testdata/element/Chart.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	chartDoc := strings.NewReplacer("name: conformance", "name: demo", "file://../../../../../../lib", "file://"+filepath.Join(repo, "lib")).Replace(string(chartYAML))
	tree := func(values, converter string) string {
		dir := t.TempDir()
		for name, body := range map[string]string{
			"stack/demo/Chart.yaml":                 chartDoc,
			"stack/demo/templates/policy.yaml":      `{{- include "policy-library.render" . -}}` + "\n",
			"stack/demo/values.yaml":                "stack:\n  demo:\n    enabled: false\n    config: {" + values + "}\n",
			"stack/demo/converters/a/cm.yaml":       converter,
			"values.yaml":                           "policyNamespace: pstack\n",
			"values/clusters/.keep":                 "",
			testdata + "/baseline.yaml":             "selector:\n  matchExpressions:\n    - {key: name, operator: Exists}\n",
			testdata + "/inputs.yaml":               "{}\n",
			testdata + "/clusters/prod-east-1.yaml": "revision: main\n",
		} {
			p := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	const cm = "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: a, namespace: default}\ndata: {size: "
	release := tree("", cm+"small}\n")
	flipped := tree("size: large", cm+"{{ .Config.size }}}\n")
	optIn := tree("size: small", cm+"{{ .Config.size }}}\n")
	broken := tree("size: small", cm+"{{ fail \"broken\" }}}\n")

	for _, tc := range []struct {
		name, base, head string
		changed          bool
	}{
		{"default flipped", release, flipped, true},
		{"new key defaults to the old output", release, optIn, false},
		{"already on the base", flipped, flipped, false},
		{"restores the release", flipped, optIn, false},
		{"a base that failed approves nothing", broken, flipped, true},
		{"the change breaks the render", release, broken, true},
		{"no base", "", flipped, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Diff(t.Context(), helm, Trees{Release: release, Base: tc.base, Head: tc.head})
			if err != nil {
				t.Fatal(err)
			}
			if (len(r.Changes) > 0) != tc.changed || (r.Label != "") != tc.changed {
				t.Errorf("%d changes, label %q; want changed %v", len(r.Changes), r.Label, tc.changed)
			}
		})
	}
}

// TestTemplatedScenario: the release's templated values come first, so a change can add to them but
// not drop them, and the scenario exists while either side has the file.
func TestTemplatedScenario(t *testing.T) {
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	tree := func(withValues bool) string {
		dir := t.TempDir()
		if err := os.CopyFS(filepath.Join(dir, "stack", "acm"), os.DirFS(filepath.Join(repo, "stack", "acm"))); err != nil {
			t.Fatal(err)
		}
		files := map[string]string{"values.yaml": "policyNamespace: pstack\n"}
		if withValues {
			files[testdata+"/compat/values.yaml"] = "{}\n"
		}
		for name, body := range files {
			p := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	values := func(dir string) string { return filepath.Join(dir, testdata, "compat", "values.yaml") }
	with, without := tree(true), tree(false)
	for _, tc := range []struct {
		name          string
		release, head string
		want          []string
	}{
		{"both", with, with, []string{values(with), values(with)}},
		{"dropped by the change", with, without, []string{values(with)}},
		{"added by the change", without, with, []string{values(with)}},
		{"neither", without, without, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scenarios(t.TempDir(), Trees{Release: tc.release, Head: tc.head})
			if err != nil {
				t.Fatal(err)
			}
			templated, ok := got["templated"]
			if ok != (tc.want != nil) || ok && !slices.Equal(templated[2:], tc.want) {
				t.Errorf("templated %q, want %q after the baseline and on.yaml", templated, tc.want)
			}
		})
	}
}
