// Package fixtures loads fleet files from a directory: <dir>/<cluster>.yaml for
// spokes and <dir>/hubs/<hubName>.yaml for hubs, the layout of fleet/.
package fixtures

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"
)

// Cluster is one fleet file.
type Cluster struct {
	// Name is the file name without .yaml: the ManagedCluster name for a
	// spoke, the appset chart's hubName for a hub.
	Name     string
	Revision string
	// ValueFiles lists paths under values/, lowest precedence first.
	// Entries with an issue are left out.
	ValueFiles []string
	// Hub is the hubName of the hub that builds the cluster; empty for an
	// import-only cluster. State is present or absent, empty meaning present.
	Hub, State string
	SourceFile string
	// Issues lists problems in the file. POLICY050 reports them.
	Issues []Issue

	hubLine int // line of the hub key, 0 when absent
}

// Issue is a problem in a fleet file.
type Issue struct {
	Line    int // 0 when unknown
	Message string
}

// fleetFile is the on-disk format. Nodes keep line numbers for findings.
type fleetFile struct {
	Revision   yaml.Node   `yaml:"revision"`
	ValueFiles []yaml.Node `yaml:"valueFiles"`
	Hub        yaml.Node   `yaml:"hub"`
	State      yaml.Node   `yaml:"state"`
	Install    yaml.Node   `yaml:"install"`
}

// argoFile is the file as Argo CD's git files generator reads it:
// sigs.k8s.io/yaml (YAML 1.1) into untyped values, so an unquoted 1.10 or yes
// reaches the template as a number or a boolean.
type argoFile struct {
	Revision   any   `json:"revision"`
	ValueFiles []any `json:"valueFiles"`
	Hub        any   `json:"hub"`
	State      any   `json:"state"`
	Install    any   `json:"install"`
}

// dnsLabel is the RFC 1123 label a HostedCluster name must be.
var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// LoadDir loads dir/*.yaml as spokes and dir/hubs/*.yaml as hubs. hubs/ is
// optional. Other files are ignored.
func LoadDir(dir string) ([]*Cluster, error) {
	spokes, err := loadFiles(dir)
	if err != nil {
		return nil, err
	}
	hubs, err := loadFiles(filepath.Join(dir, "hubs"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, h := range hubs {
		if h.hubLine != 0 {
			h.Issues = append(h.Issues, Issue{Line: h.hubLine, Message: "hub is not allowed in a hub file: a hub cannot build itself"})
			h.Hub = "" // the lifecycle ApplicationSet skips hub files
		}
	}
	for _, c := range spokes {
		if c.Hub == "" {
			continue
		}
		// A typo would leave the cluster with no hub to build or destroy it.
		if !slices.ContainsFunc(hubs, func(h *Cluster) bool { return h.Name == c.Hub }) {
			c.Issues = append(c.Issues, Issue{Line: c.hubLine, Message: fmt.Sprintf("hub %q has no hubs/%s.yaml", c.Hub, c.Hub)})
		}
		if !dnsLabel.MatchString(c.Name) {
			c.Issues = append(c.Issues, Issue{Message: fmt.Sprintf("cluster name %q is not a DNS label: the hub names the HostedCluster after it", c.Name)})
		}
	}
	return append(spokes, hubs...), nil
}

func loadFiles(dir string) ([]*Cluster, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read fixtures dir: %w", err)
	}
	var out []*Cluster
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".yaml")
		if e.IsDir() || !ok {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		c := parse(data)
		c.Name, c.SourceFile = name, path
		out = append(out, c)
	}
	return out, nil
}

