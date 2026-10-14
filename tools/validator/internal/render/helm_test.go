package render

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAlreadyDownloaded(t *testing.T) {
	const chartYAML = "apiVersion: v2\nname: e\nversion: 0.1.0\ndependencies:\n  - name: policy-library\n    version: \"3.1.0\"\n"
	tests := []struct {
		name     string
		chart    string
		archives []string
		want     bool
	}{
		{"pinned archive present", chartYAML, []string{"policy-library-3.1.0.tgz"}, true},
		{"only a stale archive", chartYAML, []string{"policy-library-2.1.0.tgz"}, false},
		{"stale and pinned archives", chartYAML, []string{"policy-library-2.1.0.tgz", "policy-library-3.1.0.tgz"}, true},
		{"no archives", chartYAML, nil, false},
		{"no dependencies", "apiVersion: v2\nname: e\nversion: 0.1.0\n", nil, true},
		{"no Chart.yaml", "", nil, false},
		{"invalid Chart.yaml", "dependencies: [", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.chart != "" {
				if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(tt.chart), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(filepath.Join(dir, "charts"), 0o755); err != nil {
				t.Fatal(err)
			}
			for _, a := range tt.archives {
				if err := os.WriteFile(filepath.Join(dir, "charts", a), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := alreadyDownloaded(dir); got != tt.want {
				t.Errorf("alreadyDownloaded() = %v, want %v", got, tt.want)
			}
		})
	}
}
