package cascade

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
)

func TestResolve_managedCluster(t *testing.T) {
	root := mkRepo(t)
	mc := &fixtures.ManagedCluster{
		Metadata: fixtures.Metadata{
			Name: "prod-east",
			Labels: map[string]string{
				"config.example.com/environment.10": "prod",
				"config.example.com/datacenter.20":  "dc1",
				"unrelated":                          "x",
			},
		},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"), "example.com")
	if r.IsLocalHub {
		t.Fatal("should not be hub")
	}
	if r.ClusterName != "prod-east" {
		t.Fatalf("ClusterName = %q", r.ClusterName)
	}
	if r.ReleaseName != "foo-prod-east" {
		t.Fatalf("ReleaseName = %q", r.ReleaseName)
	}
	if r.Datacenter != "dc1" || r.Environment != "prod" {
		t.Fatalf("env/dc not captured: %+v", r)
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

func TestResolve_localClusterHub(t *testing.T) {
	root := mkRepo(t)
	mc := &fixtures.ManagedCluster{
		Metadata: fixtures.Metadata{
			Name: "local-cluster",
			Labels: map[string]string{
				"local-cluster":                     "true",
				"config.example.com/datacenter.10":  "dc1",
			},
		},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"), "example.com")
	if !r.IsLocalHub {
		t.Fatal("expected hub")
	}
	if r.ClusterName != "acm-dc1" {
		t.Fatalf("ClusterName = %q", r.ClusterName)
	}
	if r.ReleaseName != "foo-acm-dc1" {
		t.Fatalf("ReleaseName = %q", r.ReleaseName)
	}
	hasACM := false
	for _, f := range r.ValueFiles {
		if endsWith(f, "values/acm/acm-dc1.yaml") {
			hasACM = true
		}
	}
	if !hasACM {
		t.Fatalf("expected acm-dc1 value file in: %v", r.ValueFiles)
	}
}

func TestResolve_priorityOrdering(t *testing.T) {
	root := mkRepo(t)
	// higher priority must appear LATER in ValueFiles list
	mc := &fixtures.ManagedCluster{
		Metadata: fixtures.Metadata{
			Name: "x",
			Labels: map[string]string{
				"config.example.com/environment.30": "prod",
				"config.example.com/datacenter.10":  "dc1",
				"config.example.com/region.20":      "east",
			},
		},
	}
	// create the region file too
	mustMkdir(t, filepath.Join(root, "values/regions"))
	mustWrite(t, filepath.Join(root, "values/regions/east.yaml"), "east: true\n")
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"), "example.com")
	// expected order of label-driven: dc1 (10) < east (20) < prod (30)
	got := joinFilenames(r.ValueFiles)
	want := []string{"values.yaml", "values.yaml", "dc1.yaml", "east.yaml", "prod.yaml"}
	if len(got) < len(want) {
		t.Fatalf("not enough files: %v", got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] got %s want %s (full: %v)", i, got[i], w, got)
		}
	}
}

func TestResolve_duplicatePriority(t *testing.T) {
	root := mkRepo(t)
	mc := &fixtures.ManagedCluster{
		Metadata: fixtures.Metadata{
			Name: "x",
			Labels: map[string]string{
				"config.example.com/environment.10": "prod",
				"config.example.com/region.10":      "east",
			},
		},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"), "example.com")
	// Different categories, same priority — that's fine, no issue.
	if len(r.LabelIssues) != 0 {
		t.Fatalf("unexpected label issues: %+v", r.LabelIssues)
	}
}

func TestResolve_malformedLabel(t *testing.T) {
	root := mkRepo(t)
	mc := &fixtures.ManagedCluster{
		Metadata: fixtures.Metadata{
			Name: "x",
			Labels: map[string]string{
				"config.example.com/no-priority":   "prod",
				"config.example.com/env.notnumber": "prod",
			},
		},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"), "example.com")
	if len(r.LabelIssues) != 2 {
		t.Fatalf("expected 2 issues, got %d: %+v", len(r.LabelIssues), r.LabelIssues)
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
