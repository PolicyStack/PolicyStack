package chart

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadComponents(t *testing.T) {
	const schema = `{"properties": {"install": {"properties": {"engine": {"enum": ["hypershift", "hive"]}}}}}`
	const two = "stack:\n  hypershift:\n    enabled: true\n  hive:\n    enabled: false\n"
	tests := []struct {
		name, values, schema string
		wantKeys             []string // StackKey of each returned Element
		wantComponents       int      // len(Components) on every Element
	}{
		{"element", "stack:\n  myElement:\n    enabled: true\n", "", []string{"myElement"}, 1},
		// Without an engine enum the chart is one element that checks only its first stack key.
		{"element with two keys", two, "", []string{"hypershift"}, 1},
		{"lifecycle", two, schema, []string{"hypershift", "hive"}, 2},
		{"lifecycle with one engine", "stack:\n  hypershift:\n    enabled: true\n", schema, []string{"hypershift"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{"Chart.yaml": "apiVersion: v2\nname: lifecycle\nversion: 0.1.0\n", "values.yaml": tt.values}
			if tt.schema != "" {
				files["values.schema.json"] = tt.schema
			}
			for n, b := range files {
				if err := os.WriteFile(filepath.Join(dir, n), []byte(b), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			els, err := LoadComponents(dir, "policy")
			if err != nil {
				t.Fatal(err)
			}
			if len(els) != len(tt.wantKeys) {
				t.Fatalf("got %d elements, want %d", len(els), len(tt.wantKeys))
			}
			for i, el := range els {
				if el.StackKey != tt.wantKeys[i] || len(el.Components) != tt.wantComponents {
					t.Errorf("[%d] StackKey %q with %d components, want %q with %d", i, el.StackKey, len(el.Components), tt.wantKeys[i], tt.wantComponents)
				}
				// Each Element checks its own component.
				if el.Values.Component != el.Components[i].Component {
					t.Errorf("[%d] Values.Component is not stack.%s", i, el.StackKey)
				}
			}
		})
	}
}
