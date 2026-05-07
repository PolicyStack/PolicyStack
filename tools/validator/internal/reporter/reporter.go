// Package reporter renders Findings to terminal or GitHub Actions format.
package reporter

import (
	"io"
	"sort"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/checks"
)

// Reporter writes findings to w.
type Reporter interface {
	Write(w io.Writer, findings []checks.Finding) error
}

// SortFindings orders findings deterministically: severity (errors first),
// then RuleID, element, cluster, file, line.
func SortFindings(f []checks.Finding) {
	sort.SliceStable(f, func(i, j int) bool {
		a, b := f[i], f[j]
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.Element != b.Element {
			return a.Element < b.Element
		}
		if a.Cluster != b.Cluster {
			return a.Cluster < b.Cluster
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
}
