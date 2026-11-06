package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

// PinningCheck (POLICY060) checks that every chart takes policy-library from lib/policy-library:
// one dependency, alias stack, repository file:// to lib/, and a caret floor that the library's own
// version satisfies. Helm reports a broken file:// dependency only as "can't get a valid version",
// so this rule says what to change. Root is set by run.buildRegistry; the check does nothing
// without it.
type PinningCheck struct {
	Root string
}

func (PinningCheck) ID() string   { return "POLICY060" }
func (PinningCheck) Phase() Phase { return PhaseRepo }

// libraryDir is policy-library's chart directory, relative to the repo root.
const libraryDir = "lib/policy-library"

func (c *PinningCheck) Run(ctx Context) []Finding {
	if c.Root == "" {
		return nil
	}
	lib, err := filepath.Abs(filepath.Join(c.Root, libraryDir))
	if err != nil {
		return nil
	}
	libVersion, libErr := chartVersion(lib)
	var out []Finding
	seen := map[string]bool{} // the lifecycle chart's components share one Chart.yaml
	for _, el := range ctx.AllElements {
		if seen[el.Dir] {
			continue
		}
		seen[el.Dir] = true
		report := func(format string, args ...any) {
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevError, Element: el.ChartName,
				Message: fmt.Sprintf(format, args...), File: filepath.Join(el.Dir, "Chart.yaml"), Line: 1,
			})
		}
		if libErr != nil {
			report("cannot read the policy-library version: %v", libErr)
			continue
		}
		rel, _ := filepath.Rel(el.Dir, lib)
		if len(el.Dependencies) != 1 || el.Dependencies[0].Name != "policy-library" {
			report("Chart.yaml needs exactly one dependency, policy-library (name: policy-library, alias: stack, version: \"^%d.0.0\", repository: \"file://%s\"); it has %d",
				libVersion.Major(), filepath.ToSlash(rel), len(el.Dependencies))
			continue
		}
		d := el.Dependencies[0]
		if d.Alias != "stack" {
			report("policy-library has alias %q; set alias: stack, the values key the library reads and validates", d.Alias)
		}
		if p, ok := strings.CutPrefix(d.Repository, "file://"); !ok || filepath.IsAbs(p) || resolve(el.Dir, p) != lib {
			report("policy-library repository %q is not this repo's %s; set repository: \"file://%s\"", d.Repository, libraryDir, filepath.ToSlash(rel))
		}
		floor, cons := caret(d.Version)
		switch {
		case cons == nil:
			report("policy-library version %q is not a caret floor; set version: \"^%d.<minor>.0\", the lowest %d.x this chart needs", d.Version, libVersion.Major(), libVersion.Major())
		case libVersion.Prerelease() != "":
			report("%s version %s is a prerelease, which no ^%d.<minor>.0 floor accepts; drop -%s from %s/Chart.yaml", libraryDir, libVersion, libVersion.Major(), libVersion.Prerelease(), libraryDir)
		case floor.GreaterThan(libVersion):
			report("policy-library floor %s is above %s version %s; lower the floor, or bump %s/Chart.yaml in the PR that adds what this chart needs", d.Version, libraryDir, libVersion, libraryDir)
		case !cons.Check(libVersion):
			report("policy-library %s does not accept %s version %s; set version: \"^%d.<minor>.0\"", d.Version, libraryDir, libVersion, libVersion.Major())
		}
	}
	return out
}

// chartVersion reads version from dir/Chart.yaml.
func chartVersion(dir string) (*semver.Version, error) {
	data, err := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
	if err != nil {
		return nil, err
	}
	var c struct{ Version string }
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s/Chart.yaml: %w", dir, err)
	}
	return semver.StrictNewVersion(c.Version)
}

// resolve makes a relative file:// path absolute against the chart directory, as Helm does.
func resolve(dir, p string) string {
	abs, _ := filepath.Abs(filepath.Join(dir, p))
	return abs
}

// caret parses a "^X.Y.Z" constraint. Both are nil for anything else.
func caret(v string) (*semver.Version, *semver.Constraints) {
	rest, ok := strings.CutPrefix(v, "^")
	if !ok {
		return nil, nil
	}
	floor, err := semver.NewVersion(rest)
	if err != nil {
		return nil, nil
	}
	cons, err := semver.NewConstraint(v)
	if err != nil {
		return nil, nil
	}
	return floor, cons
}
