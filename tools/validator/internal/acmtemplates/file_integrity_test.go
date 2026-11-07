package acmtemplates

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

var fileIntegrityDir = filepath.Join(repoRoot, "stack/file-integrity")

// exclusionLayers writes one value file per aideExclusions flow map and returns their paths in order.
func exclusionLayers(t *testing.T, maps ...string) []string {
	t.Helper()
	var files []string
	for _, m := range maps {
		f := filepath.Join(t.TempDir(), "values.yaml")
		if err := os.WriteFile(f, []byte("stack: {fileIntegrity: {config: {aideExclusions: "+m+"}}}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	return files
}

// TestFileIntegrityExclusions: exclusions follow the copied default in key order, and a later layer's
// null drops an entry, the default one included.
func TestFileIntegrityExclusions(t *testing.T) {
	const head, catchAll = "@@define DBDIR /hostroot/etc/kubernetes\n", "/hostroot/etc/    CONTENT_EX\n"
	for _, tt := range []struct {
		name   string
		layers []string
		want   string // everything after the default's last rule
	}{
		{"default", nil, "!/etc/machine-config-daemon/previous-logs\n"},
		{"added", []string{`{/etc/myapp: x}`}, "!/etc/machine-config-daemon/previous-logs\n!/etc/myapp\n"},
		{"anchored file", []string{`{/etc/myapp.conf$: x}`}, "!/etc/machine-config-daemon/previous-logs\n!/etc/myapp.conf$\n"},
		{"nulled", []string{`{/etc/myapp: x}`, `{/etc/myapp: null, /etc/machine-config-daemon/previous-logs: null}`}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var o map[string]any
			if err := json.Unmarshal(objectDefinition(t, fileIntegrityDir, "ConfigMap", "file-integrity-aide", exclusionLayers(t, tt.layers...)...), &o); err != nil {
				t.Fatal(err)
			}
			conf, _, _ := unstructured.NestedString(o, "data", "aide.conf")
			_, tail, ok := strings.Cut(conf, catchAll)
			if !strings.HasPrefix(conf, head) || !ok || tail != tt.want {
				t.Fatalf("aide.conf tail = %q, want %q; full:\n%s", tail, tt.want, conf)
			}
		})
	}
}

func TestFileIntegrityExclusionsRejected(t *testing.T) {
	runner := render.New(testenv.Helm(t))
	if err := runner.EnsureDeps(context.Background(), fileIntegrityDir); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, exclusions string }{
		{"relative", `{etc/foo: x}`},
		{"whitespace", `{"/etc/my dir": x}`},
		{"root", `{"/": x}`},
		{"negated", `{"!/etc/foo": x}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := append(slices.Clone(allOn), exclusionLayers(t, tt.exclusions)...)
			tr := runner.Template(context.Background(), fileIntegrityDir, "file-integrity", files, nil)
			if tr.Err == nil || !strings.Contains(string(tr.Stderr), "must be an absolute host path") {
				t.Fatalf("want rejection, got err %v: %s", tr.Err, tr.Stderr)
			}
		})
	}
}
