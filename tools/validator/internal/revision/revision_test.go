package revision

import (
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
}
