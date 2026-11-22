// Package revision resolves a fleet file's revision to the commit Argo CD renders, with git.
package revision

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
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

// Base returns the commit HEAD forked from branch at, origin's before the local one. Without a
// branch, or when it does not resolve, that is HEAD.
func Base(ctx context.Context, dir, branch string) string {
	if branch != "" {
		for _, ref := range []string{"refs/remotes/origin/" + branch, "refs/heads/" + branch} {
			if out, err := git(ctx, dir, "merge-base", "HEAD", ref).Output(); err == nil {
				return strings.TrimSpace(string(out))
			}
		}
	}
	return "HEAD"
}

// Extract writes the tree at commit into dst, which it creates. It reads blobs rather than using
// git archive, which applies the commit's export-ignore and export-subst where Argo CD's checkout
// doesn't. Nothing is written outside dst, whatever the tree's links point at.
func Extract(ctx context.Context, dir, commit, dst string) (err error) {
	list, err := git(ctx, dir, "ls-tree", "-r", "-z", "--full-tree", commit).Output()
	if err != nil {
		return fmt.Errorf("read the tree at %s: %w", commit, err)
	}
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return err
	}
	root, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	for entry := range strings.SplitSeq(strings.TrimSuffix(string(list), "\x00"), "\x00") {
		// <mode> <type> <object>\t<path>
		meta, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || fields[1] != "blob" {
			continue // an empty tree, or a submodule
		}
		data, err := git(ctx, dir, "cat-file", "blob", fields[2]).Output()
		if err != nil {
			return fmt.Errorf("read %s at %s: %w", name, commit, err)
		}
		if err := root.MkdirAll(path.Dir(name), 0o750); err != nil {
			return err
		}
		if fields[0] == "120000" {
			err = root.Symlink(string(data), name)
		} else {
			err = root.WriteFile(name, data, 0o600)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func git(ctx context.Context, dir string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // G204: no shell; fixed binary
}
