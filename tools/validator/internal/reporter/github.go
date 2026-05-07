package reporter

import (
	"fmt"
	"io"
	"strings"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/checks"
)

// GitHub emits ::error / ::warning workflow commands so findings appear
// inline on the PR diff. Format documented at:
// https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions
type GitHub struct {
	// RepoRoot lets us emit paths relative to the workspace, which GitHub
	// requires for inline annotations.
	RepoRoot string
}

func (g GitHub) Write(w io.Writer, findings []checks.Finding) error {
	for _, f := range findings {
		level := "warning"
		if f.Severity == checks.SevError {
			level = "error"
		}
		var props []string
		if rel := relTo(g.RepoRoot, f.File); rel != "" {
			props = append(props, "file="+rel)
		}
		if f.Line > 0 {
			props = append(props, fmt.Sprintf("line=%d", f.Line))
		}
		if f.Col > 0 {
			props = append(props, fmt.Sprintf("col=%d", f.Col))
		}
		props = append(props, "title="+f.RuleID)
		msg := escape(f.Message)
		if f.Element != "" {
			prefix := "[" + f.Element
			if f.Cluster != "" {
				prefix += "/" + f.Cluster
			}
			prefix += "] "
			msg = prefix + msg
		}
		fmt.Fprintf(w, "::%s %s::%s\n", level, strings.Join(props, ","), msg)
	}
	return nil
}

func escape(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// relTo trims the leading repoRoot+/ from path so GitHub can locate the file.
func relTo(repoRoot, path string) string {
	if repoRoot == "" || path == "" {
		return path
	}
	if !strings.HasPrefix(repoRoot, "/") {
		return path
	}
	rr := strings.TrimRight(repoRoot, "/") + "/"
	if strings.HasPrefix(path, rr) {
		return strings.TrimPrefix(path, rr)
	}
	return path
}
