package acmtemplates

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stolostron/go-template-utils/v7/pkg/templates"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

// acsTemplates renders stack/acs with the all-on profile and returns every object-template
// definition keyed by ConfigurationPolicy name, in order.
func acsTemplates(t *testing.T) map[string][]json.RawMessage {
	t.Helper()
	out := map[string][]json.RawMessage{}
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(helmTemplate(t, filepath.Join(repoRoot, "stack/acs"), "acs", allOn)), 4096)
	for {
		var p policy
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		for _, pt := range p.Spec.PolicyTemplates {
			var cp configPolicy
			if json.Unmarshal(pt.ObjectDefinition, &cp) != nil {
				continue
			}
			for _, ot := range cp.Spec.ObjectTemplates {
				out[cp.Metadata.Name] = append(out[cp.Metadata.Name], ot.ObjectDefinition)
			}
		}
	}
	return out
}

// acsObject returns the only object template of kind in configPolicy cp.
func acsObject(t *testing.T, cp, kind string) json.RawMessage {
	t.Helper()
	var found []json.RawMessage
	for _, od := range acsTemplates(t)[cp] {
		var o unstructured.Unstructured
		if json.Unmarshal(od, &o.Object) == nil && o.GetKind() == kind {
			found = append(found, od)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s: %d %s templates, want 1", cp, len(found), kind)
	}
	return found[0]
}

func obj(t *testing.T, s string) *unstructured.Unstructured {
	t.Helper()
	u := &unstructured.Unstructured{}
	if err := json.Unmarshal([]byte(s), &u.Object); err != nil {
		t.Fatal(err)
	}
	return u
}

// resolveHub resolves od as the propagator would for a cluster with labels.
func resolveHub(t *testing.T, od []byte, labels map[string]string, live ...runtime.Object) ([]byte, error) {
	t.Helper()
	r, err := NewResolver(templates.Config{DisabledFunctions: []string{}, StartDelim: "{{hub", StopDelim: "hub}}"}, live...)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.ResolveTemplate(od, hubContext{ManagedClusterName: "spoke1", ManagedClusterLabels: labels}, &templates.ResolveOptions{
		EncryptionConfig: templates.EncryptionConfig{EncryptionEnabled: true, AESKey: aesKey, InitializationVector: aesIV}})
	if err != nil {
		return nil, err
	}
	return res.ResolvedJSON, nil
}

// resolveManaged resolves od as the config-policy-controller would; skipped reports skipObject.
func resolveManaged(t *testing.T, od []byte, ctx any) (u *unstructured.Unstructured, skipped bool) {
	t.Helper()
	r, err := NewResolver(templates.Config{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.ResolveTemplate(od, ctx, &templates.ResolveOptions{CustomFunctions: map[string]any{
		"skipObject": func(skip ...bool) string { skipped = len(skip) == 0 || skip[0]; return "" }}})
	if err != nil {
		t.Fatal(err)
	}
	u = &unstructured.Unstructured{}
	if err := json.Unmarshal(res.ResolvedJSON, &u.Object); err != nil {
		t.Fatal(err)
	}
	return u, skipped
}

const (
	acsSource = `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"cluster-registration-secret","namespace":"stackrox"},"data":{"crs":"Q1JT"}}`
	acsRoute  = `{"apiVersion":"route.openshift.io/v1","kind":"Route","metadata":{"name":"central","namespace":"stackrox"},"spec":{"host":"central-stackrox.apps.hub.example"}}`
)

// TestACSRegistrationCopy: the tls policy copies only the hub's CRS, encrypted, to every cluster
// but the hub, and fails closed without it.
func TestACSRegistrationCopy(t *testing.T) {
	var secrets []string
	for _, od := range acsTemplates(t)["secured-tls"] {
		var o unstructured.Unstructured
		if json.Unmarshal(od, &o.Object) == nil && o.GetKind() == "Secret" {
			secrets = append(secrets, o.GetName())
		}
	}
	if len(secrets) != 1 || !strings.HasSuffix(secrets[0], "}}cluster-registration-secret") {
		t.Fatalf("tls Secrets %v, want one cluster-registration-secret", secrets)
	}
	od := acsObject(t, "secured-tls", "Secret")
	for _, tt := range []struct {
		name   string
		labels map[string]string
		skip   bool
	}{
		{"spoke", map[string]string{"name": "spoke1"}, false},
		{"hub", map[string]string{"name": "local-cluster", "local-cluster": "true"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hub, err := resolveHub(t, od, tt.labels, obj(t, acsSource))
			if err != nil {
				t.Fatal(err)
			}
			got, skipped := resolveManaged(t, hub, nil)
			if skipped != tt.skip {
				t.Errorf("skipped = %v, want %v", skipped, tt.skip)
			}
			if got.GetName() != "cluster-registration-secret" {
				t.Errorf("name %q", got.GetName())
			}
			if crs, _, _ := unstructured.NestedString(got.Object, "data", "crs"); !strings.HasPrefix(crs, "$ocm_encrypted:") {
				t.Errorf("data.crs %q, want $ocm_encrypted:", crs)
			}
		})
	}
	if _, err := resolveHub(t, od, nil); !apierrors.IsNotFound(err) {
		t.Errorf("without source: %v, want NotFound", err)
	}
}

// TestACSEndpoint: centralEndpoint is the hub Route host, and a missing Route is a hub error, never
// an empty or <no value> endpoint.
func TestACSEndpoint(t *testing.T) {
	od := acsObject(t, "secured-agents", "SecuredCluster")
	got, err := resolveHub(t, od, nil, obj(t, acsRoute))
	if err != nil {
		t.Fatal(err)
	}
	var sc unstructured.Unstructured
	if err := json.Unmarshal(got, &sc.Object); err != nil {
		t.Fatal(err)
	}
	if ep, _, _ := unstructured.NestedString(sc.Object, "spec", "centralEndpoint"); ep != "central-stackrox.apps.hub.example:443" {
		t.Errorf("centralEndpoint %q", ep)
	}
	if _, err := resolveHub(t, od, nil); err == nil || apierrors.IsNotFound(err) {
		t.Errorf("without Route: %v, want a non-NotFound hub error", err)
	}
}

// TestACSCRSExpiry: crs-ready matches the live annotation only while over 90 days remain.
func TestACSCRSExpiry(t *testing.T) {
	od := acsObject(t, "central-crs-ready", "Secret")
	const key = "crs.platform.stackrox.io/expires-at"
	in := func(d time.Duration) string { return time.Now().Add(d).UTC().Format(time.RFC3339Nano) }
	far := in(300 * 24 * time.Hour)
	for _, tt := range []struct {
		name, want string
		object     map[string]any
	}{
		{"valid", far, map[string]any{"metadata": map[string]any{"annotations": map[string]any{key: far}}}},
		{"expiring", "renew", map[string]any{"metadata": map[string]any{"annotations": map[string]any{key: in(30 * 24 * time.Hour)}}}},
		{"expired", "renew", map[string]any{"metadata": map[string]any{"annotations": map[string]any{key: in(-time.Hour)}}}},
		{"no annotation", "renew", map[string]any{"metadata": map[string]any{}}},
		{"absent", "renew", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := struct {
				Object                      map[string]any
				ObjectNamespace, ObjectName string
			}{tt.object, "stackrox", "cluster-registration-secret"}
			got, _ := resolveManaged(t, od, ctx)
			if a := got.GetAnnotations()[key]; a != tt.want {
				t.Errorf("%s = %q, want %q", key, a, tt.want)
			}
		})
	}
}

// TestACSNoLegacy: nothing rendered still references the init bundle or a minting workload.
func TestACSNoLegacy(t *testing.T) {
	out := string(helmTemplate(t, filepath.Join(repoRoot, "stack/acs"), "acs", allOn))
	for _, s := range []string{"bootstrapImage", "sensor-tls", "init-bundle", "kind: CronJob", "kind: Job"} {
		if strings.Contains(out, s) {
			t.Errorf("render contains %q", s)
		}
	}
}
