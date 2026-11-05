package checks

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestRulesDocumented: docs/reference/rules.md renders the README rules table, and the table has one
// row per registered rule ID.
func TestRulesDocumented(t *testing.T) {
	doc, err := os.ReadFile("../../../../docs/reference/rules.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `--8<-- "tools/validator/README.md:rules"`) {
		t.Fatal("docs/reference/rules.md does not include the tools/validator/README.md rules section")
	}
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, rules, _ := strings.Cut(string(readme), "--8<-- [start:rules]")
	rules, _, _ = strings.Cut(rules, "--8<-- [end:rules]")
	var documented []string
	for _, m := range regexp.MustCompile(`(?m)^\|\s*([A-Z]+\d{3})\s*\|`).FindAllStringSubmatch(rules, -1) {
		documented = append(documented, m[1])
	}
	var registered []string
	for _, c := range All() {
		registered = append(registered, c.ID())
	}
	slices.Sort(documented)
	slices.Sort(registered)
	if !slices.Equal(documented, registered) {
		t.Errorf("README rules table\n got %q\nwant %q (checks.All)", documented, registered)
	}
}
