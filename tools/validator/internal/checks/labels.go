package checks

import "fmt"

// LabelCheck (POLICY050) surfaces label issues collected by the cascade
// resolver as findings, pinned to the fixture file.
type LabelCheck struct{}

func (LabelCheck) ID() string  { return "POLICY050" }
func (LabelCheck) Phase() Phase { return PhaseCluster }

func (c *LabelCheck) Run(ctx Context) []Finding {
	if ctx.Cluster == nil || len(ctx.Cluster.LabelIssues) == 0 {
		return nil
	}
	var out []Finding
	for _, li := range ctx.Cluster.LabelIssues {
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevError,
			Cluster:  ctx.Cluster.ClusterName,
			Element:  elementName(ctx),
			Message:  fmt.Sprintf("label %q: %s", li.Key, li.Reason),
			// Fixture file path is filled in by the runner when it knows it.
		})
	}
	return out
}

func elementName(ctx Context) string {
	if ctx.Element != nil {
		return ctx.Element.ChartName
	}
	return ""
}
