package checks

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/acmtemplates"
)

// ACMTemplateCheck (POLICY081) resolves the ACM templates converters escape past Helm, hub then
// managed, as ACM would on an empty cluster (acmtemplates.Evaluate). Findings name the sub-policy
// without a cluster suffix, carry no cluster and point at the converter, so one broken template is
// one finding however many clusters render it.
type ACMTemplateCheck struct{}

func (ACMTemplateCheck) ID() string   { return "POLICY081" }
func (ACMTemplateCheck) Phase() Phase { return PhaseCluster }

func (c *ACMTemplateCheck) Run(ctx Context) []Finding {
	if len(ctx.Rendered) == 0 || ctx.Element == nil {
		return nil
	}
	failures, err := acmtemplates.Evaluate(ctx.Rendered, clusterName(ctx))
	if err != nil {
		return []Finding{{RuleID: c.ID(), Severity: SevError, Element: ctx.Element.ChartName,
			Message: "rendered output: " + err.Error(), File: ctx.Element.ValuesFile}}
	}
	out := make([]Finding, 0, len(failures))
	for _, f := range failures {
		pass := "managed"
		if f.Hub {
			pass = "hub"
		}
		name, converters := subPolicy(ctx, f.ConfigPolicy)
		file, line := locate(ctx.Element.ConvertersDir, converters, f)
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevError,
			Element:  ctx.Element.ChartName,
			Message:  fmt.Sprintf("%s (%s template): %s", name, pass, f.Err),
			File:     cmp.Or(file, ctx.Element.ValuesFile),
			Line:     line,
		})
	}
	return out
}

// subPolicy returns the configPolicies entry rendered as name, "<policyRef>-<name>" plus the cluster
// under suffixTemplateNames, by that unsuffixed name and its templateNames. An unknown name is
// returned as is.
func subPolicy(ctx Context, name string) (string, []string) {
	if ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return name, nil
	}
	suffix := ""
	if ctx.Cluster != nil {
		suffix, _ = strings.CutPrefix(ctx.Cluster.ReleaseName, ctx.Element.ChartName+"-")
	}
	for _, cp := range ctx.Element.Values.Component.ConfigPolicies {
		if base := cp.PolicyRef + "-" + cp.Name; name == base || name == base+"-"+suffix {
			out := make([]string, len(cp.TemplateNames))
			for i, tn := range cp.TemplateNames {
				out[i] = tn.Name
			}
			return base, out
		}
	}
	return name, nil
}

var (
	undefinedFunc = regexp.MustCompile(`function "([^"]+)" not defined`)
	escapedHub    = regexp.MustCompile(`\{\{ "\{\{-?" \}\}\s*hub`)
	escaped       = regexp.MustCompile(`\{\{ "\{\{-?" \}\}`)
)

// locate returns the converter behind f and the line of its first template of f's pass, or for an
// undefined function the line naming it.
func locate(dir string, converters []string, f acmtemplates.Failure) (string, int) {
	if f.Template >= 0 {
		if f.Template >= len(converters) {
			return "", 0
		}
		converters = converters[f.Template : f.Template+1]
	}
	want := escaped
	if f.Hub {
		want = escapedHub
	}
	if m := undefinedFunc.FindStringSubmatch(f.Err); m != nil {
		want = regexp.MustCompile(`\b` + regexp.QuoteMeta(m[1]) + `\b`)
	}
	for _, n := range converters {
		path := filepath.Join(dir, n+".yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for i, line := range bytes.Split(data, []byte("\n")) {
			if want.Match(line) {
				return path, i + 1
			}
		}
	}
	return "", 0
}
