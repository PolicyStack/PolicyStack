// Package acmtemplates evaluates the ACM templates a render escapes past Helm the way ACM does:
// hub templates as the governance-policy-propagator resolves them, then managed templates as the
// config-policy-controller does, against a fake cluster. helm template and kubeconform see these
// templates only as strings.
package acmtemplates

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/stolostron/go-template-utils/v7/pkg/templates"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/klog"
)

func init() {
	// go-template-utils logs every failed template in full through klog v1.
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	_ = fs.Set("logtostderr", "false")
	_ = fs.Set("stderrthreshold", "FATAL")
	klog.SetOutput(io.Discard)
}

// kinds the templates look up, served by the fake discovery client. A lookup of any other kind
// fails with ErrMissingAPIResource, which is how a misspelled kind or apiVersion surfaces.
var kinds = []struct {
	gv, kind, plural string
	namespaced       bool
}{
	{"v1", "ConfigMap", "configmaps", true},
	{"v1", "Node", "nodes", false},
	{"v1", "Secret", "secrets", true},
	{"apps/v1", "StatefulSet", "statefulsets", true},
	{"cert-manager.io/v1", "Certificate", "certificates", true},
	{"cluster.open-cluster-management.io/v1", "ManagedCluster", "managedclusters", false},
	{"config.openshift.io/v1", "ClusterVersion", "clusterversions", false},
	{"config.openshift.io/v1", "DNS", "dnses", false},
	{"config.openshift.io/v1", "Infrastructure", "infrastructures", false},
	{"config.openshift.io/v1", "Ingress", "ingresses", false},
	{"hive.openshift.io/v1", "ClusterDeployment", "clusterdeployments", true},
	{"hive.openshift.io/v1", "ClusterImageSet", "clusterimagesets", false},
	{"hypershift.openshift.io/v1beta1", "HostedCluster", "hostedclusters", true},
	{"hypershift.openshift.io/v1beta1", "NodePool", "nodepools", true},
	{"imageregistry.operator.openshift.io/v1", "Config", "configs", false},
	{"machine.openshift.io/v1beta1", "MachineSet", "machinesets", true},
	{"operator.openshift.io/v1", "IngressController", "ingresscontrollers", true},
	{"pipelines.openshift.io/v1alpha1", "GitopsService", "gitopsservices", false},
	{"route.openshift.io/v1", "Route", "routes", true},
	{"siteconfig.open-cluster-management.io/v1alpha1", "ClusterInstance", "clusterinstances", true},
	{"storage.k8s.io/v1", "StorageClass", "storageclasses", false},
}

// NewResolver returns a resolver over a fake cluster holding objs.
func NewResolver(cfg templates.Config, objs ...runtime.Object) (*templates.TemplateResolver, error) {
	lists := map[schema.GroupVersionResource]string{}
	byGV := map[string]*metav1.APIResourceList{}
	var resources []*metav1.APIResourceList
	for _, k := range kinds {
		gv, _ := schema.ParseGroupVersion(k.gv)
		lists[gv.WithResource(k.plural)] = k.kind + "List"
		if byGV[k.gv] == nil {
			byGV[k.gv] = &metav1.APIResourceList{GroupVersion: k.gv}
			resources = append(resources, byGV[k.gv])
		}
		byGV[k.gv].APIResources = append(byGV[k.gv].APIResources,
			metav1.APIResource{Name: k.plural, Kind: k.kind, Namespaced: k.namespaced, Verbs: []string{"get", "list"}})
	}
	dyn := fakedynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), lists, objs...)
	disc := &fakediscovery.FakeDiscovery{Fake: &clienttesting.Fake{Resources: resources}}
	return templates.NewResolverWithClients(dyn, disc, cfg)
}

// Failure is a template ACM would fail to resolve, or whose output it would enforce broken.
type Failure struct {
	ConfigPolicy string // metadata.name of the ConfigurationPolicy
	// Template indexes object-templates, or is -1 for object-templates-raw and the hub pass.
	Template int
	Hub      bool
	Err      string
}

// Stub key material: the propagator encrypts fromSecret output with a per-cluster AES key.
var (
	aesKey = make([]byte, 32)
	aesIV  = make([]byte, 16)
)

// hubContext is the propagator's template context.
type hubContext struct {
	ManagedClusterName   string
	ManagedClusterLabels map[string]string
	PolicyMetadata       map[string]any
}

