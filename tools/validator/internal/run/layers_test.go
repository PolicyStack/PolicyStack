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
	repo, _ := filepath.Abs("../../../..")
	chartYAML, err := os.ReadFile(filepath.Join(repo, "stack/upgrade/Chart.yaml"))
	if err != nil {
		t.Fatal(err)
	}
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
	// The pinned archive saves a fetch when stack/upgrade has one.
	archives, _ := filepath.Glob(filepath.Join(repo, "stack/upgrade/charts/policy-library-*.tgz"))
	for _, a := range archives {
		data, err := os.ReadFile(a)
		if err != nil {
			t.Fatal(err)
		}
		write("stack/gate/charts/"+filepath.Base(a), string(data))
	}
	write("stack/gate/Chart.yaml", strings.Replace(string(chartYAML), "name: upgrade", "name: gate", 1))
	write("stack/gate/templates/policy.yaml", `{{- include "policy-library.render" . -}}`+"\n")
	write("stack/gate/converters/cv.yaml", "apiVersion: config.openshift.io/v1\nkind: ClusterVersion\nmetadata:\n  name: version\n")
	write("stack/gate/values.yaml", `stack:
  gate:
    enabled: true
    policies:
      - name: gate
        enabled: true
    configPolicies:
      - name: cv
        enabled: true
        policyRef: gate
        complianceType: mustnothave
        templateNames:
          - name: cv
`)
	long := strings.Repeat("l", 48)
	// regression: 9ddc38f, restating a Policy-level enforce over the inform gate; audit probe B, a
	// rename past 63 characters.
	write("values/clusters/c.yaml", `stack:
  gate:
    policies:
      - name: gate
        enabled: true
        remediationAction: enforce
      - name: `+long+`
        enabled: true
    configPolicies:
      - name: cv
        enabled: true
        policyRef: gate
        complianceType: mustnothave
        templateNames:
          - name: cv
      - name: x
        enabled: true
        policyRef: `+long+`
        templateNames:
          - name: cv
`)
	// regression: 12e312c, a CertificatePolicy duration as an integer, which the chart schema rejects.
	write("values/clusters/e.yaml", `stack:
  gate:
    certificatePolicies:
      - name: certs
        enabled: true
        policyRef: gate
        minimumDuration: 720
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
		Only:         []string{"RENDER000", "POLICY001", "POLICY036", "POLICY037"},
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
		"POLICY036  values/clusters/c.yaml",
		"POLICY037  stack/gate/values.yaml", // the base entry, rendered for d
		"POLICY037  values/clusters/c.yaml", // the layer's copy of it, rendered for c
		"RENDER000 e stack/gate/values.yaml",
	}
	if !slices.Equal(got, want) {
		t.Errorf("findings\n got %q\nwant %q\n%+v", got, want, res.Findings)
	}
}
