// policystack-validator runs CI-friendly checks over PolicyStack ACM-policy
// charts: renders each element under stack/ for every fixture cluster, walks
// the values cascade exactly like appset.yaml does at runtime, and emits
// findings as pretty terminal output or GitHub Actions annotations.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/checks"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/reporter"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/run"
)

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
}

func realMain() error {
	var (
		repoRoot       = flag.String("repo-root", ".", "path to repo root")
		stackDir       = flag.String("stack-dir", "", "override stack/ (default <repo>/stack)")
		sampleDir      = flag.String("sample-dir", "", "override sample-element/ (default <repo>/sample-element)")
		valuesDir      = flag.String("values-dir", "", "override values/ (default <repo>/values)")
		fixturesDir    = flag.String("fixtures-dir", "", "fixture ManagedCluster YAMLs (default <repo>/tools/validator/testdata/clusters)")
		baseDomain     = flag.String("base-domain", "example.com", "config label prefix")
		github         = flag.Bool("github", false, "emit GitHub Actions ::error/::warning annotations")
		severity       = flag.String("severity", "error", "fail threshold: error|warning")
		skipFlag       = flag.String("skip", "", "comma-separated rule IDs to skip")
		onlyFlag       = flag.String("only", "", "comma-separated rule IDs to run exclusively")
		jobs           = flag.Int("jobs", 0, "parallel render workers (default NumCPU)")
		helmBin        = flag.String("helm-bin", "helm", "helm binary")
		kcBin          = flag.String("kubeconform-bin", "kubeconform", "kubeconform binary; POLICY080 is skipped if not on PATH")
		schemasDir     = flag.String("schemas-dir", "", "extra -schema-location for kubeconform")
		includeSample  = flag.Bool("include-sample-element", false, "validate sample-element/ alongside stack/")
		verbose        = flag.Bool("v", false, "verbose logging")
	)
	flag.Parse()

	root, err := filepath.Abs(*repoRoot)
	if err != nil {
		return err
	}
	defaults := struct{ stack, sample, values, fixtures string }{
		stack:    filepath.Join(root, "stack"),
		sample:   filepath.Join(root, "sample-element"),
		values:   filepath.Join(root, "values"),
		fixtures: filepath.Join(root, "tools/validator/testdata/clusters"),
	}
	if *stackDir == "" {
		*stackDir = defaults.stack
	}
	if *sampleDir == "" {
		*sampleDir = defaults.sample
	}
	if *valuesDir == "" {
		*valuesDir = defaults.values
	}
	if *fixturesDir == "" {
		*fixturesDir = defaults.fixtures
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	// kubeconform is optional — strip from opts if not on PATH.
	resolvedKc := *kcBin
	if resolvedKc != "" {
		if _, err := exec.LookPath(resolvedKc); err != nil {
			logger.Warn("kubeconform not on PATH; POLICY080 will be skipped", "binary", resolvedKc)
			resolvedKc = ""
		}
	}

	opts := run.Options{
		RepoRoot:       root,
		StackDir:       *stackDir,
		SampleDir:      *sampleDir,
		ValuesDir:      *valuesDir,
		FixturesDir:    *fixturesDir,
		BaseDomain:     *baseDomain,
		HelmBin:        *helmBin,
		KubeconformBin: resolvedKc,
		SchemasDir:     *schemasDir,
		Skip:           splitCSV(*skipFlag),
		Only:           splitCSV(*onlyFlag),
		IncludeSample:  *includeSample,
		Jobs:           *jobs,
		Logger:         logger,
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

var _ = checks.Finding{}
