// Package cascade resolves a fleet file into the ordered list of values files
// passed to helm.
//
// It mirrors the valueFiles block in appset/templates/appset.yaml. Keep the two
// in sync; any divergence is a bug. appset/templates/lifecycle.yaml applies the
// same cascade to lifecycle/, and run.Run appends the fleet file for its
// valuesObject.
//
// Cascade order (lowest precedence first):
//
//  1. element defaults  <element>/values.yaml
//  2. global root       <repoRoot>/values.yaml
//  3. valueFiles        <values>/<entry>, in fleet file order
//  4. cluster-specific  <values>/clusters/<name>.yaml
//
// Missing files in steps 1, 2 and 4 are skipped, matching the appset's
// ignoreMissingValueFiles: true. A missing valueFiles entry is an issue.
package cascade

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
)

// Resolved is the output of Resolve.
type Resolved struct {
	// ClusterName is the helm release suffix and the <cluster> part of
	// policy names: the ManagedCluster name, or hubName for a hub.
	ClusterName string
	// ReleaseName is the helm Release.Name, <element>-<cluster>, matching the
	// Application name set in appset.yaml.
	ReleaseName string
	// ValueFiles is the ordered list of -f arguments (absolute paths).
	ValueFiles []string
	// Issues lists the fleet file's problems, then valueFiles entries that
	// are not files. POLICY050 reports them.
	Issues []fixtures.Issue
}

// Resolve builds the cascade for c. element is the chart directory,
// repoRoot holds the global values.yaml, and valuesRoot is normally
// <repoRoot>/values.
func Resolve(c *fixtures.Cluster, element, chartName, repoRoot, valuesRoot string) Resolved {
	r := Resolved{Issues: slices.Clone(c.Issues), ClusterName: c.Name}

	addIfExists(&r.ValueFiles, filepath.Join(element, "values.yaml"))
	addIfExists(&r.ValueFiles, filepath.Join(repoRoot, "values.yaml"))
	for _, f := range c.ValueFiles {
		path := filepath.Join(valuesRoot, f)
		if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() {
			r.Issues = append(r.Issues, fixtures.Issue{Message: fmt.Sprintf("valueFiles %q: not a file under values/", f)})
			continue
		}
		r.ValueFiles = append(r.ValueFiles, path)
	}
	addIfExists(&r.ValueFiles, filepath.Join(valuesRoot, "clusters", c.Name+".yaml"))

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
