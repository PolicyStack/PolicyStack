package checks

import (
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

// elementNamed builds an element whose Chart.yaml name and stack key are chosen by the caller, so
// cross-element `element:` references can be exercised.
func elementNamed(t *testing.T, chartName, key, body string) *chart.Element {
	t.Helper()
	chartYAML := "apiVersion: v2\nname: " + chartName + "\nversion: 0.1.0\n"
	return loadElement(t, "stack:\n  "+key+":\n    enabled: true\n"+body, chartYAML, nil)
}

// certManager is a well-formed sibling: policy install renders its operator, policy bare has nothing
// to render.
func certManager(t *testing.T) *chart.Element {
	return elementNamed(t, "cert-manager", "certManager", `
    policies:
      bare: {}
    operatorPolicies:
      cert-manager:
        subscription: {name: cert-manager}
`)
}

func runDeps(els ...*chart.Element) []Finding {
	return (&DependencyCheck{}).Run(Context{AllElements: els})
}

func TestPolicy011_ValidReferencesAreSilent(t *testing.T) {
	el := elementNamed(t, "loki", "loki", `
    policies:
      stack:
        dependencies:
          - name: install
          - name: install
            element: cert-manager
          - name: anything
            raw: true
          - name: pinned
            release: other-thing-somecluster
        templates:
          lokistack:
            waitForOperator: loki
            extraDependencies:
              - name: loki-status
                policyRef: install
              - name: other
          other: {}
    operatorPolicies:
      loki:
        subscription: {name: loki}
`)
	if got := runDeps(el, certManager(t)); len(got) != 0 {
		t.Fatalf("expected no findings, got %d: %+v", len(got), got)
	}
}

func TestPolicy011_DanglingAndBadReferences(t *testing.T) {
	const cp = `
        templates:
          cp: {}`
	cases := []struct{ name, body, want string }{
		{"same-element typo", `
    policies:
      stack:
        dependencies:
          - name: instal` + cp + `
    operatorPolicies:
      op:
        subscription: {name: op}
`, `dependency "instal" is not a policy of this element`},

		{"unknown element", `
    policies:
      stack:
        dependencies:
          - name: install
            element: cert-manger` + cp + `
`, "is not an element under stack/"},

		{"known element, missing policy", `
    policies:
      stack:
        dependencies:
          - name: nope
            element: cert-manager` + cp + `
`, `is not a policy of element "cert-manager"`},

		{"target policy renders nothing", `
    policies:
      stack:
        dependencies:
          - name: bare
            element: cert-manager` + cp + `
`, "has no template or operator"},

		{"element on a template kind", `
    policies:
      stack:
        templates:
          cp:
            extraDependencies:
              - name: other
                kind: ConfigurationPolicy
                element: cert-manager
`, "element: is only valid for kind Policy"},

		{"waitForOperator typo", `
    policies:
      stack:
        templates:
          cp:
            waitForOperator: nosuchop
`, "policies.stack.templates.cp: waitForOperator \"nosuchop\" matches no operatorPolicies.nosuchop"},

		{"extraDependencies typo", `
    policies:
      stack:
        templates:
          cp:
            extraDependencies:
              - name: ghost
`, `resolves to "stack-ghost", which no template or operator of policy "stack" renders`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runDeps(elementNamed(t, "loki", "loki", tc.body), certManager(t))
			if len(got) != 1 {
				t.Fatalf("expected exactly 1 finding, got %d: %+v", len(got), got)
			}
			if !strings.Contains(got[0].Message, tc.want) {
				t.Errorf("message %q does not contain %q", got[0].Message, tc.want)
			}
			if got[0].RuleID != "POLICY011" || got[0].Severity != SevError || got[0].Line == 0 {
				t.Errorf("unexpected rule/severity/line: %+v", got[0])
			}
		})
	}
}

// TestPolicy011_DisabledEntries: enabled: false hides nothing. A values layer may turn an entry on,
// so its references must resolve and a disabled target is valid.
func TestPolicy011_DisabledEntries(t *testing.T) {
	sibling := elementNamed(t, "cert-manager", "certManager", `
    policies:
      install:
        enabled: false
    operatorPolicies:
      cert-manager:
        enabled: false
        subscription: {name: cert-manager}
`)
	el := elementNamed(t, "loki", "loki", `
    policies:
      stack:
        enabled: false
        dependencies:
          - name: install
            element: cert-manager
          - name: instal
            element: cert-manager
        templates:
          cp:
            enabled: false
`)
	got := runDeps(el, sibling)
	if len(got) != 1 || !strings.Contains(got[0].Message, `"instal" is not a policy`) {
		t.Fatalf("got %+v, want only the instal finding", got)
	}
}

