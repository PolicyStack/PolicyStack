package imageset

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/run"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

// policy renders a Policy holding one OperatorPolicy template per op, each written as the YAML of
// its spec, indented under spec:.
func policy(ops ...string) string {
	var b strings.Builder
	b.WriteString("---\napiVersion: policy.open-cluster-management.io/v1\nkind: Policy\nmetadata:\n  name: p\nspec:\n  policy-templates:\n")
	for _, op := range ops {
		b.WriteString("    - objectDefinition:\n        apiVersion: policy.open-cluster-management.io/v1beta1\n        kind: OperatorPolicy\n        metadata:\n          name: install-op\n        spec:\n")
		for line := range strings.SplitSeq(strings.TrimSpace(op), "\n") {
			b.WriteString("          " + line + "\n")
		}
	}
	return b.String()
}

func sub(name, channel string, extra ...string) string {
	s := "subscription:\n  name: " + name + "\n  source: redhat-operators\n"
	if channel != "" {
		s += "  channel: " + channel + "\n"
	}
	return s + strings.Join(extra, "\n")
}

// mine subscribes to name from my-catalog.
func mine(name string) string {
	return strings.Replace(sub(name, "stable"), "redhat-operators", "my-catalog", 1)
}

const header = "kind: ImageSetConfiguration\napiVersion: mirror.openshift.io/v2alpha1\nmirror:\n"