type policy struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name        string            `json:"name"`
		Namespace   string            `json:"namespace"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		HubTemplateOptions struct {
			ServiceAccountName string `json:"serviceAccountName"`
		} `json:"hubTemplateOptions"`
		PolicyTemplates []struct {
			ObjectDefinition json.RawMessage `json:"objectDefinition"`
		} `json:"policy-templates"`
	} `json:"spec"`
}

type configPolicy struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name        string            `json:"name"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		NamespaceSelector json.RawMessage `json:"namespaceSelector"`
		Raw               string          `json:"object-templates-raw"`
		ObjectTemplates   []struct {
			ObjectSelector   json.RawMessage `json:"objectSelector"`
			ObjectDefinition json.RawMessage `json:"objectDefinition"`
		} `json:"object-templates"`
	} `json:"spec"`
}

const disableTemplates = "policy.open-cluster-management.io/disable-templates"

// hubTemplate matches a hub template, which a missing hub object leaves unresolved.
var hubTemplate = regexp.MustCompile(`(?s)\{\{hub.*?hub\}\}`)

// Evaluate resolves every templated ConfigurationPolicy in rendered, the Policies Helm rendered for
// cluster, against an empty fake cluster. A missing object is not a failure: lookups of objects that
// other policies or operators create are normal on a real cluster, so evaluation stops there.
func Evaluate(rendered []byte, cluster string) ([]Failure, error) {
	hub, err := NewResolver(templates.Config{AdditionalIndentation: 8, DisabledFunctions: []string{}, StartDelim: "{{hub", StopDelim: "hub}}"})
	if err != nil {
		return nil, err
	}
	managed, err := NewResolver(templates.Config{})
	if err != nil {
		return nil, err
	}
	var out []Failure
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	for {
		var p policy
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if disabled, _ := strconv.ParseBool(p.Metadata.Annotations[disableTemplates]); p.Kind != "Policy" || disabled {
			continue
		}
		for _, pt := range p.Spec.PolicyTemplates {
			od := []byte(pt.ObjectDefinition)
			var cp configPolicy
			if json.Unmarshal(od, &cp) != nil || cp.Kind != "ConfigurationPolicy" {
				continue
			}
			// The annotation stops only the config-policy-controller, the propagator ignores it.
			managedDisabled, _ := strconv.ParseBool(cp.Metadata.Annotations[disableTemplates])
			fail := func(i int, hub bool, err string) {
				out = append(out, Failure{ConfigPolicy: cp.Metadata.Name, Template: i, Hub: hub, Err: err})
			}

			// The propagator, per policy-template, with lookups restricted to the policy namespace
			// unless a ServiceAccount resolves them.
			var managedOpts templates.ResolveOptions
			if templates.HasTemplate(od, "{{hub", false) {
				opts := &templates.ResolveOptions{}
				if p.Spec.HubTemplateOptions.ServiceAccountName == "" {
					opts.LookupNamespace = p.Metadata.Namespace
					opts.ClusterScopedAllowList = []templates.ClusterScopedObjectIdentifier{
						{Group: "cluster.open-cluster-management.io", Kind: "ManagedCluster", Name: cluster}}
				}
				if templates.UsesEncryption(od, "{{hub", "hub}}") {
					opts.EncryptionConfig = templates.EncryptionConfig{EncryptionEnabled: true, AESKey: aesKey, InitializationVector: aesIV}
					managedOpts.EncryptionConfig = templates.EncryptionConfig{DecryptionEnabled: true, AESKey: aesKey, InitializationVector: aesIV}
				}
				ctx := hubContext{cluster, map[string]string{"name": cluster}, map[string]any{
					"name": p.Metadata.Name, "namespace": p.Metadata.Namespace,
					"labels": p.Metadata.Labels, "annotations": p.Metadata.Annotations}}
				res, err := hub.ResolveTemplate(od, ctx, opts)
				switch {
				case apierrors.IsNotFound(err):
					// Stand in for the missing object so the managed templates are still checked.
					res.ResolvedJSON = hubTemplate.ReplaceAll(od, []byte("hub"))
				case err != nil:
					fail(-1, true, short(err))
					continue
				}
				cp = configPolicy{}
				if err := json.Unmarshal(res.ResolvedJSON, &cp); err != nil {
					fail(-1, true, err.Error())
					continue
				}
			}
			if managedDisabled {
				continue
			}

			// The config-policy-controller: object-templates-raw whole, with no context.
			if raw := []byte(cp.Spec.Raw); templates.HasTemplate(raw, "", true) {
				opts := managedOpts
				opts.InputIsYAML = true
				res, err := managed.ResolveTemplate(raw, nil, &opts)
				if err != nil {
					if !apierrors.IsNotFound(err) {
						fail(-1, false, short(err))
					}
					continue
				}
				var ots []struct {
					ComplianceType   string          `json:"complianceType"`
					ObjectDefinition json.RawMessage `json:"objectDefinition"`
				}
				if err := json.Unmarshal(res.ResolvedJSON, &ots); err != nil {
					fail(-1, false, "resolves to something other than a list of object templates")
					continue
				}
				for _, ot := range ots {
					if msg := checkObject(ot.ObjectDefinition); ot.ComplianceType == "" || msg != "" {
						fail(-1, false, cmp.Or(msg, "resolves to an object template without complianceType"))
						break
					}
				}
				continue
			}

			// Each object-templates entry separately, with skipObject.
			for i, ot := range cp.Spec.ObjectTemplates {
				if !templates.HasTemplate(ot.ObjectDefinition, "", true) {
					continue
				}
				skipped := false
				opts := managedOpts
				opts.CustomFunctions = map[string]any{"skipObject": func(skips ...any) (string, error) {
					switch {
					case len(skips) == 0:
						skipped = true
					case len(skips) == 1:
						skip, ok := skips[0].(bool)
						if !ok {
							return "", fmt.Errorf("expected boolean but received '%v'", skips[0])
						}
						skipped = skipped || skip
					default:
						return "", fmt.Errorf("expected one optional boolean argument but received %d arguments", len(skips))
					}
					return "", nil
				}}
				ctx := objectContext(ot.ObjectDefinition, len(ot.ObjectSelector) > 0, len(cp.Spec.NamespaceSelector) > 0)
				res, err := managed.ResolveTemplate(ot.ObjectDefinition, ctx, &opts)
				switch {
				case skipped:
				case err != nil && !apierrors.IsNotFound(err):
					fail(i, false, short(err))
				case err == nil:
					if msg := checkObject(res.ResolvedJSON); msg != "" {
						fail(i, false, msg)
					}
				}
			}
		}
	}
	return out, nil
}

