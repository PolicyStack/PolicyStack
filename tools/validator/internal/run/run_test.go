package run

import (
	"cmp"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/checks"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
)

func TestBuildPairs(t *testing.T) {
	a, b := &chart.Element{ChartName: "a"}, &chart.Element{ChartName: "b"}
	lc := []*chart.Element{{ChartName: "lifecycle"}}
	spoke := &fixtures.Cluster{Name: "spoke"}
	built := &fixtures.Cluster{Name: "built", Hub: "acm-dc1"}
	retired := &fixtures.Cluster{Name: "retired", Hub: "acm-dc1", State: "absent"}
	hub := &fixtures.Cluster{Name: "acm-dc1"}
	clusters := []*fixtures.Cluster{spoke, built, retired, hub}

	tests := []struct {
		name      string
		elements  []*chart.Element
		lifecycle []*chart.Element
		clusters  []*fixtures.Cluster
		want      []string // "<element>/<cluster>", "+fleet" when the fleet file is appended
	}{
		{
			name:      "lifecycle only for clusters that set hub",
			elements:  []*chart.Element{a, b},
			lifecycle: lc,
			clusters:  clusters,
			want: []string{
				"a/spoke", "a/built", "a/retired", "a/acm-dc1",
				"b/spoke", "b/built", "b/retired", "b/acm-dc1",
				"lifecycle/built+fleet", "lifecycle/retired+fleet",
			},
		},
		{
			name:     "no lifecycle chart",
			elements: []*chart.Element{a},
			clusters: clusters,
			want:     []string{"a/spoke", "a/built", "a/retired", "a/acm-dc1"},
		},
		{
			name:      "no element",
			lifecycle: lc,
			clusters:  clusters,
			want:      []string{"lifecycle/built+fleet", "lifecycle/retired+fleet"},
		},
		{
			name:      "no cluster sets hub",
			elements:  []*chart.Element{a},
			lifecycle: lc,
			clusters:  []*fixtures.Cluster{spoke, hub},
			want:      []string{"a/spoke", "a/acm-dc1"},
		},
		{name: "no clusters", elements: []*chart.Element{a}, lifecycle: lc},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, p := range buildPairs(tt.elements, tt.lifecycle, tt.clusters) {
				s := p.el.ChartName + "/" + p.cl.Name
				if p.fleetValues {
					s += "+fleet"
				}
				got = append(got, s)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValueFiles(t *testing.T) {
	opts := Options{ExtraValues: []string{"/extra/baseline.yaml"}}
	cl := &fixtures.Cluster{Name: "built", Hub: "acm-dc1", SourceFile: "/fleet/built.yaml"}
	cascadeFiles := []string{"/el/values.yaml", "/repo/values.yaml", "/repo/values/clusters/built.yaml"}

	tests := []struct {
		name string
		p    pair
		want []string
	}{
		{
			name: "element ends with ExtraValues",
			p:    pair{el: &chart.Element{ChartName: "a"}, cl: cl},
			want: append(slices.Clone(cascadeFiles), "/extra/baseline.yaml"),
		},
		{
			name: "lifecycle ends with the fleet file, without ExtraValues",
			p:    pair{el: &chart.Element{ChartName: "lifecycle"}, cl: cl, fleetValues: true},
			want: append(slices.Clone(cascadeFiles), "/fleet/built.yaml"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := valueFiles(slices.Clone(cascadeFiles), opts, tt.p); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectEngine(t *testing.T) {
	hcp, hive := &chart.Element{StackKey: "hypershift"}, &chart.Element{StackKey: "hive"}
	engines := []*chart.Element{hcp, hive}
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	defaults := write("values.yaml", "install:\n  engine: hypershift\n")
	aws := write("aws.yaml", "install:\n  engine: hive\n")
	other := write("other.yaml", "install:\n  platform: KubeVirt\nstack: {}\n")
	unknown := write("unknown.yaml", "install:\n  engine: siteconfig\n")
	tests := []struct {
		name  string
		files []string
		want  *chart.Element
	}{
		{"chart default", []string{defaults, other}, hcp},
		{"layer selects", []string{defaults, aws, other}, hive},
		{"later layer wins", []string{defaults, aws, write("back.yaml", "install:\n  engine: hypershift\n")}, hcp},
		{"missing file skipped", []string{defaults, aws, filepath.Join(dir, "absent.yaml")}, hive},
		{"unknown engine falls back to the first", []string{defaults, unknown}, hcp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectEngine(engines, tt.files); got != tt.want {
				t.Errorf("got %q, want %q", got.StackKey, tt.want.StackKey)
			}
		})
	}
}

func TestSetString(t *testing.T) {
	el := &chart.Element{ChartName: "upgrade"}
	tests := []struct {
		name string
		p    pair
		want []string
	}{
		{"neither set", pair{el: el, cl: &fixtures.Cluster{}}, nil},
		{"version only", pair{el: el, cl: &fixtures.Cluster{Version: "4.20.12"}}, []string{"version=4.20.12"}},
		{"channel only", pair{el: el, cl: &fixtures.Cluster{Channel: "stable-4.20"}}, []string{"channel=stable-4.20"}},
		{"both", pair{el: el, cl: &fixtures.Cluster{Version: "4.20.12", Channel: "stable-4.20"}}, []string{"version=4.20.12", "channel=stable-4.20"}},
		{
			name: "lifecycle reads the fleet file instead",
			p:    pair{el: &chart.Element{ChartName: "lifecycle"}, cl: &fixtures.Cluster{Version: "4.20.12"}, fleetValues: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setString(tt.p); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteRender(t *testing.T) {
	spoke := &fixtures.Cluster{Name: "x", SourceFile: "/fx/x.yaml"}
	hub := &fixtures.Cluster{Name: "x", SourceFile: "/fx/hubs/x.yaml"}
	el := &chart.Element{ChartName: "a"}
	type write struct {
		p      pair
		out    string
		failed *checks.Finding
	}
	tests := []struct {
		name        string
		fixturesDir string
		writes      []write
		want        map[string]string
	}{
		{
			name:   "spoke",
			writes: []write{{p: pair{el: el, cl: spoke}, out: "kind: Policy\n"}},
			want:   map[string]string{"x/a.yaml": "kind: Policy\n"},
		},
		{
			name: "hub and spoke share a name",
			writes: []write{
				{p: pair{el: el, cl: spoke}, out: "spoke\n"},
				{p: pair{el: el, cl: hub}, out: "hub\n"},
			},
			want: map[string]string{"x/a.yaml": "spoke\n", "hubs/x/a.yaml": "hub\n"},
		},
		{
			name:   "lifecycle",
			writes: []write{{p: pair{el: &chart.Element{ChartName: "lifecycle"}, cl: spoke, fleetValues: true}, out: "l\n"}},
			want:   map[string]string{"x/lifecycle.yaml": "l\n"},
		},
		{
			name: "empty render writes nothing",
			writes: []write{
				{p: pair{el: el, cl: spoke}, out: ""},
				{p: pair{el: el, cl: hub}, out: "\n \n"},
			},
			want: map[string]string{},
		},
		{
			name: "failure as comments without the tree's paths",
			writes: []write{{p: pair{el: el, cl: spoke}, out: "ignored", failed: &checks.Finding{
				Message: "boom in /repo/stack/a/values.yaml\nand /fx/x.yaml",
			}}},
			want: map[string]string{"x/a.yaml": "# RENDER000: boom in stack/a/values.yaml\n# and x.yaml\n"},
		},
		{
			name:        "relative fixtures dir with trailing slash",
			fixturesDir: "fleet/",
			writes:      []write{{p: pair{el: el, cl: &fixtures.Cluster{Name: "h", SourceFile: "fleet/hubs/h.yaml"}}, out: "h\n"}},
			want:        map[string]string{"hubs/h/a.yaml": "h\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{RepoRoot: "/repo", FixturesDir: cmp.Or(tt.fixturesDir, "/fx"), RenderDir: t.TempDir()}
			for _, w := range tt.writes {
				if err := writeRender(opts, w.p, []byte(w.out), w.failed); err != nil {
					t.Fatal(err)
				}
			}
			got := map[string]string{}
			err := filepath.WalkDir(opts.RenderDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				b, err := os.ReadFile(path)
				rel, _ := filepath.Rel(opts.RenderDir, path)
				got[filepath.ToSlash(rel)] = string(b)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDedupe(t *testing.T) {
	layer := checks.Finding{RuleID: "POLICY032", Element: "a", File: "/v/prod.yaml", Line: 4, Message: "m"}
	perCluster := checks.Finding{RuleID: "POLICY001", Element: "a", Cluster: "x", Message: "m"}
	other := perCluster
	other.Cluster = "y"
	got := dedupe([]checks.Finding{layer, perCluster, layer, other, perCluster, layer})
	if want := []checks.Finding{layer, perCluster, other}; !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDependencyFailure(t *testing.T) {
	const chartYAML = "apiVersion: v2\nname: a\nversion: 0.1.0\ndependencies:\n  - name: policy-library\n    version: 3.4.0\n    repository: https://example.invalid\n"
	tests := []struct {
		name     string
		archive  string // in stack/a/charts
		only     string
		want     []string // findings, "<rule> <element> <file>"
		rendered bool     // stack/a is rendered
	}{
		{
			name:    "stale archive is not rendered",
			archive: "policy-library-3.3.0.tgz",
			only:    "RENDER000",
			want:    []string{"RENDER000 a stack/a/Chart.yaml"},
		},
		{name: "no archive", only: "RENDER000", want: []string{"RENDER000 a stack/a/Chart.yaml"}},
		{name: "RENDER000 not run", archive: "policy-library-3.3.0.tgz", only: "POLICY081"},
		{name: "pinned archive needs no fetch", archive: "policy-library-3.4.0.tgz", only: "RENDER000", rendered: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			log := filepath.Join(root, "helm.log")
			write := func(rel, body string, mode os.FileMode) {
				p := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), mode); err != nil {
					t.Fatal(err)
				}
			}
			// A helm whose dependency update fails, as for an unpublished policy-library.
			write("helm", "#!/bin/sh\necho \"$*\" >> "+log+"\n[ \"$1\" = dependency ] && { echo 'Error: chart not found' >&2; exit 1; }\nexit 0\n", 0o755)
			write("stack/a/Chart.yaml", chartYAML, 0o644)
			write("stack/a/values.yaml", "stack:\n  a:\n    enabled: true\n", 0o644)
			write("stack/b/Chart.yaml", "apiVersion: v2\nname: b\nversion: 0.1.0\n", 0o644)
			write("stack/b/values.yaml", "stack:\n  b:\n    enabled: true\n", 0o644)
			write("values.yaml", "policyNamespace: policies\n", 0o644)
			write("fleet/c.yaml", "revision: main\n", 0o644)
			if tt.archive != "" {
				write("stack/a/charts/"+tt.archive, "", 0o644)
			}
			res, err := Run(t.Context(), Options{
				RepoRoot:     root,
				StackDir:     filepath.Join(root, "stack"),
				LifecycleDir: filepath.Join(root, "lifecycle"),
				ValuesDir:    filepath.Join(root, "values"),
				FixturesDir:  filepath.Join(root, "fleet"),
				HelmBin:      filepath.Join(root, "helm"),
				RenderDir:    filepath.Join(root, "rd"),
				Only:         []string{tt.only},
				Logger:       slog.New(slog.DiscardHandler),
			})
			if err != nil {
				t.Fatal(err)
			}
			// A failed fetch is written into the render, so a render diff shows a failure, not a deletion.
			out, _ := os.ReadFile(filepath.Join(root, "rd/c/a.yaml"))
			if failed := strings.HasPrefix(string(out), "# RENDER000: dependencies not fetched"); failed == tt.rendered {
				t.Errorf("render of a = %q, want RENDER000 comment = %v", out, !tt.rendered)
			}
			var got []string
			for _, f := range res.Findings {
				rel, _ := filepath.Rel(root, f.File)
				got = append(got, f.RuleID+" "+f.Element+" "+rel)
				if !strings.Contains(f.Message, "Error: chart not found") {
					t.Errorf("message %q does not carry helm's error", f.Message)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("findings %q, want %q", got, tt.want)
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(calls), "template b-c "+filepath.Join(root, "stack/b")) {
				t.Errorf("b not rendered:\n%s", calls)
			}
			if got := strings.Contains(string(calls), "template a-c "); got != tt.rendered {
				t.Errorf("a rendered = %v, want %v:\n%s", got, tt.rendered, calls)
			}
		})
	}
}
