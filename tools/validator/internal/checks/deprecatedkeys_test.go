package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/fixtures"
)

func TestPolicy039_DeprecatedKeys(t *testing.T) {
	metallb := loadElement(t, `stack:
  metallb:
    enabled: false
    config:
      # Address pools by name.
      # Deprecated: use config.ipAddressPools; removed in 2.0.0.
      pools: {}
      ipAddressPools: {}
      # Deprecated: use config.peers; removed in 2.0.0

      bgp: {}
      peers: {}
      # Old toggle. Deprecated: ask in chat
      legacy: false
      nested:
        # Deprecated: use config.nested.newer; removed in 1.5.0
        older: 1
        newer: 1
      list:
        - # Deprecated: use config.x; removed in 2.0.0
          item: 1
      flow: {a: 1, b: 2}
`, "apiVersion: v2\nname: metallb\nversion: 1.2.0\n", nil)
	lifecycle := loadElement(t, `stack:
  hypershift: {enabled: true}
install:
  # Deprecated: use install.workers; removed in v2.0.0
  nodes: 2
  workers: 2
`, "apiVersion: v2\nname: lifecycle\nversion: 1.0.0\n", nil)

	dir := t.TempDir()
	write := func(name, data string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	layers := []string{
		write("values.yaml", "stack:\n  metallb:\n    config:\n      pools: {a: {}}\n"),
		write("current.yaml", "stack:\n  metallb:\n    config:\n      ipAddressPools: {}\n      bgp: {}\n      nested: {newer: 2}\n"),
		write("nested.yaml", "stack:\n  metallb:\n    config:\n      nested:\n        older: 2\n"),
		write("lifecycle.yaml", "install:\n  nodes: 3\n"),
		write("unrelated.yaml", "stack:\n  other:\n    config:\n      pools: {}\nconfig:\n  pools: {}\n"),
		write("invalid.yaml", "stack: [\n"),
	}
	clusters := []*fixtures.Cluster{{SourceFile: write("fleet.yaml", "revision: main\nhub: acm-dc1\ninstall:\n  nodes: 1\n")}}

	// lifecycle twice: its engine components share one values.yaml.
	got := (&DeprecatedKeyCheck{}).Run(Context{AllElements: []*chart.Element{metallb, lifecycle, lifecycle}, Layers: layers, Clusters: clusters})
	var lines []string
	for _, f := range got {
		if f.RuleID != "POLICY039" || f.Severity != SevWarning {
			t.Errorf("unexpected finding: %+v", f)
		}
		lines = append(lines, fmt.Sprintf("%s %s:%d:%d: %s", f.Element, filepath.Base(f.File), f.Line, f.Col, f.Message))
	}
	want := []string{
		`metallb values.yaml:14:7: stack.metallb.config.legacy: write the deprecation as "Deprecated: use <key>; removed in <version>", not "Deprecated: ask in chat"`,
		`metallb values.yaml:4:7: stack.metallb.config.pools is deprecated and removed in 2.0.0: use config.ipAddressPools instead`,
		`metallb nested.yaml:5:9: stack.metallb.config.nested.older is deprecated and removed in 1.5.0: use config.nested.newer instead`,
		`lifecycle lifecycle.yaml:2:3: install.nodes is deprecated and removed in v2.0.0: use install.workers instead`,
		`lifecycle fleet.yaml:4:3: install.nodes is deprecated and removed in v2.0.0: use install.workers instead`,
	}
	if !slices.Equal(lines, want) {
		t.Errorf("got\n%q\nwant\n%q", lines, want)
	}

	// The key stays known: POLICY035 reports nothing for the layer setting it.
	if fs := (&LayerKeyCheck{}).Run(Context{AllElements: []*chart.Element{metallb}, Layers: layers[:1]}); len(fs) != 0 {
		t.Errorf("POLICY035 on a deprecated key: %+v", fs)
	}

	if fs := (&DeprecatedKeyCheck{}).Run(Context{AllElements: []*chart.Element{lifecycle}}); len(fs) != 0 {
		t.Errorf("no layers: want no findings, got %+v", fs)
	}
}
