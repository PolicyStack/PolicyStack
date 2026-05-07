// Package cascade resolves a ManagedCluster + label set into the ordered
// list of values files that helm should be invoked with.
//
// This mirrors appset/templates/appset.yaml lines 71-149 of the PolicyStack
// repo. Any divergence here is a bug — keep them in sync.
//
// Cascade order (lowest precedence first):
//
//  1. element-defaults       <element>/values.yaml
//  2. global root            <repoRoot>/values.yaml
//  3. label-driven entries   <values>/<categorys>/<value>.yaml, sorted by
//                            ascending priority (higher priority overrides)
//  4. cluster-specific       hub: <values>/acm/acm-<dc>.yaml then
//                                  <values>/clusters/acm-<dc>.yaml
//                            else: <values>/clusters/<cluster-name>.yaml
//
// Files that do not exist are silently dropped (matches the appset's
// ignoreMissingValueFiles: true).
package cascade

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
)

// Resolved is the output of Resolve.
type Resolved struct {
	// ClusterName is what gets used as the helm release suffix and as
	// <cluster> in the policy-naming math. For local-cluster=true hubs
	// this is "acm-<datacenter>".
	ClusterName string
	// ReleaseName is the helm Release.Name (matches appset's Application
	// name, line 26 of appset.yaml: <element>-<cluster>).
	ReleaseName string
	// ValueFiles is the ordered list of -f arguments (absolute paths).
	ValueFiles []string
	// IsLocalHub mirrors metadata.labels["local-cluster"] == "true".
	IsLocalHub bool
	Datacenter string
	Environment string
	// LabelIssues is non-nil when malformed config labels were observed.
	// The check layer turns these into POLICY050 findings.
	LabelIssues []LabelIssue
}

// LabelIssue describes a malformed or duplicate config label key.
type LabelIssue struct {
	Key    string
	Reason string // e.g. "duplicate priority", "missing priority", "non-numeric priority"
}

type entry struct {
	priority int
	category string
	value    string
}

// Resolve mirrors appset/templates/appset.yaml. element is the chart dir
// (used for the chart-default values.yaml). repoRoot must contain the
// global values.yaml. valuesRoot is typically <repoRoot>/values.
func Resolve(mc *fixtures.ManagedCluster, element, chartName, repoRoot, valuesRoot, baseDomain string) Resolved {
	r := Resolved{}
	prefix := "config." + baseDomain + "/"

	// 1. element-default + global root
	addIfExists(&r.ValueFiles, filepath.Join(element, "values.yaml"))
	addIfExists(&r.ValueFiles, filepath.Join(repoRoot, "values.yaml"))

	// 2. parse config labels
	type seen struct{ value string }
	dups := map[string]struct{}{}
	priorityKeys := map[string]struct{}{}
	var entries []entry

	for k, v := range mc.Metadata.Labels {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		// expect <category>.<priority>
		dot := strings.LastIndex(rest, ".")
		if dot < 0 {
			r.LabelIssues = append(r.LabelIssues, LabelIssue{Key: k, Reason: "missing priority"})
			continue
		}
		category := rest[:dot]
		prioStr := rest[dot+1:]
		prio, err := strconv.Atoi(prioStr)
		if err != nil {
			r.LabelIssues = append(r.LabelIssues, LabelIssue{Key: k, Reason: "non-numeric priority"})
			continue
		}
		if category == "" || prioStr == "" {
			r.LabelIssues = append(r.LabelIssues, LabelIssue{Key: k, Reason: "empty category or priority"})
			continue
		}
		// duplicate (category.priority) detection — case-insensitive on category
		dupKey := strings.ToLower(category) + "." + prioStr
		if _, ok := priorityKeys[dupKey]; ok {
			if _, alreadyReported := dups[dupKey]; !alreadyReported {
				r.LabelIssues = append(r.LabelIssues, LabelIssue{Key: k, Reason: "duplicate <category>.<priority>"})
				dups[dupKey] = struct{}{}
			}
		}
		priorityKeys[dupKey] = struct{}{}

		entries = append(entries, entry{priority: prio, category: category, value: v})

		// capture environment / datacenter for downstream use (best-effort)
		switch strings.ToLower(category) {
		case "environment":
			r.Environment = v
		case "datacenter":
			r.Datacenter = v
		}
	}

	// 3. sort numerically ascending — higher priority is appended later (helm
	// merges right-to-left so later -f wins).
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].priority != entries[j].priority {
			return entries[i].priority < entries[j].priority
		}
		return entries[i].category < entries[j].category
	})
	for _, e := range entries {
		// pluralize by appending "s" — matches appset's printf "%ss".
		path := filepath.Join(valuesRoot, e.category+"s", e.value+".yaml")
		addIfExists(&r.ValueFiles, path)
	}

	// 4. cluster-specific
	r.IsLocalHub = strings.EqualFold(mc.Metadata.Labels["local-cluster"], "true")
	r.ClusterName = mc.Metadata.Name
	if r.IsLocalHub && r.Datacenter != "" {
		r.ClusterName = fmt.Sprintf("acm-%s", r.Datacenter)
		addIfExists(&r.ValueFiles, filepath.Join(valuesRoot, "acm", r.ClusterName+".yaml"))
		addIfExists(&r.ValueFiles, filepath.Join(valuesRoot, "clusters", r.ClusterName+".yaml"))
	} else {
		addIfExists(&r.ValueFiles, filepath.Join(valuesRoot, "clusters", mc.Metadata.Name+".yaml"))
	}

	r.ReleaseName = chartName + "-" + r.ClusterName
	return r
}

func addIfExists(list *[]string, path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err == nil {
		*list = append(*list, path)
	}
}