func parse(data []byte) *Cluster {
	c := &Cluster{}
	var f fleetFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		te, ok := errors.AsType[*yaml.TypeError](err)
		if !ok {
			c.Issues = append(c.Issues, yamlIssue(err.Error()))
			return c
		}
		// yaml.v3 keeps decoding after a type error, so f holds what it could read.
		for _, msg := range te.Errors {
			c.Issues = append(c.Issues, yamlIssue(msg))
		}
	}
	var a argoFile
	_ = sigsyaml.Unmarshal(data, &a) // yaml.v3 above reports what fails here

	switch r := a.Revision.(type) {
	case nil:
		c.Issues = append(c.Issues, Issue{Line: f.Revision.Line, Message: "revision is required"})
	case string:
		if r == "" {
			c.Issues = append(c.Issues, Issue{Line: f.Revision.Line, Message: "revision is empty"})
		}
		c.Revision = r
	default:
		c.Issues = append(c.Issues, Issue{Line: f.Revision.Line, Message: "revision " + notString(r)})
	}

	for i, v := range a.ValueFiles {
		var line int
		if i < len(f.ValueFiles) {
			line = f.ValueFiles[i].Line
		}
		switch s, ok := v.(string); {
		case v == nil:
			c.Issues = append(c.Issues, Issue{Line: line, Message: "valueFiles entry has no value"})
		case !ok:
			c.Issues = append(c.Issues, Issue{Line: line, Message: "valueFiles entry " + notString(v)})
		case slices.Contains(c.ValueFiles, s):
			c.Issues = append(c.Issues, Issue{Line: line, Message: fmt.Sprintf("valueFiles %q is listed twice", s)})
		default:
			c.ValueFiles = append(c.ValueFiles, s)
		}
	}

	// Kind is 0 when the key is absent. Argo CD's selector compares hub as a string.
	c.hubLine = f.Hub.Line
	switch h := a.Hub.(type) {
	case nil:
		if f.Hub.Kind != 0 {
			c.Issues = append(c.Issues, Issue{Line: f.Hub.Line, Message: "hub has no value"})
		}
	case string:
		if h == "" {
			c.Issues = append(c.Issues, Issue{Line: f.Hub.Line, Message: "hub is empty"})
		}
		c.Hub = h
	default:
		c.Issues = append(c.Issues, Issue{Line: f.Hub.Line, Message: "hub " + notString(h)})
	}
	switch s := a.State.(type) {
	case string:
		if s != "present" && s != "absent" {
			c.Issues = append(c.Issues, Issue{Line: f.State.Line, Message: fmt.Sprintf("state %q must be present or absent", s)})
		}
		c.State = s
	case nil:
		if f.State.Kind != 0 {
			c.Issues = append(c.Issues, Issue{Line: f.State.Line, Message: "state has no value: it must be present or absent"})
		}
	default:
		c.Issues = append(c.Issues, Issue{Line: f.State.Line, Message: "state " + notString(s)})
	}
	if _, ok := a.Install.(map[string]any); !ok && f.Install.Kind != 0 {
		c.Issues = append(c.Issues, Issue{Line: f.Install.Line, Message: "install must be a map"})
	}
	if f.Hub.Kind == 0 {
		if f.State.Kind != 0 {
			c.Issues = append(c.Issues, Issue{Line: f.State.Line, Message: "state requires hub: only a hub acts on it"})
		}
		if f.Install.Kind != 0 {
			c.Issues = append(c.Issues, Issue{Line: f.Install.Line, Message: "install requires hub: only a hub acts on it"})
		}
	}
	return c
}

// notString describes a value Argo CD would not template as written.
func notString(v any) string {
	return fmt.Sprintf("is read by Argo CD as %v (%T), not a string: quote it", v, v)
}

// yamlIssue turns a yaml.v3 message such as "yaml: line 3: field revison not
// found in type fixtures.fleetFile" into an Issue with its line.
func yamlIssue(msg string) Issue {
	msg = strings.TrimPrefix(msg, "yaml: ")
	is := Issue{Message: msg}
	if n, _ := fmt.Sscanf(msg, "line %d:", &is.Line); n == 1 {
		_, is.Message, _ = strings.Cut(msg, ": ")
	}
	if field, ok := strings.CutPrefix(is.Message, "field "); ok {
		if name, _, ok := strings.Cut(field, " not found in type "); ok {
			is.Message = "unknown key " + name + ": only revision, valueFiles, hub, state and install are allowed"
		}
	}
	return is
}
