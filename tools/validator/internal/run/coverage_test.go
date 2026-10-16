package run

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

// TestProfileCoverage renders every element for every fixture under each testdata/profiles file,
// as the POLICY081 CI step does, and fails for a converter with escaped ACM templates that no
// render includes.
func TestProfileCoverage(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not on PATH")
	}
	root, _ := filepath.Abs("../../../..")
	testdata := filepath.Join(root, "tools/validator/testdata")
	profiles, _ := filepath.Glob(filepath.Join(testdata, "profiles/*.yaml"))
	if len(profiles) == 0 {
		t.Fatal("no profiles in testdata/profiles")
	}

	rendered := map[string]bool{} // <chart>/<ConfigurationPolicy>
	for _, p := range profiles {
		dir := t.TempDir()
		res, err := Run(t.Context(), Options{
			RepoRoot:     root,
			StackDir:     filepath.Join(root, "stack"),
			LifecycleDir: dir, // no lifecycle chart: its converters have their own tests
			ValuesDir:    filepath.Join(root, "values"),
			FixturesDir:  filepath.Join(testdata, "clusters"),
			HelmBin:      helm,
			ExtraValues:  []string{filepath.Join(testdata, "baseline.yaml"), p},
			Only:         []string{"RENDER000"},
			RenderDir:    dir,
			Logger:       slog.New(slog.DiscardHandler),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range res.Findings {
			t.Errorf("%s: %s does not render for %s", filepath.Base(p), f.Element, f.Cluster)
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
		for _, cp := range el.Values.Component.ConfigPolicies {
			for _, tn := range cp.TemplateNames {
				path := filepath.Join(el.ConvertersDir, tn.Name+".yaml")
				data, _ := os.ReadFile(path)
				if bytes.Contains(data, []byte(`{{ "{{" }}`)) {
					rel, _ := filepath.Rel(root, path)
					covered[rel] = covered[rel] || rendered[el.ChartName+"/"+cp.PolicyRef+"-"+cp.Name]
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
		t.Errorf("%s is not evaluated: enable its element and toggle in a testdata/profiles file", m)
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
