package acmtemplates

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/stolostron/go-template-utils/v7/pkg/templates"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
)

var complianceDir = filepath.Join(repoRoot, "stack/compliance")

const complianceValues = `stack:
  compliance:
    config:
      remediations:
        ocp4-stig-apply: true
        ocp4-stig-pass: true
        ocp4-stig-typo: true
        ocp4-stig-revert: false
        ocp4-stig-kubelet: false
        ocp4-stig-absent: false
        ocp4-stig-nulled: null
      waivers:
        rule-waived: "Logs ship off-cluster. SEC-1234"
        rule-nulled: null
`

// complianceFiles returns allOn plus a value file holding values.
func complianceFiles(t *testing.T, values string) []string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "values.yaml")
	if err := os.WriteFile(f, []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	return append(slices.Clone(allOn), f)
}

func complianceRaw(t *testing.T) map[string]string {
	t.Helper()
	return renderRaw(t, complianceDir, "compliance", complianceFiles(t, complianceValues))
}

func complianceObject(kind, name, body string) string {
	return fmt.Sprintf("apiVersion: compliance.openshift.io/v1alpha1\nkind: %s\nmetadata: {name: %s, namespace: openshift-compliance}\n%s\n", kind, name, body)
}

// scan is a ComplianceScan of suite stig that started the given time ago. A zero age has no startTimestamp.
func scan(name, phase, result string, age time.Duration) string {
	started := ""
	if age != 0 {
		started = fmt.Sprintf(", startTimestamp: %q", time.Now().Add(-age).Format(time.RFC3339))
	}
	return fmt.Sprintf(`apiVersion: compliance.openshift.io/v1alpha1
kind: ComplianceScan
metadata:
  name: %s
  namespace: openshift-compliance
  labels: {compliance.openshift.io/suite: stig}
status: {phase: %s, result: %s%s}
`, name, phase, result, started)
}

func scanDone() string { return scan("ocp4-stig", "DONE", "NON-COMPLIANT", time.Hour) }

func foreign(obj string) string {
	return strings.Replace(obj, "namespace: openshift-compliance", "namespace: other", 1)
}

// result is a ComplianceCheckResult of suite stig. annotations is appended to the rule annotation.
func result(name, rule, status, severity, annotations string) string {
	return suiteResult(name, rule, status, severity, "stig", annotations)
}

func suiteResult(name, rule, status, severity, suiteName, annotations string) string {
	return fmt.Sprintf(`apiVersion: compliance.openshift.io/v1alpha1
kind: ComplianceCheckResult
metadata:
  name: %s
  namespace: openshift-compliance
  labels:
    compliance.openshift.io/suite: %s
    compliance.openshift.io/check-status: %s
    compliance.openshift.io/check-severity: %s
  annotations: {compliance.openshift.io/rule: %s%s}
status: %s
severity: %s
`, name, suiteName, status, severity, rule, annotations, status, severity)
}

func remediation(name, kind string, apply bool, state string) string {
	return complianceObject("ComplianceRemediation", name, fmt.Sprintf("spec: {apply: %v, current: {object: {kind: %s}}}\nstatus: {applicationState: %s}", apply, kind, state))
}

// summary lists objs as "complianceType Kind/name", sorted, and requires them in the operator's namespace.
func summary(t *testing.T, objs []object) []string {
	t.Helper()
	out := []string{}
	for _, o := range objs {
		if ns := o.ObjectDefinition.GetNamespace(); ns != "openshift-compliance" {
			t.Errorf("%s in namespace %q", o.ObjectDefinition.GetName(), ns)
		}
		out = append(out, o.ComplianceType+" "+o.ObjectDefinition.GetKind()+"/"+o.ObjectDefinition.GetName())
	}
	slices.Sort(out)
	return out
}

