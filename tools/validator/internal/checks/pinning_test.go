package checks

import (
	"cmp"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

func TestPolicy060_Pinning(t *testing.T) {
	lib := func(d chart.Dependency) chart.Dependency {
		d.Name = "policy-library"
		if d.Alias == "" {
			d.Alias = "stack"
		}
		if d.Version == "" {
			d.Version = "^4.0.0"
		}
		if d.Repository == "" {
			d.Repository = "file://../../lib/policy-library"
		}
		return d
	}
	tests := []struct {
		name       string
		libVersion string // lib/policy-library/Chart.yaml version; empty: no lib/
		dir        string // element directory under the repo root
		deps       []chart.Dependency
		want       []string // finding messages, each a substring
	}{
		{name: "valid", deps: []chart.Dependency{lib(chart.Dependency{})}},
		{name: "floor equal to the library", deps: []chart.Dependency{lib(chart.Dependency{Version: "^4.2.0"})}},
		{name: "lifecycle path", dir: "lifecycle", deps: []chart.Dependency{lib(chart.Dependency{Repository: "file://../lib/policy-library"})}},
		{name: "trailing slash", deps: []chart.Dependency{lib(chart.Dependency{Repository: "file://../../lib/policy-library/"})}},
		{
			name: "fixture deep in tools", dir: "tools/validator/internal/chart/testdata/element",
			deps: []chart.Dependency{lib(chart.Dependency{Repository: "file://../../../../../../lib/policy-library"})},
		},
		{
			name: "wrong path", deps: []chart.Dependency{lib(chart.Dependency{Repository: "file://../lib/policy-library"})},
			want: []string{`policy-library repository "file://../lib/policy-library" is not this repo's lib/policy-library; set repository: "file://../../lib/policy-library"`},
		},
		{
			name: "absolute path", deps: []chart.Dependency{lib(chart.Dependency{Repository: "file://<root>/lib/policy-library"})},
			want: []string{`/lib/policy-library" is not this repo's lib/policy-library; set repository: "file://../../lib/policy-library"`},
		},
		{
			name: "http repository", deps: []chart.Dependency{lib(chart.Dependency{Repository: "https://policystack.github.io/PolicyStack-chart"})},
			want: []string{`repository "https://policystack.github.io/PolicyStack-chart" is not this repo's lib/policy-library`},
		},
		{
			name: "missing alias", deps: []chart.Dependency{{Name: "policy-library", Version: "^4.0.0", Repository: "file://../../lib/policy-library"}},
			want: []string{`policy-library has alias ""; set alias: stack`},
		},
		{
			name: "floor above the library", deps: []chart.Dependency{lib(chart.Dependency{Version: "^4.3.0"})},
			want: []string{"policy-library floor ^4.3.0 is above lib/policy-library version 4.2.0; lower the floor, or bump lib/policy-library/Chart.yaml"},
		},
		{
			name: "next major", deps: []chart.Dependency{lib(chart.Dependency{Version: "^5.0.0"})},
			want: []string{"floor ^5.0.0 is above lib/policy-library version 4.2.0"},
		},
		{
			name: "previous major", deps: []chart.Dependency{lib(chart.Dependency{Version: "^3.4.0"})},
			want: []string{`policy-library ^3.4.0 does not accept lib/policy-library version 4.2.0; set version: "^4.<minor>.0"`},
		},
		{
			name: "prerelease library", libVersion: "4.3.0-rc.1", deps: []chart.Dependency{lib(chart.Dependency{})},
			want: []string{"lib/policy-library version 4.3.0-rc.1 is a prerelease, which no ^4.<minor>.0 floor accepts; drop -rc.1 from lib/policy-library/Chart.yaml"},
		},
		{
			name: "prerelease library below the floor", libVersion: "4.3.0-rc.1", deps: []chart.Dependency{lib(chart.Dependency{Version: "^4.3.0"})},
			want: []string{"version 4.3.0-rc.1 is a prerelease"},
		},
		{
			name: "exact version", deps: []chart.Dependency{lib(chart.Dependency{Version: "4.2.0"})},
			want: []string{`policy-library version "4.2.0" is not a caret floor; set version: "^4.<minor>.0"`},
		},
		{name: "tilde", deps: []chart.Dependency{lib(chart.Dependency{Version: "~4.2.0"})}, want: []string{"is not a caret floor"}},
		{name: "range", deps: []chart.Dependency{lib(chart.Dependency{Version: ">=4.0.0"})}, want: []string{"is not a caret floor"}},
		{name: "wildcard", deps: []chart.Dependency{lib(chart.Dependency{Version: "^4.x"})}, want: []string{"is not a caret floor"}},
		{
			name: "every field wrong", deps: []chart.Dependency{{Name: "policy-library", Version: "4.0.0", Repository: "https://example.com"}},
			want: []string{`alias ""`, `repository "https://example.com"`, "is not a caret floor"},
		},
		{
			name: "two dependencies", deps: []chart.Dependency{lib(chart.Dependency{}), {Name: "other", Version: "1.0.0", Repository: "https://example.com"}},
			want: []string{`Chart.yaml needs exactly one dependency, policy-library (name: policy-library, alias: stack, version: "^4.0.0", repository: "file://../../lib/policy-library"); it has 2`},
		},
		{name: "policy-library twice", deps: []chart.Dependency{lib(chart.Dependency{}), lib(chart.Dependency{})}, want: []string{"it has 2"}},
		{name: "no dependency", want: []string{"needs exactly one dependency, policy-library"}},
		{name: "another chart only", deps: []chart.Dependency{{Name: "other"}}, want: []string{"it has 1"}},
		{name: "no library", libVersion: "-", deps: []chart.Dependency{lib(chart.Dependency{})}, want: []string{"cannot read the policy-library version"}},
		{name: "invalid library version", libVersion: "four", deps: []chart.Dependency{lib(chart.Dependency{})}, want: []string{"cannot read the policy-library version"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.libVersion != "-" {
				v := cmp.Or(tt.libVersion, "4.2.0")
				if err := os.MkdirAll(filepath.Join(root, "lib/policy-library"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "lib/policy-library/Chart.yaml"), []byte("name: policy-library\nversion: "+v+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(root, cmp.Or(tt.dir, "stack/a"))
			for i, d := range tt.deps {
				tt.deps[i].Repository = strings.ReplaceAll(d.Repository, "<root>", root)
			}
			el := &chart.Element{ChartName: "a", Dir: dir, Dependencies: tt.deps}
			// A second component of the same chart, as the lifecycle chart has, is reported once.
			twin := *el
			got := (&PinningCheck{Root: root}).Run(Context{AllElements: []*chart.Element{el, &twin}})
			if len(got) != len(tt.want) {
				t.Fatalf("got %d findings %+v, want %d", len(got), got, len(tt.want))
			}
			for i, f := range got {
				if f.RuleID != "POLICY060" || f.Severity != SevError || f.Element != "a" || f.File != filepath.Join(dir, "Chart.yaml") {
					t.Errorf("unexpected finding: %+v", f)
				}
				if !strings.Contains(f.Message, tt.want[i]) {
					t.Errorf("message %q does not contain %q", f.Message, tt.want[i])
				}
			}
		})
	}
}

// TestPolicy060_Repo checks every chart in this repo, the ones make validate does not load included.
func TestPolicy060_Repo(t *testing.T) {
	root, _ := filepath.Abs("../../../..")
	charts, _ := filepath.Glob(filepath.Join(root, "stack/*/Chart.yaml"))
	charts = append(charts, filepath.Join(root, "lifecycle/Chart.yaml"), filepath.Join(root, "tools/sample-element/Chart.yaml"),
		filepath.Join(root, "tools/validator/internal/chart/testdata/element/Chart.yaml"))
	var els []*chart.Element
	for _, c := range charts {
		data, err := os.ReadFile(c)
		if err != nil {
			t.Fatal(err)
		}
		var cy struct{ Dependencies []chart.Dependency }
		if err := yaml.Unmarshal(data, &cy); err != nil {
			t.Fatal(err)
		}
		els = append(els, &chart.Element{ChartName: filepath.Base(filepath.Dir(c)), Dir: filepath.Dir(c), Dependencies: cy.Dependencies})
	}
	if got := (&PinningCheck{Root: root}).Run(Context{AllElements: els}); len(got) != 0 {
		t.Errorf("findings: %+v", got)
	}
	if got := (&PinningCheck{}).Run(Context{AllElements: els}); got != nil {
		t.Errorf("without Root: %+v", got)
	}
}
