package checks

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/revision"
)

// FleetPinCheck (POLICY052) resolves each fleet file's revision in git as Argo CD does and checks
// that the commit has what the file uses. Argo CD skips a valueFiles entry missing at the revision,
// so the cluster silently loses it. A hub file's installRevision must resolve to a commit with the
// pointer, install/templates/install.yaml.
//
// In a copier copy (a root .copier-answers.yml), it also checks the framework release each revision
// records: against this branch's, which renders its own major and the one before, and against the
// install of the hub that renders it, its installRevision or else this branch, which must be at least
// as new (docs/reference/compatibility.md#hub-and-revision-skew). A hub file is rendered by its hub
// and a file with hub: by that hub, so a newer revision there is an error. Any other cluster may be
// managed by any hub, so it is compared with the oldest hub install, as a warning. A revision
// without the file predates the copy and is not checked. Root and Branch are set by
// run.buildRegistry.
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
	installs := c.installs(bg, ctx.Clusters, copied, report)
	builtBy := map[string]string{}
	for _, cl := range ctx.Clusters {
		if !cl.IsHub && cl.Hub != "" {
			builtBy[cl.Name] = cl.Hub
		}
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
		if copied {
			if msg := c.skew(bg, at, headTag, head); msg != "" {
				report(cl.SourceFile, cl.Name, SevError, fmt.Sprintf("revision %q %s", cl.Revision, msg))
			} else {
				for _, f := range hubSkew(cl, c.framework(bg, at), installs, builtBy) {
					report(cl.SourceFile, cl.Name, f.sev, fmt.Sprintf("revision %q %s", cl.Revision, f.msg))
				}
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

// hubInstall is the framework release a hub's install runs.
type hubInstall struct {
	hub, ref string // ref is the installRevision, or "" for this branch
	version  *semver.Version
}

func (h hubInstall) String() string {
	at := "this branch"
	if h.ref != "" {
		at = fmt.Sprintf("installRevision %q", h.ref)
	}
	return fmt.Sprintf("hub %s's install, framework v%s at %s", h.hub, h.version, at)
}

// installs resolves each hub file's installRevision, reporting one that doesn't resolve or has no
// pointer, and returns the framework release each hub's install runs, by hubName. A hub whose install
// records no release is left out; in a copier copy, a pinned one is reported, since its clusters go
// unchecked.
func (c *FleetPinCheck) installs(ctx context.Context, clusters []*fixtures.Cluster, copied bool, report func(file, cluster string, sev Severity, msg string)) map[string]hubInstall {
	out := map[string]hubInstall{}
	for _, h := range clusters {
		if !h.IsHub {
			continue
		}
		at, err := revision.Resolve(ctx, c.Root, cmp.Or(h.InstallRevision, "HEAD"), c.Branch, "HEAD")
		switch {
		case err != nil:
			report(h.SourceFile, h.Name, SevError, "installRevision: "+err.Error()+" of this repository")
			continue
		case h.InstallRevision != "" && !revision.Has(ctx, c.Root, at, "install/templates/install.yaml"):
			report(h.SourceFile, h.Name, SevError, fmt.Sprintf("installRevision %q has no install/templates/install.yaml, the pointer "+
				"that keeps this hub's install in git: pin a release that has it", h.InstallRevision))
			continue
		}
		switch v := c.framework(ctx, at); {
		case v != nil:
			out[h.Name] = hubInstall{h.Name, h.InstallRevision, v}
		case copied && h.InstallRevision != "":
			report(h.SourceFile, h.Name, SevWarning, fmt.Sprintf("installRevision %q records no framework release tag in /%s: "+
				"the revisions this hub renders are not checked against it", h.InstallRevision, answersFile))
		}
	}
	return out
}

// framework returns the framework release commit records, nil when it records none.
func (c *FleetPinCheck) framework(ctx context.Context, commit string) *semver.Version {
	data, err := revision.Show(ctx, c.Root, commit, answersFile)
	if err != nil {
		return nil
	}
	tag, _, _ := frameworkMajor(data)
	v, _ := release(tag)
	return v
}

// skewFinding is one way a cluster's revision is newer than a hub install that renders it.
type skewFinding struct {
	sev Severity
	msg string
}

// hubSkew reports each hub install that renders cl and is older than cl's framework release v.
// builtBy maps a cluster to the hub that builds it, its fleet file's hub. A hub built by another is
// that hub's Global Hub child in default mode (docs/guides/global-hub.md), so the parent renders its
// hub file and the lifecycle of the clusters it names in hub. Where Git doesn't say, as for a hub
// file of a hub nobody builds, which may be an imported default-mode child, or a cluster without
// hub, the finding is a warning.
func hubSkew(cl *fixtures.Cluster, v *semver.Version, installs map[string]hubInstall, builtBy map[string]string) []skewFinding {
	if v == nil || len(installs) == 0 {
		return nil
	}
	var out []skewFinding
	check := func(hub string, sev Severity, advice string) {
		if h, ok := installs[hub]; ok && v.GreaterThan(h.version) {
			out = append(out, skewFinding{sev, fmt.Sprintf("is framework v%s, newer than %s, %s", v, h, advice)})
		}
	}
	switch {
	case cl.IsHub && builtBy[cl.Name] != "":
		check(builtBy[cl.Name], SevError, "which renders it as its Global Hub child: upgrade that hub first")
	case cl.IsHub:
		check(cl.Name, SevWarning, "which renders it unless a Global Hub parent does: upgrade the hub first")
	case cl.Hub != "":
		check(cl.Hub, SevError, "which renders it: upgrade the hub first")
		if p := builtBy[cl.Hub]; p != "" {
			check(p, SevError, "which builds it for its Global Hub child "+cl.Hub+": upgrade that hub first")
		}
	default:
		oldest := slices.MinFunc(slices.Collect(maps.Values(installs)), func(a, b hubInstall) int {
			return cmp.Or(a.version.Compare(b.version), strings.Compare(a.hub, b.hub))
		})
		check(oldest.hub, SevWarning, "the oldest hub install: upgrade every hub that manages this cluster first")
	}
	return out
}
