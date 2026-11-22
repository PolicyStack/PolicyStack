// Package compat finds what a change does to output for unchanged values, which the compatibility
// contract freezes within a major (docs/reference/compatibility.md#unchanged-values). It renders the
// last release and the change with the release's values and fleet fixtures. Upstream only.
package compat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/run"
)

// testdata is where each tree keeps the validator's test inputs.
const testdata = "tools/validator/testdata"

// Change is one rendered file that differs from the last release's, and from the base branch's: a
// change already approved on the base, or one restoring the release's output, is not a Change.
type Change struct {
	// Path is <scenario>/<fleet file without .yaml>/<chart>.yaml, or install/<chart>.yaml.
	Path string `json:"path"`
	// Release and After are the renders, empty when the chart renders nothing.
	Release []byte `json:"release"`
	After   []byte `json:"after"`
}

// Report is what Diff found.
type Report struct {
	Changes []Change `json:"changes"`
	// Label approves exactly these changes: any other difference has another label.
	Label    string `json:"label"`
	Approved bool   `json:"approved"`
}

// Trees are the checkouts Diff renders.
type Trees struct {
	// Release is the last release, Base the branch the change lands on and Head the change. An
	// empty Base compares with Release alone.
	Release, Base, Head string
}

// Diff renders the trees with Release's values and returns the changes in Head.
func Diff(ctx context.Context, helm string, t Trees) (*Report, error) {
	work, err := os.MkdirTemp("", "compat-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	scenarios, err := scenarios(work, t)
	if err != nil {
		return nil, err
	}
	renders := func(tree string) (map[string][]byte, error) {
		out, err := renderTree(ctx, helm, work, tree, t.Release, scenarios)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", tree, err)
		}
		return out, nil
	}
	release, err := renders(t.Release)
	if err != nil {
		return nil, err
	}
	head, err := renders(t.Head)
	if err != nil {
		return nil, err
	}
	differ, err := differing(release, head)
	if err != nil || len(differ) == 0 || t.Base == "" {
		return newReport(differ, release, head, err)
	}
	base, err := renders(t.Base)
	if err != nil {
		return nil, err
	}
	var unapproved []string
	for _, p := range differ {
		// Only what the base renders is approved: a failed render there approves nothing.
		ok, err := same(base[p], head[p])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if !ok {
			unapproved = append(unapproved, p)
		}
	}
	return newReport(unapproved, release, head, nil)
}

