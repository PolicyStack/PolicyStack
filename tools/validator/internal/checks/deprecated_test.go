package checks

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

func TestPolicy092_DeprecatedElement(t *testing.T) {
	const deprecatedChart = "apiVersion: v2\nname: old\nversion: 0.3.0\ndeprecated: true\n"
	old := loadElement(t, "stack:\n  old:\n    enabled: false\n", deprecatedChart, nil)
	current := loadElement(t, "stack:\n  current:\n    enabled: false\n", "apiVersion: v2\nname: current\nversion: 1.0.0\n", nil)
	notDeprecated := loadElement(t, "stack:\n  kept:\n    enabled: false\n", "apiVersion: v2\nname: kept\nversion: 0.1.0\ndeprecated: false\n", nil)
	onByDefault := loadElement(t, "# defaults\nstack:\n  legacy:\n    enabled: true\n", "apiVersion: v2\nname: legacy\nversion: 1.4.0\ndeprecated: true\n", nil)
	noStack := loadElement(t, "{}\n", deprecatedChart, nil)

	tests := []struct {
		name     string
		elements []*chart.Element
		layers   map[string]string // file name → content
		want     []string          // file:line, one per finding
	}{
		{
			name:     "no deprecated element",
			elements: []*chart.Element{current, notDeprecated},
			layers:   map[string]string{"a.yaml": "stack:\n  current: {enabled: true}\n  kept: {enabled: true}\n"},
		},
		{
			name:     "a layer turns a deprecated element on",
			elements: []*chart.Element{old, current},
			layers:   map[string]string{"a.yaml": "stack:\n  current:\n    enabled: true\n  old:\n    enabled: true\n"},
			want:     []string{"a.yaml:5"},
		},
		{
			name:     "every layer that turns it on is reported",
			elements: []*chart.Element{old},
			layers: map[string]string{
				"a.yaml": "stack:\n  old: {enabled: true}\n",
				"b.yaml": "stack:\n  old:\n    config: {}\n    enabled: true\n",
				"c.yaml": "stack:\n  old: {enabled: yes}\n", // YAML 1.1, as Helm reads it
			},
			want: []string{"a.yaml:2", "b.yaml:4", "c.yaml:2"},
		},
		{
			name:     "off, unset, null, non-boolean or other keys are silent",
			elements: []*chart.Element{old},
			layers: map[string]string{
				"off.yaml":    "stack:\n  old: {enabled: false}\n",
				"unset.yaml":  "stack:\n  old:\n    config: {}\n",
				"null.yaml":   "stack:\n  old:\n    enabled:\n",
				"string.yaml": "stack:\n  old: {enabled: maybe}\n",
				"other.yaml":  "global: {remediationAction: inform}\n",
				"empty.yaml":  "",
			},
		},
		{
			name:     "invalid and missing layers are left to RENDER000",
			elements: []*chart.Element{old},
			layers:   map[string]string{"bad.yaml": "stack: [\n"},
		},
		{
			name:     "on by default in the element's own values.yaml",
			elements: []*chart.Element{onByDefault},
			want:     []string{"values.yaml:4"},
		},
		{
			name:     "an element with no stack block names no key",
			elements: []*chart.Element{noStack},
			layers:   map[string]string{"a.yaml": "stack:\n  old: {enabled: true}\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			layers := []string{filepath.Join(dir, "missing.yaml")}
			for name, content := range tt.layers {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
				layers = append(layers, path)
			}
			got := (&DeprecationCheck{}).Run(Context{AllElements: tt.elements, Layers: layers})
			var locs []string
			for _, f := range got {
				if f.RuleID != "POLICY092" || f.Severity != SevWarning || !strings.Contains(f.Message, `element "`+f.Element+`"`) {
					t.Errorf("finding %+v: want a POLICY092 warning naming its element", f)
				}
				locs = append(locs, filepath.Base(f.File)+":"+strconv.Itoa(f.Line))
			}
			slices.Sort(locs)
			if !slices.Equal(locs, tt.want) {
				t.Errorf("findings at %q, want %q: %+v", locs, tt.want, got)
			}
		})
	}
}
