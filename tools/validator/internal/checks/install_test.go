package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

// TestPolicy071_Install runs the check on a copy of install/ and the values it reads, each case
// breaking the copy as a past commit did.
func TestPolicy071_Install(t *testing.T) {
	runner := render.New(testenv.Helm(t))
	const repo = "../../../.."
	// replace rewrites old to new in the files under root matching glob, failing when none has old.
	replace := func(t *testing.T, root, glob, old, new string) {
		t.Helper()
		files, _ := filepath.Glob(filepath.Join(root, glob))
		n := 0
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			n += strings.Count(string(data), old)
			write(t, f, strings.ReplaceAll(string(data), old, new))
		}
		if n == 0 {
			t.Fatalf("%s: no %q", glob, old)
		}
	}

	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, root string)
		want   []string // message prefixes, in order
	}{
		{name: "upstream install"},
		// regression: 772c88b
		{
			name: "policyNamespace differs from the root values.yaml",
			mutate: func(t *testing.T, root string) {
				replace(t, root, "install/values.yaml", "policyNamespace: pstack", "policyNamespace: policy")
			},
			want: []string{`policyNamespace "policy" must equal "pstack" in values.yaml`},
		},
		// regression: 44afbe1
		{
			name: "ApplicationSet without the backup-exclude label",
			mutate: func(t *testing.T, root string) {
				replace(t, root, "install/templates/appset.yaml", backupLabel, "example.com/other")
			},
			want: []string{
				`ApplicationSet policystack has no velero.io/exclude-from-backup: "true" label`,
				`ApplicationSet policystack template has no velero.io/exclude-from-backup: "true" label`,
			},
		},
		// regression: 44afbe1
		{
			name: "GitOpsCluster and Placement without the backup-exclude label",
			mutate: func(t *testing.T, root string) {
				replace(t, root, "install/templates/gitopscluster.yaml", `exclude-from-backup: "true"`, `exclude-from-backup: "false"`)
			},
			want: []string{
				`GitOpsCluster argo-acm-importer-global has no velero.io/exclude-from-backup: "true" label`,
				`Placement global has no velero.io/exclude-from-backup: "true" label`,
			},
		},
		{
			name: "hubName not required",
			mutate: func(t *testing.T, root string) {
				replace(t, root, "install/templates/*.yaml", `required "hubName must be set (--set hubName=<name>)" .Values.hubName`, ".Values.hubName")
			},
			want: []string{"hubName is not required"},
		},
		{
			name: "operators render without the gitops values",
			mutate: func(t *testing.T, root string) {
				write(t, filepath.Join(root, "stack/gitops/values.yaml"), "stack: {}\n")
			},
			want: []string{`renders "`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.CopyFS(filepath.Join(root, "install"), os.DirFS(filepath.Join(repo, "install"))); err != nil {
				t.Fatal(err)
			}
			for _, f := range []string{"values.yaml", "stack/acm/values.yaml", "stack/gitops/values.yaml"} {
				data, err := os.ReadFile(filepath.Join(repo, f))
				if err != nil {
					t.Fatal(err)
				}
				write(t, filepath.Join(root, f), string(data))
			}
			if tc.mutate != nil {
				tc.mutate(t, root)
			}

			got := (&InstallCheck{Runner: runner, Root: root}).Run(Context{})
			if len(got) != len(tc.want) {
				t.Fatalf("got %d findings, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, f := range got {
				if f.RuleID != "POLICY071" || f.Severity != SevError || !strings.HasPrefix(f.Message, tc.want[i]) {
					t.Errorf("finding %d: got %+v, want message prefix %q", i, f, tc.want[i])
				}
			}
		})
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
