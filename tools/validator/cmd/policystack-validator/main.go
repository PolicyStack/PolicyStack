// policystack-validator checks the PolicyStack element charts. It resolves the
// values cascade the same way appset.yaml does, renders each element under
// stack/ for every fixture cluster and lifecycle/ for each that sets hub, and
// reports findings as terminal output or GitHub Actions annotations.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/checks"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/reporter"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/revision"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/run"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "resolve-revision" {
		os.Exit(resolveRevision(os.Args[2:]))
	}
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
}

func realMain() error {
	var (
		repoRoot      = flag.String("repo-root", ".", "path to repo root")
		stackDir      = flag.String("stack-dir", "", "override stack/ (default <repo>/stack)")
		sampleDir     = flag.String("sample-dir", "", "override sample-element/ (default <repo>/tools/sample-element)")
		lifecycleDir  = flag.String("lifecycle-dir", "", "override lifecycle/, rendered for fleet files that set hub (default <repo>/lifecycle)")
		valuesDir     = flag.String("values-dir", "", "override values/ (default <repo>/values)")
		fixturesDir   = flag.String("fixtures-dir", "", "fleet files to render, hubs in its hubs/ (default <repo>/tools/validator/testdata/clusters)")
		github        = flag.Bool("github", false, "emit GitHub Actions ::error/::warning annotations")
		severity      = flag.String("severity", "error", "fail threshold: error|warning")
		skipFlag      = flag.String("skip", "", "comma-separated rule IDs to skip")
		onlyFlag      = flag.String("only", "", "comma-separated rule IDs to run exclusively")
		jobs          = flag.Int("jobs", 0, "parallel render workers (default NumCPU)")
		helmBin       = flag.String("helm-bin", "helm", "helm binary")
		crdsDir       = flag.String("crds-dir", "", "CRDs POLICY080 validates against, fetched by make crds (default <repo>/tools/validator/.crds)")
		includeSample = flag.Bool("include-sample-element", false, "validate tools/sample-element/ alongside stack/")
		extraValues   = flag.String("extra-values", "", "comma-separated values files appended (highest precedence) to every cascade but lifecycle's — useful for supplying baseline `selector` etc.")
		renderDir     = flag.String("render-dir", "", "write each render to <dir>/<fleet file path without .yaml>/<chart>.yaml, a failed one as # comments, for diffing two trees")
		baseBranch    = flag.String("base-branch", os.Getenv("GITHUB_BASE_REF"), "branch HEAD lands on; POLICY052 resolves a revision naming it to HEAD")
		verbose       = flag.Bool("v", false, "verbose logging")
	)
	flag.Parse()

	root, err := filepath.Abs(*repoRoot)
	if err != nil {
		return err
	}
	defaults := struct{ stack, sample, lifecycle, values, fixtures, crds string }{
		stack:     filepath.Join(root, "stack"),
		sample:    filepath.Join(root, "tools", "sample-element"),
		lifecycle: filepath.Join(root, "lifecycle"),
		values:    filepath.Join(root, "values"),
		fixtures:  filepath.Join(root, "tools/validator/testdata/clusters"),
		crds:      filepath.Join(root, "tools/validator/.crds"),
	}
	if *stackDir == "" {
		*stackDir = defaults.stack
	}
	if *sampleDir == "" {
		*sampleDir = defaults.sample
	}
	if *lifecycleDir == "" {
		*lifecycleDir = defaults.lifecycle
	}
	if *valuesDir == "" {
		*valuesDir = defaults.values
	}
	if *fixturesDir == "" {
		*fixturesDir = defaults.fixtures
	}
	if *crdsDir == "" {
		*crdsDir = defaults.crds
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	extras, err := absPaths(splitCSV(*extraValues))
	if err != nil {
		return err
	}

	opts := run.Options{
		RepoRoot:      root,
		StackDir:      *stackDir,
		SampleDir:     *sampleDir,
		LifecycleDir:  *lifecycleDir,
		ValuesDir:     *valuesDir,
		FixturesDir:   *fixturesDir,
		HelmBin:       *helmBin,
		CRDsDir:       *crdsDir,
		ExtraValues:   extras,
		RenderDir:     *renderDir,
		BaseBranch:    *baseBranch,
		Skip:          splitCSV(*skipFlag),
		Only:          splitCSV(*onlyFlag),
		IncludeSample: *includeSample,
		Jobs:          *jobs,
		Logger:        logger,
	}

	res, err := run.Run(context.Background(), opts)
	if err != nil {
		return err
	}
	reporter.SortFindings(res.Findings)

	if *github {
		_ = (reporter.GitHub{RepoRoot: root}).Write(os.Stdout, res.Findings)
	} else {
		_ = (reporter.Pretty{}).Write(os.Stdout, res.Findings)
	}

	if shouldFail(res, *severity) {
		os.Exit(1)
	}
	return nil
}

// resolveRevision prints the commit Argo CD renders for a fleet file's revision, as POLICY052
// resolves it. Exit 1 when it does not resolve, 2 on bad usage.
func resolveRevision(args []string) int {
	fs := flag.NewFlagSet("resolve-revision", flag.ContinueOnError)
	root := fs.String("repo-root", ".", "path to repo root")
	branch := fs.String("branch", "", "branch that points to --at; the revision HEAD or this branch resolves to --at")
	at := fs.String("at", "HEAD", "commit --branch points to")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: policystack-validator resolve-revision [flags] [--] <revision>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 {
		if err == nil {
			fs.Usage()
		}
		return 2
	}
	sha, err := revision.Resolve(context.Background(), *root, fs.Arg(0), *branch, *at)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Println(sha)
	return 0
}

func shouldFail(r run.Result, threshold string) bool {
	switch threshold {
	case "warning":
		return r.Errors > 0 || r.Warnings > 0
	default:
		return r.Errors > 0
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func absPaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		ap, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("--extra-values %s: %w", p, err)
		}
		out = append(out, ap)
	}
	return out, nil
}

var _ = checks.Finding{}
