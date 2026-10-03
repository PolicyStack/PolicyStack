// Package fixtures loads fleet files from a directory: <dir>/<cluster>.yaml for
// spokes and <dir>/hubs/<hubName>.yaml for hubs, the layout of fleet/.
package fixtures

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
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
	// Config maps <category>.<priority> to a value. Entries with an issue
	// are left out.
	Config map[string]string
	// Hub is true for files under hubs/.
	Hub        bool
	SourceFile string
	// Issues lists problems in the file. POLICY050 reports them.
	Issues []Issue
}

// Issue is a problem in a fleet file.
type Issue struct {
	Line    int // 0 when unknown
	Message string
}

// fleetFile is the on-disk format. Nodes keep line numbers for findings.
type fleetFile struct {
	Revision yaml.Node            `yaml:"revision"`
	Config   map[string]yaml.Node `yaml:"config"`
}

// argoFile is the file as Argo CD's git files generator reads it:
// sigs.k8s.io/yaml (YAML 1.1) into untyped values, so an unquoted 1.10 or yes
// reaches the template as a number or a boolean.
type argoFile struct {
	Revision any            `json:"revision"`
	Config   map[string]any `json:"config"`
}

// labelValue is Kubernetes label-value syntax, without the empty value.
var labelValue = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$`)

// LoadDir loads dir/*.yaml as spokes and dir/hubs/*.yaml as hubs. hubs/ is
// optional. Other files are ignored.
func LoadDir(dir string) ([]*Cluster, error) {
	spokes, err := loadFiles(dir, false)
	if err != nil {
		return nil, err
	}
	hubs, err := loadFiles(filepath.Join(dir, "hubs"), true)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return append(spokes, hubs...), nil
}

func loadFiles(dir string, hub bool) ([]*Cluster, error) {
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
		c.Name, c.Hub, c.SourceFile = name, hub, path
		out = append(out, c)
	}
	return out, nil
}

func parse(data []byte) *Cluster {
	c := &Cluster{Config: map[string]string{}}
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

	for _, k := range slices.Sorted(maps.Keys(a.Config)) {
		n := f.Config[k]
		switch v, ok := a.Config[k].(string); {
		case a.Config[k] == nil:
			c.Issues = append(c.Issues, Issue{Line: n.Line, Message: fmt.Sprintf("config %q has no value", k)})
		case !ok:
			c.Issues = append(c.Issues, Issue{Line: n.Line, Message: fmt.Sprintf("config %q %s", k, notString(a.Config[k]))})
		case len(v) > 63 || !labelValue.MatchString(v):
			c.Issues = append(c.Issues, Issue{Line: n.Line, Message: fmt.Sprintf(
				"config %q: %q is not a label value (at most 63 alphanumerics, '-', '_' or '.', starting and ending with an alphanumeric)", k, v)})
		default:
			c.Config[k] = v
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
			is.Message = "unknown key " + name + ": only revision and config are allowed"
		}
	}
	return is
}
