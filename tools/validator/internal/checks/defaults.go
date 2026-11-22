package checks

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// defaultChange finds the note a key whose default changes in the next major carries in its
// description: "Default changes to <value> in <version>".
var defaultChange = regexp.MustCompile(`\bDefault changes to (.+?) in (\d+\.\d+\.\d+)\b`)

// DefaultChangeCheck (POLICY043) warns where a cluster renders with a default that the next major
// changes and no values file chooses a value. Setting the key, to the old value or the new one,
// settles it. A note under policies.<p> counts only where that Policy renders.
type DefaultChangeCheck struct{}

func (DefaultChangeCheck) ID() string   { return "POLICY043" }
func (DefaultChangeCheck) Phase() Phase { return PhaseCluster }

func (c *DefaultChangeCheck) Run(ctx Context) []Finding {
	if len(ctx.Rendered) == 0 || ctx.Element == nil || ctx.Element.ValuesDoc == nil || ctx.Cluster == nil {
		return nil
	}
	el := ctx.Element
	data, err := os.ReadFile(el.ValuesFile)
	if err != nil {
		return nil
	}
	rendered := PolicyKeys(ctx)
	if len(rendered) == 0 {
		return nil
	}
	layers := slices.DeleteFunc(slices.Clone(ctx.Cluster.ValueFiles), func(f string) bool { return f == el.ValuesFile })
	lines := strings.Split(string(data), "\n")
	var out []Finding
	walkKeys(el.ValuesDoc, nil, func(k *yaml.Node, path []string) {
		m := defaultChange.FindStringSubmatch(description(lines, k))
		// path is stack.<element>.policies.<policy>... for a key of one Policy.
		if m == nil || len(path) > 3 && path[2] == "policies" && !slices.Contains(rendered, path[3]) {
			return
		}
		if file, _ := lastSetting(layers, path...); file != "" {
			return
		}
		out = append(out, Finding{RuleID: c.ID(), Severity: SevWarning, Element: el.ChartName,
			Message: fmt.Sprintf("the default of %s changes to %s in %s: set it in a values layer, to the old or the new value, to choose now",
				strings.Join(path, "."), m[1], m[2]),
			File: el.ValuesFile, Line: k.Line, Col: k.Column})
	})
	return out
}
