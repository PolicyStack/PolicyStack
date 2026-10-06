package fixtures

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadDir(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string // path under the dir -> content; a trailing "/" makes a directory
		want  []Cluster         // Name and SourceFile (relative) are compared, in order
	}{
		{
			name: "spokes then hubs, named by file stem",
			files: map[string]string{
				"prod-east-1.yaml":  "revision: main\n",
				"a.yaml":            "revision: main\n",
				"hubs/acm-dc1.yaml": "revision: main\n",
			},
			want: []Cluster{
				{Name: "a", SourceFile: "a.yaml"},
				{Name: "prod-east-1", SourceFile: "prod-east-1.yaml"},
				{Name: "acm-dc1", SourceFile: "hubs/acm-dc1.yaml"},
			},
		},
		{
			name:  "missing hubs dir",
			files: map[string]string{"prod-east-1.yaml": "revision: main\n"},
			want:  []Cluster{{Name: "prod-east-1", SourceFile: "prod-east-1.yaml"}},
		},
		{
			name: "only .yaml files are read",
			files: map[string]string{
				"prod-east-1.yaml": "revision: main\n",
				"other.yml":        "revision: main\n",
				"README.md":        "# fleet\n",
				"dir.yaml/":        "",
				"hubs/other.yml":   "revision: main\n",
				"hubs/nested/":     "",
			},
			want: []Cluster{{Name: "prod-east-1", SourceFile: "prod-east-1.yaml"}},
		},
		{
			name:  "hub and spoke may share a name",
			files: map[string]string{"x.yaml": "revision: main\n", "hubs/x.yaml": "revision: main\n"},
			want:  []Cluster{{Name: "x", SourceFile: "x.yaml"}, {Name: "x", SourceFile: "hubs/x.yaml"}},
		},
		{
			name:  "empty dir",
			files: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, tt.files)
			got, err := LoadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d clusters, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Name != w.Name || g.SourceFile != filepath.Join(dir, w.SourceFile) {
					t.Errorf("[%d] got {%s %s}, want {%s %s}", i, g.Name, g.SourceFile, w.Name, w.SourceFile)
				}
			}
		})
	}
}

