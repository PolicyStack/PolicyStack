package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/sourceloc"
)

// hostFields are the fields no two siteconfig nodes on one hub may share, with how each compares: a
// MAC case-insensitively, a BMC address also without its scheme, which only picks the driver.
var hostFields = []struct {
	key  string
	norm func(string) string
}{
	{"bootMACAddress", strings.ToLower},
	{"bmcAddress", func(s string) string {
		s = strings.ToLower(s)
		if _, addr, ok := strings.Cut(s, "://"); ok {
			return addr
		}
		return s
	}},
}

// BareMetalCheck (POLICY051) reports two siteconfig nodes on one hub with the same boot MAC or BMC
// address, in the same or different clusters. Two BareMetalHosts would then drive one server, and a
// new cluster re-images a running one. Ironic catches only part of this, after the spec is frozen. A
// node's field is the last value the cluster's files set.
type BareMetalCheck struct{}

func (BareMetalCheck) ID() string   { return "POLICY051" }
func (BareMetalCheck) Phase() Phase { return PhaseRepo }

func (c *BareMetalCheck) Run(ctx Context) []Finding {
	type host struct {
		cluster, name, file string
		line                int
	}
	var out []Finding
	seen := map[[3]string]host{}
	for _, f := range ctx.Fleet {
		nodes := map[string]map[string]host{}
		values := map[[2]string]string{}
		var order [][2]string
		for _, file := range f.Files {
			// An unreadable or invalid file fails the render, which RENDER000 reports.
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			var doc yaml.Node
			if yaml.Unmarshal(data, &doc) != nil {
				continue
			}
			m := sourceloc.FindNode(&doc, "install", "siteconfig", "nodes")
			if m == nil || m.Kind != yaml.MappingNode {
				continue
			}
			for i := 0; i+1 < len(m.Content); i += 2 {
				name := m.Content[i].Value
				for _, hf := range hostFields {
					n := sourceloc.FindNode(m.Content[i+1], hf.key)
					if n == nil || n.Kind != yaml.ScalarNode || n.Value == "" {
						continue
					}
					if nodes[name] == nil {
						nodes[name] = map[string]host{}
					}
					key := [2]string{name, hf.key}
					if _, ok := values[key]; !ok {
						order = append(order, key)
					}
					values[key] = hf.norm(n.Value)
					nodes[name][hf.key] = host{f.Name, name, file, n.Line}
				}
			}
		}
		for _, key := range order {
			h := nodes[key[0]][key[1]]
			id := [3]string{f.Hub, key[1], values[key]}
			prior, ok := seen[id]
			if !ok {
				seen[id] = h
				continue
			}
			out = append(out, Finding{
				RuleID: c.ID(), Severity: SevError, Element: "lifecycle", Cluster: f.Name,
				Message: fmt.Sprintf("%s %q of node %s in cluster %s is also node %s in cluster %s (%s:%d), both on hub %s: two BareMetalHosts would drive one server",
					key[1], values[key], h.name, h.cluster, prior.name, prior.cluster, filepath.Base(prior.file), prior.line, f.Hub),
				File: h.file, Line: h.line,
			})
		}
	}
	return out
}