func TestComplianceScanState(t *testing.T) {
	raw := complianceRaw(t)["findings-scan-state"]
	suite := complianceObject("ComplianceSuite", "stig", "status: {phase: DONE, result: ERROR, scanStatuses: [{name: dropped, phase: DONE, result: ERROR, startTimestamp: \"2020-01-01T00:00:00Z\"}]}")
	passing := result("c", "r", "PASS", "high", "")
	otherSuite := suiteResult("o", "r", "PASS", "high", "other", "")
	const hour = time.Hour

	tests := []struct {
		name      string
		live      []string
		noResults bool     // reports the missing results
		bad       []string // scans asserted Healthy, the violation
	}{
		{name: "nothing yet", noResults: true},
		{name: "first scan", live: []string{suite, scan("a", "RUNNING", "NOT-AVAILABLE", hour)}, noResults: true},
		{name: "rescan keeps results", live: []string{suite, scan("a", "RUNNING", "NOT-AVAILABLE", hour), passing}},
		{name: "rules failed is not a scan problem", live: []string{suite, scan("a", "DONE", "NON-COMPLIANT", hour), passing}},
		{name: "done with no results", live: []string{suite, scan("a", "DONE", "COMPLIANT", hour)}, noResults: true},
		{name: "another suite's results do not count", live: []string{suite, scan("a", "DONE", "COMPLIANT", hour), otherSuite}, noResults: true},
		{name: "error", live: []string{suite, scan("a", "DONE", "ERROR", hour), passing}, bad: []string{"a"}},
		{name: "error with no results", live: []string{suite, scan("a", "DONE", "ERROR", hour)}, noResults: true, bad: []string{"a"}},
		{name: "not available", live: []string{suite, scan("a", "DONE", "NOT-AVAILABLE", hour), passing}, bad: []string{"a"}},
		{name: "not applicable", live: []string{suite, scan("a", "DONE", "NOT-APPLICABLE", hour), passing}, bad: []string{"a"}},
		{name: "one bad scan of two", live: []string{suite, scan("a", "DONE", "COMPLIANT", hour), scan("b", "DONE", "ERROR", hour), passing}, bad: []string{"b"}},
		{name: "47 hours is current", live: []string{suite, scan("a", "DONE", "COMPLIANT", 47*hour), passing}},
		{name: "49 hours is stale", live: []string{suite, scan("a", "DONE", "COMPLIANT", 49*hour), passing}, bad: []string{"a"}},
		{name: "one stale scan of two", live: []string{suite, scan("a", "DONE", "COMPLIANT", hour), scan("b", "DONE", "COMPLIANT", 72*hour), passing}, bad: []string{"b"}},
		{name: "wedged in progress", live: []string{suite, scan("a", "RUNNING", "NOT-AVAILABLE", 72*hour), passing}, bad: []string{"a"}},
		{name: "no timestamp is not stale", live: []string{suite, scan("a", "PENDING", "NOT-AVAILABLE", 0), passing}},
		{name: "dropped scan in suite status is ignored", live: []string{suite, scan("a", "DONE", "COMPLIANT", hour), passing}},
		{name: "another namespace's scan is ignored", live: []string{suite, foreign(scan("a", "DONE", "ERROR", hour)), passing}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := resolve(t, raw, tt.live...)
			want := []string{"musthave ComplianceSuite/stig", "musthave Profile/ocp4-stig", "musthave Profile/ocp4-stig-node", "musthave Profile/rhcos4-stig"}
			if tt.noResults {
				want = append(want, "musthave ComplianceCheckResult/")
			}
			for _, b := range tt.bad {
				want = append(want, "musthave ComplianceScan/"+b)
			}
			slices.Sort(want)
			if got := summary(t, objs); !slices.Equal(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
			for _, b := range tt.bad {
				if got := field(find(t, objs, "ComplianceScan", b), "status", "phase"); got != "Healthy" {
					t.Errorf("%s asserted phase %q", b, got)
				}
			}
			if nested(find(t, objs, "ComplianceSuite", "stig"), "status") != nil {
				t.Error("the suite's status must not be asserted")
			}
			if tt.noResults {
				if got := field(find(t, objs, "ComplianceCheckResult", ""), "metadata", "labels", "compliance.openshift.io/suite"); got != "stig" {
					t.Errorf("results assertion selects suite %q", got)
				}
			}
		})
	}
}