// Checks that need the hub files: hub names one, hub files cannot set hub, and a cluster a hub
// builds needs a DNS label name.
func TestLoadDir_hub(t *testing.T) {
	const hubFile = "revision: main\n"
	tests := []struct {
		name   string
		files  map[string]string
		hubs   map[string]string  // relative SourceFile -> Hub; unlisted files want ""
		issues map[string][]Issue // relative SourceFile -> issues; unlisted files want none
	}{
		{
			name:  "hub names a hub file",
			files: map[string]string{"c1.yaml": "revision: main\nhub: acm-dc1\n", "hubs/acm-dc1.yaml": hubFile},
			hubs:  map[string]string{"c1.yaml": "acm-dc1"},
		},
		{
			name:  "absent cluster",
			files: map[string]string{"c1.yaml": "revision: main\nhub: acm-dc1\nstate: absent\n", "hubs/acm-dc1.yaml": hubFile},
			hubs:  map[string]string{"c1.yaml": "acm-dc1"},
		},
		{
			name:   "hub with no hub file",
			files:  map[string]string{"c1.yaml": "revision: main\nhub: acm-dc2\n", "hubs/acm-dc1.yaml": hubFile},
			hubs:   map[string]string{"c1.yaml": "acm-dc2"},
			issues: map[string][]Issue{"c1.yaml": {{Line: 2, Message: `hub "acm-dc2" has no hubs/acm-dc2.yaml`}}},
		},
		{
			name:   "no hubs dir",
			files:  map[string]string{"c1.yaml": "revision: main\nhub: acm-dc1\n"},
			hubs:   map[string]string{"c1.yaml": "acm-dc1"},
			issues: map[string][]Issue{"c1.yaml": {{Line: 2, Message: `hub "acm-dc1" has no hubs/acm-dc1.yaml`}}},
		},
		{
			// A spoke with the hub's name is not a hub file.
			name:   "hub names a spoke",
			files:  map[string]string{"c1.yaml": "revision: main\nhub: c2\n", "c2.yaml": hubFile},
			hubs:   map[string]string{"c1.yaml": "c2"},
			issues: map[string][]Issue{"c1.yaml": {{Line: 2, Message: `hub "c2" has no hubs/c2.yaml`}}},
		},
		{
			name: "hub file sets hub",
			files: map[string]string{
				"hubs/acm-dc1.yaml": "revision: main\nhub: acm-dc2\n",
				"hubs/acm-dc2.yaml": hubFile,
			},
			issues: map[string][]Issue{"hubs/acm-dc1.yaml": {{Line: 2, Message: "hub is not allowed in a hub file"}}},
		},
		{
			name: "hub file sets an empty hub",
			files: map[string]string{
				"hubs/acm-dc1.yaml": "revision: main\nhub: \"\"\n",
			},
			issues: map[string][]Issue{"hubs/acm-dc1.yaml": {
				{Line: 2, Message: "hub is empty"},
				{Line: 2, Message: "hub is not allowed in a hub file"},
			}},
		},
		{
			name: "cluster name not a DNS label",
			files: map[string]string{
				"Prod_East.yaml":    "revision: main\nhub: acm-dc1\n",
				"prod.east.yaml":    "revision: main\nhub: acm-dc1\n",
				"-prod.yaml":        "revision: main\nhub: acm-dc1\n",
				"hubs/acm-dc1.yaml": hubFile,
			},
			hubs: map[string]string{"Prod_East.yaml": "acm-dc1", "prod.east.yaml": "acm-dc1", "-prod.yaml": "acm-dc1"},
			issues: map[string][]Issue{
				"Prod_East.yaml": {{Message: `cluster name "Prod_East" is not a DNS label`}},
				"prod.east.yaml": {{Message: `cluster name "prod.east" is not a DNS label`}},
				"-prod.yaml":     {{Message: `cluster name "-prod" is not a DNS label`}},
			},
		},
		{
			name: "64-character cluster name",
			files: map[string]string{
				strings.Repeat("a", 64) + ".yaml": "revision: main\nhub: acm-dc1\n",
				strings.Repeat("b", 63) + ".yaml": "revision: main\nhub: acm-dc1\n",
				"hubs/acm-dc1.yaml":               hubFile,
			},
			hubs: map[string]string{strings.Repeat("a", 64) + ".yaml": "acm-dc1", strings.Repeat("b", 63) + ".yaml": "acm-dc1"},
			issues: map[string][]Issue{
				strings.Repeat("a", 64) + ".yaml": {{Message: "is not a DNS label"}},
			},
		},
		{
			// Import-only clusters keep whatever name ACM accepts.
			name:  "import-only cluster name not checked",
			files: map[string]string{"Prod_East.yaml": "revision: main\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, tt.files)
			got, err := LoadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range got {
				rel, err := filepath.Rel(dir, c.SourceFile)
				if err != nil {
					t.Fatal(err)
				}
				if c.Hub != tt.hubs[rel] {
					t.Errorf("%s: Hub = %q, want %q", rel, c.Hub, tt.hubs[rel])
				}
				want := tt.issues[rel]
				if len(c.Issues) != len(want) {
					t.Errorf("%s: got %d issues, want %d: %+v", rel, len(c.Issues), len(want), c.Issues)
					continue
				}
				for i, w := range want {
					g := c.Issues[i]
					if g.Line != w.Line || !strings.Contains(g.Message, w.Message) {
						t.Errorf("%s [%d] got {%d %q}, want {%d %q}", rel, i, g.Line, g.Message, w.Line, w.Message)
					}
				}
			}
		})
	}
}

func TestLoadDir_missingDir(t *testing.T) {
	if _, err := LoadDir(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error for a missing dir")
	}
}