// differing returns, sorted, the paths whose render in after means something else than in before. A
// render that failed had no behaviour to keep.
func differing(before, after map[string][]byte) ([]string, error) {
	all := maps.Clone(before)
	maps.Copy(all, after)
	var out []string
	for _, p := range slices.Sorted(maps.Keys(all)) {
		if failed(before[p]) {
			continue
		}
		ok, err := same(before[p], after[p])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if !ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// same reports whether two renders mean the same thing. Two failed ones do.
func same(x, y []byte) (bool, error) {
	if failed(x) || failed(y) {
		return failed(x) && failed(y), nil
	}
	a, err := Canonical(x)
	if err != nil {
		return false, err
	}
	b, err := Canonical(y)
	return bytes.Equal(a, b), err
}

func newReport(paths []string, release, head map[string][]byte, err error) (*Report, error) {
	if err != nil {
		return nil, err
	}
	r := &Report{}
	h := sha256.New()
	for _, p := range paths {
		after := []byte("render failed") // the message holds this machine's paths
		if !failed(head[p]) {
			if after, err = Canonical(head[p]); err != nil {
				return nil, err
			}
		}
		_, _ = fmt.Fprintf(h, "%s\x00%s\x00", p, after)
		r.Changes = append(r.Changes, Change{Path: p, Release: release[p], After: head[p]})
	}
	if len(paths) > 0 {
		r.Label = "output-change-" + hex.EncodeToString(h.Sum(nil))[:12]
	}
	return r, nil
}

// Approve records whether one of labels is r's.
func (r *Report) Approve(labels []string) {
	r.Approved = r.Label != "" && slices.Contains(labels, r.Label)
}

func failed(render []byte) bool { return bytes.HasPrefix(render, []byte("# RENDER000")) }

// Canonical is what a render means: its objects as JSON lines sorted by text, without formatting,
// comments or description annotations, none of which ACM acts on.
func Canonical(rendered []byte) ([]byte, error) {
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	var objects []string
	for {
		var o map[string]any
		if err := dec.Decode(&o); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		if len(o) == 0 {
			continue
		}
		dropDescription(o)
		if spec, ok := o["spec"].(map[string]any); ok {
			templates, _ := spec["policy-templates"].([]any)
			for _, t := range templates {
				if t, ok := t.(map[string]any); ok {
					if od, ok := t["objectDefinition"].(map[string]any); ok {
						dropDescription(od)
					}
				}
			}
		}
		line, err := json.Marshal(o)
		if err != nil {
			return nil, err
		}
		objects = append(objects, string(line))
	}
	slices.Sort(objects)
	return []byte(strings.Join(objects, "\n")), nil
}

func dropDescription(o map[string]any) {
	md, _ := o["metadata"].(map[string]any)
	annotations, ok := md["annotations"].(map[string]any)
	if !ok {
		return
	}
	delete(annotations, "description")
	if len(annotations) == 0 {
		delete(md, "annotations")
	}
}

// scenarios returns the values files each render run layers over the fixtures' cascade, by name:
// every element on with its defaults, the same with the templated values, the release's all-on
// profile, and its other profiles. All but the templated values are the release's own: those are
// the release's, then the change's, so a change can add to them but not drop them.
func scenarios(work string, t Trees) (map[string][]string, error) {
	data := filepath.Join(t.Release, testdata)
	elements, _, err := chart.LoadAll(filepath.Join(t.Release, "stack"), "", filepath.Join(t.Release, "values.yaml"))
	if err != nil {
		return nil, err
	}
	allOn, err := chart.AllOn(elements)
	if err != nil {
		return nil, err
	}
	stack := map[string]any{}
	for _, el := range elements {
		if el.StackKey != "" {
			stack[el.StackKey] = map[string]any{"enabled": true}
		}
	}
	on, err := yaml.Marshal(map[string]any{"stack": stack})
	if err != nil {
		return nil, err
	}
	onFile, allFile := filepath.Join(work, "on.yaml"), filepath.Join(work, "all-on.yaml")
	if err := errors.Join(os.WriteFile(onFile, on, 0o600), os.WriteFile(allFile, allOn, 0o600)); err != nil {
		return nil, err
	}
	baseline := filepath.Join(data, "baseline.yaml")
	out := map[string][]string{
		"defaults": {baseline, onFile},
		"all-on":   {baseline, allFile, filepath.Join(data, "inputs.yaml")},
	}
	templated := []string{baseline, onFile}
	for _, tree := range []string{t.Release, t.Head} {
		file := filepath.Join(tree, testdata, "compat", "values.yaml")
		if _, err := os.Stat(file); err == nil {
			templated = append(templated, file)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	if len(templated) > 2 {
		out["templated"] = templated
	}
	profiles, err := filepath.Glob(filepath.Join(data, "profiles", "*.yaml"))
	if err != nil {
		return nil, err
	}
	for _, p := range profiles {
		out[strings.TrimSuffix(filepath.Base(p), ".yaml")] = []string{baseline, p}
	}
	return out, nil
}

// renderTree renders tree's elements, lifecycle and install charts by Change path.
func renderTree(ctx context.Context, helm, work, tree, release string, scenarios map[string][]string) (map[string][]byte, error) {
	dir, err := os.MkdirTemp(work, "render-")
	if err != nil {
		return nil, err
	}
	for _, name := range slices.Sorted(maps.Keys(scenarios)) {
		if _, err := run.Run(ctx, run.Options{
			RepoRoot:     tree,
			StackDir:     filepath.Join(tree, "stack"),
			LifecycleDir: filepath.Join(tree, "lifecycle"),
			ValuesDir:    filepath.Join(release, "values"),
			FixturesDir:  filepath.Join(release, testdata, "clusters"),
			HelmBin:      helm,
			ExtraValues:  scenarios[name],
			Only:         []string{"RENDER000"},
			RenderDir:    filepath.Join(dir, name),
			Logger:       slog.New(slog.DiscardHandler),
		}); err != nil {
			return nil, err
		}
	}
	out := map[string][]byte{}
	if err := fs.WalkDir(os.DirFS(dir), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		out[p], err = os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		return err
	}); err != nil {
		return nil, err
	}

	runner := render.New(helm)
	for _, c := range []struct {
		dir    string
		values []string
		set    []string
	}{
		{dir: "install", set: []string{"hubName=hub", "gitRepo=https://git.example/fleet.git"}},
		{dir: "install/operators", values: []string{"stack/acm/values.yaml", "stack/gitops/values.yaml"}},
	} {
		if _, err := os.Stat(filepath.Join(tree, c.dir, "Chart.yaml")); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		var files []string
		for _, v := range c.values {
			files = append(files, filepath.Join(tree, v))
		}
		res, key := runner.Template(ctx, filepath.Join(tree, c.dir), "policystack", files, c.set), path.Join("install", path.Base(c.dir)+".yaml")
		out[key] = res.Stdout
		if res.Err != nil {
			out[key] = []byte("# RENDER000: " + res.Stderr + "\n")
		}
	}
	return out, nil
}