func TestComplianceFindings(t *testing.T) {
	raw := complianceRaw(t)
	inconsistent := func(name, source, common string) string {
		return result(name, "rule-"+name, "INCONSISTENT", "high", fmt.Sprintf(", compliance.openshift.io/inconsistent-source: %q, compliance.openshift.io/most-common-status: %s", source, common))
	}
	live := []string{
		result("fail", "rule-fail", "FAIL", "high", ""),
		result("waived-fail", "rule-waived", "FAIL", "high", ""),
		result("waived-error", "rule-waived", "ERROR", "high", ""),
		result("waived-pass", "rule-waived", "PASS", "low", ""),
		result("nulled", "rule-nulled", "FAIL", "high", ""),
		result("error", "rule-error", "ERROR", "high", ""),
		result("pass", "rule-pass", "PASS", "high", ""),
		result("manual", "rule-manual", "MANUAL", "high", ""),
		suiteResult("other-suite", "rule-other", "FAIL", "high", "other", ""),
		inconsistent("minority-fail", "node-1:FAIL", "PASS"),
		inconsistent("majority-fail", "node-1:PASS", "FAIL"),
		inconsistent("benign", "node-1:NOT-APPLICABLE", "PASS"),
		inconsistent("node-error", "node-1:ERROR", "PASS"),
		result("waived-node-fail", "rule-waived", "INCONSISTENT", "high", ", compliance.openshift.io/inconsistent-source: \"node-1:FAIL\", compliance.openshift.io/most-common-status: PASS"),
		result("waived-node-error", "rule-waived", "INCONSISTENT", "high", ", compliance.openshift.io/inconsistent-source: \"node-1:ERROR\", compliance.openshift.io/most-common-status: PASS"),
		suiteResult("other-waived", "rule-waived", "FAIL", "high", "other", ""),
		foreign(result("other-namespace", "rule-other", "FAIL", "high", "")),
		result("medium", "rule-medium", "FAIL", "medium", ""),
		result("low", "rule-low", "FAIL", "low", ""),
		result("unknown", "rule-unknown", "FAIL", "unknown", ""),
	}
	violations := func(names ...string) []string {
		out := []string{}
		for _, n := range names {
			out = append(out, "mustnothave ComplianceCheckResult/"+n)
		}
		slices.Sort(out)
		return out
	}
	for name, want := range map[string][]string{
		"findings-high":   violations("fail", "waived-error", "nulled", "error", "minority-fail", "majority-fail", "node-error", "waived-node-error"),
		"findings-medium": violations("medium"),
		"findings-low":    violations("low", "unknown"),
	} {
		if got := summary(t, resolve(t, raw[name], live...)); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
		if got := resolve(t, raw[name]); len(got) != 0 {
			t.Errorf("%s with no results: %v", name, got)
		}
	}

	want := []string{"musthave ComplianceCheckResult/waived-error", "musthave ComplianceCheckResult/waived-fail", "musthave ComplianceCheckResult/waived-node-error", "musthave ComplianceCheckResult/waived-node-fail", "musthave ComplianceCheckResult/waived-pass"}
	if got := summary(t, resolve(t, raw["findings-waived"], live...)); !slices.Equal(got, want) {
		t.Errorf("waived: got %v, want %v", got, want)
	}
}

func TestComplianceDrift(t *testing.T) {
	raw := complianceRaw(t)["findings-drift"]
	objs := resolve(t, raw,
		remediation("ocp4-stig-apply", "MachineConfig", true, "Applied"),
		remediation("ocp4-stig-apply-1", "MachineConfig", true, "Applied"),
		remediation("ocp4-stig-apply-extra", "MachineConfig", true, "Applied"),
		remediation("ocp4-stig-removed", "MachineConfig", true, "Applied"),
		remediation("ocp4-stig-revert", "MachineConfig", true, "Applied"),
		remediation("ocp4-stig-idle", "MachineConfig", false, "NotApplied"),
		foreign(remediation("ocp4-stig-foreign", "MachineConfig", true, "Applied")),
	)
	want := []string{"musthave ComplianceRemediation/ocp4-stig-apply-extra", "musthave ComplianceRemediation/ocp4-stig-removed", "musthave ComplianceRemediation/ocp4-stig-revert"}
	if got := summary(t, objs); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, o := range objs {
		if nested(o, "spec") != nil || field(o, "status", "applicationState") != "NotApplied" {
			t.Errorf("%s must assert status only: %v", o.ObjectDefinition.GetName(), o.ObjectDefinition.Object)
		}
	}

	// The operator keeps names equal after removing -N in step, wherever it occurs in the name.
	family := renderRaw(t, complianceDir, "compliance", complianceFiles(t, "stack:\n  compliance:\n    config:\n      remediations: {ocp4-cis-foo: true}\n"))["findings-drift"]
	if got := resolve(t, family, remediation("ocp4-cis-1-9-foo", "MachineConfig", true, "Applied")); len(got) != 0 {
		t.Errorf("a sibling the operator applied is not drift: %v", summary(t, got))
	}

	none := renderRaw(t, complianceDir, "compliance", allOn)["findings-drift"]
	if got := summary(t, resolve(t, none, remediation("x", "MachineConfig", true, "Applied"))); len(got) != 1 {
		t.Errorf("nothing declared must report every applied remediation: %v", got)
	}
}

