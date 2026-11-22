package compat

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/acmtemplates"
)

// clusterNS is the cluster's namespace, where its ConfigurationPolicies live.
const clusterNS = "compat"

var configPolicies = schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "configurationpolicies"}

// unwatched change by themselves or are the test's own.
var unwatched = []string{"events", "leases", "endpoints", "endpointslices", "apiservices", "componentstatuses", "customresourcedefinitions", "configurationpolicies", "operatorpolicies"}

// TestUpgrade applies, for each change the compatibility check found, the release's
// ConfigurationPolicies on the real config-policy-controller, then the change's on top, and fails
// when an object or a compliance state moves. `make upgrade-test` sets its environment.
//
// OperatorPolicies (no OLM) and ConfigurationPolicies with hub templates (no propagator) are not
// exercised. Kinds without a CRD in testenv get one without a schema.
func TestUpgrade(t *testing.T) {
	file, controller := os.Getenv("UPGRADE_CHANGES"), os.Getenv("UPGRADE_CONTROLLER")
	if file == "" {
		t.Skip("run make upgrade-test")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range report.Changes {
		if strings.HasPrefix(c.Path, "install/") || failed(c.After) {
			continue // not Policies, or nothing to apply, which the render check already reports
		}
		// Fixtures mostly render a chart alike but for the cluster's name.
		cluster := path.Base(path.Dir(c.Path))
		key := path.Base(c.Path) + strings.ReplaceAll(string(c.Release)+string(c.After), cluster, "<cluster>")
		if seen[key] {
			continue
		}
		seen[key] = true
		t.Run(strings.TrimSuffix(c.Path, ".yaml"), func(t *testing.T) {
			t.Parallel()
			moved := upgrade(t, controller, cluster, c)
			for _, m := range moved {
				if report.Approved {
					t.Log("approved: " + m)
				} else {
					t.Error("moved: " + m)
				}
			}
		})
	}
}

type cluster struct {
	dyn    dynamic.Interface
	disc   discovery.DiscoveryInterface
	mapper *restmapper.DeferredDiscoveryRESTMapper
}

// upgrade returns what moved from the release's render to the change's.
func upgrade(t *testing.T, controller, name string, change Change) []string {
	t.Helper()
	testdata, err := filepath.Abs("../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	seedData, err := os.ReadFile(filepath.Join(testdata, "compat", "seed.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	seeds := documents(t, seedData)
	before, after := templatesOf(t, change.Release), templatesOf(t, change.After)

	env := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join(testdata, "..", "crds")}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Error(err)
		}
	})
	cfg.QPS, cfg.Burst = 200, 400

	var c cluster
	if c.dyn, err = dynamic.NewForConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if c.disc, err = discovery.NewDiscoveryClientForConfig(cfg); err != nil {
		t.Fatal(err)
	}
	c.mapper = restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(c.disc))

	objects := slices.Clone(seeds)
	for _, rendered := range [][]byte{change.Release, change.After} {
		objects = append(objects, resolved(t, rendered, name)...)
	}
	if _, err := envtest.InstallCRDs(cfg, envtest.CRDInstallOptions{CRDs: c.missingCRDs(t, objects)}); err != nil {
		t.Fatal(err)
	}
	c.mapper.Reset()
	namespaces := map[string]bool{clusterNS: true}
	for _, o := range objects {
		if ns := o.GetNamespace(); ns != "" && !strings.Contains(ns, "{{") {
			namespaces[ns] = true
		}
	}
	for _, ns := range slices.Sorted(maps.Keys(namespaces)) {
		o := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns}}}
		if err := c.create(t.Context(), o); err != nil {
			t.Fatal(err)
		}
	}
	for _, o := range seeds {
		if err := c.create(t.Context(), o); err != nil {
			t.Fatal(err)
		}
	}

	start(t, env, controller)
	c.apply(t, before)
	objectsBefore, statesBefore := c.settle(t)
	c.apply(t, after)
	objectsAfter, statesAfter := c.settle(t)

	var moved []string
	all := maps.Clone(objectsBefore)
	maps.Copy(all, objectsAfter)
	for _, key := range slices.Sorted(maps.Keys(all)) {
		if b, a := objectsBefore[key], objectsAfter[key]; b != a {
			moved = append(moved, fmt.Sprintf("%s\n  release: %s\n  change:  %s", key, cmp.Or(b, "absent"), cmp.Or(a, "absent")))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(statesAfter)) {
		if was, ok := statesBefore[key]; ok && was != statesAfter[key] {
			moved = append(moved, fmt.Sprintf("ConfigurationPolicy %s: %s, now %s", key, was, statesAfter[key]))
		}
	}
	return moved
}

