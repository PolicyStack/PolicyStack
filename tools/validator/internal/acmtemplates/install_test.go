package acmtemplates

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestChildPointer: the parent's Policy policystack-hubs gives a child the root's pointer with the
// child's ManagedCluster name as hubName, the parent's as parent, and the Argo CD templates untouched,
// since the ConfigurationPolicy disables managed templates.
func TestChildPointer(t *testing.T) {
	rendered := helmTemplate(t, filepath.Join(repoRoot, "install"), "policystack", nil,
		"hubName=gh", "gitRepo=https://example.invalid/r.git", "repoSecret=policystack-repo")
	objs, failures, err := Resolve(rendered, "hub-a")
	if err != nil || len(failures) > 0 {
		t.Fatalf("resolve: %v %+v", err, failures)
	}
	var pointer struct {
		Kind string `json:"kind"`
		Spec struct {
			Generators []struct {
				Git struct {
					Files []struct{ Path string } `json:"files"`
				} `json:"git"`
			} `json:"generators"`
			Template struct {
				Spec struct {
					Source struct {
						TargetRevision string `json:"targetRevision"`
						Helm           struct {
							ValuesObject map[string]string `json:"valuesObject"`
						} `json:"helm"`
					} `json:"source"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}
	var secret bool
	for _, o := range objs {
		switch o.ConfigPolicy {
		case "policystack-hubs-install":
			// musthave would merge lists: a changed gitRepo would add a second generator.
			if o.ComplianceType != "mustonlyhave" {
				t.Errorf("pointer complianceType %q, want mustonlyhave", o.ComplianceType)
			}
			if err := json.Unmarshal(o.Definition, &pointer); err != nil {
				t.Fatal(err)
			}
		case "policystack-hubs-repo":
			secret = o.ComplianceType == "mustonlyhave"
		}
	}
	if pointer.Kind != "ApplicationSet" || len(pointer.Spec.Generators) != 1 || len(pointer.Spec.Generators[0].Git.Files) != 1 {
		t.Fatalf("pointer %+v", pointer)
	}
	if got := pointer.Spec.Generators[0].Git.Files[0].Path; got != "fleet/hubs/hub-a.yaml" {
		t.Errorf("pointer reads %q", got)
	}
	src := pointer.Spec.Template.Spec.Source
	if src.TargetRevision != `{{ dig "installRevision" "HEAD" . }}` {
		t.Errorf("targetRevision %q", src.TargetRevision)
	}
	want := map[string]string{"gitRepo": "https://example.invalid/r.git", "hubName": "hub-a", "repoSecret": "policystack-repo", "parent": "gh"}
	for k, v := range want {
		if src.Helm.ValuesObject[k] != v {
			t.Errorf("valuesObject %v, want %v", src.Helm.ValuesObject, want)
			break
		}
	}
	if !secret {
		t.Error("no mustonlyhave repository Secret")
	}
}
