package acmtemplates

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PolicyStack/PolicyStack/tools/validator/internal/render"
	"github.com/PolicyStack/PolicyStack/tools/validator/internal/testenv"
)

var loggingDir = filepath.Join(repoRoot, "stack/logging")

// logging resolves ConfigurationPolicy name with the all-on profile, whose inputs hold forwarders
// audit-syslog, app-splunk, infra-http and the disabled retired, plus forwarders on top.
func logging(t *testing.T, forwarders, name string, live ...string) []object {
	t.Helper()
	return resolveTemplate(t, loggingDir, "logging", valuesFile(t, "stack: {logging: {config: {forwarders: "+forwarders+"}}}\n"), name, live...)
}

// names returns kind/name of each object.
func names(objs []object) []string {
	var out []string
	for _, o := range objs {
		out = append(out, o.ObjectDefinition.GetKind()+"/"+o.ObjectDefinition.GetName())
	}
	slices.Sort(out)
	return out
}

// TestLoggingForwardersReady: each forwarder's DaemonSet must have numberReady equal to its live
// desiredNumberScheduled, 1 when the DaemonSet does not exist yet or schedules no pod.
func TestLoggingForwardersReady(t *testing.T) {
	const ds = `apiVersion: apps/v1
kind: DaemonSet
metadata: {name: %s, namespace: openshift-logging}
status: {desiredNumberScheduled: %d}`
	objs := logging(t, "{}", "forwarders-ready", fmt.Sprintf(ds, "audit-syslog", 6), fmt.Sprintf(ds, "app-splunk", 0))
	for name, want := range map[string]string{"audit-syslog": "6", "app-splunk": "1", "infra-http": "1"} {
		got := nested(find(t, objs, "DaemonSet", name), "status", "numberReady")
		if _, isString := got.(string); isString || fmt.Sprint(got) != want {
			t.Errorf("%s numberReady %#v, want integer %s", name, got, want)
		}
		if field(find(t, objs, "ClusterLogForwarder", name), "metadata", "namespace") != "openshift-logging" {
			t.Errorf("%s ClusterLogForwarder not checked in openshift-logging", name)
		}
	}
	if len(objs) != 6 {
		t.Errorf("objects %v, want a ClusterLogForwarder and DaemonSet per enabled forwarder", names(objs))
	}
}

// TestLoggingForwardersWiring: each secret key feeds the field its output type expects, from the
// copied Secret <name>-output.
func TestLoggingForwardersWiring(t *testing.T) {
	const ref = "map[key:%s secretName:%s-output]"
	objs := logging(t, `{web: {type: http, url: "https://w", logTypes: [audit], secret: {name: s, token: t, ca: c}}}`, "forwarders-clusterlogforwarders")
	for _, tt := range []struct {
		name string
		path []string
		want string
	}{
		{"app-splunk", []string{"splunk", "authentication", "token"}, fmt.Sprintf(ref, "hecToken", "app-splunk")},
		{"app-splunk", []string{"splunk", "index"}, "openshift"},
		{"audit-syslog", []string{"syslog", "rfc"}, "RFC5424"},
		{"audit-syslog", []string{"tls", "ca"}, fmt.Sprintf(ref, "ca-bundle.crt", "audit-syslog")},
		{"audit-syslog", []string{"tls", "certificate"}, fmt.Sprintf(ref, "tls.crt", "audit-syslog")},
		{"audit-syslog", []string{"tls", "key"}, fmt.Sprintf(ref, "tls.key", "audit-syslog")},
		{"infra-http", []string{"http", "authentication", "username"}, fmt.Sprintf(ref, "username", "infra-http")},
		{"infra-http", []string{"http", "authentication", "password"}, fmt.Sprintf(ref, "password", "infra-http")},
		{"web", []string{"http", "authentication", "token"}, "map[from:secret secret:map[key:t name:web-output]]"},
		{"web", []string{"tls", "ca"}, fmt.Sprintf(ref, "c", "web")},
	} {
		outputs, _ := nested(find(t, objs, "ClusterLogForwarder", tt.name), "spec", "outputs").([]any)
		if len(outputs) != 1 {
			t.Fatalf("%s outputs %v, want one", tt.name, outputs)
		}
		var got any = outputs[0]
		for _, k := range tt.path {
			got, _ = got.(map[string]any)[k]
		}
		if fmt.Sprint(got) != tt.want {
			t.Errorf("%s %s = %v, want %s", tt.name, strings.Join(tt.path, "."), got, tt.want)
		}
	}
}

// TestLoggingForwardersRemoved: a disabled entry deletes every object it had, an enabled one the
// bindings of log types it no longer lists and its Secret once secret is unset. enabled: null keeps
// a forwarder on, as everywhere else.
func TestLoggingForwardersRemoved(t *testing.T) {
	objs := logging(t, `{infra-http: {secret: null}, app-splunk: {enabled: null}}`, "forwarders-removed")
	want := []string{
		"ClusterLogForwarder/retired",
		"ClusterRoleBinding/app-splunk-collect-audit-logs",
		"ClusterRoleBinding/audit-syslog-collect-application-logs",
		"ClusterRoleBinding/audit-syslog-collect-infrastructure-logs",
		"ClusterRoleBinding/infra-http-collect-application-logs",
		"ClusterRoleBinding/infra-http-collect-audit-logs",
		"ClusterRoleBinding/metadata-reader-openshift-logging-retired-collector",
		"ClusterRoleBinding/retired-collect-application-logs",
		"ClusterRoleBinding/retired-collect-audit-logs",
		"ClusterRoleBinding/retired-collect-infrastructure-logs",
		"Secret/infra-http-output",
		"Secret/retired-output",
		"ServiceAccount/retired-collector",
	}
	if got := names(objs); !slices.Equal(got, want) {
		t.Errorf("mustnothave %v,\nwant %v", got, want)
	}
}

