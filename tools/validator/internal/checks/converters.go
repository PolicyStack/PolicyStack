package checks

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// UnusedConverterCheck (POLICY021) warns about a converter file policy-library never renders: a .yml
// file, or a .yaml file no template renders, such as a top-level file no templates.<t>.file names
// or converters/<p>/<t>.yaml replaced by a file: of its own. A partial, a path with a segment
// starting with _, also counts as used when another chart file names it, as lifecycle's
// templates/_helpers.tpl reads converters/_<engine>/root.yaml.
type UnusedConverterCheck struct{}

func (UnusedConverterCheck) ID() string   { return "POLICY021" }
func (UnusedConverterCheck) Phase() Phase { return PhaseChart }

func (c *UnusedConverterCheck) Run(ctx Context) []Finding {
	el := ctx.Element
	if el == nil || el.Values == nil || el.Values.Component == nil {
		return nil
	}
	// Every component's templates count: the lifecycle chart's engines share converters/.
	used := map[string]bool{}
	for _, nc := range el.Components {
		for _, s := range nc.Component.SubPolicies() {
			for _, f := range s.Files {
				used[f+".yaml"] = true
			}
		}
	}
	text := map[string][]byte{} // the chart's templates and converters, which may read a partial
	chartFS := os.DirFS(el.Dir)
	for _, dir := range []string{"templates", "converters"} {
		_ = fs.WalkDir(chartFS, dir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if data, err := fs.ReadFile(chartFS, path); err == nil {
					text[filepath.Join(el.Dir, path)] = data
				}
			}
			return nil
		})
	}

	var out []Finding
	for _, rel := range converterFiles(el.ConvertersDir) {
		why := ""
		switch {
		case strings.HasSuffix(rel, ".yml"):
			why = "policy-library renders only .yaml converters"
		case used[rel]:
			continue
		case partial(rel):
			if readBy(text, filepath.Join(el.ConvertersDir, rel)) {
				continue
			}
			why = "no templates.<t>.file names this partial and no chart file reads it"
		default:
			why = "it is not converters/<policy>/<template>.yaml of a template and no templates.<t>.file names it"
		}
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevWarning,
			Element:  el.ChartName,
			Message:  fmt.Sprintf("converters/%s is never rendered: %s", rel, why),
			File:     filepath.Join(el.ConvertersDir, rel),
			Line:     1,
		})
	}
	return out
}

// readBy reports whether a file in text other than path names path's base name.
func readBy(text map[string][]byte, path string) bool {
	name := []byte(filepath.Base(path))
	for p, data := range text {
		if p != path && bytes.Contains(data, name) {
			return true
		}
	}
	return false
}

// converterFiles returns every .yaml and .yml file under dir, by slash path relative to dir, sorted.
func converterFiles(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if ext := filepath.Ext(path); ext == ".yaml" || ext == ".yml" {
			rel, _ := filepath.Rel(dir, path)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(out)
	return out
}

// partial reports a converter path with a segment starting with _, which policy-library never
// discovers.
func partial(rel string) bool {
	return slices.ContainsFunc(strings.Split(rel, "/"), func(s string) bool { return strings.HasPrefix(s, "_") })
}
