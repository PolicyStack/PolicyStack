package checks

import (
	"fmt"
	"strconv"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// DeadKeyCheck (POLICY031) reports values keys that policy-library never reads. `helm template`
// succeeds either way: an element using `enable:` renders nothing, and one using `defaultPolicy:`
// loses its compliance annotations.
type DeadKeyCheck struct{}

func (DeadKeyCheck) ID() string   { return "POLICY031" }
func (DeadKeyCheck) Phase() Phase { return PhaseChart }

func (c *DeadKeyCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	comp := ctx.Element.Values.Component
	key := ctx.Element.StackKey

	var out []Finding
	add := func(sev Severity, msg string, path ...string) {
		loc := sourceloc.Find(ctx.Element.ValuesDoc, append([]string{"stack", key}, path...)...)
		out = append(out, Finding{
			RuleID: c.ID(), Severity: sev,
			Element: ctx.Element.ChartName,
			Message: msg,
			File:    ctx.Element.ValuesFile, Line: loc.Line, Col: loc.Col,
		})
	}

	if comp.LegacyEnable != nil {
		add(SevError, "`enable:` is never read by policy-library - the element renders nothing. Rename it to `enabled:`", "enable")
	}
	if comp.LegacyDefaultPolicy != nil {
		add(SevError, "`defaultPolicy:` is never read by policy-library - its categories/controls/standards are silently dropped. Rename it to `default:`", "defaultPolicy")
	}
	if comp.LegacyOperatorPolicies {
		add(SevError, legacyOperatorPolicies, "operatorPolicies")
	}
	if comp.Default != nil {
		// policy-library consumes only categories/controls/standards from `default`.
		if comp.Default.Severity != "" {
			add(SevWarning, "`default.severity` is not consumed by policy-library; set severity on each policy instead", "default", "severity")
		}
		if comp.Default.RemediationAction != "" {
			add(SevWarning, "`default.remediationAction` is not consumed by policy-library; set remediationAction on each policy instead", "default", "remediationAction")
		}
		if comp.Default.Disabled != nil {
			add(SevWarning, "`default.disabled` is not consumed by policy-library; set disabled on each policy instead", "default", "disabled")
		}
	}
	for i, p := range comp.ConfigPolicies {
		if p.RawTemplate && len(p.TemplateNames) != 1 {
			add(SevError, fmt.Sprintf("configPolicies[%d] %q: rawTemplate maps to the single-valued object-templates-raw field, so it needs exactly one templateNames entry (got %d)", i, p.Name, len(p.TemplateNames)),
				"configPolicies", strconv.Itoa(i), "rawTemplate")
		}
	}
	return out
}
