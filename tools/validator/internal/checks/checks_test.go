package checks

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/cascade"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
)

// loadElement is a test helper: writes values.yaml + Chart.yaml in a temp
// dir and parses them via chart.Load.
func loadElement(t *testing.T, valuesYAML string, chartYAML string, convertersFiles []string) *chart.Element {
	t.Helper()
	dir := t.TempDir()
	if chartYAML == "" {
		chartYAML = `apiVersion: v2
name: my-element
version: 0.1.0
dependencies:
  - name: policy-library
    version: "1.1.0"
    repository: https://policystack.github.io/PolicyStack-chart
`
	}
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(chartYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), []byte(valuesYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(convertersFiles) > 0 {
		_ = os.MkdirAll(filepath.Join(dir, "converters"), 0o755)
		for _, n := range convertersFiles {
			_ = os.MkdirAll(filepath.Dir(filepath.Join(dir, "converters", n)), 0o755)
			_ = os.WriteFile(filepath.Join(dir, "converters", n), []byte("kind: ConfigMap\n"), 0o644)
		}
	}
	el, err := chart.Load(dir, "policy")
	if err != nil {
		t.Fatal(err)
	}
	return el
}

func TestPolicy021_UnusedConverter(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enabled: true
    policies:
      config:
        templates:
          replaced:
            file: _shared/used
          extra:
            file: [top, deep/a/b]
`, "", []string{
		"config/cm.yaml", "config/replaced.yaml", "config/old.yml", "config/notes.txt",
		"_shared/used.yaml", "_shared/dead.yaml", "_shared/read.yaml", "config/_partial.yaml",
		"top.yaml", "orphan.yaml", "deep/a/b.yaml", "deep/a/c.yaml",
	})
	if err := os.WriteFile(filepath.Join(el.Dir, "converters/config/cm.yaml"), []byte(`{{ tpl ($.Files.Get "converters/_shared/read.yaml") . }}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range (&UnusedConverterCheck{}).Run(Context{Element: el}) {
		if f.Severity != SevWarning || f.Line != 1 || f.File != filepath.Join(el.ConvertersDir, strings.Fields(f.Message)[0][len("converters/"):]) {
			t.Errorf("finding %+v", f)
		}
		got = append(got, f.Message)
	}
	want := []string{
		"converters/_shared/dead.yaml is never rendered: no templates.<t>.file names this partial and no chart file reads it",
		"converters/config/_partial.yaml is never rendered: no templates.<t>.file names this partial and no chart file reads it",
		"converters/config/old.yml is never rendered: policy-library renders only .yaml converters",
		"converters/config/replaced.yaml is never rendered: it is not converters/<policy>/<template>.yaml of a template and no templates.<t>.file names it",
		"converters/deep/a/c.yaml is never rendered: it is not converters/<policy>/<template>.yaml of a template and no templates.<t>.file names it",
		"converters/orphan.yaml is never rendered: it is not converters/<policy>/<template>.yaml of a template and no templates.<t>.file names it",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestPolicy030_BadEnums(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enabled: true
    default:
      severity: hi
    policies:
      p:
        severity: extreme
        remediationAction: nuke
        templates:
          t:
            complianceType: musthav
`, "", nil)
	var got []string
	for _, f := range (&EnumCheck{}).Run(Context{Element: el}) {
		got = append(got, f.Message)
	}
	want := []string{
		`default.severity = "hi"; allowed: [low medium high critical]`,
		`policies.p.severity = "extreme"; allowed: [low medium high critical]`,
		`policies.p.remediationAction = "nuke"; allowed: [inform enforce]`,
		`policies.p.templates.t.complianceType = "musthav"; allowed: [musthave mustnothave mustonlyhave]`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestPolicy030_InformOnly: a template's own action is InformOnly or nothing; a Policy's is never
// InformOnly.
func TestPolicy030_InformOnly(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enabled: true
    policies:
      p:
        remediationAction: InformOnly
        templates:
          a:
            remediationAction: InformOnly
          b:
            remediationAction: inform
`, "", nil)
	var got []string
	for _, f := range (&EnumCheck{}).Run(Context{Element: el}) {
		got = append(got, f.Message)
	}
	want := []string{
		`policies.p.remediationAction = "InformOnly"; allowed: [inform enforce]`,
		`policies.p.templates.b.remediationAction = "inform"; allowed: [InformOnly]`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPolicy040_PolicySetMissingMember(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enabled: true
    operatorPolicies:
      op:
        subscription: {name: op}
    policySets:
      - name: set
        policies:
          - real
          - install
          - imaginary
`, "", []string{"real/cm.yaml"})
	got := (&PolicySetCheck{}).Run(Context{Element: el})
	if len(got) != 1 || !strings.Contains(got[0].Message, `"imaginary", which is not a policy of the element`) || got[0].Line != 13 {
		t.Fatalf("got %+v, want one imaginary finding", got)
	}
}

func TestPolicy090_Engines(t *testing.T) {
	const schema = `{"properties": {"install": {"properties": {"engine": {"enum": ["hypershift", "hive"]}}}}}`
	tests := []struct {
		name   string
		values string
		want   []string // substrings, one per finding
	}{
		{"one component per engine", "stack:\n  hypershift: {}\n  hive: {}\n", nil},
		{"component without engine", "stack:\n  hypershift: {}\n  hive: {}\n  lifecycle: {}\n", []string{`stack key "lifecycle" is not an install.engine`}},
		{"engine without component", "stack:\n  hypershift: {}\n", []string{`install.engine "hive" in values.schema.json has no stack.hive component`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range map[string]string{
				"Chart.yaml":         "apiVersion: v2\nname: lifecycle\nversion: 0.1.0\n",
				"values.yaml":        tt.values,
				"values.schema.json": schema,
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			el, err := chart.Load(dir, "policy")
			if err != nil {
				t.Fatal(err)
			}
			got := (&CamelCaseCheck{}).Run(Context{Element: el})
			if len(got) != len(tt.want) {
				t.Fatalf("got %d findings, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				if !strings.Contains(got[i].Message, w) || got[i].Line == 0 {
					t.Errorf("[%d] got %q at line %d, want %q with a line", i, got[i].Message, got[i].Line, w)
				}
			}
		})
	}
}

func TestPolicy090_CamelCaseMismatch(t *testing.T) {
	el := loadElement(t, `
stack:
  wrongKey:
    enable: false
`, "", nil)
	got := (&CamelCaseCheck{}).Run(Context{Element: el})
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "myElement") {
		t.Errorf("unexpected: %s", got[0].Message)
	}
}

func TestPolicy091_ElementNames(t *testing.T) {
	el := func(dir, name string) *chart.Element { return &chart.Element{Dir: dir, ChartName: name} }
	tests := []struct {
		name string
		els  []*chart.Element
		want []string // substrings, one per finding
	}{
		{"distinct", []*chart.Element{el("stack/gitops", "gitops"), el("stack/team-gitops", "team-gitops"), el("lifecycle", "lifecycle")}, nil},
		{"lifecycle components share a dir", []*chart.Element{el("lifecycle", "lifecycle"), el("lifecycle", "lifecycle")}, nil},
		{"no hyphen after prefix", []*chart.Element{el("stack/acm", "acm"), el("stack/acms", "acms")}, nil},
		{"prefix", []*chart.Element{el("stack/gitops-dev", "gitops-dev"), el("stack/gitops", "gitops")},
			[]string{`gitops-dev on cluster "c" and gitops on cluster "dev-c" share the release name gitops-dev-c`}},
		{"reserved lifecycle", []*chart.Element{el("stack/lifecycle-extras", "lifecycle-extras"), el("lifecycle", "lifecycle")},
			[]string{`element name "lifecycle-extras" starts with element name "lifecycle"`}},
		{"same chart name", []*chart.Element{el("stack/a", "loki"), el("stack/b", "loki")},
			[]string{`element name "loki" is also the chart name in a`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&ElementNameCheck{}).Run(Context{AllElements: tt.els})
			if len(got) != len(tt.want) {
				t.Fatalf("got %d findings, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				if !strings.Contains(got[i].Message, w) {
					t.Errorf("[%d] got %q, want %q", i, got[i].Message, w)
				}
			}
		})
	}
}

func TestPolicy050_FleetFileIssues(t *testing.T) {
	if got := (&LabelCheck{}).Run(Context{Cluster: &cascade.Resolved{ClusterName: "x"}}); len(got) != 0 {
		t.Fatalf("expected no findings for a clean fleet file, got %+v", got)
	}
	cl := &cascade.Resolved{ClusterName: "x", Issues: []fixtures.Issue{
		{Line: 2, Message: "field revison not found in type fixtures.fleetFile"},
		{Message: `valueFiles "platforms/aws.yaml": not a file under values/`},
	}}
	got := (&LabelCheck{}).Run(Context{Cluster: cl})
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %+v", len(got), got)
	}
	if got[0].Line != 2 || got[0].Message != "invalid fleet file: field revison not found in type fixtures.fleetFile" {
		t.Errorf("unexpected finding: %+v", got[0])
	}
	if got[1].Line != 0 || got[1].Cluster != "x" || got[1].Severity != SevError {
		t.Errorf("unexpected finding: %+v", got[1])
	}
}
