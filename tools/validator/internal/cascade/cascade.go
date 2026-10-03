// Package cascade resolves a fleet file into the ordered list of values files
// passed to helm.
//
// It mirrors the valueFiles block in appset/templates/appset.yaml. Keep the two
// in sync; any divergence is a bug.
//
// Cascade order (lowest precedence first):
//
//  1. element defaults       <element>/values.yaml
//  2. global root            <repoRoot>/values.yaml
//  3. config entries         <values>/<category>s/<value>.yaml, in ascending
//     priority order (higher priority overrides)
//  4. cluster-specific       hub: <values>/acm/acm-<dc>.yaml then
//     <values>/clusters/acm-<dc>.yaml
//     else: <values>/clusters/<cluster-name>.yaml
//
// Missing files are skipped, matching the appset's ignoreMissingValueFiles: true.
package cascade

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
)

// Resolved is the output of Resolve.
type Resolved struct {
	// ClusterName is the helm release suffix and the <cluster> part of
	// policy names. For hubs it is "acm-<datacenter>".
	ClusterName string
	// ReleaseName is the helm Release.Name, <element>-<cluster>, matching the
	// Application name set in appset.yaml.
	ReleaseName string
	// ValueFiles is the ordered list of -f arguments (absolute paths).
	ValueFiles []string
	// IsLocalHub is true for a hub fleet file (hubs/<hubName>.yaml). On a
	// cluster it is ACM's local-cluster label.
	IsLocalHub  bool
	Datacenter  string
	Environment string
	// Issues lists the fleet file's problems, then malformed or duplicate
	// config keys. POLICY050 reports them.
	Issues []fixtures.Issue
}

type entry struct {
	priority int
	category string
	value    string
}

// Resolve builds the cascade for c. element is the chart directory,
// repoRoot holds the global values.yaml, and valuesRoot is normally
// <repoRoot>/values.
func Resolve(c *fixtures.Cluster, element, chartName, repoRoot, valuesRoot string) Resolved {
	r := Resolved{Issues: slices.Clone(c.Issues)}

	// 1-2. element defaults and global root
	addIfExists(&r.ValueFiles, filepath.Join(element, "values.yaml"))
	addIfExists(&r.ValueFiles, filepath.Join(repoRoot, "values.yaml"))

	// 3. config entries, keyed <category>.<priority>
	dups := map[string]struct{}{}
	priorityKeys := map[string]struct{}{}
	var entries []entry

	// Sorted, as Go templates range over maps, so the last datacenter wins in both.
	for _, k := range slices.Sorted(maps.Keys(c.Config)) {
		v := c.Config[k]
		// Split at the first dot, as appset's splitList "." does.
		category, prioStr, ok := strings.Cut(k, ".")
		if !ok {
			r.Issues = append(r.Issues, keyIssue(k, "missing priority"))
			continue
		}
		if category == "" || prioStr == "" {
			r.Issues = append(r.Issues, keyIssue(k, "empty category or priority"))
			continue
		}
		prio, err := strconv.Atoi(prioStr)
		if err != nil {
			r.Issues = append(r.Issues, keyIssue(k, "non-numeric priority"))
			continue
		}
		if !categoryRE.MatchString(category) {
			r.Issues = append(r.Issues, keyIssue(k, "category must be alphanumerics, '-' or '_'"))
			continue
		}
		// Category comparison is case-insensitive; 10 and 010 sort as one priority in appset.
		dupKey := strings.ToLower(category) + "." + strconv.Itoa(prio)
		if _, ok := priorityKeys[dupKey]; ok {
			if _, alreadyReported := dups[dupKey]; !alreadyReported {
				r.Issues = append(r.Issues, keyIssue(k, "duplicate <category>.<priority>"))
				dups[dupKey] = struct{}{}
			}
		}
		priorityKeys[dupKey] = struct{}{}

		entries = append(entries, entry{priority: prio, category: category, value: v})

		// Datacenter names the hub values files in step 4; Environment is informational.
		// Case-sensitive, as appset's eq $category "datacenter".
		switch category {
		case "environment":
			r.Environment = v
		case "datacenter":
			r.Datacenter = v
		}
	}

	// Ascending priority, so higher priorities come later; helm gives later -f files precedence.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].priority != entries[j].priority {
			return entries[i].priority < entries[j].priority
		}
		return entries[i].category < entries[j].category
	})
	for _, e := range entries {
		// Pluralised with a plain "s", as appset's printf "%ss" does.
		path := filepath.Join(valuesRoot, e.category+"s", e.value+".yaml")
		addIfExists(&r.ValueFiles, path)
	}

	// 4. cluster-specific
	r.IsLocalHub = c.Hub
	r.ClusterName = c.Name
	if r.IsLocalHub && r.Datacenter == "" {
		// appset would name the hub's Applications <element>-acm-.
		r.Issues = append(r.Issues, fixtures.Issue{Message: "a hub fleet file needs a datacenter.<priority> entry"})
	}
	if r.IsLocalHub && r.Datacenter != "" {
		r.ClusterName = fmt.Sprintf("acm-%s", r.Datacenter)
		addIfExists(&r.ValueFiles, filepath.Join(valuesRoot, "acm", r.ClusterName+".yaml"))
		addIfExists(&r.ValueFiles, filepath.Join(valuesRoot, "clusters", r.ClusterName+".yaml"))
	} else {
		addIfExists(&r.ValueFiles, filepath.Join(valuesRoot, "clusters", c.Name+".yaml"))
	}

	r.ReleaseName = chartName + "-" + r.ClusterName
	return r
}

// categoryRE is a label name without dots: the category becomes a directory
// name, values/<category>s/.
var categoryRE = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_]*[A-Za-z0-9])?$`)

func keyIssue(key, reason string) fixtures.Issue {
	return fixtures.Issue{Message: fmt.Sprintf("config %q: %s", key, reason)}
}

func addIfExists(list *[]string, path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err == nil {
		*list = append(*list, path)
	}
}
