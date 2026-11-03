// Package testenv holds test prerequisites. A missing one skips the test locally and fails it under
// CI, where a skip would pass silently.
package testenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Skip skips t, or fails it when CI is set.
func Skip(t testing.TB, reason string) {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Fatal(reason)
	}
	t.Skip(reason)
}

// Helm returns the path of helm, calling Skip when it is not on PATH.
func Helm(t testing.TB) string {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		Skip(t, "helm not on PATH")
	}
	return helm
}

// Git runs git in dir, without user or system config, and returns its trimmed output.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...) //nolint:gosec // G204: test helper, fixed binary
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Commit writes files, paths relative to dir, into the git repository at dir, initializing it on
// first use, and commits them. Returns the commit SHA.
func Commit(t testing.TB, dir string, files ...string) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		Git(t, dir, "init", "-q")
	}
	for _, f := range files {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	Git(t, dir, "add", "-A")
	Git(t, dir, "commit", "-q", "--allow-empty", "-m", "c")
	return Git(t, dir, "rev-parse", "HEAD")
}
