package checks

// LabelCheck (POLICY050) reports invalid fleet files: the issues found by
// fixtures.LoadDir and cascade.Resolve, against the fleet file.
type LabelCheck struct{}

func (LabelCheck) ID() string   { return "POLICY050" }
func (LabelCheck) Phase() Phase { return PhaseCluster }

func (c *LabelCheck) Run(ctx Context) []Finding {
	if ctx.Cluster == nil || len(ctx.Cluster.Issues) == 0 {
		return nil
	}
	var out []Finding
	for _, is := range ctx.Cluster.Issues {
		out = append(out, Finding{
			RuleID:   c.ID(),
			Severity: SevError,
			Cluster:  ctx.Cluster.ClusterName,
			Element:  elementName(ctx),
			Message:  "invalid fleet file: " + is.Message,
			Line:     is.Line,
			// run.Run sets File to the fleet file path.
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