func TestBuild(t *testing.T) {
	cases := []struct {
		name     string
		clusters []Cluster
		extra    []Package
		want     string
	}{
		{
			name: "releases span each channel, stable-<minor> by default",
			clusters: []Cluster{
				{Name: "a", Version: "4.20.12", Channel: "fast-4.20"},
				{Name: "b", Version: "4.20.8", Channel: "fast-4.20"},
				{Name: "c", Version: "4.21.1"},
				{Name: "d", Channel: "stable-4.20"},
			},
			want: header + `  platform:
    channels:
      - name: fast-4.20
        minVersion: 4.20.8
        maxVersion: 4.20.12
      - name: stable-4.21
        minVersion: 4.21.1
        maxVersion: 4.21.1
    graph: true
`,
		},
		{
			name:     "no version: no platform",
			clusters: []Cluster{{Name: "a", Channel: "eus-4.20", Rendered: []byte(policy(sub("metallb-operator", "stable")))}},
			want: header + `  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: metallb-operator
          defaultChannel: stable
          channels:
            - name: stable
`,
		},
		{
			name: "catalog per source and minor, packages merged and sorted",
			clusters: []Cluster{
				{Name: "a", Version: "4.20.1", Rendered: []byte(policy(sub("z-op", "stable"), sub("a-op", "v1")))},
				{Name: "b", Version: "4.20.2", Rendered: []byte(policy(sub("a-op", "v2"), strings.Replace(sub("cert", "alpha"), "redhat-operators", "certified-operators", 1)))},
				{Name: "c", Version: "4.21.0", Rendered: []byte(policy(sub("a-op", "v1")))},
			},
			want: header + `  platform:
    channels:
      - name: stable-4.20
        minVersion: 4.20.1
        maxVersion: 4.20.2
      - name: stable-4.21
        minVersion: 4.21.0
        maxVersion: 4.21.0
    graph: true
  operators:
    - catalog: registry.redhat.io/redhat/certified-operator-index:v4.20
      packages:
        - name: cert
          defaultChannel: alpha
          channels:
            - name: alpha
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: a-op
          defaultChannel: v1
          channels:
            - name: v1
            - name: v2
        - name: z-op
          defaultChannel: stable
          channels:
            - name: stable
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.21
      packages:
        - name: a-op
          defaultChannel: v1
          channels:
            - name: v1
`,
		},
		{
			name: "pins bound the channel",
			clusters: []Cluster{
				{Name: "a", Channel: "stable-4.20", Rendered: []byte(policy(sub("acm", "release-2.17", "versions:\n  - advanced-cluster-management.v2.17.3"),
					sub("odf-operator", "stable-4.20", "versions:\n  - odf-operator.v4.20.2-rhodf, odf-operator.v4.20.0-rhodf"),
					sub("kubernetes-nmstate-operator", "stable", "versions:\n  - kubernetes-nmstate-operator.4.20.0-202511040000"),
					sub("nfd", "stable", "  startingCSV: nfd.4.20.0-202510010000", "versions:\n  - nfd.4.20.0-202511040403")))},
				{Name: "b", Channel: "stable-4.20", Rendered: []byte(policy(sub("acm", "release-2.17", "versions:\n  - advanced-cluster-management.v2.17.1")))},
			},
			want: header + `  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: acm
          defaultChannel: release-2.17
          channels:
            - name: release-2.17
              minVersion: 2.17.1
              maxVersion: 2.17.3
        - name: kubernetes-nmstate-operator
          defaultChannel: stable
          channels:
            - name: stable
              minVersion: 4.20.0-202511040000
              maxVersion: 4.20.0-202511040000
        - name: nfd
          defaultChannel: stable
          channels:
            - name: stable
              minVersion: 4.20.0-202510010000
              maxVersion: 4.20.0-202511040403
        - name: odf-operator
          defaultChannel: stable-4.20
          channels:
            - name: stable-4.20
              minVersion: 4.20.0-rhodf
              maxVersion: 4.20.2-rhodf
`,
		},
		{
			name: "an unpinned subscription lifts the upper bound",
			clusters: []Cluster{
				{Name: "a", Channel: "stable-4.20", Rendered: []byte(policy(sub("acm", "release-2.17", "versions:\n  - advanced-cluster-management.v2.17.1")))},
				{Name: "b", Channel: "stable-4.20", Rendered: []byte(policy(sub("acm", "release-2.17")))},
			},
			want: header + `  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: acm
          defaultChannel: release-2.17
          channels:
            - name: release-2.17
              minVersion: 2.17.1
`,
		},
		{
			name:     "an unpinned startingCSV sets the lower bound",
			clusters: []Cluster{{Name: "a", Channel: "stable-4.20", Rendered: []byte(policy(sub("metallb-operator", "stable", "  startingCSV: metallb-operator.v4.20.0-202510010000", "versions: []")))}},
			want: header + `  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: metallb-operator
          defaultChannel: stable
          channels:
            - name: stable
              minVersion: 4.20.0-202510010000
`,
		},
		{
			name: "removals, other kinds and no channel",
			clusters: []Cluster{{Name: "a", Channel: "stable-4.20", Rendered: []byte(
				"---\nkind: ConfigMap\nmetadata:\n  name: x\n---\nkind: Placement\nspec:\n  predicates: []\n" +
					policy(sub("gone", "stable", "complianceType: MustNotHave"), sub("default-only", "")) +
					"---\nkind: PolicySet\nspec:\n  policies: [p]\n")}},
			want: header + `  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: default-only
`,
		},
		{
			name: "other catalogs are named, not mirrored",
			clusters: []Cluster{
				{Name: "a", Channel: "stable-4.20", Rendered: []byte(policy(mine("w"), mine("x"), sub("z", "stable")))},
				{Name: "b", Channel: "stable-4.20", Rendered: []byte(policy(mine("x")))},
			},
			want: "# Not mirrored: w, x from my-catalog, not a catalog OpenShift ships.\n" + header + `  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: z
          defaultChannel: stable
          channels:
            - name: stable
`,
		},
		{
			name:     "extra packages join redhat-operators at every minor",
			clusters: []Cluster{{Name: "a", Version: "4.20.1", Rendered: []byte(policy(sub("acm", "release-2.17", "versions:\n  - advanced-cluster-management.v2.17.1")))}, {Name: "b", Channel: "stable-4.21"}},
			extra:    []Package{{Name: "multicluster-engine", Channel: "stable-2.12"}, {Name: "acm", Channel: "release-2.17"}, {Name: "ocs-operator"}},
			want: header + `  platform:
    channels:
      - name: stable-4.20
        minVersion: 4.20.1
        maxVersion: 4.20.1
    graph: true
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: acm
          defaultChannel: release-2.17
          channels:
            - name: release-2.17
              minVersion: 2.17.1
        - name: multicluster-engine
          defaultChannel: stable-2.12
          channels:
            - name: stable-2.12
        - name: ocs-operator
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.21
      packages:
        - name: acm
          defaultChannel: release-2.17
          channels:
            - name: release-2.17
        - name: multicluster-engine
          defaultChannel: stable-2.12
          channels:
            - name: stable-2.12
        - name: ocs-operator
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Build(tc.clusters, tc.extra)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestBuildErrors(t *testing.T) {
	cases := []struct {
		name    string
		cluster Cluster
		want    []string
	}{
		{"no minor", Cluster{Name: "a", Channel: "stable"}, []string{"a: set version, or a channel"}},
		{"bad version", Cluster{Name: "a", Version: "4.20"}, []string{`a: version "4.20" is not an x.y.z release`}},
		{"templated channel", Cluster{Name: "a", Version: "4.20.1", Rendered: []byte(policy(sub("x", "'{{hub .ManagedClusterLabels.channel hub}}'")))},
			[]string{"a: OperatorPolicy install-op: a hub or managed cluster template"}},
		{"templated pin", Cluster{Name: "a", Version: "4.20.1", Rendered: []byte(policy(sub("x", "stable", "versions:\n  - '{{ .x }}'")))},
			[]string{"a: OperatorPolicy install-op: a hub or managed cluster template"}},
		{"pin without version", Cluster{Name: "a", Version: "4.20.1", Rendered: []byte(policy(sub("x", "stable", "versions:\n  - x.latest, x.v1.2")))},
			[]string{`"x.latest": no x.y.z version`, `"x.v1.2": no x.y.z version`}},
		{"no channel and a channel", Cluster{Name: "a", Version: "4.20.1", Rendered: []byte(policy(sub("x", ""), sub("x", "stable")))},
			[]string{"x in registry.redhat.io/redhat/redhat-operator-index:v4.20: subscribed both with and without a channel"}},
		{"pinned without a channel", Cluster{Name: "a", Version: "4.20.1", Rendered: []byte(policy(sub("x", "", "versions:\n  - x.v1.2.3")))},
			[]string{"x in registry.redhat.io/redhat/redhat-operator-index:v4.20: pinned without a channel"}},
		{"undecodable render", Cluster{Name: "a", Version: "4.20.1", Rendered: []byte("kind: Policy\nspec: [\n")}, []string{"a: "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Build([]Cluster{tc.cluster}, nil)
			if err == nil {
				t.Fatal("want error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
		})
	}
}

func TestBuildExtraWithoutChannel(t *testing.T) {
	_, err := Build([]Cluster{{Name: "a", Version: "4.20.1", Rendered: []byte(policy(sub("x", "stable")))}}, []Package{{Name: "x"}})
	if err == nil || !strings.Contains(err.Error(), "subscribed both with and without a channel") {
		t.Errorf("err = %v", err)
	}
}

func TestParsePackage(t *testing.T) {
	for in, want := range map[string]Package{
		"multicluster-engine:stable-2.12": {"multicluster-engine", "stable-2.12"},
		"ocs-operator":                    {"ocs-operator", ""},
	} {
		if got, err := ParsePackage(in); err != nil || got != want {
			t.Errorf("ParsePackage(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", ":stable", "x:"} {
		if _, err := ParsePackage(in); err == nil {
			t.Errorf("ParsePackage(%q): want error", in)
		}
	}
}

// TestGenerate renders upstream's elements for a fleet whose site layer renames redhat-operators,
// as docs/guides/disconnected.md does, and checks that the catalog is the source the element names.
func TestGenerate(t *testing.T) {
	helm := testenv.Helm(t)
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("values/site.yaml", `global:
  catalogSources:
    redhat-operators: cs-redhat-operator-index-v4-20
stack:
  metallb:
    enabled: true
    operatorPolicies:
      metallb:
        versions: [metallb-operator.v4.20.0-202511040000]
`)
	write("values/clusters/west.yaml", "stack:\n  certManager:\n    enabled: true\n")
	write("fleet/west.yaml", "revision: main\nversion: 4.20.8\nvalueFiles: [site.yaml]\n")
	write("fleet/east.yaml", "revision: main\nchannel: stable-4.20\nvalueFiles: [site.yaml]\n")
	write("fleet/other.yaml", "revision: main\n")
	opts := run.Options{
		RepoRoot:     repo,
		StackDir:     filepath.Join(repo, "stack"),
		LifecycleDir: filepath.Join(repo, "lifecycle"),
		ValuesDir:    filepath.Join(dir, "values"),
		FixturesDir:  filepath.Join(dir, "fleet"),
		HelmBin:      helm,
		ExtraValues:  []string{filepath.Join(repo, "tools/validator/testdata/baseline.yaml")},
		Logger:       slog.New(slog.DiscardHandler),
	}

	got, err := Generate(t.Context(), opts, []string{"west", "east"}, []Package{{Name: "extra-op"}})
	if err != nil {
		t.Fatal(err)
	}
	want := header + `  platform:
    channels:
      - name: stable-4.20
        minVersion: 4.20.8
        maxVersion: 4.20.8
    graph: true
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20
      packages:
        - name: extra-op
        - name: metallb-operator
          defaultChannel: stable
          channels:
            - name: stable
              minVersion: 4.20.0-202511040000
              maxVersion: 4.20.0-202511040000
        - name: openshift-cert-manager-operator
          defaultChannel: stable-v1
          channels:
            - name: stable-v1
`
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	if _, err := Generate(t.Context(), opts, []string{"west", "nope"}, nil); err == nil || !strings.Contains(err.Error(), "nope: no fleet file") {
		t.Errorf("unknown fleet file: err = %v", err)
	}
	// Every fleet file, other included, which sets neither version nor channel.
	if _, err := Generate(t.Context(), opts, nil, nil); err == nil || !strings.Contains(err.Error(), "other: set version") {
		t.Errorf("all fleet files: err = %v", err)
	}
	write("fleet/typo.yaml", "revision: main\nverison: 4.20.8\nchannel: stable-4.20\n")
	if _, err := Generate(t.Context(), opts, []string{"typo"}, nil); err == nil || !strings.Contains(err.Error(), "POLICY050") {
		t.Errorf("invalid fleet file: err = %v", err)
	}
	write("values/clusters/west.yaml", "stack:\n  metallb:\n    config:\n      bgpPeers: [\n")
	if _, err := Generate(t.Context(), opts, []string{"west"}, nil); err == nil || !strings.Contains(err.Error(), "render failed") {
		t.Errorf("failed render: err = %v", err)
	}
}