// start runs the config-policy-controller against env until the test ends. Not in hosted mode
// (--target-kubeconfig-path): its second manager listens on :8080 on every interface.
func start(t *testing.T, env *envtest.Environment, binary string) {
	t.Helper()
	user, err := env.AddUser(envtest.User{Name: "controller", Groups: []string{"system:masters"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := user.KubeConfig()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	kubeconfig := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(kubeconfig, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, "controller", "--kubeconfig="+kubeconfig,
		"--leader-elect=false", "--cluster-name="+clusterNS, "--evaluation-backoff=1", "--enable-ocm-policy-namespace=false",
		"--health-probe-bind-address=127.0.0.1:0", "--metrics-bind-address=127.0.0.1:0")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig, "WATCH_NAMESPACE="+clusterNS, "POD_NAMESPACE="+clusterNS, "OSDK_FORCE_RUN_MODE=local")
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
		if t.Failed() {
			b := log.Bytes()
			t.Logf("controller log, its end:\n%s", b[max(0, len(b)-4000):])
		}
	})
}

// templatesOf returns the ConfigurationPolicies in a render as the framework add-on creates them:
// the Policy's action, unless the template's is InformOnly.
func templatesOf(t *testing.T, rendered []byte) []*unstructured.Unstructured {
	t.Helper()
	var out []*unstructured.Unstructured
	for _, p := range documents(t, rendered) {
		if p.GetKind() != "Policy" {
			continue
		}
		action, _, _ := unstructured.NestedString(p.Object, "spec", "remediationAction")
		templates, _, err := unstructured.NestedSlice(p.Object, "spec", "policy-templates")
		if err != nil {
			t.Fatal(err)
		}
		for _, pt := range templates {
			m, _ := pt.(map[string]any)
			od, _ := m["objectDefinition"].(map[string]any)
			o := &unstructured.Unstructured{Object: od}
			if o.GetKind() != "ConfigurationPolicy" {
				continue
			}
			data, err := json.Marshal(od)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte("{{hub")) {
				t.Logf("%s has hub templates and is not exercised", o.GetName())
				continue
			}
			effective := action
			if own, _, _ := unstructured.NestedString(od, "spec", "remediationAction"); strings.EqualFold(own, "informonly") {
				effective = "inform"
			}
			o.SetNamespace(clusterNS)
			if err := errors.Join(
				unstructured.SetNestedField(od, cmp.Or(effective, "inform"), "spec", "remediationAction"),
				unstructured.SetNestedStringMap(od, map[string]string{"compliant": "watch", "noncompliant": "watch"}, "spec", "evaluationInterval"),
			); err != nil {
				t.Fatal(err)
			}
			out = append(out, o)
		}
	}
	return out
}

func documents(t *testing.T, data []byte) []*unstructured.Unstructured {
	t.Helper()
	var out []*unstructured.Unstructured
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); errors.Is(err, io.EOF) {
			return out
		} else if err != nil {
			t.Fatal(err)
		}
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		o := &unstructured.Unstructured{}
		if err := o.UnmarshalJSON(raw); err != nil {
			t.Fatal(err)
		}
		out = append(out, o)
	}
}

