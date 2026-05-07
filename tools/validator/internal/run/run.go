// Package run is the top-level orchestrator: discovers elements, loads
// fixtures, dispatches checks across the right phase, and aggregates findings.
package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/cascade"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/checks"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
)

// Options is the runtime configuration for a single Run.
type Options struct {
	RepoRoot       string
	StackDir       string
	SampleDir      string
	ValuesDir      string
	FixturesDir    string
	BaseDomain     string
	HelmBin        string
	KubeconformBin string
	SchemasDir     string
	// ExtraValues are absolute paths appended to every (element, fixture)
	// cascade with highest precedence. Useful for supplying baseline values
	// (e.g. a default `selector`) when fixture cluster names don't match
	// real per-cluster files in values/clusters/.
	ExtraValues   []string
	Skip          []string
	Only          []string
	IncludeSample bool
	Jobs          int
	Logger        *slog.Logger
}

// Result aggregates the run output.
type Result struct {
	Findings []checks.Finding
	Errors   int
	Warnings int
}

// Run executes the validator with opts. ctx is used to cancel helm calls.
func Run(ctx context.Context, opts Options) (Result, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Jobs <= 0 {
		opts.Jobs = runtime.NumCPU()
	}
	if err := render.HelmAvailable(opts.HelmBin); err != nil {
		return Result{}, err
	}

	rootValues := filepath.Join(opts.RepoRoot, "values.yaml")
	sample := ""
	if opts.IncludeSample {
		sample = opts.SampleDir
	}
	elements, _, err := chart.LoadAll(opts.StackDir, sample, rootValues)
	if err != nil {
		return Result{}, fmt.Errorf("load elements: %w", err)
	}
	if len(elements) == 0 {
		opts.Logger.Warn("no elements found", "stack-dir", opts.StackDir)
	}

	clusters, err := fixtures.LoadDir(opts.FixturesDir)
	if err != nil {
		return Result{}, fmt.Errorf("load fixtures: %w", err)
	}
	if len(clusters) == 0 {
		opts.Logger.Warn("no fixture clusters found", "fixtures-dir", opts.FixturesDir)
	}

	runner := render.New(opts.HelmBin)

	registered := buildRegistry(opts, runner)
	enabled := filterRules(registered, opts.Only, opts.Skip)
	opts.Logger.Debug("rule set", "enabled", ruleIDs(enabled))

	res := Result{}
	var mu sync.Mutex
	addFindings := func(fs []checks.Finding) {
		if len(fs) == 0 {
			return
		}
		mu.Lock()
		res.Findings = append(res.Findings, fs...)
		mu.Unlock()
	}

	// Pre-warm dependencies for every element so chart-phase checks like
	// helm lint don't fail on missing charts/policy-library-*.tgz.
	for _, el := range elements {
		if err := runner.EnsureDeps(ctx, el.Dir); err != nil {
			opts.Logger.Warn("dependency update failed", "element", el.ChartName, "err", err)
		}
	}

	// Phase 1: chart-wide checks (per element).
	for _, el := range elements {
		c := checks.Context{Element: el, Logger: opts.Logger}
		for _, ck := range enabled {
			if ck.Phase() != checks.PhaseChart {
				continue
			}
			addFindings(ck.Run(c))
		}
	}

	// Phase 2: per-cluster checks (parallel, with helm template per pair).
	type pair struct {
		el *chart.Element
		mc *fixtures.ManagedCluster
	}
	var pairs []pair
	for _, el := range elements {
		for _, mc := range clusters {
			pairs = append(pairs, pair{el, mc})
		}
	}

	// Cross-element duplicate-name accumulator (POLICY002 cross-element).
	type clusterKey struct {
		policyNs    string
		clusterName string
	}
	type rendered struct {
		element string
		path    string // values.yaml path for source line lookups
		name    string
	}
	dupAccum := map[clusterKey][]rendered{}
	var dupMu sync.Mutex

	sem := make(chan struct{}, opts.Jobs)
	var wg sync.WaitGroup
	for _, p := range pairs {
		p := p
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			c, renderFinding := perCluster(ctx, opts, runner, p.el, p.mc)
			if renderFinding != nil && !skipped("RENDER000", opts) {
				addFindings([]checks.Finding{*renderFinding})
			}
			for _, ck := range enabled {
				if ck.Phase() != checks.PhaseCluster {
					continue
				}
				fs := ck.Run(c)
				for i := range fs {
					if fs[i].File == "" {
						fs[i].File = p.mc.SourceFile
					}
				}
				addFindings(fs)
			}
			// Collect rendered policy names for cross-element dup detection.
			if c.Element != nil && c.Element.Values != nil && c.Element.Values.Component != nil {
				ns := c.Element.Values.PolicyNamespace
				rel := c.Cluster.ReleaseName
				key := clusterKey{policyNs: ns, clusterName: c.Cluster.ClusterName}
				comp := c.Element.Values.Component
				dupMu.Lock()
				for _, pol := range comp.Policies {
					if pol.Name == "" {
						continue
					}
					dupAccum[key] = append(dupAccum[key], rendered{
						element: c.Element.ChartName,
						path:    c.Element.ValuesFile,
						name:    fmt.Sprintf("%s-%s", pol.Name, rel),
					})
				}
				dupMu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Cross-element POLICY002.
	if !skipped("POLICY002", opts) {
		for key, list := range dupAccum {
			seen := map[string]rendered{}
			for _, r := range list {
				if prior, ok := seen[r.name]; ok && prior.element != r.element {
					addFindings([]checks.Finding{{
						RuleID:   "POLICY002",
						Severity: checks.SevError,
						Element:  r.element,
						Cluster:  key.clusterName,
						Message: fmt.Sprintf("rendered policy name %q in namespace %q collides across elements (also in %s)",
							r.name, key.policyNs, prior.element),
						File: r.path,
					}})
				}
				seen[r.name] = r
			}
		}
	}

	// Phase 3: repo-wide checks.
	repoCtx := checks.Context{AllElements: elements, Logger: opts.Logger}
	for _, ck := range enabled {
		if ck.Phase() != checks.PhaseRepo {
			continue
		}
		addFindings(ck.Run(repoCtx))
	}

	for _, f := range res.Findings {
		if f.Severity == checks.SevError {
			res.Errors++
		} else {
			res.Warnings++
		}
	}
	return res, nil
}

func perCluster(ctx context.Context, opts Options, runner *render.Runner, el *chart.Element, mc *fixtures.ManagedCluster) (checks.Context, *checks.Finding) {
	res := cascade.Resolve(mc, el.Dir, el.ChartName, opts.RepoRoot, opts.ValuesDir, opts.BaseDomain)
	if len(opts.ExtraValues) > 0 {
		res.ValueFiles = append(res.ValueFiles, opts.ExtraValues...)
	}
	c := checks.Context{
		Element: el,
		Cluster: &res,
		Logger:  opts.Logger,
	}
	if err := runner.EnsureDeps(ctx, el.Dir); err != nil {
		opts.Logger.Warn("dependency update failed", "element", el.ChartName, "err", err)
	}
	tr := runner.Template(ctx, el.Dir, res.ReleaseName, res.ValueFiles)
	if tr.Err != nil && !errors.Is(tr.Err, context.Canceled) {
		return c, renderErrorFinding(el, &res, tr)
	}
	c.Rendered = tr.Stdout
	return c, nil
}

// renderErrorFinding turns a TemplateResult error into a RENDER000 finding.
// Returns nil if there was no error.
func renderErrorFinding(el *chart.Element, cl *cascade.Resolved, tr render.TemplateResult) *checks.Finding {
	if tr.Err == nil {
		return nil
	}
	msg := tr.Stderr
	if msg == "" {
		msg = tr.Err.Error()
	}
	if hint := renderHint(msg); hint != "" {
		msg = hint + "\n\n" + msg
	}
	f := &checks.Finding{
		RuleID:   "RENDER000",
		Severity: checks.SevError,
		Element:  el.ChartName,
		Message:  truncate(msg, 1500),
	}
	if cl != nil {
		f.Cluster = cl.ClusterName
	}
	if tr.ErrFile != "" {
		f.File = tr.ErrFile
		f.Line = tr.ErrLine
	} else {
		f.File = el.ValuesFile
	}
	return f
}

// renderHint maps known template error patterns to actionable suggestions.
// Empty string means no hint applies.
func renderHint(stderr string) string {
	switch {
	case strings.Contains(stderr, "Values.selector.matchExpressions") && strings.Contains(stderr, "nil pointer"):
		return "hint: cascade did not provide `selector`. Either name the fixture cluster to match a real values/clusters/<name>.yaml that defines selector, or pass --extra-values <file> with a baseline `selector:` block (or set `disablePlacements: true` on the element)."
	case strings.Contains(stderr, "nil pointer evaluating"):
		return "hint: a values key referenced by the chart template is missing. Check that your fixture's cascade resolves to a values file that supplies the missing key."
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// buildRegistry instantiates checks with their runtime-configured fields.
func buildRegistry(opts Options, runner *render.Runner) []checks.Check {
	out := []checks.Check{}
	for _, c := range checks.All() {
		switch v := c.(type) {
		case *checks.LintCheck:
			v.Runner = runner
			out = append(out, v)
		case *checks.KubeconformCheck:
			v.Bin = opts.KubeconformBin
			v.SchemasDir = opts.SchemasDir
			out = append(out, v)
		default:
			out = append(out, c)
		}
	}
	return out
}

func filterRules(all []checks.Check, only, skip []string) []checks.Check {
	if len(only) > 0 {
		set := map[string]struct{}{}
		for _, id := range only {
			set[strings.TrimSpace(id)] = struct{}{}
		}
		var out []checks.Check
		for _, c := range all {
			if _, ok := set[c.ID()]; ok {
				out = append(out, c)
			}
		}
		return out
	}
	if len(skip) == 0 {
		return all
	}
	var out []checks.Check
	for _, c := range all {
		if slices.Contains(skip, c.ID()) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func ruleIDs(in []checks.Check) []string {
	out := make([]string, len(in))
	for i, c := range in {
		out[i] = c.ID()
	}
	return out
}

func skipped(ruleID string, opts Options) bool {
	if len(opts.Only) > 0 {
		return !slices.Contains(opts.Only, ruleID)
	}
	return slices.Contains(opts.Skip, ruleID)
}
