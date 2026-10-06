package cascade

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
)

func TestResolve_managedCluster(t *testing.T) {
	root := mkRepo(t)
	mc := &fixtures.Cluster{Name: "prod-east", ValueFiles: []string{"environments/prod.yaml", "datacenters/dc1.yaml"}}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	if r.ClusterName != "prod-east" || r.ReleaseName != "foo-prod-east" {
		t.Fatalf("ClusterName = %q, ReleaseName = %q", r.ClusterName, r.ReleaseName)
	}
	if len(r.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", r.Issues)
	}
	wantSuffixes := []string{
		"stack/foo/values.yaml",
		"values.yaml",
		"values/environments/prod.yaml",
		"values/datacenters/dc1.yaml",
		"values/clusters/prod-east.yaml",
	}
	if len(r.ValueFiles) != len(wantSuffixes) {
		t.Fatalf("ValueFiles len = %d; got: %v", len(r.ValueFiles), r.ValueFiles)
	}
	for i, suf := range wantSuffixes {
		if !filepath.IsAbs(r.ValueFiles[i]) {
			t.Errorf("[%d] not absolute: %s", i, r.ValueFiles[i])
		}
		if !endsWith(r.ValueFiles[i], suf) {
			t.Errorf("[%d] %s does not end with %s", i, r.ValueFiles[i], suf)
		}
	}
}

