package checks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utiljson "k8s.io/apimachinery/pkg/util/json"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

// CRDSchemaCheck (POLICY080) validates rendered objects against the CRDs ACM serves them with, the
// way the API server does on apply: unknown fields are pruned and reported, defaults applied, then
// the OpenAPI schema checked. Pass 1 checks every rendered object, pass 2 every
// policy-templates[].objectDefinition as the framework addon creates it on the managed cluster,
// InformOnly normalized to inform. Findings carry no cluster and name the values entry, so one
// defect is one finding. Kinds without a CRD are one warning per render. CRDs is set by
// run.buildRegistry; the check does nothing without it.
type CRDSchemaCheck struct {
	CRDs      CRDs
	validated atomic.Int64
}

func (*CRDSchemaCheck) ID() string   { return "POLICY080" }
func (*CRDSchemaCheck) Phase() Phase { return PhaseCluster }

// Validated returns how many objects the check has validated.
func (c *CRDSchemaCheck) Validated() int64 { return c.validated.Load() }

func (c *CRDSchemaCheck) Run(ctx Context) []Finding {
	if c.CRDs == nil || len(ctx.Rendered) == 0 || ctx.Element == nil {
		return nil
	}
	el := ctx.Element
	var out []Finding
	unresolved := map[string]bool{}
	check := func(obj map[string]any) {
		apiVersion, _ := obj["apiVersion"].(string)
		kind, _ := obj["kind"].(string)
		gv, _ := schema.ParseGroupVersion(apiVersion)
		crd := c.CRDs[gv.WithKind(kind)]
		if crd == nil {
			unresolved[apiVersion+"/"+kind] = true
			return
		}
		c.validated.Add(1)
		meta, _ := obj["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		for _, e := range crd.validate(obj) {
			where, file, loc := origin(ctx, kind, name, leaf(e))
			if file == "" {
				file = el.ValuesFile
			}
			out = append(out, Finding{RuleID: c.ID(), Severity: SevError, Element: el.ChartName,
				Message: where + ": " + e, File: file, Line: loc.Line, Col: loc.Col})
		}
	}

	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(ctx.Rendered), 4096)
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err == io.EOF {
			break
		} else if err != nil {
			return append(out, Finding{RuleID: c.ID(), Severity: SevError, Element: el.ChartName,
				Message: "rendered output: " + err.Error(), File: el.ValuesFile})
		}
		var obj map[string]any
		if err := utiljson.Unmarshal(raw, &obj); err != nil || obj == nil {
			continue
		}
		check(obj)
		if obj["kind"] != "Policy" {
			continue
		}
		spec, _ := obj["spec"].(map[string]any)
		templates, _ := spec["policy-templates"].([]any)
		for _, t := range templates {
			pt, _ := t.(map[string]any)
			od, ok := pt["objectDefinition"].(map[string]any)
			if !ok {
				continue
			}
			if s, ok := od["spec"].(map[string]any); ok {
				if ra, _ := s["remediationAction"].(string); informOnly(ra) {
					s["remediationAction"] = "inform"
				}
			}
			check(od)
		}
	}
	if len(unresolved) > 0 {
		out = append(out, Finding{RuleID: c.ID(), Severity: SevWarning, Element: el.ChartName,
			Message: fmt.Sprintf("no CRD for %s: not schema-checked; add it to `make crds` (docs/develop/validation.md#crd-schemas)",
				strings.Join(slices.Sorted(maps.Keys(unresolved)), ", ")),
			File: el.ValuesFile})
	}
	return out
}