func TestComplianceRemediate(t *testing.T) {
	raw := complianceRaw(t)["remediate-remediations"]
	done := scanDone()
	present := []string{
		remediation("ocp4-stig-apply", "MachineConfig", false, "NotApplied"),
		remediation("ocp4-stig-revert", "MachineConfig", true, "Applied"),
		remediation("ocp4-stig-kubelet", "KubeletConfig", true, "Applied"),
		remediation("ocp4-stig-nulled", "MachineConfig", true, "Applied"),
	}

	objs := resolve(t, raw, append(present, done)...)
	want := map[string]bool{"ocp4-stig-apply": true, "ocp4-stig-revert": false, "ocp4-stig-kubelet": false}
	if len(objs) != len(want) {
		t.Fatalf("only existing, non-null names: %v", summary(t, objs))
	}
	for _, o := range objs {
		n := o.ObjectDefinition.GetName()
		if o.ComplianceType != "musthave" || nested(o, "spec", "apply") != want[n] {
			t.Errorf("%s: %s apply %v, want %v", n, o.ComplianceType, nested(o, "spec", "apply"), want[n])
		}
	}
	if got := resolve(t, raw, done); len(got) != 0 {
		t.Errorf("a missing name must not be created: %v", summary(t, got))
	}
	if got := resolve(t, raw, append(present, done, scan("b", "RUNNING", "NOT-AVAILABLE", time.Hour))...); len(got) != 0 {
		t.Errorf("no writes while any scan runs: %v", summary(t, got))
	}
	if got := resolve(t, raw, present...); len(got) != 0 {
		t.Errorf("no writes without a scan: %v", summary(t, got))
	}
}

func TestComplianceRemediationState(t *testing.T) {
	raw := complianceRaw(t)["remediate-remediation-state"]
	objs := resolve(t, raw,
		scanDone(),
		remediation("ocp4-stig-nulled", "MachineConfig", true, "Applied"),
		remediation("ocp4-stig-apply", "MachineConfig", true, "Outdated"),
		remediation("ocp4-stig-revert", "MachineConfig", false, "NotApplied"),
		remediation("ocp4-stig-kubelet", "KubeletConfig", false, "NotApplied"),
		result("ocp4-stig-pass", "pass", "PASS", "low", ""),
	)
	want := map[string]string{
		"ocp4-stig-apply":   "Applied",        // Outdated is a violation
		"ocp4-stig-typo":    "Applied",        // missing and not passing is a violation
		"ocp4-stig-revert":  "NotApplied",     // satisfied
		"ocp4-stig-kubelet": "RevertManually", // apply: false does not undo it
	}
	if len(objs) != len(want) {
		t.Fatalf("got %v, want %v", summary(t, objs), want)
	}
	for _, o := range objs {
		n := o.ObjectDefinition.GetName()
		if got := field(o, "status", "applicationState"); got != want[n] || nested(o, "spec") != nil || o.ComplianceType != "musthave" {
			t.Errorf("%s: state %q, want %q, object %v", n, got, want[n], o.ObjectDefinition.Object)
		}
	}
	if got := resolve(t, raw, scan("a", "RUNNING", "NOT-AVAILABLE", time.Hour)); len(got) != 0 {
		t.Errorf("states are in flux while a scan runs: %v", summary(t, got))
	}
}

