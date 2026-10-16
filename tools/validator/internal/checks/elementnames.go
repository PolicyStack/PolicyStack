package checks

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

// ElementNameCheck (POLICY091) rejects an element name equal to another's, or
// starting with another's followed by "-". Release names join element and
// cluster with "-", so "a" on cluster "b-c" and "a-b" on cluster "c" would both
// be "a-b-c": one Application and the same Policy names. lifecycle/ takes part
// as "lifecycle", which reserves that name.
type ElementNameCheck struct{}

func (ElementNameCheck) ID() string   { return "POLICY091" }
func (ElementNameCheck) Phase() Phase { return PhaseRepo }

func (c *ElementNameCheck) Run(ctx Context) []Finding {
	// The lifecycle chart's components share one directory.
	var els []*chart.Element
	for _, el := range ctx.AllElements {
		if !slices.ContainsFunc(els, func(e *chart.Element) bool { return e.Dir == el.Dir }) {
			els = append(els, el)
		}
	}
	var out []Finding
	for _, a := range els {
		for _, b := range els {
			var msg string
			switch {
			case a.ChartName == b.ChartName && a.Dir < b.Dir:
				msg = fmt.Sprintf("element name %q is also the chart name in %s", b.ChartName, filepath.Base(a.Dir))
			case strings.HasPrefix(b.ChartName, a.ChartName+"-"):
				rest := strings.TrimPrefix(b.ChartName, a.ChartName+"-")
				msg = fmt.Sprintf("element name %q starts with element name %q and \"-\": %s on cluster \"c\" and %s on cluster \"%s-c\" share the release name %s-c",
					b.ChartName, a.ChartName, b.ChartName, a.ChartName, rest, b.ChartName)
			default:
				continue
			}
			out = append(out, Finding{RuleID: c.ID(), Severity: SevError, Element: b.ChartName, Message: msg, File: b.Dir + "/Chart.yaml", Line: 1})
		}
	}
	return out
}
