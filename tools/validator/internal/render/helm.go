// Package render runs `helm` to render an element chart for a resolved values
// cascade. Values merging and templating are left to helm: files are passed
// with -f in cascade order, as the ApplicationSet does.
package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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

// EnsureDeps runs `helm dependency update` on elementDir at most once per process, and not at all
// when upToDate. Later calls return the first call's error. --skip-refresh only when every
// dependency is file://, which needs no repository index. A remote one, as in every revision before
// lib/ that Render Diff renders, needs its index fetched: a fresh Helm cache has none.
func (r *Runner) EnsureDeps(ctx context.Context, elementDir string) error {
	v, _ := r.depOnce.LoadOrStore(elementDir, &sync.Once{})
	once := v.(*sync.Once)
	once.Do(func() {
		deps, ok := readDeps(elementDir)
		local := ok && !slices.ContainsFunc(deps, func(d dependency) bool { return !strings.HasPrefix(d.Repository, "file://") })
		if local && upToDate(elementDir, deps) {
			return
		}
		args := []string{"dependency", "update"}
		if local {
			args = append(args, "--skip-refresh")
		}
		cmd := exec.CommandContext(ctx, r.HelmBin, append(args, elementDir)...) //nolint:gosec // G204: no shell; the binary is --helm-bin
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

type dependency struct{ Name, Repository string }

// readDeps returns the dependencies in dir/Chart.yaml. An unreadable Chart.yaml is !ok, leaving the
// error to helm.
func readDeps(dir string) (deps []dependency, ok bool) {
	data, err := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
	var c struct{ Dependencies []dependency }
	if err != nil || yaml.Unmarshal(data, &c) != nil {
		return nil, false
	}
	return c.Dependencies, true
}

// upToDate reports whether charts/ holds exactly one archive per file:// dependency, newer than
// every file under its source directory, so an update would rewrite nothing. Skipping it keeps
// parallel renders of the same chart, as in concurrent test packages, from rewriting charts/ under
// each other. A git checkout sets mtimes to checkout time, so a changed lib/ is never missed.
func upToDate(dir string, deps []dependency) bool {
	for _, d := range deps {
		archives, _ := filepath.Glob(filepath.Join(dir, "charts", d.Name+"-*.tgz"))
		if len(archives) != 1 {
			return false
		}
		built, err := os.Stat(archives[0])
		if err != nil {
			return false
		}
		src := filepath.Join(dir, strings.TrimPrefix(d.Repository, "file://"))
		errStale := errors.New("stale")
		err = filepath.WalkDir(src, func(_ string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if info, err := e.Info(); err != nil || info.ModTime().After(built.ModTime()) {
				return errStale
			}
			return nil
		})
		if err != nil {
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
	cmd := exec.CommandContext(ctx, r.HelmBin, args...) //nolint:gosec // G204: no shell; the binary is --helm-bin
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

// Lint runs `helm lint <element> <args...>`. Returns non-nil err on lint failure.
func (r *Runner) Lint(ctx context.Context, elementDir string, args ...string) (output string, err error) {
	cmd := exec.CommandContext(ctx, r.HelmBin, append([]string{"lint", elementDir}, args...)...) //nolint:gosec // G204: no shell; the binary is --helm-bin
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