// TestComplianceMessages runs every customMessage with and without related objects, as the
// controller does (controllers/configurationpolicy_controller.go doCustomMessage).
func TestComplianceMessages(t *testing.T) {
	funcs := template.FuncMap{}
	all := sprig.TxtFuncMap()
	for _, n := range templates.AvailableSprigFunctions() {
		funcs[n] = all[n]
	}
	objects := func(docs ...string) []any {
		out := []any{}
		for _, d := range docs {
			var o map[string]any
			if err := yaml.Unmarshal([]byte(d), &o); err != nil {
				t.Fatal(err)
			}
			out = append(out, map[string]any{"object": o})
		}
		return out
	}
	run := func(name, msg string, related []any) string {
		tmpl, err := template.New(name).Funcs(funcs).Parse(msg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		status := map[string]any{}
		if related != nil {
			status["relatedObjects"] = related
		}
		var b strings.Builder
		if err := tmpl.Execute(&b, map[string]any{"DefaultMessage": "default", "Policy": map[string]any{"status": status}}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return b.String()
	}

	msgs := complianceMessages(t)
	fail, kubelet := result("a", "r", "FAIL", "high", ""), remediation("rem", "KubeletConfig", false, "NotApplied")
	// The controller leaves this skeleton when the live object is missing.
	missing := "kind: ComplianceRemediation\nmetadata: {name: typo}\n"
	for _, tt := range []struct {
		name, state string
		related     []any
		want        string
	}{
		{"findings-high", "noncompliant", objects(fail, result("b", "r", "FAIL", "high", "")), "2 findings: a, b"},
		{"findings-medium", "noncompliant", objects(fail), "1 findings: a"},
		{"findings-low", "noncompliant", nil, "default"},
		{"findings-high", "compliant", nil, "No findings"},
		{"findings-waived", "compliant", objects(fail), "1 waived: a=FAIL"},
		{"findings-waived", "compliant", nil, "0 waived"},
		{"remediate-remediation-state", "noncompliant", objects(kubelet), "default; states: rem=NotApplied "},
		{"remediate-remediation-state", "noncompliant", objects(missing), "default; states: typo=missing "},
	} {
		if got := run(tt.name, msgs[tt.name][tt.state], tt.related); got != tt.want {
			t.Errorf("%s %s: got %q, want %q", tt.name, tt.state, got, tt.want)
		}
	}
}

// complianceRender returns the rendered Policies and their ConfigurationPolicies, by name.
func complianceRender(t *testing.T, files []string) (policies, tmpls map[string]map[string]any) {
	t.Helper()
	policies, tmpls = map[string]map[string]any{}, map[string]map[string]any{}
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(helmTemplate(t, complianceDir, "compliance", files)), 4096)
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err == io.EOF {
			return policies, tmpls
		} else if err != nil {
			t.Fatal(err)
		}
		if doc["kind"] != "Policy" {
			continue
		}
		name := nestedString(doc, "metadata", "name")
		policies[name[strings.LastIndex(name, ".")+1:]] = doc
		pts, _, _ := unstructured.NestedSlice(doc, "spec", "policy-templates")
		for _, pt := range pts {
			if def, ok := pt.(map[string]any)["objectDefinition"].(map[string]any); ok {
				tmpls[nestedString(def, "metadata", "name")] = def
			}
		}
	}
}

func nestedString(o map[string]any, path ...string) string {
	v, _, _ := unstructured.NestedString(o, path...)
	return v
}

// complianceMessages returns the customMessage of every ConfigurationPolicy, by name then state.
func complianceMessages(t *testing.T) map[string]map[string]string {
	t.Helper()
	_, tmpls := complianceRender(t, allOn)
	out := map[string]map[string]string{}
	for name, def := range tmpls {
		if msgs, ok, _ := unstructured.NestedStringMap(def, "spec", "customMessage"); ok {
			out[name] = msgs
		}
	}
	return out
}

