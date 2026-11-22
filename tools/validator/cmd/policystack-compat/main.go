// policystack-compat fails a change to upstream's output for unchanged values that no maintainer
// approved (docs/reference/ci.md#compatibility). Upstream only.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/compat"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/revision"
)

func main() {
	ok, err := realMain(context.Background())
	switch {
	case err != nil:
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	case !ok:
		os.Exit(1)
	}
}

func realMain(ctx context.Context) (ok bool, err error) {
	root := flag.String("repo-root", ".", "the change: a checkout")
	release := flag.String("release", "", "the last release's tag or commit")
	base := flag.String("base", "", "the commit the change lands on; its differences from the release are already approved")
	labels := flag.String("labels", "", "comma-separated labels on the PR")
	out := flag.String("out", "", "directory to write changes.json and both sides of each change to")
	flag.Parse()
	if *release == "" || *out == "" {
		return false, fmt.Errorf("--release and --out are required")
	}
	head, err := filepath.Abs(*root)
	if err != nil {
		return false, err
	}
	work, err := os.MkdirTemp("", "compat-trees-")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(work) }()

	trees := compat.Trees{Head: head, Release: filepath.Join(work, "release")}
	if err := revision.Extract(ctx, head, *release, trees.Release); err != nil {
		return false, err
	}
	if *base != "" {
		trees.Base = filepath.Join(work, "base")
		if err := revision.Extract(ctx, head, *base, trees.Base); err != nil {
			return false, err
		}
	}
	report, err := compat.Diff(ctx, "helm", trees)
	if err != nil {
		return false, err
	}
	report.Approve(strings.Split(*labels, ","))
	if err := write(*out, report); err != nil {
		return false, err
	}
	summarise(report, *release)
	return len(report.Changes) == 0 || report.Approved, nil
}

// write puts the report in dir, and each side of each change under dir/release and dir/after.
func write(dir string, r *compat.Report) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "changes.json"), data, 0o600); err != nil {
		return err
	}
	for _, c := range r.Changes {
		for side, data := range map[string][]byte{"release": c.Release, "after": c.After} {
			file := filepath.Join(dir, side, filepath.FromSlash(c.Path))
			if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
				return err
			}
			if err := os.WriteFile(file, data, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

func summarise(r *compat.Report, release string) {
	if len(r.Changes) == 0 {
		fmt.Printf("ok: no new change to output for unchanged values since %s\n", release)
		return
	}
	charts := map[string][]string{}
	for _, c := range r.Changes {
		name := strings.TrimSuffix(path.Base(c.Path), ".yaml")
		scenario, _, _ := strings.Cut(c.Path, "/")
		if !slices.Contains(charts[name], scenario) {
			charts[name] = append(charts[name], scenario)
		}
	}
	fmt.Printf("Output for unchanged values differs from %s:\n\n| Chart | Scenarios |\n|---|---|\n", release)
	for _, name := range slices.Sorted(maps.Keys(charts)) {
		fmt.Printf("| %s | %s |\n", name, strings.Join(charts[name], ", "))
	}
	if r.Approved {
		fmt.Printf("\nApproved by label %s.\n", r.Label)
		return
	}
	fmt.Printf("\nKeep the release's output: a new key defaults to it and a new template ships enabled: false.\n"+
		"For a security fix with no compatible path, a moved operator channel or version, or a framework major, a maintainer adds label %s and re-runs the job. "+
		"The label covers exactly this difference.\n", r.Label)
}
