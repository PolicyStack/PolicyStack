package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/cascade"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

// renderCtx is a render-phase Context for el on cluster spoke. The cascade is el's values.yaml, then
// each layer in order.
func renderCtx(t *testing.T, el *chart.Element, rendered string, layers ...string) Context {
	t.Helper()
	files := []string{el.ValuesFile}
	for i, l := range layers {
		f := filepath.Join(t.TempDir(), fmt.Sprintf("layer%d.yaml", i))
		if err := os.WriteFile(f, []byte(l), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	return Context{
		Element:  el,
		Cluster:  &cascade.Resolved{ClusterName: "spoke", ReleaseName: el.ChartName + "-spoke", ValueFiles: files},
		Rendered: []byte(rendered),
	}
}

// policyDoc renders Policy <name>-my-element-spoke as policy-library does. spec holds extra spec
// lines, indented two spaces.
func policyDoc(name, spec string, templates ...string) string {
	return "---\napiVersion: policy.open-cluster-management.io/v1\nkind: Policy\nmetadata:\n  name: " + name +
		"-my-element-spoke\n  namespace: policy\nspec:\n" + spec + "  policy-templates:\n" + strings.Join(templates, "")
}

// template renders a policy-templates entry. header holds policy-template keys ahead of
// objectDefinition, each followed by a newline and six spaces; spec holds spec lines, indented ten.
func template(kind, name, header, spec string) string {
	return "    - " + header + "objectDefinition:\n        apiVersion: policy.open-cluster-management.io/v1\n        kind: " + kind +
		"\n        metadata:\n          name: " + name + "\n        spec:\n" + spec
}

// Where a finding is expected: an index into the cascade, or nowhere.
const (
	inValues = 0
	inLayer  = 1
	nowhere  = -1
)

type want struct {
	sev  Severity
	msg  string // message prefix
	file int    // inValues, inLayer or nowhere
	line int
}

// assertFindings compares got with want, in order.
func assertFindings(t *testing.T, id string, ctx Context, got []Finding, want []want) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d findings, got %d: %+v", len(want), len(got), got)
	}
	for i, w := range want {
		f := got[i]
		file := ""
		if w.file != nowhere {
			file = ctx.Cluster.ValueFiles[w.file]
		}
		if f.RuleID != id || f.Severity != w.sev || !strings.HasPrefix(f.Message, w.msg) || f.Element != ctx.Element.ChartName {
			t.Errorf("finding %d = %+v, want %s starting %q", i, f, w.sev, w.msg)
		}
		if f.File != file || f.Line != w.line {
			t.Errorf("finding %d at %s:%d, want %s:%d", i, f.File, f.Line, file, w.line)
		}
	}
}

// TestRenderedInvalid: every render-phase rule reports an undecodable render once and skips an empty one.
func TestRenderedInvalid(t *testing.T) {
	el := loadElement(t, "stack:\n  myElement: {}\n", "", nil)
	for _, c := range []Check{&NameLengthCheck{}, &PruneCheck{}, &RemediationCheck{}, &ProtectedObjectCheck{}, &EmptyPolicyCheck{}} {
		if got := c.Run(renderCtx(t, el, "\n---\n")); len(got) != 0 {
			t.Errorf("%s: empty render got %+v", c.ID(), got)
		}
		got := c.Run(renderCtx(t, el, "kind: ["))
		if len(got) != 1 || got[0].RuleID != c.ID() || !strings.HasPrefix(got[0].Message, "rendered output: ") {
			t.Errorf("%s: got %+v, want one rendered output finding", c.ID(), got)
		}
	}
}
