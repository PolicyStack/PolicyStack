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
	ComplianceType         string                    `json:"complianceType"`
	MetadataComplianceType string                    `json:"metadataComplianceType"`
	ObjectDefinition       unstructured.Unstructured `json:"objectDefinition"`
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
	release, files := lifecycleFiles(t, cluster)
	return renderRaw(t, filepath.Join(repoRoot, "lifecycle"), release, files, setString...)
}

// lifecycleFiles returns the release name and value files of the fixture's lifecycle release.
func lifecycleFiles(t *testing.T, cluster string) (string, []string) {
	t.Helper()
	clusters, err := fixtures.LoadDir(filepath.Join(repoRoot, "tools/validator/testdata/clusters"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range clusters {
		if c.Name == cluster && c.Hub != "" {
			res := cascade.Resolve(c, filepath.Join(repoRoot, "lifecycle"), "lifecycle", repoRoot, filepath.Join(repoRoot, "values"))
			return res.ReleaseName, append(res.ValueFiles, c.SourceFile)
		}
	}
	t.Fatalf("no hub-built fixture %q", cluster)
	return "", nil
}

// allOn is the value files that turn every element and toggle on.
var allOn = []string{
	filepath.Join(repoRoot, "tools/validator/testdata/baseline.yaml"),
	filepath.Join(repoRoot, "tools/validator/testdata/profiles/all-on.yaml"),
}

// helmTemplate runs helm template on the chart in dir and returns its output.
func helmTemplate(t *testing.T, dir, release string, valueFiles []string, setString ...string) []byte {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not on PATH")
	}
	runner := render.New(helm)
	if err := runner.EnsureDeps(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	tr := runner.Template(context.Background(), dir, release, valueFiles, setString)
	if tr.Err != nil {
		t.Fatalf("helm template: %v: %s", tr.Err, tr.Stderr)
	}
	return tr.Stdout
}

// renderRaw runs helm template on the chart in dir and returns the object-templates-raw of every
// ConfigurationPolicy, by name.
func renderRaw(t *testing.T, dir, release string, valueFiles []string, setString ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(helmTemplate(t, dir, release, valueFiles, setString...)), 4096)
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

// customMessages runs helm template on the chart in dir and returns the noncompliant customMessage
// of every ConfigurationPolicy that sets one, by name.
func customMessages(t *testing.T, dir, release string, valueFiles []string, setString ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(helmTemplate(t, dir, release, valueFiles, setString...)), 4096)
	for {
		var p policy
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		for _, pt := range p.Spec.PolicyTemplates {
			var cp struct {
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
				Spec struct {
					CustomMessage struct {
						NonCompliant string `json:"noncompliant"`
					} `json:"customMessage"`
				} `json:"spec"`
			}
			if json.Unmarshal(pt.ObjectDefinition, &cp) == nil && cp.Spec.CustomMessage.NonCompliant != "" {
				out[cp.Metadata.Name] = cp.Spec.CustomMessage.NonCompliant
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