// TestLoggingForwardersRejected: an invalid entry fails the render, naming its path.
func TestLoggingForwardersRejected(t *testing.T) {
	runner := render.New(testenv.Helm(t))
	if err := runner.EnsureDeps(context.Background(), loggingDir); err != nil {
		t.Fatal(err)
	}
	const ok = `type: syslog, url: "tls://s:6514", logTypes: [audit]`
	for _, tt := range []struct{ name, entry, want string }{
		{"loki name", `logging: {` + ok + `}`, "forwarders.logging: logging is the loki element's ClusterLogForwarder"},
		{"uppercase", `Fwd: {` + ok + `}`, "forwarders.Fwd: the name must be 3-54"},
		{"short", `ab: {` + ok + `}`, "forwarders.ab: the name must be 3-54"},
		{"trailing hyphen", `abc-: {` + ok + `}`, "ending with a letter or digit"},
		{"long", strings.Repeat("a", 55) + `: {` + ok + `}`, "the name must be 3-54"},
		{"null entry", `fwd: null`, "forwarders.fwd must be a map"},
		{"type", `fwd: {type: kafka, url: "tls://s:1", logTypes: [audit]}`, `fwd.type: "kafka" must be one of syslog, splunk, http`},
		{"no type", `fwd: {url: "tls://s:1", logTypes: [audit]}`, "fwd.type is required"},
		{"no url", `fwd: {type: syslog, logTypes: [audit]}`, "fwd.url is required"},
		{"syslog scheme", `fwd: {type: syslog, url: "https://s", logTypes: [audit]}`, "fwd.url: syslog needs a tls://, tcp://, udp:// URL"},
		{"splunk scheme", `fwd: {type: splunk, url: "tcp://s:1", logTypes: [audit], secret: {name: s, token: t}}`, "fwd.url: splunk needs a https://, http:// URL"},
		{"no logTypes", `fwd: {type: syslog, url: "tls://s:1"}`, "fwd.logTypes is required"},
		{"logTypes string", `fwd: {type: syslog, url: "tls://s:1", logTypes: audit}`, "fwd.logTypes is required, a list"},
		{"logType", `fwd: {type: syslog, url: "tls://s:1", logTypes: [audits]}`, `fwd.logTypes: "audits" must be one of`},
		{"rfc", `fwd: {` + ok + `, rfc: RFC1}`, `fwd.rfc: "RFC1" must be RFC5424 or RFC3164`},
		{"index on syslog", `fwd: {` + ok + `, index: main}`, "fwd.index is not a syslog setting"},
		{"rfc on http", `fwd: {type: http, url: "https://h", logTypes: [audit], rfc: RFC5424}`, "fwd.rfc is not a http setting"},
		{"unknown key", `fwd: {` + ok + `, spec: {managementState: Managed}}`, "fwd.spec is not a syslog setting"},
		{"index map", `fwd: {type: splunk, url: "https://s", logTypes: [audit], index: {a: b}, secret: {name: s, token: t}}`, "fwd.index must be a string"},
		{"secret string", `fwd: {` + ok + `, secret: s}`, "fwd.secret must be a map"},
		{"token on syslog", `fwd: {` + ok + `, secret: {name: s, token: t}}`, "fwd.secret.token is not used by type syslog"},
		{"no secret name", `fwd: {` + ok + `, secret: {ca: c}}`, "fwd.secret.name is required"},
		{"certificate alone", `fwd: {` + ok + `, secret: {name: s, certificate: c}}`, "fwd.secret: certificate and key are set together"},
		{"empty certificate", `fwd: {` + ok + `, secret: {name: s, certificate: "", key: k}}`, "fwd.secret: certificate and key are set together"},
		{"username alone", `fwd: {type: http, url: "https://h", logTypes: [audit], secret: {name: s, username: u}}`, "fwd.secret: username and password are set together"},
		{"token and basic", `fwd: {type: http, url: "https://h", logTypes: [audit], secret: {name: s, token: t, username: u, password: p}}`, "set token or username and password, not both"},
		{"splunk token", `fwd: {type: splunk, url: "https://s", logTypes: [audit], secret: {name: s, ca: c}}`, "fwd.secret.token is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := "stack: {logging: {config: {forwarders: {" + tt.entry + "}}}}\n"
			tr := runner.Template(context.Background(), loggingDir, "logging", valuesFile(t, body), nil)
			if tr.Err == nil || !strings.Contains(string(tr.Stderr), tt.want) {
				t.Fatalf("want %q, got err %v: %s", tt.want, tr.Err, tr.Stderr)
			}
		})
	}
}

// TestLoggingForwardersClearedKeys: a null in a later layer unsets a key, so an inherited entry can
// change its type and auth mode.
func TestLoggingForwardersClearedKeys(t *testing.T) {
	objs := logging(t, `{app-splunk: {type: syslog, url: "tls://s:6514", index: null, secret: {token: null, ca: ca.crt}}}`, "forwarders-clusterlogforwarders")
	outputs, _ := nested(find(t, objs, "ClusterLogForwarder", "app-splunk"), "spec", "outputs").([]any)
	if got := fmt.Sprint(outputs); !strings.Contains(got, "type:syslog") || strings.Contains(got, "authentication") || !strings.Contains(got, "key:ca.crt") {
		t.Errorf("outputs %s, want syslog with tls.ca and no authentication", got)
	}
}
