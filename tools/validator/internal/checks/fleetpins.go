package checks

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/revision"
)

// FleetPinCheck (POLICY052) resolves each fleet file's revision in git as Argo CD does and checks
// that the commit has what the file uses. Argo CD skips a valueFiles entry missing at the revision,
// so the cluster silently loses it. Root and Branch are set by run.buildRegistry.
type FleetPinCheck struct {
	// Root is the repository. Branch is the branch HEAD lands on, a PR's base branch: a revision
	// naming it resolves to HEAD, the merge commit, which is what it resolves to once merged.
	Root, Branch string
}

func (FleetPinCheck) ID() string   { return "POLICY052" }
func (FleetPinCheck) Phase() Phase { return PhaseRepo }

// upgradeHelpers are what a fleet file that sets version or channel needs, one of them: an upgrade
// element under its current or former name.
var upgradeHelpers = []string{"stack/upgrade/templates/_helpers.tpl", "stack/openshift-upgrade/templates/_helpers.tpl"}

func (c *FleetPinCheck) Run(ctx Context) []Finding {
	bg := context.Background()
	var out []Finding
	report := func(file, cluster string, sev Severity, msg string) {
		out = append(out, Finding{RuleID: c.ID(), Severity: sev, Cluster: cluster, Message: msg, File: file})
	}
	for _, cl := range ctx.Clusters {
		if cl.Revision == "" {
			continue // POLICY050
		}
		at, err := revision.Resolve(bg, c.Root, cl.Revision, c.Branch, "HEAD")
		if err != nil {
			report(cl.SourceFile, cl.Name, SevError, err.Error()+" of this repository")
			continue
		}
		has := func(paths ...string) bool {
			return slices.ContainsFunc(paths, func(p string) bool { return revision.Has(bg, c.Root, at, p) })
		}
		var needs, skipped []string
		if !has("stack") {
			needs = append(needs, "stack/")
		}
		if cl.Hub != "" && !has("lifecycle/converters/_hypershift", "lifecycle/converters/hypershift") {
			needs = append(needs, "lifecycle/converters/_hypershift/, which hub needs")
		}
		if (cl.Version != "" || cl.Channel != "") && !has(upgradeHelpers...) {
			needs = append(needs, "an upgrade element reading version and channel")
		}
		for _, f := range cl.ValueFiles {
			if !has("values/" + f) {
				skipped = append(skipped, "values/"+f)
			}
		}
		if len(needs) > 0 {
			report(cl.SourceFile, cl.Name, SevError, fmt.Sprintf("revision %q has no %s", cl.Revision, strings.Join(needs, "; ")))
		}
		if len(skipped) > 0 {
			report(cl.SourceFile, cl.Name, SevError, fmt.Sprintf("revision %q has no %s, which Argo CD skips", cl.Revision, strings.Join(skipped, ", ")))
		}
	}
	// A cluster file is read by fleet file name, so a misnamed one is skipped. Only a warning: a
	// tag-pinned cluster's file is committed before its fleet file (docs/rollout.md#onboarding-a-cluster).
	if len(ctx.Clusters) == 0 {
		return out
	}
	files, _ := filepath.Glob(filepath.Join(c.Root, "values", "clusters", "*.yaml"))
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".yaml")
		if !slices.ContainsFunc(ctx.Clusters, func(cl *fixtures.Cluster) bool { return cl.Name == name }) {
			report(f, "", SevWarning, "no fleet file reads this cluster file: fleet/"+name+".yaml or fleet/hubs/"+name+".yaml")
		}
	}
	return out
}
