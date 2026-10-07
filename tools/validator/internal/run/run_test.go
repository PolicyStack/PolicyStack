package run

import (
	"slices"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
)

func TestBuildPairs(t *testing.T) {
	a, b := &chart.Element{ChartName: "a"}, &chart.Element{ChartName: "b"}
	lc := &chart.Element{ChartName: "lifecycle"}
	spoke := &fixtures.Cluster{Name: "spoke"}
	built := &fixtures.Cluster{Name: "built", Hub: "acm-dc1"}
	retired := &fixtures.Cluster{Name: "retired", Hub: "acm-dc1", State: "absent"}
	hub := &fixtures.Cluster{Name: "acm-dc1"}
	clusters := []*fixtures.Cluster{spoke, built, retired, hub}

	tests := []struct {
		name      string
		elements  []*chart.Element
		lifecycle *chart.Element
		clusters  []*fixtures.Cluster
		want      []string // "<element>/<cluster>", "+fleet" when the fleet file is appended
	}{
		{
			name:      "lifecycle only for clusters that set hub",
			elements:  []*chart.Element{a, b},
			lifecycle: lc,
			clusters:  clusters,
			want: []string{
				"a/spoke", "a/built", "a/retired", "a/acm-dc1",
				"b/spoke", "b/built", "b/retired", "b/acm-dc1",
				"lifecycle/built+fleet", "lifecycle/retired+fleet",
			},
		},
		{
			name:     "no lifecycle chart",
			elements: []*chart.Element{a},
			clusters: clusters,
			want:     []string{"a/spoke", "a/built", "a/retired", "a/acm-dc1"},
		},
		{
			name:      "no element",
			lifecycle: lc,
			clusters:  clusters,
			want:      []string{"lifecycle/built+fleet", "lifecycle/retired+fleet"},
		},
		{
			name:      "no cluster sets hub",
			elements:  []*chart.Element{a},
			lifecycle: lc,
			clusters:  []*fixtures.Cluster{spoke, hub},
			want:      []string{"a/spoke", "a/acm-dc1"},
		},
		{name: "no clusters", elements: []*chart.Element{a}, lifecycle: lc},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, p := range buildPairs(tt.elements, tt.lifecycle, tt.clusters) {
				s := p.el.ChartName + "/" + p.cl.Name
				if p.fleetValues {
					s += "+fleet"
				}
				got = append(got, s)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValueFiles(t *testing.T) {
	opts := Options{ExtraValues: []string{"/extra/baseline.yaml"}}
	cl := &fixtures.Cluster{Name: "built", Hub: "acm-dc1", SourceFile: "/fleet/built.yaml"}
	cascadeFiles := []string{"/el/values.yaml", "/repo/values.yaml", "/repo/values/clusters/built.yaml"}

	tests := []struct {
		name string
		p    pair
		want []string
	}{
		{
			name: "element ends with ExtraValues",
			p:    pair{el: &chart.Element{ChartName: "a"}, cl: cl},
			want: append(slices.Clone(cascadeFiles), "/extra/baseline.yaml"),
		},
		{
			name: "lifecycle ends with the fleet file, without ExtraValues",
			p:    pair{el: &chart.Element{ChartName: "lifecycle"}, cl: cl, fleetValues: true},
			want: append(slices.Clone(cascadeFiles), "/fleet/built.yaml"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := valueFiles(slices.Clone(cascadeFiles), opts, tt.p); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
