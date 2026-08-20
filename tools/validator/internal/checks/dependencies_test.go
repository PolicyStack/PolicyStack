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
// attached (so policy-library actually emits a Policy for it). "bare" has nothing attached.
func certManager(t *testing.T) *chart.Element {
	return elementNamed(t, "cert-manager", "certManager", `
    policies:
      - name: install
        enabled: true
      - name: bare
        enabled: true
    operatorPolicies:
      - name: cert-manager
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
      - name: loki
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
      - name: op
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
`, "matches no enabled entry in operatorPolicies[]"},

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
      - name: cert-manager
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
	// The mirror image: a target that is toggle-governed and currently off is fine - it is off on
	// this cluster only, and a cluster values file can turn it on.
	sibling := elementNamed(t, "cert-manager", "certManager", `
    toggles:
      install: false
    policies:
      - name: install
        enabled: true
    operatorPolicies:
      - name: cert-manager
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
