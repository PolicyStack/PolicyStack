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

// certManager is a well-formed sibling: policy "install" exists, is enabled, and has an operator
// attached, so policy-library renders the Policy. "bare" has nothing attached.
func certManager(t *testing.T) *chart.Element {
	return elementNamed(t, "cert-manager", "certManager", `
    policies:
      - name: install
        enabled: true
      - name: bare
        enabled: true
    operatorPolicies:
      cert-manager:
        enabled: true
        policyRef: install
`)
}

func runDeps(els ...*chart.Element) []Finding {
	return (&DependencyCheck{}).Run(Context{AllElements: els})
}

func TestPolicy011_ValidReferencesAreSilent(t *testing.T) {
	el := elementNamed(t, "loki", "loki", `
    policies:
      - name: install
        enabled: true
      - name: stack
        enabled: true
        dependencies:
          - name: install
          - name: install
            element: cert-manager
          - name: anything
            raw: true
          - name: pinned
            release: other-thing-somecluster
    operatorPolicies:
      loki:
        enabled: true
        policyRef: install
    configPolicies:
      - name: lokistack
        enabled: true
        policyRef: stack
        waitForOperator: loki
        extraDependencies:
          - name: loki-status
            policyRef: install
`)
	if got := runDeps(el, certManager(t)); len(got) != 0 {
		t.Fatalf("expected no findings, got %d: %+v", len(got), got)
	}
}

func TestPolicy011_DanglingAndBadReferences(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"same-element typo", `
    policies:
      - name: install
        enabled: true
      - name: stack
        enabled: true
        dependencies:
          - name: instal
    operatorPolicies:
      op:
        enabled: true
        policyRef: install
    configPolicies:
      - name: cp
        enabled: true
        policyRef: stack
`, "not defined in policies[] of this element"},

		{"unknown element", `
    policies:
      - name: stack
        enabled: true
        dependencies:
          - name: install
            element: cert-manger
    configPolicies:
      - name: cp
        enabled: true
        policyRef: stack
`, "is not an element under stack/"},

		{"known element, missing policy", `
    policies:
      - name: stack
        enabled: true
        dependencies:
          - name: nope
            element: cert-manager
    configPolicies:
      - name: cp
        enabled: true
        policyRef: stack
`, `not defined in policies[] of element "cert-manager"`},

		{"target policy renders nothing", `
    policies:
      - name: stack
        enabled: true
        dependencies:
          - name: bare
            element: cert-manager
    configPolicies:
      - name: cp
        enabled: true
        policyRef: stack
`, "has no enabled sub-policy attached"},

		{"element on a template kind", `
    policies:
      - name: stack
        enabled: true
    configPolicies:
      - name: cp
        enabled: true
        policyRef: stack
        extraDependencies:
          - name: other
            kind: ConfigurationPolicy
            element: cert-manager
`, "element: is only valid for kind Policy"},

		{"waitForOperator typo", `
    policies:
      - name: stack
        enabled: true
    configPolicies:
      - name: cp
        enabled: true
        policyRef: stack
        waitForOperator: nosuchop
`, "matches no enabled operatorPolicies.nosuchop"},

		{"extraDependencies typo", `
    policies:
      - name: stack
        enabled: true
    configPolicies:
      - name: cp
        enabled: true
        policyRef: stack
        extraDependencies:
          - name: ghost
`, "which no enabled configPolicies/operatorPolicies/certificatePolicies entry declares"},
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
			if got[0].RuleID != "POLICY011" || got[0].Severity != SevError {
				t.Errorf("unexpected rule/severity: %s/%v", got[0].RuleID, got[0].Severity)
			}
		})
	}
}

func TestPolicy011_HardDisabledTargetIsAnError(t *testing.T) {
	// A target disabled outright, with no toggle governing it, can never be satisfied.
	sibling := elementNamed(t, "cert-manager", "certManager", `
    policies:
      - name: install
        enabled: false
    operatorPolicies:
      cert-manager:
        enabled: true
        policyRef: install
`)
	el := elementNamed(t, "loki", "loki", `
    policies:
      - name: stack
        enabled: true
        dependencies:
          - name: install
            element: cert-manager
    configPolicies:
      - name: cp
        enabled: true
        policyRef: stack
`)
	got := runDeps(el, sibling)
	if len(got) != 1 || !strings.Contains(got[0].Message, "is disabled") {
		t.Fatalf("expected a disabled-target finding, got %+v", got)
	}
}