// objectContext is the config-policy-controller's context for an object-templates entry whose
// object does not exist yet (getTemplateContext). A templated name or namespace counts as unset, and
// a selector stands in for one.
func objectContext(od json.RawMessage, objectSelector, namespaceSelector bool) any {
	var o struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(od, &o)
	name, ns := o.Metadata.Name, o.Metadata.Namespace
	if templates.HasTemplate([]byte(name), "", true) {
		name = ""
	}
	if templates.HasTemplate([]byte(ns), "", true) {
		ns = ""
	}
	if name == "" && objectSelector {
		name = "selected"
	}
	if ns == "" && namespaceSelector {
		ns = "selected"
	}
	obj := map[string]any{}
	switch {
	case name != "" && ns != "":
		return struct {
			Object          map[string]any
			ObjectNamespace string
			ObjectName      string
		}{obj, ns, name}
	case name != "":
		return struct {
			Object     map[string]any
			ObjectName string
		}{obj, name}
	case ns != "":
		return struct{ ObjectNamespace string }{ns}
	}
	return struct{}{}
}

// noValue is what text/template prints for a missing map key, plain or JSON-escaped.
var noValue = regexp.MustCompile(`(<|\\u003c)no value(>|\\u003e)`)

// checkObject reports a resolved objectDefinition ACM could not apply as intended.
func checkObject(od json.RawMessage) string {
	var o struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	switch {
	case json.Unmarshal(od, &o) != nil || o.APIVersion == "" || o.Kind == "":
		return "resolves to an object without apiVersion or kind"
	case noValue.Match(od):
		return "resolves to <no value>: a field the template reads is missing"
	}
	return ""
}

// tmplPrefix is text/template's location prefix, which points into the rendered template.
var tmplPrefix = regexp.MustCompile(`template: tmpl:\d+(:\d+)?: (executing "tmpl" )?`)

// short drops the whole-template text go-template-utils wraps around an error.
func short(err error) string {
	if u := errors.Unwrap(err); u != nil && strings.HasPrefix(err.Error(), "failed to ") {
		err = u
	}
	return tmplPrefix.ReplaceAllString(err.Error(), "")
}
