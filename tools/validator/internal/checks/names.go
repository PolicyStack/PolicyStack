package checks

import (
	"fmt"
	"path/filepath"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// MaxPolicyNameLen is the k8s DNS-1123 label limit applied to
// `<policyNamespace>.<policy-name>`. ACM rejects anything longer.
const MaxPolicyNameLen = 63

// NameLengthCheck (POLICY001) enforces the 63-char limit on every name policy-library rendered,
// layer overrides included:
//
//	Policy, PolicySet   <ns>.<name>-<release>, the name ACM replicates it under
//	policy-templates    metadata.name, <parent>-<sub>
//
// ACM enforces the DNS-1123 label limit on the replicated name on each managed cluster, and on the
// templates it creates there.
type NameLengthCheck struct{}

func (NameLengthCheck) ID() string   { return "POLICY001" }
func (NameLengthCheck) Phase() Phase { return PhaseCluster }

func (c *NameLengthCheck) Run(ctx Context) []Finding {
	if len(ctx.Rendered) == 0 || ctx.Element == nil || ctx.Cluster == nil {
		return nil
	}
	pols, bad := policies(ctx, c.ID())
	if bad != nil {
		return bad
	}
	var out []Finding
	// check reports name, which kind/obj is created under, over the limit.
	check := func(kind, obj, label, name string) {
		if len(name) <= MaxPolicyNameLen {
			return
		}
		where, file, loc := origin(ctx, kind, obj, "name")
		out = append(out, Finding{
			RuleID: c.ID(), Severity: SevError,
			Element: ctx.Element.ChartName, Cluster: ctx.Cluster.ClusterName,
			Message: fmt.Sprintf("%s: %s %s %q is %d chars > %d", where, kind, label, name, len(name), MaxPolicyNameLen),
			File:    file, Line: loc.Line, Col: loc.Col,
		})
	}
	for _, p := range pols {
		check(p.Kind, p.Metadata.Name, "replicated name", p.Metadata.Namespace+"."+p.Metadata.Name)
		for _, t := range p.Spec.PolicyTemplates {
			od := t.ObjectDefinition
			check(od.Kind, od.Metadata.Name, "metadata.name", od.Metadata.Name)
		}
	}
	return out
}

// DuplicateNameCheck (POLICY002) reports policy-templates within one element that render to the same
// metadata.name, such as template b-c of policy a and template c of policy a-b. Cross-element
// collisions are reported by run.Run (parent policies, POLICY002) and by POLICY003 (sub-policy
// templates).
type DuplicateNameCheck struct{}

func (DuplicateNameCheck) ID() string   { return "POLICY002" }
func (DuplicateNameCheck) Phase() Phase { return PhaseCluster }

func (c *DuplicateNameCheck) Run(ctx Context) []Finding {
	if ctx.Element == nil || ctx.Cluster == nil || ctx.Element.Values == nil || ctx.Element.Values.Component == nil {
		return nil
	}
	seen := map[string]string{}
	var out []Finding
	for _, s := range ctx.Element.Values.Component.SubPolicies() {
		prior, ok := seen[s.Name]
		if !ok {
			seen[s.Name] = s.Where()
			continue
		}
		file, loc := ctx.Element.ValuesFile, sourceloc.Find(ctx.Element.ValuesDoc, append([]string{"stack", ctx.Element.StackKey}, s.Path...)...)
		if loc.Line == 0 && len(s.Files) > 0 { // discovered from converters/, absent from values.yaml
			file, loc = filepath.Join(ctx.Element.ConvertersDir, s.Files[0]+".yaml"), sourceloc.Loc{Line: 1}
		}
		out = append(out, Finding{
			RuleID:   "POLICY002",
			Severity: SevError,
			Element:  ctx.Element.ChartName,
			Cluster:  ctx.Cluster.ClusterName,
			Message:  fmt.Sprintf("rendered name %q from %s collides with prior %s in same element", s.Name, s.Where(), prior),
			File:     file,
			Line:     loc.Line,
			Col:      loc.Col,
		})
	}
	return out
}
