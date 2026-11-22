package checks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

// TestPolicy053_Report: everything warns for now. Outage and safety rules say when they start to
// fail, and warnings at the commit are left out.
func TestPolicy053_Report(t *testing.T) {
	clusters := []*fixtures.Cluster{
		{Name: "prod-east-1", Revision: "v1.5.0", SourceFile: "fleet/prod-east-1.yaml"},
		{Name: "prod-west-1", Revision: "v1.5.0", SourceFile: "fleet/prod-west-1.yaml"},
	}
	found := []Finding{
		{RuleID: "RENDER000", Severity: SevError, Cluster: "prod-west-1", Message: "failed", File: "stack/odf/values.yaml", Line: 3},
		{RuleID: "POLICY037", Severity: SevError, Message: "destructive"},
		{RuleID: "POLICY041", Severity: SevError, Message: "ORed"},
		{RuleID: "POLICY042", Severity: SevError, Message: "gate"},
		{RuleID: "POLICY035", Severity: SevError, Message: "typo", File: "values/environments/prod.yaml"},
		{RuleID: "POLICY092", Severity: SevWarning, Message: "deprecated"},
	}
	want := []Finding{
		{Severity: SevWarning, Cluster: "prod-west-1", File: "fleet/prod-west-1.yaml", Message: `revision "v1.5.0" fails RENDER000 at stack/odf/values.yaml:3: failed` + promotionNotice},
		{Severity: SevWarning, File: "fleet/prod-east-1.yaml", Message: `revision "v1.5.0" fails POLICY037: destructive` + promotionNotice},
		{Severity: SevWarning, File: "fleet/prod-east-1.yaml", Message: `revision "v1.5.0" fails POLICY041: ORed` + promotionNotice},
		{Severity: SevWarning, File: "fleet/prod-east-1.yaml", Message: `revision "v1.5.0" fails POLICY042: gate` + promotionNotice},
		{Severity: SevWarning, File: "fleet/prod-east-1.yaml", Message: `revision "v1.5.0" fails POLICY035 at values/environments/prod.yaml: typo`},
	}
	for i := range want {
		want[i].RuleID = "POLICY053"
	}
	if got := (&PromotionCheck{}).report(clusters, found); !slices.Equal(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	// With one cluster pinning the commit, every finding is that cluster's.
	if got := (&PromotionCheck{}).report(clusters[:1], found[1:2]); len(got) != 1 || got[0].Cluster != "prod-east-1" {
		t.Errorf("single cluster: got %+v", got)
	}
}

// TestPolicy053_Unreadable: a commit that can't be checked is reported, and is to fail, since no
// safety rule ran on it.
func TestPolicy053_Unreadable(t *testing.T) {
	cl := &fixtures.Cluster{Name: "prod", Revision: "v1", SourceFile: "fleet/prod.yaml"}
	got := (&PromotionCheck{Root: t.TempDir()}).validate(t.Context(), "absent", []*fixtures.Cluster{cl})
	if len(got) != 1 || got[0].Severity != SevWarning || !strings.HasSuffix(got[0].Message, promotionNotice) || got[0].Cluster != "prod" || got[0].File != cl.SourceFile {
		t.Errorf("got %+v, want one warning on the fleet file", got)
	}
}

func TestPolicy053_Promotion(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("marker", "bad")
	write("fleet/prod.yaml", "revision: v1\n")
	testenv.Commit(t, dir)
	testenv.Git(t, dir, "tag", "v1")
	write("marker", "good")
	testenv.Commit(t, dir)
	testenv.Git(t, dir, "tag", "v2")
	write("marker", "head")
	head := testenv.Commit(t, dir)
	testenv.Git(t, dir, "update-ref", "refs/remotes/origin/main", head)
	t.Chdir(dir) // fleet paths are relative, as make passes them

	for _, tc := range []struct {
		name, fleet, rev, branch, def string
		fail                          error
		want                          []string // the trees validated, then the messages reported
	}{
		{name: "unchanged pin", fleet: "revision: v1\n", rev: "v1"},
		{name: "pin of HEAD", fleet: "revision: main\n", rev: "main", branch: "main"},
		{name: "unresolved is POLICY052's", fleet: "revision: v9\n", rev: "v9"},
		{name: "no revision is POLICY050's", fleet: "{}\n"},
		{
			name: "promotion validates the pinned tree", fleet: "revision: v2\n", rev: "v2", branch: "main",
			want: []string{"good", `revision "v2" fails RENDER000 at marker: marker` + promotionNotice},
		},
		{
			name: "an edit beside an unchanged pin", fleet: "revision: v1\nvalueFiles: []\n", rev: "v1",
			want: []string{"bad", `revision "v1" fails RENDER000 at marker: marker` + promotionNotice},
		},
		{
			name: "a PR to another branch compares with the default", fleet: "revision: v2\n", rev: "v2", branch: "stacked", def: "main",
			want: []string{"good", `revision "v2" fails RENDER000 at marker: marker` + promotionNotice},
		},
		{
			name: "validator failure is reported", fleet: "revision: v2\n", rev: "v2", fail: errors.New("cannot load"),
			want: []string{"good", `revision "v2" could not be validated: cannot load` + promotionNotice},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			write("fleet/prod.yaml", tc.fleet)
			var got []string
			c := &PromotionCheck{Root: dir, Branch: tc.branch, DefaultBranch: tc.def, Fleet: "fleet"}
			c.Validate = func(_ context.Context, root, fleet string) ([]Finding, error) {
				marker, err := os.ReadFile(filepath.Join(root, "marker"))
				if err != nil {
					return nil, err
				}
				if f, err := os.ReadFile(filepath.Join(fleet, "prod.yaml")); err != nil || string(f) != tc.fleet {
					t.Errorf("fleet file %q, %v; want the working tree's", f, err)
				}
				got = append(got, string(marker))
				return []Finding{{RuleID: "RENDER000", Severity: SevError, Cluster: "prod", File: filepath.Join(root, "marker"),
					Message: root + string(filepath.Separator) + "marker"}}, tc.fail
			}
			cl := &fixtures.Cluster{Name: "prod", Revision: tc.rev, SourceFile: filepath.Join("fleet", "prod.yaml")}
			for _, f := range c.Run(Context{Clusters: []*fixtures.Cluster{cl}}) {
				if f.Severity != SevWarning {
					t.Errorf("severity %s, want warning: %s", f.Severity, f.Message)
				}
				got = append(got, f.Message)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}
