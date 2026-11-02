package reporter

import (
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/checks"
)

func TestGitHub(t *testing.T) {
	tests := []struct {
		name string
		f    checks.Finding
		want string
	}{
		{
			name: "location and cluster prefix",
			f:    checks.Finding{RuleID: "POLICY020", Severity: checks.SevError, Element: "metallb", Cluster: "sno-edge", Message: "missing", File: "/repo/stack/metallb/values.yaml", Line: 7, Col: 3},
			want: "::error file=stack/metallb/values.yaml,line=7,col=3,title=POLICY020::[metallb/sno-edge] missing\n",
		},
		{
			// The workflow-command spec ends a property at ',' and the properties at '::'.
			name: "property values are escaped",
			f:    checks.Finding{RuleID: "POLICY035", Message: "m", File: "/repo/values/a:b,c%d\ne.yaml"},
			want: "::warning file=values/a%3Ab%2Cc%25d%0Ae.yaml,title=POLICY035::m\n",
		},
		{
			name: "message keeps ':' and ',' but escapes '%' and newlines",
			f:    checks.Finding{RuleID: "POLICY081", Severity: checks.SevError, Element: "acs", Message: "a: b, 100%\r\nnext"},
			want: "::error title=POLICY081::[acs] a: b, 100%25%0D%0Anext\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			if err := (GitHub{RepoRoot: "/repo"}).Write(&b, []checks.Finding{tt.f}); err != nil {
				t.Fatal(err)
			}
			if b.String() != tt.want {
				t.Errorf("got  %q\nwant %q", b.String(), tt.want)
			}
		})
	}
}
