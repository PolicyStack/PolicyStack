// Package revision resolves a fleet file's revision to the commit Argo CD renders, with git.
package revision

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// sha is the commit SHA form Argo CD resolves: 5 to 40 hex characters.
var sha = regexp.MustCompile(`^[0-9a-f]{5,40}$`)

// Resolve returns the commit Argo CD renders for rev in the repository at dir. rev naming branch,
// or HEAD, resolves to at: the commit branch points to once merged, such as a PR's merge commit.
// Any other rev is a tag, a branch of origin, then a commit SHA, in that order.
func Resolve(ctx context.Context, dir, rev, branch, at string) (string, error) {
	// check-ref-format rejects forms Argo CD does not resolve but rev-parse would, such as v1~1.
	if rev != "" && git(ctx, dir, "check-ref-format", "--allow-onelevel", rev).Run() == nil {
		candidates := []string{"refs/tags/" + rev, "refs/remotes/origin/" + rev}
		switch {
		case rev == "HEAD" || rev == branch:
			candidates = []string{at}
		case sha.MatchString(rev):
			candidates = append(candidates, rev)
		}
		for _, c := range candidates {
			if out, err := git(ctx, dir, "rev-parse", "-q", "--verify", c+"^{commit}").Output(); err == nil {
				return strings.TrimSpace(string(out)), nil
			}
		}
	}
	return "", fmt.Errorf("revision %q is not a tag, branch or commit SHA", rev)
}

// Has reports whether path exists at commit.
func Has(ctx context.Context, dir, commit, path string) bool {
	return git(ctx, dir, "cat-file", "-e", commit+":"+path).Run() == nil
}

// Show returns the content of path at commit, or an error when the commit has no such file.
func Show(ctx context.Context, dir, commit, path string) ([]byte, error) {
	return git(ctx, dir, "cat-file", "blob", commit+":"+path).Output()
}

func git(ctx context.Context, dir string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // G204: no shell; fixed binary
}
