package checks

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// policyObject is the part of a rendered Policy or PolicySet the render-phase rules read. They
// check what Helm rendered from the whole cascade, so a values layer cannot bypass them.
type policyObject struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		RemediationAction string           `json:"remediationAction"`
		Dependencies      []dependency     `json:"dependencies"`
		PolicyTemplates   []policyTemplate `json:"policy-templates"`
	} `json:"spec"`
}

type dependency struct {
	Name string `json:"name"`
}

type policyTemplate struct {
	ExtraDependencies []dependency `json:"extraDependencies"`
	ObjectDefinition  struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Name        string            `json:"name"`
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
		Spec struct {
			RemediationAction   string           `json:"remediationAction"`
			PruneObjectBehavior string           `json:"pruneObjectBehavior"`
			Raw                 string           `json:"object-templates-raw"`
			ObjectTemplates     []objectTemplate `json:"object-templates"`
		} `json:"spec"`
	} `json:"objectDefinition"`
}

type objectTemplate struct {
	ComplianceType   string `json:"complianceType"`
	ObjectDefinition struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Metadata   struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	} `json:"objectDefinition"`
}

// policies decodes the Policies and PolicySets in ctx.Rendered. An undecodable render is returned
// as the single finding of rule id.
func policies(ctx Context, id string) ([]policyObject, []Finding) {
	var out []policyObject
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(ctx.Rendered), 4096)
	for {
		var p policyObject
		if err := dec.Decode(&p); err == io.EOF {
			return out, nil
		} else if err != nil {
			return nil, []Finding{{RuleID: id, Severity: SevError, Element: ctx.Element.ChartName,
				Message: "rendered output: " + err.Error(), File: ctx.Element.ValuesFile}}
		}
		if p.Kind == "Policy" || p.Kind == "PolicySet" {
			out = append(out, p)
		}
	}
}

// action is the remediationAction ACM runs template with under p: the Policy's, unless the
// template's is InformOnly.
func (p policyObject) action(template string) string {
	if p.Spec.RemediationAction != "" && !informOnly(template) {
		return p.Spec.RemediationAction
	}
	return template
}

// informOnly reports a template action ACM keeps as inform under any Policy-level action. ACM
// matches it case-insensitively.
func informOnly(action string) bool { return strings.EqualFold(action, "informonly") }

// origin locates the values entry that renders the object kind/name for ctx.Cluster, in the last
// cascade file defining it: Helm replaces lists whole, so that file holds the entry. It returns the
// entry's description, the file and the location of field, else of the entry. Helm merges maps, so a
// later file setting field on a map entry, such as operatorPolicies.<key>, takes the finding. An
// object no entry renders is described by kind and name with no file, which run.Run reports at the
// fleet file.
func origin(ctx Context, kind, name, field string) (string, string, sourceloc.Loc) {
	el := ctx.Element
	type layer struct {
		file string
		doc  *yaml.Node
	}
	var later []layer // nearest-last order: the last cascade file first
	for _, f := range slices.Backward(ctx.Cluster.ValueFiles) {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var doc yaml.Node
		var comp chart.Component
		if yaml.Unmarshal(data, &doc) != nil {
			continue
		}
		if n := sourceloc.FindNode(&doc, "stack", el.StackKey); n == nil || n.Decode(&comp) != nil {
			continue
		}
		if where, path := entry(&comp, el, ctx.Cluster.ReleaseName, kind, name); path != nil {
			path = append([]string{"stack", el.StackKey}, path...)
			for _, l := range later {
				if loc := sourceloc.Find(l.doc, append(path, field)...); loc.Line != 0 {
					return where, l.file, loc
				}
			}
			loc := sourceloc.Find(&doc, append(path, field)...)
			if loc.Line == 0 {
				loc = sourceloc.Find(&doc, path...)
			}
			return where, f, loc
		}
		later = append(later, layer{f, &doc})
	}
	return fmt.Sprintf("%s %q", kind, name), "", sourceloc.Loc{}
}

// dryRun reports whether the cascade sets global.remediationAction, which policy-library renders as
// every Policy's remediationAction in place of its own (docs/rollout.md#dry-run).
func dryRun(ctx Context) bool {
	for _, f := range ctx.Cluster.ValueFiles {
		var v struct {
			Global struct {
				RemediationAction string `yaml:"remediationAction"`
			} `yaml:"global"`
		}
		if data, err := os.ReadFile(f); err == nil && yaml.Unmarshal(data, &v) == nil && v.Global.RemediationAction != "" {
			return true
		}
	}
	return false
}
