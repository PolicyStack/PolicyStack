package acmtemplates

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stolostron/go-template-utils/v7/pkg/templates"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/cascade"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
)

const repoRoot = "../../../.."

// object is one resolved object-templates-raw entry.
type object struct {
	ComplianceType   string                    `json:"complianceType"`
	ObjectDefinition unstructured.Unstructured `json:"objectDefinition"`
}

// rawTemplate renders lifecycle/ for the fixture fleet file cluster, with setString ranking above
// the fleet file, and returns the object-templates-raw of ConfigurationPolicy name.
func rawTemplate(t *testing.T, cluster, name string, setString ...string) string {
	t.Helper()
	raw, ok := rawTemplates(t, cluster, setString...)[name]
	if !ok {
		t.Fatalf("no ConfigurationPolicy %q rendered for %s", name, cluster)
	}
	return raw
}

// rawTemplates is rawTemplate for every ConfigurationPolicy with object-templates-raw, by name.
func rawTemplates(t *testing.T, cluster string, setString ...string) map[string]string {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not on PATH")
	}
	clusters, err := fixtures.LoadDir(filepath.Join(repoRoot, "tools/validator/testdata/clusters"))
	if err != nil {
		t.Fatal(err)
	}
	var cl *fixtures.Cluster
	for _, c := range clusters {
		if c.Name == cluster && c.Hub != "" {
			cl = c
		}
	}
	if cl == nil {
		t.Fatalf("no hub-built fixture %q", cluster)
	}
	dir := filepath.Join(repoRoot, "lifecycle")
	res := cascade.Resolve(cl, dir, "lifecycle", repoRoot, filepath.Join(repoRoot, "values"))
	runner := render.New(helm)
	if err := runner.EnsureDeps(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	tr := runner.Template(context.Background(), dir, res.ReleaseName, append(res.ValueFiles, cl.SourceFile), setString)
	if tr.Err != nil {
		t.Fatalf("helm template: %v: %s", tr.Err, tr.Stderr)
	}
	out := map[string]string{}
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(tr.Stdout), 4096)
	for {
		var doc struct {
			Kind string `json:"kind"`
			Spec struct {
				PolicyTemplates []struct {
					ObjectDefinition struct {
						Metadata struct {
							Name string `json:"name"`
						} `json:"metadata"`
						Spec struct {
							Raw string `json:"object-templates-raw"`
						} `json:"spec"`
					} `json:"objectDefinition"`
				} `json:"policy-templates"`
			} `json:"spec"`
		}
		if err := dec.Decode(&doc); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		for _, pt := range doc.Spec.PolicyTemplates {
			if pt.ObjectDefinition.Spec.Raw != "" {
				out[pt.ObjectDefinition.Metadata.Name] = pt.ObjectDefinition.Spec.Raw
			}
		}
	}
	return out
}

// resolve evaluates raw with live objects (YAML documents) in the fake cluster. A nil result is
// what ACM reports Compliant for a raw template that renders nothing.
func resolve(t *testing.T, raw string, live ...string) []object {
	t.Helper()
	objs, err := tryResolve(t, raw, live...)
	if err != nil {
		t.Fatalf("resolve: %v\n%s", err, raw)
	}
	return objs
}

// tryResolve is resolve, returning the error the config-policy-controller would report.
func tryResolve(t *testing.T, raw string, live ...string) ([]object, error) {
	t.Helper()
	var objs []runtime.Object
	for _, l := range live {
		u := &unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(l), &u.Object); err != nil {
			t.Fatalf("live object: %v", err)
		}
		objs = append(objs, u)
	}
	r, err := NewResolver(templates.Config{}, objs...)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.ResolveTemplate([]byte(raw), nil, &templates.ResolveOptions{InputIsYAML: true})
	if err != nil {
		return nil, err
	}
	var out []object
	if err := json.Unmarshal(res.ResolvedJSON, &out); err != nil {
		t.Fatalf("resolved output: %v: %s", err, res.ResolvedJSON)
	}
	return out, nil
}

// find returns the resolved object of kind and name, failing the test when absent.
func find(t *testing.T, objs []object, kind, name string) object {
	t.Helper()
	for _, o := range objs {
		if o.ObjectDefinition.GetKind() == kind && o.ObjectDefinition.GetName() == name {
			return o
		}
	}
	t.Fatalf("no %s %s in %+v", kind, name, objs)
	return object{}
}

// nested reads the value at path in o's objectDefinition, nil when absent.
func nested(o object, path ...string) any {
	v, _, _ := unstructured.NestedFieldNoCopy(o.ObjectDefinition.Object, path...)
	return v
}

// field reads a string at path in o's objectDefinition.
func field(o object, path ...string) string {
	v, _, _ := unstructured.NestedString(o.ObjectDefinition.Object, path...)
	return v
}