// The shipped fixtures are the documented fleet file examples, so they must load cleanly.
func TestLoadDir_testdata(t *testing.T) {
	got, err := LoadDir("../../testdata/clusters")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ name, hub, state string }{
		{"hcp-agent", "acm-dc1", ""},
		{"hcp-kubevirt", "acm-dc1", ""},
		{"hcp-retired", "acm-dc1", "absent"},
		{"nonprod-west-1", "", ""},
		{"prod-east-1", "", ""},
		{"acm-dc1", "", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d clusters, want %d", len(got), len(want))
	}
	for i, c := range got {
		w := want[i]
		if c.Name != w.name || c.Hub != w.hub || c.State != w.state ||
			len(c.Issues) != 0 || c.Revision != "main" || len(c.ValueFiles) == 0 {
			t.Errorf("unexpected fixture %+v", c)
		}
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		revision   string
		valueFiles []string
		hub, state string
		issues     []Issue // Message is a substring of the actual message
	}{
		{
			name:       "valid",
			in:         "revision: main\nvalueFiles:\n  - environments/prod.yaml\n  - datacenters/dc1.yaml\n",
			revision:   "main",
			valueFiles: []string{"environments/prod.yaml", "datacenters/dc1.yaml"},
		},
		{
			name:       "list order kept",
			in:         "revision: main\nvalueFiles:\n  - platforms/aws.yaml\n  - environments/prod.yaml\n",
			revision:   "main",
			valueFiles: []string{"platforms/aws.yaml", "environments/prod.yaml"},
		},
		{name: "missing valueFiles", in: "revision: v1.4.0\n", revision: "v1.4.0"},
		{name: "empty valueFiles", in: "revision: v1.4.0\nvalueFiles: []\n", revision: "v1.4.0"},
		{name: "null valueFiles", in: "revision: v1.4.0\nvalueFiles:\n", revision: "v1.4.0"},
		{name: "quoted numeric revision", in: "revision: \"1.10\"\n", revision: "1.10"},
		{name: "tagged numeric revision", in: "revision: !!str 1.10\n", revision: "1.10"},
		{name: "revision with slash", in: "revision: feature/fleet-files\n", revision: "feature/fleet-files"},
		{
			name:   "numeric revision",
			in:     "revision: 1.10\n",
			issues: []Issue{{Line: 1, Message: "revision is read by Argo CD as 1.1 (float64), not a string"}},
		},
		{
			name:   "all-digit SHA",
			in:     "revision: 1234567\n",
			issues: []Issue{{Line: 1, Message: "revision is read by Argo CD as 1.234567e+06 (float64)"}},
		},
		{
			name:       "missing revision",
			in:         "valueFiles:\n  - environments/prod.yaml\n",
			valueFiles: []string{"environments/prod.yaml"},
			issues:     []Issue{{Message: "revision is required"}},
		},
		{name: "null revision", in: "revision:\n", issues: []Issue{{Line: 1, Message: "revision is required"}}},
		{name: "tilde revision", in: "revision: ~\n", issues: []Issue{{Line: 1, Message: "revision is required"}}},
		{name: "bool revision", in: "revision: true\n", issues: []Issue{{Line: 1, Message: "revision is read by Argo CD as true (bool)"}}},
		{name: "exponent revision", in: "revision: 1e3\n", issues: []Issue{{Line: 1, Message: "revision is read by Argo CD as 1000 (float64)"}}},
		// YAML 1.1 booleans: plain strings to yaml.v3, booleans to Argo CD.
		{name: "yes revision", in: "revision: yes\n", issues: []Issue{{Line: 1, Message: "(bool)"}}},
		{name: "empty file", in: "", issues: []Issue{{Message: "revision is required"}}},
		{name: "comment-only file", in: "# nothing\n", issues: []Issue{{Message: "revision is required"}}},
		{name: "empty revision", in: "revision: \"\"\n", issues: []Issue{{Line: 1, Message: "revision is empty"}}},
		{
			name:   "map revision",
			in:     "revision:\n  branch: main\n",
			issues: []Issue{{Line: 2, Message: "revision is read by Argo CD as map[branch:main]"}},
		},
		{
			name:     "unknown key",
			in:       "revision: main\nlabels:\n  a: b\n",
			revision: "main",
			issues:   []Issue{{Line: 2, Message: "unknown key labels: only revision, valueFiles, hub, state and install are allowed"}},
		},
		{
			name:     "old config key",
			in:       "revision: main\nconfig:\n  environment.10: prod\n",
			revision: "main",
			issues:   []Issue{{Line: 2, Message: "unknown key config: only revision, valueFiles, hub, state and install are allowed"}},
		},
		{
			name: "misspelt revision",
			in:   "revison: main\n",
			issues: []Issue{
				{Line: 1, Message: "unknown key revison"},
				{Message: "revision is required"},
			},
		},
		{
			// Argo CD's parser keeps the last value.
			name:     "duplicate key",
			in:       "revision: a\nrevision: b\n",
			revision: "b",
			issues:   []Issue{{Line: 2, Message: `mapping key "revision" already defined at line 1`}},
		},
		{
			name:       "numeric entry",
			in:         "revision: main\nvalueFiles:\n  - 1.10\n  - environments/prod.yaml\n",
			revision:   "main",
			valueFiles: []string{"environments/prod.yaml"},
			issues:     []Issue{{Line: 3, Message: "valueFiles entry is read by Argo CD as 1.1 (float64), not a string"}},
		},
		{
			name:     "YAML 1.1 boolean entries",
			in:       "revision: main\nvalueFiles:\n  - yes\n  - off\n",
			revision: "main",
			issues: []Issue{
				{Line: 3, Message: "valueFiles entry is read by Argo CD as true (bool)"},
				{Line: 4, Message: "valueFiles entry is read by Argo CD as false (bool)"},
			},
		},
		{
			name:       "quoted numeric entry",
			in:         "revision: main\nvalueFiles:\n  - \"1.10\"\n",
			revision:   "main",
			valueFiles: []string{"1.10"},
		},
		{
			name:       "null entry",
			in:         "revision: main\nvalueFiles:\n  -\n  - environments/prod.yaml\n",
			revision:   "main",
			valueFiles: []string{"environments/prod.yaml"},
			issues:     []Issue{{Line: 3, Message: "valueFiles entry has no value"}},
		},
		{
			name:       "duplicate entry",
			in:         "revision: main\nvalueFiles:\n  - environments/prod.yaml\n  - platforms/aws.yaml\n  - environments/prod.yaml\n",
			revision:   "main",
			valueFiles: []string{"environments/prod.yaml", "platforms/aws.yaml"},
			issues:     []Issue{{Line: 5, Message: `valueFiles "environments/prod.yaml" is listed twice`}},
		},
		{
			name:     "valueFiles is a map",
			in:       "revision: main\nvalueFiles:\n  environment.10: prod\n",
			revision: "main",
			issues:   []Issue{{Line: 3, Message: "cannot unmarshal !!map"}},
		},
		{
			name:     "valueFiles is a string",
			in:       "revision: main\nvalueFiles: environments/prod.yaml\n",
			revision: "main",
			issues:   []Issue{{Line: 2, Message: "cannot unmarshal !!str"}},
		},
		{
			name: "list instead of a map",
			in:   "- revision: main\n",
			issues: []Issue{
				{Line: 1, Message: "cannot unmarshal !!seq"},
				{Message: "revision is required"},
			},
		},
		{
			name:       "hub-built cluster",
			in:         "revision: main\nhub: acm-dc1\nstate: present\nvalueFiles:\n  - platforms/kubevirt.yaml\ninstall:\n  version: 4.20.8\n",
			revision:   "main",
			valueFiles: []string{"platforms/kubevirt.yaml"},
			hub:        "acm-dc1",
			state:      "present",
		},
		{name: "absent", in: "revision: main\nhub: acm-dc1\nstate: absent\n", revision: "main", hub: "acm-dc1", state: "absent"},
		{name: "empty install", in: "revision: main\nhub: acm-dc1\ninstall: {}\n", revision: "main", hub: "acm-dc1"},
		{name: "quoted numeric hub", in: "revision: main\nhub: \"1.10\"\n", revision: "main", hub: "1.10"},
		{
			name:     "numeric hub",
			in:       "revision: main\nhub: 1.10\n",
			revision: "main",
			issues:   []Issue{{Line: 2, Message: "hub is read by Argo CD as 1.1 (float64), not a string"}},
		},
		{name: "yes hub", in: "revision: main\nhub: yes\n", revision: "main", issues: []Issue{{Line: 2, Message: "hub is read by Argo CD as true (bool)"}}},
		{name: "empty hub", in: "revision: main\nhub: \"\"\n", revision: "main", issues: []Issue{{Line: 2, Message: "hub is empty"}}},
		{name: "null hub", in: "revision: main\nhub:\n", revision: "main", issues: []Issue{{Line: 2, Message: "hub has no value"}}},
		{name: "map hub", in: "revision: main\nhub:\n  name: acm-dc1\n", revision: "main", issues: []Issue{{Line: 3, Message: "hub is read by Argo CD as map[name:acm-dc1]"}}},
		{
			name:     "unknown state",
			in:       "revision: main\nhub: acm-dc1\nstate: deleted\n",
			revision: "main",
			hub:      "acm-dc1",
			state:    "deleted",
			issues:   []Issue{{Line: 3, Message: `state "deleted" must be present or absent`}},
		},
		{
			name:     "state is case-sensitive",
			in:       "revision: main\nhub: acm-dc1\nstate: Absent\n",
			revision: "main",
			hub:      "acm-dc1",
			state:    "Absent",
			issues:   []Issue{{Line: 3, Message: `state "Absent" must be present or absent`}},
		},
		{
			name:     "empty state",
			in:       "revision: main\nhub: acm-dc1\nstate: \"\"\n",
			revision: "main",
			hub:      "acm-dc1",
			issues:   []Issue{{Line: 3, Message: `state "" must be present or absent`}},
		},
		{
			name:     "null state",
			in:       "revision: main\nhub: acm-dc1\nstate:\n",
			revision: "main",
			hub:      "acm-dc1",
			issues:   []Issue{{Line: 3, Message: "state has no value"}},
		},
		{
			name:     "boolean state",
			in:       "revision: main\nhub: acm-dc1\nstate: off\n",
			revision: "main",
			hub:      "acm-dc1",
			issues:   []Issue{{Line: 3, Message: "state is read by Argo CD as false (bool)"}},
		},
		{
			name:     "install is a string",
			in:       "revision: main\nhub: acm-dc1\ninstall: 4.20.8\n",
			revision: "main",
			hub:      "acm-dc1",
			issues:   []Issue{{Line: 3, Message: "install must be a map"}},
		},
		{
			name:     "install is a list",
			in:       "revision: main\nhub: acm-dc1\ninstall:\n  - version: 4.20.8\n",
			revision: "main",
			hub:      "acm-dc1",
			issues:   []Issue{{Line: 4, Message: "install must be a map"}},
		},
		{
			// valuesObject would pass install: null, which deletes the chart's install defaults.
			name:     "null install",
			in:       "revision: main\nhub: acm-dc1\ninstall:\n",
			revision: "main",
			hub:      "acm-dc1",
			issues:   []Issue{{Line: 3, Message: "install must be a map"}},
		},
		{
			name:     "state without hub",
			in:       "revision: main\nstate: absent\n",
			revision: "main",
			state:    "absent",
			issues:   []Issue{{Line: 2, Message: "state requires hub"}},
		},
		{
			name:     "install without hub",
			in:       "revision: main\ninstall:\n  version: 4.20.8\n",
			revision: "main",
			issues:   []Issue{{Line: 3, Message: "install requires hub"}},
		},
		{
			// An invalid hub still counts as set; its own issue covers it.
			name:     "state with an invalid hub",
			in:       "revision: main\nhub: 1.10\nstate: absent\n",
			revision: "main",
			state:    "absent",
			issues:   []Issue{{Line: 2, Message: "hub is read by Argo CD as 1.1"}},
		},
		{
			name:     "misspelt hub",
			in:       "revision: main\nhubs: acm-dc1\nstate: absent\n",
			revision: "main",
			state:    "absent",
			issues: []Issue{
				{Line: 2, Message: "unknown key hubs"},
				{Line: 3, Message: "state requires hub"},
			},
		},
		{
			name:   "invalid YAML",
			in:     "revision: [main\n",
			issues: []Issue{{Line: 1, Message: "did not find expected"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := parse([]byte(tt.in))
			if c.Revision != tt.revision {
				t.Errorf("Revision = %q, want %q", c.Revision, tt.revision)
			}
			if !slices.Equal(c.ValueFiles, tt.valueFiles) {
				t.Errorf("ValueFiles = %v, want %v", c.ValueFiles, tt.valueFiles)
			}
			if c.Hub != tt.hub || c.State != tt.state {
				t.Errorf("Hub, State = %q, %q, want %q, %q", c.Hub, c.State, tt.hub, tt.state)
			}
			if len(c.Issues) != len(tt.issues) {
				t.Fatalf("got %d issues, want %d: %+v", len(c.Issues), len(tt.issues), c.Issues)
			}
			for i, w := range tt.issues {
				g := c.Issues[i]
				if g.Line != w.Line || !strings.Contains(g.Message, w.Message) {
					t.Errorf("[%d] got {%d %q}, want {%d %q}", i, g.Line, g.Message, w.Line, w.Message)
				}
			}
		})
	}
}

// writeFiles creates files (path -> content; a trailing "/" makes a directory) in a temp dir.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, body := range files {
		full := filepath.Join(dir, p)
		if strings.HasSuffix(p, "/") {
			mustMkdir(t, full)
			continue
		}
		mustMkdir(t, filepath.Dir(full))
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}