func TestComplianceValuesRejected(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not on PATH")
	}
	runner := render.New(helm)
	if err := runner.EnsureDeps(context.Background(), complianceDir); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, config, want string // want empty means the render succeeds
	}{
		{"empty reason", `waivers: {rule: ""}`, "needs a reason"},
		{"blank reason", `waivers: {rule: "  "}`, "needs a reason"},
		{"non-string reason", `waivers: {rule: true}`, "needs a reason"},
		{"result name as waiver", `waivers: {ocp4-stig-rule: "x"}`, "is a result name"},
		{"node result name as waiver", `waivers: {rhcos4-stig-master-rule: "x"}`, "is a result name"},
		{"null waiver", `waivers: {rule: null}`, ""},
		{"prefix only at the start", `waivers: {foo-ocp4-bar: "x"}`, ""},
		{"string remediation", `remediations: {a: "false"}`, "must be true or false"},
		{"sibling conflict", `remediations: {ocp4-stig-a: true, ocp4-stig-a-1: false}`, "conflicts"},
		{"cross benchmark conflict", `remediations: {ocp4-cis-foo: true, ocp4-cis-1-9-foo: false}`, "conflicts"},
		{"consistent siblings", `remediations: {ocp4-stig-a: true, ocp4-stig-a-1: true}`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := complianceFiles(t, "stack:\n  compliance:\n    config:\n      "+tt.config+"\n")
			tr := runner.Template(context.Background(), complianceDir, "compliance", files, nil)
			if tt.want == "" {
				if tr.Err != nil {
					t.Fatalf("render failed: %s", tr.Stderr)
				}
				return
			}
			if tr.Err == nil || !strings.Contains(string(tr.Stderr), tt.want) {
				t.Fatalf("want failure %q, got err %v: %s", tt.want, tr.Err, tr.Stderr)
			}
		})
	}
}

// TestComplianceStructure pins settings no template evaluation can see.
func TestComplianceStructure(t *testing.T) {
	policies, tmpls := complianceRender(t, allOn)

	// Under enforce a named mustnothave would delete results, and with them their remediations.
	for _, name := range []string{"findings-scan-state", "findings-high", "findings-medium", "findings-low", "findings-waived", "findings-drift", "remediate-remediation-state"} {
		def, ok := tmpls[name]
		if !ok {
			t.Fatalf("no template %s", name)
		}
		if got := nestedString(def, "spec", "remediationAction"); !strings.EqualFold(got, "InformOnly") {
			t.Errorf("%s: remediationAction %q", name, got)
		}
	}

	// Scan age changes with no event: without polling in both states, staleness never fires or clears.
	for state, want := range map[string]string{"compliant": "1h", "noncompliant": "10m"} {
		if got := nestedString(tmpls["findings-scan-state"], "spec", "evaluationInterval", state); got != want {
			t.Errorf("scan-state %s interval %q, want %q", state, got, want)
		}
	}
	// Those evaluations have no full objects, so a customMessage reading them would break.
	if _, ok, _ := unstructured.NestedMap(tmpls["findings-scan-state"], "spec", "customMessage"); ok {
		t.Error("scan-state must not set a customMessage")
	}

	// musthave would merge lists and keep a removed profile bound.
	found := false
	entries, _, _ := unstructured.NestedSlice(tmpls["scan-setting"], "spec", "object-templates")
	for _, e := range entries {
		entry := e.(map[string]any)
		if nestedString(entry, "objectDefinition", "kind") != "ScanSettingBinding" {
			continue
		}
		found = true
		if entry["complianceType"] != "mustonlyhave" || entry["metadataComplianceType"] != "musthave" {
			t.Errorf("binding: complianceType %v, metadataComplianceType %v", entry["complianceType"], entry["metadataComplianceType"])
		}
		if got := nestedString(entry, "objectDefinition", "metadata", "name"); got != "stig" {
			t.Errorf("binding %q; every check looks for suite stig", got)
		}
	}
	if !found {
		t.Error("no ScanSettingBinding in scan-setting")
	}

	for name, def := range policies {
		if !strings.HasPrefix(name, "findings-") && !strings.HasPrefix(name, "remediate-") {
			continue
		}
		if disabled, _, _ := unstructured.NestedBool(def, "spec", "disabled"); disabled {
			t.Errorf("%s is disabled", name)
		}
		deps, _ := json.Marshal(def["spec"].(map[string]any)["dependencies"])
		if !strings.Contains(string(deps), "scan-compliance") {
			t.Errorf("%s depends on %s, want scan", name, deps)
		}
		if strings.Contains(string(deps), "findings-compliance") {
			t.Errorf("%s must not wait on findings: failures would block it", name)
		}
	}

	// Applying remediations reboots nodes, so it stays opt-in.
	defaults, _ := complianceRender(t, []string{allOn[0], complianceFiles(t, "stack:\n  compliance:\n    enabled: true\n")[2]})
	names := strings.Join(slices.Sorted(maps.Keys(defaults)), " ")
	if !strings.Contains(names, "findings-compliance") || strings.Contains(names, "remediate-compliance") {
		t.Errorf("default Policies: %s", names)
	}
}
