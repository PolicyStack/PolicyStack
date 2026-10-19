package acmtemplates

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stolostron/go-template-utils/v7/pkg/templates"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// objectDefinition renders the element in dir with the all-on profile and returns the
// object-templates entry for kind/name as the JSON the config-policy-controller resolves.
func objectDefinition(t *testing.T, dir, kind, name string) []byte {
	t.Helper()
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(helmTemplate(t, dir, filepath.Base(dir), allOn)), 4096)
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
				var o unstructured.Unstructured
				if json.Unmarshal(ot.ObjectDefinition, &o.Object) == nil && o.GetKind() == kind && o.GetName() == name {
					return ot.ObjectDefinition
				}
			}
		}
	}
	t.Fatalf("no object template %s %s rendered from %s", kind, name, dir)
	return nil
}

// ingressCert is the ingress operator's default-ingress-cert: the router's serving chain.
const ingressCert = `apiVersion: v1
kind: ConfigMap
metadata: {name: default-ingress-cert, namespace: openshift-config-managed}
data:
  ca-bundle.crt: |
    -----BEGIN CERTIFICATE-----
    LEAF+/=
    -----END CERTIFICATE-----
    -----BEGIN CERTIFICATE-----
    INTERMEDIATE
    -----END CERTIFICATE-----
`

// TestQuayIngressCA: the bootstrap Job's ingress CA is default-ingress-cert's data verbatim, and a
// missing default-ingress-cert fails the template instead of writing an empty bundle.
func TestQuayIngressCA(t *testing.T) {
	od := objectDefinition(t, filepath.Join(repoRoot, "stack/quay"), "ConfigMap", "quay-ingress-ca")
	live := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(ingressCert), &live.Object); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		live []runtime.Object
	}{{"present", []runtime.Object{live}}, {"absent", nil}} {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewResolver(templates.Config{}, tt.live...)
			if err != nil {
				t.Fatal(err)
			}
			res, err := r.ResolveTemplate(od, nil, &templates.ResolveOptions{})
			if tt.live == nil {
				if !apierrors.IsNotFound(err) {
					t.Errorf("resolve without default-ingress-cert: %v, want NotFound", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got unstructured.Unstructured
			if err := json.Unmarshal(res.ResolvedJSON, &got.Object); err != nil {
				t.Fatal(err)
			}
			if want := live.Object["data"]; !reflect.DeepEqual(got.Object["data"], want) {
				t.Errorf("data %#v, want %#v", got.Object["data"], want)
			}
		})
	}
}
