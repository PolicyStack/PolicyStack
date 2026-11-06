package run

import (
	"cmp"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

// TestPreview runs one element for one fleet file with --evaluate, as make preview does, on a copy of
// the conformance fixture.
func TestPreview(t *testing.T) {
	helm := testenv.Helm(t)
	repo, _ := filepath.Abs("../../../..")
	root := t.TempDir()
	for _, name := range []string{"conformance", "other"} {
		dir := filepath.Join(root, "stack", name)
		if err := os.CopyFS(dir, os.DirFS(filepath.Join(repo, "tools/validator/internal/chart/testdata/element"))); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(fixtureChart(t, name)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for rel, body := range map[string]string{"values.yaml": "policyNamespace: policy\n", "fleet/c.yaml": "revision: main\n", "fleet/d.yaml": "revision: main\n"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts := Options{
		RepoRoot:     root,
		StackDir:     filepath.Join(root, "stack"),
		LifecycleDir: filepath.Join(root, "lifecycle"),
		ValuesDir:    filepath.Join(root, "values"),
		FixturesDir:  filepath.Join(root, "fleet"),
		HelmBin:      helm,
		Only:         []string{"RENDER000", "POLICY081"},
		Logger:       slog.New(slog.DiscardHandler),
		RenderDir:    filepath.Join(root, "out"),
		Element:      "conformance",
		Cluster:      "c",
		Evaluate:     true,
	}
	res, err := Run(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		t.Errorf("findings %+v", res.Findings)
	}
	var files []string
	_ = filepath.WalkDir(opts.RenderDir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(opts.RenderDir, path)
			files = append(files, rel)
		}
		return err
	})
	if want := []string{"c/conformance.evaluated.yaml", "c/conformance.yaml"}; !slices.Equal(files, want) {
		t.Fatalf("wrote %v, want %v", files, want)
	}
	evaluated, _ := os.ReadFile(filepath.Join(opts.RenderDir, "c/conformance.evaluated.yaml"))
	for _, want := range []string{
		"---\n# config-rawlist (musthave)\napiVersion: v1\ndata:\n  value: x\nkind: ConfigMap\n", // raw, resolved by Helm
		"# gates-version (mustnothave)\napiVersion: config.openshift.io/v1\n",                    // no ACM template
		"# install-my-operator-ns (musthave)\n",                                                  // the operator's namespace
		"# config-pair (musthave, looks up an object the cluster lacks: as rendered)\n",          // fromConfigMap of a missing ConfigMap
	} {
		if !strings.Contains(string(evaluated), want) {
			t.Errorf("evaluated objects lack %q:\n%s", want, evaluated)
		}
	}

	for _, bad := range []Options{{Element: "nope"}, {Cluster: "nope"}} {
		o := opts
		o.Element, o.Cluster, o.RenderDir = cmp.Or(bad.Element, opts.Element), cmp.Or(bad.Cluster, opts.Cluster), ""
		if _, err := Run(t.Context(), o); err == nil || !strings.Contains(err.Error(), "nope") {
			t.Errorf("%+v: err %v, want an unknown name error", bad, err)
		}
	}
}
