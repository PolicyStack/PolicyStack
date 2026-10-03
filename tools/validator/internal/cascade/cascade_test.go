package cascade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
)

func TestResolve_managedCluster(t *testing.T) {
	root := mkRepo(t)
	mc := &fixtures.Cluster{
		Name: "prod-east",
		Config: map[string]string{
			"environment.10": "prod",
			"datacenter.20":  "dc1",
		},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
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
	mc := &fixtures.Cluster{
		Name:   "acm-hub",
		Hub:    true,
		Config: map[string]string{"datacenter.10": "dc1"},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
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
	// Higher priority must come later in ValueFiles.
	mc := &fixtures.Cluster{
		Name: "x",
		Config: map[string]string{
			"environment.30": "prod",
			"datacenter.10":  "dc1",
			"region.20":      "east",
		},
	}
	// create the region file too
	mustMkdir(t, filepath.Join(root, "values/regions"))
	mustWrite(t, filepath.Join(root, "values/regions/east.yaml"), "east: true\n")
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
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
	mc := &fixtures.Cluster{
		Name: "x",
		Config: map[string]string{
			"environment.10": "prod",
			"region.10":      "east",
		},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	// Different categories may share a priority.
	if len(r.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", r.Issues)
	}
}

func TestResolve_malformedLabel(t *testing.T) {
	root := mkRepo(t)
	mc := &fixtures.Cluster{
		Name: "x",
		Config: map[string]string{
			"no-priority":   "prod",
			"env.notnumber": "prod",
		},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	if len(r.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d: %+v", len(r.Issues), r.Issues)
	}
}

func TestResolve_caseVariantDuplicate(t *testing.T) {
	root := mkRepo(t)
	mc := &fixtures.Cluster{
		Name: "x",
		Config: map[string]string{
			"environment.10": "prod",
			"Environment.10": "prod",
		},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	if len(r.Issues) != 1 || !strings.Contains(r.Issues[0].Message, "duplicate <category>.<priority>") {
		t.Fatalf("expected one duplicate issue, got %+v", r.Issues)
	}
}

func TestResolve_fileIssuesFirst(t *testing.T) {
	root := mkRepo(t)
	fileIssue := fixtures.Issue{Line: 3, Message: "revision is empty"}
	mc := &fixtures.Cluster{
		Name:   "x",
		Config: map[string]string{"environment": "prod"},
		Issues: []fixtures.Issue{fileIssue},
	}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	if len(r.Issues) != 2 || r.Issues[0] != fileIssue {
		t.Fatalf("expected the file issue then the key issue, got %+v", r.Issues)
	}
	if len(mc.Issues) != 1 {
		t.Fatalf("Resolve modified the cluster's issues: %+v", mc.Issues)
	}
}

func TestResolve_hubWithoutDatacenter(t *testing.T) {
	root := mkRepo(t)
	mc := &fixtures.Cluster{Name: "acm-hub", Hub: true, Config: map[string]string{"environment.10": "prod"}}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	if !r.IsLocalHub || r.ClusterName != "acm-hub" || r.ReleaseName != "foo-acm-hub" {
		t.Fatalf("hub without a datacenter should keep its file name: %+v", r)
	}
	if len(r.Issues) != 1 || !strings.Contains(r.Issues[0].Message, "needs a datacenter") {
		t.Fatalf("expected a missing-datacenter issue, got %+v", r.Issues)
	}
}

func TestResolve_keySyntax(t *testing.T) {
	tests := []struct {
		key, reason string
	}{
		{"environment", "missing priority"},
		{".10", "empty category or priority"},
		{"environment.", "empty category or priority"},
		{"env.notnumber", "non-numeric priority"},
		// A label key pasted with its prefix: appset splits at the first dot.
		{"config.example.com/environment.10", "non-numeric priority"},
		{"environment.10.5", "non-numeric priority"},
		{"a/b.10", "category must be"},
		{"-env.10", "category must be"},
	}
	root := mkRepo(t)
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			mc := &fixtures.Cluster{Name: "x", Config: map[string]string{tt.key: "prod"}}
			r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
			if len(r.Issues) != 1 || !strings.Contains(r.Issues[0].Message, tt.reason) {
				t.Fatalf("want one %q issue, got %+v", tt.reason, r.Issues)
			}
		})
	}
}

func TestResolve_zeroPaddedDuplicate(t *testing.T) {
	root := mkRepo(t)
	// appset pads priorities to five characters, so 10 and 010 are one priority.
	mc := &fixtures.Cluster{Name: "x", Config: map[string]string{"environment.10": "prod", "environment.010": "dev"}}
	r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
	if len(r.Issues) != 1 || !strings.Contains(r.Issues[0].Message, "duplicate") {
		t.Fatalf("expected one duplicate issue, got %+v", r.Issues)
	}
}

func TestResolve_lastDatacenterWins(t *testing.T) {
	root := mkRepo(t)
	// appset ranges over config in key order and keeps the last datacenter.
	mc := &fixtures.Cluster{Name: "hub", Hub: true, Config: map[string]string{"datacenter.20": "dc1", "datacenter.5": "dc9"}}
	for range 20 {
		r := Resolve(mc, filepath.Join(root, "stack/foo"), "foo", root, filepath.Join(root, "values"))
		if r.Datacenter != "dc9" || r.ReleaseName != "foo-acm-dc9" {
			t.Fatalf("want datacenter dc9 from the last key, got %q (%s)", r.Datacenter, r.ReleaseName)
		}
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
