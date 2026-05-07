package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/cascade"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

const (
	stackKey = "myElement"
)

// loadElement is a test helper: writes values.yaml + Chart.yaml in a temp
// dir and parses them via chart.Load.
func loadElement(t *testing.T, valuesYAML string, chartYAML string, convertersFiles []string) *chart.Element {
	t.Helper()
	dir := t.TempDir()
	if chartYAML == "" {
		chartYAML = `apiVersion: v2
name: my-element
version: 0.1.0
dependencies:
  - name: policy-library
    version: "1.1.0"
    repository: https://policystack.github.io/PolicyStack-chart
`
	}
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(chartYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), []byte(valuesYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(convertersFiles) > 0 {
		_ = os.MkdirAll(filepath.Join(dir, "converters"), 0o755)
		for _, n := range convertersFiles {
			_ = os.WriteFile(filepath.Join(dir, "converters", n), []byte("kind: ConfigMap\n"), 0o644)
		}
	}
	el, err := chart.Load(dir, "policy")
	if err != nil {
		t.Fatal(err)
	}
	return el
}

func TestPolicy001_ParentPolicyReplicated(t *testing.T) {
	// "policy." + "long-name-..." + "-my-element-huge-cluster-name" must exceed 63.
	el := loadElement(t, `
stack:
  myElement:
    enable: true
    policies:
      - name: short
        enabled: true
      - name: this-is-a-deliberately-very-long-policy-name-overflow
        enabled: true
`, "", nil)
	// PolicyNamespace defaults to "policy" via loadElement.
	cl := &cascade.Resolved{ClusterName: "huge-cluster-name", ReleaseName: "my-element-huge-cluster-name"}
	got := (&NameLengthCheck{}).Run(Context{Element: el, Cluster: cl})
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "ACM-replicated") {
		t.Errorf("expected message to mention ACM-replicated form, got: %s", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "metadata.name=") {
		t.Errorf("expected message to disclose metadata.name, got: %s", got[0].Message)
	}
}

func TestPolicy001_SubPolicyNoNamespacePrefix(t *testing.T) {
	// Sub-policy metadata.name = <parent>-<sub>; no namespace, no Release.Name.
	// Construct a name that's >63 only because of the parent-sub concatenation.
	el := loadElement(t, `
stack:
  myElement:
    enable: true
    policies:
      - name: parent-policy-with-a-medium-length-name
        enabled: true
    configPolicies:
      - name: child-policy-with-yet-another-medium-length-name
        enabled: true
        policyRef: parent-policy-with-a-medium-length-name
`, "", nil)
	cl := &cascade.Resolved{ClusterName: "x", ReleaseName: "my-element-x"}
	got := (&NameLengthCheck{}).Run(Context{Element: el, Cluster: cl})
	if len(got) == 0 {
		t.Fatalf("expected at least one finding, got 0")
	}
	// Parent policy shouldn't be flagged (its ACM-replicated form is < 63 here),
	// only the sub-policy should be — verify via message content.
	subFound := false
	for _, f := range got {
		if strings.Contains(f.Message, "ConfigurationPolicy") && !strings.Contains(f.Message, "ACM-replicated") {
			subFound = true
		}
	}
	if !subFound {
		t.Fatalf("expected a ConfigurationPolicy finding without ACM-replicated wording, got: %+v", got)
	}
}

func TestPolicy010_PolicyRefMissing(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enable: true
    policies:
      - name: real
        enabled: true
    configPolicies:
      - name: typo
        enabled: true
        policyRef: realll
`, "", nil)
	got := (&PolicyRefCheck{}).Run(Context{Element: el})
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "not defined") {
		t.Errorf("unexpected: %s", got[0].Message)
	}
}

func TestPolicy010_PolicyRefDisabled(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enable: true
    policies:
      - name: real
        enabled: false
    configPolicies:
      - name: child
        enabled: true
        policyRef: real
`, "", nil)
	got := (&PolicyRefCheck{}).Run(Context{Element: el})
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "disabled") {
		t.Errorf("unexpected: %s", got[0].Message)
	}
}

func TestPolicy020_MissingConverter(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enable: true
    configPolicies:
      - name: c
        enabled: true
        policyRef: foo
        templateNames:
          - name: present
          - name: missing
`, "", []string{"present.yaml"})
	got := (&MissingConverterCheck{}).Run(Context{Element: el})
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "missing.yaml") {
		t.Errorf("unexpected: %s", got[0].Message)
	}
}

func TestPolicy020_BareStringTemplateName(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enable: true
    configPolicies:
      - name: c
        enabled: true
        policyRef: foo
        templateNames:
          - bare-form
`, "", nil)
	got := (&MissingConverterCheck{}).Run(Context{Element: el})
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "bare-form") {
		t.Errorf("unexpected: %s", got[0].Message)
	}
}

func TestPolicy021_UnusedConverter(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enable: true
    configPolicies:
      - name: c
        enabled: true
        policyRef: foo
        templateNames:
          - name: present
`, "", []string{"present.yaml", "dead.yaml"})
	got := (&UnusedConverterCheck{}).Run(Context{Element: el})
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if got[0].Severity != SevWarning {
		t.Error("expected warning severity")
	}
	if !strings.Contains(got[0].Message, "dead") {
		t.Errorf("unexpected: %s", got[0].Message)
	}
}

func TestPolicy030_BadEnums(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enable: true
    policies:
      - name: p
        enabled: true
        severity: extreme
        remediationAction: nuke
`, "", nil)
	got := (&EnumCheck{}).Run(Context{Element: el})
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %+v", len(got), got)
	}
}

func TestPolicy040_PolicySetMissingMember(t *testing.T) {
	el := loadElement(t, `
stack:
  myElement:
    enable: true
    policies:
      - name: real
        enabled: true
    policySets:
      - name: set
        enabled: true
        policies:
          - real
          - imaginary
`, "", nil)
	got := (&PolicySetCheck{}).Run(Context{Element: el})
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "imaginary") {
		t.Errorf("unexpected: %s", got[0].Message)
	}
}

func TestPolicy090_CamelCaseMismatch(t *testing.T) {
	el := loadElement(t, `
stack:
  wrongKey:
    enable: false
`, "", nil)
	got := (&CamelCaseCheck{}).Run(Context{Element: el})
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "myElement") {
		t.Errorf("unexpected: %s", got[0].Message)
	}
}