func TestPolicy031_DeadKeys(t *testing.T) {
	cases := []struct {
		name, body, want string
		sev              Severity
	}{
		{"legacy enable", "stack:\n  myElement:\n    enable: true\n", "renders nothing", SevError},
		{"legacy defaultPolicy", "stack:\n  myElement:\n    enabled: true\n    defaultPolicy:\n      severity: high\n", "silently dropped", SevError},
		{"default.severity", "stack:\n  myElement:\n    enabled: true\n    default:\n      severity: high\n", "not consumed by policy-library", SevWarning},
		{"legacy operatorPolicies list", "stack:\n  myElement:\n    enabled: true\n    operatorPolicies:\n      - name: op\n        policyRef: p\n", "requires a map keyed by entry name", SevError},
		{"rawTemplate arity", "stack:\n  myElement:\n    enabled: true\n    configPolicies:\n      - name: cp\n        rawTemplate: true\n        templateNames:\n          - name: a\n          - name: b\n", "exactly one templateNames entry", SevError},
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
}

func TestPolicy010_ToggledOffParentIsNotAnError(t *testing.T) {
	// A sub-feature that ships off and is switched on per cluster is the intended shape.
	el := loadElement(t, `
stack:
  myElement:
    enabled: true
    toggles:
      optional: false
    policies:
      - name: optional
        enabled: true
    configPolicies:
      - name: cp
        enabled: true
        policyRef: optional
`, "", nil)
	if got := (&PolicyRefCheck{}).Run(Context{Element: el}); len(got) != 0 {
		t.Fatalf("toggle-controlled parent should not be reported, got: %+v", got)
	}

	// A parent disabled outright, with no toggle governing it, is still dead config.
	el = loadElement(t, `
stack:
  myElement:
    enabled: true
    policies:
      - name: dead
        enabled: false
    configPolicies:
      - name: cp
        enabled: true
        policyRef: dead
`, "", nil)
	got := (&PolicyRefCheck{}).Run(Context{Element: el})
	if len(got) != 1 || !strings.Contains(got[0].Message, "exists but is disabled") {
		t.Fatalf("expected the disabled-parent finding, got: %+v", got)
	}
}

func TestPolicy011_TogglesDoNotHideBrokenReferences(t *testing.T) {
	// A reference inside a toggled-OFF sub-feature must still resolve. Toggles are per-cluster
	// switches, so skipping these would let a typo surface only when someone turns it on.
	el := elementNamed(t, "loki", "loki", `
    toggles:
      forwarder: false
    policies:
      - name: forwarder
        enabled: true
        dependencies:
          - name: instal
            element: cert-manager
    configPolicies:
      - name: cp
        enabled: true
        policyRef: forwarder
`)
	got := runDeps(el, certManager(t))
	if len(got) != 1 || !strings.Contains(got[0].Message, "not defined in policies[]") {
		t.Fatalf("expected the dangling reference to be reported, got: %+v", got)
	}
}

func TestPolicy011_ToggledOffTargetIsNotAnError(t *testing.T) {
	// A toggle-governed target that is off by default is valid; a cluster values file can turn it on.
	sibling := elementNamed(t, "cert-manager", "certManager", `
    toggles:
      install: false
    policies:
      - name: install
        enabled: true
    operatorPolicies:
      cert-manager:
        enabled: true
        policyRef: install
`)
	el := elementNamed(t, "loki", "loki", `
    policies:
      - name: stack
        enabled: true
        dependencies:
          - name: install
            element: cert-manager
    configPolicies:
      - name: cp
        enabled: true
        policyRef: stack
`)
	if got := runDeps(el, sibling); len(got) != 0 {
		t.Fatalf("a toggle-governed target should not be reported, got: %+v", got)
	}
}

func TestPolicy003_CrossElementTemplateNameCollision(t *testing.T) {
	// Two elements each with policies[].name=install and
	// configPolicies[].name=ns-monitoring both render a ConfigurationPolicy called
	// "install-ns-monitoring". ACM rejects the second with "Template name must be unique".
	mk := func(chartName, key string) *chart.Element {
		return elementNamed(t, chartName, key, `
    policies:
      - name: install
        enabled: true
    configPolicies:
      - name: ns-monitoring
        enabled: true
        policyRef: install
`)
	}
	got := (&SubPolicyNameCheck{}).Run(Context{AllElements: []*chart.Element{
		mk("cert-manager", "certManager"), mk("cluster-observability", "clusterObservability"),
	}})
	if len(got) != 2 {
		t.Fatalf("expected both elements reported, got %d: %+v", len(got), got)
	}
	for _, f := range got {
		if !strings.Contains(f.Message, "install-ns-monitoring") || f.RuleID != "POLICY003" {
			t.Errorf("unexpected finding: %+v", f)
		}
	}
}

func TestPolicy003_ElementSpecificNamesAreSilent(t *testing.T) {
	a := elementNamed(t, "cert-manager", "certManager", `
    policies:
      - name: install
        enabled: true
    configPolicies:
      - name: cert-manager-ns-monitoring
        enabled: true
        policyRef: install
`)
	b := elementNamed(t, "cluster-observability", "clusterObservability", `
    policies:
      - name: install
        enabled: true
    configPolicies:
      - name: cluster-observability-ns-monitoring
        enabled: true
        policyRef: install
`)
	if got := (&SubPolicyNameCheck{}).Run(Context{AllElements: []*chart.Element{a, b}}); len(got) != 0 {
		t.Fatalf("expected no findings, got: %+v", got)
	}
}

func TestPolicy003_CoversToggledOffAndOperatorTriples(t *testing.T) {
	// Covers a collision behind an off-by-default toggle, and the -ns / -status templates an
	// operatorPolicy renders.
	mk := func(chartName, key string) *chart.Element {
		return elementNamed(t, chartName, key, `
    toggles:
      aws: false
    policies:
      - name: aws
        enabled: true
    operatorPolicies:
      thing:
        enabled: true
        policyRef: aws
`)
	}
	got := (&SubPolicyNameCheck{}).Run(Context{AllElements: []*chart.Element{
		mk("infra-nodes", "infraNodes"), mk("storage-nodes", "storageNodes"),
	}})
	names := map[string]bool{}
	for _, f := range got {
		for _, n := range []string{"aws-thing", "aws-thing-ns", "aws-thing-status"} {
			if strings.Contains(f.Message, "\""+n+"\"") {
				names[n] = true
			}
		}
	}
	for _, n := range []string{"aws-thing", "aws-thing-ns", "aws-thing-status"} {
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
      - name: present
        enabled: true
    configPolicies:
      - name: guard
        enabled: true
        policyRef: present
`
	broken := elementNamed(t, "lifecycle", "hypershift", ok+`        extraDependencies:
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
