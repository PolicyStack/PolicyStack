package checks

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
)

// InstallCheck (POLICY071) checks install/, which each hub runs by hand, so nothing else renders it.
// Runner and Root are set by run.buildRegistry; the check does nothing without them.
type InstallCheck struct {
	Runner *render.Runner
	Root   string
}

func (InstallCheck) ID() string   { return "POLICY071" }
func (InstallCheck) Phase() Phase { return PhaseRepo }

const (
	// backupLabel keeps an object out of ACM backup. Restored on a passive hub, an ApplicationSet
	// would rebuild the old hub's clusters there (docs/guides/lifecycle.md#hub-disaster-recovery).
	backupLabel = "velero.io/exclude-from-backup"
	// operatorKinds is what install/operators renders from the acm and gitops values, sorted.
	operatorKinds = "MultiClusterHub,Namespace,Namespace,OperatorGroup,OperatorGroup,Subscription,Subscription"
)

// backupExcluded are the install kinds ACM backup would restore.
var backupExcluded = []string{"ApplicationSet", "GitOpsCluster", "Placement"}

func (c *InstallCheck) Run(Context) []Finding {
	dir := filepath.Join(c.Root, "install")
	if c.Runner == nil || !render.FileExists(filepath.Join(dir, "Chart.yaml")) {
		return nil
	}
	bg, values := context.Background(), filepath.Join(dir, "values.yaml")
	var out []Finding
	report := func(file, msg string) {
		out = append(out, Finding{RuleID: c.ID(), Severity: SevError, Message: msg, File: file})
	}
	required := []string{"hubName=ci", "gitRepo=https://example.invalid/r.git"}

	if msg, err := c.Runner.Lint(bg, dir, "--set-string", required[0], "--set-string", required[1]); err != nil {
		report(dir, "helm lint failed: "+truncate(msg, 800))
	}
	for _, kv := range required {
		key, _, _ := strings.Cut(kv, "=")
		if tr := c.Runner.Template(bg, dir, "ci", nil, append(slices.Clone(required), key+"=")); !strings.Contains(tr.Stderr, key+" must be set") {
			report(values, key+" is not required: an install without it must fail")
		}
	}
	ns, _ := chart.LoadPolicyNamespace(values)
	if root, _ := chart.LoadPolicyNamespace(filepath.Join(c.Root, "values.yaml")); ns == "" || ns != root {
		report(values, fmt.Sprintf("policyNamespace %q must equal %q in values.yaml, which the elements read", ns, root))
	}

	if tr := c.Runner.Template(bg, dir, "ci", nil, required); tr.Err != nil {
		report(dir, "helm template failed: "+truncate(tr.Stderr, 800))
	} else {
		for _, o := range objects(tr.Stdout) {
			if !slices.Contains(backupExcluded, o.Kind) {
				continue
			}
			where := o.Kind + " " + o.Metadata.Name
			if o.Metadata.Labels[backupLabel] != "true" {
				report(dir, fmt.Sprintf("%s has no %s: \"true\" label, so ACM backup restores it on a passive hub", where, backupLabel))
			}
			if o.Kind == "ApplicationSet" && o.Spec.Template.Metadata.Labels[backupLabel] != "true" {
				report(dir, fmt.Sprintf("%s template has no %s: \"true\" label, so ACM backup restores its Applications on a passive hub", where, backupLabel))
			}
		}
	}

	ops := filepath.Join(dir, "operators")
	tr := c.Runner.Template(bg, ops, "ci", []string{
		filepath.Join(c.Root, "stack", "acm", "values.yaml"), filepath.Join(c.Root, "stack", "gitops", "values.yaml"),
	}, nil)
	var kinds []string
	for _, o := range objects(tr.Stdout) {
		kinds = append(kinds, o.Kind)
	}
	slices.Sort(kinds)
	if got := strings.Join(kinds, ","); tr.Err != nil || got != operatorKinds {
		report(ops, fmt.Sprintf("renders %q from the acm and gitops values, want %q: %s", got, operatorKinds, truncate(tr.Stderr, 800)))
	}
	return out
}

// object is the part of a rendered install object POLICY071 reads.
type object struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name   string         `json:"name"`
		Labels map[string]any `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Template struct {
			Metadata struct {
				Labels map[string]any `json:"labels"`
			} `json:"metadata"`
		} `json:"template"`
	} `json:"spec"`
}

// objects decodes a render, skipping empty documents.
func objects(rendered []byte) []object {
	var out []object
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	for {
		var o object
		if dec.Decode(&o) != nil {
			return out
		}
		if o.Kind != "" {
			out = append(out, o)
		}
	}
}
