// Package checks defines the Check interface and the rule registry. Each rule
// is in its own file; All is the authoritative list of rule IDs.
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
	// PhaseCluster runs once per (element, fixture) with the rendered manifests.
	PhaseCluster
	// PhaseRepo runs once with every element loaded (e.g. dependency pinning).
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

// Context is the input to Check.Run.
type Context struct {
	Element  *chart.Element
	Cluster  *cascade.Resolved
	Rendered []byte // helm template stdout, only populated for PhaseCluster
	// AllElements is set for PhaseRepo only.
	AllElements []*chart.Element
	// AllRendered is not populated. Cross-element POLICY002 runs in run.Run.
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
		&NameLengthCheck{},       // POLICY001
		&DuplicateNameCheck{},    // POLICY002
		&SubPolicyNameCheck{},    // POLICY003
		&PolicyRefCheck{},        // POLICY010
		&DependencyCheck{},       // POLICY011
		&MissingConverterCheck{}, // POLICY020
		&UnusedConverterCheck{},  // POLICY021
		&EnumCheck{},             // POLICY030
		&DeadKeyCheck{},          // POLICY031
		&LayerOperatorCheck{},    // POLICY032
		&PruneCheck{},            // POLICY033
		&LayerFleetKeyCheck{},    // POLICY034
		&PolicySetCheck{},        // POLICY040
		&LabelCheck{},            // POLICY050
		&PinningCheck{},          // POLICY060
		&LintCheck{},             // POLICY070 (Runner set by run.buildRegistry)
		&KubeconformCheck{},      // POLICY080 (Bin, SchemasDir set by run.buildRegistry)
		&ACMTemplateCheck{},      // POLICY081
		&CamelCaseCheck{},        // POLICY090
		&ElementNameCheck{},      // POLICY091
		&RenderCheck{},           // RENDER000
	}
}
