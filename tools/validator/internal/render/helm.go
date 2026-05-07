// Package render shells out to `helm` to materialize an element chart for a
// given resolved values cascade. It does NOT re-implement helm's merge or
// template logic — passing -f files in cascade order matches what
// ApplicationSet does at runtime, so any rendering divergence is a helm bug
// rather than ours.
package render

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Runner caches `helm dependency update` per element and runs `helm template`.
type Runner struct {
	HelmBin string

	depOnce sync.Map // map[elementDir]*sync.Once
	depErr  sync.Map // map[elementDir]error
}

// New returns a Runner; helmBin defaults to "helm" if empty.
func New(helmBin string) *Runner {
	if helmBin == "" {
		helmBin = "helm"
	}
	return &Runner{HelmBin: helmBin}
}

// EnsureDeps runs `helm dependency update` on elementDir at most once per
// process. Cached error is returned on subsequent calls.
func (r *Runner) EnsureDeps(ctx context.Context, elementDir string) error {
	v, _ := r.depOnce.LoadOrStore(elementDir, &sync.Once{})
	once := v.(*sync.Once)
	once.Do(func() {
		if alreadyDownloaded(elementDir) {
			return
		}
		cmd := exec.CommandContext(ctx, r.HelmBin, "dependency", "update", elementDir)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			r.depErr.Store(elementDir, fmt.Errorf("helm dependency update %s: %w: %s", elementDir, err, strings.TrimSpace(stderr.String())))
		}
	})
	if v, ok := r.depErr.Load(elementDir); ok {
		return v.(error)
	}
	return nil
}

func alreadyDownloaded(elementDir string) bool {
	matches, _ := filepath.Glob(filepath.Join(elementDir, "charts", "*.tgz"))
	return len(matches) > 0
}

// TemplateResult is the captured output of a helm template invocation.
type TemplateResult struct {
	Stdout    []byte
	Err       error  // non-nil if helm exited non-zero or the command failed to start
	Stderr    string // helm's stderr, trimmed
	ErrFile   string // file extracted from "Error: ... in \"<file>\" line N"
	ErrLine   int
}

// Template runs `helm template <release> <element> -f <values...>`.
func (r *Runner) Template(ctx context.Context, elementDir, releaseName string, valueFiles []string) TemplateResult {
	args := []string{"template", releaseName, elementDir}
	for _, vf := range valueFiles {
		args = append(args, "-f", vf)
	}
	cmd := exec.CommandContext(ctx, r.HelmBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := TemplateResult{Stdout: stdout.Bytes(), Stderr: strings.TrimSpace(stderr.String()), Err: err}
	if err != nil {
		res.ErrFile, res.ErrLine = parseHelmError(res.Stderr)
	}
	return res
}

// Lint runs `helm lint <element>`. Returns non-nil err on lint failure.
func (r *Runner) Lint(ctx context.Context, elementDir string) (output string, err error) {
	cmd := exec.CommandContext(ctx, r.HelmBin, "lint", elementDir)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err = cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

var helmErrFileLine = regexp.MustCompile(`in "([^"]+)" line (\d+)`)

func parseHelmError(stderr string) (string, int) {
	m := helmErrFileLine.FindStringSubmatch(stderr)
	if m == nil {
		return "", 0
	}
	line, _ := strconv.Atoi(m[2])
	return m[1], line
}

// HelmAvailable returns nil if `helm` is found on PATH.
func HelmAvailable(helmBin string) error {
	if helmBin == "" {
		helmBin = "helm"
	}
	if _, err := exec.LookPath(helmBin); err != nil {
		return fmt.Errorf("%s not found on PATH: %w", helmBin, err)
	}
	return nil
}

// FileExists is a convenience for callers building cascades.
func FileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
