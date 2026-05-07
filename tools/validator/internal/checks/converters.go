package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// MissingConverterCheck (POLICY020) flags every templateNames[].name that
// has no matching converters/<name>.yaml on disk.
type MissingConverterCheck struct{}

func (MissingConverterCheck) ID() string  { return "POLICY020" }
func (MissingConverterCheck) Phase() Phase { return PhaseChart }

func (c *MissingConverterCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	actual := actualConverters(ctx.Element.ConvertersDir)

	var out []Finding
	for i, cp := range ctx.Element.Values.Component.ConfigPolicies {
		for j, tn := range cp.TemplateNames {
			if tn.Name == "" {
				continue
			}
			if _, ok := actual[tn.Name]; ok {
				continue
			}
			loc := sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey,
				"configPolicies", strconv.Itoa(i), "templateNames", strconv.Itoa(j), "name")
			// Fall back to the templateNames node itself for bare-string form.
			if loc.Line == 0 {
				loc = sourceloc.Find(ctx.Element.ValuesDoc, "stack", ctx.Element.StackKey,
					"configPolicies", strconv.Itoa(i), "templateNames", strconv.Itoa(j))
			}
			out = append(out, Finding{
				RuleID:   c.ID(),
				Severity: SevError,
				Element:  ctx.Element.ChartName,
				Message: fmt.Sprintf("configPolicies[%d] %q references templateNames[%d] %q but converters/%s.yaml does not exist",
					i, cp.Name, j, tn.Name, tn.Name),
				File: ctx.Element.ValuesFile,
				Line: loc.Line, Col: loc.Col,
			})
		}
	}
	return out
}

// UnusedConverterCheck (POLICY021) warns when a converter file is on disk
// but no templateNames[].name references it. Warning only — dead files are
// noise, not breakage.
type UnusedConverterCheck struct{}

func (UnusedConverterCheck) ID() string  { return "POLICY021" }
func (UnusedConverterCheck) Phase() Phase { return PhaseChart }

func (c *UnusedConverterCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	actual := actualConverters(ctx.Element.ConvertersDir)
	if len(actual) == 0 {
		return nil
	}

	referenced := map[string]struct{}{}
	for _, cp := range ctx.Element.Values.Component.ConfigPolicies {
		for _, tn := range cp.TemplateNames {
			if tn.Name != "" {
				referenced[tn.Name] = struct{}{}
			}
		}
	}

	var unused []string
	for name := range actual {
		if _, ok := referenced[name]; !ok {
			unused = append(unused, name)
		}
	}
	sort.Strings(unused)
	var out []Finding
	for _, name := range unused {
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevWarning,
			Element:  ctx.Element.ChartName,
			Message:  fmt.Sprintf("converters/%s.yaml is not referenced by any templateNames[].name", name),
			File:     filepath.Join(ctx.Element.ConvertersDir, name+".yaml"),
			Line:     1,
		})
	}
	return out
}

func actualConverters(dir string) map[string]struct{} {
	out := map[string]struct{}{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		switch {
		case strings.HasSuffix(n, ".yaml"):
			out[strings.TrimSuffix(n, ".yaml")] = struct{}{}
		case strings.HasSuffix(n, ".yml"):
			out[strings.TrimSuffix(n, ".yml")] = struct{}{}
		}
	}
	return out
}
