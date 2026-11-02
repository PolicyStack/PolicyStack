package checks

import (
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

func TestPolicy060_Pinning(t *testing.T) {
	el := func(name, version string) *chart.Element {
		return &chart.Element{ChartName: name, Dir: "/r/stack/" + name, Dependencies: []chart.Dependency{{Name: "policy-library", Version: version}}}
	}
	if got := (&PinningCheck{}).Run(Context{AllElements: []*chart.Element{el("a", "3.4.0"), el("b", "3.4.0")}}); len(got) != 0 {
		t.Errorf("one version: got %+v", got)
	}
	got := (&PinningCheck{}).Run(Context{AllElements: []*chart.Element{el("a", "3.4.0"), el("b", "3.3.0")}})
	if len(got) != 2 {
		t.Fatalf("expected a finding per element, got %+v", got)
	}
	for _, f := range got {
		const want = "policy-library version drift across elements: 3.3.0=b; 3.4.0=a"
		if f.RuleID != "POLICY060" || f.Severity != SevError || f.Message != want {
			t.Errorf("unexpected finding: %+v", f)
		}
	}
}