// resolved returns the objects a render's templates name, for their CRDs and namespaces.
func resolved(t *testing.T, rendered []byte, name string) []*unstructured.Unstructured {
	t.Helper()
	if len(bytes.TrimSpace(rendered)) == 0 {
		return nil
	}
	found, _, err := acmtemplates.Resolve(rendered, name)
	if err != nil {
		t.Fatal(err)
	}
	var out []*unstructured.Unstructured
	for _, o := range found {
		var m map[string]any
		if o.Definition != nil && json.Unmarshal(o.Definition, &m) == nil {
			out = append(out, &unstructured.Unstructured{Object: m})
		}
	}
	return out
}

// missingCRDs returns a schemaless CRD for each kind of objects, and each kind the validator's fake
// cluster serves, that the API server lacks.
func (c cluster) missingCRDs(t *testing.T, objects []*unstructured.Unstructured) []*apiextv1.CustomResourceDefinition {
	t.Helper()
	_, served, err := c.disc.ServerGroupsAndResources()
	if err != nil {
		t.Fatal(err)
	}
	have := map[schema.GroupKind]bool{}
	for _, list := range served {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range list.APIResources {
			have[schema.GroupKind{Group: gv.Group, Kind: r.Kind}] = true
		}
	}
	crds := map[schema.GroupKind]*apiextv1.CustomResourceDefinition{}
	add := func(apiVersion, kind, plural string, namespaced bool) {
		gv, err := schema.ParseGroupVersion(apiVersion)
		gk := schema.GroupKind{Group: gv.Group, Kind: kind}
		if err != nil || kind == "" || !strings.Contains(gv.Group, ".") || strings.Contains(apiVersion+kind, "{{") || have[gk] {
			return
		}
		version := apiextv1.CustomResourceDefinitionVersion{Name: gv.Version, Served: true,
			Schema: &apiextv1.CustomResourceValidation{OpenAPIV3Schema: &apiextv1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: new(true)}}}
		if crd := crds[gk]; crd != nil {
			if !slices.ContainsFunc(crd.Spec.Versions, func(v apiextv1.CustomResourceDefinitionVersion) bool { return v.Name == gv.Version }) {
				crd.Spec.Versions = append(crd.Spec.Versions, version)
			}
			return
		}
		version.Storage = true
		scope := apiextv1.ClusterScoped
		if namespaced {
			scope = apiextv1.NamespaceScoped
		}
		crds[gk] = &apiextv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: plural + "." + gv.Group, Annotations: map[string]string{"api-approved.kubernetes.io": "unapproved, test only"}},
			Spec: apiextv1.CustomResourceDefinitionSpec{Group: gv.Group, Scope: scope, Versions: []apiextv1.CustomResourceDefinitionVersion{version},
				Names: apiextv1.CustomResourceDefinitionNames{Kind: kind, ListKind: kind + "List", Plural: plural, Singular: strings.ToLower(kind)}},
		}
	}
	acmtemplates.Kinds(add)
	for _, o := range objects {
		add(o.GetAPIVersion(), o.GetKind(), strings.ToLower(o.GetKind())+"s", o.GetNamespace() != "")
	}
	return slices.Collect(maps.Values(crds))
}

