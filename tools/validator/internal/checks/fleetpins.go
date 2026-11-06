package checks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/revision"
)

// FleetPinCheck (POLICY052) resolves each fleet file's revision in git as Argo CD does and checks
// that the commit has what the file uses. Argo CD skips a valueFiles entry missing at the revision,
// so the cluster silently loses it. In a copier copy (a root .copier-answers.yml), it also checks the
// framework release each revision records against this branch's: a hub runs this branch's install
// chart, which renders its own major and the one before. A revision without the file predates the
// copy and is not checked. Root and Branch are set by run.buildRegistry.
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
	// Not a copier copy, or its framework release is unreadable, which POLICY061 reports: no skew check.
	data, _ := os.ReadFile(filepath.Join(c.Root, answersFile))
	headTag, head, copied := frameworkMajor(data)
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
		if copied {
			if msg := c.skew(bg, at, headTag, head); msg != "" {
				report(cl.SourceFile, cl.Name, SevError, fmt.Sprintf("revision %q %s", cl.Revision, msg))
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
	// tag-pinned cluster's file is committed before its fleet file
	// (docs/start/first-cluster.md#onboard-the-cluster).
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

// skew describes how the framework release at commit falls outside what hubs on this branch's
// release, headTag of major head, render: the same major or the one before. Empty when it is
// inside, or when commit has no answers file.
func (c *FleetPinCheck) skew(ctx context.Context, commit, headTag string, head uint64) string {
	data, err := revision.Show(ctx, c.Root, commit, answersFile)
	if err != nil {
		return ""
	}
	switch tag, major, ok := frameworkMajor(data); {
	case !ok:
		return fmt.Sprintf("has no framework release tag in /%s (_commit %q)", answersFile, tag)
	case major > head:
		return fmt.Sprintf("is framework %s, a newer major than this branch's %s: hubs run this branch's install chart, "+
			"which cannot render it. Merge the framework update first", tag, headTag)
	case major+1 < head:
		return fmt.Sprintf("is framework %s, more than one major behind this branch's %s: hubs render this major and "+
			"the one before only. Move the revision forward", tag, headTag)
	}
	return ""
}
