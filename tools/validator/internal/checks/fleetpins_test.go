package checks

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

func TestPolicy052_FleetPins(t *testing.T) {
	dir := t.TempDir()
	v1 := testenv.Commit(t, dir, "stack/acm/Chart.yaml", "values/environments/prod.yaml")
	testenv.Git(t, dir, "tag", "v1.0.0")
	testenv.Git(t, dir, "update-ref", "refs/remotes/origin/main", v1)
	testenv.Commit(t, dir, "stack/openshift-upgrade/templates/_helpers.tpl")
	testenv.Git(t, dir, "tag", "v1.1.0")
	head := testenv.Commit(t, dir, "values/platforms/kubevirt.yaml", "lifecycle/converters/hypershift/ready.yaml",
		"stack/upgrade/templates/_helpers.tpl")

	for _, tc := range []struct {
		name   string
		branch string
		cl     fixtures.Cluster
		want   []string
	}{
		{name: "tag with every file", cl: fixtures.Cluster{Revision: "v1.0.0", ValueFiles: []string{"environments/prod.yaml"}}},
		{name: "short SHA", cl: fixtures.Cluster{Revision: head[:7], ValueFiles: []string{"platforms/kubevirt.yaml"}}},
		{name: "empty revision is POLICY050's", cl: fixtures.Cluster{}},
		// regression: bf79781
		{
			name: "valueFiles entry missing at the revision",
			cl:   fixtures.Cluster{Revision: "v1.0.0", ValueFiles: []string{"environments/prod.yaml", "platforms/kubevirt.yaml", "typo.yaml"}},
			want: []string{`revision "v1.0.0" has no values/platforms/kubevirt.yaml, values/typo.yaml, which Argo CD skips`},
		},
		{
			name: "pin to a branch Argo CD does not serve",
			cl:   fixtures.Cluster{Revision: "gh-pages"},
			want: []string{`revision "gh-pages" is not a tag, branch or commit SHA of this repository`},
		},
		{
			name: "hub and version need lifecycle and an upgrade element",
			cl:   fixtures.Cluster{Revision: "v1.0.0", Hub: "acm-dc1", Version: "4.20.8"},
			want: []string{`revision "v1.0.0" has no lifecycle/converters/hypershift/, which hub needs; an upgrade element reading version and channel`},
		},
		{name: "upgrade element under its former name", cl: fixtures.Cluster{Revision: "v1.1.0", Channel: "stable-4.20"}},
		{name: "hub at HEAD", cl: fixtures.Cluster{Revision: head, Hub: "acm-dc1", Version: "4.20.8"}},
		{
			name: "base branch outside a PR is origin's",
			cl:   fixtures.Cluster{Revision: "main", ValueFiles: []string{"platforms/kubevirt.yaml"}},
			want: []string{`revision "main" has no values/platforms/kubevirt.yaml, which Argo CD skips`},
		},
		{
			name:   "base branch in a PR is the merge commit",
			branch: "main",
			cl:     fixtures.Cluster{Revision: "main", ValueFiles: []string{"platforms/kubevirt.yaml"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.cl.Name, tc.cl.SourceFile = "prod-east-1", "fleet/prod-east-1.yaml"
			var got []string
			for _, f := range (&FleetPinCheck{Root: dir, Branch: tc.branch}).Run(Context{Clusters: []*fixtures.Cluster{&tc.cl}}) {
				if f.RuleID != "POLICY052" || f.Severity != SevError || f.Cluster != "prod-east-1" || f.File != tc.cl.SourceFile {
					t.Errorf("unexpected finding: %+v", f)
				}
				got = append(got, f.Message)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestPolicy052_ClusterFiles(t *testing.T) {
	dir := t.TempDir()
	testenv.Commit(t, dir, "stack/acm/Chart.yaml", "values/clusters/prod-east-1.yaml", "values/clusters/acm-dc1.yaml",
		"values/clusters/prod-eats-1.yaml")
	clusters := []*fixtures.Cluster{{Name: "prod-east-1"}, {Name: "acm-dc1"}}
	got := (&FleetPinCheck{Root: dir}).Run(Context{Clusters: clusters})
	if len(got) != 1 || got[0].Severity != SevWarning || got[0].File != filepath.Join(dir, "values/clusters/prod-eats-1.yaml") ||
		got[0].Message != "no fleet file reads this cluster file: fleet/prod-eats-1.yaml or fleet/hubs/prod-eats-1.yaml" {
		t.Errorf("want one warning on prod-eats-1.yaml, got %+v", got)
	}
	if got := (&FleetPinCheck{Root: dir}).Run(Context{}); len(got) != 0 {
		t.Errorf("no fleet: want no findings, got %+v", got)
	}
}