func (c cluster) create(ctx context.Context, o *unstructured.Unstructured) error {
	gvk := o.GroupVersionKind()
	m, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}
	r := c.dyn.Resource(m.Resource)
	if o.GetNamespace() != "" {
		_, err = r.Namespace(o.GetNamespace()).Create(ctx, o.DeepCopy(), metav1.CreateOptions{})
	} else {
		_, err = r.Create(ctx, o.DeepCopy(), metav1.CreateOptions{})
	}
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// apply makes want the cluster's ConfigurationPolicies: in place, as template sync updates them.
func (c cluster) apply(t *testing.T, want []*unstructured.Unstructured) {
	t.Helper()
	r := c.dyn.Resource(configPolicies).Namespace(clusterNS)
	keep := map[string]bool{}
	for _, o := range want {
		keep[o.GetName()] = true
		// The controller writes status and finalizers, so retry a conflict.
		for attempt := 0; ; attempt++ {
			live, err := r.Get(t.Context(), o.GetName(), metav1.GetOptions{})
			switch {
			case apierrors.IsNotFound(err):
				_, err = r.Create(t.Context(), o.DeepCopy(), metav1.CreateOptions{})
			case err == nil && reflect.DeepEqual(live.Object["spec"], o.Object["spec"]):
			case err == nil:
				live.Object["spec"] = o.DeepCopy().Object["spec"]
				_, err = r.Update(t.Context(), live, metav1.UpdateOptions{})
			}
			if err == nil {
				break
			}
			if !apierrors.IsConflict(err) || attempt == 5 {
				t.Fatalf("ConfigurationPolicy %s: %v", o.GetName(), err)
			}
		}
	}
	list, err := r.List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range list.Items {
		if !keep[o.GetName()] {
			if err := r.Delete(t.Context(), o.GetName(), metav1.DeleteOptions{}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// settle waits until every ConfigurationPolicy has evaluated its spec and nothing has changed for
// five seconds, and returns the cluster's objects and the compliance states. A cluster that never
// settles fails: two templates are rewriting each other.
func (c cluster) settle(t *testing.T) (objects, states map[string]string) {
	t.Helper()
	const quiet = 5
	same, restless, pending := 0, map[string]bool{}, []string{}
	for range 90 {
		time.Sleep(time.Second)
		list, err := c.dyn.Resource(configPolicies).Namespace(clusterNS).List(t.Context(), metav1.ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		now := map[string]string{}
		pending = pending[:0]
		for _, o := range list.Items {
			gen, found, _ := unstructured.NestedInt64(o.Object, "status", "lastEvaluatedGeneration")
			if !found || gen != o.GetGeneration() {
				pending = append(pending, fmt.Sprintf("%s generation %d, evaluated %d", o.GetName(), o.GetGeneration(), gen))
			}
			now[o.GetName()], _, _ = unstructured.NestedString(o.Object, "status", "compliant")
		}
		snap := c.snapshot(t)
		for k, v := range snap {
			if objects != nil && objects[k] != v {
				restless[k] = true
			}
		}
		if len(pending) == 0 && maps.Equal(snap, objects) && maps.Equal(now, states) {
			if same++; same == quiet {
				return objects, states
			}
			continue
		}
		same, objects, states = 0, snap, now
	}
	t.Fatalf("did not settle in 90 seconds: not evaluated %v, objects that changed %v", pending, slices.Sorted(maps.Keys(restless)))
	return nil, nil
}

// snapshot is every watched object on the cluster as JSON, without what every write changes.
func (c cluster) snapshot(t *testing.T) map[string]string {
	t.Helper()
	lists, err := c.disc.ServerPreferredResources()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, l := range lists {
		gv, err := schema.ParseGroupVersion(l.GroupVersion)
		if err != nil {
			t.Fatal(err)
		}
		if gv.Group == "flowcontrol.apiserver.k8s.io" {
			continue
		}
		for _, r := range l.APIResources {
			if slices.Contains(unwatched, r.Name) || !slices.Contains(r.Verbs, "list") {
				continue
			}
			list, err := c.dyn.Resource(gv.WithResource(r.Name)).List(t.Context(), metav1.ListOptions{})
			if err != nil {
				t.Fatalf("list %s: %v", r.Name, err)
			}
			for _, o := range list.Items {
				o.SetManagedFields(nil)
				o.SetResourceVersion("")
				o.SetGeneration(0)
				data, err := o.MarshalJSON()
				if err != nil {
					t.Fatal(err)
				}
				out[strings.TrimPrefix(gv.Group+"/", "/")+r.Name+" "+strings.TrimPrefix(o.GetNamespace()+"/"+o.GetName(), "/")] = string(data)
			}
		}
	}
	return out
}
