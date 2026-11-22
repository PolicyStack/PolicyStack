package revision

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	first := testenv.Commit(t, dir, "stack/a")
	testenv.Git(t, dir, "tag", "-a", "-m", "v1", "v1.0.0") // annotated: resolves to the commit, not the tag object
	testenv.Git(t, dir, "tag", "light")
	second := testenv.Commit(t, dir, "stack/b")
	testenv.Git(t, dir, "update-ref", "refs/remotes/origin/feature", second)
	testenv.Git(t, dir, "update-ref", "refs/remotes/origin/v1.0.0", second) // a tag wins over a branch
	testenv.Git(t, dir, "branch", "local-only")
	merge := testenv.Commit(t, dir, "stack/c")

	for _, tc := range []struct {
		name, rev, branch, want string
	}{
		{name: "annotated tag", rev: "v1.0.0", want: first},
		{name: "lightweight tag", rev: "light", want: first},
		{name: "origin branch", rev: "feature", want: second},
		{name: "full SHA", rev: first, want: first},
		{name: "short SHA", rev: first[:7], want: first},
		{name: "branch the merge lands on", rev: "main", branch: "main", want: merge},
		{name: "HEAD", rev: "HEAD", want: merge},
		{name: "local branch only", rev: "local-only"},
		{name: "SHA under 5 characters", rev: first[:4]},
		{name: "uppercase SHA", rev: "ABCDEF0"},
		{name: "unknown", rev: "gh-pages"},
		{name: "empty", rev: ""},
		{name: "rev-parse expression", rev: "v1.0.0~1"},
		{name: "option", rev: "--all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(t.Context(), dir, tc.rev, tc.branch, merge)
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Errorf("Resolve(%q) = %q, %v; want %q", tc.rev, got, err, tc.want)
			}
		})
	}

	if !Has(t.Context(), dir, first, "stack/a") || Has(t.Context(), dir, first, "stack/b") {
		t.Error("Has does not reflect the tree at the commit")
	}
	if got, err := Show(t.Context(), dir, first, "stack/a"); err != nil || string(got) != "stack/a\n" {
		t.Errorf("Show(stack/a) = %q, %v", got, err)
	}
	if _, err := Show(t.Context(), dir, first, "stack/b"); err == nil {
		t.Error("Show of a file the commit lacks: want an error")
	}
}

func TestBaseExtract(t *testing.T) {
	dir := t.TempDir()
	first := testenv.Commit(t, dir, "stack/a")
	testenv.Git(t, dir, "update-ref", "refs/remotes/origin/main", first)
	if err := os.Symlink("stack/a", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc", filepath.Join(dir, "outside")); err != nil {
		t.Fatal(err)
	}
	// git archive would drop stack/b.
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("stack/b export-ignore\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := testenv.Commit(t, dir, "stack/b")

	for branch, want := range map[string]string{"main": first, "": "HEAD", "absent": "HEAD"} {
		if got := Base(t.Context(), dir, branch); got != want {
			t.Errorf("Base(%q) = %q, want %q", branch, got, want)
		}
	}

	dst := filepath.Join(t.TempDir(), "tree")
	if err := Extract(t.Context(), dir, second, dst); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stack/a", "stack/b"} {
		if got, err := os.ReadFile(filepath.Join(dst, name)); err != nil || string(got) != name+"\n" {
			t.Errorf("%s = %q, %v", name, got, err)
		}
	}
	for link, want := range map[string]string{"link": "stack/a", "outside": "/etc"} {
		if got, err := os.Readlink(filepath.Join(dst, link)); err != nil || got != want {
			t.Errorf("%s -> %q, %v; want %q", link, got, err, want)
		}
	}

	earlier := filepath.Join(t.TempDir(), "tree")
	if err := Extract(t.Context(), dir, first, earlier); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(earlier, "stack/b")); !os.IsNotExist(err) {
		t.Errorf("stack/b at the first commit: %v, want absent", err)
	}
	if err := Extract(t.Context(), dir, "absent", filepath.Join(t.TempDir(), "tree")); err == nil {
		t.Error("Extract of an unknown commit: want an error")
	}
}
