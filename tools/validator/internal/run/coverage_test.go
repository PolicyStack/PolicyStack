package run

import (
	"bytes"
	"cmp"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/checks"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

// TestProfileCoverage renders every element for every fixture in the all-on run and under each
// testdata/profiles file, as make validate does, and fails for a converter with escaped ACM
// templates that no render includes.
func TestProfileCoverage(t *testing.T) {
	helm := testenv.Helm(t)
	root, _ := filepath.Abs("../../../..")
	testdata := filepath.Join(root, "tools/validator/testdata")
	profiles, _ := filepath.Glob(filepath.Join(testdata, "profiles/*.yaml"))

	rendered := map[string]bool{} // <chart>/<ConfigurationPolicy>
	// "" is the all-on run: --all-on and testdata/inputs.yaml.
	for _, p := range append([]string{""}, profiles...) {
		dir := t.TempDir()
		values := []string{filepath.Join(testdata, "baseline.yaml"), cmp.Or(p, filepath.Join(testdata, "inputs.yaml"))}
		res, err := Run(t.Context(), Options{
			RepoRoot:     root,
			StackDir:     filepath.Join(root, "stack"),
			LifecycleDir: dir, // no lifecycle chart: its converters have their own tests
			ValuesDir:    filepath.Join(root, "values"),
			FixturesDir:  filepath.Join(testdata, "clusters"),
			HelmBin:      helm,
			ExtraValues:  values,
			AllOn:        p == "",
			Only:         []string{"RENDER000"},
			RenderDir:    dir,
			Logger:       slog.New(slog.DiscardHandler),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range res.Findings {
			t.Errorf("%s: %s does not render for %s", filepath.Base(cmp.Or(p, "all-on")), f.Element, f.Cluster)
		}
		err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			el := strings.TrimSuffix(d.Name(), ".yaml")
			for _, name := range configPolicies(t, data) {
				rendered[el+"/"+name] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	elements, _, err := chart.LoadAll(filepath.Join(root, "stack"), "", filepath.Join(root, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	covered := map[string]bool{} // stack/<chart>/converters/<name>.yaml with escaped templates
	for _, el := range elements {
		for _, s := range el.Values.Component.SubPolicies() {
			for _, f := range s.Files {
				path := filepath.Join(el.ConvertersDir, f+".yaml")
				data, _ := os.ReadFile(path)
				if checks.Escaped.Match(data) {
					rel, _ := filepath.Rel(root, path)
					covered[rel] = covered[rel] || rendered[el.ChartName+"/"+s.Name]
				}
			}
		}
	}
	var missed []string
	for conv, ok := range covered {
		if !ok {
			missed = append(missed, conv)
		}
	}
	slices.Sort(missed)
	for _, m := range missed {
		t.Errorf("%s is not evaluated: give it its config input in testdata/inputs.yaml, or a testdata/profiles file", m)
	}
	t.Logf("%d/%d converters with ACM templates evaluated", len(covered)-len(missed), len(covered))
}

// configPolicies returns the names of the ConfigurationPolicies in a render's Policies.
func configPolicies(t *testing.T, data []byte) []string {
	t.Helper()
	var out []string
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc struct {
			Kind string `yaml:"kind"`
			Spec struct {
				PolicyTemplates []struct {
					ObjectDefinition struct {
						Kind     string `yaml:"kind"`
						Metadata struct {
							Name string `yaml:"name"`
						} `yaml:"metadata"`
					} `yaml:"objectDefinition"`
				} `yaml:"policy-templates"`
			} `yaml:"spec"`
		}
		if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
			return out
		} else if err != nil {
			t.Fatal(err)
		}
		for _, pt := range doc.Spec.PolicyTemplates {
			if doc.Kind == "Policy" && pt.ObjectDefinition.Kind == "ConfigurationPolicy" {
				out = append(out, pt.ObjectDefinition.Metadata.Name)
			}
		}
	}
}
