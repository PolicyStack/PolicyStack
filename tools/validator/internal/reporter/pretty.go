package reporter

import (
	"fmt"
	"io"
	"os"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/checks"
)

// Pretty writes human-readable findings. Color is disabled when NoColor is
// set, NO_COLOR is set, or w is not a terminal.
type Pretty struct {
	NoColor bool
}

func (p Pretty) Write(w io.Writer, findings []checks.Finding) error {
	useColor := !p.NoColor && os.Getenv("NO_COLOR") == "" && isTerminal(w)
	errorCount, warnCount := 0, 0
	for _, f := range findings {
		switch f.Severity {
		case checks.SevError:
			errorCount++
		case checks.SevWarning:
			warnCount++
		}
		if err := writeOne(w, f, useColor); err != nil {
			return err
		}
	}
	if errorCount == 0 && warnCount == 0 {
		_, err := fmt.Fprintln(w, color("ok: no findings", useColor, ansiGreen))
		return err
	}
	_, err := fmt.Fprintf(w, "\n%d error(s), %d warning(s)\n", errorCount, warnCount)
	return err
}

func writeOne(w io.Writer, f checks.Finding, useColor bool) error {
	tag := "warning"
	c := ansiYellow
	if f.Severity == checks.SevError {
		tag = "error"
		c = ansiRed
	}
	loc := f.File
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		if f.Col > 0 {
			loc = fmt.Sprintf("%s:%d", loc, f.Col)
		}
	}
	target := ""
	if f.Element != "" {
		target = "[" + f.Element
		if f.Cluster != "" {
			target += "/" + f.Cluster
		}
		target += "] "
	}
	_, err := fmt.Fprintf(w, "%s %s %s%s\n    at %s\n",
		color(tag, useColor, c),
		color(f.RuleID, useColor, ansiBold),
		target,
		f.Message,
		loc,
	)
	return err
}

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiGreen  = "\x1b[32m"
)

func color(s string, on bool, code string) string {
	if !on {
		return s
	}
	return code + s + ansiReset
}

// isTerminal reports whether w is a character device. Avoids a dependency
// on x/term.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}
