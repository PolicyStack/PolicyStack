package checks

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/revision"
)

// promotionBlocking are the rules whose error fails a promotion: a render failure is an outage,
// and the others reach or destroy what they must not. Every other error at a pinned commit is a
// warning, so a rule newer than the commit can't block a rollback to it.
var promotionBlocking = []string{"RENDER000", "POLICY037", "POLICY041", "POLICY042"}

// PromotionCheck (POLICY053) validates the commit a changed fleet file pins. The other rules read
// the working tree, which is not what a cluster pinned to a tag, a SHA or another branch gets.
// Root, Branch, DefaultBranch, Fleet and Validate are set by run.buildRegistry.
type PromotionCheck struct {
	// Root and Branch are as in FleetPinCheck. Fleet is the directory the fleet files load from.
	// DefaultBranch is the branch Argo CD reads fleet files from, Branch when empty: comparing with
	// it catches a promotion that reaches it through another branch or a retargeted PR.
	Root, Branch, DefaultBranch, Fleet string
	// Validate runs the validator on the tree at root for the fleet files in fleet.
	Validate func(ctx context.Context, root, fleet string) ([]Finding, error)
}

func (PromotionCheck) ID() string   { return "POLICY053" }
func (PromotionCheck) Phase() Phase { return PhaseRepo }

func (c *PromotionCheck) Run(ctx Context) []Finding {
	if c.Validate == nil {
		return nil
	}
	bg := context.Background()
	promoted := c.promoted(bg, ctx.Clusters)
	var out []Finding
	for _, commit := range slices.Sorted(maps.Keys(promoted)) {
		out = append(out, c.validate(bg, commit, promoted[commit])...)
	}
	return out
}

// promoted groups by the commit it pins each cluster whose fleet file differs from the one on the
// default branch. A pin of HEAD is left out: the run itself validates that tree.
func (c *PromotionCheck) promoted(ctx context.Context, clusters []*fixtures.Cluster) map[string][]*fixtures.Cluster {
	head, err := revision.Resolve(ctx, c.Root, "HEAD", "", "HEAD")
	if err != nil {
		return nil // not a repository, which POLICY052 reports
	}
	base := revision.Base(ctx, c.Root, cmp.Or(c.DefaultBranch, c.Branch))
	out := map[string][]*fixtures.Cluster{}
	for _, cl := range clusters {
		if cl.Revision == "" {
			continue // POLICY050
		}
		at, err := revision.Resolve(ctx, c.Root, cl.Revision, c.Branch, "HEAD")
		if err != nil || at == head {
			continue // unresolved is POLICY052
		}
		// make passes --fixtures-dir fleet, relative to the working directory, and Root is absolute.
		file, err := filepath.Abs(cl.SourceFile)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(c.Root, file)
		if err != nil {
			continue
		}
		now, err := os.ReadFile(cl.SourceFile)
		if err != nil {
			continue
		}
		if was, err := revision.Show(ctx, c.Root, base, filepath.ToSlash(rel)); err == nil && bytes.Equal(was, now) {
			continue
		}
		out[at] = append(out[at], cl)
	}
	return out
}

// validate runs Validate on commit for clusters and reports each error at the fleet file pinning it.
func (c *PromotionCheck) validate(ctx context.Context, commit string, clusters []*fixtures.Cluster) []Finding {
	found, err := c.findings(ctx, commit, clusters)
	if err != nil {
		// Nothing at the commit was checked, so this blocks too.
		return []Finding{{RuleID: c.ID(), Severity: SevError, Cluster: clusters[0].Name, File: clusters[0].SourceFile,
			Message: fmt.Sprintf("revision %q could not be validated: %v", clusters[0].Revision, err)}}
	}
	return c.report(clusters, found)
}

// report moves the errors found at a commit to the fleet files of clusters, which pin it.
func (c *PromotionCheck) report(clusters []*fixtures.Cluster, found []Finding) []Finding {
	var out []Finding
	for _, f := range found {
		if f.Severity != SevError {
			continue
		}
		// Most rules name no cluster: the first pinning the commit anchors the finding, and is named
		// only when it is the one cluster pinning it.
		i := slices.IndexFunc(clusters, func(cl *fixtures.Cluster) bool { return cl.Name == f.Cluster })
		cl, cluster := clusters[max(0, i)], f.Cluster
		if len(clusters) == 1 {
			cluster = cl.Name
		}
		sev := SevWarning
		if slices.Contains(promotionBlocking, f.RuleID) {
			sev = SevError
		}
		where := f.RuleID
		if f.File != "" {
			where += " at " + f.File
			if f.Line > 0 {
				where += fmt.Sprintf(":%d", f.Line)
			}
		}
		out = append(out, Finding{RuleID: c.ID(), Severity: sev, Element: f.Element, Cluster: cluster, File: cl.SourceFile,
			Message: fmt.Sprintf("revision %q fails %s: %s", cl.Revision, where, f.Message)})
	}
	return out
}

// findings validates commit, extracted to a temporary directory, for the fleet files of clusters as
// they are in the working tree. Files are returned relative to the commit's tree, or empty for a
// fleet file.
func (c *PromotionCheck) findings(ctx context.Context, commit string, clusters []*fixtures.Cluster) (_ []Finding, err error) {
	tmp, err := os.MkdirTemp("", "promotion-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	tree, fleet := filepath.Join(tmp, "tree"), filepath.Join(tmp, "fleet")
	if err := revision.Extract(ctx, c.Root, commit, tree); err != nil {
		return nil, err
	}
	for _, cl := range clusters {
		rel, err := filepath.Rel(c.Fleet, cl.SourceFile)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(cl.SourceFile)
		if err != nil {
			return nil, err
		}
		to := filepath.Join(fleet, rel)
		if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
			return nil, err
		}
		if err := os.WriteFile(to, data, 0o600); err != nil { //nolint:gosec // G703: rel is the path the fleet file was loaded from, under Fleet
			return nil, err
		}
	}
	prefix := tree + string(filepath.Separator)
	found, err := c.Validate(ctx, tree, fleet)
	if err != nil {
		return nil, errors.New(strings.ReplaceAll(err.Error(), prefix, ""))
	}
	for i, f := range found {
		rel, ok := strings.CutPrefix(f.File, prefix)
		if !ok {
			rel = ""
		}
		found[i].File, found[i].Message = filepath.ToSlash(rel), strings.ReplaceAll(f.Message, prefix, "")
	}
	return found, nil
}
