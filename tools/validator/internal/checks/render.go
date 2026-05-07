package checks

// RenderCheck (RENDER000) is a sentinel check whose findings are produced
// by the runner when `helm template` fails. It has no Run() body of its own
// — it exists only so the rule ID is part of the registry.
type RenderCheck struct{}

func (RenderCheck) ID() string  { return "RENDER000" }
func (RenderCheck) Phase() Phase { return PhaseCluster }

func (c *RenderCheck) Run(_ Context) []Finding { return nil }
