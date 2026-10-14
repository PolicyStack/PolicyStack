// Package render runs `helm` to render an element chart for a resolved values
// cascade. Values merging and templating are left to helm: files are passed
// with -f in cascade order, as the ApplicationSet does.
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

	"gopkg.in/yaml.v3"
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
// process, and not at all when charts/ already holds every Chart.yaml
// dependency at its pinned version. Later calls return the first call's error.
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

// alreadyDownloaded reports whether charts/<name>-<version>.tgz exists for every dependency, so a
// stale archive left by an earlier pin is never rendered in place of the pinned one.
func alreadyDownloaded(elementDir string) bool {
	data, err := os.ReadFile(filepath.Join(elementDir, "Chart.yaml"))
	if err != nil {
		return false
	}
	var c struct {
		Dependencies []struct{ Name, Version string }
	}
	if yaml.Unmarshal(data, &c) != nil {
		return false
	}
	for _, d := range c.Dependencies {
		if !FileExists(filepath.Join(elementDir, "charts", d.Name+"-"+d.Version+".tgz")) {
			return false
		}
	}
	return true
}

// TemplateResult is the captured output of a helm template invocation.
type TemplateResult struct {
	Stdout  []byte
	Err     error  // non-nil if helm exited non-zero or the command failed to start
	Stderr  string // helm's stderr, trimmed
	ErrFile string // file extracted from "Error: ... in \"<file>\" line N"
	ErrLine int
}

// Template runs `helm template <release> <element> -f <values...> --set-string <setString...>`.
// setString entries are key=value and rank above every values file, as valuesObject does.
func (r *Runner) Template(ctx context.Context, elementDir, releaseName string, valueFiles, setString []string) TemplateResult {
	args := []string{"template", releaseName, elementDir}
	for _, vf := range valueFiles {
		args = append(args, "-f", vf)
	}
	for _, kv := range setString {
		args = append(args, "--set-string", kv)
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

// FileExists reports whether p exists.
func FileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
