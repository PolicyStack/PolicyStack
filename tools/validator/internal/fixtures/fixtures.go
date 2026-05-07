// Package fixtures loads ManagedCluster YAML manifests from a directory.
// Fixtures stand in for real ACM-managed clusters during local/CI rendering.
package fixtures

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// ManagedCluster is the trimmed ACM ManagedCluster shape we need.
type ManagedCluster struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
	// SourceFile is the absolute fixture path; populated during Load.
	SourceFile string `json:"-"`
}

// Metadata holds the cluster name and labels (which drive the cascade).
type Metadata struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

// LoadDir reads every *.yaml/*.yml under dir as a ManagedCluster and returns
// the parsed list. Files that don't unmarshal as kind=ManagedCluster are
// skipped silently — keeps the fixture dir flexible.
func LoadDir(dir string) ([]*ManagedCluster, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read fixtures dir: %w", err)
	}
	var out []*ManagedCluster
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".yaml") && !strings.HasSuffix(n, ".yml") {
			continue
		}
		path := filepath.Join(dir, n)
		mc, err := load(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if mc == nil {
			continue
		}
		out = append(out, mc)
	}
	return out, nil
}

func load(path string) (*ManagedCluster, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mc ManagedCluster
	if err := yaml.Unmarshal(data, &mc); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if mc.Kind != "ManagedCluster" || mc.Metadata.Name == "" {
		return nil, nil
	}
	mc.SourceFile = path
	return &mc, nil
}
