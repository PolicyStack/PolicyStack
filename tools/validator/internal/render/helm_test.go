package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureDeps checks that EnsureDeps runs one `helm dependency update` per chart and process,
// even when charts/ already holds the archive, and repeats the first error. --skip-refresh is passed
// only when every dependency is file://.
func TestEnsureDeps(t *testing.T) {
	const local = "dependencies:\n- name: policy-library\n  repository: file://../../lib/policy-library\n"
	for _, tt := range []struct {
		name      string
		fail      bool
		archive   bool   // charts/ holds a policy-library archive already
		chartYaml string // empty: no Chart.yaml
		refresh   bool   // want no --skip-refresh
	}{
		{name: "no archive", chartYaml: local},
		{name: "archive present is refreshed", archive: true, chartYaml: local},
		{name: "failure is cached", fail: true, chartYaml: local},
		{name: "failure despite an archive", fail: true, archive: true, chartYaml: local},
		{name: "no dependencies", chartYaml: "name: el\n"},
		{
			name:      "remote dependency fetches its index",
			chartYaml: "dependencies:\n- name: policy-library\n  repository: https://policystack.github.io/PolicyStack-chart\n",
			refresh:   true,
		},
		{
			name:      "one remote among local",
			chartYaml: local + "- name: other\n  repository: oci://example.com/charts\n",
			refresh:   true,
		},
		{name: "no Chart.yaml", refresh: true},
		{name: "invalid Chart.yaml", chartYaml: "dependencies: [\n", refresh: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			log, chart := filepath.Join(dir, "helm.log"), filepath.Join(dir, "el")
			status := "0"
			if tt.fail {
				status = "1"
			}
			helm := filepath.Join(dir, "helm")
			script := "#!/bin/sh\necho \"$*\" >> " + log + "\necho 'Error: no valid version' >&2\nexit " + status + "\n"
			if err := os.WriteFile(helm, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(chart, "charts"), 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.chartYaml != "" {
				if err := os.WriteFile(filepath.Join(chart, "Chart.yaml"), []byte(tt.chartYaml), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.archive {
				if err := os.WriteFile(filepath.Join(chart, "charts", "policy-library-4.0.0.tgz"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			r := New(helm)
			first, second := r.EnsureDeps(t.Context(), chart), r.EnsureDeps(t.Context(), chart)
			if (first != nil) != tt.fail || second != first {
				t.Errorf("errors = %v, %v; want failure %v, the same error twice", first, second, tt.fail)
			}
			if first != nil && !strings.Contains(first.Error(), "no valid version") {
				t.Errorf("error %q does not carry helm's stderr", first)
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			want := "dependency update --skip-refresh " + chart + "\n"
			if tt.refresh {
				want = "dependency update " + chart + "\n"
			}
			if string(calls) != want {
				t.Errorf("helm calls %q, want %q", calls, want)
			}
		})
	}
}
