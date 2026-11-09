package checks

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/acmtemplates"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
)

// InstallCheck (POLICY071) checks install/, which each hub's Argo CD syncs through its pointer
// (install/templates/install.yaml) and nothing else renders. Argo CD applies it on merge, so a
// change must never delete what it created: every object carries Prune=false and Delete=false, and
// the pointer's Application has no finalizer and never prunes. Runner and Root are set by
// run.buildRegistry; the check does nothing without them.
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

// backupExcluded are the install kinds ACM backup would restore. Restored on a passive hub, the
// Policy would install PolicyStack on the old hub's children a second time.
var backupExcluded = []string{"ApplicationSet", "GitOpsCluster", "Placement", "Policy", "PlacementBinding"}

// keep is the sync option on every install object: Argo CD reads it from the live object, so a
// release that drops or renames a template can neither prune nor delete it.
const keep = "Prune=false,Delete=false"

// pointer is the ApplicationSet that generates the Application syncing install/.
const pointer = "policystack-install"

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
	required := []string{"hubName=ci", "gitRepo=https://example.invalid/r.git", "repoSecret=policystack-repo"}

	if msg, err := c.Runner.Lint(bg, dir, "--set-string", required[0], "--set-string", required[1]); err != nil {
		report(dir, "helm lint failed: "+truncate(msg, 800))
	}
	for _, kv := range required[:2] {
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
		pointers := 0
		for _, o := range objects(tr.Stdout) {
			where := o.Kind + " " + o.Metadata.Name
			if o.Metadata.Annotations["argocd.argoproj.io/sync-options"] != keep {
				report(dir, fmt.Sprintf("%s has no argocd.argoproj.io/sync-options: %s annotation, so a release that drops it deletes it", where, keep))
			}
			if o.Kind == "GitOpsCluster" && o.Spec.GitopsAddon.Enabled {
				report(dir, where+" enables gitopsAddon, which turns off the push model every PolicyStack Application relies on")
			}
			if o.Kind == "ApplicationSet" && o.Metadata.Name == pointer {
				pointers++
				if t := o.Spec.Template; len(t.Metadata.Finalizers) > 0 || t.Spec.SyncPolicy.Automated.Prune {
					report(dir, where+" generates an Application with a finalizer or prune: deleting or resyncing it would delete the install")
				}
			}
			if !slices.Contains(backupExcluded, o.Kind) {
				continue
			}
			if o.Metadata.Labels[backupLabel] != "true" {
				report(dir, fmt.Sprintf("%s has no %s: \"true\" label, so ACM backup restores it on a passive hub", where, backupLabel))
			}
			if o.Kind == "ApplicationSet" && o.Spec.Template.Metadata.Labels[backupLabel] != "true" {
				report(dir, fmt.Sprintf("%s template has no %s: \"true\" label, so ACM backup restores its Applications on a passive hub", where, backupLabel))
			}
		}
		if pointers != 1 {
			report(dir, fmt.Sprintf("renders %d ApplicationSet %s, want 1: without it the hub's install stops following git", pointers, pointer))
		}
		failures, err := acmtemplates.Evaluate(tr.Stdout, "hub-a")
		if err != nil {
			report(dir, "rendered output: "+err.Error())
		}
		for _, f := range failures {
			report(dir, fmt.Sprintf("%s: ACM cannot resolve its template: %s", f.ConfigPolicy, f.Err))
		}
	}
	// A child's pointer is its parent's Policy's: rendering its own too would give it two owners.
	if tr := c.Runner.Template(bg, dir, "ci", nil, append(slices.Clone(required), "parent=gh")); tr.Err != nil {
		report(dir, "helm template with parent set, as on a child hub, failed: "+truncate(tr.Stderr, 800))
	} else if slices.ContainsFunc(objects(tr.Stdout), func(o object) bool { return o.Metadata.Name == pointer }) {
		report(dir, "renders ApplicationSet "+pointer+" with parent set: its parent's Policy already enforces it")
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
		Name        string            `json:"name"`
		Labels      map[string]any    `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		Template struct {
			Metadata struct {
				Labels     map[string]any `json:"labels"`
				Finalizers []string       `json:"finalizers"`
			} `json:"metadata"`
			Spec struct {
				SyncPolicy struct {
					Automated struct {
						Prune bool `json:"prune"`
					} `json:"automated"`
				} `json:"syncPolicy"`
			} `json:"spec"`
		} `json:"template"`
		GitopsAddon struct {
			Enabled bool `json:"enabled"`
		} `json:"gitopsAddon"`
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
