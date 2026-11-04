package chart

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// TestLoad40 loads testdata/element: discovery, partials, file reuse, the install Policy and the
// defaults chain.
func TestLoad40(t *testing.T) {
	el, err := Load("testdata/element", "policy")
	if err != nil {
		t.Fatal(err)
	}
	c := el.Values.Component
	if c.V3 {
		t.Fatal("V3 set on a 4.0 element")
	}
	files := map[string][]string{}
	for _, s := range c.SubPolicies() {
		files[s.Name] = s.Files
	}
	want := map[string][]string{
		"config-multi":               {"config/multi"},
		"config-rawlist":             {"config/rawlist"},
		"config-reused":              {"_shared/configmap"}, // file: replaces converters/config/reused.yaml
		"config-pair":                {"_shared/configmap", "gates/version"},
		"config-off":                 {"_shared/configmap"},
		"gates-version":              {"gates/version"},
		"install-instance":           {"install/instance"},
		"unused-cm":                  {"_shared/configmap"},
		"install-my-operator":        nil,
		"install-my-operator-ns":     nil,
		"install-my-operator-status": nil,
		"install-other-operator":     nil, "install-other-operator-ns": nil, "install-other-operator-status": nil,
	}
	if len(files) != len(want) {
		t.Errorf("sub-policies %v, want %v", slices.Sorted(maps.Keys(files)), slices.Sorted(maps.Keys(want)))
	}
	for name, w := range want {
		if got, ok := files[name]; !ok || !slices.Equal(got, w) {
			t.Errorf("%s files %v, want %v", name, got, w)
		}
	}

	config, gates, install := c.Policies["config"], c.Policies["gates"], c.Policies["install"]
	for _, tt := range []struct {
		name, got, want string
	}{
		{"config action", c.Action(config), "inform"},
		{"install action from default", c.Action(install), "enforce"},
		{"InformOnly template", c.TemplateAction(gates, gates.Templates["version"]), "InformOnly"},
		{"template takes its policy's action", c.TemplateAction(config, config.Templates["multi"]), "inform"},
		{"template severity", c.Severity(config, config.Templates["multi"]), "high"},
		{"severity from default", c.Severity(config, config.Templates["pair"]), "medium"},
		{"orderManifests", strings.Join(config.OrderManifests, ","), "rawlist"},
	} {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
	if !install.Templates["instance"].Pending() || config.Templates["rawlist"].Pending() {
		t.Error("waitForOperator implies ignorePending unless it is set false")
	}
	if c.Policies["unused"].On() || config.Templates["off"].On() || c.OperatorPolicies["other-operator"].On() || !config.On() {
		t.Error("enabled: false turns an entry off, and every other entry is on")
	}
	if got := strings.Join(config.Templates["multi"].Path, "."); got != "policies.config.templates.multi" {
		t.Errorf("path %s", got)
	}
	if c.Severity(&Policy{}, nil) != "medium" || (&Component{}).Severity(&Policy{}, nil) != "low" || (&Component{}).Action(&Policy{}) != "inform" {
		t.Error("severity and remediationAction fall back to default, then low and inform")
	}
}

// TestLoadV3 detects the 3.x shape without modelling it, so Render Diff can load a revision pinned
// before 4.0.
func TestLoadV3(t *testing.T) {
	for name, body := range map[string]string{
		"policies list":  "    policies:\n      - name: install\n        enabled: true\n",
		"configPolicies": "    configPolicies:\n      - name: cm\n        policyRef: install\n        templateNames: [cm]\n",
		"toggles":        "    toggles: {cm: true}\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for n, b := range map[string]string{
				"Chart.yaml":          "apiVersion: v2\nname: old\nversion: 0.1.0\n",
				"values.yaml":         "stack:\n  old:\n    enabled: true\n" + body,
				"converters/cm.yaml":  "kind: ConfigMap\n",
				"converters/x/y.yaml": "kind: ConfigMap\n",
			} {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, n)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, n), []byte(b), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			el, err := Load(dir, "policy")
			if err != nil {
				t.Fatal(err)
			}
			if c := el.Values.Component; !c.V3 || len(c.SubPolicies()) != 0 {
				t.Errorf("V3 %v, sub-policies %v: want V3 and nothing modelled or discovered", c.V3, c.SubPolicies())
			}
		})
	}
}