// entry names the entry of comp, el's component, that renders the object kind/name for release,
// and returns its path under stack.<key> and, for a template, its converters. The path is nil when
// no entry does.
func entry(comp *chart.Component, el *chart.Element, release, kind, name string) (string, []string, []string) {
	switch kind {
	case "Policy":
		if p := comp.Policies[strings.TrimSuffix(name, "-"+release)]; p != nil {
			return strings.Join(p.Path, "."), p.Path, nil
		}
	case "PolicySet":
		for i, p := range comp.PolicySets {
			if p.Name+"-"+release == name {
				return fmt.Sprintf("policySets[%d] %q", i, p.Name), []string{"policySets", strconv.Itoa(i)}, nil
			}
		}
	default:
		if s := renders(comp, el, release, kind, name); s != nil {
			return s.Where(), s.Path, s.Files
		}
	}
	return "", nil, nil
}

// renders returns the policy-template of comp rendered as the object kind/name for release, nil when
// none is. Template objects carry the release's cluster suffix under suffixTemplateNames.
func renders(comp *chart.Component, el *chart.Element, release, kind, name string) *chart.SubPolicy {
	suffix, _ := strings.CutPrefix(release, el.ChartName+"-")
	for _, s := range comp.SubPolicies() {
		if s.Kind == kind && (name == s.Name || name == s.Name+"-"+suffix) {
			return &s
		}
	}
	return nil
}

// leafField matches the last field name of the path an error starts with.
var leafField = regexp.MustCompile(`([^.\[\]\s:]+)(?:\[\d+\])*:`)

// leaf returns the last field name of the path an error starts with, such as remediationAction for
// "spec.remediationAction: Unsupported value", which values entries mostly share with the spec.
func leaf(err string) string {
	if m := leafField.FindStringSubmatch(err); m != nil {
		return m[1]
	}
	return ""
}

// CRDs maps each served group, version and kind to its schema.
type CRDs map[schema.GroupVersionKind]*crdVersion

type crdVersion struct {
	structural *structuralschema.Structural
	validator  validation.SchemaValidator
}

// LoadCRDs reads every CustomResourceDefinition in dir's .yaml files. No CRDs is an error wrapping
// fs.ErrNotExist.
func LoadCRDs(dir string) (CRDs, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	out := CRDs{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
		for {
			var crd apiextensionsv1.CustomResourceDefinition
			if err := dec.Decode(&crd); err == io.EOF {
				break
			} else if err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			if crd.Kind != "CustomResourceDefinition" {
				continue
			}
			for _, v := range crd.Spec.Versions {
				if !v.Served || v.Schema == nil || v.Schema.OpenAPIV3Schema == nil {
					continue
				}
				cv, err := newCRDVersion(v.Schema.OpenAPIV3Schema)
				if err != nil {
					return nil, fmt.Errorf("%s %s: %w", f, v.Name, err)
				}
				out[schema.GroupVersionKind{Group: crd.Spec.Group, Version: v.Name, Kind: crd.Spec.Names.Kind}] = cv
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no CRDs in %s: %w", dir, fs.ErrNotExist)
	}
	return out, nil
}

func newCRDVersion(s *apiextensionsv1.JSONSchemaProps) (*crdVersion, error) {
	var internal apiextensions.JSONSchemaProps
	if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(s, &internal, nil); err != nil {
		return nil, err
	}
	structural, err := structuralschema.NewStructural(&internal)
	if err != nil {
		return nil, err
	}
	validator, _, err := validation.NewSchemaValidator(&internal)
	if err != nil {
		return nil, err
	}
	return &crdVersion{structural: structural, validator: validator}, nil
}

// validate prunes, defaults and validates obj in place, as the API server does, and returns each
// error prefixed with its field path.
func (v *crdVersion) validate(obj map[string]any) []string {
	var errs []string
	for _, p := range pruning.PruneWithOptions(obj, v.structural, true, structuralschema.UnknownFieldPathOptions{TrackUnknownFieldPaths: true}) {
		errs = append(errs, p+": unknown field")
	}
	defaulting.Default(obj, v.structural)
	for _, e := range validation.ValidateCustomResource(nil, obj, v.validator) {
		errs = append(errs, e.Error())
	}
	return errs
}

func clusterName(ctx Context) string {
	if ctx.Cluster != nil {
		return ctx.Cluster.ClusterName
	}
	return ""
}
