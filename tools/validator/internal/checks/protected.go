package checks

import (
	"fmt"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// AllowDestructive is the ConfigurationPolicy annotation that exempts it from POLICY037. Its value
// is the reason.
const AllowDestructive = "validator.policystk.io/allow-destructive"

// ProtectedObjectCheck (POLICY037) reports a rendered ConfigurationPolicy object-template that is
// mustonlyhave or mustnothave on an object the OpenShift installer or its operators own. Enforced,
// mustonlyhave replaces the object, dropping what the platform wrote (the installer's CAs in
// user-ca-bundle), and mustnothave deletes it (a shared operator namespace). musthave is not
// covered: merging into these objects is how elements configure them. An InformOnly template is
// skipped, since ACM never enforces it whatever the Policy says, and so is a template annotated
// AllowDestructive, and so is a mustonlyhave that keepsLive. Templated object-templates-raw is read with its ACM template actions removed, so
// every branch counts, and a templated name counts as the text around the actions.
type ProtectedObjectCheck struct{}

func (ProtectedObjectCheck) ID() string   { return "POLICY037" }
func (ProtectedObjectCheck) Phase() Phase { return PhaseCluster }

// templateAction matches an ACM template action, hub or managed.
var templateAction = regexp.MustCompile(`(?s)\{\{.*?\}\}`)

func (c *ProtectedObjectCheck) Run(ctx Context) []Finding {
	if len(ctx.Rendered) == 0 || ctx.Element == nil || ctx.Cluster == nil {
		return nil
	}
	pols, bad := policies(ctx, c.ID())
	if bad != nil {
		return bad
	}
	var out []Finding
	for _, p := range pols {
		for _, t := range p.Spec.PolicyTemplates {
			od := t.ObjectDefinition
			if od.Kind != "ConfigurationPolicy" || informOnly(od.Spec.RemediationAction) || od.Metadata.Annotations[AllowDestructive] != "" {
				continue
			}
			add := func(sev Severity, msg string) {
				where, file, loc := origin(ctx, od.Kind, od.Metadata.Name, "name")
				out = append(out, Finding{RuleID: c.ID(), Severity: sev, Element: ctx.Element.ChartName,
					Message: where + ": " + msg, File: file, Line: loc.Line, Col: loc.Col})
			}
			ots := od.Spec.ObjectTemplates
			if raw := templateAction.ReplaceAllString(od.Spec.Raw, ""); raw != "" {
				if err := yaml.Unmarshal([]byte(raw), &ots); err != nil {
					add(SevWarning, "object-templates-raw is not YAML once its template actions are removed, so it is not checked: "+err.Error())
				}
			}
			for _, ot := range ots {
				ct, o := ot.ComplianceType, ot.ObjectDefinition
				name := templateAction.ReplaceAllString(o.Metadata.Name, "")
				ns := templateAction.ReplaceAllString(o.Metadata.Namespace, "")
				if (!strings.EqualFold(ct, "mustonlyhave") && !strings.EqualFold(ct, "mustnothave")) || !protected(o.APIVersion, o.Kind, ns, name) || keepsLive(ot) {
					continue
				}
				add(SevError, fmt.Sprintf("%s %s %s, which OpenShift owns: enforced, it replaces or deletes platform state. Use musthave, or InformOnly for a gate",
					ct, o.Kind, strings.TrimPrefix(ns+"/"+name, "/")))
			}
		}
	}
	return out
}

// specAssign matches an assignment of $spec.
var specAssign = regexp.MustCompile(`\$spec\s*:?=`)

// keepsLive reports a mustonlyhave on a config.openshift.io object that starts from what the
// platform wrote: its metadata is musthave and its whole spec is one template that first binds $spec
// to a copy of the same object's live spec, never reassigns it, and last outputs it. It is how an
// element takes an entry out of a list there, which musthave can only add to. Raw templates lose
// their actions before this and never qualify.
func keepsLive(ot objectTemplate) bool {
	o := ot.ObjectDefinition
	spec, ok := o.Spec.(string)
	if !ok || !strings.EqualFold(ot.ComplianceType, "mustonlyhave") || !strings.EqualFold(ot.MetadataComplianceType, "musthave") ||
		!strings.HasPrefix(o.APIVersion, "config.openshift.io/") {
		return false
	}
	spec = strings.TrimSpace(spec)
	first := fmt.Sprintf(`{{ $spec := deepCopy (dig "spec" dict (lookup %q %q %q %q)) }}`, o.APIVersion, o.Kind, o.Metadata.Namespace, o.Metadata.Name)
	return strings.HasPrefix(spec, first) && strings.HasSuffix(spec, "{{ $spec | toRawJson | toLiteral }}") &&
		len(specAssign.FindAllString(spec, -1)) == 1 && !strings.Contains(spec, "{{hub")
}

// protected reports an object the OpenShift installer or its operators own.
func protected(apiVersion, kind, ns, name string) bool {
	config := strings.HasPrefix(apiVersion, "config.openshift.io/")
	switch kind {
	case "Namespace":
		return apiVersion == "v1" && (strings.HasPrefix(name, "openshift-") || strings.HasPrefix(name, "kube-"))
	case "ConfigMap":
		return apiVersion == "v1" && ns == "openshift-config" && name == "user-ca-bundle"
	case "Secret":
		return apiVersion == "v1" && ns == "openshift-config" && name == "pull-secret"
	case "ClusterVersion":
		return config && name == "version"
	case "Proxy", "Image":
		return config && name == "cluster"
	}
	return false
}