func TestResolve_hubNamedByFile(t *testing.T) {
	root := mkRepo(t)
	mustWrite(t, filepath.Join(root, "values/clusters/acm-hub.yaml"), "x: 1\n")
	// The datacenter no longer names a hub: the file name, hubName, does.
	mc := &fixtures.Cluster{Name: "acm-hub", ValueFiles: []string{"datacenters/dc1.yaml", "acm/acm-dc1.yaml"}}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	if r.ClusterName != "acm-hub" || r.ReleaseName != "foo-acm-hub" {
		t.Fatalf("ClusterName = %q, ReleaseName = %q", r.ClusterName, r.ReleaseName)
	}
	want := []string{"values.yaml", "values.yaml", "dc1.yaml", "acm-dc1.yaml", "acm-hub.yaml"}
	if got := joinFilenames(r.ValueFiles); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolve_listOrder(t *testing.T) {
	root := mkRepo(t)
	mustMkdir(t, filepath.Join(root, "values/regions"))
	mustWrite(t, filepath.Join(root, "values/regions/east.yaml"), "east: true\n")
	// List order is precedence; nothing sorts the entries.
	mc := &fixtures.Cluster{Name: "x", ValueFiles: []string{"regions/east.yaml", "environments/prod.yaml", "datacenters/dc1.yaml"}}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	want := []string{"values.yaml", "values.yaml", "east.yaml", "prod.yaml", "dc1.yaml"}
	if got := joinFilenames(r.ValueFiles); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolve_notAFile(t *testing.T) {
	root := mkRepo(t)
	for _, entry := range []string{
		"platforms/aws.yaml", // missing
		"environments/prod",  // no extension
		"environments",       // a directory
		"",                   // values/ itself
	} {
		t.Run(entry, func(t *testing.T) {
			mc := &fixtures.Cluster{Name: "x", ValueFiles: []string{entry, "environments/prod.yaml"}}
			r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
			if len(r.Issues) != 1 || !strings.Contains(r.Issues[0].Message, "not a file under values/") {
				t.Fatalf("want one not-a-file issue, got %+v", r.Issues)
			}
			want := []string{"values.yaml", "values.yaml", "prod.yaml"}
			if got := joinFilenames(r.ValueFiles); !slices.Equal(got, want) {
				t.Fatalf("the bad entry should be left out: got %v, want %v", got, want)
			}
		})
	}
}

func TestResolve_fileIssuesFirst(t *testing.T) {
	root := mkRepo(t)
	fileIssue := fixtures.Issue{Line: 3, Message: "revision is empty"}
	mc := &fixtures.Cluster{
		Name:       "x",
		ValueFiles: []string{"environments/missing.yaml"},
		Issues:     []fixtures.Issue{fileIssue},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	if len(r.Issues) != 2 || r.Issues[0] != fileIssue {
		t.Fatalf("expected the file issue then the entry issue, got %+v", r.Issues)
	}
	if len(mc.Issues) != 1 {
		t.Fatalf("Resolve modified the cluster's issues: %+v", mc.Issues)
	}
}

func TestResolve_noValueFiles(t *testing.T) {
	root := mkRepo(t)
	r := Resolve(&fixtures.Cluster{Name: "prod-east"}, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	want := []string{"values.yaml", "values.yaml", "prod-east.yaml"}
	if got := joinFilenames(r.ValueFiles); !slices.Equal(got, want) || len(r.Issues) != 0 {
		t.Fatalf("got %v %+v, want %v", got, r.Issues, want)
	}
}

// helpers

func mkRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "values.yaml"), "policyNamespace: policy\n")
	mustMkdir(t, filepath.Join(root, "stack/foo"))
	mustWrite(t, filepath.Join(root, "stack/foo/values.yaml"), "stack: {}\n")
	mustMkdir(t, filepath.Join(root, "values/environments"))
	mustWrite(t, filepath.Join(root, "values/environments/prod.yaml"), "x: 1\n")
	mustMkdir(t, filepath.Join(root, "values/datacenters"))
	mustWrite(t, filepath.Join(root, "values/datacenters/dc1.yaml"), "x: 1\n")
	mustMkdir(t, filepath.Join(root, "values/clusters"))
	mustWrite(t, filepath.Join(root, "values/clusters/prod-east.yaml"), "x: 1\n")
	mustMkdir(t, filepath.Join(root, "values/acm"))
	mustWrite(t, filepath.Join(root, "values/acm/acm-dc1.yaml"), "x: 1\n")
	return root
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func endsWith(s, suf string) bool {
	return len(s) >= len(suf) && s[len(s)-len(suf):] == suf
}

func joinFilenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}

// TestResolve_testdata pins the example fleet files' cascades, the ones docs/values.md shows.
func TestResolve_testdata(t *testing.T) {
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	clusters, err := fixtures.LoadDir(filepath.Join(repo, "tools/validator/testdata/clusters"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"foo-prod-east-1": {
			"stack/foo/values.yaml", "values.yaml", "values/environments/prod.yaml",
			"values/datacenters/dc1.yaml", "values/platforms/aws.yaml", "values/clusters/prod-east-1.yaml",
		},
		"foo-nonprod-west-1": {
			"stack/foo/values.yaml", "values.yaml", "values/environments/nonprod.yaml",
			"values/datacenters/dc2.yaml", "values/clusters/nonprod-west-1.yaml",
		},
		"foo-hcp-agent": {
			"stack/foo/values.yaml", "values.yaml", "values/environments/prod.yaml",
			"values/datacenters/dc1.yaml", "values/platforms/agent.yaml", "values/clusters/hcp-agent.yaml",
		},
		"foo-hcp-kubevirt": {
			"stack/foo/values.yaml", "values.yaml", "values/environments/nonprod.yaml", "values/platforms/kubevirt.yaml",
		},
		"foo-hcp-retired": {
			"stack/foo/values.yaml", "values.yaml", "values/environments/nonprod.yaml", "values/platforms/kubevirt.yaml",
		},
		"foo-acm-dc1": {
			"stack/foo/values.yaml", "values.yaml", "values/environments/prod.yaml",
			"values/datacenters/dc1.yaml", "values/platforms/baremetal.yaml",
			"values/acm/acm-dc1.yaml", "values/clusters/acm-dc1.yaml",
		},
	}
	// The real repo has no stack/foo, so the element defaults come from a temp dir.
	element := filepath.Join(t.TempDir(), "stack/foo")
	mustMkdir(t, element)
	mustWrite(t, filepath.Join(element, "values.yaml"), "stack: {}\n")
	got := map[string][]string{}
	for _, c := range clusters {
		r := Resolve(c, element, "foo", repo, filepath.Join(repo, "values"))
		if len(r.Issues) != 0 {
			t.Errorf("%s: unexpected issues %+v", c.SourceFile, r.Issues)
		}
		files := []string{"stack/foo/values.yaml"}
		for _, f := range r.ValueFiles[1:] {
			rel, err := filepath.Rel(repo, f)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, rel)
		}
		got[r.ReleaseName] = files
	}
	if !maps.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("cascades differ\n got: %v\nwant: %v", got, want)
	}
}
