package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRelease(t *testing.T) {
	for _, tc := range []struct {
		commit string
		want   string // the parsed version, empty when not a release tag
	}{
		{commit: "v1.2.3", want: "1.2.3"},
		{commit: "v10.0.0", want: "10.0.0"},
		{commit: "v1.0.0-rc.1", want: "1.0.0-rc.1"},
		{commit: "v1.0.0-rc.1.2", want: "1.0.0-rc.1.2"},
		{commit: "v1.2.3-4-gabc1234"},
		{commit: "v1.0.0-rc.1-2-g0123456"},
		{commit: "acm-v1.0.0"},
		{commit: "1.2.3"},
		{commit: "v1.2"},
		{commit: "V1.2.3"},
		{commit: "v1.2.3+build"},
		{commit: "v01.2.3"},
		{commit: "v1.0.0-01"},
		{commit: "abc1234"},
		{commit: ""},
	} {
		v, ok := release(tc.commit)
		got := ""
		if ok {
			got = v.String()
		}
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("release(%q) = %q, %v; want %q", tc.commit, got, ok, tc.want)
		}
	}
}

func TestPolicy061_Answers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string // file: line: message
	}{
		{name: "not a copier copy"},
		{
			name: "framework and tracked elements",
			files: map[string]string{
				".copier-answers.yml":           "_commit: v1.2.0\n_src_path: gh:PolicyStack/PolicyStack\nelement: ''\n",
				"stack/acm/.copier-answers.yml": "_commit: v1.2.0\n_src_path: gh:PolicyStack/PolicyStack\nelement: acm\n",
				"stack/odf/.copier-answers.yml": "_commit: v1.0.0-rc.1\nelement: odf\n",
				"stack/mine/Chart.yaml":         "name: mine\n",
			},
		},
		{
			name: "element on an older minor of the framework's major",
			files: map[string]string{
				".copier-answers.yml":           "_commit: v2.3.0\n",
				"stack/acm/.copier-answers.yml": "_commit: v2.0.0\nelement: acm\n",
			},
		},
		{
			name: "element on an older major than the framework",
			files: map[string]string{
				".copier-answers.yml":           "_commit: v2.0.0\n",
				"stack/acm/.copier-answers.yml": "_commit: v1.9.3\nelement: acm\n",
			},
			want: []string{
				"stack/acm/.copier-answers.yml:1: _commit v1.9.3 is on major 1 and the framework, v2.0.0 in /.copier-answers.yml, on major 2: update this element to a v2 release, copier update --vcs-ref v2.0.0 stack/acm",
			},
		},
		{
			name: "element on the previous major's prerelease, framework on the next major's prerelease",
			files: map[string]string{
				".copier-answers.yml":           "_commit: v2.0.0-rc.1\n",
				"stack/acm/.copier-answers.yml": "_commit: v1.0.0-rc.1\nelement: acm\n",
			},
			want: []string{
				"stack/acm/.copier-answers.yml:1: _commit v1.0.0-rc.1 is on major 1 and the framework, v2.0.0-rc.1 in /.copier-answers.yml, on major 2: update this element to a v2 release, copier update --vcs-ref v2.0.0-rc.1 stack/acm",
			},
		},
		{
			name: "element newer than the framework",
			files: map[string]string{
				".copier-answers.yml":           "_commit: v1.1.0\n",
				"stack/odf/.copier-answers.yml": "_commit: v1.2.0\nelement: odf\n",
			},
			want: []string{
				`stack/odf/.copier-answers.yml:1: _commit v1.2.0 is newer than the framework's, v1.1.0 in /.copier-answers.yml: update the framework to v1.2.0 or later first, or this element back to v1.1.0 or earlier`,
			},
		},
		{
			name: "element on a prerelease of the next framework release",
			files: map[string]string{
				".copier-answers.yml":           "_commit: v1.1.0\n",
				"stack/odf/.copier-answers.yml": "_commit: v1.2.0-rc.1\nelement: odf\n",
			},
			want: []string{
				`stack/odf/.copier-answers.yml:1: _commit v1.2.0-rc.1 is newer than the framework's, v1.1.0 in /.copier-answers.yml: update the framework to v1.2.0-rc.1 or later first, or this element back to v1.1.0 or earlier`,
			},
		},
		{
			name: "framework on a prerelease, element on an older prerelease of it",
			files: map[string]string{
				".copier-answers.yml":           "_commit: v1.0.0-rc.2\n",
				"stack/odf/.copier-answers.yml": "_commit: v1.0.0-rc.1\nelement: odf\n",
				"stack/acm/.copier-answers.yml": "_commit: v1.0.0-rc.10\nelement: acm\n",
			},
			want: []string{
				`stack/acm/.copier-answers.yml:1: _commit v1.0.0-rc.10 is newer than the framework's, v1.0.0-rc.2 in /.copier-answers.yml: update the framework to v1.0.0-rc.10 or later first, or this element back to v1.0.0-rc.2 or earlier`,
			},
		},
		{
			name: "framework release after the element's prerelease",
			files: map[string]string{
				".copier-answers.yml":           "_commit: v1.0.0\n",
				"stack/odf/.copier-answers.yml": "_commit: v1.0.0-rc.1\nelement: odf\n",
			},
		},
		{
			name:  "element without a framework answers file: no order to check",
			files: map[string]string{"stack/odf/.copier-answers.yml": "_commit: v9.0.0\nelement: odf\n"},
		},
		{
			name: "unreadable framework release: only the root is reported",
			files: map[string]string{
				".copier-answers.yml":           "_commit: v1.0.0-2-gabc1234\n",
				"stack/odf/.copier-answers.yml": "_commit: v1.2.0\nelement: odf\n",
			},
			want: []string{`.copier-answers.yml:1: _commit "v1.0.0-2-gabc1234" is not a release tag vX.Y.Z: rerun copier update with --vcs-ref vX.Y.Z`},
		},
		{
			name:  "framework copied without --vcs-ref from an untagged commit",
			files: map[string]string{".copier-answers.yml": "_src_path: gh:PolicyStack/PolicyStack\n_commit: v1.2.0-3-gabc1234\n"},
			want:  []string{`.copier-answers.yml:2: _commit "v1.2.0-3-gabc1234" is not a release tag vX.Y.Z: rerun copier update with --vcs-ref vX.Y.Z`},
		},
		{
			name:  "prefixed tag at the root",
			files: map[string]string{".copier-answers.yml": "_commit: acm-v1.0.0\n"},
			want:  []string{`.copier-answers.yml:1: _commit "acm-v1.0.0" is not a release tag vX.Y.Z: rerun copier update with --vcs-ref vX.Y.Z`},
		},
		{
			name:  "no _commit: not copied from git",
			files: map[string]string{".copier-answers.yml": "_src_path: /tmp/policystack\n"},
			want:  []string{`.copier-answers.yml:0: _commit "" is not a release tag vX.Y.Z: rerun copier update with --vcs-ref vX.Y.Z`},
		},
		{
			name:  "element copied from an untagged commit",
			files: map[string]string{".copier-answers.yml": "_commit: v1.2.0\n", "stack/acm/.copier-answers.yml": "_commit: v1.1.0-4-g0123abc\nelement: acm\n"},
			want: []string{
				`stack/acm/.copier-answers.yml:1: _commit "v1.1.0-4-g0123abc" is not a release tag vX.Y.Z: rerun copier update in stack/acm with --vcs-ref vX.Y.Z`,
			},
		},
		{
			name:  "prefixed tag in an element",
			files: map[string]string{"stack/acm/.copier-answers.yml": "_commit: acm-v1.0.0\nelement: acm\n"},
			want: []string{
				`stack/acm/.copier-answers.yml:1: _commit "acm-v1.0.0" is not a release tag vX.Y.Z: rerun copier update in stack/acm with --vcs-ref vX.Y.Z`,
			},
		},
		{
			name:  "element copied into another directory",
			files: map[string]string{"stack/acm2/.copier-answers.yml": "_commit: v1.0.0\nelement: acm\n"},
			want: []string{
				`stack/acm2/.copier-answers.yml:2: element "acm" is not this directory, "acm2": copy each element into stack/<element> with -d element=<element>`,
			},
		},
		{
			name:  "framework answers in an element: no element answer",
			files: map[string]string{".copier-answers.yml": "_commit: v1.0.0\n", "stack/acm/.copier-answers.yml": "_commit: v1.0.0\n"},
			want: []string{
				`stack/acm/.copier-answers.yml:0: element "" is not this directory, "acm": copy each element into stack/<element> with -d element=<element>`,
			},
		},
		{
			name:  "invalid YAML",
			files: map[string]string{"stack/acm/.copier-answers.yml": "_commit: [\n"},
			want:  []string{`stack/acm/.copier-answers.yml:0: not valid YAML: yaml: line 1: did not find expected node content`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range tc.files {
				p := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var got []string
			for _, f := range (&AnswersCheck{Root: dir}).Run(Context{}) {
				rel, _ := filepath.Rel(dir, f.File)
				if f.RuleID != "POLICY061" || f.Severity != SevError || f.Element != filepath.Base(filepath.Dir(rel)) && f.Element != "" {
					t.Errorf("unexpected finding: %+v", f)
				}
				got = append(got, fmt.Sprintf("%s:%d: %s", rel, f.Line, f.Message))
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}
