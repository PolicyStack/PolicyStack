package checks

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// PlacementCheck (POLICY041) reports a rendered Placement with more than one predicate. ACM ORs
// predicates, so each one beside the cluster selector policy-library renders sends this cluster's
// Policies to other clusters, clusters pinned to another revision included.
type PlacementCheck struct{}

func (PlacementCheck) ID() string   { return "POLICY041" }
func (PlacementCheck) Phase() Phase { return PhaseCluster }

func (c *PlacementCheck) Run(ctx Context) []Finding {
	if len(ctx.Rendered) == 0 || ctx.Element == nil || ctx.Cluster == nil {
		return nil
	}
	var out []Finding
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(ctx.Rendered), 4096)
	for {
		var p struct {
			Kind string `json:"kind"`
			Spec struct {
				Predicates []any `json:"predicates"`
			} `json:"spec"`
		}
		if err := dec.Decode(&p); err == io.EOF {
			return out
		} else if err != nil {
			return []Finding{{RuleID: c.ID(), Severity: SevError, Element: ctx.Element.ChartName,
				Message: "rendered output: " + err.Error(), File: ctx.Element.ValuesFile}}
		}
		if p.Kind != "Placement" || len(p.Spec.Predicates) < 2 {
			continue
		}
		file, loc := lastSetting(ctx.Cluster.ValueFiles, "placement", "predicates")
		element := ctx.Element.ChartName
		if file != "" {
			element = "" // one mistake in the layer, not one per element that reads it
		}
		out = append(out, Finding{RuleID: c.ID(), Severity: SevError, Element: element,
			Message: fmt.Sprintf("the Placement renders %d predicates, which ACM ORs: the Policies reach every cluster any of them selects, whatever that cluster pins. "+
				"Remove placement.predicates; to narrow, use placement.claimSelector or celSelector", len(p.Spec.Predicates)),
			File: file, Line: loc.Line, Col: loc.Col})
	}
}

// lastSetting returns the last of files that sets path, with its location, or no file when none does.
func lastSetting(files []string, path ...string) (string, sourceloc.Loc) {
	for _, f := range slices.Backward(files) {
		data, err := os.ReadFile(f)
		var doc yaml.Node
		if err != nil || yaml.Unmarshal(data, &doc) != nil {
			continue
		}
		if loc := sourceloc.Find(&doc, path...); loc.Line != 0 {
			return f, loc
		}
	}
	return "", sourceloc.Loc{}
}
