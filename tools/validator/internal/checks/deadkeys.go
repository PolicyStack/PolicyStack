package checks

import (
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// DeadKeyCheck (POLICY031) reports values keys that policy-library never reads, and the 3.x element
// shape. An element using `enable:` renders nothing, and one using `defaultPolicy:` loses its
// compliance annotations.
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
	if comp.V3 {
		add(SevError, "policies, configPolicies and toggles have the policy-library 3.x shape, which 4.x rejects: run tools/migrate-4.0.py", "policies")
	}
	if comp.Default != nil && comp.Default.Disabled != nil {
		add(SevWarning, "`default.disabled` is not consumed by policy-library; set disabled on each policy instead", "default", "disabled")
	}
	return out
}
