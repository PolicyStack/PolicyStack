// Package checks defines the Check interface and the rule registry. Each
// rule lives in its own file in this package so the test surface stays
// small and the registry stays the single source of truth for rule IDs.
package checks

import (
	"log/slog"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/cascade"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/chart"
)

// Severity controls whether a Finding fails the run by default.
type Severity int

const (
	SevWarning Severity = iota
	SevError
)

func (s Severity) String() string {
	switch s {
	case SevError:
		return "error"
	default:
		return "warning"
	}
}

// Phase describes when a check runs.
type Phase int

const (
	// PhaseChart runs once per element (no cluster/render context).
	PhaseChart Phase = iota
	// PhaseCluster runs once per (element, fixture) and may consume rendered manifests.
	PhaseCluster
	// PhaseRepo runs once across all elements (e.g. dependency-pinning consistency).
	PhaseRepo
)

// Finding is a single rule violation.
type Finding struct {
	RuleID   string
	Severity Severity
	Element  string // chart name, may be empty for repo-wide
	Cluster  string // fixture cluster name, empty for chart/repo-wide
	Message  string
	File     string
	Line     int
	Col      int
}

// Context is what each check receives.
type Context struct {
	Element  *chart.Element
	Cluster  *cascade.Resolved
	Rendered []byte // helm template stdout, only populated for PhaseCluster
	// AllElements is set for PhaseRepo so checks can reason globally.
	AllElements []*chart.Element
	// AllRendered is the per-cluster rendered output keyed by element+cluster.
	// Used by POLICY002 cross-element duplicate detection.
	AllRendered map[string][]byte
	Logger      *slog.Logger
}

// Check is implemented by every concrete rule.
type Check interface {
	ID() string
	Phase() Phase
	Run(Context) []Finding
}

// All returns the registered checks in stable order.
func All() []Check {
	return []Check{
		&NameLengthCheck{},     // POLICY001
		&DuplicateNameCheck{},  // POLICY002
		&PolicyRefCheck{},      // POLICY010
		&MissingConverterCheck{}, // POLICY020
		&UnusedConverterCheck{},  // POLICY021
		&EnumCheck{},           // POLICY030
		&PolicySetCheck{},      // POLICY040
		&LabelCheck{},          // POLICY050
		&PinningCheck{},        // POLICY060
		&LintCheck{},           // POLICY070 (set HelmBin via runtime)
		&KubeconformCheck{},    // POLICY080 (set Bin/SchemasDir via runtime)
		&CamelCaseCheck{},      // POLICY090
		&RenderCheck{},         // RENDER000
	}
}
