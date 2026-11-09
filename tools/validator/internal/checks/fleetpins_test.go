package checks

import (
	"os"
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
	head := testenv.Commit(t, dir, "values/platforms/kubevirt.yaml", "lifecycle/converters/_hypershift/root.yaml",
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
			want: []string{`revision "v1.0.0" has no lifecycle/converters/_hypershift/, which hub needs; an upgrade element reading version and channel`},
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

func TestPolicy052_FrameworkSkew(t *testing.T) {
	dir := t.TempDir()
	answers := func(commit string) {
		t.Helper()
		data := "_commit: " + commit + "\n_src_path: https://github.com/PolicyStack/PolicyStack\n"
		if err := os.WriteFile(filepath.Join(dir, answersFile), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	testenv.Commit(t, dir, "stack/acm/Chart.yaml")
	testenv.Git(t, dir, "tag", "pre-copier")
	for _, tag := range []string{"v1.4.0", "v2.0.0", "v3.1.0", "v3.0.0-2-gabc1234", "acm-v3.0.0", "v4.0.0-rc.1"} {
		answers(tag)
		testenv.Commit(t, dir)
		testenv.Git(t, dir, "tag", "at-"+tag)
	}

	for _, tc := range []struct {
		name, head, rev string
		want            []string
	}{
		{name: "same major", head: "v3.2.0", rev: "at-v3.1.0"},
		{name: "one major behind", head: "v3.2.0", rev: "at-v2.0.0"},
		{name: "revision before the copier copy", head: "v3.2.0", rev: "pre-copier"},
		{name: "not a copier copy", rev: "at-v1.4.0"},
		{name: "unreadable framework release here is POLICY061's", head: "v3.2.0-1-g1234567", rev: "at-v1.4.0"},
		{
			name: "two majors behind", head: "v3.2.0", rev: "at-v1.4.0",
			want: []string{`revision "at-v1.4.0" is framework v1.4.0, more than one major behind this branch's v3.2.0: hubs render this major and the one before only. Move the revision forward`},
		},
		{
			name: "newer major", head: "v2.5.0", rev: "at-v3.1.0",
			want: []string{`revision "at-v3.1.0" is framework v3.1.0, a newer major than this branch's v2.5.0: hubs run this branch's install chart, which cannot render it. Merge the framework update first`},
		},
		{name: "prerelease of the next major is newer", head: "v3.2.0", rev: "at-v4.0.0-rc.1", want: []string{
			`revision "at-v4.0.0-rc.1" is framework v4.0.0-rc.1, a newer major than this branch's v3.2.0: hubs run this branch's install chart, which cannot render it. Merge the framework update first`,
		}},
		{
			name: "git describe output", head: "v3.2.0", rev: "at-v3.0.0-2-gabc1234",
			want: []string{`revision "at-v3.0.0-2-gabc1234" has no framework release tag in /.copier-answers.yml (_commit "v3.0.0-2-gabc1234")`},
		},
		{
			name: "prefixed tag at the root", head: "v3.2.0", rev: "at-acm-v3.0.0",
			want: []string{`revision "at-acm-v3.0.0" has no framework release tag in /.copier-answers.yml (_commit "acm-v3.0.0")`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.head == "" {
				if err := os.Remove(filepath.Join(dir, answersFile)); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
			} else {
				answers(tc.head)
			}
			cl := &fixtures.Cluster{Name: "prod-east-1", SourceFile: "fleet/prod-east-1.yaml", Revision: tc.rev}
			var got []string
			for _, f := range (&FleetPinCheck{Root: dir}).Run(Context{Clusters: []*fixtures.Cluster{cl}}) {
				if f.Severity != SevError {
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

// TestPolicy052_HubInstall: a hub's install runs its installRevision, or this branch, and must be at
// least as new as every revision it renders.
func TestPolicy052_HubInstall(t *testing.T) {
	dir := t.TempDir()
	answers := func(commit string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, answersFile), []byte("_commit: "+commit+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	answers("v1.0.0")
	testenv.Commit(t, dir, "stack/acm/Chart.yaml", "lifecycle/converters/_hypershift/root.yaml")
	testenv.Git(t, dir, "tag", "at-v1.0.0")
	answers("v1.1.0")
	testenv.Commit(t, dir, "install/templates/install.yaml")
	testenv.Git(t, dir, "tag", "at-v1.1.0")
	if err := os.Remove(filepath.Join(dir, answersFile)); err != nil {
		t.Fatal(err)
	}
	testenv.Commit(t, dir)
	testenv.Git(t, dir, "tag", "at-none")
	answers("v1.2.0")
	testenv.Commit(t, dir)
	testenv.Git(t, dir, "tag", "at-v1.2.0")

	hub := func(rev, install string) *fixtures.Cluster {
		return &fixtures.Cluster{Name: "gh", SourceFile: "fleet/hubs/gh.yaml", IsHub: true, Revision: rev, InstallRevision: install}
	}
	spoke := func(rev, builder string) *fixtures.Cluster {
		return &fixtures.Cluster{Name: "c1", SourceFile: "fleet/c1.yaml", Revision: rev, Hub: builder}
	}
	type finding struct {
		sev Severity
		msg string
	}
	for _, tc := range []struct {
		name     string
		clusters []*fixtures.Cluster
		want     []finding
	}{
		{name: "every install on this branch", clusters: []*fixtures.Cluster{hub("at-v1.2.0", ""), spoke("at-v1.2.0", "gh")}},
		{name: "revisions older than the install", clusters: []*fixtures.Cluster{hub("at-v1.0.0", "at-v1.1.0"), spoke("at-v1.1.0", "gh")}},
		{
			name:     "built cluster newer than its hub's install",
			clusters: []*fixtures.Cluster{hub("at-v1.1.0", "at-v1.1.0"), spoke("at-v1.2.0", "gh")},
			want: []finding{{SevError, `revision "at-v1.2.0" is framework v1.2.0, newer than hub gh's install, framework v1.1.0 at ` +
				`installRevision "at-v1.1.0", which renders it: upgrade the hub first`}},
		},
		{
			// An imported Global Hub child's hub file is rendered by its parent, which Git doesn't name.
			name:     "hub's own revision newer than its install",
			clusters: []*fixtures.Cluster{hub("at-v1.2.0", "at-v1.1.0")},
			want: []finding{{SevWarning, `revision "at-v1.2.0" is framework v1.2.0, newer than hub gh's install, framework v1.1.0 at ` +
				`installRevision "at-v1.1.0", which renders it unless a Global Hub parent does: upgrade the hub first`}},
		},
		{
			// hub-a is built by gh, so it is gh's default-mode child: gh renders its hub file and builds its
			// clusters, whatever hub-a's own install.
			name: "Global Hub child built by its parent",
			clusters: []*fixtures.Cluster{
				hub("at-v1.1.0", "at-v1.1.0"),
				{Name: "hub-a", SourceFile: "fleet/hubs/hub-a.yaml", IsHub: true, Revision: "at-v1.2.0", InstallRevision: "at-v1.2.0"},
				{Name: "hub-a", SourceFile: "fleet/hub-a.yaml", Revision: "at-v1.1.0", Hub: "gh"},
				{Name: "edge-1", SourceFile: "fleet/edge-1.yaml", Revision: "at-v1.2.0", Hub: "hub-a"},
			},
			want: []finding{
				{SevError, `revision "at-v1.2.0" is framework v1.2.0, newer than hub gh's install, framework v1.1.0 at ` +
					`installRevision "at-v1.1.0", which renders it as its Global Hub child: upgrade that hub first`},
				{SevError, `revision "at-v1.2.0" is framework v1.2.0, newer than hub gh's install, framework v1.1.0 at ` +
					`installRevision "at-v1.1.0", which builds it for its Global Hub child hub-a: upgrade that hub first`},
			},
		},
		{
			name:     "import-only cluster newer than the oldest hub install",
			clusters: []*fixtures.Cluster{hub("at-v1.1.0", "at-v1.1.0"), spoke("at-v1.2.0", "")},
			want: []finding{{SevWarning, `revision "at-v1.2.0" is framework v1.2.0, newer than hub gh's install, framework v1.1.0 at ` +
				`installRevision "at-v1.1.0", the oldest hub install: upgrade every hub that manages this cluster first`}},
		},
		{
			name:     "installRevision without the pointer",
			clusters: []*fixtures.Cluster{hub("at-v1.0.0", "at-v1.0.0")},
			want: []finding{{SevError, `installRevision "at-v1.0.0" has no install/templates/install.yaml, the pointer that keeps ` +
				`this hub's install in git: pin a release that has it`}},
		},
		{
			name:     "installRevision with no framework release",
			clusters: []*fixtures.Cluster{hub("at-v1.1.0", "at-none")},
			want: []finding{{SevWarning, `installRevision "at-none" records no framework release tag in /.copier-answers.yml: ` +
				`the revisions this hub renders are not checked against it`}},
		},
		{
			name:     "installRevision that does not resolve",
			clusters: []*fixtures.Cluster{hub("at-v1.0.0", "nope")},
			want:     []finding{{SevError, `installRevision: revision "nope" is not a tag, branch or commit SHA of this repository`}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []finding
			for _, f := range (&FleetPinCheck{Root: dir}).Run(Context{Clusters: tc.clusters}) {
				got = append(got, finding{f.Severity, f.Message})
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}
