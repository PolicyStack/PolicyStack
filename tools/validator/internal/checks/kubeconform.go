package checks

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// KubeconformCheck (POLICY080) feeds rendered manifests to kubeconform for
// structural validation. Skipped silently when Bin is empty.
type KubeconformCheck struct {
	Bin        string   // e.g. "kubeconform" or absolute path; empty disables check
	SchemasDir string   // optional override (-schema-location)
	Extra      []string // additional CLI args
}

func (KubeconformCheck) ID() string  { return "POLICY080" }
func (KubeconformCheck) Phase() Phase { return PhaseCluster }

func (c *KubeconformCheck) Run(ctx Context) []Finding {
	if c.Bin == "" || ctx.Rendered == nil || len(ctx.Rendered) == 0 {
		return nil
	}
	args := []string{"-strict", "-summary", "-skip", "Policy,ConfigurationPolicy,OperatorPolicy,CertificatePolicy,PolicySet,PlacementBinding,PlacementRule,Placement"}
	if c.SchemasDir != "" {
		args = append([]string{"-schema-location", c.SchemasDir, "-schema-location", "default"}, args...)
	}
	args = append(args, c.Extra...)
	cmd := exec.CommandContext(context.Background(), c.Bin, args...)
	cmd.Stdin = bytes.NewReader(ctx.Rendered)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(stdout.String())
	if msg == "" {
		msg = strings.TrimSpace(stderr.String())
	}
	if msg == "" {
		msg = err.Error()
	}
	return []Finding{{
		RuleID:   "POLICY080",
		Severity: SevError,
		Element:  elementName(ctx),
		Cluster:  clusterName(ctx),
		Message:  fmt.Sprintf("kubeconform: %s", truncate(msg, 800)),
		File:     ctx.Element.ValuesFile,
	}}
}

func clusterName(ctx Context) string {
	if ctx.Cluster != nil {
		return ctx.Cluster.ClusterName
	}
	return ""
}
