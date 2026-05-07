package checks

import (
	"fmt"
	"sort"
	"strings"
)

// PinningCheck (POLICY060) flags policy-library version drift across
// elements. A repo-phase check — runs once with all elements available.
type PinningCheck struct{}

func (PinningCheck) ID() string  { return "POLICY060" }
func (PinningCheck) Phase() Phase { return PhaseRepo }

func (c *PinningCheck) Run(ctx Context) []Finding {
	if len(ctx.AllElements) < 2 {
		return nil
	}
	versions := map[string][]string{} // version -> elements at that version
	for _, el := range ctx.AllElements {
		for _, dep := range el.Dependencies {
			if dep.Name != "policy-library" {
				continue
			}
			versions[dep.Version] = append(versions[dep.Version], el.ChartName)
		}
	}
	if len(versions) <= 1 {
		return nil
	}
	keys := make([]string, 0, len(versions))
	for k := range versions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, strings.Join(versions[k], ",")))
	}
	var out []Finding
	for _, el := range ctx.AllElements {
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevWarning,
			Element:  el.ChartName,
			Message:  "policy-library version drift across elements: " + strings.Join(parts, "; "),
			File:     el.Dir + "/Chart.yaml",
			Line:     1,
		})
	}
	return out
}
