package run

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

// TestRenderPhaseLayers renders with the real policy-library: the render-phase rules see what a
// values layer changes and report it at the layer.
func TestRenderPhaseLayers(t *testing.T) {
	helm := testenv.Helm(t)
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("stack/gate/Chart.yaml", fixtureChart(t, "gate"))
	write("stack/gate/templates/policy.yaml", `{{- include "policy-library.render" . -}}`+"\n")
	write("stack/gate/converters/gate/cv.yaml", "apiVersion: config.openshift.io/v1\nkind: ClusterVersion\nmetadata:\n  name: version\n")
	write("stack/gate/values.yaml", `stack:
  gate:
    enabled: true
    policies:
      gate:
        templates:
          cv:
            complianceType: mustnothave
`)
	long := strings.Repeat("l", 48)
	// regression: 9ddc38f, an enforced mustnothave gate; audit probe B, a layer adding a Policy past 63
	// characters.
	write("values/clusters/c.yaml", `stack:
  gate:
    policies:
      gate:
        remediationAction: enforce
        templates:
          cv:
            complianceType: mustnothave
      `+long+`:
        templates:
          x:
            file: gate/cv
`)
	// regression: 12e312c, a value the chart schema rejects.
	write("values/clusters/e.yaml", `stack:
  gate:
    policies:
      gate:
        orderManifests: true
`)
	write("values.yaml", "policyNamespace: policies\n")
	write("baseline.yaml", "selector:\n  matchExpressions:\n    - {key: name, operator: Exists}\n")
	write("fleet/c.yaml", "revision: main\n")
	write("fleet/d.yaml", "revision: main\n")
	write("fleet/e.yaml", "revision: main\n")

	res, err := Run(t.Context(), Options{
		RepoRoot:     root,
		StackDir:     filepath.Join(root, "stack"),
		LifecycleDir: filepath.Join(root, "lifecycle"),
		ValuesDir:    filepath.Join(root, "values"),
		FixturesDir:  filepath.Join(root, "fleet"),
		HelmBin:      helm,
		ExtraValues:  []string{filepath.Join(root, "baseline.yaml")},
		Only:         []string{"RENDER000", "POLICY001", "POLICY037"},
		Logger:       slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range res.Findings {
		rel, _ := filepath.Rel(root, f.File)
		got = append(got, f.RuleID+" "+f.Cluster+" "+rel)
	}
	slices.Sort(got)
	want := []string{
		"POLICY001 c values/clusters/c.yaml", // policies.<long>-gate-c, 64 characters
		"POLICY037  stack/gate/values.yaml",  // the base entry, rendered for d
		"POLICY037  values/clusters/c.yaml",  // the layer's copy of it, rendered for c
		"RENDER000 e stack/gate/values.yaml",
	}
	if !slices.Equal(got, want) {
		t.Errorf("findings\n got %q\nwant %q\n%+v", got, want, res.Findings)
	}
}

// fixtureChart returns the conformance fixture's Chart.yaml, which declares policy-library as the
// elements do, for a copy named name outside the repo: the library path is made absolute.
func fixtureChart(t *testing.T, name string) string {
	t.Helper()
	repo, _ := filepath.Abs("../../../..")
	data, err := os.ReadFile(filepath.Join(repo, "tools/validator/internal/chart/testdata/element/Chart.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReplacer("name: conformance", "name: "+name, "file://../../../../../../lib", "file://"+filepath.Join(repo, "lib")).Replace(string(data))
}