func TestPolicy031_DeadKeys(t *testing.T) {
	cases := []struct {
		name, body, want string
		sev              Severity
	}{
		{"legacy enable", "stack:\n  myElement:\n    enable: true\n", "renders nothing", SevError},
		{"legacy defaultPolicy", "stack:\n  myElement:\n    enabled: true\n    defaultPolicy:\n      severity: high\n", "silently dropped", SevError},
		{"default.disabled", "stack:\n  myElement:\n    enabled: true\n    default:\n      disabled: true\n", "not consumed by policy-library", SevWarning},
		{"legacy operatorPolicies list", "stack:\n  myElement:\n    enabled: true\n    operatorPolicies:\n      - name: op\n        policyRef: p\n", "requires a map keyed by entry name", SevError},
		{"3.x policies list", "stack:\n  myElement:\n    enabled: true\n    policies:\n      - name: p\n", "run tools/migrate-4.0.py", SevError},
		{"3.x toggles", "stack:\n  myElement:\n    enabled: true\n    toggles: {p: true}\n", "run tools/migrate-4.0.py", SevError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := loadElement(t, tc.body, "", nil)
			got := (&DeadKeyCheck{}).Run(Context{Element: el})
			if len(got) != 1 {
				t.Fatalf("expected 1 finding, got %d: %+v", len(got), got)
			}
			if !strings.Contains(got[0].Message, tc.want) {
				t.Errorf("message %q does not contain %q", got[0].Message, tc.want)
			}
			if got[0].Severity != tc.sev {
				t.Errorf("severity = %v, want %v", got[0].Severity, tc.sev)
			}
		})
	}
	// default.severity and default.remediationAction are the defaults chain in 4.0.
	el := loadElement(t, "stack:\n  myElement:\n    enabled: true\n    default: {severity: high, remediationAction: enforce}\n", "", nil)
	if got := (&DeadKeyCheck{}).Run(Context{Element: el}); len(got) != 0 {
		t.Errorf("default.severity reported: %+v", got)
	}
}

func TestPolicy003_CrossElementTemplateNameCollision(t *testing.T) {
	// Two elements each with template ns-monitoring of policy install render a ConfigurationPolicy
	// called install-ns-monitoring. ACM rejects the second with "Template name must be unique".
	mk := func(chartName, key string) *chart.Element {
		return elementNamed(t, chartName, key, `
    policies:
      install:
        templates:
          ns-monitoring: {}
`)
	}
	got := (&SubPolicyNameCheck{}).Run(Context{AllElements: []*chart.Element{
		mk("cert-manager", "certManager"), mk("cluster-observability", "clusterObservability"),
	}})
	if len(got) != 2 {
		t.Fatalf("expected both elements reported, got %d: %+v", len(got), got)
	}
	for _, f := range got {
		if !strings.Contains(f.Message, "install-ns-monitoring") || f.RuleID != "POLICY003" || f.Line != 8 {
			t.Errorf("unexpected finding: %+v", f)
		}
	}
}

func TestPolicy003_ElementSpecificNamesAreSilent(t *testing.T) {
	a := elementNamed(t, "cert-manager", "certManager", "    policies:\n      install:\n        templates:\n          cert-manager-ns: {}\n")
	b := elementNamed(t, "cluster-observability", "clusterObservability", "    policies:\n      install:\n        templates:\n          observability-ns: {}\n")
	if got := (&SubPolicyNameCheck{}).Run(Context{AllElements: []*chart.Element{a, b}}); len(got) != 0 {
		t.Fatalf("expected no findings, got: %+v", got)
	}
}

func TestPolicy003_CoversDisabledAndDiscoveredOperatorTriples(t *testing.T) {
	// Covers a disabled operator, the -ns / -status templates an operatorPolicy renders, and a
	// template discovered from converters/.
	mk := func(chartName, key string) *chart.Element {
		return loadElement(t, "stack:\n  "+key+":\n    enabled: true\n    operatorPolicies:\n      thing:\n        enabled: false\n        subscription: {name: thing}\n",
			"apiVersion: v2\nname: "+chartName+"\nversion: 0.1.0\n", []string{"config/cm.yaml"})
	}
	got := (&SubPolicyNameCheck{}).Run(Context{AllElements: []*chart.Element{
		mk("infra-nodes", "infraNodes"), mk("storage-nodes", "storageNodes"),
	}})
	names := map[string]bool{}
	for _, f := range got {
		for _, n := range []string{"install-thing", "install-thing-ns", "install-thing-status", "config-cm"} {
			if strings.Contains(f.Message, "\""+n+"\"") {
				names[n] = true
			}
		}
	}
	for _, n := range []string{"install-thing", "install-thing-ns", "install-thing-status", "config-cm"} {
		if !names[n] {
			t.Errorf("expected %q to be reported as colliding; got %+v", n, got)
		}
	}
}

// TestPolicy011_EveryLifecycleComponent: the lifecycle chart loads as one Element per engine, all
// named lifecycle. A broken reference in any of them is reported, not only in the last.
func TestPolicy011_EveryLifecycleComponent(t *testing.T) {
	const ok = `
    policies:
      present:
        templates:
          guard:
            file: guard
`
	broken := elementNamed(t, "lifecycle", "hypershift", ok+`            extraDependencies:
              - name: gaurd
`)
	fine := elementNamed(t, "lifecycle", "hive", ok)
	for _, order := range [][]*chart.Element{{broken, fine}, {fine, broken}} {
		got := runDeps(order...)
		if len(got) != 1 || !strings.Contains(got[0].Message, "gaurd") {
			t.Errorf("order %s,%s: got %+v", order[0].StackKey, order[1].StackKey, got)
		}
	}
}
