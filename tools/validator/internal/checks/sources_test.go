package checks

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

func TestPolicy038_CatalogSources(t *testing.T) {
	element := func(source string) *chart.Element {
		return loadElement(t, `
stack:
  myElement:
    enabled: true
    operatorPolicies:
      my-op:
        subscription:
          name: my-op
          source: `+source+`
`, "", nil)
	}
	tests := []struct {
		name, source, layer string
		want                []string // "<file>:<line>", file "values" or "layer"
	}{
		{name: "default catalog", source: "redhat-operators"},
		{name: "unset is redhat-operators", source: ""},
		{name: "certified", source: "certified-operators"},
		{name: "templated", source: `'{{ "{{hub" }} .ManagedClusterName {{ "hub}}" }}'`},
		{name: "regression: 929c474", source: "redhat-operator-index", want: []string{"values:9"}},
		{name: "declared by a layer", source: "my-catalog", layer: "global:\n  catalogSources:\n    my-catalog: cs-my-catalog\n"},
		{name: "a mapped name is not a source", source: "cs-my-catalog", layer: "global:\n  catalogSources:\n    my-catalog: cs-my-catalog\n", want: []string{"values:9"}},
		{
			name: "layer override", source: "redhat-operators",
			layer: "stack:\n  myElement:\n    operatorPolicies:\n      my-op:\n        subscription:\n          source: community-operator\n",
			want:  []string{"layer:6"},
		},
		{name: "invalid layer is left to RENDER000", source: "redhat-operators", layer: "stack: [\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			el := element(tt.source)
			layer := filepath.Join(t.TempDir(), "layer.yaml")
			if err := os.WriteFile(layer, []byte(tt.layer), 0o644); err != nil {
				t.Fatal(err)
			}
			got := (&SourceCheck{}).Run(Context{AllElements: []*chart.Element{el, {ChartName: "lifecycle"}}, Layers: []string{layer, layer + ".missing"}})
			var locs []string
			for _, f := range got {
				if f.RuleID != "POLICY038" || f.Severity != SevError || f.Element != "my-element" {
					t.Errorf("unexpected finding shape: %+v", f)
				}
				name := map[string]string{el.ValuesFile: "values", layer: "layer"}[f.File]
				locs = append(locs, name+":"+strconv.Itoa(f.Line))
			}
			if !slices.Equal(locs, tt.want) {
				t.Errorf("got %v, want %v (%+v)", locs, tt.want, got)
			}
		})
	}
}
