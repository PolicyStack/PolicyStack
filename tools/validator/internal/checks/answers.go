package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// answersFile is copier's record of what a copy came from: the framework's at the repository root,
// an element's in stack/<element>/. Its _commit is the git describe output of the copied ref.
const answersFile = ".copier-answers.yml"

var (
	// releaseTag is a release tag. The framework and every element share one tag line, v<semver>.
	releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)
	// describeSuffix is what git describe appends for a commit past the tag (v1.2.0-3-gabc1234),
	// which copier records when the copied ref is not a tag.
	describeSuffix = regexp.MustCompile(`-\d+-g[0-9a-f]+$`)
)

// release parses a _commit that is a release tag.
func release(commit string) (*semver.Version, bool) {
	if !releaseTag.MatchString(commit) || describeSuffix.MatchString(commit) {
		return nil, false
	}
	v, err := semver.StrictNewVersion(commit[1:])
	return v, err == nil
}

// frameworkMajor returns the major of the framework release an answers file records.
func frameworkMajor(data []byte) (string, uint64, bool) {
	var a struct {
		Commit string `yaml:"_commit"`
	}
	if yaml.Unmarshal(data, &a) != nil {
		return "", 0, false
	}
	v, ok := release(a.Commit)
	if !ok {
		return a.Commit, 0, false
	}
	return a.Commit, v.Major(), true
}

// AnswersCheck (POLICY061) checks the copier answers files: the root one must record a release
// tag, and each stack/<element>/ one its own element and a release tag no newer than the root's and
// on the same major. copier copy or update without --vcs-ref records git describe output for an
// untagged ref, which POLICY052 can't read; an element newer than the framework may need what only
// the newer framework has, and one on an older major was written against the element authoring API
// that the framework's major no longer promises. Root is set by run.buildRegistry.
type AnswersCheck struct {
	Root string
}

func (AnswersCheck) ID() string   { return "POLICY061" }
func (AnswersCheck) Phase() Phase { return PhaseRepo }

func (c *AnswersCheck) Run(Context) []Finding {
	if c.Root == "" {
		return nil
	}
	var out []Finding
	// check reports what is wrong with the answers file of element, the framework when empty, and
	// returns the release it records.
	check := func(file, element string, framework *semver.Version) *semver.Version {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil // no answers file: not a copier copy, or an ejected or local element
		}
		report := func(n *yaml.Node, msg string) {
			f := Finding{RuleID: c.ID(), Severity: SevError, Element: element, Message: msg, File: file}
			if n != nil {
				f.Line, f.Col = n.Line, n.Column
			}
			out = append(out, f)
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(data, &doc); err != nil {
			report(nil, "not valid YAML: "+err.Error())
			return nil
		}
		value := func(key string) (*yaml.Node, string) {
			n := sourceloc.FindNode(&doc, key)
			if n == nil {
				return nil, ""
			}
			return n, n.Value
		}
		commit, tag := value("_commit")
		v, ok := release(tag)
		if element == "" {
			if !ok {
				report(commit, fmt.Sprintf("_commit %q is not a release tag vX.Y.Z: rerun copier update with --vcs-ref vX.Y.Z", tag))
			}
			return v
		}
		if n, el := value("element"); el != element {
			report(n, fmt.Sprintf("element %q is not this directory, %q: copy each element into stack/<element> with -d element=<element>", el, element))
		}
		switch {
		case !ok:
			report(commit, fmt.Sprintf("_commit %q is not a release tag vX.Y.Z: rerun copier update in stack/%s with --vcs-ref vX.Y.Z", tag, element))
		case framework != nil && v.GreaterThan(framework):
			report(commit, fmt.Sprintf("_commit %s is newer than the framework's, v%s in /%s: update the framework to %s or later first, or this element back to v%s or earlier",
				tag, framework, answersFile, tag, framework))
		case framework != nil && v.Major() != framework.Major():
			report(commit, fmt.Sprintf("_commit %s is on major %d and the framework, v%s in /%s, on major %d: update this element to a v%d release, copier update --vcs-ref v%s stack/%s",
				tag, v.Major(), framework, answersFile, framework.Major(), framework.Major(), framework, element))
		}
		return v
	}
	framework := check(filepath.Join(c.Root, answersFile), "", nil)
	files, _ := filepath.Glob(filepath.Join(c.Root, "stack", "*", answersFile))
	for _, f := range files {
		check(f, filepath.Base(filepath.Dir(f)), framework)
	}
	return out
}
