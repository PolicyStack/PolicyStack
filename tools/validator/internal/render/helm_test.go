package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEnsureDeps checks that EnsureDeps runs at most one `helm dependency update` per chart and
// process, none for a chart without dependencies, and repeats the first error. An archive whose
// file:// source is missing is not trusted. --skip-refresh is passed only when every dependency is
// file://.
func TestEnsureDeps(t *testing.T) {
	const local = "dependencies:\n- name: policy-library\n  repository: file://../../lib/policy-library\n"
	for _, tt := range []struct {
		name      string
		fail      bool
		archive   bool   // charts/ holds a policy-library archive already
		chartYaml string // empty: no Chart.yaml
		refresh   bool   // want no --skip-refresh
		noCall    bool   // want no helm call
	}{
		{name: "no archive", chartYaml: local},
		{name: "archive without its source is refreshed", archive: true, chartYaml: local},
		{name: "failure is cached", fail: true, chartYaml: local},
		{name: "failure despite an archive", fail: true, archive: true, chartYaml: local},
		{name: "no dependencies", chartYaml: "name: el\n", noCall: true},
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
			calls, _ := os.ReadFile(log)
			want := "dependency update --skip-refresh " + chart + "\n"
			switch {
			case tt.noCall:
				want = ""
			case tt.refresh:
				want = "dependency update " + chart + "\n"
			}
			if string(calls) != want {
				t.Errorf("helm calls %q, want %q", calls, want)
			}
		})
	}
}

// TestEnsureDepsUpToDate checks that a file:// dependency is not updated again while its archive is
// newer than every file in the source directory, and is updated once any source file is newer or
// charts/ holds no single archive for it.
func TestEnsureDepsUpToDate(t *testing.T) {
	const chartYaml = "dependencies:\n- name: policy-library\n  repository: file://../lib\n"
	old, built, edited := time.Unix(1000, 0), time.Unix(2000, 0), time.Unix(3000, 0)
	for _, tt := range []struct {
		name     string
		archives []string
		srcTime  time.Time
		want     bool // want helm dependency update
	}{
		{name: "archive newer than source", archives: []string{"policy-library-4.1.0.tgz"}, srcTime: old},
		{name: "source edited after build", archives: []string{"policy-library-4.1.0.tgz"}, srcTime: edited, want: true},
		{name: "no archive", srcTime: old, want: true},
		{name: "stale second archive", archives: []string{"policy-library-4.0.0.tgz", "policy-library-4.1.0.tgz"}, srcTime: old, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			log, chart, src := filepath.Join(dir, "helm.log"), filepath.Join(dir, "el"), filepath.Join(dir, "lib")
			helm := filepath.Join(dir, "helm")
			if err := os.WriteFile(helm, []byte("#!/bin/sh\necho \"$*\" >> "+log+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			for _, d := range []string{filepath.Join(chart, "charts"), filepath.Join(src, "templates")} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(chart, "Chart.yaml"), []byte(chartYaml), 0o644); err != nil {
				t.Fatal(err)
			}
			srcFile := filepath.Join(src, "templates", "_helpers.tpl")
			if err := os.WriteFile(srcFile, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			for _, p := range []string{src, filepath.Join(src, "templates"), srcFile} {
				if err := os.Chtimes(p, old, tt.srcTime); err != nil {
					t.Fatal(err)
				}
			}
			for _, a := range tt.archives {
				p := filepath.Join(chart, "charts", a)
				if err := os.WriteFile(p, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(p, built, built); err != nil {
					t.Fatal(err)
				}
			}
			if err := New(helm).EnsureDeps(t.Context(), chart); err != nil {
				t.Fatal(err)
			}
			_, err := os.Stat(log)
			if got := err == nil; got != tt.want {
				t.Errorf("helm dependency update ran = %v, want %v", got, tt.want)
			}
		})
	}
}
